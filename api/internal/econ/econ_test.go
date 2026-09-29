package econ

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/objects"
)

func TestEmptyResultShapeFrozen(t *testing.T) {
	got, err := json.Marshal(EmptyResult("warehouse", DefaultSeed))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model_version":"econ-v3","object_type":"warehouse","seed":0,"scenarios":[{"kind":"baseline","solution_id":null,"fleet_size":null,"capex_rub":null,"opex_year_rub":null,"annual_effect_rub":null,"payback_years":null,"roi_pct":null,"tco_rub":null},{"kind":"buy","solution_id":null,"fleet_size":null,"capex_rub":null,"opex_year_rub":null,"annual_effect_rub":null,"payback_years":null,"roi_pct":null,"tco_rub":null},{"kind":"raas","solution_id":null,"fleet_size":null,"capex_rub":null,"opex_year_rub":null,"annual_effect_rub":null,"payback_years":null,"roi_pct":null,"tco_rub":null}],"match":{"weights":{"fit":0.4,"process_match":0.25,"data_quality":0.2,"price_band":0.15},"selected_ids":[],"items":[],"match_version":"match-v4"},"assumptions":[],"verification_flag":false}`
	if string(got) != want {
		t.Fatalf("shape\ngot  %s\nwant %s", got, want)
	}
}

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

func h1500() Robot {
	price := 2_700_000.0
	speed := 1.5
	width := 654.0
	payload := 1500.0
	return Robot{
		ID:        "5760e938-9a43-45a7-b8e8-f4f2e6383930",
		Name:      "Ronavi H1500",
		Family:    "AMR",
		PriceRub:  &price,
		SpeedMps:  &speed,
		WidthMm:   &width,
		PayloadKg: &payload,
	}
}

func scenario(t *testing.T, res Result, kind string) Scenario {
	t.Helper()
	for _, sc := range res.Scenarios {
		if sc.Kind == kind {
			return sc
		}
	}
	t.Fatalf("no scenario %s", kind)
	return Scenario{}
}

func TestWarehouseH1500Golden(t *testing.T) {
	r := h1500()
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robot:      &r,
		PickReason: "recommended",
		Seed:       0,
	})
	if err != nil {
		t.Fatal(err)
	}
	buy := scenario(t, got, "buy")
	raas := scenario(t, got, "raas")
	base := scenario(t, got, "baseline")
	if buy.FleetSize == nil || *buy.FleetSize != 13 {
		t.Fatalf("fleet %v", buy.FleetSize)
	}
	if buy.CapexRub == nil || *buy.CapexRub != 56_370_600 {
		t.Fatalf("buy capex %v", buy.CapexRub)
	}
	checks := []struct {
		name string
		got  *float64
		want float64
	}{
		{"buy opex", buy.OpexYearRub, 29_994_510},
		{"buy cash effect", buy.AnnualEffectRub, 16_877_490},
		{"buy payback", buy.PaybackYears, 3.4941},
		{"buy discounted payback", buy.DiscountedPaybackYears, 5.3844},
		{"buy npv", buy.NpvRub, -2_804_917},
		{"buy irr", buy.IrrPct, 12.82},
		{"buy roi", buy.RoiPct, 40.36},
		{"buy accounting effect", buy.AccountingEffectRub, 9_707_061},
		{"buy tco", buy.TcoRub, 211_608_150},
		{"raas opex", raas.OpexYearRub, 36_676_510},
		{"raas cash effect", raas.AnnualEffectRub, 10_195_490},
		{"raas payback", raas.PaybackYears, 1.6284},
		{"raas npv", raas.NpvRub, 17_574_564},
		{"raas tco", raas.TcoRub, 199_984_850},
	}
	for _, c := range checks {
		if c.got == nil || *c.got != c.want {
			t.Errorf("%s got %v want %v", c.name, derefFloat(c.got), c.want)
		}
	}
	if raas.CapexRub == nil || *raas.CapexRub != 16_602_300 {
		t.Fatalf("raas capex %v", raas.CapexRub)
	}
	if raas.CapexRub != nil && buy.CapexRub != nil && *raas.CapexRub >= *buy.CapexRub {
		t.Fatal("raas capex should be below buy")
	}
	if base.CapexRub == nil || *base.CapexRub != 0 {
		t.Fatalf("baseline capex %v", base.CapexRub)
	}
	if base.OpexYearRub == nil || *base.OpexYearRub != 46_872_000 {
		t.Fatalf("baseline opex %v", base.OpexYearRub)
	}
	if base.PaybackYears != nil || base.RoiPct != nil {
		t.Fatalf("baseline payback/roi %+v", base)
	}
	if got.Shared == nil || got.Shared.WorkKind != WorkPallet {
		t.Fatalf("shared %+v", got.Shared)
	}
	if len(got.Sensitivity) != 6 {
		t.Fatalf("sensitivity %d", len(got.Sensitivity))
	}
	if got.VerificationFlag {
		t.Fatal("verification")
	}
	if len(got.Formulas) == 0 || len(got.Sources) == 0 || len(got.Breakdown) == 0 {
		t.Fatal("missing formulas/sources/breakdown")
	}
	if buy.PaybackBand != BandFrom3To5 {
		t.Fatalf("buy band %q", buy.PaybackBand)
	}
	if raas.PaybackBand == "" {
		t.Fatal("raas band empty")
	}
	if !hasRisk(got, "simple_payback") || !hasRisk(got, "raas_fee") {
		t.Fatalf("risks %+v", got.Risks)
	}
	if len(got.Interpretation) < 3 {
		t.Fatalf("interpretation %v", got.Interpretation)
	}
	if got.HorizonYears != 5 {
		t.Fatalf("horizon %v", got.HorizonYears)
	}
	if got.SolutionName != "Ronavi H1500" {
		t.Fatalf("name %q", got.SolutionName)
	}
	joined := ""
	for _, s := range got.Interpretation {
		joined += s
	}
	if !strings.Contains(joined, "до 3 лет") || !strings.Contains(joined, "не решение go/no-go") {
		t.Fatalf("interpretation %v", got.Interpretation)
	}
	if !strings.Contains(joined, "ROI за горизонт") || !strings.Contains(joined, "NPV") {
		t.Fatalf("interpretation missing ROI and NPV %v", got.Interpretation)
	}
}

func TestWarehouseReferenceFixtureMatchesGolden(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "data", "fixtures", "warehouse-reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Params   json.RawMessage `json:"params"`
		Seed     int             `json:"seed"`
		Expected struct {
			WorkKind  string `json:"work_kind"`
			FleetSize int    `json:"fleet_size"`
			Buy       struct {
				CapexRub               float64 `json:"capex_rub"`
				OpexYearRub            float64 `json:"opex_year_rub"`
				AnnualEffectRub        float64 `json:"annual_effect_rub"`
				PaybackYears           float64 `json:"payback_years"`
				DiscountedPaybackYears float64 `json:"discounted_payback_years"`
				NpvRub                 float64 `json:"npv_rub"`
				IrrPct                 float64 `json:"irr_pct"`
				RoiPct                 float64 `json:"roi_pct"`
				AccountingEffectRub    float64 `json:"accounting_effect_rub"`
				TcoRub                 float64 `json:"tco_rub"`
			} `json:"buy"`
		} `json:"expected"`
	}
	if err := json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	r := h1500()
	got, err := Calculate(Input{ObjectType: objects.Warehouse, Params: fixture.Params, Robot: &r, PickReason: "recommended", Seed: fixture.Seed})
	if err != nil {
		t.Fatal(err)
	}
	buy := scenario(t, got, "buy")
	if got.Shared == nil || got.Shared.WorkKind != fixture.Expected.WorkKind || got.Shared.FleetSize != fixture.Expected.FleetSize {
		t.Fatalf("shared got %+v expected %+v", got.Shared, fixture.Expected)
	}
	checks := []struct {
		name string
		got  *float64
		want float64
	}{
		{"capex", buy.CapexRub, fixture.Expected.Buy.CapexRub},
		{"opex", buy.OpexYearRub, fixture.Expected.Buy.OpexYearRub},
		{"cash effect", buy.AnnualEffectRub, fixture.Expected.Buy.AnnualEffectRub},
		{"payback", buy.PaybackYears, fixture.Expected.Buy.PaybackYears},
		{"discounted payback", buy.DiscountedPaybackYears, fixture.Expected.Buy.DiscountedPaybackYears},
		{"npv", buy.NpvRub, fixture.Expected.Buy.NpvRub},
		{"irr", buy.IrrPct, fixture.Expected.Buy.IrrPct},
		{"roi", buy.RoiPct, fixture.Expected.Buy.RoiPct},
		{"accounting effect", buy.AccountingEffectRub, fixture.Expected.Buy.AccountingEffectRub},
		{"tco", buy.TcoRub, fixture.Expected.Buy.TcoRub},
	}
	for _, check := range checks {
		if check.got == nil || *check.got != check.want {
			t.Fatalf("%s got %v want %v", check.name, check.got, check.want)
		}
	}
}

func TestNoRobotFillsBaselineOnly(t *testing.T) {
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		PickReason: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	buy := scenario(t, got, "buy")
	base := scenario(t, got, "baseline")
	if buy.CapexRub != nil {
		t.Fatalf("buy should be empty %+v", buy)
	}
	if base.OpexYearRub == nil || *base.OpexYearRub <= 0 {
		t.Fatalf("baseline opex %v", base.OpexYearRub)
	}
	if got.Shared != nil {
		t.Fatalf("shared %+v", got.Shared)
	}
	if !hasRisk(got, "no_robot") {
		t.Fatalf("risks %+v", got.Risks)
	}
}

func TestNoPriceSkipsBuy(t *testing.T) {
	r := h1500()
	r.PriceRub = nil
	got, err := Calculate(Input{ObjectType: objects.Warehouse, Params: warehouseParams(t), Robot: &r})
	if err != nil {
		t.Fatal(err)
	}
	if scenario(t, got, "buy").CapexRub != nil {
		t.Fatal("expected empty buy")
	}
	if !hasRisk(got, "no_price") {
		t.Fatalf("risks %+v", got.Risks)
	}
}

func TestOverrideFleet(t *testing.T) {
	r := h1500()
	n := 5
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robot:      &r,
		Overrides:  Overrides{FleetSize: &n},
	})
	if err != nil {
		t.Fatal(err)
	}
	buy := scenario(t, got, "buy")
	if buy.FleetSize == nil || *buy.FleetSize != 5 {
		t.Fatalf("fleet %v", buy.FleetSize)
	}
	if *buy.CapexRub != 21_681_000 {
		t.Fatalf("capex %v", *buy.CapexRub)
	}
	found := false
	for _, o := range got.Overrides {
		if o.Field == "fleet_size" && o.Flag && o.Value == 5 && o.Computed == 13 {
			found = true
		}
	}
	if !found {
		t.Fatalf("override flag %+v", got.Overrides)
	}
}

func TestVolumeFactorChangesFleet(t *testing.T) {
	r := h1500()
	low := 0.5
	high := 2.0
	a, err := Calculate(Input{ObjectType: objects.Warehouse, Params: warehouseParams(t), Robot: &r, Overrides: Overrides{VolumeFactor: &low}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Calculate(Input{ObjectType: objects.Warehouse, Params: warehouseParams(t), Robot: &r, Overrides: Overrides{VolumeFactor: &high}})
	if err != nil {
		t.Fatal(err)
	}
	if *scenario(t, a, "buy").FleetSize >= *scenario(t, b, "buy").FleetSize {
		t.Fatalf("volume fleet %d vs %d", *scenario(t, a, "buy").FleetSize, *scenario(t, b, "buy").FleetSize)
	}
}

func TestPaybackNoneIfEffectNotPositive(t *testing.T) {
	r := h1500()
	price := 1e12
	r.PriceRub = &price
	got, err := Calculate(Input{ObjectType: objects.Warehouse, Params: warehouseParams(t), Robot: &r})
	if err != nil {
		t.Fatal(err)
	}
	buy := scenario(t, got, "buy")
	if buy.PaybackYears != nil {
		t.Fatalf("payback %v", buy.PaybackYears)
	}
	if buy.PaybackBand != BandNone {
		t.Fatalf("band %q", buy.PaybackBand)
	}
	if buy.AnnualEffectRub == nil || *buy.AnnualEffectRub > 0 {
		t.Fatalf("effect %v", buy.AnnualEffectRub)
	}
	if !hasRisk(got, "no_effect") {
		t.Fatalf("risks %+v", got.Risks)
	}
}

func TestPieceUsesPickLines(t *testing.T) {
	price := 2_000_000.0
	payload := 50.0
	r := Robot{ID: "x", Name: "Ronavi SR", Family: "AMR", PriceRub: &price, PayloadKg: &payload}
	got, err := Calculate(Input{ObjectType: objects.Warehouse, Params: warehouseParams(t), Robot: &r})
	if err != nil {
		t.Fatal(err)
	}
	if got.Shared == nil || got.Shared.WorkKind != WorkPiece {
		t.Fatalf("kind %+v", got.Shared)
	}
	if got.Shared.PeakOps < 1000 {
		t.Fatalf("peak %v", got.Shared.PeakOps)
	}
	base := scenario(t, got, "baseline")
	if base.OpexYearRub == nil || *base.OpexYearRub != 156_240_000 {
		t.Fatalf("picker fot %v", base.OpexYearRub)
	}
}

func TestCleanerUsesArea(t *testing.T) {
	price := 1_000_000.0
	speed := 1.11
	width := 610.0
	r := Robot{ID: "c", Name: "MARK 2 SE", Family: "Клининг", PriceRub: &price, SpeedMps: &speed, WidthMm: &width}
	got, err := Calculate(Input{ObjectType: objects.Warehouse, Params: warehouseParams(t), Robot: &r})
	if err != nil {
		t.Fatal(err)
	}
	if got.Shared == nil || got.Shared.WorkKind != WorkCleaner || got.Shared.Unit != "м²/ч" {
		t.Fatalf("shared %+v", got.Shared)
	}
}

func TestHospitalCart(t *testing.T) {
	def, err := objects.Defaults(objects.Hospital)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	price := 800_000.0
	speed := 1.2
	payload := 10.0
	r := Robot{ID: "h", Name: "PuduBot 2", Family: "AMR", PriceRub: &price, SpeedMps: &speed, PayloadKg: &payload}
	got, err := Calculate(Input{ObjectType: objects.Hospital, Params: raw, Robot: &r})
	if err != nil {
		t.Fatal(err)
	}
	if got.Shared == nil || got.Shared.WorkKind != WorkHospitalCart {
		t.Fatalf("shared %+v", got.Shared)
	}
	if *scenario(t, got, "buy").FleetSize < 1 {
		t.Fatal("fleet")
	}
}

func TestAirportRamp(t *testing.T) {
	def, err := objects.Defaults(objects.Airport)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	price := 5_000_000.0
	speed := 6.94
	payload := 3000.0
	r := Robot{ID: "a", Name: "Cognitive Pilot tug", Family: "AMR", PriceRub: &price, SpeedMps: &speed, PayloadKg: &payload}
	got, err := Calculate(Input{ObjectType: objects.Airport, Params: raw, Robot: &r})
	if err != nil {
		t.Fatal(err)
	}
	if got.Shared == nil || got.Shared.WorkKind != WorkAirportRamp {
		t.Fatalf("shared %+v", got.Shared)
	}
}

func TestIntegrationWithoutWMS(t *testing.T) {
	def, err := objects.Defaults(objects.Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	def["has_wms"] = false
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	r := h1500()
	got, err := Calculate(Input{ObjectType: objects.Warehouse, Params: raw, Robot: &r})
	if err != nil {
		t.Fatal(err)
	}
	if *scenario(t, got, "buy").CapexRub <= 55_212_300 {
		t.Fatalf("expected higher capex without WMS, got %v", *scenario(t, got, "buy").CapexRub)
	}
}

func TestBadVolumeFactor(t *testing.T) {
	z := 0.0
	_, err := Calculate(Input{ObjectType: objects.Warehouse, Params: warehouseParams(t), Overrides: Overrides{VolumeFactor: &z}})
	var ie *InputError
	if !errors.As(err, &ie) {
		t.Fatalf("err %v", err)
	}
}

func TestPickSolutionOrder(t *testing.T) {
	out := matching.Output{
		SelectedIDs: []string{"sel"},
		Items: []matching.Item{
			{SolutionID: "rec", Status: matching.StatusRecommended},
			{SolutionID: "sel", Status: matching.StatusNeedsReview},
		},
	}
	id, reason := PickSolution(out)
	if id != "sel" || reason != "selected" {
		t.Fatalf("%s %s", id, reason)
	}
	out.SelectedIDs = nil
	id, reason = PickSolution(out)
	if id != "rec" || reason != "recommended" {
		t.Fatalf("%s %s", id, reason)
	}
}

func TestPaybackBandThresholds(t *testing.T) {
	if paybackBand(nil) != BandNone {
		t.Fatal("nil")
	}
	if paybackBand(fptr(3)) != BandUpTo3 {
		t.Fatal("3")
	}
	if paybackBand(fptr(3.0001)) != BandFrom3To5 {
		t.Fatal("just over 3")
	}
	if paybackBand(fptr(5)) != BandFrom3To5 {
		t.Fatal("5")
	}
	if paybackBand(fptr(5.0001)) != BandOver5 {
		t.Fatal("over 5")
	}
}

func TestFormatRub(t *testing.T) {
	if formatRub(55_212_300) != "55\u00a0212\u00a0300\u00a0₽" {
		t.Fatalf("got %q", formatRub(55_212_300))
	}
	if formatRub(-12) != "-12\u00a0₽" {
		t.Fatalf("neg %q", formatRub(-12))
	}
}

func TestFormatYears(t *testing.T) {
	cases := map[float64]string{
		1: "1 год", 2: "2 года", 4: "4 года", 5: "5 лет", 11: "11 лет", 12: "12 лет",
		21: "21 год", 22: "22 года", 2.5: "2,5 года", 0.5: "0,5 года", 3.04: "3 года", 1.96: "2 года",
	}
	for in, want := range cases {
		if got := formatYears(in); got != want {
			t.Errorf("formatYears(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestNotRecommendedRisk(t *testing.T) {
	r := h1500()
	got, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Robot:      &r,
		PickReason: "needs_review",
		Match: matching.Output{
			Items: []matching.Item{{
				SolutionID: r.ID,
				Status:     matching.StatusNeedsReview,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasRisk(got, "not_recommended") || !hasRisk(got, "missing_ttx") {
		t.Fatalf("risks %+v", got.Risks)
	}
}

func hasRisk(res Result, id string) bool {
	for _, r := range res.Risks {
		if r.ID == id {
			return true
		}
	}
	return false
}

func TestSensitivityPriceMovesCapex(t *testing.T) {
	r := h1500()
	got, err := Calculate(Input{ObjectType: objects.Warehouse, Params: warehouseParams(t), Robot: &r})
	if err != nil {
		t.Fatal(err)
	}
	var down, up float64
	for _, row := range got.Sensitivity {
		if row.Param != "equipment_price" {
			continue
		}
		if row.Buy.CapexRub == nil {
			t.Fatal("capex")
		}
		if row.DeltaPct < 0 {
			down = *row.Buy.CapexRub
		} else {
			up = *row.Buy.CapexRub
		}
	}
	if down == 0 || up == 0 || down >= up {
		t.Fatalf("price sensitivity %v %v", down, up)
	}
}

func TestApplyVerificationAddsRisk(t *testing.T) {
	res := EmptyResult("warehouse", 0)
	res.VerificationFlag = true
	res.Sim = &SimSummary{Throughput: 10, EconThroughput: 20, Divergence: 0.5, SimulatedS: 900}
	ApplyVerification(&res)
	if !hasRisk(res, "verification") {
		t.Fatalf("risks %+v", res.Risks)
	}
	found := false
	for _, a := range res.Assumptions {
		if strings.Contains(a, "Симуляция пикового окна") {
			found = true
		}
	}
	if !found {
		t.Fatalf("assumptions %v", res.Assumptions)
	}
}

// Every formula the API lists has a MathML form for the web page.
func TestFormulasHaveMathML(t *testing.T) {
	for _, f := range formulas(DefaultNorms()) {
		if f.MathML == "" || !strings.HasPrefix(f.MathML, `<math display="block"><mrow>`) {
			t.Errorf("formula %s has no MathML", f.ID)
		}
	}
}

// A result saved before formulas carried MathML gets it when it is read back.
func TestWithMathMLFillsOldResults(t *testing.T) {
	old := json.RawMessage(`{"formulas":[{"id":"tco","text":"TCO = CAPEX + OPEX_год x горизонт","unit":"₽"}],"seed":1}`)
	var got struct {
		Formulas []Formula `json:"formulas"`
		Seed     int       `json:"seed"`
	}
	if err := json.Unmarshal(WithMathML(old), &got); err != nil {
		t.Fatal(err)
	}
	if got.Seed != 1 || len(got.Formulas) != 1 || !strings.HasPrefix(got.Formulas[0].MathML, "<math") {
		t.Fatalf("got %+v", got)
	}
	if string(WithMathML(json.RawMessage(`not json`))) != "not json" {
		t.Fatal("unreadable input must come back unchanged")
	}
}
