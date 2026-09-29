package httpserver

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"moscow_hackathon_2026/api/internal/ops"
)

func guestBody(snap snapshotResp, extra map[string]any) map[string]any {
	body := map[string]any{"object_type": "warehouse", "schema_version": snap.SchemaVersion, "collections": snap.Collections}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

// withoutRunFields drops what only a stored calculation has.
func withoutRunFields(m map[string]any) map[string]any {
	for _, k := range []string{"run_id", "input_hash", "project_id", "project_name", "stale_vs_draft", "run"} {
		delete(m, k)
	}
	return m
}

func TestGuestCalculationTakesTheProjectFromTheBrowser(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "guestdraft@example.com")
	p, _ := warehouseProject(t, owner, "Проект в браузере")
	var snap snapshotResp
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/snapshot", nil, http.StatusOK, &snap)

	var stored map[string]any
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/calculations", map[string]any{"seed": 11}, http.StatusOK, &stored)

	guest := &itClient{t: t, h: env.h}
	tables := []string{"projects", "project_drafts", "project_versions", "project_processes", "solution_variants", "variant_fleet_items",
		"calculation_runs", "calculation_results", "simulation_jobs", "assumption_sets", "shared_cost_items"}
	count := func() map[string]int64 {
		out := map[string]int64{}
		for _, tbl := range tables {
			var n int64
			if err := env.db.QueryRow(context.Background(), "SELECT count(*) FROM "+tbl).Scan(&n); err != nil {
				t.Fatalf("count %s: %v", tbl, err)
			}
			out[tbl] = n
		}
		return out
	}
	before := count()
	var got map[string]any
	guest.json(http.MethodPost, "/api/guest/calculate", guestBody(snap, map[string]any{"seed": 11}), http.StatusOK, &got)
	if !reflect.DeepEqual(withoutRunFields(stored), withoutRunFields(got)) {
		t.Fatalf("a project sent from the browser must calculate as the saved one\nsaved %s\nguest %s", mustJSON(stored), mustJSON(got))
	}
	if variants, _ := got["variants"].([]any); len(variants) < 2 {
		t.Fatalf("the variants of the project are missing from the guest result: %v", got["variants"])
	}

	for _, format := range []string{"pdf", "xlsx"} {
		rec := guest.do(http.MethodPost, "/api/guest/export?format="+format, guestBody(snap, nil), false)
		if rec.Code != http.StatusOK || rec.Header().Get("X-Run-Id") != "" {
			t.Fatalf("guest export %s: %d %s", format, rec.Code, rec.Body.String())
		}
	}
	after := count()
	for _, tbl := range tables {
		if before[tbl] != after[tbl] {
			t.Errorf("a guest calculation changed %s: %d -> %d", tbl, before[tbl], after[tbl])
		}
	}
}

func TestGuestCalculationRefusesABadProject(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "guestbad@example.com")
	p, _ := warehouseProject(t, owner, "Проект")
	var snap snapshotResp
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/snapshot", nil, http.StatusOK, &snap)
	guest := &itClient{t: t, h: env.h}

	for _, path := range []string{"/api/guest/calculate", "/api/guest/export?format=xlsx"} {
		t.Run(path, func(t *testing.T) {
			newer := guestBody(snap, map[string]any{"schema_version": snap.SchemaVersion + 1})
			guest.json(http.MethodPost, path, newer, http.StatusConflict, nil)

			wrongType := guestBody(snap, map[string]any{"object_type": "airport"})
			guest.json(http.MethodPost, path, wrongType, http.StatusBadRequest, nil)

			orphan := cloneCollections(snap.Collections)
			for id := range orphan[ops.CollVariants] {
				delete(orphan[ops.CollVariants], id)
			}
			var refused struct {
				Code string `json:"code"`
			}
			guest.json(http.MethodPost, path, guestBody(snap, map[string]any{"collections": orphan}), http.StatusBadRequest, &refused)
			if refused.Code != ops.ReasonRefMissing {
				t.Fatalf("code %q", refused.Code)
			}

			noProject := cloneCollections(snap.Collections)
			delete(noProject[ops.CollProject], ops.CollProject)
			guest.json(http.MethodPost, path, guestBody(snap, map[string]any{"collections": noProject}), http.StatusBadRequest, nil)
		})
	}
}
