package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/projects"
)

type projectJSON struct {
	ID                    string                   `json:"id"`
	Name                  string                   `json:"name"`
	ObjectType            string                   `json:"object_type"`
	Params                json.RawMessage          `json:"params"`
	Processes             []projects.Process       `json:"processes"`
	Variants              []projects.Variant       `json:"variants"`
	SharedCosts           []projects.SharedCost    `json:"shared_costs"`
	AssumptionSets        []projects.AssumptionSet `json:"assumption_sets"`
	ActiveAssumptionSetID string                   `json:"active_assumption_set_id,omitempty"`
	Map                   json.RawMessage          `json:"map"`
	MatchSelectedIDs      []string                 `json:"match_selected_ids"`
	EconOverrides         json.RawMessage          `json:"econ_overrides"`
	Results               json.RawMessage          `json:"results"`
	ModelVersion          string                   `json:"model_version"`
	CalcSeed              *int32                   `json:"calc_seed"`
	CreatedAt             *time.Time               `json:"created_at"`
	InputHash             string                   `json:"input_hash"`
	CurrentRunID          *string                  `json:"current_run_id"`
	Stale                 bool                     `json:"stale"`
	Access                string                   `json:"access"`
}

type updateProjectBody struct {
	Name       *string `json:"name"`
	ObjectType *string `json:"object_type"`
	Params     any     `json:"params"`
}

type calculateBody struct {
	Seed       *int            `json:"seed"`
	IncludeIDs []string        `json:"include_ids"`
	Overrides  *econ.Overrides `json:"overrides"`
}

func guestForbidden(w http.ResponseWriter, r *http.Request) bool {
	if !auth.FromRequest(r).IsGuest() {
		return false
	}
	writeError(w, r, http.StatusForbidden, "Гость не может сохранять проект. Выполните демо-расчёт: данные останутся только в браузере.")
	return true
}

type projectListItem struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	ObjectType   string     `json:"object_type"`
	ModelVersion string     `json:"model_version"`
	CalcSeed     *int32     `json:"calc_seed"`
	CreatedAt    *time.Time `json:"created_at"`
	HasResults   bool       `json:"has_results"`
	Stale        bool       `json:"stale"`
	InputHash    string     `json:"input_hash"`
	CurrentRunID *string    `json:"current_run_id"`
	Access       string     `json:"access"`
}

func resultsJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}

func writeProject(w http.ResponseWriter, status int, p projectJSON) {
	writeJSON(w, status, p)
}

func uuidStringPtr(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func (s *Server) projectJSONFromRow(w http.ResponseWriter, r *http.Request, row db.GetProjectRow) (projectJSON, bool) {
	d, hash, err := s.ensureProjectGraph(r.Context(), row)
	if err != nil {
		s.log.Error("projects.graph", "op", "projects.graph", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось собрать проект. Повторите запрос.")
		return projectJSON{}, false
	}
	stale := false
	results := resultsJSON(row.Results)
	if row.CurrentRunID != nil {
		run, err := s.q.GetCalculationRun(r.Context(), *row.CurrentRunID)
		if err == nil {
			stale = projects.Stale(hash, run.DraftHash)
			results = run.Summary
		} else if !isNoRows(err) {
			s.log.Error("projects.run.get", "op", "projects.graph", "request_id", middleware.GetReqID(r.Context()), "err", err)
			writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить запуск. Повторите запрос.")
			return projectJSON{}, false
		}
	} else if len(row.Results) > 0 {
		stale = true
	}
	return projectJSON{
		ID:                    row.ID.String(),
		Name:                  row.Name,
		ObjectType:            row.ObjectType,
		Params:                row.Params,
		Processes:             d.Processes,
		Variants:              d.Variants,
		SharedCosts:           d.SharedCosts,
		AssumptionSets:        d.AssumptionSets,
		ActiveAssumptionSetID: d.ActiveAssumptionSetID,
		Map:                   d.Map,
		MatchSelectedIDs:      d.MatchSelectedIDs,
		EconOverrides:         d.EconOverrides,
		Results:               econ.WithMathML(results),
		ModelVersion:          row.ModelVersion,
		CalcSeed:              row.CalcSeed,
		CreatedAt:             timestamptzPtr(row.CreatedAt),
		InputHash:             hash,
		CurrentRunID:          uuidStringPtr(row.CurrentRunID),
		Stale:                 stale,
	}, true
}

func marshalParams(v any) (json.RawMessage, error) {
	if v == nil {
		return json.RawMessage(`{}`), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	p, ok := s.projectJSONFromRow(w, r, row)
	if !ok {
		return
	}
	p.Access = roleFrom(r)
	writeProject(w, http.StatusOK, p)
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	if guestForbidden(w, r) {
		return
	}
	uid := auth.FromRequest(r).UserID
	rows, err := s.q.ListProjectsByUser(r.Context(), db.ListProjectsByUserParams{
		UserID:  uid,
		MaxRows: 100,
	})
	if err != nil {
		s.log.Error("projects.list", "op", "projects.list", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить проекты. Повторите запрос.")
		return
	}
	items := make([]projectListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, projectListItem{
			ID:           row.ID.String(),
			Name:         row.Name,
			ObjectType:   row.ObjectType,
			ModelVersion: row.ModelVersion,
			CalcSeed:     row.CalcSeed,
			CreatedAt:    timestamptzPtr(row.CreatedAt),
			HasResults:   row.HasResults,
			Stale:        row.Stale,
			InputHash:    row.InputHash,
			CurrentRunID: uuidStringPtr(row.CurrentRunID),
			Access:       row.Access,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) calculateProject(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	var body calculateBody
	if !decodeJSON(w, r, &body, true) {
		return
	}
	raw, err := filledValidated(row.ObjectType, row.Params)
	if err != nil {
		rejectParams(w, r, err)
		return
	}
	seed := econ.DefaultSeed
	if body.Seed != nil {
		if *body.Seed < 0 {
			writeError(w, r, http.StatusBadRequest, "Номер случайной выборки должен быть целым числом, 0 или больше.")
			return
		}
		seed = *body.Seed
	}
	if !validIncludeIDs(w, r, body.IncludeIDs) {
		return
	}
	d, _, seq, err := s.currentDraft(r.Context(), row)
	if err != nil {
		s.log.Error("projects.calculate.graph", "op", "projects.calculate", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось сохранить результат. Повторите запрос.")
		return
	}
	include := body.IncludeIDs
	if include == nil {
		if len(d.MatchSelectedIDs) > 0 {
			include = d.MatchSelectedIDs
		} else {
			include = selectedFromResults(row.Results)
		}
	}
	ov := econ.Overrides{}
	if body.Overrides != nil {
		ov = *body.Overrides
		d.EconOverrides = mustJSON(ov)
	} else if len(d.EconOverrides) > 0 {
		if err := json.Unmarshal(d.EconOverrides, &ov); err != nil {
			s.log.Error("projects.calculate.overrides", "op", "projects.calculate", "request_id", middleware.GetReqID(r.Context()), "err", err)
			writeError(w, r, http.StatusInternalServerError, "Сохранённые переопределения повреждены. Сохраните их заново и повторите расчёт.")
			return
		}
	}
	result, err := s.computeProject(r.Context(), row.ObjectType, raw, include, seed, ov, d)
	if err != nil {
		s.failCalc(w, r, "projects.calculate", err)
		return
	}
	d.MatchSelectedIDs = include
	hash, err := projects.InputHash(d)
	if err != nil {
		s.log.Error("projects.calculate.hash", "op", "projects.calculate", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось сохранить результат. Повторите запрос.")
		return
	}
	runID, err := s.persistRun(r.Context(), row, d, hash, seq, seed, result)
	if err != nil {
		s.log.Error("projects.calculate.save", "op", "projects.calculate", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось сохранить результат. Повторите запрос.")
		return
	}
	resp := calculateResponse{Result: result, InputHash: hash, ProjectID: row.ID.String(), ProjectName: row.Name}
	resp.RunID = runID.String()
	if saved, err := s.q.GetCalculationRun(r.Context(), *runID); err == nil {
		meta := s.runMeta(r.Context(), calcRun(saved), runID)
		resp.Run = &meta
	}
	writeJSON(w, http.StatusOK, resp)
}

type calculateResponse struct {
	econ.Result
	RunID        string       `json:"run_id,omitempty"`
	InputHash    string       `json:"input_hash,omitempty"`
	ProjectID    string       `json:"project_id,omitempty"`
	ProjectName  string       `json:"project_name,omitempty"`
	StaleVsDraft bool         `json:"stale_vs_draft"`
	Run          *runMetaJSON `json:"run,omitempty"`
}

type copyProjectBody struct {
	Name *string `json:"name"`
}

func (s *Server) copyProject(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	var body copyProjectBody
	if !decodeJSON(w, r, &body, true) {
		return
	}
	name := "Копия: " + row.Name
	if body.Name != nil {
		if *body.Name == "" {
			writeError(w, r, http.StatusBadRequest, "Укажите имя проекта.")
			return
		}
		name = *body.Name
	}
	ctx := r.Context()
	var copiedID uuid.UUID
	err := s.inProjectTx(ctx, row.ID, func(tx *Server) error {
		copied, err := tx.q.CopyProject(ctx, db.CopyProjectParams{ID: row.ID, OwnerID: auth.FromRequest(r).UserID, Name: name})
		if errors.Is(err, pgx.ErrNoRows) {
			return &httpError{status: http.StatusNotFound, msg: "Проект не найден."}
		}
		if err != nil {
			return err
		}
		copiedID = copied.ID
		if err := tx.copyGraph(ctx, row.ID, copied.ID); err != nil {
			return err
		}
		// The copy starts at seq 1, so lists emptied in the original stay empty instead of being reseeded.
		cp := *tx
		cp.lockedSeq = 0
		return cp.logReset(ctx, copied.ID, "copy")
	})
	if err != nil {
		s.txFail(w, r, "projects.copy", err, "Не удалось скопировать проект. Повторите запрос.")
		return
	}
	fresh, err := s.q.GetProject(ctx, copiedID)
	if err != nil {
		s.log.Error("projects.copy.reload", "op", "projects.copy", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось скопировать проект. Повторите запрос.")
		return
	}
	p, ok := s.projectJSONFromRow(w, r, fresh)
	if !ok {
		return
	}
	p.Access = RoleOwner
	writeProject(w, http.StatusCreated, p)
}
