package matching

import (
	"encoding/json"
	"strings"
	"testing"

	"moscow_hackathon_2026/api/internal/objects"
)

func f64(v float64) *float64 { return &v }

func str(v string) *string { return &v }

func TestClassify(t *testing.T) {
	full := Specs{
		PayloadKg:  f64(1500),
		WidthMm:    f64(654),
		MinAisleMm: f64(900),
		TempMinC:   f64(5),
		TempMaxC:   f64(25),
	}
	ok := Classify(full)
	if ok.Status != StatusOK {
		t.Fatalf("full specs: status %q", ok.Status)
	}
	if len(ok.Missing) != 0 || len(ok.Reasons) != 0 {
		t.Fatalf("full specs: missing=%v reasons=%v", ok.Missing, ok.Reasons)
	}

	h1500 := Specs{
		PayloadKg: f64(1500),
		WidthMm:   f64(654),
		TempMinC:  f64(5),
		TempMaxC:  f64(25),
	}
	review := Classify(h1500)
	if review.Status != StatusNeedsReview {
		t.Fatalf("H1500 without aisle: status %q", review.Status)
	}
	if len(review.Missing) != 1 || review.Missing[0] != "min_aisle_mm" {
		t.Fatalf("H1500 without aisle: missing %v", review.Missing)
	}

	empty := Classify(Specs{})
	if empty.Status != StatusNeedsReview {
		t.Fatalf("empty: status %q", empty.Status)
	}
	if len(empty.Missing) != 5 {
		t.Fatalf("empty: want 5 missing, got %v", empty.Missing)
	}
}

func TestWeightsSumToOne(t *testing.T) {
	w := DefaultWeights()
	sum := w.Fit + w.ProcessMatch + w.DataQuality + w.PriceBand
	if sum != 1 {
		t.Fatalf("weights sum %v", sum)
	}
}

func siteFromDefaults(t *testing.T, objectType string) Site {
	t.Helper()
	def, err := objects.Defaults(objectType)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	s, err := SiteFromParams(objectType, raw)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSiteFromWarehouseDefaults(t *testing.T) {
	s := siteFromDefaults(t, objects.Warehouse)
	if s.AisleMm == nil || *s.AisleMm != 2800 {
		t.Fatalf("aisle mm %+v", s.AisleMm)
	}
	if s.AisleField != "aisle_working_m" {
		t.Fatalf("aisle field %q", s.AisleField)
	}
	if s.LightLoadKg == nil || *s.LightLoadKg != 1.8 {
		t.Fatalf("light %+v", s.LightLoadKg)
	}
	if s.HeavyLoadKg == nil || *s.HeavyLoadKg != 800 {
		t.Fatalf("heavy %+v", s.HeavyLoadKg)
	}
	if s.BudgetRub == nil || *s.BudgetRub != 80_000_000 {
		t.Fatalf("budget %+v", s.BudgetRub)
	}
	if s.TempC != nil {
		t.Fatalf("warehouse temp %+v", s.TempC)
	}
}

func TestSiteFromAirportHospital(t *testing.T) {
	air := siteFromDefaults(t, objects.Airport)
	if air.AisleMm != nil {
		t.Fatalf("airport aisle %+v", air.AisleMm)
	}
	if air.TempC == nil || *air.TempC != -25 {
		t.Fatalf("apron temp %+v", air.TempC)
	}
	if air.LightLoadKg == nil || *air.LightLoadKg != 18 {
		t.Fatalf("baggage %+v", air.LightLoadKg)
	}
	hosp := siteFromDefaults(t, objects.Hospital)
	if hosp.AisleMm == nil || *hosp.AisleMm != 2400 {
		t.Fatalf("corridor %+v", hosp.AisleMm)
	}
	if hosp.LightLoadKg != nil {
		t.Fatalf("hospital light %+v", hosp.LightLoadKg)
	}
	if hosp.HeavyLoadKg == nil || *hosp.HeavyLoadKg != 120 {
		t.Fatalf("meal cart %+v", hosp.HeavyLoadKg)
	}
}

func itemByID(t *testing.T, out Output, id string) Item {
	t.Helper()
	for _, it := range out.Items {
		if it.SolutionID == id {
			return it
		}
	}
	t.Fatalf("missing %s", id)
	return Item{}
}

func TestMatchWarehouseFixtures(t *testing.T) {
	site := siteFromDefaults(t, objects.Warehouse)
	h1500 := Candidate{
		ID:          "h1500",
		Name:        "Ronavi H1500",
		Vendor:      str("Ronavi"),
		Subtype:     str("AMR"),
		Family:      "Мобильные роботы",
		Scenario:    str("Внутрискладская логистика"),
		PriceRub:    f64(2_700_000),
		Confidence:  str("vendor"),
		ObjectTypes: []string{"warehouse", "airport"},
		PayloadKg:   f64(1500),
		WidthMm:     f64(654),
		TempMinC:    f64(5),
		TempMaxC:    f64(25),
	}
	wide := h1500
	wide.ID = "wide"
	wide.Name = "Wide AMR"
	wide.WidthMm = f64(4000)
	wide.MinAisleMm = f64(4200)
	light := h1500
	light.ID = "light"
	light.Name = "Light AMR"
	light.PayloadKg = f64(1)
	light.ObjectTypes = []string{"warehouse"}
	tote := h1500
	tote.ID = "tote"
	tote.Name = "Tote AMR"
	tote.PayloadKg = f64(50)
	tote.MinAisleMm = f64(900)
	tote.ObjectTypes = []string{"warehouse"}
	marine := Candidate{
		ID:       "boat",
		Name:     "Sea bot",
		Family:   "Морские роботы",
		Scenario: str("Проведение подводных работ и мониторинга"),
		PriceRub: f64(1_000_000),
	}
	unknown := Candidate{ID: "x", Name: "Unknown box", PriceRub: f64(100)}
	out := Match(site, []Candidate{h1500, wide, light, tote, marine, unknown}, []string{"wide", "missing"})
	if len(out.SelectedIDs) != 1 || out.SelectedIDs[0] != "wide" {
		t.Fatalf("selected %v", out.SelectedIDs)
	}

	h := itemByID(t, out, "h1500")
	if h.Status != StatusNeedsReview {
		t.Fatalf("H1500 status %s reasons %v", h.Status, h.Reasons)
	}
	if h.ScoreParts.ProcessMatch != 1 {
		t.Fatalf("H1500 process %v", h.ScoreParts.ProcessMatch)
	}
	if h.Forced {
		t.Fatal("H1500 forced")
	}

	w := itemByID(t, out, "wide")
	if w.Status != StatusExcluded {
		t.Fatalf("wide status %s %v", w.Status, w.Reasons)
	}
	if !w.Forced {
		t.Fatal("wide should be forced")
	}
	if w.ScoreParts.Fit != 0 {
		t.Fatalf("wide fit %v", w.ScoreParts.Fit)
	}

	l := itemByID(t, out, "light")
	if l.Status != StatusExcluded {
		t.Fatalf("light status %s %v", l.Status, l.Reasons)
	}

	to := itemByID(t, out, "tote")
	if to.Status != StatusNeedsReview {
		t.Fatalf("tote status %s %v", to.Status, to.Reasons)
	}

	b := itemByID(t, out, "boat")
	if b.Status != StatusExcluded {
		t.Fatalf("boat status %s %v", b.Status, b.Reasons)
	}

	u := itemByID(t, out, "x")
	if u.Status != StatusNeedsReview {
		t.Fatalf("unknown status %s %v", u.Status, u.Reasons)
	}
	if u.ScoreParts.ProcessMatch != 0.4 {
		t.Fatalf("unknown process %v", u.ScoreParts.ProcessMatch)
	}

	order := make([]string, 0, len(out.Items))
	for _, it := range out.Items {
		order = append(order, it.Status)
	}
	if strings.Index(strings.Join(order, ","), StatusExcluded+","+StatusNeedsReview) >= 0 {
		t.Fatalf("sort statuses %v", order)
	}
	seenExcl := false
	for _, st := range order {
		if st == StatusExcluded {
			seenExcl = true
		}
		if seenExcl && st != StatusExcluded {
			t.Fatalf("excluded not last: %v", order)
		}
	}
}

func TestMatchAirportTempAndRecommended(t *testing.T) {
	site := siteFromDefaults(t, objects.Airport)
	h1500 := Candidate{
		ID:          "h1500",
		Name:        "Ronavi H1500",
		Subtype:     str("AMR"),
		PriceRub:    f64(2_700_000),
		Confidence:  str("vendor"),
		ObjectTypes: []string{"warehouse", "airport"},
		PayloadKg:   f64(1500),
		WidthMm:     f64(654),
		TempMinC:    f64(5),
		TempMaxC:    f64(25),
	}
	evo := Candidate{
		ID:          "evo",
		Name:        "EVOCARGO N1",
		Family:      "Автономные наземные транспортные средства",
		PriceRub:    f64(10_000_000),
		Confidence:  str("vendor"),
		ObjectTypes: []string{"airport"},
		PayloadKg:   f64(2000),
		WidthMm:     f64(1800),
		TempMinC:    f64(-40),
		TempMaxC:    f64(50),
	}
	out := Match(site, []Candidate{h1500, evo}, nil)
	h := itemByID(t, out, "h1500")
	if h.Status != StatusExcluded {
		t.Fatalf("H1500 airport %s %v", h.Status, h.Reasons)
	}
	e := itemByID(t, out, "evo")
	if e.Status != StatusRecommended {
		t.Fatalf("EVOCARGO %s %v", e.Status, e.Reasons)
	}
	if e.ScoreParts.ProcessMatch != 1 || e.ScoreParts.DataQuality != 1 || e.ScoreParts.PriceBand != 1 {
		t.Fatalf("EVOCARGO parts %+v", e.ScoreParts)
	}
	if out.Items[0].SolutionID != "evo" {
		t.Fatalf("first %s", out.Items[0].SolutionID)
	}
}

func TestMatchHospitalLightPayload(t *testing.T) {
	site := siteFromDefaults(t, objects.Hospital)
	sd := Candidate{
		ID:          "sd",
		Name:        "Ronavi SD",
		Subtype:     str("AMR"),
		PriceRub:    f64(1_400_000),
		Confidence:  str("vendor"),
		ObjectTypes: []string{"hospital"},
		PayloadKg:   f64(10),
		WidthMm:     f64(400),
		MinAisleMm:  f64(700),
	}
	out := Match(site, []Candidate{sd}, nil)
	it := itemByID(t, out, "sd")
	if it.Status != StatusNeedsReview {
		t.Fatalf("SD %s %v", it.Status, it.Reasons)
	}
	if it.ScoreParts.ProcessMatch != 1 {
		t.Fatalf("SD process %v", it.ScoreParts.ProcessMatch)
	}
}

func TestMatchSeedObjectTypesExclude(t *testing.T) {
	site := siteFromDefaults(t, objects.Warehouse)
	sd := Candidate{
		ID:          "sd",
		Name:        "Ronavi SD",
		Scenario:    str("Внутрискладская логистика"),
		ObjectTypes: []string{"hospital"},
		PayloadKg:   f64(10),
		WidthMm:     f64(400),
		MinAisleMm:  f64(700),
		PriceRub:    f64(1_400_000),
		Confidence:  str("vendor"),
	}
	out := Match(site, []Candidate{sd}, nil)
	it := itemByID(t, out, "sd")
	if it.Status != StatusExcluded {
		t.Fatalf("SD warehouse %s %v", it.Status, it.Reasons)
	}
	if it.ScoreParts.ProcessMatch != 0 {
		t.Fatalf("process %v", it.ScoreParts.ProcessMatch)
	}
}

func TestMatchPriceBand(t *testing.T) {
	site := Site{ObjectType: "airport", LightLoadKg: f64(18), TempC: f64(-10), BudgetRub: f64(10_000_000)}
	base := Candidate{
		ID:          "p",
		Name:        "Pricy",
		ObjectTypes: []string{"airport"},
		PayloadKg:   f64(100),
		WidthMm:     f64(800),
		TempMinC:    f64(-20),
		TempMaxC:    f64(40),
		Confidence:  str("vendor"),
		PriceRub:    f64(10_000_000),
	}
	ok := Match(site, []Candidate{base}, nil).Items[0]
	if ok.ScoreParts.PriceBand != 1 {
		t.Fatalf("at budget %v", ok.ScoreParts.PriceBand)
	}
	over := base
	over.PriceRub = f64(30_000_000)
	got := Match(site, []Candidate{over}, nil).Items[0]
	if got.ScoreParts.PriceBand != 0 {
		t.Fatalf("3x budget %v", got.ScoreParts.PriceBand)
	}
	mid := base
	mid.PriceRub = f64(20_000_000)
	got = Match(site, []Candidate{mid}, nil).Items[0]
	if got.ScoreParts.PriceBand != 0.5 {
		t.Fatalf("2x budget %v", got.ScoreParts.PriceBand)
	}
	none := base
	none.PriceRub = nil
	got = Match(site, []Candidate{none}, nil).Items[0]
	if got.ScoreParts.PriceBand != 0.5 || got.Status != StatusNeedsReview {
		t.Fatalf("missing price %+v", got)
	}
}

func TestClassifyProcess(t *testing.T) {
	match, _ := classifyProcess("warehouse", Candidate{
		ObjectTypes: []string{"warehouse"},
		Family:      "Морские роботы",
	})
	if match != processMatch {
		t.Fatalf("seed overrides ban: %v", match)
	}
	ban, reason := classifyProcess("warehouse", Candidate{Family: "Морские роботы"})
	if ban != processMismatch || reason == "" {
		t.Fatalf("ban %v %q", ban, reason)
	}
	wh, _ := classifyProcess("warehouse", Candidate{Scenario: str("Внутрискладская логистика")})
	if wh != processMatch {
		t.Fatalf("warehouse scenario %v", wh)
	}
	cross, _ := classifyProcess("hospital", Candidate{Scenario: str("Внутрискладская логистика")})
	if cross != processMismatch {
		t.Fatalf("cross %v", cross)
	}
	unk, _ := classifyProcess("airport", Candidate{Name: "Box"})
	if unk != processUnknown {
		t.Fatalf("unknown %v", unk)
	}
	clean, _ := classifyProcess("hospital", Candidate{Subtype: str("Робот-уборщик"), Scenario: str("Уборка помещений")})
	if clean != processMatch {
		t.Fatalf("cleaner %v", clean)
	}
}

func TestExplanationChainAisleAndPayload(t *testing.T) {
	site := siteFromDefaults(t, objects.Warehouse)
	c := Candidate{
		ID:          "h1500",
		Name:        "Ronavi H1500",
		Subtype:     str("AMR"),
		Family:      "Мобильные роботы",
		Scenario:    str("Внутрискладская логистика"),
		PriceRub:    f64(2_700_000),
		Confidence:  str("vendor"),
		SourceURL:   str("https://example.test/h1500"),
		ObjectTypes: []string{"warehouse"},
		PayloadKg:   f64(1500),
		WidthMm:     f64(654),
		TempMinC:    f64(5),
		TempMaxC:    f64(25),
	}
	out := Match(site, []Candidate{c}, nil)
	it := itemByID(t, out, "h1500")
	aisle := findStep(it.Explanation, "aisle_width", OutcomePass)
	if aisle == nil {
		t.Fatalf("missing aisle pass: %+v", it.Explanation)
	}
	if aisle.ObjectField != "aisle_working_m" || aisle.ObjectValue == nil || *aisle.ObjectValue != 2800 {
		t.Fatalf("object aisle %+v", aisle)
	}
	if aisle.SolutionField != "width_mm" || aisle.SolutionValue == nil || *aisle.SolutionValue != 654 {
		t.Fatalf("solution width %+v", aisle)
	}
	if aisle.SourceURL == nil || *aisle.SourceURL != "https://example.test/h1500" {
		t.Fatalf("source %+v", aisle.SourceURL)
	}
	if aisle.Kind != KindHard {
		t.Fatalf("aisle kind %s", aisle.Kind)
	}
	payload := findStep(it.Explanation, "payload_kg", OutcomePass)
	if payload == nil || payload.ObjectField != "unit_mass_kg" || payload.SolutionField != "payload_kg" {
		t.Fatalf("payload step %+v", payload)
	}
	if len(it.MissingEvidence) == 0 {
		t.Fatalf("H1500 should miss min_aisle_mm: %+v", it)
	}
}

func TestTaskCapabilitySoftFail(t *testing.T) {
	site := siteFromDefaults(t, objects.Warehouse)
	site.TaskCodes = []string{"pallet_inbound", "piece_pick"}
	c := Candidate{
		ID:          "tote",
		Name:        "Tote AMR",
		ObjectTypes: []string{"warehouse"},
		PayloadKg:   f64(50),
		WidthMm:     f64(600),
		MinAisleMm:  f64(900),
		PriceRub:    f64(1_000_000),
		Confidence:  str("vendor"),
	}
	it := Match(site, []Candidate{c}, nil).Items[0]
	if it.Status != StatusNeedsReview {
		t.Fatalf("status %s %v", it.Status, it.Reasons)
	}
	st := findTaskStep(it.Explanation, "pallet_inbound")
	if st == nil || st.Outcome != OutcomeFail || st.Kind != KindSoft {
		t.Fatalf("pallet task %+v", st)
	}
	if st.CapabilityCode != "payload_pallet" {
		t.Fatalf("cap %s", st.CapabilityCode)
	}
	pick := findTaskStep(it.Explanation, "piece_pick")
	if pick == nil || pick.Outcome != OutcomePass {
		t.Fatalf("piece_pick %+v", pick)
	}
}

func TestHardAisleFailIsHardKind(t *testing.T) {
	site := siteFromDefaults(t, objects.Warehouse)
	c := Candidate{
		ID:          "wide",
		Name:        "Wide AMR",
		ObjectTypes: []string{"warehouse"},
		PayloadKg:   f64(1500),
		WidthMm:     f64(4000),
		MinAisleMm:  f64(4200),
		PriceRub:    f64(1_000_000),
		Confidence:  str("vendor"),
	}
	it := Match(site, []Candidate{c}, nil).Items[0]
	if it.Status != StatusExcluded {
		t.Fatalf("status %s", it.Status)
	}
	st := findStep(it.Hard, "aisle_width", OutcomeFail)
	if st == nil {
		t.Fatalf("hard %+v explanation %+v", it.Hard, it.Explanation)
	}
	if st.ObjectField != "aisle_working_m" || st.SolutionField != "width_mm" {
		t.Fatalf("trace %+v", st)
	}
}

func findStep(steps []Step, rule, outcome string) *Step {
	for i := range steps {
		if steps[i].RuleID == rule && steps[i].Outcome == outcome {
			return &steps[i]
		}
	}
	return nil
}

func findTaskStep(steps []Step, task string) *Step {
	for i := range steps {
		if steps[i].RuleID == "task_capability" && steps[i].TaskCode == task {
			return &steps[i]
		}
	}
	return nil
}

func TestEmptyJSON(t *testing.T) {
	got, err := json.Marshal(Empty())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"weights":{"fit":0.4,"process_match":0.25,"data_quality":0.2,"price_band":0.15},"selected_ids":[],"items":[],"match_version":"match-v4"}`
	if string(got) != want {
		t.Fatalf("empty\ngot  %s\nwant %s", got, want)
	}
}

func TestBriefDropsTraceAndKeepsRanking(t *testing.T) {
	site := siteFromDefaults(t, objects.Warehouse)
	wide := Candidate{
		ID:          "wide",
		Name:        "Wide AMR",
		ObjectTypes: []string{"warehouse"},
		PayloadKg:   f64(1500),
		WidthMm:     f64(4000),
		MinAisleMm:  f64(4200),
		PriceRub:    f64(1_000_000),
		Confidence:  str("vendor"),
	}
	tote := Candidate{
		ID:          "tote",
		Name:        "Tote AMR",
		ObjectTypes: []string{"warehouse"},
		PayloadKg:   f64(50),
		WidthMm:     f64(600),
		MinAisleMm:  f64(900),
		PriceRub:    f64(1_000_000),
		Confidence:  str("vendor"),
	}
	full := Match(site, []Candidate{wide, tote}, []string{"wide"})
	brief := full.Brief()
	if len(full.Items[0].Explanation) == 0 {
		t.Fatal("full output lost its trace")
	}
	if len(brief.Items) != len(full.Items) {
		t.Fatalf("items %d, want %d", len(brief.Items), len(full.Items))
	}
	for i, it := range brief.Items {
		want := full.Items[i]
		if it.SolutionID != want.SolutionID || it.Status != want.Status || it.Score != want.Score || it.ScoreParts != want.ScoreParts || it.Forced != want.Forced {
			t.Fatalf("item %d changed: %+v vs %+v", i, it, want)
		}
	}
	if brief.Items[1].SolutionID != "wide" || !brief.Items[1].Forced {
		t.Fatalf("forced excluded item %+v", brief.Items[1])
	}
	raw, err := json.Marshal(brief.Items[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"reasons":[]`, `"hard":[]`, `"soft":[]`, `"missing_evidence":[]`, `"explanation":[]`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("brief item %s lacks %s", raw, key)
		}
	}
}
