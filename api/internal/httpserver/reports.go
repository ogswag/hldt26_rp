package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/exporters"
	"moscow_hackathon_2026/api/internal/jobs"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/simbuild"
)

const runHistoryLimit = 100

// storedRun is the part of a calculation or simulation run a report needs.
type storedRun struct {
	ID              uuid.UUID
	ProjectID       uuid.UUID
	VersionID       uuid.UUID
	Kind            string
	Status          string
	InputHash       string
	DraftHash       string
	MatchVersion    string
	EconVersion     string
	SimVersion      string
	Seed            int32
	CreatedAt       pgtype.Timestamptz
	ConfidenceLevel string
	VersionNo       int32
	Summary         []byte
	// VariantID and VariantHash say which variant a simulation is about and the hash of what it read.
	VariantID   string
	VariantHash string
}

func calcRun(r db.GetCalculationRunRow) storedRun {
	return storedRun{
		ID: r.ID, ProjectID: r.ProjectID, VersionID: r.ProjectVersionID, Kind: exporters.KindCalculation,
		Status: r.Status, InputHash: r.InputHash, DraftHash: r.DraftHash, MatchVersion: r.MatchVersion,
		EconVersion: r.EconVersion, SimVersion: r.SimVersion, Seed: r.Seed, CreatedAt: r.CreatedAt,
		ConfidenceLevel: r.ConfidenceLevel, VersionNo: r.VersionNo, Summary: r.Summary,
	}
}

func simRun(r db.GetSimulationRunRow) storedRun {
	variantID, variantHash := summaryVariant(r.Summary)
	return storedRun{
		VariantID: variantID, VariantHash: variantHash,
		ID: r.ID, ProjectID: r.ProjectID, VersionID: r.ProjectVersionID, Kind: exporters.KindSimulation,
		Status: r.Status, InputHash: r.InputHash, DraftHash: r.DraftHash, MatchVersion: r.MatchVersion,
		EconVersion: r.EconVersion, SimVersion: r.SimVersion, Seed: r.Seed, CreatedAt: r.CreatedAt,
		ConfidenceLevel: r.ConfidenceLevel, VersionNo: r.VersionNo, Summary: r.Summary,
	}
}

func (run storedRun) stale(d projects.Draft, draftHash string) bool {
	if run.Kind == exporters.KindSimulation {
		return simStaleFor(d, draftHash, run.DraftHash, run.SimVersion, run.VariantID, run.VariantHash)
	}
	return projects.Stale(draftHash, run.DraftHash)
}

// runMetaJSON identifies an immutable run in API responses.
type runMetaJSON struct {
	ID               string     `json:"id"`
	Kind             string     `json:"kind"`
	Status           string     `json:"status"`
	ProjectID        string     `json:"project_id"`
	ProjectVersionID string     `json:"project_version_id"`
	VersionNo        int32      `json:"version_no"`
	InputHash        string     `json:"input_hash"`
	MatchVersion     string     `json:"match_version"`
	EconVersion      string     `json:"econ_version"`
	SimVersion       string     `json:"sim_version"`
	Seed             int32      `json:"seed"`
	ConfidenceLevel  string     `json:"confidence_level"`
	CreatedAt        *time.Time `json:"created_at"`
	IsCurrent        bool       `json:"is_current"`
}

func (s *Server) runMeta(ctx context.Context, run storedRun, currentRunID *uuid.UUID) runMetaJSON {
	m := runMetaJSON{
		ID: run.ID.String(), Kind: run.Kind, Status: run.Status, ProjectID: run.ProjectID.String(),
		ProjectVersionID: run.VersionID.String(), VersionNo: run.VersionNo, InputHash: run.InputHash,
		MatchVersion: run.MatchVersion, EconVersion: run.EconVersion, SimVersion: run.SimVersion, Seed: run.Seed,
		ConfidenceLevel: run.ConfidenceLevel,
		CreatedAt:       timestamptzPtr(run.CreatedAt), IsCurrent: currentRunID != nil && *currentRunID == run.ID,
	}
	return m
}

// runReport assembles a report strictly from the run, its snapshot and its pinned catalog revision.
func (s *Server) runReport(ctx context.Context, proj db.GetProjectRow, run storedRun) (exporters.Report, error) {
	ver, err := s.q.GetProjectVersion(ctx, db.GetProjectVersionParams{ID: run.VersionID, ProjectID: run.ProjectID})
	if err != nil {
		return exporters.Report{}, fmt.Errorf("report.version: %w", err)
	}
	var snap projects.Snapshot
	if err := json.Unmarshal(ver.Snapshot, &snap); err != nil {
		return exporters.Report{}, fmt.Errorf("report.snapshot: %w", err)
	}
	var current projects.Draft
	if run.Kind == exporters.KindSimulation {
		if current, _, err = s.ensureProjectGraph(ctx, proj); err != nil {
			return exporters.Report{}, fmt.Errorf("report.graph: %w", err)
		}
	}
	rep := exporters.Report{
		GeneratedAt: time.Now().UTC(),
		ProjectName: snap.Name,
		ObjectType:  snap.ObjectType,
		Inputs:      snap.Draft,
		Run: &exporters.RunInfo{
			ID:              run.ID.String(),
			Kind:            run.Kind,
			VersionNo:       int(run.VersionNo),
			InputHash:       run.InputHash,
			EconVersion:     run.EconVersion,
			SimVersion:      run.SimVersion,
			ConfidenceLevel: run.ConfidenceLevel,
			StaleVsDraft:    run.stale(current, proj.InputHash),
		},
	}
	if at := timestamptzPtr(run.CreatedAt); at != nil {
		rep.Run.CreatedAt = at.UTC()
	}
	fleetOf := snap.Draft.Variants
	switch run.Kind {
	case exporters.KindCalculation:
		var res econ.Result
		if err := json.Unmarshal(run.Summary, &res); err != nil {
			return exporters.Report{}, fmt.Errorf("report.econ: %w", err)
		}
		checks, err := s.simChecks(ctx, run.ProjectID, snap.Draft)
		if err != nil {
			return exporters.Report{}, err
		}
		if checks != nil {
			res.SimChecks = checks
			res.VerificationFlag = econ.SimChecksFlag(checks)
		}
		rep.Econ = &res
	case exporters.KindSimulation:
		var sum simbuild.Summary
		if err := json.Unmarshal(run.Summary, &sum); err != nil {
			return exporters.Report{}, fmt.Errorf("report.sim: %w", err)
		}
		rep.Sim = simReport(sum)
		fleetOf = nil
		for _, v := range snap.Draft.Variants {
			if v.ID == sum.VariantID {
				fleetOf = []projects.Variant{v}
			}
		}
	}
	cat, err := s.reportCatalog(ctx, solutionIDs(fleetOf))
	if err != nil {
		return exporters.Report{}, err
	}
	rep.Catalog = cat
	return rep, nil
}

func simReport(sum simbuild.Summary) *exporters.SimReport {
	out := &exporters.SimReport{
		VariantName: sum.VariantName,
		MapWarnings: sum.MapWarnings,
		Result:      sum.Result,
	}
	if sum.EconCheck != nil {
		out.EconCheck = &exporters.EconCheck{Flag: sum.EconCheck.Flag, Text: sum.EconCheck.Text}
	}
	return out
}

func solutionIDs(vars []projects.Variant) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	var out []uuid.UUID
	for _, v := range vars {
		for _, f := range v.Fleet {
			if f.SolutionID == nil {
				continue
			}
			id, err := uuid.Parse(*f.SolutionID)
			if err != nil || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// reportCatalog lists the solutions the variants use, as the catalog holds them now, to name them and find the ones
// without a price.
func (s *Server) reportCatalog(ctx context.Context, ids []uuid.UUID) (exporters.CatalogInfo, error) {
	var out exporters.CatalogInfo
	if s.q == nil || len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.ListSolutionsByIDs(ctx, ids)
	if err != nil {
		return out, fmt.Errorf("report.catalog.items: %w", err)
	}
	for _, row := range rows {
		out.Items = append(out.Items, exporters.CatalogItem{
			SolutionID: row.ID.String(),
			Name:       row.Name,
			PriceRub:   numericPtr(row.PriceRub),
			SourceURL:  ptrString(row.SourceUrl),
			Confidence: ptrString(row.Confidence),
			SourcedAt:  timestamptzPtr(row.SourcedAt),
		})
	}
	return out, nil
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *Server) writeRunReport(w http.ResponseWriter, r *http.Request, format string, proj db.GetProjectRow, run storedRun) {
	rep, err := s.runReport(r.Context(), proj, run)
	if err != nil {
		s.internal(w, r, "export.run", err, "Не удалось собрать отчёт по запуску. Повторите запрос.")
		return
	}
	s.writeReport(w, r, format, rep)
}

func (s *Server) exportCalculation(w http.ResponseWriter, r *http.Request) {
	format, ok := parseExportFormat(w, r)
	if !ok {
		return
	}
	run, ok := s.scopedRun(w, r)
	if !ok {
		return
	}
	proj := projectFrom(r)
	s.writeRunReport(w, r, format, proj, calcRun(run))
}

func (s *Server) exportSimulationRun(w http.ResponseWriter, r *http.Request) {
	format, ok := parseExportFormat(w, r)
	if !ok {
		return
	}
	run, proj, ok := s.scopedSimRun(w, r)
	if !ok {
		return
	}
	if run.Status != jobs.StatusSucceeded || len(run.Summary) == 0 {
		writeError(w, r, http.StatusConflict, "Симуляция ещё не завершена или завершилась ошибкой. Дождитесь результата или повторите запуск.")
		return
	}
	s.writeRunReport(w, r, format, proj, simRun(run))
}

type runListItem struct {
	runMetaJSON
	JobID        *string         `json:"job_id"`
	StaleVsDraft bool            `json:"stale_vs_draft"`
	Brief        json.RawMessage `json:"brief"`
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	ctx := r.Context()
	d, hash, err := s.ensureProjectGraph(ctx, row)
	if err != nil {
		s.internal(w, r, "runs.list.graph", err, "Не удалось загрузить историю запусков. Повторите запрос.")
		return
	}
	rows, err := s.q.ListProjectRuns(ctx, db.ListProjectRunsParams{ProjectID: row.ID, Limit: runHistoryLimit})
	if err != nil {
		s.internal(w, r, "runs.list", err, "Не удалось загрузить историю запусков. Повторите запрос.")
		return
	}
	items := make([]runListItem, 0, len(rows))
	for _, run := range rows {
		it := runListItem{
			runMetaJSON: runMetaJSON{
				ID: run.ID.String(), Kind: run.Kind, Status: run.Status, ProjectID: row.ID.String(),
				ProjectVersionID: run.ProjectVersionID.String(), VersionNo: run.VersionNo, InputHash: run.InputHash,
				MatchVersion: run.MatchVersion, EconVersion: run.EconVersion, SimVersion: run.SimVersion, Seed: run.Seed,
				ConfidenceLevel: run.ConfidenceLevel,
				CreatedAt:       timestamptzPtr(run.CreatedAt), IsCurrent: row.CurrentRunID != nil && *row.CurrentRunID == run.ID,
			},
			Brief: run.Brief,
		}
		if run.JobID != "" {
			id := run.JobID
			it.JobID = &id
		}
		st := storedRun{Kind: run.Kind, DraftHash: run.DraftHash, SimVersion: run.SimVersion}
		if run.Kind == exporters.KindSimulation {
			st.VariantID, st.VariantHash = summaryVariant(run.Brief)
		}
		it.StaleVsDraft = st.stale(d, hash)
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":          items,
		"input_hash":     hash,
		"current_run_id": uuidStringPtr(row.CurrentRunID),
	})
}

func exportLogName(rep exporters.Report) (kind, runID string) {
	if rep.Run == nil {
		return exporters.KindGuest, ""
	}
	return rep.Run.Kind, rep.Run.ID
}

func contentType(format string) (ctype, ext string) {
	if strings.EqualFold(format, "xlsx") {
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx"
	}
	return "application/pdf", "pdf"
}
