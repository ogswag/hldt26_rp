package econ

import (
	"strings"
	"testing"

	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/objects"
)

func TestMixedFleetSharesInfrastructure(t *testing.T) {
	a := h1500()
	priceB := 3_500_000.0
	speed := 1.2
	payload := 50.0
	b := Robot{ID: "piece-1", Name: "Ronavi SR", Family: "AMR", PriceRub: &priceB, SpeedMps: &speed, PayloadKg: &payload}
	qtyA, qtyB := 4, 6
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robots:     []Robot{a, b},
		Processes:  warehouseProcessSpecs(),
		SharedCosts: []SharedCostSpec{
			{Code: "wms", Label: "Интеграция WMS", Bucket: "capex", Rub: 1_000_000},
		},
		Variants: []VariantSpec{{
			ID:   "mix",
			Name: "Смешанный флот",
			Fleet: []FleetSpec{
				{SolutionID: a.ID, Quantity: qtyA, TaskCodes: []string{"inbound", "putaway", "outbound"}},
				{SolutionID: b.ID, Quantity: qtyB, TaskCodes: []string{"piece_pick"}},
			},
			Financing: []FinancingSpec{
				{Kind: "buy"},
				{Kind: "raas", Tariff: "fixed"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Variants) != 1 {
		t.Fatalf("variants %d", len(got.Variants))
	}
	vr := got.Variants[0]
	if len(vr.Fleet) != 2 {
		t.Fatalf("fleet %d", len(vr.Fleet))
	}
	buy := variantScenario(t, vr, "buy", "")
	raas := variantScenario(t, vr, "raas", "fixed")
	if buy.FleetSize == nil || *buy.FleetSize != 10 {
		t.Fatalf("fleet size %v", buy.FleetSize)
	}
	if raas.FleetSize == nil || *raas.FleetSize != *buy.FleetSize {
		t.Fatal("buy and raas must share fleet size")
	}
	if buy.CapexRub == nil || raas.CapexRub == nil || *raas.CapexRub >= *buy.CapexRub {
		t.Fatalf("raas capex %v buy %v", raas.CapexRub, buy.CapexRub)
	}
	wms := 0
	for _, ln := range vr.SharedCosts {
		if ln.ID == "wms" {
			wms++
			if ln.Rub != 1_000_000 {
				t.Fatalf("wms %v", ln.Rub)
			}
		}
	}
	if wms != 1 {
		t.Fatalf("shared wms count %d, want once", wms)
	}
	base := scenario(t, got, "baseline")
	if base.OpexYearRub == nil || *base.OpexYearRub <= 0 {
		t.Fatal("baseline from processes")
	}
}

// Mixed fleet of 4 H1500 and 6 SR: the equipment is 31 800 000, delivery 3% of it is paid by a buyer only, ten robots
// need one technician a shift on two shifts, and stations are counted per line (1 + 2) since neither robot has specs.
func TestVariantCarriesDeliveryStaffAndStations(t *testing.T) {
	a := h1500()
	priceB := 3_500_000.0
	speed := 1.2
	payload := 50.0
	b := Robot{ID: "piece-1", Name: "Ronavi SR", Family: "AMR", PriceRub: &priceB, SpeedMps: &speed, PayloadKg: &payload}
	got, err := Calculate(Input{
		ObjectType:  objects.Warehouse,
		Params:      warehouseParams(t),
		Robots:      []Robot{a, b},
		Processes:   warehouseProcessSpecs(),
		SharedCosts: []SharedCostSpec{{Code: "wms", Label: "Интеграция WMS", Bucket: "capex", Rub: 1_000_000}},
		Variants: []VariantSpec{{
			ID: "mix", Name: "Смешанный флот",
			Fleet: []FleetSpec{
				{SolutionID: a.ID, Quantity: 4, TaskCodes: []string{"inbound", "putaway", "outbound"}},
				{SolutionID: b.ID, Quantity: 6, TaskCodes: []string{"piece_pick"}},
			},
			Financing: []FinancingSpec{{Kind: "buy"}, {Kind: "raas", Tariff: "fixed"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	vr := got.Variants[0]
	buy, raas := variantScenario(t, vr, "buy", ""), variantScenario(t, vr, "raas", "fixed")
	// (31 800 000 + 954 000 + 14 674 000 of shared lines) x 1,10, and without equipment and delivery for RaaS.
	if *buy.CapexRub != 52_170_800 || *raas.CapexRub != 16_141_400 {
		t.Fatalf("capex buy %v, raas %v", *buy.CapexRub, *raas.CapexRub)
	}
	if vr.Chargers != 3 || got.Shared == nil || got.Shared.Chargers != 3 {
		t.Fatalf("stations %d, shared %+v", vr.Chargers, got.Shared)
	}
	lines := map[string]float64{}
	for _, l := range got.Breakdown {
		if l.Scenario == "mix:buy" {
			lines[l.ID] = l.Rub
		}
		if l.Scenario == "mix:raas:fixed" && l.ID == "delivery" {
			t.Fatal("RaaS carries no delivery line")
		}
	}
	for id, want := range map[string]float64{"delivery": 954_000, "communications": 120_000, "technicians": 2_812_320} {
		if lines[id] != want {
			t.Errorf("%s %v, want %v", id, lines[id], want)
		}
	}
}

func TestSharedOpexAppliesToEveryFinancingScenario(t *testing.T) {
	r := h1500()
	base := Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robots:     []Robot{r},
		Processes:  warehouseProcessSpecs(),
		Variants: []VariantSpec{{
			ID: "amr", Name: "AMR",
			Fleet:     []FleetSpec{{SolutionID: r.ID, Quantity: 2, TaskCodes: []string{"inbound"}}},
			Financing: []FinancingSpec{{Kind: "buy"}, {Kind: "raas", Tariff: "fixed"}},
		}},
	}
	without, err := Calculate(base)
	if err != nil {
		t.Fatal(err)
	}
	base.SharedCosts = []SharedCostSpec{{Code: "support", Label: "Поддержка", Bucket: "opex", Rub: 300_000}}
	with, err := Calculate(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []struct{ kind, tariff string }{{"buy", ""}, {"raas", "fixed"}} {
		a := variantScenario(t, without.Variants[0], key.kind, key.tariff)
		b := variantScenario(t, with.Variants[0], key.kind, key.tariff)
		if a.OpexYearRub == nil || b.OpexYearRub == nil || *b.OpexYearRub-*a.OpexYearRub != 300_000 {
			t.Fatalf("%s %s shared opex delta: before=%v after=%v", key.kind, key.tariff, a.OpexYearRub, b.OpexYearRub)
		}
	}
}

func TestSharedUsesFirstCalculatedVariant(t *testing.T) {
	r := h1500()
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robots:     []Robot{r},
		Processes:  warehouseProcessSpecs(),
		Variants: []VariantSpec{
			{ID: "empty", Name: "Пустой", Financing: []FinancingSpec{{Kind: "buy"}}},
			{ID: "amr", Name: "AMR", Fleet: []FleetSpec{{SolutionID: r.ID, Quantity: 3}}, Financing: []FinancingSpec{{Kind: "buy"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Shared == nil || got.Shared.FleetSize != 3 || got.Shared.Throughput <= 0 || got.Shared.PeakOps <= 0 {
		t.Fatalf("shared %+v", got.Shared)
	}
}

func TestBuyAndRaasDoNotDuplicateFleet(t *testing.T) {
	r := h1500()
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robots:     []Robot{r},
		Processes:  warehouseProcessSpecs(),
		Variants: []VariantSpec{{
			ID:   "amr",
			Name: "AMR комплектация",
			Fleet: []FleetSpec{
				{SolutionID: r.ID, Quantity: 8, TaskCodes: []string{"inbound", "putaway", "outbound"}},
			},
			Financing: []FinancingSpec{
				{Kind: "buy"},
				{Kind: "raas", Tariff: "fixed"},
				{Kind: "raas", Tariff: "variable"},
				{Kind: "raas", Tariff: "mixed"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	vr := got.Variants[0]
	if len(vr.Fleet) != 1 || vr.Fleet[0].Quantity != 8 {
		t.Fatalf("tech fleet %+v", vr.Fleet)
	}
	if len(vr.Scenarios) != 4 {
		t.Fatalf("financing %d", len(vr.Scenarios))
	}
	buy := variantScenario(t, vr, "buy", "")
	fixed := variantScenario(t, vr, "raas", "fixed")
	variable := variantScenario(t, vr, "raas", "variable")
	mixed := variantScenario(t, vr, "raas", "mixed")
	if *buy.FleetSize != 8 || *fixed.FleetSize != 8 || *variable.FleetSize != 8 || *mixed.FleetSize != 8 {
		t.Fatal("financing must not copy fleet")
	}
	if *variable.OpexYearRub != *fixed.OpexYearRub {
		t.Fatalf("volume=1 variable %v fixed %v", *variable.OpexYearRub, *fixed.OpexYearRub)
	}
	if *mixed.OpexYearRub != *fixed.OpexYearRub {
		t.Fatalf("volume=1 mixed %v", *mixed.OpexYearRub)
	}
}

func TestThreeRaasTariffsDivergeWithVolume(t *testing.T) {
	r := h1500()
	vol := 2.0
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robots:     []Robot{r},
		Overrides:  Overrides{VolumeFactor: &vol},
		Processes:  warehouseProcessSpecs(),
		Variants: []VariantSpec{{
			ID:   "amr",
			Name: "AMR",
			Fleet: []FleetSpec{
				{SolutionID: r.ID, Quantity: 8, TaskCodes: []string{"inbound"}},
			},
			Financing: []FinancingSpec{
				{Kind: "raas", Tariff: "fixed"},
				{Kind: "raas", Tariff: "variable"},
				{Kind: "raas", Tariff: "mixed"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	vr := got.Variants[0]
	fixed := variantScenario(t, vr, "raas", "fixed")
	variable := variantScenario(t, vr, "raas", "variable")
	mixed := variantScenario(t, vr, "raas", "mixed")
	if *variable.OpexYearRub <= *fixed.OpexYearRub {
		t.Fatalf("variable should exceed fixed at volume 2: %v vs %v", *variable.OpexYearRub, *fixed.OpexYearRub)
	}
	if *mixed.OpexYearRub <= *fixed.OpexYearRub || *mixed.OpexYearRub >= *variable.OpexYearRub {
		t.Fatalf("mixed should sit between fixed and variable: f=%v m=%v v=%v", *fixed.OpexYearRub, *mixed.OpexYearRub, *variable.OpexYearRub)
	}
}

func TestProjectPriceOverrideSource(t *testing.T) {
	r := h1500()
	ov := 3_000_000.0
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robots:     []Robot{r},
		Processes:  warehouseProcessSpecs(),
		Variants: []VariantSpec{{
			ID:   "amr",
			Name: "AMR",
			Fleet: []FleetSpec{{
				SolutionID: r.ID, Quantity: 2, TaskCodes: []string{"inbound"},
				PriceOverrideRub: &ov, PriceOverrideReason: "КП поставщика 2026-09-01",
			}},
			Financing: []FinancingSpec{{Kind: "buy"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := got.Variants[0].Fleet[0]
	if item.PriceSource != PriceSourceOverride {
		t.Fatalf("source %q", item.PriceSource)
	}
	if item.ProjectPriceRub == nil || *item.ProjectPriceRub != ov {
		t.Fatalf("project price %v", item.ProjectPriceRub)
	}
	if item.CatalogPriceRub == nil || *item.CatalogPriceRub != 2_700_000 {
		t.Fatalf("catalog %v", item.CatalogPriceRub)
	}
	if item.PriceOverrideReason != "КП поставщика 2026-09-01" {
		t.Fatalf("reason %q", item.PriceOverrideReason)
	}
}

func TestVATRecoverableLowersCapex(t *testing.T) {
	r := h1500()
	vat := true
	rate := 0.20
	in := Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robot:      &r,
		PickReason: "recommended",
	}
	gross, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Assumptions = AssumptionValues{PricesIncludeVAT: &vat, VATRate: &rate, VATRecoverable: true}
	net, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	g := scenario(t, gross, "buy")
	n := scenario(t, net, "buy")
	if g.CapexRub == nil || n.CapexRub == nil || *n.CapexRub >= *g.CapexRub {
		t.Fatalf("vat recoverable capex %v vs %v", n.CapexRub, g.CapexRub)
	}
}

func TestLaborCashShareCutsSavings(t *testing.T) {
	r := h1500()
	full, err := Calculate(Input{ObjectType: objects.Warehouse, Params: warehouseParams(t), Robot: &r})
	if err != nil {
		t.Fatal(err)
	}
	share := 0.5
	half, err := Calculate(Input{
		ObjectType:  objects.Warehouse,
		Params:      warehouseParams(t),
		Robot:       &r,
		Assumptions: AssumptionValues{LaborCashShare: &share},
	})
	if err != nil {
		t.Fatal(err)
	}
	fb := scenario(t, full, "buy")
	hb := scenario(t, half, "buy")
	if hb.OpexYearRub == nil || fb.OpexYearRub == nil || *hb.OpexYearRub <= *fb.OpexYearRub {
		t.Fatalf("half cash share should leave more opex: %v vs %v", hb.OpexYearRub, fb.OpexYearRub)
	}
	if hb.AnnualEffectRub == nil || fb.AnnualEffectRub == nil || *hb.AnnualEffectRub >= *fb.AnnualEffectRub {
		t.Fatalf("half cash share should cut cash effect: %v vs %v", hb.AnnualEffectRub, fb.AnnualEffectRub)
	}
}

func TestThreeVariantsShareBaseline(t *testing.T) {
	r := h1500()
	priceB := 4_000_000.0
	b := Robot{ID: "stack-1", Name: "Stacker", Family: "штабелёр", PriceRub: &priceB, PayloadKg: fptr(1500)}
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robots:     []Robot{r, b},
		Processes:  warehouseProcessSpecs(),
		Variants: []VariantSpec{
			{ID: "v1", Name: "AMR", Fleet: []FleetSpec{{SolutionID: r.ID, Quantity: 5, TaskCodes: []string{"inbound"}}}, Financing: []FinancingSpec{{Kind: "buy"}}},
			{ID: "v2", Name: "Смешанный", Fleet: []FleetSpec{{SolutionID: r.ID, Quantity: 3, TaskCodes: []string{"inbound"}}, {SolutionID: b.ID, Quantity: 2, TaskCodes: []string{"putaway"}}}, Financing: []FinancingSpec{{Kind: "buy"}}},
			{ID: "v3", Name: "Штабелёр", Fleet: []FleetSpec{{SolutionID: b.ID, Quantity: 4, TaskCodes: []string{"putaway"}}}, Financing: []FinancingSpec{{Kind: "buy"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Variants) != 3 {
		t.Fatalf("variants %d", len(got.Variants))
	}
	base := scenario(t, got, "baseline")
	if base.OpexYearRub == nil {
		t.Fatal("baseline")
	}
	for _, vr := range got.Variants {
		buy := variantScenario(t, vr, "buy", "")
		if buy.AnnualEffectRub == nil {
			t.Fatalf("%s no effect", vr.Name)
		}
		if buy.VariantID != vr.VariantID {
			t.Fatalf("variant id %s %s", buy.VariantID, vr.VariantID)
		}
	}
}

func TestVariantRisksNameEveryNotRecommendedRobot(t *testing.T) {
	r := h1500()
	priceB := 4_000_000.0
	b := Robot{ID: "stack-1", Name: "Stacker", Family: "штабелёр", PriceRub: &priceB, PayloadKg: fptr(1500)}
	priceC := 3_000_000.0
	c := Robot{ID: "amr-2", Name: "Ronavi C", Family: "AMR", PriceRub: &priceC, PayloadKg: fptr(500)}
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robots:     []Robot{r, b, c},
		Processes:  warehouseProcessSpecs(),
		Match: matching.Output{Items: []matching.Item{
			{SolutionID: r.ID, Name: r.Name, Status: matching.StatusRecommended},
			{SolutionID: b.ID, Name: b.Name, Status: matching.StatusExcluded, Reasons: []string{"Не проходит по ширине прохода."}},
			{SolutionID: c.ID, Name: c.Name, Status: matching.StatusNeedsReview, Reasons: []string{"Нет ширины прохода."}},
		}},
		Variants: []VariantSpec{
			{ID: "v1", Name: "Вариант 1", Fleet: []FleetSpec{{SolutionID: r.ID, Quantity: 5, TaskCodes: []string{"inbound"}}}, Financing: []FinancingSpec{{Kind: "buy"}}},
			{ID: "v2", Name: "Вариант 2", Fleet: []FleetSpec{{SolutionID: r.ID, Quantity: 3, TaskCodes: []string{"inbound"}}, {SolutionID: b.ID, Quantity: 2, TaskCodes: []string{"putaway"}}}, Financing: []FinancingSpec{{Kind: "buy"}}},
			{ID: "v3", Name: "Вариант 3", Fleet: []FleetSpec{{SolutionID: c.ID, Quantity: 4, TaskCodes: []string{"inbound"}}}, Financing: []FinancingSpec{{Kind: "buy"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	texts := map[string]string{}
	for _, rk := range got.Risks {
		if strings.HasPrefix(rk.ID, "pick_") {
			texts[rk.ID] = rk.Text
		}
	}
	if len(texts) != 2 {
		t.Fatalf("want two pick risks, got %v", texts)
	}
	if want := "Вариант 2, Stacker: подбор исключил робота, но он взят в расчёт. Не проходит по ширине прохода."; texts["pick_excluded:v2:stack-1"] != want {
		t.Errorf("excluded risk %q, want %q", texts["pick_excluded:v2:stack-1"], want)
	}
	if want := "Вариант 3, Ronavi C: подбор требует проверки. Нет ширины прохода."; texts["pick_needs_review:v3:amr-2"] != want {
		t.Errorf("needs_review risk %q, want %q", texts["pick_needs_review:v3:amr-2"], want)
	}
}

func TestCompareRejectsMoreThanSix(t *testing.T) {
	vars := make([]VariantSpec, 7)
	for i := range vars {
		vars[i] = VariantSpec{ID: "v", Name: "x", Financing: []FinancingSpec{{Kind: "buy"}}}
	}
	_, err := Calculate(Input{ObjectType: objects.Warehouse, Params: warehouseParams(t), Variants: vars})
	var ie *InputError
	if err == nil {
		t.Fatal("expected cap error")
	}
	if !asInput(err, &ie) {
		t.Fatalf("err %v", err)
	}
}

func warehouseProcessSpecs() []ProcessSpec {
	return []ProcessSpec{
		{Code: "inbound", Name: "Приёмка", TaskType: "pallet_inbound", IsBaseline: true, UnitsPerDay: 1000, StaffHeadcount: 8, StaffRole: "приёмка"},
		{Code: "putaway", Name: "Размещение", TaskType: "pallet_putaway", IsBaseline: true, UnitsPerDay: 1000, StaffHeadcount: 6, StaffRole: "погрузчик"},
		{Code: "piece_pick", Name: "Отбор", TaskType: "piece_pick", IsBaseline: true, UnitsPerDay: 100000, StaffHeadcount: 100, StaffRole: "отборщик"},
		{Code: "outbound", Name: "Отгрузка", TaskType: "pallet_outbound", IsBaseline: true, UnitsPerDay: 1000, StaffHeadcount: 10, StaffRole: "отгрузка"},
	}
}

func variantScenario(t *testing.T, vr VariantResult, kind, tariff string) Scenario {
	t.Helper()
	for _, sc := range vr.Scenarios {
		if sc.Kind != kind {
			continue
		}
		if kind == "buy" || sc.Tariff == tariff {
			return sc
		}
	}
	t.Fatalf("no %s %s in %+v", kind, tariff, vr.Scenarios)
	return Scenario{}
}

func asInput(err error, ie **InputError) bool {
	if err == nil {
		return false
	}
	x, ok := err.(*InputError)
	if !ok {
		return false
	}
	*ie = x
	return true
}

func TestSensitivityRunsForEveryVariantWithFleet(t *testing.T) {
	r := h1500()
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robots:     []Robot{r},
		Processes:  warehouseProcessSpecs(),
		Variants: []VariantSpec{
			{ID: "a", Name: "Первый", Fleet: []FleetSpec{{SolutionID: r.ID, Quantity: 4, TaskCodes: []string{"inbound"}}}, Financing: []FinancingSpec{{Kind: "buy"}, {Kind: "raas", Tariff: "fixed"}}},
			{ID: "b", Name: "Второй", Fleet: []FleetSpec{{SolutionID: r.ID, Quantity: 6, TaskCodes: []string{"inbound"}}}, Financing: []FinancingSpec{{Kind: "buy"}, {Kind: "raas", Tariff: "fixed"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sensitivity) != 12 {
		t.Fatalf("rows %d, want 12", len(got.Sensitivity))
	}
	var volumeUp *SensitivityRow
	for i, row := range got.Sensitivity {
		if row.VariantID == "" || row.VariantName == "" {
			t.Fatalf("row %d missing variant", i)
		}
		if row.Buy.Kind != "buy" {
			t.Fatalf("row %d buy %q", i, row.Buy.Kind)
		}
		if row.Param == "volume" && row.DeltaPct == 20 && row.VariantID == "a" {
			volumeUp = &got.Sensitivity[i]
		}
	}
	if volumeUp == nil || volumeUp.Buy.FleetSize == nil || *volumeUp.Buy.FleetSize != 5 {
		t.Fatalf("volume +20 fleet %+v", volumeUp)
	}
	found := false
	for _, a := range got.Assumptions {
		if strings.Contains(a, "сдвиг объёма меняет и флот") {
			found = true
		}
	}
	if !found {
		t.Fatal("missing volume-fleet assumption")
	}
}
