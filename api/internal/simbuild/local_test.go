package simbuild

import (
	"context"
	"encoding/json"
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
)

const fixtureRobot = "5760e938-9a43-45a7-b8e8-f4f2e6383930"

func ptr[T any](v T) *T { return &v }

func fixtureCatalog() engine.Catalog {
	kind, sub, scen := "amr", "tote", "warehouse"
	return engine.Catalog{
		ContentSHA256: "local-test",
		Candidates: []matching.Candidate{{
			ID: fixtureRobot, Name: "H1500", Kind: &kind, Subtype: &sub, Scenario: &scen, Family: "AMR",
			PriceRub: ptr(3_100_000.0), ObjectTypes: []string{"warehouse"}, PayloadKg: ptr(1500.0), WidthMm: ptr(880.0),
			MinAisleMm: ptr(1400.0), TempMinC: ptr(-5.0), TempMaxC: ptr(40.0),
			CapabilityCodes: []string{"transport", "lift_pallet", "narrow_aisle"},
		}},
		Robots: []econ.Robot{{
			ID: fixtureRobot, Name: "H1500", Kind: &kind, Subtype: &sub, Family: "AMR", Scenario: &scen,
			PriceRub: ptr(3_100_000.0), SpeedMps: ptr(1.4), WidthMm: ptr(880.0), PayloadKg: ptr(1500.0),
			LifetimeYears: ptr(7.0), ServicePctYear: ptr(0.08), EnduranceH: ptr(8.0), ChargeMin: ptr(60.0),
		}},
	}
}

func defaultDraft(t *testing.T, objectType string) projects.Draft {
	t.Helper()
	params, err := objects.FillDefaults(objectType, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	procs := projects.DefaultProcesses(objectType)
	for i := range procs {
		procs[i].ID = uuid.NewString()
		procs[i].PointIDs = []string{}
	}
	d := projects.NewDraft(objectType, params, procs, nil, nil)
	d.AssumptionSets = projects.DefaultAssumptionSets()
	for i := range d.AssumptionSets {
		d.AssumptionSets[i].ID = uuid.NewString()
	}
	d.ActiveAssumptionSetID = d.AssumptionSets[0].ID
	d.Variants = []projects.Variant{{ID: "v1", Name: "Вариант 1", Fleet: []projects.FleetItem{}}}
	return d
}

func fixtureState(t *testing.T, mutate func(*projects.Draft)) ops.State {
	t.Helper()
	d := defaultDraft(t, "warehouse")
	if mutate != nil {
		mutate(&d)
	}
	st, err := ops.FromDraft("", d)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

var fixedClock = func() time.Time { return time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC) }

func quick() sim.Config {
	return sim.Config{Mode: sim.ModeDeterministic, HorizonH: 1}
}

func TestRunLocalRunsTheSuggestionOnTheTemplateMap(t *testing.T) {
	run, err := RunLocal(context.Background(), LocalRequest{Catalog: fixtureCatalog(), Collections: fixtureState(t, nil), Sim: quick()}, fixedClock)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != LocalDone || run.Summary == nil {
		t.Fatalf("run %+v", run)
	}
	s := run.Summary
	if s.VariantName != projects.SuggestionName || s.MapSource != MapSourceTemplate || s.ConfidenceLevel != ConfidencePreliminary {
		t.Fatalf("summary %s %s %s", s.VariantName, s.MapSource, s.ConfidenceLevel)
	}
	if s.InputHash == "" || s.Snapshot.SimVersion != sim.Version || s.CatalogContentSHA256 != "local-test" {
		t.Fatalf("meta %+v", s)
	}
	if run.Log != nil {
		t.Fatal("log returned without being asked")
	}
}

func TestRunLocalKeepsTheLogOnRequest(t *testing.T) {
	run, err := RunLocal(context.Background(), LocalRequest{Catalog: fixtureCatalog(), Collections: fixtureState(t, nil), Sim: quick(), WithLog: true}, fixedClock)
	if err != nil || run.Log == nil || len(run.Log.Events) == 0 {
		t.Fatalf("log %v %v", run.Log, err)
	}
}

func TestRunLocalUsesTheVariantFleet(t *testing.T) {
	st := fixtureState(t, func(d *projects.Draft) {
		d.Variants = []projects.Variant{{ID: "v1", Name: "AMR", Fleet: []projects.FleetItem{{ID: "f1", SolutionID: ptr(fixtureRobot), Quantity: 2}}}}
	})
	run, err := RunLocal(context.Background(), LocalRequest{Catalog: fixtureCatalog(), Collections: st, Sim: quick()}, fixedClock)
	if err != nil || run.Status != LocalDone {
		t.Fatalf("run %+v %v", run, err)
	}
	if run.Summary.VariantID != "v1" || run.Summary.VariantName != "AMR" {
		t.Fatalf("variant %s %s", run.Summary.VariantID, run.Summary.VariantName)
	}
}

func TestRunLocalIsDeterministic(t *testing.T) {
	a, err := RunLocal(context.Background(), LocalRequest{Catalog: fixtureCatalog(), Collections: fixtureState(t, nil), Sim: quick()}, fixedClock)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RunLocal(context.Background(), LocalRequest{Catalog: fixtureCatalog(), Collections: fixtureState(t, nil), Sim: quick()}, fixedClock)
	if err != nil {
		t.Fatal(err)
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatal("the same records and seed gave two answers")
	}
}

func TestRunLocalRefusesWhatTheUserCanFix(t *testing.T) {
	cases := []struct {
		name string
		req  LocalRequest
		want string
	}{
		{"variant without robots", LocalRequest{Catalog: fixtureCatalog(), Collections: fixtureState(t, nil), VariantID: "v1", Sim: quick()}, "нет роботов"},
		{"unknown variant", LocalRequest{Catalog: fixtureCatalog(), Collections: fixtureState(t, nil), VariantID: "nope", Sim: quick()}, "Вариант не найден"},
		{"too many repeats", LocalRequest{Catalog: fixtureCatalog(), Collections: fixtureState(t, nil), Sim: sim.Config{Replications: 99}}, "повторов"},
		{"empty catalog", LocalRequest{Catalog: engine.Catalog{}, Collections: fixtureState(t, nil), Sim: quick()}, "нужен робот"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run, err := RunLocal(context.Background(), c.req, fixedClock)
			if err != nil {
				t.Fatal(err)
			}
			if run.Status != LocalRefused || run.Summary != nil || !contains(run.Message, c.want) {
				t.Fatalf("run %+v", run)
			}
		})
	}
}

func TestRunLocalRefusesAnAirport(t *testing.T) {
	st, err := ops.FromDraft("", defaultDraft(t, "airport"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := RunLocal(context.Background(), LocalRequest{Catalog: fixtureCatalog(), Collections: st, Sim: quick()}, fixedClock)
	if err != nil || run.Status != LocalRefused || !contains(run.Message, "только для склада") {
		t.Fatalf("run %+v %v", run, err)
	}
}

func TestRunLocalReportsMapErrors(t *testing.T) {
	st := fixtureState(t, func(d *projects.Draft) {
		d.Variants = []projects.Variant{{ID: "v1", Name: "AMR", Fleet: []projects.FleetItem{{ID: "f1", SolutionID: ptr(fixtureRobot), Quantity: 1}}}}
		d.Map = json.RawMessage(`{"schema_version":"map-v1","profile":"warehouse","units":"m","layers":{}}`)
	})
	run, err := RunLocal(context.Background(), LocalRequest{Catalog: fixtureCatalog(), Collections: st, Sim: quick()}, fixedClock)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != LocalRefused || len(run.Issues) == 0 {
		t.Fatalf("run %+v", run)
	}
}

func TestDraftHashFollowsTheInputs(t *testing.T) {
	a, err := DraftHash(fixtureState(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	same, _ := DraftHash(fixtureState(t, nil))
	other, _ := DraftHash(fixtureState(t, func(d *projects.Draft) {
		d.Variants = []projects.Variant{{ID: "v1", Name: "AMR", Fleet: []projects.FleetItem{{ID: "f1", SolutionID: ptr(fixtureRobot), Quantity: 2}}}}
	}))
	if a == "" || a != same || a == other {
		t.Fatalf("hashes %s %s %s", a, same, other)
	}
}

func TestLocalMapCheckReadsTheDocument(t *testing.T) {
	st := fixtureState(t, nil)
	d, _ := DraftOf(st)
	raw, _ := json.Marshal(Template(d))
	check, err := LocalMapCheck(fixtureCatalog(), st, raw)
	if err != nil {
		t.Fatal(err)
	}
	if check.Scene == nil || len(check.Edges) == 0 || len(check.Classes) == 0 {
		t.Fatalf("check %+v", check)
	}
	if _, err := LocalMapCheck(fixtureCatalog(), st, json.RawMessage(`[1]`)); err == nil {
		t.Fatal("a map that is not an object was accepted")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func nullPaths(v any, path string, out *[]string) {
	switch x := v.(type) {
	case nil:
		*out = append(*out, path)
	case []any:
		for i, e := range x {
			if i < 3 {
				nullPaths(e, path+"[]", out)
			}
		}
	case map[string]any:
		for k, e := range x {
			nullPaths(e, path+"."+k, out)
		}
	}
}

// The page reads every list of a run with .length, so a list the engine has nothing for must be [], not null.
func TestRunLocalHasNoNullLists(t *testing.T) {
	quiet := false
	for _, qty := range []int{1, 4, 12, 40, 80} {
		st := fixtureState(t, func(d *projects.Draft) {
			for i := range d.Processes {
				d.Processes[i].Demand.UnitsPerDay = 40
			}
			d.Variants = []projects.Variant{{ID: "v1", Name: "AMR", Fleet: []projects.FleetItem{{ID: "f1", SolutionID: ptr(fixtureRobot), Quantity: qty}}}}
		})
		run, err := RunLocal(context.Background(), LocalRequest{Catalog: fixtureCatalog(), Collections: st, Sim: quick(), WithLog: true}, fixedClock)
		if err != nil || run.Status != LocalDone {
			t.Fatalf("fleet %d: run %+v %v", qty, run, err)
		}
		quiet = quiet || len(run.Summary.Result.Bottlenecks) == 0
		body, err := json.Marshal(run)
		if err != nil {
			t.Fatal(err)
		}
		var generic any
		if err := json.Unmarshal(body, &generic); err != nil {
			t.Fatal(err)
		}
		var nulls []string
		nullPaths(generic, "run", &nulls)
		if len(nulls) > 0 {
			t.Errorf("fleet %d: null values %v", qty, nulls)
		}
	}
	if !quiet {
		t.Error("no fleet ran without a bottleneck, so the empty list was never checked")
	}
}
