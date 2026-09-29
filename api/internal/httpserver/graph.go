package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/projects"
)

// txFail maps a failed project transaction to a response.
func (s *Server) txFail(w http.ResponseWriter, r *http.Request, op string, err error, msg string) {
	var he *httpError
	switch {
	case errors.As(err, &he):
		writeError(w, r, he.status, he.msg)
	case isFKViolation(err):
		writeError(w, r, http.StatusBadRequest, "Решение из позиции флота не найдено в каталоге. Выберите модель заново и сохраните.")
	case isUniqueViolation(err):
		writeError(w, r, http.StatusConflict, "Код процесса или статьи уже занят в проекте. Укажите другой код.")
	default:
		s.internal(w, r, op, err, msg)
	}
}

func parseChildID(w http.ResponseWriter, r *http.Request, param, msg string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, msg)
		return uuid.UUID{}, false
	}
	return id, true
}

func (s *Server) listProcesses(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	if _, _, err := s.ensureProjectGraph(r.Context(), row); err != nil {
		s.log.Error("projects.processes", "op", "projects.processes", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить процессы. Повторите запрос.")
		return
	}
	items, err := s.loadProcesses(r.Context(), row.ID)
	if err != nil {
		s.log.Error("projects.processes", "op", "projects.processes", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить процессы. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) listVariants(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	if _, _, err := s.ensureProjectGraph(r.Context(), row); err != nil {
		s.log.Error("projects.variants", "op", "projects.variants", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить варианты. Повторите запрос.")
		return
	}
	items, err := s.loadVariants(r.Context(), row.ID)
	if err != nil {
		s.log.Error("projects.variants", "op", "projects.variants", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить варианты. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) listSharedCosts(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	if _, _, err := s.ensureProjectGraph(r.Context(), row); err != nil {
		s.log.Error("projects.shared_costs", "op", "projects.shared_costs", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить общие статьи. Повторите запрос.")
		return
	}
	items, err := s.loadSharedCosts(r.Context(), row.ID)
	if err != nil {
		s.log.Error("projects.shared_costs", "op", "projects.shared_costs", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить общие статьи. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) listAssumptionSets(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	if _, _, err := s.ensureProjectGraph(r.Context(), row); err != nil {
		s.log.Error("projects.assumptions", "op", "projects.assumptions", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить наборы допущений. Повторите запрос.")
		return
	}
	items, err := s.loadAssumptionSets(r.Context(), row.ID)
	if err != nil {
		s.log.Error("projects.assumptions", "op", "projects.assumptions", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить наборы допущений. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) listCalculations(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	rows, err := s.q.ListCalculationRuns(r.Context(), db.ListCalculationRunsParams{ProjectID: row.ID, Limit: 100})
	if err != nil {
		s.log.Error("projects.runs", "op", "projects.runs", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить запуски. Повторите запрос.")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, run := range rows {
		items = append(items, map[string]any{
			"id":                 run.ID.String(),
			"project_id":         run.ProjectID.String(),
			"project_version_id": run.ProjectVersionID.String(),
			"input_hash":         run.InputHash,
			"match_version":      run.MatchVersion,
			"econ_version":       run.EconVersion,
			"sim_version":        run.SimVersion,
			"seed":               run.Seed,
			"status":             run.Status,
			"created_at":         timestamptzPtr(run.CreatedAt),
			"version_no":         run.VersionNo,
			"confidence_level":   run.ConfidenceLevel,
			"is_current":         row.CurrentRunID != nil && *row.CurrentRunID == run.ID,
			"stale_vs_draft":     projects.Stale(row.InputHash, run.DraftHash),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// scopedRun loads the calculation run of a route scoped by {runId}.
func (s *Server) scopedRun(w http.ResponseWriter, r *http.Request) (db.GetCalculationRunRow, bool) {
	runID, err := uuid.Parse(chi.URLParam(r, "runId"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "Некорректный идентификатор запуска.")
		return db.GetCalculationRunRow{}, false
	}
	run, err := s.q.GetCalculationRun(r.Context(), runID)
	if err != nil {
		if isNoRows(err) {
			writeError(w, r, http.StatusNotFound, "Запуск не найден.")
			return db.GetCalculationRunRow{}, false
		}
		s.log.Error("projects.run.get", "op", "projects.runs", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить запуск. Повторите запрос.")
		return db.GetCalculationRunRow{}, false
	}
	return run, true
}

func (s *Server) getCalculation(w http.ResponseWriter, r *http.Request) {
	run, ok := s.scopedRun(w, r)
	if !ok {
		return
	}
	proj := projectFrom(r)
	var result econ.Result
	if err := json.Unmarshal(run.Summary, &result); err != nil {
		s.log.Error("projects.run.parse", "op", "projects.runs", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить запуск. Повторите запрос.")
		return
	}
	meta := s.runMeta(r.Context(), calcRun(run), proj.CurrentRunID)
	writeJSON(w, http.StatusOK, calculateResponse{
		Result:       result,
		RunID:        run.ID.String(),
		InputHash:    run.InputHash,
		ProjectID:    run.ProjectID.String(),
		ProjectName:  proj.Name,
		StaleVsDraft: projects.Stale(proj.InputHash, run.DraftHash),
		Run:          &meta,
	})
}

func (s *Server) listVersions(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	rows, err := s.q.ListProjectVersions(r.Context(), row.ID)
	if err != nil {
		s.log.Error("projects.versions", "op", "projects.versions", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить версии. Повторите запрос.")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, v := range rows {
		items = append(items, map[string]any{
			"id":         v.ID.String(),
			"project_id": v.ProjectID.String(),
			"version_no": v.VersionNo,
			"input_hash": v.InputHash,
			"created_at": timestamptzPtr(v.CreatedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	vid, err := uuid.Parse(chi.URLParam(r, "versionId"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "Некорректный идентификатор версии.")
		return
	}
	ver, err := s.q.GetProjectVersion(r.Context(), db.GetProjectVersionParams{ID: vid, ProjectID: row.ID})
	if err != nil {
		if isNoRows(err) {
			writeError(w, r, http.StatusNotFound, "Версия не найдена.")
			return
		}
		s.log.Error("projects.version.get", "op", "projects.versions", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить версию. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, json.RawMessage(ver.Snapshot))
}
