package ops

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/projects"
)

func sampleDraft(t *testing.T) projects.Draft {
	t.Helper()
	procs := projects.DefaultProcesses("warehouse")
	for i := range procs {
		procs[i].ID = uuid.NewString()
		procs[i].PointIDs = []string{}
	}
	procs[0].PointIDs = []string{"D1"}
	vars := projects.DefaultVariants()
	for i := range vars {
		vars[i].ID = uuid.NewString()
		for j := range vars[i].Financing {
			vars[i].Financing[j].ID = uuid.NewString()
		}
	}
	price := 2_900_000.0
	sol := uuid.NewString()
	vars[1].Fleet = []projects.FleetItem{
		{ID: uuid.NewString(), SolutionID: &sol, Quantity: 2, TaskCodes: []string{"inbound"}, PriceOverrideRub: &price, PriceOverrideReason: "КП", SortOrder: 0},
		{ID: uuid.NewString(), Quantity: 1, TaskCodes: []string{}, SortOrder: 1},
	}
	sets := projects.DefaultAssumptionSets()
	sets[0].ID = uuid.NewString()
	d := projects.NewDraft("warehouse", json.RawMessage(`{"area_m2":5000,"power_kw":null}`), procs, vars, []string{sol})
	d.AssumptionSets = sets
	d.ActiveAssumptionSetID = sets[0].ID
	d.SharedCosts = []projects.SharedCost{{ID: uuid.NewString(), Code: "infra", Label: "Инфраструктура", Bucket: "capex", Rub: 1.5, SortOrder: 0}}
	d.EconOverrides = json.RawMessage(`{"labor_rate_rub_h":450}`)
	doc := maps.WarehouseTemplate([]string{"inbound", "putaway", "piece_pick", "outbound"}, maps.TemplateWidths{})
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	d.Map = raw
	return d
}

func TestDraftRoundTrip(t *testing.T) {
	d := sampleDraft(t)
	st, err := FromDraft("Склад", d)
	if err != nil {
		t.Fatal(err)
	}
	back, name, err := ToDraft(st)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Склад" {
		t.Fatalf("name %q", name)
	}
	clearOrder(&back)
	if !reflect.DeepEqual(back.Processes, d.Processes) {
		t.Fatalf("processes\n%+v\n%+v", back.Processes, d.Processes)
	}
	if !reflect.DeepEqual(back.Variants, d.Variants) {
		t.Fatalf("variants\n%+v\n%+v", back.Variants, d.Variants)
	}
	if !reflect.DeepEqual(back.SharedCosts, d.SharedCosts) || !reflect.DeepEqual(back.AssumptionSets, d.AssumptionSets) {
		t.Fatalf("costs or sets\n%+v %+v", back.SharedCosts, back.AssumptionSets)
	}
	if back.ActiveAssumptionSetID != d.ActiveAssumptionSetID || !reflect.DeepEqual(back.MatchSelectedIDs, d.MatchSelectedIDs) {
		t.Fatalf("active %s selected %v", back.ActiveAssumptionSetID, back.MatchSelectedIDs)
	}
	for _, pair := range [][2]json.RawMessage{{back.Params, d.Params}, {back.EconOverrides, d.EconOverrides}} {
		var a, b any
		_ = json.Unmarshal(pair[0], &a)
		_ = json.Unmarshal(pair[1], &b)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("json %s != %s", pair[0], pair[1])
		}
	}
	if got, want := normalizedMap(t, back.Map), normalizedMap(t, d.Map); !reflect.DeepEqual(got, want) {
		t.Fatalf("map changed\n%+v\n%+v", got.Layers, want.Layers)
	}

	again, err := FromDraft(name, mustDraft(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, st) {
		t.Fatal("state is not stable across a second round trip")
	}
}

func mustDraft(t *testing.T, st State) projects.Draft {
	t.Helper()
	d, _, err := ToDraft(st)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDraftWithoutMap(t *testing.T) {
	d := sampleDraft(t)
	d.Map = nil
	st, err := FromDraft("x", d)
	if err != nil {
		t.Fatal(err)
	}
	if st.Get(CollMap, CollMap)["page"] != nil {
		t.Fatal("a project without a map must have map.page null")
	}
	back := mustDraft(t, st)
	if back.Map != nil {
		t.Fatalf("map %s", back.Map)
	}
}

func TestDraftKeepsValidOrderKeys(t *testing.T) {
	d := sampleDraft(t)
	d.Variants[0].Order, d.Variants[1].Order, d.Variants[2].Order = "a0", "a0V", "a1"
	st, err := FromDraft("x", d)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Get(CollVariants, d.Variants[1].ID).String(FieldOrder); got != "a0V" {
		t.Fatalf("stored key replaced: %s", got)
	}
	d.Variants[2].Order = "a0"
	st, _ = FromDraft("x", d)
	back := mustDraft(t, st)
	for i, v := range back.Variants {
		if v.ID != d.Variants[i].ID || v.SortOrder != i {
			t.Fatalf("conflicting keys must fall back to sort_order: %+v", back.Variants)
		}
	}
}

func clearOrder(d *projects.Draft) {
	for i := range d.Processes {
		d.Processes[i].Order = ""
	}
	for i := range d.Variants {
		d.Variants[i].Order = ""
		for j := range d.Variants[i].Fleet {
			d.Variants[i].Fleet[j].Order = ""
		}
	}
	for i := range d.SharedCosts {
		d.SharedCosts[i].Order = ""
	}
	for i := range d.AssumptionSets {
		d.AssumptionSets[i].Order = ""
	}
}

// normalizedMap sorts unordered layers and drops flow ids, which the server adds on write.
func normalizedMap(t *testing.T, raw json.RawMessage) maps.Document {
	t.Helper()
	doc, err := maps.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	l := &doc.Layers
	sort.Slice(l.Points, func(i, j int) bool { return l.Points[i].ID < l.Points[j].ID })
	sort.Slice(l.Edges, func(i, j int) bool { return l.Edges[i].ID < l.Edges[j].ID })
	sort.Slice(l.Resources, func(i, j int) bool { return l.Resources[i].ID < l.Resources[j].ID })
	sort.Slice(l.Flows, func(i, j int) bool { return l.Flows[i].ProcessCode < l.Flows[j].ProcessCode })
	for i := range l.Flows {
		l.Flows[i].ID = ""
	}
	for i := range l.Resources {
		if len(l.Resources[i].PointIDs) == 0 {
			l.Resources[i].PointIDs = nil
		}
		if len(l.Resources[i].EdgeIDs) == 0 {
			l.Resources[i].EdgeIDs = nil
		}
	}
	return doc
}

const mapFixture = "../../../contracts/ops/map_document.json"

// TestMapFixture keeps contracts/ops/map_document.json: map records and the document toMap builds from them.
// web/src/map/records.test.ts builds the same document from the same records.
func TestMapFixture(t *testing.T) {
	doc := maps.WarehouseTemplate([]string{"inbound", "putaway", "piece_pick", "outbound"}, maps.TemplateWidths{})
	doc.Calibration.Segment = &maps.Segment{X1: 10, Y1: 10, X2: 110.5, Y2: 10, LengthM: 10.05}
	doc.Calibration.Check = &maps.Segment{X1: 0, Y1: 0, X2: 0, Y2: 50, LengthM: 5}
	no := false
	doc.Layers.Points = append(doc.Layers.Points, maps.PointFeature{ID: "p-extra", Kind: "task", Name: "Отбор 2", X: 12.25, Y: 7, ProcessCode: "piece_pick"})
	doc.Layers.Edges = append(doc.Layers.Edges, maps.Edge{ID: "e-extra", From: "p-extra", To: doc.Layers.Points[0].ID, Bidirectional: &no})
	doc.Layers.Resources = append(doc.Layers.Resources, maps.Resource{ID: "r-extra", Kind: "narrow_aisle", Name: "Узкий", Capacity: 1, EdgeIDs: []string{"e-extra"}})
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	d := sampleDraft(t)
	d.Map = raw
	st, err := FromDraft("Склад", d)
	if err != nil {
		t.Fatal(err)
	}
	built, err := toMap(st)
	if err != nil {
		t.Fatal(err)
	}
	records := map[string]any{}
	for _, c := range []string{CollMap, CollMapPoints, CollMapEdges, CollMapZones, CollMapObstacles, CollMapResources, CollMapFlows} {
		records[c] = st[c]
	}
	got, err := json.MarshalIndent(map[string]any{"records": records, "document": json.RawMessage(built)}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.WriteFile(mapFixture, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	stored, err := os.ReadFile(mapFixture)
	if err != nil || !bytes.Equal(stored, got) {
		t.Fatalf("%s is stale (run go test ./internal/ops -run TestMapFixture -update): %v", mapFixture, err)
	}
}
