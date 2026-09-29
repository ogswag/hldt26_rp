package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"moscow_hackathon_2026/api/internal/catalogstore"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/engine"
	"moscow_hackathon_2026/api/internal/projects"
)

// loadCatalog reads the live catalog. A server without a database (tests of routing) answers with an empty
// catalog.
func (s *Server) loadCatalog(ctx context.Context) (engine.Catalog, error) {
	if s.q == nil {
		return engine.Catalog{}, nil
	}
	return catalogstore.Load(ctx, s.q)
}

func (s *Server) compute(ctx context.Context, objectType string, params json.RawMessage, includeIDs []string, seed int, ov econ.Overrides) (econ.Result, error) {
	res, _, err := s.computeRobot(ctx, objectType, params, includeIDs, seed, ov)
	return res, err
}

func (s *Server) computeProject(ctx context.Context, objectType string, params json.RawMessage, includeIDs []string, seed int, ov econ.Overrides, d projects.Draft) (econ.Result, error) {
	cat, err := s.loadCatalog(ctx)
	if err != nil {
		return econ.Result{}, err
	}
	res, _, err := engine.Calculate(cat, engine.Request{
		ObjectType: objectType,
		Params:     params,
		IncludeIDs: includeIDs,
		Seed:       seed,
		Overrides:  ov,
		Draft:      d,
		HasDraft:   true,
	})
	return res, err
}

func (s *Server) computeRobot(ctx context.Context, objectType string, params json.RawMessage, includeIDs []string, seed int, ov econ.Overrides) (econ.Result, *econ.Robot, error) {
	cat, err := s.loadCatalog(ctx)
	if err != nil {
		return econ.Result{}, nil, err
	}
	return engine.Calculate(cat, engine.Request{
		ObjectType: objectType,
		Params:     params,
		IncludeIDs: includeIDs,
		Seed:       seed,
		Overrides:  ov,
	})
}

func (s *Server) failCalc(w http.ResponseWriter, r *http.Request, op string, err error) {
	var ie *econ.InputError
	if errors.As(err, &ie) {
		writeError(w, r, http.StatusBadRequest, ie.Msg)
		return
	}
	s.log.Error(op, "op", op, "request_id", middleware.GetReqID(r.Context()), "err", err)
	writeError(w, r, http.StatusInternalServerError, "Не удалось выполнить расчёт. Повторите запрос.")
}
