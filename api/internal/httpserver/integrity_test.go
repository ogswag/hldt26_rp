package httpserver

import (
	"testing"

	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/exporters"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/sim"
)

func TestSameModel(t *testing.T) {
	calc := db.ListRunsWithoutDraftHashRow{
		Kind: exporters.KindCalculation, EconVersion: econ.ModelVersion, MatchVersion: projects.MatchVersion,
		SimVersion: projects.SimVersion,
	}
	simRun := calc
	simRun.Kind = exporters.KindSimulation
	simRun.SimVersion = sim.Version
	oldEcon := calc
	oldEcon.EconVersion = "econ-v0"
	oldMatch := simRun
	oldMatch.MatchVersion = "match-v1"
	oldCheck := calc
	oldCheck.SimVersion = "sim-v0"
	cases := []struct {
		name string
		run  db.ListRunsWithoutDraftHashRow
		want bool
	}{
		{"calculation of this build", calc, true},
		{"simulation of this build", simRun, true},
		{"older econ", oldEcon, false},
		{"older matching", oldMatch, false},
		{"calculation with older verification", oldCheck, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sameModel(c.run); got != c.want {
				t.Fatalf("sameModel = %v, want %v", got, c.want)
			}
		})
	}
}
