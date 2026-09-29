package simbuild

import (
	"fmt"
	"math"
	"time"

	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/rutext"
	"moscow_hackathon_2026/api/internal/sim"
)

const SummarySchema = "sim-summary-v1"

// Summary is stored in calculation_results for a simulation run.
type Summary struct {
	SchemaVersion string `json:"schema_version"`
	RunID         string `json:"run_id"`
	ProjectID     string `json:"project_id"`
	VersionNo     int    `json:"version_no"`
	InputHash     string `json:"input_hash"`
	// CatalogContentSHA256 names the catalog the fleet was read from.
	CatalogContentSHA256 string       `json:"catalog_content_sha256,omitempty"`
	VariantID            string       `json:"variant_id"`
	VariantName          string       `json:"variant_name"`
	VariantHash          string       `json:"variant_hash,omitempty"`
	MapSource            string       `json:"map_source"`
	MapWarnings          []string     `json:"map_warnings"`
	ConfidenceLevel      string       `json:"confidence_level"`
	FinishedAt           time.Time    `json:"finished_at"`
	DurationMs           int64        `json:"duration_ms"`
	Result               sim.Result   `json:"result"`
	EconCheck            *EconCheck   `json:"econ_check,omitempty"`
	FleetSearch          *FleetSearch `json:"fleet_search,omitempty"`
	Snapshot             SnapshotInfo `json:"snapshot"`
}

// FleetSearch is the table of sizes a fleet_search job tried.
type FleetSearch struct {
	LineKey string           `json:"line_key"`
	Rows    []FleetSearchRow `json:"rows"`
	Best    int              `json:"best"`
	Stopped string           `json:"stopped,omitempty"`
}

type FleetSearchRow struct {
	Quantity int     `json:"quantity"`
	Coverage float64 `json:"coverage"`
	SLAOK    bool    `json:"sla_ok"`
}

type SnapshotInfo struct {
	MatchVersion string `json:"match_version"`
	EconVersion  string `json:"econ_version"`
	SimVersion   string `json:"sim_version"`
}

// EconCheck compares simulated delivery with the daily demand the economics assumes.
type EconCheck struct {
	ExpectedJobs float64 `json:"expected_jobs"`
	Completed    float64 `json:"completed"`
	Coverage     float64 `json:"coverage"`
	Flag         bool    `json:"flag"`
	Text         string  `json:"text"`
}

// SummaryMeta names the run a summary belongs to. A run in the browser has no project, version or run id.
type SummaryMeta struct {
	RunID                string
	ProjectID            string
	VersionNo            int
	InputHash            string
	CatalogContentSHA256 string
	Snapshot             SnapshotInfo
}

// NewSummary describes a finished run of a built model.
func NewSummary(b *Built, res sim.Result, meta SummaryMeta, started, finished time.Time) Summary {
	_, mapWarnings := maps.Messages(b.MapIssues)
	s := Summary{
		SchemaVersion:        SummarySchema,
		RunID:                meta.RunID,
		ProjectID:            meta.ProjectID,
		VersionNo:            meta.VersionNo,
		InputHash:            meta.InputHash,
		CatalogContentSHA256: meta.CatalogContentSHA256,
		VariantID:            b.Variant.ID,
		VariantName:          b.Variant.Name,
		VariantHash:          b.VariantHash,
		MapSource:            b.MapSource,
		MapWarnings:          mapWarnings,
		ConfidenceLevel:      b.Confidence,
		FinishedAt:           finished.UTC(),
		DurationMs:           finished.Sub(started).Milliseconds(),
		Result:               res,
		EconCheck:            econCheck(b, res),
		Snapshot:             meta.Snapshot,
	}
	s.Result.Warnings = append(append([]string{}, b.Warnings...), res.Warnings...)
	return s
}

func econCheck(b *Built, res sim.Result) *EconCheck {
	expected := 0.0
	for _, v := range b.Model.ExpectedArrivals() {
		expected += v
	}
	if expected <= 0 {
		return nil
	}
	completed := res.KPI.Completed.Median
	cov := completed / expected
	c := &EconCheck{
		ExpectedJobs: math.Round(expected*10) / 10,
		Completed:    completed,
		Coverage:     math.Round(cov*1000) / 1000,
		Flag:         math.Abs(1-cov) > 0.15,
	}
	if c.Flag {
		c.Text = fmt.Sprintf("Флот выполнил %s ожидаемых заданий за горизонт. Производительность из экономики нельзя считать подтверждённой.", rutext.Pct(cov*100, 0))
	} else {
		c.Text = fmt.Sprintf("Флот выполнил %s ожидаемых заданий за горизонт, расхождение в пределах 15%%.", rutext.Pct(cov*100, 0))
	}
	return c
}
