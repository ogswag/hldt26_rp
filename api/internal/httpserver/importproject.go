package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/ops"
)

// importClientID marks the journal entry of an import. It is not a tab, so no client ever skips it as its own.
const importClientID = "import"

const importLabel = "Перенести проект из браузера"

type importProjectBody struct {
	Name          string    `json:"name"`
	SchemaVersion int       `json:"schema_version"`
	Collections   ops.State `json:"collections"`
}

// importProject creates a project from a snapshot of the browser store. A guest works without an account, and
// everything they entered stays in their browser; after they sign in, "Сохранить в аккаунт" sends that snapshot
// here. The records arrive through the operation schema, exactly as an edit would, and the project is created
// and filled in one database transaction: a snapshot the schema refuses leaves nothing behind.
func (s *Server) importProject(w http.ResponseWriter, r *http.Request) {
	if guestForbidden(w, r) {
		return
	}
	var body importProjectBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, txBodyLimit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, r, http.StatusRequestEntityTooLarge, "Проект больше 1 МБ, перенести его целиком нельзя.")
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
	rec := body.Collections.Get(ops.CollProject, ops.CollProject)
	if rec == nil {
		writeError(w, r, http.StatusBadRequest, "В снимке нет записи проекта.")
		return
	}
	objectType := rec.String(ops.FieldObjectType)
	if !objects.Valid(objectType) {
		writeError(w, r, http.StatusBadRequest, "Тип объекта должен быть warehouse, airport или hospital.")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = strings.TrimSpace(rec.String("name"))
	}
	if name == "" {
		writeError(w, r, http.StatusBadRequest, "Укажите имя проекта.")
		return
	}
	params := json.RawMessage(`{}`)
	if rec["params"] != nil {
		raw, err := json.Marshal(rec["params"])
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "params должен быть JSON-объектом. Проверьте тело запроса.")
			return
		}
		params = raw
	}
	// A browser may keep values in an older shape; the project record must carry the same values as params.
	params, err := objects.NormalizeParams(objectType, params)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "params должен быть JSON-объектом. Проверьте тело запроса.")
		return
	}
	var normalized map[string]any
	if err := json.Unmarshal(params, &normalized); err == nil && rec["params"] != nil {
		rec["params"] = normalized
	}
	if err := objects.Validate(objectType, params); err != nil {
		if writeValidateErr(w, r, err) {
			return
		}
		writeError(w, r, http.StatusBadRequest, "Параметры не прошли проверку. Исправьте поля и повторите.")
		return
	}

	created, res, err := s.createFromSnapshot(r, schema, name, objectType, params, body.Collections)
	if err != nil {
		s.log.Error("projects.import", "op", "projects.import", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось перенести проект. Повторите запрос.")
		return
	}
	if res.Status != ops.StatusApplied {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":      "Проект из браузера не прошёл проверку и не сохранён.",
			"code":       res.Reason,
			"details":    res.Details,
			"request_id": middleware.GetReqID(r.Context()),
		})
		return
	}
	p, ok := s.projectJSONFromRow(w, r, created)
	if !ok {
		return
	}
	p.Access = RoleOwner
	writeProject(w, http.StatusCreated, p)
}

// createFromSnapshot creates the project and turns its seeded content into the snapshot under one database
// transaction. A rejected transaction rolls the creation back too, so res tells the caller why and the project
// never existed.
func (s *Server) createFromSnapshot(r *http.Request, schema *ops.Schema, name, objectType string,
	params json.RawMessage, want ops.State) (db.GetProjectRow, txResultJSON, error) {
	ctx := r.Context()
	var (
		empty db.GetProjectRow
		res   txResultJSON
	)
	tx, err := s.q.BeginTx(ctx)
	if err != nil {
		return empty, res, fmt.Errorf("projects.import.begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	cp := *s
	cp.q = s.q.WithTx(tx)

	uid := auth.FromRequest(r).UserID
	row, err := cp.q.CreateProject(ctx, db.CreateProjectParams{
		UserID: &uid, Name: name, ObjectType: objectType, Params: params,
		ModelVersion: econ.ModelVersion, InputHash: "",
	})
	if err != nil {
		return empty, res, fmt.Errorf("projects.import.create: %w", err)
	}
	seq, err := cp.q.LockProject(ctx, row.ID)
	if err != nil {
		return empty, res, fmt.Errorf("projects.import.lock: %w", err)
	}
	cp.lockedSeq = seq
	fresh, err := cp.q.GetProject(ctx, row.ID)
	if err != nil {
		return empty, res, fmt.Errorf("projects.import.reload: %w", err)
	}
	// A new project comes with the defaults of its type; the snapshot replaces them whole.
	loaded, err := cp.loadOpsState(ctx, fresh)
	if err != nil {
		return empty, res, err
	}
	// The records keep the shape they had in the browser but not their identifiers: those belong to a project,
	// and a snapshot saved twice must give two projects rather than a collision. The name is the one the person
	// gave this project when they saved it.
	next := ops.Rekey(schema, want)
	next[ops.CollProject][ops.CollProject]["name"] = name
	list := ops.Replace(schema, loaded.state, next)
	if len(list) > txOpsPerTxMax {
		return empty, txResultJSON{Status: ops.StatusRejected, Reason: ops.ReasonInvalidValue}, nil
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return empty, res, fmt.Errorf("projects.import.marshal: %w", err)
	}
	id := uuid.New()
	out, err := cp.applyBatch(ctx, fresh, importClientID, []parsedTx{{
		id: id, label: importLabel, raw: raw, tx: ops.Tx{TxID: id.String(), Label: importLabel, Ops: list},
	}})
	if err != nil {
		return empty, res, err
	}
	if len(out.Results) != 1 {
		return empty, res, fmt.Errorf("projects.import: %d results", len(out.Results))
	}
	res = out.Results[0]
	if res.Status != ops.StatusApplied {
		return empty, res, nil
	}
	final, err := cp.q.GetProject(ctx, row.ID)
	if err != nil {
		return empty, res, fmt.Errorf("projects.import.final: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, res, fmt.Errorf("projects.import.commit: %w", err)
	}
	return final, res, nil
}
