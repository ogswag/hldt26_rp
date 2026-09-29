package geom

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"moscow_hackathon_2026/api/internal/objects"
)

func warehouseParams(t *testing.T) json.RawMessage {
	t.Helper()
	def, err := objects.Defaults(objects.Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func warehouseH1500(t *testing.T) Input {
	t.Helper()
	path := 2 * math.Sqrt(10000)
	th := 3600 / (path/1.5 + HandlePalletS)
	return Input{
		ObjectType:   "warehouse",
		WorkKind:     WorkPallet,
		FleetSize:    13,
		Throughput:   math.Round(th*10000) / 10000,
		PeakOps:      (1000 + 1000) / 22 * 1.5,
		Unit:         "поддон/ч",
		Seed:         0,
		ScenarioKind: "buy",
		SpeedMps:     1.5,
		WidthMm:      654,
		EnduranceH:   6,
		ChargeMin:    18,
		Params:       warehouseParams(t),
	}
}

func TestCycleGeomMatchesEconPath(t *testing.T) {
	m := parseParams(warehouseParams(t))
	path, handle := cycleGeom("warehouse", WorkPallet, m)
	if math.Abs(path-200) > 1e-9 {
		t.Fatalf("path %v", path)
	}
	if handle != HandlePalletS {
		t.Fatalf("handle %v", handle)
	}
}

func TestWarehouseH1500WithinBand(t *testing.T) {
	in := warehouseH1500(t)
	out := Run(in)
	if out.VerificationFlag {
		t.Fatalf("flag true th=%v econ=%v div=%v queue=%v", out.Throughput, out.EconThroughput, out.Divergence, out.QueueWaitS)
	}
	if out.FleetSize != 13 {
		t.Fatalf("fleet %d", out.FleetSize)
	}
	if out.EconThroughput != in.Throughput {
		t.Fatalf("econ th %v want %v", out.EconThroughput, in.Throughput)
	}
	if out.Divergence > DivergenceMax {
		t.Fatalf("div %v", out.Divergence)
	}
	if out.Throughput <= 0 {
		t.Fatal("sim throughput")
	}
}

func TestTightSlotsSetsFlag(t *testing.T) {
	in := warehouseH1500(t)
	in.LoadSlots = 1
	in.UnloadSlots = 1
	out := Run(in)
	if !out.VerificationFlag {
		t.Fatalf("want flag th=%v econ=%v div=%v queue=%v", out.Throughput, out.EconThroughput, out.Divergence, out.QueueWaitS)
	}
	if out.Bottleneck != "op_points" {
		t.Fatalf("bottleneck %q", out.Bottleneck)
	}
}

func TestScenariosSamePhysics(t *testing.T) {
	in := warehouseH1500(t)
	in.ScenarioKind = "buy"
	buy := Run(in)
	for _, kind := range []string{"raas", "baseline"} {
		in.ScenarioKind = kind
		got := Run(in)
		if got.ScenarioKind != kind {
			t.Fatalf("scenario %q", got.ScenarioKind)
		}
		got.ScenarioKind = buy.ScenarioKind
		if !reflect.DeepEqual(buy, got) {
			t.Fatalf("%s summary differs from buy:\n%+v\n%+v", kind, got, buy)
		}
	}
	if buy.VerificationFlag {
		t.Fatal("demo warehouse must stay inside the band")
	}
}

func TestSlotCounts(t *testing.T) {
	tests := []struct {
		name         string
		in           Input
		load, unload int
	}{
		{"warehouse", Input{ObjectType: "warehouse"}, 3, 3},
		{"airport", Input{ObjectType: "airport"}, 3, 3},
		{"hospital", Input{ObjectType: "hospital"}, 2, 3},
		{"override", Input{ObjectType: "hospital", LoadSlots: 1, UnloadSlots: 5}, 1, 5},
	}
	for _, tt := range tests {
		load, unload := slotCounts(tt.in, makeLayout(tt.in))
		if load != tt.load || unload != tt.unload {
			t.Errorf("%s: slots %d/%d want %d/%d", tt.name, load, unload, tt.load, tt.unload)
		}
	}
}

func TestAirportHospitalSummary(t *testing.T) {
	tests := []struct {
		objectType string
		work       string
		throughput float64
		divergence float64
		queueWaitS float64
	}{
		{"airport", WorkAirportTrolley, 6.4171, 0.3583, 15},
		{"hospital", WorkHospitalCart, 7.8947, 0.2105, 36},
	}
	for _, tt := range tests {
		def, err := objects.Defaults(tt.objectType)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(def)
		if err != nil {
			t.Fatal(err)
		}
		out := Run(Input{
			ObjectType:   tt.objectType,
			FleetSize:    6,
			Throughput:   10,
			SpeedMps:     1.2,
			ScenarioKind: "buy",
			Params:       raw,
		})
		if out.WorkKind != tt.work || out.Throughput != tt.throughput || out.Divergence != tt.divergence || out.QueueWaitS != tt.queueWaitS || !out.VerificationFlag {
			t.Errorf("%s: %+v", tt.objectType, out)
		}
	}
}

func TestZeroFleetNoPanic(t *testing.T) {
	in := warehouseH1500(t)
	in.FleetSize = 0
	out := Run(in)
	if out.Throughput != 0 {
		t.Fatalf("th %v", out.Throughput)
	}
}
