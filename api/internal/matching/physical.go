package matching

import "math"

// NOTE: these rules do not enter the fit score. A broken limit excludes the robot (fit 0 through evalPhys) and missing
// data sends it to review, as in the aisle rule; passing them adds nothing to the average.

const (
	notPassed   = "Решение не проходит."
	askForModel = "Выберите другую модель или добавьте к сравнению с предупреждением."
	askForSpecs = "Уточните ТТХ или добавьте к сравнению с предупреждением."
)

// bodyRules checks the robot's mass and size against the ceiling, floor, lift, doors and turning room of the site.
func bodyRules(site Site, c Candidate) physResult {
	var p physResult
	for _, st := range []*Step{
		ceilingStep(site, c),
		floorLoadStep(site, c),
		liftStep(site, c),
		doorStep(site, c),
		turningStep(site, c),
	} {
		if st == nil {
			continue
		}
		p.steps = append(p.steps, *st)
		switch {
		case st.Kind == KindHard && st.Outcome == OutcomeFail:
			p.exclude = append(p.exclude, st.Text)
		case st.Outcome != OutcomePass:
			p.review = append(p.review, st.Text)
		}
	}
	return p
}

func bodyStep(c Candidate, rule, kind, outcome, text string) Step {
	return Step{RuleID: rule, Kind: kind, Outcome: outcome, SourceURL: c.SourceURL, Confidence: c.Confidence, Text: text}
}

func ceilingStep(site Site, c Candidate) *Step {
	if site.CeilingMm == nil {
		return nil
	}
	st := bodyStep(c, "ceiling_height", KindHard, OutcomePass, "")
	st.ObjectField, st.ObjectValue, st.ObjectUnit = "ceiling_height_m", site.CeilingMm, "mm"
	st.SolutionField, st.SolutionValue, st.SolutionUnit = "height_mm", c.HeightMm, "mm"
	switch {
	case c.HeightMm == nil:
		st.Kind, st.Outcome = KindMissing, OutcomeUnknown
		st.Text = "Нет высоты робота в ТТХ. Нельзя проверить высоту потолков. " + askForSpecs
	case *c.HeightMm > *site.CeilingMm:
		st.Outcome = OutcomeFail
		st.Text = "Высота робота " + fmtNum(*c.HeightMm) + " мм больше высоты потолков " + fmtNum(*site.CeilingMm) + " мм. " + notPassed + " " + askForModel
	default:
		st.Text = "Высота потолков " + fmtNum(*site.CeilingMm) + " мм больше высоты робота " + fmtNum(*c.HeightMm) + " мм."
	}
	return &st
}

// floorPressure is the robot with its payload over the area of its body, in kg/m2.
func floorPressure(c Candidate) (kgM2, areaM2, totalKg float64, ok bool) {
	if c.MassKg == nil || c.LengthMm == nil || c.WidthMm == nil || *c.LengthMm <= 0 || *c.WidthMm <= 0 {
		return 0, 0, 0, false
	}
	totalKg = *c.MassKg
	if c.PayloadKg != nil {
		totalKg += *c.PayloadKg
	}
	areaM2 = *c.LengthMm / 1000 * *c.WidthMm / 1000
	return totalKg / areaM2, areaM2, totalKg, true
}

func floorLoadStep(site Site, c Candidate) *Step {
	if site.FloorLoadKgM2 == nil {
		return nil
	}
	st := bodyStep(c, "robot_mass_floor", KindHard, OutcomePass, "")
	st.ObjectField, st.ObjectValue, st.ObjectUnit = "floor_load_kg_m2", site.FloorLoadKgM2, "kg/m2"
	st.SolutionField, st.SolutionValue, st.SolutionUnit = "mass_kg", c.MassKg, "kg"
	pressure, area, total, ok := floorPressure(c)
	switch {
	case !ok:
		st.Kind, st.Outcome = KindMissing, OutcomeUnknown
		st.Text = "Нет массы, длины или ширины робота в ТТХ. Нельзя проверить нагрузку на пол. " + askForSpecs
	case pressure > *site.FloorLoadKgM2:
		st.Outcome = OutcomeFail
		st.Text = "Робот с грузом " + fmtNum(total) + " кг давит на пол " + fmtNum(pressure) + " кг/м² по площади корпуса " + fmtNum(area) + " м², допустимо " + fmtNum(*site.FloorLoadKgM2) + " кг/м². " + notPassed + " Выберите более лёгкую модель или добавьте к сравнению с предупреждением."
	default:
		st.Text = "Нагрузка на пол " + fmtNum(pressure) + " кг/м² не выше допустимой " + fmtNum(*site.FloorLoadKgM2) + " кг/м²."
	}
	return &st
}

func liftStep(site Site, c Candidate) *Step {
	if site.LiftKg == nil {
		return nil
	}
	field := site.LiftField
	if field == "" {
		field = "lift_capacity_kg"
	}
	st := bodyStep(c, "lift_capacity", KindHard, OutcomePass, "")
	st.ObjectField, st.ObjectValue, st.ObjectUnit = field, site.LiftKg, "kg"
	st.SolutionField, st.SolutionValue, st.SolutionUnit = "mass_kg", c.MassKg, "kg"
	if c.MassKg == nil {
		st.Kind, st.Outcome = KindMissing, OutcomeUnknown
		st.Text = "Нет массы робота в ТТХ. Нельзя проверить грузоподъёмность лифта. " + askForSpecs
		return &st
	}
	total := *c.MassKg
	if c.PayloadKg != nil {
		total += *c.PayloadKg
	}
	if total > *site.LiftKg {
		st.Outcome = OutcomeFail
		st.Text = "Робот с грузом " + fmtNum(total) + " кг тяжелее грузоподъёмности лифта " + fmtNum(*site.LiftKg) + " кг. " + notPassed + " Выберите более лёгкую модель или добавьте к сравнению с предупреждением."
		return &st
	}
	st.Text = "Робот с грузом " + fmtNum(total) + " кг проходит по грузоподъёмности лифта " + fmtNum(*site.LiftKg) + " кг."
	return &st
}

func doorStep(site Site, c Candidate) *Step {
	if site.DoorMm == nil {
		return nil
	}
	st := bodyStep(c, "door_width", KindHard, OutcomePass, "")
	st.ObjectField, st.ObjectValue, st.ObjectUnit = "door_width_m", site.DoorMm, "mm"
	st.SolutionField, st.SolutionValue, st.SolutionUnit = "width_mm", c.WidthMm, "mm"
	switch {
	case c.WidthMm == nil:
		st.Kind, st.Outcome = KindMissing, OutcomeUnknown
		st.Text = "Нет ширины робота в ТТХ. Нельзя проверить проёмы. " + askForSpecs
	case *c.WidthMm > *site.DoorMm:
		st.Outcome = OutcomeFail
		st.Text = "Ширина робота " + fmtNum(*c.WidthMm) + " мм больше проёма " + fmtNum(*site.DoorMm) + " мм. " + notPassed + " Выберите более узкую модель или добавьте к сравнению с предупреждением."
	default:
		st.Text = "Проём " + fmtNum(*site.DoorMm) + " мм шире робота " + fmtNum(*c.WidthMm) + " мм."
	}
	return &st
}

func turnPlace(field string) string {
	if field == "corridor_width_m" {
		return "коридора"
	}
	return "главного проезда"
}

// turningStep compares the circle the robot sweeps in a turn with the room to turn. The catalog radius is a fact; the
// body diagonal is an estimate, so it can only send the robot to review: a robot that turns without rotating its
// body fits where the diagonal does not.
func turningStep(site Site, c Candidate) *Step {
	if site.TurnRoomMm == nil {
		return nil
	}
	field := site.TurnRoomField
	if field == "" {
		field = "aisle_main_m"
	}
	place := turnPlace(field)
	st := bodyStep(c, "turning_envelope", KindHard, OutcomePass, "")
	st.ObjectField, st.ObjectValue, st.ObjectUnit = field, site.TurnRoomMm, "mm"
	if c.TurnRadiusMm != nil {
		st.SolutionField, st.SolutionValue, st.SolutionUnit = "turn_radius_mm", c.TurnRadiusMm, "mm"
		diameter := 2 * *c.TurnRadiusMm
		if diameter > *site.TurnRoomMm {
			st.Outcome = OutcomeFail
			st.Text = "Диаметр разворота робота " + fmtNum(diameter) + " мм больше " + place + " " + fmtNum(*site.TurnRoomMm) + " мм. " + notPassed + " " + askForModel
			return &st
		}
		st.Text = "Диаметр разворота робота " + fmtNum(diameter) + " мм меньше " + place + " " + fmtNum(*site.TurnRoomMm) + " мм."
		return &st
	}
	st.SolutionField, st.SolutionValue, st.SolutionUnit = "length_mm", c.LengthMm, "mm"
	if c.LengthMm == nil || c.WidthMm == nil {
		st.Kind, st.Outcome = KindMissing, OutcomeUnknown
		st.Text = "Нет радиуса разворота, длины или ширины робота в ТТХ. Нельзя проверить разворот. " + askForSpecs
		return &st
	}
	diagonal := math.Hypot(*c.LengthMm, *c.WidthMm)
	if diagonal > *site.TurnRoomMm {
		st.Kind, st.Outcome = KindSoft, OutcomeFail
		st.Text = "Корпус робота " + fmtNum(*c.LengthMm) + " на " + fmtNum(*c.WidthMm) + " мм описывает при развороте круг " + fmtNum(diagonal) + " мм, шире " + place + " " + fmtNum(*site.TurnRoomMm) + " мм. Радиуса разворота в ТТХ нет, это оценка: робот, который разворачивается без поворота корпуса, пройдёт. Уточните радиус разворота."
		return &st
	}
	st.Text = "Круг по диагонали корпуса " + fmtNum(diagonal) + " мм меньше " + place + " " + fmtNum(*site.TurnRoomMm) + " мм, разворот возможен."
	return &st
}
