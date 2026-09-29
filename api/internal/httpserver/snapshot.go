package httpserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/ops"
)

const (
	opsPageMax = 500
	// OpsRetention is how long the journal serves incremental sync. A client away longer takes a snapshot.
	OpsRetention = 30 * 24 * time.Hour
)

type snapshotJSON struct {
	SchemaVersion int       `json:"schema_version"`
	Seq           int64     `json:"seq"`
	Collections   ops.State `json:"collections"`
}

// getSnapshot returns every record of the project and the seq they belong to.
func (s *Server) getSnapshot(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	var out snapshotJSON
	err := s.inProjectTx(r.Context(), row.ID, func(tx *Server) error {
		loaded, err := tx.loadOpsState(r.Context(), row)
		if err != nil {
			return err
		}
		out = snapshotJSON{SchemaVersion: ops.ProjectSchema().Version, Seq: loaded.seq, Collections: loaded.state}
		return nil
	})
	if err != nil {
		s.internal(w, r, "ops.snapshot", err, "Не удалось загрузить проект. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type opEntryJSON struct {
	Seq        int64           `json:"seq"`
	TxID       string          `json:"tx_id"`
	ClientID   string          `json:"client_id"`
	Actor      *string         `json:"actor"`
	ActorEmail *string         `json:"actor_email"`
	Label      string          `json:"label"`
	Ops        json.RawMessage `json:"ops"`
	CreatedAt  time.Time       `json:"created_at"`
}

type opsPageJSON struct {
	Items []opEntryJSON `json:"items"`
	Seq   int64         `json:"seq"`
	Reset bool          `json:"reset"`
}

// hasReset reports a legacy write in a journal entry. JSONB output is reformatted, so the ops are decoded.
func hasReset(raw json.RawMessage) bool {
	var list []struct {
		Op string `json:"op"`
	}
	if json.Unmarshal(raw, &list) != nil {
		return true
	}
	for _, o := range list {
		if o.Op == ops.OpReset {
			return true
		}
	}
	return false
}

// listOperations returns accepted transactions after a seq. reset tells the client to take a snapshot instead:
// the range holds a legacy write, or part of it is no longer kept.
func (s *Server) listOperations(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	after, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if err != nil || after < 0 {
		writeError(w, r, http.StatusBadRequest, "after должен быть целым числом >= 0.")
		return
	}
	limit := opsPageMax
	if v := r.URL.Query().Get("limit"); v != "" {
		if limit, err = strconv.Atoi(v); err != nil || limit < 1 || limit > opsPageMax {
			writeError(w, r, http.StatusBadRequest, "limit должен быть от 1 до 500.")
			return
		}
	}
	ctx := r.Context()
	seq, err := s.q.GetProjectDraftSeq(ctx, row.ID)
	if err != nil {
		s.internal(w, r, "ops.list.seq", err, "Не удалось загрузить изменения. Повторите запрос.")
		return
	}
	out := opsPageJSON{Items: []opEntryJSON{}, Seq: seq}
	if after > seq {
		out.Reset = true
		writeJSON(w, http.StatusOK, out)
		return
	}
	rows, err := s.q.ListProjectOperationsAfter(ctx, db.ListProjectOperationsAfterParams{ProjectID: row.ID, After: after, MaxRows: int32(limit)})
	if err != nil {
		s.internal(w, r, "ops.list", err, "Не удалось загрузить изменения. Повторите запрос.")
		return
	}
	next := after + 1
	for _, o := range rows {
		if o.Seq != next || hasReset(o.Ops) {
			out.Reset, out.Items = true, []opEntryJSON{}
			break
		}
		next++
		e := opEntryJSON{Seq: o.Seq, TxID: o.TxID.String(), ClientID: o.ClientID, Label: o.Label, Ops: o.Ops, CreatedAt: o.CreatedAt.Time}
		if o.ActorUserID != nil {
			a := o.ActorUserID.String()
			e.Actor = &a
			e.ActorEmail = o.ActorEmail
		}
		out.Items = append(out.Items, e)
	}
	if len(rows) == 0 && after < seq {
		out.Reset = true
	}
	writeJSON(w, http.StatusOK, out)
}
