package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/simbuild"
)

// simCheckRuns bounds how far back the check of a variant looks for a run made for its current inputs.
const simCheckRuns = 60

// simChecks compares the latest simulation of each warehouse variant with the economics. The other objects are
// checked when they are calculated, so they get none here.
func (s *Server) simChecks(ctx context.Context, projectID uuid.UUID, d projects.Draft) ([]econ.SimCheck, error) {
	if d.ObjectType != objects.Warehouse {
		return nil, nil
	}
	rows, err := s.q.ListRecentSimulationBriefs(ctx, db.ListRecentSimulationBriefsParams{ProjectID: projectID, Limit: simCheckRuns})
	if err != nil {
		return nil, fmt.Errorf("sim_checks.runs: %w", err)
	}
	briefs := make([]simbuild.RunBrief, 0, len(rows))
	for _, row := range rows {
		b := simbuild.RunBrief{RunID: row.ID.String(), VariantID: row.VariantID, VariantHash: row.VariantHash}
		if len(row.EconCheck) > 0 && string(row.EconCheck) != "null" {
			var ec simbuild.EconCheck
			if json.Unmarshal(row.EconCheck, &ec) == nil {
				b.EconCheck = &ec
			}
		}
		briefs = append(briefs, b)
	}
	checks, err := simbuild.Checks(d, briefs)
	if err != nil {
		return nil, fmt.Errorf("sim_checks: %w", err)
	}
	return checks, nil
}

func (s *Server) getSimChecks(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	ctx := r.Context()
	d, _, err := s.ensureProjectGraph(ctx, row)
	if err != nil {
		s.internal(w, r, "sim_checks.graph", err, "Не удалось загрузить проверку симуляцией. Повторите запрос.")
		return
	}
	checks, err := s.simChecks(ctx, row.ID, d)
	if err != nil {
		s.internal(w, r, "sim_checks", err, "Не удалось загрузить проверку симуляцией. Повторите запрос.")
		return
	}
	if checks == nil {
		checks = []econ.SimCheck{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": checks})
}
