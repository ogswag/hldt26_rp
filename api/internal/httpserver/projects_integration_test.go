package httpserver

import (
	"encoding/json"
	"net/http"
	"testing"

	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
)

func typeDefaults(t *testing.T, c *itClient, objectType string) map[string]any {
	t.Helper()
	var schema struct {
		Groups []struct {
			Fields []struct {
				ID      string `json:"id"`
				Default any    `json:"default"`
			} `json:"fields"`
		} `json:"groups"`
	}
	c.json(http.MethodGet, "/api/object-types/"+objectType+"/schema", nil, http.StatusOK, &schema)
	out := map[string]any{}
	for _, g := range schema.Groups {
		for _, f := range g.Fields {
			out[f.ID] = f.Default
		}
	}
	return out
}

type typedProjectResp struct {
	Name         string             `json:"name"`
	ObjectType   string             `json:"object_type"`
	Processes    []projects.Process `json:"processes"`
	Variants     []projects.Variant `json:"variants"`
	Map          json.RawMessage    `json:"map"`
	Results      json.RawMessage    `json:"results"`
	CurrentRunID *string            `json:"current_run_id"`
	Stale        bool               `json:"stale"`
}

func TestChangeObjectTypeResetsTypeData(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "type@example.com")
	p, _ := warehouseProject(t, owner, "Смена типа")
	var template maps.Document
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/map/template", nil, http.StatusOK, &template)
	putMap(t, owner, p.ID, template)
	var calc calculateResponse
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/calculations", nil, http.StatusOK, &calc)

	airport := typeDefaults(t, owner, "airport")

	// The type alone, with the parameters of the old one, is refused: the whole transaction writes nothing.
	out := postTxs(t, owner, p.ID, newTx(opSet(ops.CollProject, ops.CollProject, "object_type", "airport")))
	if out.Results[0].Status != "rejected" {
		t.Fatalf("warehouse params for an airport: %+v", out.Results[0])
	}
	out = postTxs(t, owner, p.ID, newTx(
		opSet(ops.CollProject, ops.CollProject, "object_type", "factory"),
		opSet(ops.CollProject, ops.CollProject, "params", airport),
	))
	if out.Results[0].Status != "rejected" {
		t.Fatalf("unknown object type accepted: %+v", out.Results[0])
	}
	var same typedProjectResp
	owner.json(http.MethodGet, "/api/projects/"+p.ID, nil, http.StatusOK, &same)
	if same.ObjectType != "warehouse" || same.CurrentRunID == nil || len(same.Map) == 0 || string(same.Map) == "null" {
		t.Fatalf("rejected type change must keep the project: %+v", same)
	}

	// The macro the object form sends: the type, its parameters, and everything chosen for the old type.
	var snap snapshotResp
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/snapshot", nil, http.StatusOK, &snap)
	st := snap.state(t)
	list := []ops.Op{
		opSet(ops.CollProject, ops.CollProject, "object_type", "airport"),
		opSet(ops.CollProject, ops.CollProject, "params", airport),
		opSet(ops.CollProject, ops.CollProject, "match_selected_ids", []string{}),
	}
	for _, coll := range []string{ops.CollMapFlows, ops.CollMapResources, ops.CollMapEdges, ops.CollMapZones,
		ops.CollMapObstacles, ops.CollMapPoints, ops.CollFleetItems, ops.CollProcesses} {
		for _, id := range st.IDs(coll) {
			list = append(list, ops.Op{Op: ops.OpDelete, Coll: coll, ID: id})
		}
	}
	// The map exists while its page is set, so emptying the layers is not enough to remove it.
	list = append(list, opSet(ops.CollMap, ops.CollMap, "page", nil))
	postTxs(t, owner, p.ID, newTx(list...))

	var after typedProjectResp
	owner.json(http.MethodGet, "/api/projects/"+p.ID, nil, http.StatusOK, &after)
	if after.ObjectType != "airport" || after.Name != "Смена типа" {
		t.Fatalf("type %q, name %q", after.ObjectType, after.Name)
	}
	// The old run answers a question about a warehouse, so the project detaches it and keeps no result of it.
	// Nothing is current, so nothing is stale either.
	if after.CurrentRunID != nil || (len(after.Results) > 0 && string(after.Results) != "null") || after.Stale {
		t.Fatalf("the run of the old type must stop being current: run %v results %s stale %v",
			after.CurrentRunID, after.Results, after.Stale)
	}
	if len(after.Map) > 0 && string(after.Map) != "null" {
		t.Fatalf("map of the old type must be removed: %s", after.Map)
	}
	// The airport has no default processes, and nothing is seeded back over an emptied project.
	if len(after.Processes) != len(projects.DefaultProcesses("airport")) {
		t.Fatalf("processes: %d, want airport defaults %d", len(after.Processes), len(projects.DefaultProcesses("airport")))
	}
	for _, v := range after.Variants {
		if len(v.Fleet) != 0 {
			t.Fatalf("variant %s keeps a fleet of the old type", v.Name)
		}
	}

	var runs runsResp
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/runs", nil, http.StatusOK, &runs)
	if len(runs.Items) == 0 {
		t.Fatal("warehouse runs must stay in history")
	}
	for _, r := range runs.Items {
		if r.IsCurrent {
			t.Fatalf("run %s is still current after the type change", r.ID)
		}
	}
}

func TestCreateProjectNeedsName(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "noname@example.com")
	for _, name := range []string{"", "   "} {
		owner.json(http.MethodPost, "/api/projects", map[string]any{"name": name, "object_type": "warehouse"}, http.StatusBadRequest, nil)
	}
	var p projectResp
	owner.json(http.MethodPost, "/api/projects", map[string]any{"name": "  Склад  ", "object_type": "warehouse"}, http.StatusCreated, &p)
	if p.Name != "Склад" {
		t.Fatalf("name %q, want trimmed", p.Name)
	}
}
