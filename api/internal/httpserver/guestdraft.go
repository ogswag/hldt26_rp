package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/engine"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
)

// guestProject is the project a browser sends with a guest calculation or export. It has no place on the server,
// so it is checked the way an import is and used once.
type guestProject struct {
	SchemaVersion int       `json:"schema_version,omitempty"`
	Collections   ops.State `json:"collections,omitempty"`
}

func (g guestProject) present() bool {
	return len(g.Collections) > 0
}

// guestDraft turns the browser's records into a draft after the operation schema has accepted them, with the same
// hooks a saved project gets. It writes the answer itself when they do not pass.
func (s *Server) guestDraft(w http.ResponseWriter, r *http.Request, objectType string, in guestProject) (projects.Draft, json.RawMessage, bool) {
	schema := ops.ProjectSchema()
	if in.SchemaVersion < 1 || in.SchemaVersion > schema.Version {
		writeCode(w, r, http.StatusConflict, ops.ReasonSchemaVersion,
			fmt.Sprintf("Версия схемы %d, сервер принимает версии от 1 до %d. Обновите страницу.", in.SchemaVersion, schema.Version))
		return projects.Draft{}, nil, false
	}
	rec := in.Collections.Get(ops.CollProject, ops.CollProject)
	if rec == nil {
		writeError(w, r, http.StatusBadRequest, "В проекте нет записи проекта.")
		return projects.Draft{}, nil, false
	}
	if rec.String(ops.FieldObjectType) != objectType {
		writeError(w, r, http.StatusBadRequest, "Тип объекта в проекте не совпадает с типом в запросе.")
		return projects.Draft{}, nil, false
	}
	empty := projects.NewDraft(objectType, nil, nil, nil, nil)
	empty.AssumptionSets = []projects.AssumptionSet{}
	base, err := ops.FromDraft("", empty)
	if err != nil {
		s.internal(w, r, "guest.draft.base", err, "Не удалось выполнить расчёт. Повторите запрос.")
		return projects.Draft{}, nil, false
	}
	list := ops.Replace(schema, base, in.Collections)
	hooks := &projectHooks{ctx: r.Context(), q: s.q}
	if s.q == nil {
		hooks.solutions = map[string]bool{}
	}
	id := uuid.NewString()
	next, _, out := ops.Apply(schema, base, ops.Tx{TxID: id, Ops: list}, hooks)
	if hooks.err != nil {
		s.internal(w, r, "guest.draft.hooks", hooks.err, "Не удалось выполнить расчёт. Повторите запрос.")
		return projects.Draft{}, nil, false
	}
	if out.Status != ops.StatusApplied {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":      "Проект из браузера не прошёл проверку. Исправьте отмеченные поля.",
			"code":       out.Reason,
			"details":    out.Details,
			"request_id": middleware.GetReqID(r.Context()),
		})
		return projects.Draft{}, nil, false
	}
	d, _, err := ops.ToDraft(next)
	if err != nil {
		s.internal(w, r, "guest.draft.to", err, "Не удалось выполнить расчёт. Повторите запрос.")
		return projects.Draft{}, nil, false
	}
	params, err := filledValidated(objectType, d.Params)
	if err != nil {
		rejectParams(w, r, err)
		return projects.Draft{}, nil, false
	}
	d.Params = params
	return d, params, true
}

// guestInput is what a guest calculation or export reads from its request.
type guestInput struct {
	ObjectType string
	Params     json.RawMessage
	Draft      projects.Draft
	HasDraft   bool
	Seed       int
	IncludeIDs []string
	Overrides  econ.Overrides
}

// readGuestInput checks a guest request and writes the answer itself when it is not valid. With collections the
// params, the processes, the fleets and the map come from them; without, only the params are known.
func (s *Server) readGuestInput(w http.ResponseWriter, r *http.Request, body guestCalculateBody) (guestInput, bool) {
	if !objects.Valid(body.ObjectType) {
		writeError(w, r, http.StatusBadRequest, "Тип объекта должен быть warehouse, airport или hospital.")
		return guestInput{}, false
	}
	in := guestInput{ObjectType: body.ObjectType, Seed: econ.DefaultSeed, IncludeIDs: body.IncludeIDs}
	if body.present() {
		var ok bool
		if in.Draft, in.Params, ok = s.guestDraft(w, r, body.ObjectType, body.guestProject); !ok {
			return guestInput{}, false
		}
		in.HasDraft = true
	} else {
		raw, err := marshalParams(body.Params)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "params должен быть JSON-объектом. Проверьте тело запроса.")
			return guestInput{}, false
		}
		if in.Params, err = filledValidated(body.ObjectType, raw); err != nil {
			rejectParams(w, r, err)
			return guestInput{}, false
		}
	}
	if body.Seed != nil {
		if *body.Seed < 0 {
			writeError(w, r, http.StatusBadRequest, "Номер случайной выборки должен быть целым числом, 0 или больше.")
			return guestInput{}, false
		}
		in.Seed = *body.Seed
	}
	if !validIncludeIDs(w, r, body.IncludeIDs) {
		return guestInput{}, false
	}
	if in.IncludeIDs == nil && len(in.Draft.MatchSelectedIDs) > 0 {
		in.IncludeIDs = in.Draft.MatchSelectedIDs
	}
	ov, err := engine.OverridesOf(body.Overrides, in.Draft)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "Переопределения в проекте повреждены. Сбросьте их и повторите.")
		return guestInput{}, false
	}
	in.Overrides = ov
	return in, true
}

// guestResult calculates a guest request. Nothing of it is stored.
func (s *Server) guestResult(ctx context.Context, in guestInput) (econ.Result, error) {
	if in.HasDraft {
		return s.computeProject(ctx, in.ObjectType, in.Params, in.IncludeIDs, in.Seed, in.Overrides, in.Draft)
	}
	return s.compute(ctx, in.ObjectType, in.Params, in.IncludeIDs, in.Seed, in.Overrides)
}
