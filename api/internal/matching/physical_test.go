package matching

import (
	"encoding/json"
	"strings"
	"testing"

	"moscow_hackathon_2026/api/internal/catalog"
	"moscow_hackathon_2026/api/internal/objects"
)

func h1500Body() Candidate {
	return Candidate{
		ID:          "h1500",
		Name:        "Ronavi H1500",
		Subtype:     str("AMR"),
		PriceRub:    f64(2_700_000),
		Confidence:  str("vendor"),
		ObjectTypes: []string{"warehouse", "airport"},
		PayloadKg:   f64(1500),
		MassKg:      f64(250),
		LengthMm:    f64(1044),
		WidthMm:     f64(654),
		HeightMm:    f64(380),
		MinAisleMm:  f64(750),
		TempMinC:    f64(5),
		TempMaxC:    f64(25),
	}
}

func stepOf(t *testing.T, it Item, rule string) Step {
	t.Helper()
	for _, st := range it.Explanation {
		if st.RuleID == rule {
			return st
		}
	}
	t.Fatalf("no %s step in %+v", rule, it.Explanation)
	return Step{}
}

func TestBodyRules(t *testing.T) {
	cases := []struct {
		name    string
		site    func(*Site)
		robot   func(*Candidate)
		rule    string
		kind    string
		outcome string
		status  string
		text    string
	}{
		{
			name: "ceiling above the robot",
			rule: "ceiling_height", kind: KindHard, outcome: OutcomePass, status: StatusRecommended,
			text: "Высота потолков 10 000 мм больше высоты робота 380 мм.",
		},
		{
			name:  "robot taller than the ceiling",
			robot: func(c *Candidate) { c.HeightMm = f64(12000) },
			rule:  "ceiling_height", kind: KindHard, outcome: OutcomeFail, status: StatusExcluded,
			text: "Высота робота 12 000 мм больше высоты потолков 10 000 мм. Решение не проходит. Выберите другую модель или добавьте к сравнению с предупреждением.",
		},
		{
			name:  "no height in the specs",
			robot: func(c *Candidate) { c.HeightMm = nil },
			rule:  "ceiling_height", kind: KindMissing, outcome: OutcomeUnknown, status: StatusNeedsReview,
		},
		{
			name: "floor holds the robot",
			site: func(s *Site) { s.FloorLoadKgM2 = f64(3000) },
			rule: "robot_mass_floor", kind: KindHard, outcome: OutcomePass, status: StatusRecommended,
			text: "Нагрузка на пол 2 563,07 кг/м² не выше допустимой 3 000 кг/м².",
		},
		{
			name: "floor too weak",
			site: func(s *Site) { s.FloorLoadKgM2 = f64(2000) },
			rule: "robot_mass_floor", kind: KindHard, outcome: OutcomeFail, status: StatusExcluded,
			text: "Робот с грузом 1 750 кг давит на пол 2 563,07 кг/м² по площади корпуса 0,68 м², допустимо 2 000 кг/м². Решение не проходит. Выберите более лёгкую модель или добавьте к сравнению с предупреждением.",
		},
		{
			name:  "no mass for the floor check",
			site:  func(s *Site) { s.FloorLoadKgM2 = f64(3000) },
			robot: func(c *Candidate) { c.MassKg = nil },
			rule:  "robot_mass_floor", kind: KindMissing, outcome: OutcomeUnknown, status: StatusNeedsReview,
		},
		{
			name: "lift carries the robot",
			site: func(s *Site) { s.LiftKg = f64(2000) },
			rule: "lift_capacity", kind: KindHard, outcome: OutcomePass, status: StatusRecommended,
			text: "Робот с грузом 1 750 кг проходит по грузоподъёмности лифта 2 000 кг.",
		},
		{
			name: "lift too weak",
			site: func(s *Site) { s.LiftKg = f64(1500) },
			rule: "lift_capacity", kind: KindHard, outcome: OutcomeFail, status: StatusExcluded,
			text: "Робот с грузом 1 750 кг тяжелее грузоподъёмности лифта 1 500 кг. Решение не проходит. Выберите более лёгкую модель или добавьте к сравнению с предупреждением.",
		},
		{
			name:  "no mass for the lift check",
			site:  func(s *Site) { s.LiftKg = f64(2000) },
			robot: func(c *Candidate) { c.MassKg = nil },
			rule:  "lift_capacity", kind: KindMissing, outcome: OutcomeUnknown, status: StatusNeedsReview,
		},
		{
			name: "door wider than the robot",
			site: func(s *Site) { s.DoorMm = f64(1000) },
			rule: "door_width", kind: KindHard, outcome: OutcomePass, status: StatusRecommended,
			text: "Проём 1 000 мм шире робота 654 мм.",
		},
		{
			name: "door narrower than the robot",
			site: func(s *Site) { s.DoorMm = f64(600) },
			rule: "door_width", kind: KindHard, outcome: OutcomeFail, status: StatusExcluded,
			text: "Ширина робота 654 мм больше проёма 600 мм. Решение не проходит. Выберите более узкую модель или добавьте к сравнению с предупреждением.",
		},
		{
			name:  "no width for the door check",
			site:  func(s *Site) { s.DoorMm = f64(1000) },
			robot: func(c *Candidate) { c.WidthMm = nil },
			rule:  "door_width", kind: KindMissing, outcome: OutcomeUnknown, status: StatusNeedsReview,
		},
		{
			name:  "stated turn radius fits",
			site:  func(s *Site) { s.TurnRoomMm = f64(1000) },
			robot: func(c *Candidate) { c.TurnRadiusMm = f64(400) },
			rule:  "turning_envelope", kind: KindHard, outcome: OutcomePass, status: StatusRecommended,
			text: "Диаметр разворота робота 800 мм меньше главного проезда 1 000 мм.",
		},
		{
			name:  "stated turn radius does not fit",
			site:  func(s *Site) { s.TurnRoomMm = f64(1000) },
			robot: func(c *Candidate) { c.TurnRadiusMm = f64(600) },
			rule:  "turning_envelope", kind: KindHard, outcome: OutcomeFail, status: StatusExcluded,
			text: "Диаметр разворота робота 1 200 мм больше главного проезда 1 000 мм. Решение не проходит. Выберите другую модель или добавьте к сравнению с предупреждением.",
		},
		{
			name: "body diagonal fits",
			rule: "turning_envelope", kind: KindHard, outcome: OutcomePass, status: StatusRecommended,
			text: "Круг по диагонали корпуса 1 231,93 мм меньше главного проезда 3 500 мм, разворот возможен.",
		},
		{
			name: "body diagonal is only an estimate",
			site: func(s *Site) { s.TurnRoomMm = f64(1200) },
			rule: "turning_envelope", kind: KindSoft, outcome: OutcomeFail, status: StatusNeedsReview,
			text: "Корпус робота 1 044 на 654 мм описывает при развороте круг 1 231,93 мм, шире главного проезда 1 200 мм. Радиуса разворота в ТТХ нет, это оценка: робот, который разворачивается без поворота корпуса, пройдёт. Уточните радиус разворота.",
		},
		{
			name:  "no turn data",
			robot: func(c *Candidate) { c.LengthMm = nil },
			rule:  "turning_envelope", kind: KindMissing, outcome: OutcomeUnknown, status: StatusNeedsReview,
		},
		{
			name:  "hospital corridor is named",
			site:  func(s *Site) { s.TurnRoomMm = f64(1000); s.TurnRoomField = "corridor_width_m" },
			robot: func(c *Candidate) { c.TurnRadiusMm = f64(600) },
			rule:  "turning_envelope", kind: KindHard, outcome: OutcomeFail, status: StatusExcluded,
			text: "Диаметр разворота робота 1 200 мм больше коридора 1 000 мм. Решение не проходит. Выберите другую модель или добавьте к сравнению с предупреждением.",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			site := siteFromDefaults(t, objects.Warehouse)
			site.AisleMm = f64(10000)
			if c.site != nil {
				c.site(&site)
			}
			robot := h1500Body()
			if c.robot != nil {
				c.robot(&robot)
			}
			it := scoreOne(site, robot, false)
			st := stepOf(t, it, c.rule)
			if st.Kind != c.kind || st.Outcome != c.outcome {
				t.Fatalf("step %s/%s, want %s/%s: %s", st.Kind, st.Outcome, c.kind, c.outcome, st.Text)
			}
			if c.text != "" && strings.ReplaceAll(st.Text, " ", " ") != c.text {
				t.Fatalf("text %q, want %q", st.Text, c.text)
			}
			if it.Status != c.status {
				t.Fatalf("status %s, want %s: %v", it.Status, c.status, it.Reasons)
			}
		})
	}
}

func TestBodyRulesStayOffWithoutSiteLimits(t *testing.T) {
	site := siteFromDefaults(t, objects.Warehouse)
	site.CeilingMm, site.TurnRoomMm = nil, nil
	it := scoreOne(site, h1500Body(), false)
	for _, rule := range []string{"ceiling_height", "robot_mass_floor", "lift_capacity", "door_width", "turning_envelope"} {
		for _, st := range it.Explanation {
			if st.RuleID == rule {
				t.Fatalf("%s ran without a site limit: %+v", rule, st)
			}
		}
	}
}

// A rule that passes adds nothing to the fit score, so data on the site never ranks a robot above another.
func TestBodyRulesLeaveFitScoreAlone(t *testing.T) {
	plain := siteFromDefaults(t, objects.Warehouse)
	plain.CeilingMm, plain.TurnRoomMm = nil, nil
	limited := plain
	limited.FloorLoadKgM2, limited.LiftKg, limited.DoorMm = f64(5000), f64(3000), f64(3000)
	limited.CeilingMm, limited.TurnRoomMm = f64(10000), f64(3500)
	a, b := scoreOne(plain, h1500Body(), false), scoreOne(limited, h1500Body(), false)
	if a.ScoreParts.Fit != b.ScoreParts.Fit || a.Score != b.Score {
		t.Fatalf("fit %v vs %v, score %v vs %v", a.ScoreParts.Fit, b.ScoreParts.Fit, a.Score, b.Score)
	}
}

func TestBrokenBodyLimitZeroesFit(t *testing.T) {
	site := siteFromDefaults(t, objects.Warehouse)
	site.DoorMm = f64(600)
	it := scoreOne(site, h1500Body(), false)
	if it.ScoreParts.Fit != 0 || it.Status != StatusExcluded {
		t.Fatalf("fit %v status %s", it.ScoreParts.Fit, it.Status)
	}
	if len(it.Hard) == 0 {
		t.Fatal("no hard steps")
	}
}

func siteOf(t *testing.T, objectType, params string) Site {
	t.Helper()
	s, err := SiteFromParams(objectType, json.RawMessage(params))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSiteBodyLimitsFromParams(t *testing.T) {
	one := siteOf(t, "warehouse", `{"floors":1,"lift_capacity_kg":2000,"floor_load_kg_m2":4000,"door_width_m":3,"ceiling_height_m":8,"aisle_main_m":3.5}`)
	if one.LiftKg != nil {
		t.Fatalf("a single floor needs no lift check: %v", *one.LiftKg)
	}
	if one.FloorLoadKgM2 == nil || *one.FloorLoadKgM2 != 4000 || one.DoorMm == nil || *one.DoorMm != 3000 {
		t.Fatalf("floor %v door %v", one.FloorLoadKgM2, one.DoorMm)
	}
	if one.CeilingMm == nil || *one.CeilingMm != 8000 || one.TurnRoomMm == nil || *one.TurnRoomMm != 3500 || one.TurnRoomField != "aisle_main_m" {
		t.Fatalf("ceiling %v turn %v %s", one.CeilingMm, one.TurnRoomMm, one.TurnRoomField)
	}
	two := siteOf(t, "warehouse", `{"floors":2,"lift_capacity_kg":2000}`)
	if two.LiftKg == nil || *two.LiftKg != 2000 || two.LiftField != "lift_capacity_kg" {
		t.Fatalf("lift %v %s", two.LiftKg, two.LiftField)
	}
	unknown := siteOf(t, "warehouse", `{"floors":2,"lift_capacity_kg":null,"floor_load_kg_m2":null,"door_width_m":null}`)
	if unknown.LiftKg != nil || unknown.FloorLoadKgM2 != nil || unknown.DoorMm != nil {
		t.Fatalf("null must stay unknown: %+v", unknown)
	}
	hosp := siteOf(t, "hospital", `{"elevator_capacity_kg":1000,"door_width_m":1.2,"corridor_width_m":2.4}`)
	if hosp.LiftKg == nil || *hosp.LiftKg != 1000 || hosp.LiftField != "elevator_capacity_kg" ||
		hosp.DoorMm == nil || *hosp.DoorMm != 1200 || hosp.TurnRoomMm == nil || *hosp.TurnRoomMm != 2400 || hosp.TurnRoomField != "corridor_width_m" {
		t.Fatalf("hospital %+v", hosp)
	}
	air := siteOf(t, "airport", `{"door_width_m":1.2}`)
	if air.DoorMm != nil || air.TurnRoomMm != nil || air.CeilingMm != nil {
		t.Fatalf("airport has no body limits: %+v", air)
	}
}

func TestWarehouseDefaultsSetOnlyCeilingAndTurning(t *testing.T) {
	s := siteFromDefaults(t, objects.Warehouse)
	if s.CeilingMm == nil || *s.CeilingMm != 10000 || s.TurnRoomMm == nil || *s.TurnRoomMm != 3500 {
		t.Fatalf("ceiling %v turn %v", s.CeilingMm, s.TurnRoomMm)
	}
	if s.FloorLoadKgM2 != nil || s.LiftKg != nil || s.DoorMm != nil {
		t.Fatalf("limits the user has not entered must stay unknown: %+v", s)
	}
}

func TestStepsUseTheSourceOfTheirField(t *testing.T) {
	site := siteFromDefaults(t, objects.Warehouse)
	robot := h1500Body()
	robot.SourceURL = str("https://card.example/h1500")
	robot.FieldSources = map[string]catalog.FieldSource{
		"min_aisle_mm": {SourceURL: "https://vendor.example/aisle", Confidence: "measured"},
		"height_mm":    {Confidence: "assumed"},
	}
	it := scoreOne(site, robot, false)
	aisle := stepOf(t, it, "aisle_width")
	if aisle.SourceURL == nil || *aisle.SourceURL != "https://vendor.example/aisle" || aisle.Confidence == nil || *aisle.Confidence != "measured" {
		t.Fatalf("aisle step %+v %+v", aisle.SourceURL, aisle.Confidence)
	}
	payload := stepOf(t, it, "payload_kg")
	if payload.SourceURL == nil || *payload.SourceURL != "https://card.example/h1500" || payload.Confidence == nil || *payload.Confidence != "vendor" {
		t.Fatalf("a value with no entry keeps the source of the card: %+v %+v", payload.SourceURL, payload.Confidence)
	}
	ceiling := stepOf(t, it, "ceiling_height")
	if ceiling.SourceURL == nil || *ceiling.SourceURL != "https://card.example/h1500" || ceiling.Confidence == nil || *ceiling.Confidence != "assumed" {
		t.Fatalf("an entry without a link keeps the link of the card: %+v %+v", ceiling.SourceURL, ceiling.Confidence)
	}
}
