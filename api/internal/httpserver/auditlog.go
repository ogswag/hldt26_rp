package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"moscow_hackathon_2026/api/internal/audit"
	"moscow_hackathon_2026/api/internal/db"
)

const auditListCap = 500

func (s *Server) recordAudit(ctx context.Context, reqID string, e audit.Event) error {
	e.RequestID = reqID
	return audit.Record(ctx, s.q, e)
}

func (s *Server) inTx(ctx context.Context, fn func(*Server) error) error {
	tx, err := s.q.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("db.tx.begin: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	cp := *s
	cp.q = s.q.WithTx(tx)
	if err := fn(&cp); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db.tx.commit: %w", err)
	}
	return nil
}

type auditItem struct {
	ID         int64      `json:"id"`
	At         *time.Time `json:"at"`
	ActorEmail *string    `json:"actor_email"`
	Action     string     `json:"action"`
	TargetType string     `json:"target_type"`
	TargetID   *string    `json:"target_id"`
	// TargetName is the current name of a project or catalog robot the event is about; null once it is gone.
	TargetName *string         `json:"target_name"`
	Meta       json.RawMessage `json:"meta"`
}

func (s *Server) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	limit := int32(100)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > auditListCap {
			writeError(w, r, http.StatusBadRequest, "limit должен быть числом от 1 до 500.")
			return
		}
		limit = int32(n)
	}
	var targetType, targetID *string
	if v := r.URL.Query().Get("target_type"); v != "" {
		targetType = &v
	}
	if v := r.URL.Query().Get("target_id"); v != "" {
		targetID = &v
	}
	actions := codeList(r.URL.Query().Get("action"))
	if len(actions) > 50 {
		writeError(w, r, http.StatusBadRequest, "В фильтре больше 50 действий. Сократите список.")
		return
	}
	var before *int64
	if raw := r.URL.Query().Get("before"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 1 {
			writeError(w, r, http.StatusBadRequest, "before должен быть номером записи журнала.")
			return
		}
		before = &n
	}
	rows, err := s.q.ListAuditEvents(r.Context(), db.ListAuditEventsParams{
		TargetType: targetType,
		TargetID:   targetID,
		Actions:    actions,
		BeforeID:   before,
		MaxRows:    limit,
	})
	if err != nil {
		s.internal(w, r, "audit.list", err, "Не удалось загрузить журнал. Повторите запрос.")
		return
	}
	items := make([]auditItem, 0, len(rows))
	for _, row := range rows {
		var actor *string
		if row.ActorEmail != nil {
			masked := audit.Mask(*row.ActorEmail)
			actor = &masked
		}
		name := row.ProjectName
		if name == nil {
			name = row.SolutionName
		}
		items = append(items, auditItem{
			ID:         row.ID,
			At:         timestamptzPtr(row.At),
			ActorEmail: actor,
			Action:     row.Action,
			TargetType: row.TargetType,
			TargetID:   row.TargetID,
			TargetName: name,
			Meta:       row.Meta,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
