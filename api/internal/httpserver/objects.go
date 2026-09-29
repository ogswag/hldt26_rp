package httpserver

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/objects"
)

func (s *Server) listObjectTypes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": objects.Types()})
}

func (s *Server) getObjectSchema(w http.ResponseWriter, r *http.Request) {
	objectType := chi.URLParam(r, "type")
	schema, err := objects.SchemaFor(objectType)
	if err != nil {
		if errors.Is(err, objects.ErrUnknownType) {
			writeError(w, r, http.StatusNotFound, "Тип объекта не найден. Используйте warehouse, airport или hospital.")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "Не удалось загрузить схему. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, schema)
}

type guestCalculateBody struct {
	ObjectType string          `json:"object_type"`
	Params     any             `json:"params"`
	Seed       *int            `json:"seed"`
	IncludeIDs []string        `json:"include_ids"`
	Overrides  *econ.Overrides `json:"overrides"`
	guestProject
}

func (s *Server) guestCalculate(w http.ResponseWriter, r *http.Request) {
	var body guestCalculateBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	in, ok := s.readGuestInput(w, r, body)
	if !ok {
		return
	}
	result, err := s.guestResult(r.Context(), in)
	if err != nil {
		s.failCalc(w, r, "guest.calculate", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
