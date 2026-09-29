package httpserver

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/config"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/jobs"
	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/sim"
	"moscow_hackathon_2026/api/internal/simbuild"
	"moscow_hackathon_2026/api/internal/simjobs"
	"moscow_hackathon_2026/api/internal/testdb"
)

const (
	h1500ID   = "5760e938-9a43-45a7-b8e8-f4f2e6383930"
	stackerID = "2ffc706d-fe43-4c2b-baad-a624a95ad3ce"
)

// itClient keeps the session cookie and CSRF token like the web client does.
type itClient struct {
	t      *testing.T
	h      http.Handler
	cookie *http.Cookie
	csrf   string
}

func testConfig() config.Config {
	return config.Config{PublicOrigin: "http://localhost", BcryptCost: 10, SimActivePerUser: 3, AppEnv: "local"}
}

func (c *itClient) do(method, path string, body any, gzipOK bool) *httptest.ResponseRecorder {
	c.t.Helper()
	return c.doCtx(context.Background(), method, path, body, gzipOK)
}

// doCtx is do with a request context; streaming routes return when it ends.
func (c *itClient) doCtx(ctx context.Context, method, path string, body any, gzipOK bool) *httptest.ResponseRecorder {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		rdr = bytes.NewReader(raw)
	}
	req := httptest.NewRequestWithContext(ctx, method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if unsafeMethod(method) {
		req.Header.Set("Origin", "http://localhost")
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if c.csrf != "" && unsafeMethod(method) {
		req.Header.Set(csrfHeader, c.csrf)
	}
	if gzipOK {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.Name != cookiePlainName {
			continue
		}
		if ck.MaxAge < 0 || ck.Value == "" {
			c.cookie, c.csrf = nil, ""
		} else {
			c.cookie = &http.Cookie{Name: ck.Name, Value: ck.Value}
		}
	}
	return rec
}

func (c *itClient) json(method, path string, body any, want int, out any) {
	c.t.Helper()
	rec := c.do(method, path, body, false)
	if rec.Code != want {
		c.t.Fatalf("%s %s: status %d, want %d: %s", method, path, rec.Code, want, rec.Body.String())
	}
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			c.t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
}

func register(t *testing.T, h http.Handler, email string) *itClient {
	c := &itClient{t: t, h: h}
	var auth authJSON
	c.json(http.MethodPost, "/api/auth/register", map[string]string{"email": email, "password": "secret-password-1"}, http.StatusOK, &auth)
	if c.cookie == nil || auth.CSRFToken == "" {
		t.Fatalf("register %s: no session", email)
	}
	c.csrf = auth.CSRFToken
	return c
}

func waitJob(t *testing.T, c *itClient, id string, want string) simulationJobJSON {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		var job simulationJobJSON
		c.json(http.MethodGet, "/api/simulation-jobs/"+id, nil, http.StatusOK, &job)
		if job.Status == want {
			return job
		}
		if !jobs.Active(job.Status) || time.Now().After(deadline) {
			t.Fatalf("job %s: status %s (%v), want %s", id, job.Status, job.ErrorText, want)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestSimulationLifecycle(t *testing.T) {
	_, q := testdb.New(t, "../../../data")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sims := simjobs.New(q, log, 0)
	pool := jobs.New(q, sims, log, jobs.Config{Workers: 2, Poll: 50 * time.Millisecond, Heartbeat: 100 * time.Millisecond})
	h := New(testConfig(), log, q, sims, pool, nil)
	owner := register(t, h, "owner@example.com")
	other := register(t, h, "other@example.com")

	var project struct {
		ID       string             `json:"id"`
		Variants []projects.Variant `json:"variants"`
		Stale    bool               `json:"stale"`
	}
	owner.json(http.MethodPost, "/api/projects", map[string]any{"name": "Склад", "object_type": "warehouse"}, http.StatusCreated, &project)
	pid := project.ID
	params := map[string]any{}
	var schemaDefaults struct {
		Groups []struct {
			Fields []struct {
				ID      string `json:"id"`
				Default any    `json:"default"`
			} `json:"fields"`
		} `json:"groups"`
	}
	owner.json(http.MethodGet, "/api/object-types/warehouse/schema", nil, http.StatusOK, &schemaDefaults)
	for _, g := range schemaDefaults.Groups {
		for _, f := range g.Fields {
			params[f.ID] = f.Default
		}
	}
	setParams(t, owner, pid, params)
	owner.json(http.MethodGet, "/api/projects/"+pid, nil, http.StatusOK, &project)

	var noFleet map[string]any
	owner.json(http.MethodPost, "/api/projects/"+pid+"/simulations", map[string]any{}, http.StatusBadRequest, &noFleet)

	variant := project.Variants[0]
	h1500, stacker := h1500ID, stackerID
	variant.Fleet = []projects.FleetItem{
		{SolutionID: &h1500, Quantity: 1, TaskCodes: []string{"inbound"}},
		{SolutionID: &stacker, Quantity: 1, TaskCodes: []string{"putaway"}},
	}
	order, err := ops.KeysAfter("", 2)
	if err != nil {
		t.Fatal(err)
	}
	inbound := uuid.NewString()
	postTxs(t, owner, pid, newTx(
		opInsert(ops.CollFleetItems, inbound, map[string]any{
			"order": order[0], "variant_id": variant.ID, "solution_id": h1500ID, "quantity": 1, "task_codes": []string{"inbound"},
		}),
		opInsert(ops.CollFleetItems, uuid.NewString(), map[string]any{
			"order": order[1], "variant_id": variant.ID, "solution_id": stackerID, "quantity": 1, "task_codes": []string{"putaway"},
		}),
	))

	var template maps.Document
	owner.json(http.MethodGet, "/api/projects/"+pid+"/map/template", nil, http.StatusOK, &template)
	putMap(t, owner, pid, template)

	// A flow cannot point at a missing point: the schema refuses the reference outright.
	var snap snapshotResp
	owner.json(http.MethodGet, "/api/projects/"+pid+"/snapshot", nil, http.StatusOK, &snap)
	flows := snap.state(t).IDs(ops.CollMapFlows)
	if len(flows) == 0 {
		t.Fatal("the template has no flows")
	}
	out := postTxs(t, owner, pid, newTx(opSet(ops.CollMapFlows, flows[0], "drop_point_ids", []string{"missing"})))
	if out.Results[0].Status != "rejected" {
		t.Fatalf("a dangling point reference was accepted: %+v", out.Results[0])
	}

	// A flow with no delivery point is a map the schema accepts and the check rejects; it must block the run.
	postTxs(t, owner, pid, newTx(opSet(ops.CollMapFlows, flows[0], "drop_point_ids", []string{})))
	var mapErr struct {
		Code   string       `json:"code"`
		Issues []maps.Issue `json:"issues"`
	}
	owner.json(http.MethodPost, "/api/projects/"+pid+"/simulations", map[string]any{}, http.StatusBadRequest, &mapErr)
	if mapErr.Code != "map_invalid" || len(mapErr.Issues) == 0 {
		t.Fatalf("map errors must block the run: %+v", mapErr)
	}
	putMap(t, owner, pid, template)

	var check simbuild.MapCheck
	owner.json(http.MethodPost, "/api/projects/"+pid+"/map/validate", template, http.StatusOK, &check)
	if maps.HasErrors(check.Issues) || len(check.Edges) == 0 {
		t.Fatalf("template check: %+v", check)
	}
	if len(check.Classes) != 2 {
		t.Fatalf("classes from the variant fleet: %+v", check.Classes)
	}
	other.json(http.MethodPost, "/api/projects/"+pid+"/map/validate", template, http.StatusNotFound, nil)

	body := map[string]any{"variant_id": variant.ID, "mode": "stochastic", "replications": 4, "horizon_h": 2, "seed": 3}
	var created createSimulationJSON
	owner.json(http.MethodPost, "/api/projects/"+pid+"/simulations", body, http.StatusAccepted, &created)
	if created.Preview.MapSource != simbuild.MapSourceProject || created.Preview.Confidence != simbuild.ConfidenceConfigured {
		t.Fatalf("preview %+v", created.Preview)
	}
	owner.json(http.MethodPost, "/api/projects/"+pid+"/simulations", body, http.StatusConflict, nil)
	other.json(http.MethodGet, "/api/simulation-jobs/"+created.Job.ID, nil, http.StatusNotFound, nil)

	owner.json(http.MethodDelete, "/api/simulation-jobs/"+created.Job.ID, nil, http.StatusAccepted, nil)
	canceled := waitJob(t, owner, created.Job.ID, jobs.StatusCanceled)
	if canceled.CanceledAt == nil {
		t.Fatal("canceled_at missing")
	}
	owner.json(http.MethodDelete, "/api/simulation-jobs/"+created.Job.ID, nil, http.StatusConflict, nil)

	ctx, stop := context.WithCancel(context.Background())
	stopPool := pool.Start(ctx)
	defer func() {
		stop()
		stopPool()
	}()
	owner.json(http.MethodPost, "/api/simulation-jobs/"+created.Job.ID+"/retry", nil, http.StatusAccepted, nil)
	done := waitJob(t, owner, created.Job.ID, jobs.StatusSucceeded)
	if done.ProgressPct != 100 || done.ReplicationsDone != 4 {
		t.Fatalf("finished job %+v", done)
	}
	owner.json(http.MethodPost, "/api/simulation-jobs/"+created.Job.ID+"/retry", nil, http.StatusConflict, nil)

	var run simulationRunResponse
	owner.json(http.MethodGet, "/api/simulation-runs/"+created.RunID, nil, http.StatusOK, &run)
	if run.Run.Status != jobs.StatusSucceeded || run.Run.SimVersion != sim.Version || run.StaleVsDraft {
		t.Fatalf("run %+v", run.Run)
	}
	if len(run.Replications) != 4 || run.Artifact == nil || run.Artifact.EventCount == 0 || run.Artifact.Pinned {
		t.Fatalf("replications %d artifact %+v", len(run.Replications), run.Artifact)
	}
	var summary simbuild.Summary
	if err := json.Unmarshal(run.Summary, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Result.Replications != 4 || summary.VariantID != variant.ID || summary.MapSource != simbuild.MapSourceProject ||
		summary.Result.Verdict == "" || len(summary.Result.Fleet) != 2 || summary.InputHash != created.InputHash {
		t.Fatalf("summary %+v", summary)
	}
	if summary.Result.Verdict != sim.VerdictFail || len(summary.Result.Bottlenecks) == 0 {
		t.Fatalf("one robot per process must fail SLA: %s", summary.Result.VerdictText)
	}

	gz := owner.do(http.MethodGet, "/api/simulation-runs/"+created.RunID+"/events", nil, true)
	if gz.Code != http.StatusOK || gz.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("gzip events: %d %v", gz.Code, gz.Header())
	}
	zr, err := gzip.NewReader(gz.Body)
	if err != nil {
		t.Fatal(err)
	}
	var journal sim.Log
	if err := json.NewDecoder(zr).Decode(&journal); err != nil {
		t.Fatal(err)
	}
	var plain sim.Log
	owner.json(http.MethodGet, "/api/simulation-runs/"+created.RunID+"/events", nil, http.StatusOK, &plain)
	if len(journal.Events) == 0 || len(journal.Events) != len(plain.Events) || journal.Seed != summary.Result.RepresentativeSeed {
		t.Fatalf("journal %d vs %d", len(journal.Events), len(plain.Events))
	}
	other.json(http.MethodGet, "/api/simulation-runs/"+created.RunID+"/events", nil, http.StatusNotFound, nil)

	var pin map[string]any
	owner.json(http.MethodPut, "/api/simulation-runs/"+created.RunID+"/pin", map[string]bool{"pinned": true}, http.StatusOK, &pin)
	if pin["pinned"] != true || pin["expires_at"] != nil {
		t.Fatalf("pin %+v", pin)
	}

	var checks struct {
		Items []econ.SimCheck `json:"items"`
	}
	owner.json(http.MethodGet, "/api/projects/"+pid+"/sim-checks", nil, http.StatusOK, &checks)
	if len(checks.Items) != 1 || checks.Items[0].RunID != created.RunID || checks.Items[0].Stale || checks.Items[0].Model != econ.SimCheckMap ||
		checks.Items[0].VariantID != variant.ID || checks.Items[0].Text == "" {
		t.Fatalf("the finished run must check its variant: %+v", checks.Items)
	}
	other.json(http.MethodGet, "/api/projects/"+pid+"/sim-checks", nil, http.StatusNotFound, nil)
	postTxs(t, owner, pid, newTx(opSet(ops.CollVariants, project.Variants[1].ID, "name", "Другой вариант")))
	owner.json(http.MethodGet, "/api/projects/"+pid+"/sim-checks", nil, http.StatusOK, &checks)
	if len(checks.Items) != 1 || checks.Items[0].Stale {
		t.Fatalf("editing another variant must not make the check stale: %+v", checks.Items)
	}

	var list struct {
		Items []simulationListItem `json:"items"`
	}
	owner.json(http.MethodGet, "/api/projects/"+pid+"/simulations", nil, http.StatusOK, &list)
	if len(list.Items) != 1 || list.Items[0].StaleVsDraft {
		t.Fatalf("list %+v", list.Items)
	}
	var brief struct {
		Verdict     string            `json:"verdict"`
		VariantName string            `json:"variant_name"`
		KPI         map[string]any    `json:"kpi"`
		Fleet       []json.RawMessage `json:"fleet"`
	}
	if err := json.Unmarshal(list.Items[0].Brief, &brief); err != nil {
		t.Fatal(err)
	}
	if brief.Verdict != sim.VerdictFail || brief.VariantName != variant.Name || brief.KPI["throughput_per_h"] == nil || len(brief.Fleet) != 2 {
		t.Fatalf("brief %s", list.Items[0].Brief)
	}
	var calcs struct {
		Items []map[string]any `json:"items"`
	}
	owner.json(http.MethodGet, "/api/projects/"+pid+"/calculations", nil, http.StatusOK, &calcs)
	if len(calcs.Items) != 0 {
		t.Fatalf("simulation runs leaked into calculation history: %+v", calcs.Items)
	}
	owner.json(http.MethodGet, "/api/calculations/"+created.RunID, nil, http.StatusNotFound, nil)

	postTxs(t, owner, pid, newTx(opSet(ops.CollFleetItems, inbound, "quantity", 2)))
	owner.json(http.MethodGet, "/api/simulation-runs/"+created.RunID, nil, http.StatusOK, &run)
	if !run.StaleVsDraft {
		t.Fatal("changing the fleet must mark the simulation stale")
	}
	owner.json(http.MethodGet, "/api/projects/"+pid+"/sim-checks", nil, http.StatusOK, &checks)
	if len(checks.Items) != 1 || !checks.Items[0].Stale || checks.Items[0].RunID != created.RunID {
		t.Fatalf("editing the fleet of the variant must make its check stale: %+v", checks.Items)
	}
	clearMap(t, owner, pid)
	var second createSimulationJSON
	owner.json(http.MethodPost, "/api/projects/"+pid+"/simulations", map[string]any{"mode": "deterministic", "horizon_h": 1}, http.StatusAccepted, &second)
	if second.Preview.MapSource != simbuild.MapSourceTemplate || second.Preview.Replications != 1 || second.VersionNo != 2 {
		t.Fatalf("template preview %+v", second)
	}
	waitJob(t, owner, second.Job.ID, jobs.StatusSucceeded)
	var bad map[string]any
	owner.json(http.MethodPost, "/api/projects/"+pid+"/simulations", map[string]any{"mode": "stochastic", "replications": 99}, http.StatusBadRequest, &bad)
	owner.json(http.MethodPost, "/api/projects/"+pid+"/simulations", map[string]any{"policy": "lifo"}, http.StatusBadRequest, nil)
	owner.json(http.MethodPost, "/api/projects/"+pid+"/simulations", map[string]any{"unknown": 1}, http.StatusBadRequest, nil)

	owner.json(http.MethodDelete, "/api/projects/"+pid, nil, http.StatusOK, nil)
	var gone errorBody
	owner.json(http.MethodGet, "/api/simulation-runs/"+created.RunID, nil, http.StatusGone, &gone)
	if gone.Code != "project_deleted" {
		t.Fatalf("run of a trashed project: %+v", gone)
	}
}
