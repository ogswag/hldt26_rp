package projects

import (
	"encoding/json"
	"testing"
)

func TestInputHashIgnoresKeyOrderAndIDs(t *testing.T) {
	p1 := json.RawMessage(`{"b":2,"a":1}`)
	p2 := json.RawMessage(`{"a":1,"b":2}`)
	d1 := NewDraft("warehouse", p1, DefaultProcesses("warehouse"), DefaultVariants(), []string{"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"})
	d1.Processes[0].ID = "11111111-1111-4111-8111-111111111111"
	d2 := NewDraft("warehouse", p2, DefaultProcesses("warehouse"), DefaultVariants(), []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"})
	h1, err := InputHash(d1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := InputHash(d2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("hash mismatch\n%s\n%s", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("hash len %d", len(h1))
	}
}

func TestInputHashChangesOnProcessAndParams(t *testing.T) {
	base := NewDraft("warehouse", json.RawMessage(`{"aisle_working_m":2.8}`), DefaultProcesses("warehouse"), DefaultVariants(), nil)
	h0, err := InputHash(base)
	if err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.Params = json.RawMessage(`{"aisle_working_m":3.0}`)
	h1, err := InputHash(changed)
	if err != nil {
		t.Fatal(err)
	}
	if h0 == h1 {
		t.Fatal("params change must change hash")
	}
	proc := base
	proc.Processes = append([]Process(nil), base.Processes...)
	proc.Processes[0].Demand.UnitsPerDay = 2000
	h2, err := InputHash(proc)
	if err != nil {
		t.Fatal(err)
	}
	if h0 == h2 {
		t.Fatal("process change must change hash")
	}
	v := base
	v.Variants = append([]Variant(nil), base.Variants...)
	qty := 4
	sid := "5760e938-9a43-45a7-b8e8-f4f2e6383930"
	v.Variants[0].Fleet = []FleetItem{{SolutionID: &sid, Quantity: qty}}
	h3, err := InputHash(v)
	if err != nil {
		t.Fatal(err)
	}
	if h0 == h3 {
		t.Fatal("fleet change must change hash")
	}
}

func TestInputHashChangesOnAssumptionSet(t *testing.T) {
	base := NewDraft("warehouse", json.RawMessage(`{"aisle_working_m":2.8}`), DefaultProcesses("warehouse"), DefaultVariants(), nil)
	h0, err := InputHash(base)
	if err != nil {
		t.Fatal(err)
	}
	next := base
	next.AssumptionSets = append([]AssumptionSet(nil), base.AssumptionSets...)
	if len(next.AssumptionSets) == 0 {
		next.AssumptionSets = DefaultAssumptionSets()
	}
	next.AssumptionSets[0].LaborCashShare = 0.5
	h1, err := InputHash(next)
	if err != nil {
		t.Fatal(err)
	}
	if h0 == h1 {
		t.Fatal("assumption change must change hash")
	}
}

func TestInputHashDoesNotMutateDraft(t *testing.T) {
	sid := "5760e938-9a43-45a7-b8e8-f4f2e6383930"
	tariff := "fixed"
	d := NewDraft("warehouse", json.RawMessage(`{"a":1}`), DefaultProcesses("warehouse"), []Variant{{
		ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Name: "AMR", Status: "ready",
		Fleet:     []FleetItem{{ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", SolutionID: &sid, Quantity: 2, TaskCodes: []string{"outbound", "inbound"}}},
		Financing: []Financing{{ID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Kind: "raas", Tariff: &tariff}},
	}}, nil)
	d.AssumptionSets[0].ID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	d.ActiveAssumptionSetID = d.AssumptionSets[0].ID
	before, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InputHash(d); err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("hash mutated draft\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestInputHashIgnoresDatabaseIDsAndUnorderedChildren(t *testing.T) {
	sid := "5760e938-9a43-45a7-b8e8-f4f2e6383930"
	fixed := "fixed"
	base := NewDraft("warehouse", json.RawMessage(`{"a":1}`), DefaultProcesses("warehouse"), []Variant{{
		ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Name: "AMR", Status: "ready",
		Fleet:     []FleetItem{{ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", SolutionID: &sid, Quantity: 2, TaskCodes: []string{"outbound", "inbound"}}},
		Financing: []Financing{{ID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Kind: "raas", Tariff: &fixed}, {Kind: "buy"}},
	}}, nil)
	base.AssumptionSets[0].ID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	base.ActiveAssumptionSetID = base.AssumptionSets[0].ID
	h1, err := InputHash(base)
	if err != nil {
		t.Fatal(err)
	}
	next, err := cloneDraft(base)
	if err != nil {
		t.Fatal(err)
	}
	next.Variants[0].ID = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	next.Variants[0].Fleet[0].ID = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	next.Variants[0].Fleet[0].TaskCodes = []string{"inbound", "outbound"}
	next.Variants[0].Financing[0], next.Variants[0].Financing[1] = next.Variants[0].Financing[1], next.Variants[0].Financing[0]
	next.AssumptionSets[0].ID = "99999999-9999-4999-8999-999999999999"
	next.ActiveAssumptionSetID = next.AssumptionSets[0].ID
	h2, err := InputHash(next)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("database ids or unordered children changed hash\n%s\n%s", h1, h2)
	}
}

func TestInputHashReadsLegacyParamFormats(t *testing.T) {
	cases := []struct {
		name   string
		legacy string
		stored string
		same   bool
	}{
		{"dims string", `{"pallet_size_mm":"1200x800x1600"}`, `{"pallet_size_mm":{"length":1200,"width":800,"height":1600}}`, true},
		{"dims spaced sign", `{"unit_size_mm":"300 \u00d7 200 \u00d7 150"}`, `{"unit_size_mm":{"length":300,"width":200,"height":150}}`, true},
		{"dims cyrillic x", `{"unit_size_mm":"300\u0445200\u0445150"}`, `{"unit_size_mm":{"length":300,"width":200,"height":150}}`, true},
		{"clock without zero", `{"shift_window":{"start":"8:00","end":"22:00:00"}}`, `{"shift_window":{"start":"08:00","end":"22:00"}}`, true},
		{"other value", `{"pallet_size_mm":"1200x800x1500"}`, `{"pallet_size_mm":{"length":1200,"width":800,"height":1600}}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := NewDraft("warehouse", json.RawMessage(c.legacy), DefaultProcesses("warehouse"), DefaultVariants(), nil)
			b := NewDraft("warehouse", json.RawMessage(c.stored), DefaultProcesses("warehouse"), DefaultVariants(), nil)
			ha, err := InputHash(a)
			if err != nil {
				t.Fatal(err)
			}
			hb, err := InputHash(b)
			if err != nil {
				t.Fatal(err)
			}
			if (ha == hb) != c.same {
				t.Fatalf("same=%v\n%s\n%s", ha == hb, ha, hb)
			}
		})
	}
}

// NOTE: the process name is test data kept as it was, so the golden value pins the hash function only.
// A new hash for the same draft makes every saved run look stale. If this fails on purpose, update the value and
// add a migration that clears calculation_runs.draft_hash.
func TestInputHashGolden(t *testing.T) {
	sid := "5760e938-9a43-45a7-b8e8-f4f2e6383930"
	d := Draft{
		ObjectType: "warehouse",
		Params:     json.RawMessage(`{"area_m2":5000,"pallet_size_mm":{"length":1200,"width":800,"height":1600}}`),
		Processes: []Process{{
			Code: "inbound", Name: "Приёмка", TaskType: "transport", IsBaseline: true,
			Demand: Demand{UnitsPerDay: 800, Unit: "pallet"}, SortOrder: 1,
		}},
		Variants: []Variant{{
			Name: "Покупка", Status: "draft", SortOrder: 1,
			Fleet:     []FleetItem{{SolutionID: &sid, Quantity: 2, TaskCodes: []string{"inbound"}}},
			Financing: []Financing{{Kind: "buy"}},
		}},
		SharedCosts:      []SharedCost{},
		AssumptionSets:   []AssumptionSet{{Name: "Базовый", IsActive: true, VATRate: 0.2, SortOrder: 1}},
		MatchSelectedIDs: []string{sid},
		ReviewedTabs:     []string{"general"},
	}
	got, err := InputHash(d)
	if err != nil {
		t.Fatal(err)
	}
	const want = "0dcc736323b13281becd6dcb3bca2edc9cfe2315c13dd829e8a8b3ea8a34656f"
	if got != want {
		t.Fatalf("InputHash changed: got %s, want %s", got, want)
	}
}

func TestStale(t *testing.T) {
	if Stale("abc", "") {
		t.Fatal("no run is not stale")
	}
	if !Stale("abc", "def") {
		t.Fatal("different hashes are stale")
	}
	if Stale("abc", "abc") {
		t.Fatal("same hash is current")
	}
}

func TestValidateProcessRejectsNegativeInputs(t *testing.T) {
	p := DefaultProcesses("warehouse")[0]
	p.BaselineStaff.Headcount = -1
	if ValidateProcess(p) == "" {
		t.Fatal("negative headcount must fail")
	}
	p = DefaultProcesses("warehouse")[0]
	p.Durations.TravelS = -1
	if ValidateProcess(p) == "" {
		t.Fatal("negative duration must fail")
	}
}

func TestValidateVariantRejectsInvalidPriceAndDuplicateFinancing(t *testing.T) {
	price := -1.0
	v := Variant{
		Name: "AMR", Status: "draft",
		Fleet:     []FleetItem{{Quantity: 1, PriceOverrideRub: &price, PriceOverrideReason: "КП"}},
		Financing: []Financing{{Kind: "buy"}},
	}
	if ValidateVariant(v) == "" {
		t.Fatal("negative project price must fail")
	}
	v.Fleet = nil
	v.Financing = []Financing{{Kind: "buy"}, {Kind: "buy"}}
	if ValidateVariant(v) == "" {
		t.Fatal("duplicate financing must fail")
	}
}

func TestInputHashChangesOnMapAndSimInputs(t *testing.T) {
	base := NewDraft("warehouse", json.RawMessage(`{}`), DefaultProcesses("warehouse"), DefaultVariants(), nil)
	h0, err := InputHash(base)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(d *Draft){
		"map": func(d *Draft) {
			d.Map = json.RawMessage(`{"schema_version":"map-v1","layers":{"edges":[{"id":"e1","width_m":2}]}}`)
		},
		"priority": func(d *Draft) {
			d.Processes = append([]Process(nil), d.Processes...)
			d.Processes[1].SLA.Priority = 3
		},
		"units per job": func(d *Draft) {
			d.Processes = append([]Process(nil), d.Processes...)
			d.Processes[2].Demand.UnitsPerJob = 25
		},
		"station time": func(d *Draft) {
			d.Processes = append([]Process(nil), d.Processes...)
			d.Processes[0].Durations.LoadS = 45
		},
	}
	seen := map[string]string{h0: "base"}
	for name, edit := range cases {
		d := base
		edit(&d)
		h, err := InputHash(d)
		if err != nil {
			t.Fatal(err)
		}
		if prev, dup := seen[h]; dup {
			t.Fatalf("%s gives the same hash as %s", name, prev)
		}
		seen[h] = name
	}
	narrow := base
	narrow.Map = json.RawMessage(`{"schema_version":"map-v1","layers":{"edges":[{"id":"e1","width_m":1.9}]}}`)
	wide := base
	wide.Map = json.RawMessage(`{"layers":{"edges":[{"width_m":1.9,"id":"e1"}]},"schema_version":"map-v1"}`)
	hn, _ := InputHash(narrow)
	hw, _ := InputHash(wide)
	if hn != hw {
		t.Fatal("map key order must not change the hash")
	}
}

func TestValidateProcessSimFields(t *testing.T) {
	p := DefaultProcesses("warehouse")[0]
	p.SLA.Priority = 10
	if ValidateProcess(p) == "" {
		t.Fatal("priority above 9 accepted")
	}
	p = DefaultProcesses("warehouse")[0]
	p.Demand.UnitsPerJob = -1
	if ValidateProcess(p) == "" {
		t.Fatal("negative units per job accepted")
	}
	p = DefaultProcesses("warehouse")[0]
	p.SLA.MaxWaitMin, p.SLA.MaxCycleMin = 50, 40
	if ValidateProcess(p) == "" {
		t.Fatal("wait longer than cycle accepted")
	}
}
