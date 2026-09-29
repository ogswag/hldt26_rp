package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/ops"
)

const (
	txBatchMax     = 200
	txBodyLimit    = 1 << 20
	txLabelMax     = 200
	txClientIDMax  = 64
	txOpsPerTxMax  = 5000
	txBodyTooLarge = "Пачка изменений больше 1 МБ. Отправьте её частями."
)

type txBatchBody struct {
	ClientID      string        `json:"client_id"`
	SchemaVersion int           `json:"schema_version"`
	Txs           []txEntryBody `json:"txs"`
}

type txEntryBody struct {
	TxID  string          `json:"tx_id"`
	Label string          `json:"label"`
	Ops   json.RawMessage `json:"ops"`
}

type txResultJSON struct {
	TxID    string       `json:"tx_id"`
	Status  string       `json:"status"`
	Seq     *int64       `json:"seq,omitempty"`
	Reason  string       `json:"reason,omitempty"`
	Details *ops.Details `json:"details,omitempty"`
}

type txBatchJSON struct {
	Results []txResultJSON `json:"results"`
	Seq     int64          `json:"seq"`
}

// parsedTx is one entry of the batch after the envelope checks.
type parsedTx struct {
	id    uuid.UUID
	label string
	raw   json.RawMessage
	tx    ops.Tx
	bad   *ops.Details
}

// postTransactions applies a batch of client transactions in order. Each one is applied whole or rejected whole;
// a tx_id seen before returns its stored outcome, so a retry after a lost response is safe.
func (s *Server) postTransactions(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	var body txBatchBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, txBodyLimit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, r, http.StatusRequestEntityTooLarge, txBodyTooLarge)
			return
		}
		writeError(w, r, http.StatusBadRequest, "Некорректный JSON. Проверьте тело запроса.")
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, r, http.StatusBadRequest, "Тело запроса должно содержать один JSON-объект.")
		return
	}
	schema := ops.ProjectSchema()
	if body.SchemaVersion < 1 || body.SchemaVersion > schema.Version {
		writeCode(w, r, http.StatusConflict, ops.ReasonSchemaVersion,
			fmt.Sprintf("Версия схемы %d, сервер принимает версии от 1 до %d. Обновите страницу.", body.SchemaVersion, schema.Version))
		return
	}
	if body.ClientID == "" || utf8.RuneCountInString(body.ClientID) > txClientIDMax {
		writeError(w, r, http.StatusBadRequest, "Укажите client_id до 64 символов.")
		return
	}
	if len(body.Txs) > txBatchMax {
		writeError(w, r, http.StatusBadRequest, fmt.Sprintf("В пачке больше %d транзакций. Отправьте её частями.", txBatchMax))
		return
	}
	txs := make([]parsedTx, 0, len(body.Txs))
	for _, e := range body.Txs {
		id, err := uuid.Parse(e.TxID)
		if err != nil || id.String() != e.TxID {
			writeError(w, r, http.StatusBadRequest, "tx_id должен быть UUID в нижнем регистре.")
			return
		}
		if utf8.RuneCountInString(e.Label) > txLabelMax {
			writeError(w, r, http.StatusBadRequest, "label транзакции длиннее 200 символов.")
			return
		}
		txs = append(txs, parseTx(id, e))
	}
	var out txBatchJSON
	err := s.inProjectTx(r.Context(), row.ID, func(tx *Server) error {
		var err error
		out, err = tx.applyBatch(r.Context(), row, body.ClientID, txs)
		return err
	})
	if err != nil {
		s.internal(w, r, "ops.transactions", err, "Не удалось применить изменения. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// parseTx decodes the operations strictly. A malformed operation rejects its transaction, not the batch, so one
// bad entry cannot block a client's queue.
func parseTx(id uuid.UUID, e txEntryBody) parsedTx {
	p := parsedTx{id: id, label: e.Label, raw: e.Ops, tx: ops.Tx{TxID: e.TxID, Label: e.Label}}
	var items []json.RawMessage
	if err := json.Unmarshal(e.Ops, &items); err != nil || items == nil || len(items) > txOpsPerTxMax {
		p.raw = json.RawMessage("[]")
		p.bad = &ops.Details{}
		return p
	}
	var compact bytes.Buffer
	if json.Compact(&compact, e.Ops) == nil {
		p.raw = compact.Bytes()
	}
	for i, item := range items {
		var op ops.Op
		dec := json.NewDecoder(bytes.NewReader(item))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&op); err != nil {
			p.bad = &ops.Details{Op: i}
			return p
		}
		if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			p.bad = &ops.Details{Op: i}
			return p
		}
		p.tx.Ops = append(p.tx.Ops, op)
	}
	return p
}

func storedResult(txID string, row db.ListProjectOperationsByTxRow) txResultJSON {
	if row.Seq != nil {
		return txResultJSON{TxID: txID, Status: ops.StatusApplied, Seq: row.Seq}
	}
	res := txResultJSON{TxID: txID, Status: ops.StatusRejected}
	if row.RejectedReason != nil {
		res.Reason = *row.RejectedReason
	}
	if len(row.RejectedDetails) > 0 {
		var d ops.Details
		if json.Unmarshal(row.RejectedDetails, &d) == nil {
			res.Details = &d
		}
	}
	return res
}

// applyBatch runs inside inProjectTx. Accepted transactions take consecutive seqs; the tables are written once
// at the end for the collections that changed.
func (s *Server) applyBatch(ctx context.Context, row db.GetProjectRow, clientID string, txs []parsedTx) (txBatchJSON, error) {
	out := txBatchJSON{Results: make([]txResultJSON, 0, len(txs))}
	ids := make([]uuid.UUID, 0, len(txs))
	for _, t := range txs {
		ids = append(ids, t.id)
	}
	prior := map[uuid.UUID]db.ListProjectOperationsByTxRow{}
	if len(ids) > 0 {
		rows, err := s.q.ListProjectOperationsByTx(ctx, db.ListProjectOperationsByTxParams{ProjectID: row.ID, TxIds: ids})
		if err != nil {
			return out, fmt.Errorf("ops.prior: %w", err)
		}
		for _, p := range rows {
			prior[p.TxID] = p
		}
	}

	var (
		st      ops.State
		loaded  opsState
		hooks   *projectHooks
		touched = map[string]bool{}
		seen    = map[uuid.UUID]int{}
	)
	var actor *uuid.UUID
	if id := auth.FromContext(ctx); !id.IsGuest() {
		actor = &id.UserID
	}
	schema := ops.ProjectSchema()
	for _, t := range txs {
		if p, ok := prior[t.id]; ok {
			out.Results = append(out.Results, storedResult(t.id.String(), p))
			continue
		}
		if i, ok := seen[t.id]; ok {
			out.Results = append(out.Results, out.Results[i])
			continue
		}
		seen[t.id] = len(out.Results)
		if st == nil {
			var err error
			if loaded, err = s.loadOpsState(ctx, row); err != nil {
				return out, err
			}
			st = loaded.state
			hooks = &projectHooks{ctx: ctx, q: s.q}
		}
		outcome := ops.Outcome{Status: ops.StatusRejected, Reason: ops.ReasonInvalidValue, Details: t.bad}
		var changes []ops.Change
		if t.bad == nil {
			var next ops.State
			next, changes, outcome = ops.Apply(schema, st, t.tx, hooks)
			if hooks.err != nil {
				return out, hooks.err
			}
			if outcome.Status == ops.StatusApplied {
				st = next
			}
		}
		res := txResultJSON{TxID: t.id.String(), Status: outcome.Status, Reason: outcome.Reason, Details: outcome.Details}
		params := db.InsertProjectOperationParams{
			ProjectID: row.ID, TxID: t.id, ClientID: clientID, ActorUserID: actor, Label: t.label, Ops: t.raw,
		}
		if outcome.Status == ops.StatusApplied {
			s.lockedSeq++
			seq := s.lockedSeq
			res.Seq, params.Seq = &seq, &seq
			for _, c := range changes {
				touched[c.Coll] = true
			}
		} else {
			reason := outcome.Reason
			params.RejectedReason = &reason
			if outcome.Details != nil {
				params.RejectedDetails = mustJSON(outcome.Details)
			}
		}
		if err := s.q.InsertProjectOperation(ctx, params); err != nil {
			return out, fmt.Errorf("ops.log: %w", err)
		}
		out.Results = append(out.Results, res)
	}
	if len(touched) > 0 {
		if err := s.writeOpsState(ctx, row, loaded, st, touched); err != nil {
			return out, err
		}
	}
	if st != nil && s.lockedSeq != loaded.seq {
		if err := s.finishSeq(ctx, row.ID, s.lockedSeq); err != nil {
			return out, err
		}
	}
	out.Seq = s.lockedSeq
	return out, nil
}
