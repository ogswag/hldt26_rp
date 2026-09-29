package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
)

var accessDeniedTexts = map[string]bool{"Проект не найден.": true, "Запуск не найден.": true, "Задание не найдено.": true}

// accessOutcome classifies a response as passed, forbidden by role, or hidden by the access check.
func accessOutcome(code int, body []byte) string {
	var e errorBody
	_ = json.Unmarshal(body, &e)
	switch {
	case code == http.StatusForbidden && e.Code == "forbidden":
		return "forbidden"
	case code == http.StatusForbidden:
		return "guest"
	case code == http.StatusNotFound && accessDeniedTexts[e.Error]:
		return "hidden"
	}
	return "passed"
}

func addMember(t *testing.T, owner *itClient, pid, email, role string) {
	t.Helper()
	owner.json(http.MethodPost, "/api/projects/"+pid+"/members", map[string]any{"email": email, "role": role}, http.StatusCreated, nil)
}

func TestAccessMatrix(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "m-owner@example.com")
	editor := register(t, env.h, "m-editor@example.com")
	viewer := register(t, env.h, "m-viewer@example.com")
	other := register(t, env.h, "m-other@example.com")
	guest := &itClient{t: t, h: env.h}
	p, params := warehouseProject(t, owner, "Матрица доступа")
	pid := p.ID
	addMember(t, owner, pid, "m-editor@example.com", RoleEditor)
	addMember(t, owner, pid, "m-viewer@example.com", RoleViewer)

	var procs struct {
		Items []projects.Process `json:"items"`
	}
	owner.json(http.MethodGet, "/api/projects/"+pid+"/processes", nil, http.StatusOK, &procs)
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
	var sim createSimulationJSON
	owner.json(http.MethodPost, "/api/projects/"+pid+"/simulations", map[string]any{"mode": "deterministic", "horizon_h": 1}, http.StatusAccepted, &sim)

	params["pick_lines_per_day"] = 90000.0
	bodies := map[string]any{
		"PUT /api/projects/{id}":                            map[string]any{"params": params},
		"PUT /api/projects/{id}/processes":                  map[string]any{"items": procs.Items},
		"POST /api/projects/{id}/processes":                 procs.Items[0],
		"PUT /api/projects/{id}/processes/{processId}":      procs.Items[0],
		"POST /api/projects/{id}/variants":                  map[string]any{"name": "Новый"},
		"PUT /api/projects/{id}/variants/{variantId}":       p.Variants[0],
		"PUT /api/projects/{id}/shared-costs":               map[string]any{"items": []any{}},
		"PUT /api/projects/{id}/assumption-sets":            map[string]any{"items": []any{map[string]any{"name": "x", "vat_rate": 0.2, "labor_cash_share": 1}}},
		"PUT /api/projects/{id}/map":                        template,
		"POST /api/projects/{id}/map/validate":              template,
		"POST /api/projects/{id}/simulations":               map[string]any{"mode": "deterministic", "horizon_h": 1},
		"PUT /api/simulation-runs/{runId}/pin":              map[string]any{"pinned": true},
		"POST /api/projects/{id}/members":                   map[string]any{"email": "nobody@example.com", "role": RoleViewer},
		"PUT /api/projects/{id}/members/{userId}":           map[string]any{"role": RoleViewer},
		"POST /api/calculations/{runId}/export":             map[string]any{},
		"POST /api/simulation-runs/{runId}/export":          map[string]any{},
		"POST /api/projects/{id}/export":                    map[string]any{},
		"POST /api/projects/{id}/copy":                      map[string]any{},
		"POST /api/projects/{id}/calculate":                 map[string]any{},
		"POST /api/projects/{id}/calculations":              map[string]any{},
		"POST /api/projects/{id}/match":                     map[string]any{},
		"POST /api/projects/{id}/variants/{variantId}/copy": map[string]any{},
	}
	path := func(pattern string) string {
		runID := calc.RunID
		if strings.HasPrefix(pattern, "/api/simulation-runs/") {
			runID = sim.RunID
		}
		repl := strings.NewReplacer(
			"{id}", pid, "{runId}", runID, "{jobId}", sim.Job.ID,
			"{processId}", procs.Items[0].ID, "{variantId}", p.Variants[0].ID,
			"{versionId}", versions.Items[0].ID, "{userId}", uuid.NewString(),
		)
		out := repl.Replace(pattern)
		if strings.HasSuffix(pattern, "/export") {
			out += "?format=pdf"
		}
		return out
	}

	keys := make([]string, 0, len(env.srv.scopedRoutes))
	for k := range env.srv.scopedRoutes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		// The project delete runs last: every other route needs the project.
		if (keys[i] == "DELETE /api/projects/{id}") != (keys[j] == "DELETE /api/projects/{id}") {
			return keys[j] == "DELETE /api/projects/{id}"
		}
		return keys[i] < keys[j]
	})
	clients := []struct {
		role string
		c    *itClient
	}{{"guest", guest}, {"other", other}, {RoleViewer, viewer}, {RoleEditor, editor}, {RoleOwner, owner}}
	for _, key := range keys {
		need := env.srv.scopedRoutes[key]
		method, pattern, _ := strings.Cut(key, " ")
		for _, cl := range clients {
			var want string
			switch cl.role {
			case "guest":
				want = "guest"
			case "other":
				want = "hidden"
			default:
				want = "passed"
				if !roleAllows(cl.role, need) {
					want = "forbidden"
				}
			}
			if key == "DELETE /api/projects/{id}/members/{userId}" && cl.role != RoleOwner && want == "passed" {
				want = "forbidden"
			}
			// The trash routes need a project in the trash; against a live one they answer as if it were gone.
			if (strings.HasSuffix(pattern, "/restore") || strings.HasSuffix(pattern, "/purge")) && cl.role != "guest" {
				want = "hidden"
			}
			ctx, cancel := context.WithCancel(context.Background())
			if strings.HasSuffix(pattern, "/events") {
				// A stream never ends by itself; its status is what the matrix checks.
				ctx, cancel = context.WithTimeout(context.Background(), 200*time.Millisecond)
			}
			rec := cl.c.doCtx(ctx, method, path(pattern), bodies[key], false)
			cancel()
			if got := accessOutcome(rec.Code, rec.Body.Bytes()); got != want {
				t.Errorf("%s as %s: %s (status %d), want %s: %s", key, cl.role, got, rec.Code, want, rec.Body.String())
			}
		}
	}
}

func TestMembers(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "own@example.com")
	colleague := register(t, env.h, "colleague@example.com")
	p, _ := warehouseProject(t, owner, "Общий склад")
	pid := p.ID

	var invited struct {
		Status     string         `json:"status"`
		Invitation invitationJSON `json:"invitation"`
	}
	owner.json(http.MethodPost, "/api/projects/"+pid+"/members", map[string]any{"email": "ghost@example.com", "role": RoleEditor}, http.StatusAccepted, &invited)
	if invited.Status != "invited" || invited.Invitation.Email != "ghost@example.com" {
		t.Fatalf("unknown email: %+v", invited)
	}
	owner.json(http.MethodPost, "/api/projects/"+pid+"/members", map[string]any{"email": "colleague@example.com", "role": "admin"}, http.StatusBadRequest, nil)
	owner.json(http.MethodPost, "/api/projects/"+pid+"/members", map[string]any{"email": "own@example.com", "role": RoleEditor}, http.StatusBadRequest, nil)
	var added memberJSON
	owner.json(http.MethodPost, "/api/projects/"+pid+"/members", map[string]any{"email": " Colleague@Example.com ", "role": RoleViewer}, http.StatusCreated, &added)
	owner.json(http.MethodPost, "/api/projects/"+pid+"/members", map[string]any{"email": "colleague@example.com", "role": RoleEditor}, http.StatusConflict, nil)

	var list struct {
		Items []memberJSON `json:"items"`
	}
	colleague.json(http.MethodGet, "/api/projects/"+pid+"/members", nil, http.StatusForbidden, nil)
	owner.json(http.MethodGet, "/api/projects/"+pid+"/members", nil, http.StatusOK, &list)
	if len(list.Items) != 2 || list.Items[0].Role != RoleOwner || list.Items[1].Email != "colleague@example.com" || list.Items[1].Role != RoleViewer {
		t.Fatalf("members %+v", list.Items)
	}

	var got struct {
		Access string `json:"access"`
		Name   string `json:"name"`
	}
	colleague.json(http.MethodGet, "/api/projects/"+pid, nil, http.StatusOK, &got)
	if got.Access != RoleViewer {
		t.Fatalf("viewer access %q", got.Access)
	}
	rename := map[string]any{
		"client_id": "colleague", "schema_version": 1,
		"txs": []testTx{newTx(opSet(ops.CollVariants, p.Variants[0].ID, "name", "Переименовал коллега"))},
	}
	colleague.json(http.MethodPost, "/api/projects/"+pid+"/transactions", rename, http.StatusForbidden, nil)

	owner.json(http.MethodPut, "/api/projects/"+pid+"/members/"+added.UserID, map[string]any{"role": RoleEditor}, http.StatusNoContent, nil)
	colleague.json(http.MethodPost, "/api/projects/"+pid+"/transactions", rename, http.StatusOK, nil)
	var listed struct {
		Items []projectListItem `json:"items"`
	}
	colleague.json(http.MethodGet, "/api/projects", nil, http.StatusOK, &listed)
	if len(listed.Items) != 1 || listed.Items[0].ID != pid || listed.Items[0].Access != RoleEditor {
		t.Fatalf("shared project list %+v", listed.Items)
	}

	var copied struct {
		ID     string `json:"id"`
		Access string `json:"access"`
	}
	colleague.json(http.MethodPost, "/api/projects/"+pid+"/copy", map[string]any{}, http.StatusCreated, &copied)
	if copied.Access != RoleOwner {
		t.Fatalf("copy access %q", copied.Access)
	}
	owner.json(http.MethodGet, "/api/projects/"+copied.ID, nil, http.StatusNotFound, nil)

	colleague.json(http.MethodDelete, "/api/projects/"+pid+"/members/"+added.UserID, nil, http.StatusNoContent, nil)
	colleague.json(http.MethodGet, "/api/projects/"+pid, nil, http.StatusNotFound, nil)
	owner.json(http.MethodDelete, "/api/projects/"+pid+"/members/"+added.UserID, nil, http.StatusNotFound, nil)
}
