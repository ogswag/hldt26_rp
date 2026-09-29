package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/catalog"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/engine"
	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/projects"
)

type guestMatchBody struct {
	ObjectType string   `json:"object_type"`
	Params     any      `json:"params"`
	IncludeIDs []string `json:"include_ids"`
	TaskCodes  []string `json:"task_codes"`
}

type projectMatchBody struct {
	IncludeIDs []string `json:"include_ids"`
}

type solutionRaw struct {
	Uses []struct {
		Industry string `json:"industry"`
		Scenario string `json:"scenario"`
		Cases    string `json:"cases"`
	} `json:"uses"`
	ObjectTypes []string `json:"object_types"`
	Family      string   `json:"Тип"`
	Description string   `json:"описание"`
}

// matchView reads ?view=full|brief. Brief drops per-item reasons and rule steps.
func matchView(w http.ResponseWriter, r *http.Request) (brief bool, ok bool) {
	switch strings.TrimSpace(r.URL.Query().Get("view")) {
	case "", "full":
		return false, true
	case "brief":
		return true, true
	default:
		writeError(w, r, http.StatusBadRequest, "view должен быть full или brief.")
		return false, false
	}
}

func writeMatch(w http.ResponseWriter, out matching.Output, brief bool) {
	if brief {
		out = out.Brief()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) guestMatch(w http.ResponseWriter, r *http.Request) {
	brief, ok := matchView(w, r)
	if !ok {
		return
	}
	var body guestMatchBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	if !objects.Valid(body.ObjectType) {
		writeError(w, r, http.StatusBadRequest, "Тип объекта должен быть warehouse, airport или hospital.")
		return
	}
	raw, err := marshalParams(body.Params)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "params должен быть JSON-объектом. Проверьте тело запроса.")
		return
	}
	raw, err = filledValidated(body.ObjectType, raw)
	if err != nil {
		rejectParams(w, r, err)
		return
	}
	if !validIncludeIDs(w, r, body.IncludeIDs) {
		return
	}
	taskCodes, ok := validatedTaskCodes(w, r, body.ObjectType, body.TaskCodes)
	if !ok {
		return
	}
	out, err := s.runMatch(r.Context(), body.ObjectType, raw, body.IncludeIDs, engine.TaskCodes(body.ObjectType, nil, taskCodes))
	if err != nil {
		s.log.Error("match.guest", "op", "match.guest", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось выполнить подбор. Повторите запрос.")
		return
	}
	writeMatch(w, out, brief)
}

type projectMatchInput struct {
	params json.RawMessage
	draft  projects.Draft
}

func (s *Server) loadProjectMatchInput(w http.ResponseWriter, r *http.Request, row db.GetProjectRow, op string) (projectMatchInput, bool) {
	raw, err := filledValidated(row.ObjectType, row.Params)
	if err != nil {
		rejectParams(w, r, err)
		return projectMatchInput{}, false
	}
	d, _, err := s.ensureProjectGraph(r.Context(), row)
	if err != nil {
		s.log.Error(op+".graph", "op", op, "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось выполнить подбор. Повторите запрос.")
		return projectMatchInput{}, false
	}
	return projectMatchInput{params: raw, draft: d}, true
}

// projectMatchPreview ranks the catalog for the project draft without saving anything.
func (s *Server) projectMatchPreview(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	brief, ok := matchView(w, r)
	if !ok {
		return
	}
	in, ok := s.loadProjectMatchInput(w, r, row, "match.preview")
	if !ok {
		return
	}
	out, err := s.runMatch(r.Context(), row.ObjectType, in.params, in.draft.MatchSelectedIDs, engine.TaskCodes(row.ObjectType, in.draft.Processes, nil))
	if err != nil {
		s.log.Error("match.preview", "op", "match.preview", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, r, http.StatusInternalServerError, "Не удалось выполнить подбор. Повторите запрос.")
		return
	}
	writeMatch(w, out, brief)
}

// runMatch loads the catalog and matches with the engine.
func (s *Server) runMatch(ctx context.Context, objectType string, params json.RawMessage, includeIDs []string, taskCodes []string) (matching.Output, error) {
	cat, err := s.loadCatalog(ctx)
	if err != nil {
		return matching.Output{}, err
	}
	return engine.Match(cat, objectType, params, includeIDs, taskCodes)
}

func validIncludeIDs(w http.ResponseWriter, r *http.Request, ids []string) bool {
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := uuid.Parse(id); err != nil {
			writeError(w, r, http.StatusBadRequest, "include_ids должен содержать UUID. Удалите некорректный идентификатор и повторите.")
			return false
		}
	}
	return true
}

func validatedTaskCodes(w http.ResponseWriter, r *http.Request, objectType string, codes []string) ([]string, bool) {
	if len(codes) == 0 {
		return nil, true
	}
	if objectType != objects.Warehouse {
		writeError(w, r, http.StatusBadRequest, "task_codes пока поддерживаются только для warehouse.")
		return nil, false
	}
	seen := make(map[string]struct{}, len(codes))
	out := make([]string, 0, len(codes))
	for _, raw := range codes {
		code := strings.TrimSpace(raw)
		if !catalog.ValidTask(code) {
			writeError(w, r, http.StatusBadRequest, "Неизвестный код задачи "+code+". Выберите код из справочника каталога.")
			return nil, false
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	return out, true
}

func selectedFromResults(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var prev econ.Result
	if err := json.Unmarshal(raw, &prev); err != nil {
		return nil
	}
	return prev.Match.SelectedIDs
}
