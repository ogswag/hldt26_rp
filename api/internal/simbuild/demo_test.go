package simbuild_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/catalogstore"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/engine"
	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/sim"
	"moscow_hackathon_2026/api/internal/simbuild"
	"moscow_hackathon_2026/api/internal/testdb"
)

var update = flag.Bool("update", false, "rewrite web/src/guest/demo-warehouse.json")

const demoPath = "../../../web/src/guest/demo-warehouse.json"

var demoIDs = uuid.MustParse("6a1f0c52-3c1e-4b6f-8f0e-2c7a4d9b5e13")

// demoFile is what the web app loads as Демо Склад: the records of a project and a version that changes with them.
type demoFile struct {
	Version     string    `json:"version"`
	Collections ops.State `json:"collections"`
}

type demoLine struct {
	robot string
	qty   int
	codes []string
}

// NOTE: the pallet demand is the schema minimum: the 84 x 52 m map has four docks and carries about 500 pallets a
// day, at the default 1000 every variant saturates them. Counts are the largest fleets the map holds without
// congestion, checked by TestDemoWarehouse.
var demoVariants = []struct {
	name  string
	lines []demoLine
}{
	{"Смешанный флот", []demoLine{{"Ronavi H1500 (", 6, []string{"inbound"}}, {"DMR Carrier P", 10, []string{"putaway"}}}},
	{"AMR комплектация", []demoLine{{"Ronavi SR (", 24, []string{"piece_pick"}}, {"Ronavi H1500 (", 6, []string{"inbound"}}}},
	{"Паллетный штабелёр", []demoLine{{"DMR Carrier P", 22, []string{"putaway", "outbound"}}}},
}

func demoID(name string) string { return uuid.NewSHA1(demoIDs, []byte(name)).String() }

func robotID(t *testing.T, cat engine.Catalog, name string) string {
	t.Helper()
	for _, c := range cat.Candidates {
		if strings.HasPrefix(c.Name, name) {
			return c.ID
		}
	}
	t.Fatalf("the catalog has no robot %q", name)
	return ""
}

func demoDraft(t *testing.T, cat engine.Catalog) projects.Draft {
	t.Helper()
	params, err := objects.FillDefaults("warehouse", json.RawMessage(
		`{"inbound_pallets_per_day":500,"outbound_pallets_per_day":500,"pick_lines_per_day":50000,"pick_units_per_day":75000,"floor_load_kg_m2":5000,"door_width_m":3}`))
	if err != nil {
		t.Fatal(err)
	}
	procs := projects.DefaultProcesses("warehouse")
	for i := range procs {
		procs[i].ID = demoID("process-" + procs[i].Code)
		procs[i].PointIDs = []string{}
		procs[i].Demand.UnitsPerDay = 500
		if procs[i].Code == "piece_pick" {
			procs[i].Demand.UnitsPerDay = 50000
		}
	}
	d := projects.NewDraft("warehouse", params, procs, nil, nil)
	d.EconOverrides = json.RawMessage(`{}`)
	d.AssumptionSets = projects.DefaultAssumptionSets()
	for i := range d.AssumptionSets {
		d.AssumptionSets[i].ID = demoID("assumption-set-" + d.AssumptionSets[i].Name)
	}
	d.ActiveAssumptionSetID = d.AssumptionSets[0].ID

	for i, spec := range demoVariants {
		v := projects.DefaultVariants()[0]
		v.ID = demoID("variant-" + spec.name)
		v.Name = spec.name
		v.SortOrder = i
		for j := range v.Financing {
			v.Financing[j].ID = demoID("financing-" + spec.name + "-" + v.Financing[j].Kind)
		}
		for j, l := range spec.lines {
			sid := robotID(t, cat, l.robot)
			v.Fleet = append(v.Fleet, projects.FleetItem{
				ID: demoID("fleet-" + spec.name + "-" + sid), SolutionID: &sid, Quantity: l.qty, TaskCodes: l.codes, SortOrder: j,
			})
		}
		d.Variants = append(d.Variants, v)
	}
	d.Map, _ = json.Marshal(simbuild.Template(d))
	return d
}

func demoBytes(t *testing.T, cat engine.Catalog) ([]byte, ops.State) {
	t.Helper()
	st, err := ops.FromDraft("Демо Склад", demoDraft(t, cat))
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	out, err := json.MarshalIndent(demoFile{Version: hex.EncodeToString(sum[:6]), Collections: st}, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return append(out, '\n'), st
}

func TestDemoWarehouse(t *testing.T) {
	_, q := testdb.New(t, "../../../data")
	cat, err := catalogstore.Load(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	want, st := demoBytes(t, cat)
	if *update {
		if err := os.WriteFile(demoPath, want, 0o644); err != nil {
			t.Fatal(err)
		}
	} else {
		got, err := os.ReadFile(demoPath)
		if err != nil {
			t.Fatalf("read the demo: %v (run: go test ./internal/simbuild -run TestDemoWarehouse -update)", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("web/src/guest/demo-warehouse.json is stale: run go test ./internal/simbuild -run TestDemoWarehouse -update")
		}
	}

	d, err := simbuild.DraftOf(st)
	if err != nil {
		t.Fatal(err)
	}
	res, _, err := engine.Calculate(cat, engine.Request{ObjectType: "warehouse", Params: d.Params, Draft: d, HasDraft: true})
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if len(res.Variants) != len(demoVariants) {
		t.Fatalf("the calculation returned %d variants, the demo has %d", len(res.Variants), len(demoVariants))
	}
	if res.VerificationFlag || len(res.SimChecks) != 0 {
		t.Errorf("a warehouse calculation carries no check of its own, the runs on the map do: %+v", res.SimChecks)
	}
	var briefs []simbuild.RunBrief
	for _, v := range d.Variants {
		run, err := simbuild.RunLocal(context.Background(), simbuild.LocalRequest{Catalog: cat, Collections: st, VariantID: v.ID}, time.Now)
		if err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		if run.Status != simbuild.LocalDone {
			t.Fatalf("%s was refused: %s", v.Name, run.Message)
		}
		sum := run.Summary
		if sum.Result.Verdict != sim.VerdictPass || sum.MapSource != simbuild.MapSourceProject {
			t.Errorf("%s: verdict %s on the %s map: %s", v.Name, sum.Result.Verdict, sum.MapSource, sum.Result.VerdictText)
		}
		if sum.EconCheck == nil || sum.EconCheck.Flag {
			t.Errorf("%s opens with the simulation and the economics apart: %+v", v.Name, sum.EconCheck)
		}
		briefs = append(briefs, simbuild.RunBrief{RunID: v.ID, VariantID: v.ID, VariantHash: sum.VariantHash, EconCheck: sum.EconCheck})
	}
	checks, err := simbuild.Checks(d, briefs)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != len(d.Variants) || econ.SimChecksFlag(checks) {
		t.Fatalf("the demo opens with a raised check: %+v", checks)
	}
	for _, c := range checks {
		if c.Stale || c.RunID == "" {
			t.Errorf("the run of %s was made for other inputs than the variant has: %+v", c.VariantName, c)
		}
	}
}

type acceptAll struct{}

func (acceptAll) Params(ops.Record, string, []string) (string, bool) { return "", true }
func (acceptAll) Object(string, any) bool                            { return true }
func (acceptAll) External(string, string) bool                       { return true }

func loadDemo(t *testing.T) (demoFile, projects.Draft) {
	t.Helper()
	raw, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	var f demoFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	d, err := simbuild.DraftOf(f.Collections)
	if err != nil {
		t.Fatal(err)
	}
	return f, d
}

func TestDemoWarehouseShape(t *testing.T) {
	f, d := loadDemo(t)
	if f.Version == "" || d.ObjectType != "warehouse" {
		t.Fatalf("version %q type %q", f.Version, d.ObjectType)
	}
	if len(d.Variants) != len(demoVariants) {
		t.Fatalf("variants %d", len(d.Variants))
	}
	for _, v := range d.Variants {
		if len(v.Fleet) == 0 || !simbuild.HasFleet(projects.Draft{Variants: []projects.Variant{v}}) {
			t.Errorf("%s has no robots", v.Name)
		}
		for _, item := range v.Fleet {
			if item.SolutionID == nil || uuid.Validate(*item.SolutionID) != nil || item.Quantity < 1 || len(item.TaskCodes) == 0 {
				t.Errorf("%s: fleet item %+v", v.Name, item)
			}
		}
	}

	t.Run("the schema takes it as it is", func(t *testing.T) {
		schema := ops.ProjectSchema()
		base, err := ops.FromDraft("", projectsEmpty())
		if err != nil {
			t.Fatal(err)
		}
		list := ops.Replace(schema, base, f.Collections)
		_, _, out := ops.Apply(schema, base, ops.Tx{TxID: uuid.NewString(), Ops: list}, acceptAll{})
		if out.Status != ops.StatusApplied {
			t.Fatalf("the operation schema refused the demo: %+v %+v", out, out.Details)
		}
	})

	t.Run("the map is the prepared one and passes the check", func(t *testing.T) {
		doc, err := maps.Decode(d.Map)
		if err != nil {
			t.Fatal(err)
		}
		want := simbuild.Template(d)
		route := func(doc maps.Document) map[string][2][]string {
			out := map[string][2][]string{}
			for _, f := range doc.Layers.Flows {
				out[f.ProcessCode] = [2][]string{f.PickupPointIDs, f.DropPointIDs}
			}
			return out
		}
		if !reflect.DeepEqual(route(doc), route(want)) || len(doc.Layers.Points) != len(want.Layers.Points) ||
			len(doc.Layers.Edges) != len(want.Layers.Edges) || len(doc.Layers.Obstacles) != len(want.Layers.Obstacles) {
			t.Fatal("the demo map is not the template of its processes")
		}
		check := simbuild.CheckMap(doc, simbuild.MapContext(nil, d))
		if maps.HasErrors(check.Issues) {
			t.Fatalf("map issues %+v", check.Issues)
		}
	})
}

func projectsEmpty() projects.Draft {
	d := projects.NewDraft("warehouse", nil, nil, nil, nil)
	d.AssumptionSets = []projects.AssumptionSet{}
	return d
}
