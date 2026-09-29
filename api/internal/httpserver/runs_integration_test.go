package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"

	"moscow_hackathon_2026/api/internal/jobs"
	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/realtime"
	"moscow_hackathon_2026/api/internal/simjobs"
	"moscow_hackathon_2026/api/internal/testdb"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type itEnv struct {
	srv  *Server
	h    http.Handler
	db   *pgxpool.Pool
	pool *jobs.Pool
	logs *syncBuffer
	hub  *realtime.Hub
}

func newEnv(t *testing.T) itEnv {
	t.Helper()
	dsn, dbPool, q := testdb.NewDSN(t, "../../../data")
	logs := &syncBuffer{}
	log := slog.New(slog.NewJSONHandler(logs, nil))
	sims := simjobs.New(q, log, 0)
	pool := jobs.New(q, sims, log, jobs.Config{Workers: 2, Poll: 50 * time.Millisecond, Heartbeat: 100 * time.Millisecond})
	hub := realtime.New(dsn, log)
	ctx, stop := context.WithCancel(context.Background())
	go hub.Run(ctx)
	t.Cleanup(func() {
		hub.CloseAll()
		stop()
	})
	srv, h := newServer(testConfig(), log, q, sims, pool, hub)
	return itEnv{srv: srv, h: h, db: dbPool, pool: pool, logs: logs, hub: hub}
}

func (e itEnv) startWorkers(t *testing.T) {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	stopPool := e.pool.Start(ctx)
	t.Cleanup(func() {
		stop()
		stopPool()
	})
}

func schemaDefaults(t *testing.T, c *itClient) map[string]any {
	t.Helper()
	var schema struct {
		Groups []struct {
			Fields []struct {
				ID      string `json:"id"`
				Default any    `json:"default"`
			} `json:"fields"`
		} `json:"groups"`
	}
	c.json(http.MethodGet, "/api/object-types/warehouse/schema", nil, http.StatusOK, &schema)
	out := map[string]any{}
	for _, g := range schema.Groups {
		for _, f := range g.Fields {
			out[f.ID] = f.Default
		}
	}
	return out
}

type projectResp struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Variants     []projects.Variant `json:"variants"`
	Stale        bool               `json:"stale"`
	InputHash    string             `json:"input_hash"`
	CurrentRunID *string            `json:"current_run_id"`
}

// warehouseProject creates a project with defaults, one unknown parameter and a mixed fleet. Everything is
// written the way the app writes it: one transaction of operations.
func warehouseProject(t *testing.T, c *itClient, name string) (projectResp, map[string]any) {
	t.Helper()
	var p projectResp
	c.json(http.MethodPost, "/api/projects", map[string]any{"name": name, "object_type": "warehouse"}, http.StatusCreated, &p)
	params := schemaDefaults(t, c)
	params["power_kw"] = nil
	setParams(t, c, p.ID, params)
	v := p.Variants[1]
	order, err := ops.KeysAfter("", 2)
	if err != nil {
		t.Fatal(err)
	}
	postTxs(t, c, p.ID, newTx(
		opInsert(ops.CollFleetItems, uuid.NewString(), map[string]any{
			"order": order[0], "variant_id": v.ID, "solution_id": h1500ID, "quantity": 2,
			"task_codes": []string{"inbound"}, "price_override_rub": 2_900_000.0, "price_override_reason": "КП 2026-09",
		}),
		opInsert(ops.CollFleetItems, uuid.NewString(), map[string]any{
			"order": order[1], "variant_id": v.ID, "solution_id": stackerID, "quantity": 1, "task_codes": []string{"putaway"},
		}),
	))
	c.json(http.MethodGet, "/api/projects/"+p.ID, nil, http.StatusOK, &p)
	return p, params
}

// setParams writes the object parameters as the object form does: one operation per parameter.
func setParams(t *testing.T, c *itClient, pid string, params map[string]any) {
	t.Helper()
	list := make([]ops.Op, 0, len(params))
	for _, key := range sortedKeys(params) {
		list = append(list, opSet(ops.CollProject, ops.CollProject, "params."+key, params[key]))
	}
	postTxs(t, c, pid, newTx(list...))
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func openXLSX(t *testing.T, body []byte) *excelize.File {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("xlsx: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

const financialSheets = "Итог,Варианты,Допущения и риски,Параметры объекта,Источники,Чувствительность"

func sheetNames(f *excelize.File) string { return strings.Join(f.GetSheetList(), ",") }

func workbookText(t *testing.T, f *excelize.File) string {
	t.Helper()
	var out []string
	for _, sheet := range f.GetSheetList() {
		out = append(out, flatten(mustRows(t, f, sheet))...)
	}
	return strings.Join(out, "\n")
}

// comparisonRows returns the rows of the comparison table on the first sheet, as stored in the cells.
func comparisonRows(t *testing.T, f *excelize.File) []string {
	t.Helper()
	rows, err := f.GetRows("Итог", excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows[1:] {
		if len(r) == 0 {
			break
		}
		out = append(out, strings.Join(r, "|"))
	}
	if len(out) < 2 {
		t.Fatalf("comparison table has %d rows: %v", len(out), rows)
	}
	return out
}

func mustNotShow(t *testing.T, where, text string, marks ...string) {
	t.Helper()
	for _, m := range marks {
		if m != "" && strings.Contains(text, m) {
			t.Fatalf("%s shows %q", where, m)
		}
	}
}

// exportFile downloads a report and returns its body and the file name the server chose.
func (c *itClient) exportFile(path string, want int) (*bytes.Buffer, string) {
	c.t.Helper()
	rec := c.do(http.MethodPost, path, map[string]any{}, false)
	if rec.Code != want {
		c.t.Fatalf("POST %s: status %d, want %d: %s", path, rec.Code, want, rec.Body.String())
	}
	if want == http.StatusOK && rec.Header().Get("X-Run-Id") == "" {
		c.t.Fatalf("POST %s: missing X-Run-Id", path)
	}
	return rec.Body, rec.Header().Get("Content-Disposition")
}

func (c *itClient) export(path string, want int) *bytes.Buffer {
	c.t.Helper()
	body, _ := c.exportFile(path, want)
	return body
}

type runsResp struct {
	Items []struct {
		ID              string          `json:"id"`
		Kind            string          `json:"kind"`
		Status          string          `json:"status"`
		VersionNo       int             `json:"version_no"`
		ConfidenceLevel string          `json:"confidence_level"`
		IsCurrent       bool            `json:"is_current"`
		StaleVsDraft    bool            `json:"stale_vs_draft"`
		JobID           *string         `json:"job_id"`
		Brief           json.RawMessage `json:"brief"`
	} `json:"items"`
	InputHash    string  `json:"input_hash"`
	CurrentRunID *string `json:"current_run_id"`
}

func TestReportIsBoundToOneRun(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "reports@example.com")
	other := register(t, env.h, "intruder@example.com")
	p, _ := warehouseProject(t, owner, "Склад отчётов")
	pid := p.ID

	var template maps.Document
	owner.json(http.MethodGet, "/api/projects/"+pid+"/map/template", nil, http.StatusOK, &template)
	putMap(t, owner, pid, template)

	owner.export("/api/projects/"+pid+"/export?format=pdf", http.StatusBadRequest)

	var first calculateResponse
	owner.json(http.MethodPost, "/api/projects/"+pid+"/calculations", map[string]any{"seed": 11}, http.StatusOK, &first)
	if first.Run == nil || first.Run.VersionNo != 1 || first.Run.ConfidenceLevel != projects.ConfidencePreliminary ||
		!first.Run.IsCurrent || first.Run.Seed != 11 || first.Run.ID != first.RunID {
		t.Fatalf("run meta %+v", first.Run)
	}

	currentBody, disposition := owner.exportFile("/api/projects/"+pid+"/export?format=xlsx", http.StatusOK)
	if !strings.Contains(disposition, `filename="raschet-warehouse-1.xlsx"`) {
		t.Fatalf("file name %q must carry the project version", disposition)
	}
	current := openXLSX(t, currentBody.Bytes())
	if got := sheetNames(current); got != financialSheets {
		t.Fatalf("current report sheets %s", got)
	}
	currentText := workbookText(t, current)
	mustNotShow(t, "current report", currentText, first.RunID, first.InputHash, h1500ID, "seed", "econ-v", "match-v", "sim-v", "LINESTRING", "res-docks", "catalog_", "catalog-v")
	for _, want := range []string{"Склад отчётов", "Мощность электроснабжения"} {
		if !strings.Contains(currentText, want) {
			t.Fatalf("current report lacks %q:\n%s", want, currentText)
		}
	}
	if strings.Contains(currentText, "Черновик проекта изменён после этого запуска") {
		t.Fatal("a report of the current run must not warn about a stale draft")
	}
	currentRows := comparisonRows(t, current)
	pdf := owner.export("/api/projects/"+pid+"/export?format=pdf", http.StatusOK)
	if !bytes.HasPrefix(pdf.Bytes(), []byte("%PDF")) {
		t.Fatal("not a pdf")
	}

	setParams(t, owner, pid, map[string]any{"pick_lines_per_day": 120000.0, "wage_picker_month_rub": 90000.0})
	owner.json(http.MethodGet, "/api/projects/"+pid, nil, http.StatusOK, &p)
	if !p.Stale {
		t.Fatal("changed params must mark the result stale")
	}
	var conflict errorBody
	owner.json(http.MethodPost, "/api/projects/"+pid+"/export?format=pdf", map[string]any{}, http.StatusConflict, &conflict)
	if conflict.Code != "stale_result" {
		t.Fatalf("stale export error %+v", conflict)
	}

	historicBody, disposition := owner.exportFile("/api/calculations/"+first.RunID+"/export?format=xlsx", http.StatusOK)
	if !strings.Contains(disposition, `filename="raschet-warehouse-1.xlsx"`) {
		t.Fatalf("historic file name %q", disposition)
	}
	historic := openXLSX(t, historicBody.Bytes())
	if got := sheetNames(historic); got != financialSheets {
		t.Fatalf("historic report sheets %s", got)
	}
	if !strings.Contains(workbookText(t, historic), "Черновик проекта изменён после этого запуска") {
		t.Fatal("a report of a historic run must say the draft has changed")
	}
	if got := strings.Join(comparisonRows(t, historic), "\n"); got != strings.Join(currentRows, "\n") {
		t.Fatalf("historic report must show the numbers of its snapshot:\n%s\nwant\n%s", got, strings.Join(currentRows, "\n"))
	}
	var opened calculateResponse
	owner.json(http.MethodGet, "/api/calculations/"+first.RunID, nil, http.StatusOK, &opened)
	if !opened.StaleVsDraft || opened.Run == nil || opened.Run.VersionNo != 1 || !opened.Run.IsCurrent || opened.ProjectName != "Склад отчётов" {
		t.Fatalf("opened historic run %+v %+v", opened.StaleVsDraft, opened.Run)
	}

	var second calculateResponse
	owner.json(http.MethodPost, "/api/projects/"+pid+"/calculations", nil, http.StatusOK, &second)
	if second.RunID == first.RunID || second.Run.VersionNo != 2 {
		t.Fatalf("second run %+v", second.Run)
	}
	freshBody, disposition := owner.exportFile("/api/projects/"+pid+"/export?format=xlsx", http.StatusOK)
	if !strings.Contains(disposition, `filename="raschet-warehouse-2.xlsx"`) {
		t.Fatalf("current file name %q", disposition)
	}
	fresh := openXLSX(t, freshBody.Bytes())
	if strings.Join(comparisonRows(t, fresh), "\n") == strings.Join(currentRows, "\n") {
		t.Fatal("current export must follow the new run")
	}
	again := openXLSX(t, owner.export("/api/calculations/"+first.RunID+"/export?format=xlsx", http.StatusOK).Bytes())
	if strings.Join(comparisonRows(t, again), "\n") != strings.Join(currentRows, "\n") {
		t.Fatal("old run report changed after a new calculation")
	}

	env.startWorkers(t)
	var simCreated createSimulationJSON
	owner.json(http.MethodPost, "/api/projects/"+pid+"/simulations", map[string]any{"variant_id": p.Variants[1].ID, "mode": "deterministic", "horizon_h": 1, "seed": 5}, http.StatusAccepted, &simCreated)
	owner.json(http.MethodPost, "/api/simulation-runs/"+simCreated.RunID+"/export?format=pdf", map[string]any{}, http.StatusConflict, nil)
	waitJob(t, owner, simCreated.Job.ID, jobs.StatusSucceeded)
	simPDF := owner.export("/api/simulation-runs/"+simCreated.RunID+"/export?format=pdf", http.StatusOK)
	if !bytes.HasPrefix(simPDF.Bytes(), []byte("%PDF")) {
		t.Fatal("simulation pdf")
	}
	simBody, disposition := owner.exportFile("/api/simulation-runs/"+simCreated.RunID+"/export?format=xlsx", http.StatusOK)
	if !strings.Contains(disposition, `filename="simulyaciya-warehouse-3.xlsx"`) {
		t.Fatalf("simulation file name %q", disposition)
	}
	simX := openXLSX(t, simBody.Bytes())
	if got := sheetNames(simX); got != "Итог,Процессы,Узкие места,Допущения и риски" {
		t.Fatalf("simulation report sheets %s", got)
	}
	simText := workbookText(t, simX)
	for _, want := range []string{"Вывод", "Производительность", p.Variants[1].Name} {
		if !strings.Contains(simText, want) {
			t.Fatalf("simulation report lacks %q:\n%s", want, simText)
		}
	}
	mustNotShow(t, "simulation report", simText, simCreated.RunID, "seed", "sim-v", "econ-v", "match-v", "LINESTRING", h1500ID)
	owner.json(http.MethodPost, "/api/calculations/"+simCreated.RunID+"/export", map[string]any{}, http.StatusNotFound, nil)

	var runs runsResp
	owner.json(http.MethodGet, "/api/projects/"+pid+"/runs", nil, http.StatusOK, &runs)
	if len(runs.Items) != 3 || runs.CurrentRunID == nil || *runs.CurrentRunID != second.RunID {
		t.Fatalf("runs %+v", runs)
	}
	sr, cr2, cr1 := runs.Items[0], runs.Items[1], runs.Items[2]
	if sr.Kind != "simulation" || sr.ID != simCreated.RunID || sr.ConfidenceLevel != projects.ConfidenceConfigured ||
		sr.JobID == nil || *sr.JobID != simCreated.Job.ID || sr.StaleVsDraft || sr.VersionNo != 3 {
		t.Fatalf("simulation history item %+v", sr)
	}
	if cr2.Kind != "calculation" || cr2.ID != second.RunID || !cr2.IsCurrent || cr2.StaleVsDraft || cr2.JobID != nil {
		t.Fatalf("current calculation item %+v", cr2)
	}
	if cr1.ID != first.RunID || cr1.IsCurrent || !cr1.StaleVsDraft {
		t.Fatalf("first calculation item %+v", cr1)
	}
	var calcBrief struct {
		VariantNames []string `json:"variant_names"`
		BestPayback  *float64 `json:"best_payback_years"`
	}
	if err := json.Unmarshal(cr2.Brief, &calcBrief); err != nil || len(calcBrief.VariantNames) != 3 {
		t.Fatalf("calculation brief %s %v", cr2.Brief, err)
	}
	var simBrief struct {
		Verdict     string `json:"verdict"`
		VariantName string `json:"variant_name"`
	}
	if err := json.Unmarshal(sr.Brief, &simBrief); err != nil || simBrief.Verdict == "" || simBrief.VariantName != p.Variants[1].Name {
		t.Fatalf("simulation brief %s %v", sr.Brief, err)
	}

	other.json(http.MethodGet, "/api/projects/"+pid+"/runs", nil, http.StatusNotFound, nil)
	other.json(http.MethodGet, "/api/calculations/"+first.RunID, nil, http.StatusNotFound, nil)
	other.export("/api/calculations/"+first.RunID+"/export?format=pdf", http.StatusNotFound)
	other.export("/api/simulation-runs/"+simCreated.RunID+"/export?format=pdf", http.StatusNotFound)
	guest := &itClient{t: t, h: env.h}
	guest.json(http.MethodGet, "/api/projects/"+pid+"/runs", nil, http.StatusForbidden, nil)
	guest.export("/api/calculations/"+first.RunID+"/export?format=pdf", http.StatusForbidden)

	var versions struct {
		Items []map[string]any `json:"items"`
	}
	owner.json(http.MethodGet, "/api/projects/"+pid+"/versions", nil, http.StatusOK, &versions)
	if len(versions.Items) != 3 {
		t.Fatalf("each run must own one snapshot, got %d versions", len(versions.Items))
	}
	var snap projects.Snapshot
	owner.json(http.MethodGet, "/api/projects/"+pid+"/versions/"+versions.Items[0]["id"].(string), nil, http.StatusOK, &snap)
	if snap.ConfidenceLevel != projects.ConfidenceConfigured || snap.SimVersion != "sim-v2" {
		t.Fatalf("simulation snapshot model %s %s", snap.ConfidenceLevel, snap.SimVersion)
	}
}

func flatten(rows [][]string) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r...)
	}
	return out
}

func mustRows(t *testing.T, f *excelize.File, sheet string) [][]string {
	t.Helper()
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestOtherUserCannotReachProject(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "owner-acl@example.com")
	other := register(t, env.h, "other-acl@example.com")
	guest := &itClient{t: t, h: env.h}
	p, _ := warehouseProject(t, owner, "Закрытый склад")
	pid := p.ID

	var calc calculateResponse
	owner.json(http.MethodPost, "/api/projects/"+pid+"/calculations", nil, http.StatusOK, &calc)
	var template maps.Document
	owner.json(http.MethodGet, "/api/projects/"+pid+"/map/template", nil, http.StatusOK, &template)
	var versions struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	owner.json(http.MethodGet, "/api/projects/"+pid+"/versions", nil, http.StatusOK, &versions)
	var simCreated createSimulationJSON
	owner.json(http.MethodPost, "/api/projects/"+pid+"/simulations", map[string]any{"mode": "deterministic", "horizon_h": 1}, http.StatusAccepted, &simCreated)
	var before projectResp
	owner.json(http.MethodGet, "/api/projects/"+pid, nil, http.StatusOK, &before)
	routes := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/projects/" + pid, nil},
		{http.MethodPost, "/api/projects/" + pid + "/transactions", map[string]any{"client_id": "intruder", "schema_version": 1, "txs": []any{}}},
		{http.MethodGet, "/api/projects/" + pid + "/snapshot", nil},
		{http.MethodGet, "/api/projects/" + pid + "/operations", nil},
		{http.MethodPost, "/api/projects/" + pid + "/copy", map[string]any{}},
		{http.MethodPost, "/api/projects/" + pid + "/calculations", map[string]any{}},
		{http.MethodPost, "/api/projects/" + pid + "/calculate", map[string]any{}},
		{http.MethodGet, "/api/projects/" + pid + "/calculations", nil},
		{http.MethodGet, "/api/projects/" + pid + "/runs", nil},
		{http.MethodGet, "/api/projects/" + pid + "/processes", nil},
		{http.MethodGet, "/api/projects/" + pid + "/variants", nil},
		{http.MethodGet, "/api/projects/" + pid + "/shared-costs", nil},
		{http.MethodGet, "/api/projects/" + pid + "/assumption-sets", nil},
		{http.MethodGet, "/api/projects/" + pid + "/versions", nil},
		{http.MethodGet, "/api/projects/" + pid + "/versions/" + versions.Items[0].ID, nil},
		{http.MethodPost, "/api/projects/" + pid + "/map/validate", template},
		{http.MethodGet, "/api/projects/" + pid + "/map/template", nil},
		{http.MethodGet, "/api/projects/" + pid + "/simulations", nil},
		{http.MethodPost, "/api/projects/" + pid + "/simulations", map[string]any{}},
		{http.MethodGet, "/api/projects/" + pid + "/match", nil},
		{http.MethodPost, "/api/projects/" + pid + "/export?format=pdf", map[string]any{}},
		{http.MethodGet, "/api/calculations/" + calc.RunID, nil},
		{http.MethodPost, "/api/calculations/" + calc.RunID + "/export?format=xlsx", map[string]any{}},
		{http.MethodGet, "/api/simulation-jobs/" + simCreated.Job.ID, nil},
		{http.MethodDelete, "/api/simulation-jobs/" + simCreated.Job.ID, nil},
		{http.MethodPost, "/api/simulation-jobs/" + simCreated.Job.ID + "/retry", nil},
		{http.MethodGet, "/api/simulation-runs/" + simCreated.RunID, nil},
		{http.MethodGet, "/api/simulation-runs/" + simCreated.RunID + "/events", nil},
		{http.MethodPut, "/api/simulation-runs/" + simCreated.RunID + "/pin", map[string]any{"pinned": true}},
		{http.MethodPost, "/api/simulation-runs/" + simCreated.RunID + "/export?format=pdf", map[string]any{}},
		{http.MethodDelete, "/api/projects/" + pid, nil},
	}
	for _, rt := range routes {
		if rec := other.do(rt.method, rt.path, rt.body, false); rec.Code != http.StatusNotFound {
			t.Errorf("other user %s %s: status %d, want 404: %s", rt.method, rt.path, rec.Code, rec.Body.String())
		}
		if rec := guest.do(rt.method, rt.path, rt.body, false); rec.Code != http.StatusForbidden {
			t.Errorf("guest %s %s: status %d, want 403: %s", rt.method, rt.path, rec.Code, rec.Body.String())
		}
	}

	var after projectResp
	owner.json(http.MethodGet, "/api/projects/"+pid, nil, http.StatusOK, &after)
	if after.Name != before.Name || after.InputHash != before.InputHash || len(after.Variants) != len(before.Variants) {
		t.Fatalf("project changed by another user: %+v vs %+v", after, before)
	}
	var job simulationJobJSON
	owner.json(http.MethodGet, "/api/simulation-jobs/"+simCreated.Job.ID, nil, http.StatusOK, &job)
	if job.CanceledAt != nil {
		t.Fatal("another user canceled the job")
	}
	var runs runsResp
	owner.json(http.MethodGet, "/api/projects/"+pid+"/runs", nil, http.StatusOK, &runs)
	if len(runs.Items) != 2 {
		t.Fatalf("another user created runs: %d", len(runs.Items))
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	other.json(http.MethodGet, "/api/projects", nil, http.StatusOK, &list)
	if len(list.Items) != 0 {
		t.Fatalf("other user sees projects: %v", list.Items)
	}
}

func TestGuestDoesNotPersistCommercialInputs(t *testing.T) {
	env := newEnv(t)
	guest := &itClient{t: t, h: env.h}
	params := schemaDefaults(t, guest)
	params["wage_picker_month_rub"] = 123457.0
	params["capex_budget_mln_rub"] = 91.37
	ctx := context.Background()
	tables := []string{"users", "projects", "project_drafts", "project_versions", "project_processes", "solution_variants",
		"variant_fleet_items", "financing_scenarios", "calculation_runs", "calculation_results", "simulation_jobs",
		"simulation_replications", "simulation_artifacts", "shared_cost_items", "assumption_sets", "solutions"}
	count := func() map[string]int64 {
		out := map[string]int64{}
		for _, tbl := range tables {
			var n int64
			if err := env.db.QueryRow(ctx, "SELECT count(*) FROM "+tbl).Scan(&n); err != nil {
				t.Fatalf("count %s: %v", tbl, err)
			}
			out[tbl] = n
		}
		return out
	}
	before := count()

	body := map[string]any{"object_type": "warehouse", "params": params, "include_ids": []string{h1500ID}}
	guest.json(http.MethodPost, "/api/guest/calculate", body, http.StatusOK, nil)
	guest.json(http.MethodPost, "/api/guest/match", map[string]any{"object_type": "warehouse", "params": params}, http.StatusOK, nil)
	for _, format := range []string{"pdf", "xlsx"} {
		rec := guest.do(http.MethodPost, "/api/guest/export?format="+format, body, false)
		if rec.Code != http.StatusOK || rec.Header().Get("X-Run-Id") != "" {
			t.Fatalf("guest export %s: %d run %q", format, rec.Code, rec.Header().Get("X-Run-Id"))
		}
		if format == "xlsx" {
			if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="ocenka-warehouse.xlsx"`) {
				t.Fatalf("guest file name %q", cd)
			}
			f := openXLSX(t, rec.Body.Bytes())
			if got := sheetNames(f); got != financialSheets {
				t.Fatalf("guest report sheets %s", got)
			}
			if !strings.Contains(workbookText(t, f), "Не сохранён") {
				t.Fatal("a guest report must say it is not saved")
			}
			if !strings.Contains(workbookText(t, f), "123457") {
				t.Fatal("a guest report lists the object parameters the guest sent, in the guest's own file")
			}
		}
	}
	guest.json(http.MethodPost, "/api/projects", map[string]any{"name": "g", "object_type": "warehouse", "params": params}, http.StatusForbidden, nil)

	after := count()
	for _, tbl := range tables {
		if before[tbl] != after[tbl] {
			t.Errorf("guest flow changed %s: %d -> %d", tbl, before[tbl], after[tbl])
		}
	}
	var leaked int64
	if err := env.db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM projects WHERE params::text LIKE '%123457%') +
		(SELECT count(*) FROM project_drafts WHERE document::text LIKE '%123457%') +
		(SELECT count(*) FROM calculation_results WHERE summary::text LIKE '%123457%')`).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked != 0 {
		t.Fatalf("guest inputs found in %d rows", leaked)
	}
	logs := env.logs.String()
	if strings.Contains(logs, "123457") || strings.Contains(logs, "91.37") {
		t.Fatal("guest commercial inputs leaked into logs")
	}
}

func TestParamValidationKeepsDraft(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "params@example.com")
	p, params := warehouseProject(t, owner, "Проверка параметров")
	var calc calculateResponse
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/calculations", nil, http.StatusOK, &calc)

	bad := map[string]any{}
	for k, v := range params {
		bad[k] = v
	}
	bad["area_total_m2"] = "двадцать тысяч"
	bad["floors"] = 1.5
	bad["peak_load_factor"] = 9.0
	bad["has_wms"] = "может быть"
	bad["shift_window"] = map[string]any{"start": "25:00", "end": "08:00"}
	bad["staff_total"] = nil
	bad["legacy_field"] = 1
	delete(bad, "pallet_slots")
	// Each bad parameter is its own operation, so the refusal names the one that is wrong; a transaction with
	// a refused operation writes nothing at all.
	for _, key := range sortedKeys(bad) {
		if reflect.DeepEqual(bad[key], params[key]) {
			continue
		}
		out := postTxs(t, owner, p.ID, newTx(opSet(ops.CollProject, ops.CollProject, "params."+key, bad[key])))
		res := out.Results[0]
		if res.Status != "rejected" {
			t.Errorf("params.%s accepted: %+v", key, res)
			continue
		}
		if res.Details == nil || res.Details.Field != "params."+key {
			t.Errorf("params.%s refused without naming the field: %+v", key, res.Details)
		}
	}
	// A parameter the schema does not know is refused the same way.
	out := postTxs(t, owner, p.ID, newTx(opSet(ops.CollProject, ops.CollProject, "params.legacy_field", 1)))
	if out.Results[0].Status != "rejected" {
		t.Fatalf("unknown parameter accepted: %+v", out.Results[0])
	}
	var after projectResp
	owner.json(http.MethodGet, "/api/projects/"+p.ID, nil, http.StatusOK, &after)
	if after.Stale || after.InputHash != calc.InputHash {
		t.Fatal("rejected params must not change the draft")
	}
	setParams(t, owner, p.ID, params)
	owner.json(http.MethodGet, "/api/projects/"+p.ID, nil, http.StatusOK, &after)
	if after.Stale {
		t.Fatal("saving the same params must keep the result current")
	}
}

func TestProjectGraphWritesAreAtomic(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "atomic@example.com")
	p, _ := warehouseProject(t, owner, "Атомарность")
	v := p.Variants[1]
	if len(v.Fleet) != 2 {
		t.Fatalf("fixture fleet %d", len(v.Fleet))
	}
	order, err := ops.KeysAfter("", 3)
	if err != nil {
		t.Fatal(err)
	}
	ghost := "0b1c2d3e-4f50-4a6b-8c7d-9e0f1a2b3c4d"
	// One good operation and one pointing at a robot outside the revision: the whole transaction is refused.
	out := postTxs(t, owner, p.ID, newTx(
		opSet(ops.CollVariants, v.ID, "name", "Переименован"),
		opInsert(ops.CollFleetItems, uuid.NewString(), map[string]any{
			"order": order[2], "variant_id": v.ID, "solution_id": ghost, "quantity": 1,
		}),
	))
	if out.Results[0].Status != "rejected" || out.Results[0].Reason != ops.ReasonRefMissing {
		t.Fatalf("unknown solution accepted: %+v", out.Results[0])
	}
	var after projectResp
	owner.json(http.MethodGet, "/api/projects/"+p.ID, nil, http.StatusOK, &after)
	if len(after.Variants[1].Fleet) != 2 || after.Variants[1].Name != v.Name || after.InputHash != p.InputHash {
		t.Fatalf("failed transaction changed the variant: %d items, name %q", len(after.Variants[1].Fleet), after.Variants[1].Name)
	}

	var procs struct {
		Items []projects.Process `json:"items"`
	}
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/processes", nil, http.StatusOK, &procs)
	// A code another process already uses is refused, and nothing of the transaction lands.
	out = postTxs(t, owner, p.ID, newTx(
		opSet(ops.CollProcesses, procs.Items[1].ID, "name", "Переименован"),
		opInsert(ops.CollProcesses, uuid.NewString(), map[string]any{
			"order": order[0], "code": procs.Items[0].Code, "name": "Дубль", "task_type": "pallet_move",
			"demand": map[string]any{"units_per_day": 1, "unit": "ед"}, "sla": map[string]any{"max_wait_min": 30},
			"durations": map[string]any{"load_s": 60, "unload_s": 60}, "baseline_staff": map[string]any{"headcount": 1},
		}),
	))
	if out.Results[0].Status != "rejected" || out.Results[0].Reason != ops.ReasonUnique {
		t.Fatalf("duplicate code accepted: %+v", out.Results[0])
	}
	var again struct {
		Items []projects.Process `json:"items"`
	}
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/processes", nil, http.StatusOK, &again)
	if len(again.Items) != 4 || again.Items[1].Name != procs.Items[1].Name {
		t.Fatalf("failed transaction touched the processes: %d items", len(again.Items))
	}

	ctx := context.Background()
	var uid string
	if err := env.db.QueryRow(ctx, "SELECT id::text FROM users WHERE email = 'atomic@example.com'").Scan(&uid); err != nil {
		t.Fatal(err)
	}
	var legacy string
	if err := env.db.QueryRow(ctx, `INSERT INTO projects (user_id, name, object_type, params, model_version)
		VALUES ($1, 'Старый проект', 'warehouse', '{}', 'econ-v1') RETURNING id::text`, uid).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	codes := make(chan int, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			path := "/api/projects/" + legacy + "/variants"
			if i%2 == 1 {
				path = "/api/projects/" + legacy + "/processes"
			}
			codes <- owner.do(http.MethodGet, path, nil, false).Code
		}(i)
	}
	close(start)
	wg.Wait()
	close(codes)
	for c := range codes {
		if c != http.StatusOK {
			t.Fatalf("concurrent first open: status %d", c)
		}
	}
	var vars, prs int
	if err := env.db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM solution_variants WHERE project_id = $1),
		(SELECT count(*) FROM project_processes WHERE project_id = $1)`, legacy).Scan(&vars, &prs); err != nil {
		t.Fatal(err)
	}
	if vars != 3 || prs != 4 {
		t.Fatalf("defaults seeded %d variants and %d processes, want 3 and 4", vars, prs)
	}
}

// TestFormatMigrationKeepsRunCurrent replays migrations 0020 and 0021: a run saved while the sizes were "LxWxH"
// strings stays current after they became objects, and opening the project adds nothing to the history.
func TestFormatMigrationKeepsRunCurrent(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "rehash@example.com")
	p, _ := warehouseProject(t, owner, "Склад форматов")
	pid := p.ID
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := env.db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}

	var older, cur calculateResponse
	owner.json(http.MethodPost, "/api/projects/"+pid+"/calculations", map[string]any{}, http.StatusOK, &older)
	setParams(t, owner, pid, map[string]any{"pick_lines_per_day": 120000.0, "wage_picker_month_rub": 90000.0})
	owner.json(http.MethodPost, "/api/projects/"+pid+"/calculations", map[string]any{}, http.StatusOK, &cur)

	// What the migrations left: the current run's snapshot holds string sizes, the project and the run carry
	// the hash the old code computed from them, and no run has a draft_hash.
	for _, key := range []string{"pallet_size_mm", "unit_size_mm"} {
		exec(`UPDATE project_versions v
			SET snapshot = jsonb_set(v.snapshot, ARRAY['draft', 'params', $2::text], to_jsonb(concat_ws('x',
				v.snapshot #>> ARRAY['draft', 'params', $2::text, 'length'],
				v.snapshot #>> ARRAY['draft', 'params', $2::text, 'width'],
				v.snapshot #>> ARRAY['draft', 'params', $2::text, 'height'])))
			FROM calculation_runs r
			WHERE r.project_version_id = v.id AND r.id = $1`, cur.RunID, key)
	}
	exec(`UPDATE calculation_runs SET input_hash = 'legacy' WHERE id = $1`, cur.RunID)
	exec(`UPDATE calculation_runs SET draft_hash = NULL WHERE project_id = $1`, pid)
	exec(`UPDATE projects SET input_hash = 'legacy' WHERE id = $1`, pid)
	exec(`UPDATE project_drafts SET input_hash = 'legacy' WHERE project_id = $1`, pid)

	listStale := func() bool {
		t.Helper()
		var list struct {
			Items []struct {
				ID    string `json:"id"`
				Stale bool   `json:"stale"`
			} `json:"items"`
		}
		owner.json(http.MethodGet, "/api/projects", nil, http.StatusOK, &list)
		for _, it := range list.Items {
			if it.ID == pid {
				return it.Stale
			}
		}
		t.Fatalf("project %s is not in the list", pid)
		return false
	}
	if listStale() {
		t.Fatal("start page before opening: run must be current")
	}
	owner.json(http.MethodGet, "/api/projects/"+pid, nil, http.StatusOK, &p)
	if p.Stale || p.InputHash != cur.InputHash {
		t.Fatalf("opened project: stale=%v input_hash=%s, want current with %s", p.Stale, p.InputHash, cur.InputHash)
	}
	if listStale() {
		t.Fatal("start page after opening: run must be current")
	}

	var runs struct {
		Items []struct {
			ID           string `json:"id"`
			InputHash    string `json:"input_hash"`
			StaleVsDraft bool   `json:"stale_vs_draft"`
		} `json:"items"`
	}
	owner.json(http.MethodGet, "/api/projects/"+pid+"/runs", nil, http.StatusOK, &runs)
	got := map[string]bool{}
	for _, r := range runs.Items {
		got[r.ID] = r.StaleVsDraft
		if r.ID == cur.RunID && r.InputHash != "legacy" {
			t.Fatalf("recorded input_hash rewritten to %s", r.InputHash)
		}
	}
	if len(runs.Items) != 2 || got[cur.RunID] || !got[older.RunID] {
		t.Fatalf("history %+v: want 2 runs, only the older one stale", runs.Items)
	}
	owner.export("/api/projects/"+pid+"/export?format=xlsx", http.StatusOK)

	var before, after time.Time
	if err := env.db.QueryRow(ctx, `SELECT updated_at FROM projects WHERE id = $1`, pid).Scan(&before); err != nil {
		t.Fatal(err)
	}
	owner.json(http.MethodGet, "/api/projects/"+pid, nil, http.StatusOK, &p)
	if err := env.db.QueryRow(ctx, `SELECT updated_at FROM projects WHERE id = $1`, pid).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Fatalf("reading the project moved updated_at from %v to %v", before, after)
	}

	// A run of another econ model does not become current by rehashing: its numbers came from that model.
	exec(`UPDATE calculation_runs SET draft_hash = NULL, econ_version = 'econ-v0' WHERE id = $1`, cur.RunID)
	owner.json(http.MethodGet, "/api/projects/"+pid, nil, http.StatusOK, &p)
	if !p.Stale {
		t.Fatal("a run of an older econ model must be stale")
	}
}
