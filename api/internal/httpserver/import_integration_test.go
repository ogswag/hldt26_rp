package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"testing"

	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/ops"
)

// importBody is what the page sends from the browser store: the snapshot it has been editing without an account.
func importBody(name string, snap snapshotResp) map[string]any {
	return map[string]any{"name": name, "schema_version": snap.SchemaVersion, "collections": snap.Collections}
}

func TestImportProjectFromBrowser(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "import@example.com")
	p, _ := warehouseProject(t, owner, "Черновик в браузере")
	var template maps.Document
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/map/template", nil, http.StatusOK, &template)
	putMap(t, owner, p.ID, template)
	postTxs(t, owner, p.ID, newTx(opSet(ops.CollProject, ops.CollProject, "match_selected_ids", []string{stackerID})))

	var snap snapshotResp
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/snapshot", nil, http.StatusOK, &snap)

	var made projectResp
	owner.json(http.MethodPost, "/api/projects/import", importBody("Из браузера", snap), http.StatusCreated, &made)
	if made.ID == p.ID || made.Name != "Из браузера" {
		t.Fatalf("imported project %+v", made)
	}

	var copied snapshotResp
	owner.json(http.MethodGet, "/api/projects/"+made.ID+"/snapshot", nil, http.StatusOK, &copied)
	schema := ops.ProjectSchema()
	// Identifiers are new, so the two snapshots are compared after both are keyed the same way: the content,
	// the order and every reference between records must survive the import.
	want, got := shapeOf(t, schema, snap.state(t)), shapeOf(t, schema, copied.state(t))
	want[ops.CollProject][0]["name"] = "Из браузера"
	for _, coll := range schema.Names() {
		if !reflect.DeepEqual(want[coll], got[coll]) {
			t.Fatalf("%s differs after import:\nwant %s\ngot  %s", coll, mustJSON(want[coll]), mustJSON(got[coll]))
		}
	}

	// The import is one entry in the journal of the new project, and nothing before it.
	var page opsPageJSON
	owner.json(http.MethodGet, "/api/projects/"+made.ID+"/operations?after=0", nil, http.StatusOK, &page)
	if len(page.Items) != 1 || page.Items[0].Label != importLabel || page.Items[0].ClientID != importClientID {
		t.Fatalf("journal %+v", page.Items)
	}
}

func TestImportProjectRefusesBadSnapshot(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "badimport@example.com")
	p, _ := warehouseProject(t, owner, "Черновик")
	var snap snapshotResp
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/snapshot", nil, http.StatusOK, &snap)

	before := projectCount(t, owner)

	// A fleet item of a variant that is not in the snapshot cannot be written, so nothing is.
	orphan := snap
	orphan.Collections = cloneCollections(snap.Collections)
	for id := range orphan.Collections[ops.CollVariants] {
		delete(orphan.Collections[ops.CollVariants], id)
	}
	var bad struct {
		Code    string       `json:"code"`
		Details *ops.Details `json:"details"`
	}
	owner.json(http.MethodPost, "/api/projects/import", importBody("Сломанный", orphan), http.StatusBadRequest, &bad)
	if bad.Code != ops.ReasonRefMissing || bad.Details == nil || bad.Details.Field != "variant_id" {
		t.Fatalf("refusal %+v %+v", bad.Code, bad.Details)
	}

	// An unknown object type never reaches the schema.
	wrongType := snap
	wrongType.Collections = cloneCollections(snap.Collections)
	rec := map[string]any{}
	if err := json.Unmarshal(wrongType.Collections[ops.CollProject][ops.CollProject], &rec); err != nil {
		t.Fatal(err)
	}
	rec["object_type"] = "factory"
	wrongType.Collections[ops.CollProject][ops.CollProject] = mustJSON(rec)
	owner.json(http.MethodPost, "/api/projects/import", importBody("Завод", wrongType), http.StatusBadRequest, nil)

	owner.json(http.MethodPost, "/api/projects/import",
		map[string]any{"name": "Старая схема", "schema_version": snap.SchemaVersion + 1, "collections": snap.Collections},
		http.StatusConflict, nil)

	if after := projectCount(t, owner); after != before {
		t.Fatalf("a refused import left %d projects, had %d", after, before)
	}

	guest := &itClient{t: t, h: env.h}
	guest.json(http.MethodPost, "/api/projects/import", importBody("Гость", snap), http.StatusForbidden, nil)
}

// shapeOf lists a state's records in their own order with every UUID replaced by the position of the record it
// names, so two states that differ only in identifiers compare equal.
func shapeOf(t *testing.T, schema *ops.Schema, st ops.State) map[string][]map[string]any {
	t.Helper()
	place := map[string]string{}
	for _, coll := range schema.Names() {
		c := schema.Collections[coll]
		list := st.Sorted(coll)
		if c.Ordered {
			for i, rec := range list {
				place[rec.String(ops.FieldID)] = fmt.Sprintf("%s#%d", coll, i)
			}
			continue
		}
		// Records kept by id alone come back in another order, so they are named after what they hold.
		ranks := map[string]int{}
		sigs := []string{}
		for _, rec := range list {
			sig := signature(c, rec)
			if _, seen := ranks[sig]; !seen {
				ranks[sig] = 0
				sigs = append(sigs, sig)
			}
		}
		sort.Strings(sigs)
		for i, sig := range sigs {
			ranks[sig] = i
		}
		for _, rec := range list {
			place[rec.String(ops.FieldID)] = fmt.Sprintf("%s#%d", coll, ranks[signature(c, rec)])
		}
	}
	out := map[string][]map[string]any{}
	for _, coll := range schema.Names() {
		list := []map[string]any{}
		for _, rec := range st.Sorted(coll) {
			item := map[string]any{}
			for k, v := range rec {
				item[k] = maskIDs(place, v)
			}
			list = append(list, item)
		}
		// A collection without an order key has none: its records are kept by their id, which the import
		// renews, so they are compared as a set.
		if !schema.Collections[coll].Ordered {
			sort.Slice(list, func(i, j int) bool { return string(mustJSON(list[i])) < string(mustJSON(list[j])) })
		}
		out[coll] = list
	}
	return out
}

// signature is what a record holds apart from identifiers: its fields with the id and every reference dropped.
func signature(c *ops.Collection, rec ops.Record) string {
	out := map[string]any{}
	for k, v := range rec {
		if k == ops.FieldID {
			continue
		}
		if f := c.Fields[k]; f != nil && (f.Type == ops.TypeRef || f.Type == ops.TypeRefList) {
			continue
		}
		out[k] = v
	}
	return string(mustJSON(out))
}

func maskIDs(place map[string]string, v any) any {
	switch t := v.(type) {
	case string:
		if at, ok := place[t]; ok {
			return at
		}
		return t
	case []any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			out = append(out, maskIDs(place, item))
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, item := range t {
			out[k] = maskIDs(place, item)
		}
		return out
	}
	return v
}

func cloneCollections(in map[string]map[string]json.RawMessage) map[string]map[string]json.RawMessage {
	out := map[string]map[string]json.RawMessage{}
	for coll, recs := range in {
		out[coll] = map[string]json.RawMessage{}
		for id, raw := range recs {
			out[coll][id] = raw
		}
	}
	return out
}

func projectCount(t *testing.T, c *itClient) int {
	t.Helper()
	var list struct {
		Items []json.RawMessage `json:"items"`
	}
	c.json(http.MethodGet, "/api/projects", nil, http.StatusOK, &list)
	return len(list.Items)
}
