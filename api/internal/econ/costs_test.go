package econ

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"moscow_hackathon_2026/api/internal/objects"
)

func paramsWith(t *testing.T, over map[string]any) json.RawMessage {
	t.Helper()
	def, err := objects.Defaults(objects.Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range over {
		def[k] = v
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func robotWith(id, name string, price, life, service float64) Robot {
	speed := 1.5
	width := 654.0
	payload := 1500.0
	r := Robot{ID: id, Name: name, Family: "AMR", PriceRub: &price, SpeedMps: &speed, WidthMm: &width, PayloadKg: &payload}
	if life > 0 {
		r.LifetimeYears = &life
	}
	if service > 0 {
		r.ServicePctYear = &service
	}
	return r
}

func TestBuildCostsMixedFleetUsesEachItemOwnLifeAndService(t *testing.T) {
	a := robotWith("a", "Первый", 2_700_000, 5, 0.05)
	b := robotWith("b", "Второй", 3_500_000, 10, 0.10)
	items := func(order ...string) []fleetItem {
		var out []fleetItem
		for _, id := range order {
			if id == "a" {
				out = append(out, newFleetItem(a, 4, 2_700_000, AssumptionValues{}, DefaultNorms()))
			} else {
				out = append(out, newFleetItem(b, 6, 3_500_000, AssumptionValues{}, DefaultNorms()))
			}
		}
		return out
	}
	one := buildCosts(costInput{Items: items("a", "b"), HoursYear: 1000, IntegFrac: IntegrationYesFrac, Norms: DefaultNorms()})
	two := buildCosts(costInput{Items: items("b", "a"), HoursYear: 1000, IntegFrac: IntegrationYesFrac, Norms: DefaultNorms()})
	if one.Equipment != 31_800_000 {
		t.Fatalf("equipment %v", one.Equipment)
	}
	if one.Service != 2_640_000 {
		t.Fatalf("service %v, want 5%% of 10 800 000 plus 10%% of 21 000 000", one.Service)
	}
	if one.Depreciation != 6_213_429 {
		t.Fatalf("depreciation %v, want 4 260 000 for the robots plus 13 674 000 / 7", one.Depreciation)
	}
	one.Items, two.Items = nil, nil
	if !reflect.DeepEqual(one, two) {
		t.Fatalf("order changed the costs:\n%+v\n%+v", one, two)
	}
}

func TestBuildCostsDepreciableBase(t *testing.T) {
	r := robotWith("a", "Робот", 1_000_000, 8, 0.08)
	cm := buildCosts(costInput{
		Items:     []fleetItem{newFleetItem(r, 10, 1_000_000, AssumptionValues{}, DefaultNorms())},
		HoursYear: 1000, IntegFrac: IntegrationYesFrac,
		Shared: []SharedCostSpec{{Code: "permits", Label: "Разрешения", Bucket: "capex", Rub: 500_000}},
	})
	// infrastructure 1 500 000 + software 800 000 + integration 1 200 000 + commissioning 500 000 + permits 500 000;
	// training 300 000 and the reserve are not assets.
	if cm.DepreciableBase != 4_500_000 {
		t.Fatalf("base %v", cm.DepreciableBase)
	}
	if cm.RaasDepreciation != 642_857 {
		t.Fatalf("raas depreciation %v, want 4 500 000 / 7", cm.RaasDepreciation)
	}
	// NOTE: the buyer's delivery (3% of the equipment) is capitalised too, the RaaS customer's is not.
	if want := roundRub(10_000_000.0/8 + 4_800_000.0/7); cm.Depreciation != want {
		t.Fatalf("depreciation %v, want %v", cm.Depreciation, want)
	}
}

func TestVATPolicyReachesEveryVATBearingLine(t *testing.T) {
	r := robotWith("a", "Робот", 1_000_000, 8, 0.08)
	shared := []SharedCostSpec{
		{Code: "one_off", Label: "Разовая", Bucket: "capex", Rub: 122_000},
		{Code: "yearly", Label: "Годовая", Bucket: "opex", Rub: 244_000},
	}
	build := func(a AssumptionValues) costModel {
		return buildCosts(costInput{
			Items:     []fleetItem{newFleetItem(r, 10, 1_000_000, a, DefaultNorms())},
			HoursYear: 1000, IntegFrac: IntegrationYesFrac, Shared: shared, Assumptions: a,
		})
	}
	rate := 0.22
	gross := build(AssumptionValues{VATRate: &rate})
	net := build(AssumptionValues{VATRate: &rate, VATRecoverable: true})
	cases := []struct {
		name       string
		gross, net float64
	}{
		{"equipment", 10_000_000, 8_196_720},
		{"licenses", 800_000, 655_738},
		{"energy", 130_000, 106_557},
		{"consumables", 200_000, 163_934},
		{"shared capex, whole", 4_422_000, 3_624_590},
		{"shared opex", 244_000, 200_000},
	}
	got := map[string][2]float64{
		"equipment":           {gross.Equipment, net.Equipment},
		"licenses":            {gross.Licenses, net.Licenses},
		"energy":              {gross.Energy, net.Energy},
		"consumables":         {gross.Consumables, net.Consumables},
		"shared capex, whole": {gross.SharedCapex, net.SharedCapex},
		"shared opex":         {gross.SharedOpex, net.SharedOpex},
	}
	for _, c := range cases {
		if g := got[c.name]; g[0] != c.gross || g[1] != c.net {
			t.Errorf("%s: gross %v net %v, want %v and %v", c.name, g[0], g[1], c.gross, c.net)
		}
	}
	excluded := false
	if got := build(AssumptionValues{VATRate: &rate, PricesIncludeVAT: &excluded}); got.Equipment != 12_200_000 || got.Licenses != 800_000 {
		t.Errorf("prices without VAT: equipment %v licenses %v, want 12 200 000 and the constant 800 000", got.Equipment, got.Licenses)
	}
}

func TestVATRecoverableLeavesWagesAlone(t *testing.T) {
	r := h1500()
	rate := 0.22
	run := func(a AssumptionValues) Result {
		got, err := Calculate(Input{ObjectType: objects.Warehouse, Params: warehouseParams(t), Robot: &r, Assumptions: a})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	gross := run(AssumptionValues{VATRate: &rate})
	net := run(AssumptionValues{VATRate: &rate, VATRecoverable: true})
	if *scenario(t, gross, "baseline").OpexYearRub != *scenario(t, net, "baseline").OpexYearRub {
		t.Fatal("wages carry no VAT, the baseline must not move")
	}
	fot := func(res Result) float64 {
		for _, l := range res.Breakdown {
			if l.Scenario == "buy" && l.ID == "fot_remaining" {
				return l.Rub
			}
		}
		t.Fatal("no fot line")
		return 0
	}
	if fot(gross) != fot(net) {
		t.Fatal("remaining payroll moved with VAT")
	}
	if *scenario(t, net, "buy").CapexRub >= *scenario(t, gross, "buy").CapexRub {
		t.Fatal("recoverable VAT must lower CAPEX")
	}
	if *scenario(t, net, "buy").OpexYearRub >= *scenario(t, gross, "buy").OpexYearRub {
		t.Fatal("recoverable VAT must lower OPEX (energy, licenses, consumables, service)")
	}
	if !hasRisk(net, "vat_deduction_conditions") || hasRisk(gross, "vat_deduction_conditions") {
		t.Fatal("the deduction conditions belong to the recoverable case only")
	}
}

func TestMixedFleetOrderDoesNotChangeTheResult(t *testing.T) {
	a := robotWith("a", "Первый", 2_700_000, 5, 0.05)
	b := robotWith("b", "Второй", 3_500_000, 10, 0.10)
	calc := func(fleet ...FleetSpec) Result {
		got, err := Calculate(Input{
			ObjectType: objects.Warehouse,
			Params:     warehouseParams(t),
			Robots:     []Robot{a, b},
			Processes:  warehouseProcessSpecs(),
			Variants: []VariantSpec{{
				ID: "mix", Name: "Смешанный", Fleet: fleet,
				Financing: []FinancingSpec{{Kind: "buy"}, {Kind: "raas", Tariff: "fixed"}},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	fa := FleetSpec{SolutionID: a.ID, Quantity: 4, TaskCodes: []string{"inbound", "putaway", "outbound"}}
	fb := FleetSpec{SolutionID: b.ID, Quantity: 6, TaskCodes: []string{"piece_pick"}}
	ab, ba := calc(fa, fb), calc(fb, fa)
	// NOTE: solution_id names the first robot of the fleet, so it follows the order by design.
	unnamed := func(scs []Scenario) []Scenario {
		out := append([]Scenario(nil), scs...)
		for i := range out {
			out[i].SolutionID = nil
		}
		return out
	}
	if !reflect.DeepEqual(unnamed(ab.Variants[0].Scenarios), unnamed(ba.Variants[0].Scenarios)) {
		t.Fatalf("scenarios differ with the order of the fleet:\n%+v\n%+v", ab.Variants[0].Scenarios, ba.Variants[0].Scenarios)
	}
	buy := variantScenario(t, ab.Variants[0], "buy", "")
	// battery: 15% of 10 800 000 (life 5) and of 21 000 000 (life 10) in year 4.
	year4, year5 := buy.CashFlow[4], buy.CashFlow[5]
	if year4.BatteryRub != 4_770_000 {
		t.Errorf("battery in year 4 %v, want 1 620 000 + 3 150 000", year4.BatteryRub)
	}
	if year5.ReplacementRub != 0 {
		t.Errorf("the 5 year robot ends at the horizon and is not bought again, got %v", year5.ReplacementRub)
	}
	if strings.Contains(strings.Join(ab.Assumptions, " "), "Срок службы не задан") {
		t.Errorf("both robots have a life, no assumed-life note expected: %v", ab.Assumptions)
	}
}

func TestAssumedLifeIsNamed(t *testing.T) {
	a := robotWith("a", "Первый", 2_700_000, 5, 0.05)
	b := robotWith("b", "Второй", 3_500_000, 0, 0)
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robots:     []Robot{a, b},
		Processes:  warehouseProcessSpecs(),
		Variants: []VariantSpec{{ID: "mix", Name: "Смешанный", Fleet: []FleetSpec{
			{SolutionID: a.ID, Quantity: 4}, {SolutionID: b.ID, Quantity: 6},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(got.Assumptions, "\n")
	if !strings.Contains(text, "Срок службы не задан в ТТХ, принят 7 лет: Второй.") {
		t.Errorf("assumptions %q", text)
	}
	if strings.Contains(text, "Первый.") {
		t.Errorf("the robot with a life must not be listed: %q", text)
	}
}

func TestHorizonLongerThanLife(t *testing.T) {
	r := h1500()
	run := func(horizon float64, life *float64) Result {
		robot := r
		robot.LifetimeYears = life
		got, err := Calculate(Input{ObjectType: objects.Warehouse, Params: paramsWith(t, map[string]any{"payback_horizon_years": horizon}), Robot: &robot})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	seven := 7.0
	long := run(9, &seven)
	buy, raas := scenario(t, long, "buy"), scenario(t, long, "raas")
	if !hasRisk(long, "horizon_over_life") {
		t.Fatalf("risks %+v", long.Risks)
	}
	if got := buy.CashFlow[7].ReplacementRub; got != 35_100_000 {
		t.Errorf("replacement in year 7 %v", got)
	}
	// NOTE: the second purchase is paid in full and its remaining life is not credited back.
	gross := *buy.CapexRub + *buy.OpexYearRub*9
	for _, row := range buy.CashFlow {
		gross += row.BatteryRub + row.ReplacementRub
	}
	if *buy.TcoRub != gross {
		t.Errorf("costs over the horizon %v, want capex + 9 years of opex + batteries + replacements = %v", *buy.TcoRub, gross)
	}
	for _, row := range raas.CashFlow {
		if row.BatteryRub != 0 || row.ReplacementRub != 0 {
			t.Errorf("RaaS carries no batteries or replacements: %+v", row)
		}
	}
	if hasRisk(run(5, &seven), "horizon_over_life") {
		t.Error("horizon 5 inside a 7 year life needs no warning")
	}
	short := 4.0
	if !hasRisk(run(5, &short), "horizon_over_life") {
		t.Error("horizon 5 over a 4 year life needs the warning")
	}
}

func TestPayrollCap(t *testing.T) {
	cases := []struct {
		name            string
		head, wage, tax float64
		want            float64
	}{
		{"under the cap", 2, 100_000, 1.302, 3_124_800},
		{"exactly at the cap", 1, 248_250, 1.302, 3_878_658},
		{"above the cap, reduced rate on the excess", 1, 300_000, 1.302, 4_594_671},
		{"a low factor cannot go below zero contributions", 1, 300_000, 1.0, 3_600_000},
		{"no staff", 0, 300_000, 1.302, 0},
	}
	for _, c := range cases {
		if got := yearFot(c.head, c.wage, c.tax, 1); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRaasFeeTable(t *testing.T) {
	const equipment = 35_100_000.0
	def := parseRaas(nil, DefaultNorms())
	cases := []struct {
		name      string
		tariff    string
		annualOps float64
		volume    float64
		p         raasParams
		fee       float64
		perOp     float64
		skipped   bool
		implied   bool
	}{
		{"fixed", "fixed", 0, 1, def, 10_530_000, 0, false, false},
		{"fixed with its own fraction", "fixed", 0, 1, raasParams{MonthlyFrac: 0.03, MixFixedShare: 0.5}, 12_636_000, 0, false, false},
		{"variable without volume is skipped", "variable", 0, 1, def, 0, 0, true, false},
		{"mixed without volume is skipped", "mixed", 0, 1, def, 0, 0, true, false},
		{"variable takes the rate that reproduces the fixed fee at plan volume", "variable", 365_000, 1, def, 10_530_000, 28.849315068493152, false, true},
		{"variable follows the volume factor", "variable", 438_000, 1.2, def, 12_636_000, 28.849315068493152, false, true},
		{"variable with its own rate", "variable", 365_000, 1, raasParams{MonthlyFrac: 0.025, MixFixedShare: 0.5, PerOpRub: 20}, 7_300_000, 20, false, false},
		{"mixed", "mixed", 365_000, 1, raasParams{MonthlyFrac: 0.025, MixFixedShare: 0.5, PerOpRub: 20}, 8_915_000, 20, false, false},
	}
	for _, c := range cases {
		got := raasFee(c.tariff, equipment, c.annualOps, c.volume, c.p)
		if got.Skipped != c.skipped || got.Fee != c.fee || got.Implied != c.implied || (got.PerOp-c.perOp > 1e-9 || c.perOp-got.PerOp > 1e-9) {
			t.Errorf("%s: %+v", c.name, got)
		}
		if got.Note == "" {
			t.Errorf("%s: the quote must say what it used", c.name)
		}
	}
}

func TestRaasVariableWithoutProcessesIsSkippedWithNote(t *testing.T) {
	r := h1500()
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robots:     []Robot{r},
		Variants: []VariantSpec{{ID: "amr", Name: "AMR", Fleet: []FleetSpec{{SolutionID: r.ID, Quantity: 3}},
			Financing: []FinancingSpec{{Kind: "buy"}, {Kind: "raas", Tariff: "variable"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Variants[0].Scenarios) != 1 {
		t.Fatalf("scenarios %+v", got.Variants[0].Scenarios)
	}
	if !strings.Contains(strings.Join(got.Assumptions, "\n"), "не посчитан") {
		t.Fatalf("no note: %v", got.Assumptions)
	}
}

func TestUnsetDiscountRateFallsBack(t *testing.T) {
	zero := 0.0
	if (AssumptionValues{}).discountRate() != DefaultDiscountRate || (AssumptionValues{DiscountRate: &zero}).discountRate() != DefaultDiscountRate {
		t.Fatal("an unset rate is the default")
	}
	rate := 0.2
	if (AssumptionValues{DiscountRate: &rate}).discountRate() != 0.2 {
		t.Fatal("a set rate stands")
	}
}

func TestDiscountRateMovesNPVNotPayback(t *testing.T) {
	r := h1500()
	lo, hi := 0.05, 0.25
	run := func(rate *float64) Scenario {
		got, err := Calculate(Input{ObjectType: objects.Warehouse, Params: warehouseParams(t), Robot: &r, Assumptions: AssumptionValues{DiscountRate: rate}})
		if err != nil {
			t.Fatal(err)
		}
		return scenario(t, got, "buy")
	}
	a, b := run(&lo), run(&hi)
	if *a.NpvRub <= *b.NpvRub {
		t.Fatalf("npv at 5%% %v must exceed npv at 25%% %v", *a.NpvRub, *b.NpvRub)
	}
	if *a.PaybackYears != *b.PaybackYears || *a.IrrPct != *b.IrrPct {
		t.Fatal("simple payback and IRR do not depend on the discount rate")
	}
}
