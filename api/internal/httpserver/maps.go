package httpserver

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"moscow_hackathon_2026/api/internal/catalogstore"
	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/simbuild"
)

const mapBodyLimit = 4 << 20

type mapSaveJSON struct {
	Map       maps.Document     `json:"map"`
	Check     simbuild.MapCheck `json:"check"`
	InputHash string            `json:"input_hash"`
	Stale     bool              `json:"stale"`
}

func readMapBody(w http.ResponseWriter, r *http.Request) (maps.Document, bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, mapBodyLimit))
	if err != nil {
		writeError(w, r, http.StatusRequestEntityTooLarge, "Карта больше 4 МБ. Упростите контуры или разбейте план.")
		return maps.Document{}, false
	}
	doc, err := maps.Decode(raw)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "Карта не соответствует формату map-v1. Проверьте поля документа.")
		return maps.Document{}, false
	}
	return doc, true
}

// mapContext collects process codes and robot classes from every variant of the draft.
func (s *Server) mapContext(ctx context.Context, d projects.Draft) (maps.Context, error) {
	robots, err := s.robotsFor(ctx, d, nil)
	if err != nil {
		return maps.Context{}, err
	}
	return simbuild.MapContext(robots, d), nil
}

// robotsFor reads the robots the draft's fleets, and the suggestion if there is one, name.
func (s *Server) robotsFor(ctx context.Context, d projects.Draft, sug *simbuild.Suggestion) (simbuild.Robots, error) {
	return catalogstore.RobotsFor(ctx, s.q, simbuild.SolutionIDs(d, sug))
}

func (s *Server) validateProjectMap(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	doc, ok := readMapBody(w, r)
	if !ok {
		return
	}
	d, _, err := s.ensureProjectGraph(r.Context(), row)
	if err != nil {
		s.internal(w, r, "maps.validate.graph", err, "Не удалось проверить карту. Повторите запрос.")
		return
	}
	mc, err := s.mapContext(r.Context(), d)
	if err != nil {
		s.internal(w, r, "maps.validate.robots", err, "Не удалось проверить карту. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, simbuild.CheckMap(doc, mc))
}

func (s *Server) projectMapTemplate(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	if row.ObjectType != "warehouse" {
		writeError(w, r, http.StatusBadRequest, "Шаблон карты есть только для склада.")
		return
	}
	d, _, err := s.ensureProjectGraph(r.Context(), row)
	if err != nil {
		s.internal(w, r, "maps.template.graph", err, "Не удалось подготовить шаблон. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, simbuild.Template(d))
}

func (s *Server) internal(w http.ResponseWriter, r *http.Request, op string, err error, msg string) {
	if errors.Is(err, context.Canceled) {
		return
	}
	s.log.Error(op, "op", op, "request_id", middleware.GetReqID(r.Context()), "err", err)
	writeError(w, r, http.StatusInternalServerError, msg)
}
