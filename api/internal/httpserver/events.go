package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
	"unicode/utf8"

	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/realtime"
)

const defaultHeartbeat = 20 * time.Second

// projectEvents streams project events as SSE: ops {seq}, presence {list}, runs, access {role}, deleted, reset.
// The first events are the current seq, which the client compares with its own, and the presence list.
// Access is checked on connect and on every access event for this user; the session on each heartbeat.
func (s *Server) projectEvents(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		writeError(w, r, http.StatusServiceUnavailable, "Живые обновления недоступны. Обновите страницу позже.")
		return
	}
	row := projectFrom(r)
	ident := auth.FromRequest(r)
	sub, err := s.hub.Subscribe(row.ID, ident.UserID)
	if errors.Is(err, realtime.ErrTooManyStreams) {
		writeError(w, r, http.StatusTooManyRequests, "Открыто слишком много вкладок с проектами. Закройте лишние.")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "Сервер перезапускается. Подключение восстановится само.")
		return
	}
	defer sub.Close()
	ctx := r.Context()
	// Subscribed first, then read: an ops event racing with this read is either in seq or queued in sub.C.
	seq, err := s.q.GetProjectDraftSeq(ctx, row.ID)
	if err != nil {
		s.internal(w, r, "events.seq", err, "Не удалось подключиться к проекту. Повторите запрос.")
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	send := func(event string, data json.RawMessage) bool {
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send(realtime.EventOps, mustJSON(map[string]int64{"seq": seq})) || !send(realtime.EventPresence, s.hub.Presence(row.ID)) {
		return
	}

	beat := s.heartbeat
	if beat <= 0 {
		beat = defaultHeartbeat
	}
	ticker := time.NewTicker(beat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.Done():
			return
		case <-ticker.C:
			active, err := s.q.SessionActive(ctx, db.SessionActiveParams{
				ID: ident.SessionID, IdleSeconds: int32(s.cfg.SessionIdle / time.Second),
			})
			if err != nil || !active {
				return
			}
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		case ev := <-sub.C:
			switch ev.Type {
			case realtime.EventAccess:
				if ev.User != ident.UserID {
					continue
				}
				role := s.currentRole(r, row)
				if !send(realtime.EventAccess, mustJSON(map[string]*string{"role": role})) || role == nil {
					return
				}
			case realtime.EventDeleted:
				send(realtime.EventDeleted, ev.Data)
				return
			default:
				if !send(ev.Type, ev.Data) {
					return
				}
			}
		}
	}
}

// currentRole re-reads the caller's role; nil means the access is gone.
func (s *Server) currentRole(r *http.Request, row db.GetProjectRow) *string {
	acc, err := s.q.GetProjectAccess(r.Context(), db.GetProjectAccessParams{UserID: auth.FromRequest(r).UserID, ProjectID: row.ID})
	if err != nil || acc.DeletedAt.Valid {
		return nil
	}
	return &acc.Role
}

type presenceBody struct {
	ClientID  string          `json:"client_id"`
	Route     string          `json:"route"`
	Selection json.RawMessage `json:"selection"`
	Leave     bool            `json:"leave"`
}

type presenceSelection struct {
	Coll string `json:"coll"`
	ID   string `json:"id"`
}

// postPresence reports where a tab is in the project. Every instance's hub hears it through NOTIFY.
func (s *Server) postPresence(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	ident := auth.FromRequest(r)
	var body presenceBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeError(w, r, http.StatusBadRequest, "Некорректный JSON. Проверьте тело запроса.")
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, r, http.StatusBadRequest, "Тело запроса должно содержать один JSON-объект.")
		return
	}
	if body.ClientID == "" || utf8.RuneCountInString(body.ClientID) > txClientIDMax || utf8.RuneCountInString(body.Route) > 200 {
		writeError(w, r, http.StatusBadRequest, "Укажите client_id до 64 символов и route до 200.")
		return
	}
	selection := json.RawMessage("null")
	if len(body.Selection) > 0 && string(body.Selection) != "null" {
		var sel presenceSelection
		dec := json.NewDecoder(bytes.NewReader(body.Selection))
		dec.DisallowUnknownFields()
		if dec.Decode(&sel) != nil || dec.Decode(&struct{}{}) != io.EOF || sel.Coll == "" || len(sel.Coll) > 64 || len(sel.ID) > 64 {
			writeError(w, r, http.StatusBadRequest, "selection должен быть {coll, id} или null.")
			return
		}
		selection = mustJSON(sel)
	}
	msg := map[string]any{
		"project_id": row.ID.String(), "client_id": body.ClientID, "user_id": ident.UserID.String(), "email": ident.Email,
		"route": body.Route, "selection": selection, "leave": body.Leave,
	}
	if err := s.q.NotifyProjectPresence(r.Context(), string(mustJSON(msg))); err != nil {
		s.internal(w, r, "presence.notify", err, "Не удалось отметить присутствие. Повторите запрос.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
