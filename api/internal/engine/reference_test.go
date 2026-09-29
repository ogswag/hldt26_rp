package engine_test

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/engine"
	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/sim"
	"moscow_hackathon_2026/api/internal/simbuild"
)

var update = flag.Bool("update", false, "rewrite contracts/engine/reference.json")

const referencePath = "../../../contracts/engine/reference.json"

// Case is one engine call with the answer the native build gives. scripts/ci/wasm-parity.mjs replays the
// requests through the browser build and compares.
type Case struct {
	Name    string          `json:"name"`
	Method  string          `json:"method"`
	Request json.RawMessage `json:"request"`
	Result  json.RawMessage `json:"result"`
	// Ignore lists result paths the browser build fills from its own clock.
	Ignore []string `json:"ignore,omitempty"`
}

func ptr[T any](v T) *T { return &v }

// catalogFixture is a small catalog, so the parity check needs no database.
func catalogFixture() engine.Catalog {
	kind, sub, scen := "amr", "tote", "warehouse"
	return engine.Catalog{
		ContentSHA256: "reference",
		Candidates: []matching.Candidate{
			{
				ID: "5760e938-9a43-45a7-b8e8-f4f2e6383930", Name: "H1500", Kind: &kind, Subtype: &sub, Scenario: &scen,
				Family: "AMR", PriceRub: ptr(3_100_000.0), ObjectTypes: []string{"warehouse"},
				PayloadKg: ptr(1500.0), WidthMm: ptr(880.0), MinAisleMm: ptr(1400.0),
				TempMinC: ptr(-5.0), TempMaxC: ptr(40.0),
				CapabilityCodes: []string{"transport", "lift_pallet", "narrow_aisle"},
			},
			{
				ID: "2ffc706d-fe43-4c2b-baad-a624a95ad3ce", Name: "Штабелёр", Kind: &kind, Subtype: &sub, Scenario: &scen,
				Family: "Stacker", PriceRub: ptr(4_800_000.0), ObjectTypes: []string{"warehouse"},
				PayloadKg: ptr(1200.0), WidthMm: ptr(1200.0), MinAisleMm: ptr(2600.0),
				TempMinC: ptr(0.0), TempMaxC: ptr(35.0),
				CapabilityCodes: []string{"transport", "lift_pallet", "stack_high"},
			},
		},
		Robots: []econ.Robot{
			{
				ID: "5760e938-9a43-45a7-b8e8-f4f2e6383930", Name: "H1500", Kind: &kind, Subtype: &sub, Family: "AMR",
				Scenario: &scen, PriceRub: ptr(3_100_000.0), SpeedMps: ptr(1.4), WidthMm: ptr(880.0),
				PayloadKg: ptr(1500.0), LifetimeYears: ptr(7.0), ServicePctYear: ptr(0.08),
				EnduranceH: ptr(8.0), ChargeMin: ptr(60.0),
			},
			{
				ID: "2ffc706d-fe43-4c2b-baad-a624a95ad3ce", Name: "Штабелёр", Kind: &kind, Subtype: &sub, Family: "Stacker",
				Scenario: &scen, PriceRub: ptr(4_800_000.0), SpeedMps: ptr(1.1), WidthMm: ptr(1200.0),
				PayloadKg: ptr(1200.0), LifetimeYears: ptr(8.0), ServicePctYear: ptr(0.09),
				EnduranceH: ptr(6.0), ChargeMin: ptr(75.0),
			},
		},
	}
}

func defaults(t *testing.T, objectType string) json.RawMessage {
	t.Helper()
	out, err := objects.FillDefaults(objectType, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("defaults %s: %v", objectType, err)
	}
	return out
}

type wasmRequest struct {
	Catalog    engine.Catalog  `json:"catalog"`
	ObjectType string          `json:"object_type"`
	Params     json.RawMessage `json:"params"`
	Seed       int             `json:"seed"`
	Scenario   string          `json:"scenario,omitempty"`

	Collections ops.State       `json:"collections,omitempty"`
	Sim         *sim.Config     `json:"sim,omitempty"`
	Document    json.RawMessage `json:"document,omitempty"`
}

const referenceRobot = "5760e938-9a43-45a7-b8e8-f4f2e6383930"

var referenceIDs = uuid.MustParse("0b5f6c2e-8a54-4a0e-9a6b-3d5f52a1c7e1")

// referenceState is a warehouse project as its records, with ids that do not change between runs.
func referenceState(t *testing.T, withFleet bool) ops.State {
	t.Helper()
	id := func(name string) string { return uuid.NewSHA1(referenceIDs, []byte(name)).String() }
	procs := projects.DefaultProcesses("warehouse")
	for i := range procs {
		procs[i].ID = id("process-" + procs[i].Code)
		procs[i].PointIDs = []string{}
	}
	d := projects.NewDraft("warehouse", defaults(t, "warehouse"), procs, nil, nil)
	sets := projects.DefaultAssumptionSets()
	for i := range sets {
		sets[i].ID = id("set-" + sets[i].Name)
	}
	d.AssumptionSets = sets
	d.ActiveAssumptionSetID = sets[0].ID
	v := projects.Variant{ID: id("variant"), Name: "Вариант 1", Fleet: []projects.FleetItem{}}
	if withFleet {
		robot := referenceRobot
		v.Fleet = []projects.FleetItem{{ID: id("fleet"), SolutionID: &robot, Quantity: 2}}
	}
	d.Variants = []projects.Variant{v}
	st, err := ops.FromDraft("Склад", d)
	if err != nil {
		t.Fatalf("reference state: %v", err)
	}
	return st
}

func mustRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// localCases are the calls that take a project as its records.
func localCases(t *testing.T, cat engine.Catalog) []Case {
	t.Helper()
	clock := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	quick := &sim.Config{Mode: sim.ModeDeterministic, HorizonH: 1}
	var out []Case
	for _, c := range []struct {
		name  string
		fleet bool
	}{{"simulate_map_suggestion", false}, {"simulate_map_fleet", true}} {
		st := referenceState(t, c.fleet)
		run, err := simbuild.RunLocal(context.Background(), simbuild.LocalRequest{Catalog: cat, Collections: st, Sim: *quick}, clock)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		out = append(out, Case{
			Name: c.name, Method: "simulateMap",
			Request: mustRaw(t, wasmRequest{Catalog: cat, Collections: st, Sim: quick}),
			Result:  mustJSON(t, run),
			Ignore:  []string{"summary.finished_at", "summary.duration_ms"},
		})
	}

	st := referenceState(t, true)
	d, err := simbuild.DraftOf(st)
	if err != nil {
		t.Fatal(err)
	}
	res, _, err := engine.Calculate(cat, engine.Request{ObjectType: "warehouse", Params: defaults(t, "warehouse"), Draft: d, HasDraft: true, Seed: 7})
	if err != nil {
		t.Fatalf("calculate project: %v", err)
	}
	out = append(out, Case{Name: "calculate_project", Method: "calculate", Request: mustRaw(t, wasmRequest{Catalog: cat, Collections: st, Seed: 7}), Result: mustJSON(t, res)})

	template := simbuild.Template(d)
	out = append(out, Case{Name: "map_template", Method: "mapTemplate", Request: mustRaw(t, wasmRequest{Collections: st}), Result: mustJSON(t, template)})

	check, err := simbuild.LocalMapCheck(cat, st, mustRaw(t, template))
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, Case{Name: "check_map", Method: "checkMap", Request: mustRaw(t, wasmRequest{Catalog: cat, Collections: st, Document: mustRaw(t, template)}), Result: mustJSON(t, check)})

	hash, err := simbuild.DraftHash(st)
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, Case{Name: "draft_hash", Method: "draftHash", Request: mustRaw(t, wasmRequest{Collections: st}), Result: mustJSON(t, hash)})
	return out
}

func TestReferenceMatchesEngine(t *testing.T) {
	cat := catalogFixture()
	var cases []Case
	for _, objectType := range []string{"warehouse", "airport", "hospital"} {
		params := defaults(t, objectType)
		req := wasmRequest{Catalog: cat, ObjectType: objectType, Params: params, Seed: 7}
		raw, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		out, err := engine.Match(cat, objectType, params, nil, engine.TaskCodes(objectType, nil, nil))
		if err != nil {
			t.Fatalf("match %s: %v", objectType, err)
		}
		cases = append(cases, Case{Name: "match_" + objectType, Method: "match", Request: raw, Result: mustJSON(t, out)})

		res, _, err := engine.Calculate(cat, engine.Request{ObjectType: objectType, Params: params, Seed: 7})
		if err != nil {
			t.Fatalf("calculate %s: %v", objectType, err)
		}
		cases = append(cases, Case{Name: "calculate_" + objectType, Method: "calculate", Request: raw, Result: mustJSON(t, res)})
	}

	cases = append(cases, localCases(t, cat)...)

	want := mustJSON(t, cases)
	if *update {
		if err := os.MkdirAll(filepath.Dir(referencePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(referencePath, append(want, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatalf("read reference: %v (run: go test ./internal/engine -update)", err)
	}
	if string(want)+"\n" != string(got) {
		t.Fatalf("reference.json is stale: run go test ./internal/engine -update")
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	out, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return out
}
