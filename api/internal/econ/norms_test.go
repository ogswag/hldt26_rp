package econ

import (
	"testing"

	"moscow_hackathon_2026/api/internal/objects"
)

func TestChargersFromSpecsAndFromTheNorm(t *testing.T) {
	spec := robotWith("a", "С ТТХ", 1_000_000, 8, 0.08)
	hours, minutes := 6.0, 18.0
	spec.EnduranceH, spec.ChargeMin = &hours, &minutes
	bare := robotWith("b", "Без ТТХ", 1_000_000, 8, 0.08)
	n := DefaultNorms()
	item := func(r Robot, qty int) fleetItem { return newFleetItem(r, qty, 1_000_000, AssumptionValues{}, n) }

	cases := []struct {
		name  string
		items []fleetItem
		want  int
	}{
		{"few robots with specs share one station", []fleetItem{item(spec, 13)}, 1},
		{"a share of 18 in 378 minutes of a cycle", []fleetItem{item(spec, 40)}, 2},
		{"without specs one station per four robots", []fleetItem{item(bare, 13)}, 4},
		{"lines add up", []fleetItem{item(spec, 40), item(bare, 5)}, 4},
		{"no robots, no stations", nil, 0},
	}
	for _, c := range cases {
		if got := chargers(c.items, n); got != c.want {
			t.Errorf("%s: %d stations, want %d", c.name, got, c.want)
		}
	}
}

func TestTechniciansPerShift(t *testing.T) {
	n := DefaultNorms()
	for _, c := range []struct {
		fleet  int
		shifts float64
		want   float64
	}{{13, 2, 4}, {10, 3, 3}, {11, 3, 6}, {0, 2, 0}, {5, 0, 0}} {
		if got := technicians(c.fleet, c.shifts, n); got != c.want {
			t.Errorf("fleet %d on %v shifts: %v people, want %v", c.fleet, c.shifts, got, c.want)
		}
	}
}

func TestAssumptionSetReplacesNormsOnlyWhereItIsFilled(t *testing.T) {
	util, avail, bad := 0.7, 0.95, 1.5
	got := DefaultNorms().with(AssumptionValues{Utilization: &util, Availability: &avail, Reserve: &bad})
	if got.Utilization != 0.7 || got.Availability != 0.95 {
		t.Fatalf("filled fields %+v", got)
	}
	if got.Reserve != Reserve {
		t.Fatalf("a reserve above 100%% must leave the norm, got %v", got.Reserve)
	}
	if got.DeliveryFrac != DeliveryFrac || got.TechnicianWageMonthRub != TechnicianWageMonthRub {
		t.Fatalf("empty fields must keep the norm: %+v", got)
	}
}

func TestFleetSizeFollowsUtilizationAndReserve(t *testing.T) {
	n := DefaultNorms()
	// 100 / (10 x 0,9 x 0,8) = 13,9 -> 14; 14 x 1,15 = 16,1 -> 17.
	if got := fleetSize(100, 10, n); got != 17 {
		t.Fatalf("default fleet %d", got)
	}
	util := 0.7
	// 100 / (10 x 0,9 x 0,7) = 15,9 -> 16; 16 x 1,15 = 18,4 -> 19.
	if got := fleetSize(100, 10, n.with(AssumptionValues{Utilization: &util})); got != 19 {
		t.Fatalf("fleet at 70%% utilization %d", got)
	}
}

func TestServiceShareOfTheSetBeatsTheRobot(t *testing.T) {
	r := robotWith("a", "Робот", 1_000_000, 8, 0.10)
	n := DefaultNorms()
	if v, assumed := serviceFracOf(r, n, AssumptionValues{}); v != 0.10 || assumed {
		t.Fatalf("robot share %v %v", v, assumed)
	}
	share := 0.05
	if v, assumed := serviceFracOf(r, n, AssumptionValues{ServiceShare: &share}); v != 0.05 || assumed {
		t.Fatalf("set share %v %v", v, assumed)
	}
	bare := robotWith("b", "Без доли", 1_000_000, 8, 0)
	if v, assumed := serviceFracOf(bare, n, AssumptionValues{}); v != DefaultServiceFrac || !assumed {
		t.Fatalf("norm share %v %v", v, assumed)
	}
}

func TestViewSaysWhichValuesTheSetFixed(t *testing.T) {
	wage := 100_000.0
	v := AssumptionValues{TechnicianWageMonthRub: &wage}.view(DefaultNorms())
	if !v.TechnicianWageMonthRub.Set || v.TechnicianWageMonthRub.Value != 100_000 {
		t.Fatalf("wage %+v", v.TechnicianWageMonthRub)
	}
	if v.Utilization.Set || v.Utilization.Value != Utilization {
		t.Fatalf("utilization %+v", v.Utilization)
	}
	if v.ServiceShare.Set || v.ServiceShare.Value != DefaultServiceFrac {
		t.Fatalf("service share %+v", v.ServiceShare)
	}
}

// H1500 fleet of 13 on two shifts: 2 technicians a shift, 4 people at 90 000 a month with contributions 1,302.
func TestOperationCostsOfTheH1500Fleet(t *testing.T) {
	r := h1500()
	run := func(a AssumptionValues) Result {
		got, err := Calculate(Input{ObjectType: objects.Warehouse, Params: warehouseParams(t), Robot: &r, PickReason: "recommended", Assumptions: a})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	def := run(AssumptionValues{})
	if def.Shared.Chargers != 4 {
		t.Fatalf("stations %d, want 13 robots / 4", def.Shared.Chargers)
	}
	lines := map[string]float64{}
	for _, l := range def.Breakdown {
		if l.Scenario == "buy" {
			lines[l.ID] = l.Rub
		}
	}
	for id, want := range map[string]float64{"delivery": 1_053_000, "communications": 156_000, "technicians": 5_624_640, "service": 2_808_000} {
		if lines[id] != want {
			t.Errorf("%s %v, want %v", id, lines[id], want)
		}
	}
	for _, l := range def.Breakdown {
		if l.Scenario == "raas" && l.ID == "delivery" {
			t.Fatal("RaaS carries no delivery line")
		}
	}

	wage, comm, delivery := 100_000.0, 0.0, 0.0
	set := run(AssumptionValues{TechnicianWageMonthRub: &wage, CommRubPerRobotYear: &comm, DeliveryShare: &delivery})
	buy, raas := scenario(t, set, "buy"), scenario(t, set, "raas")
	// opex: 29 994 510 - 5 624 640 + 4 x 1 200 000 x 1,302 - 156 000; capex without the 1 053 000 of delivery and its 10% reserve.
	if *buy.OpexYearRub != 30_463_470 {
		t.Fatalf("buy opex %v", *buy.OpexYearRub)
	}
	if *buy.CapexRub != 55_212_300 {
		t.Fatalf("buy capex %v", *buy.CapexRub)
	}
	if *raas.OpexYearRub != 37_145_470 {
		t.Fatalf("raas opex %v", *raas.OpexYearRub)
	}
	if !set.AssumptionSet.TechnicianWageMonthRub.Set || !set.AssumptionSet.DeliveryShare.Set {
		t.Fatalf("view %+v", set.AssumptionSet)
	}
}
