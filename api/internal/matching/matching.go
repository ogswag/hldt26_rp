package matching

import (
	"math"
	"sort"
	"strings"

	"moscow_hackathon_2026/api/internal/catalog"
	"moscow_hackathon_2026/api/internal/rutext"
)

const (
	StatusOK          = "ok"
	StatusRecommended = "recommended"
	StatusNeedsReview = "needs_review"
	StatusExcluded    = "excluded"
)

// confidenceText names where a robot's specs came from, for texts users read.
var confidenceText = map[string]string{
	"measured": "измерения",
	"vendor":   "данные производителя",
}

const (
	WeightFit         = 0.40
	WeightProcess     = 0.25
	WeightDataQuality = 0.20
	WeightPrice       = 0.15
)

type Specs struct {
	PayloadKg  *float64
	WidthMm    *float64
	MinAisleMm *float64
	TempMinC   *float64
	TempMaxC   *float64
}

type DataQuality struct {
	Status  string   `json:"status"`
	Missing []string `json:"missing"`
	Reasons []string `json:"reasons"`
}

type Use struct {
	Industry string
	Scenario string
	Cases    string
}

type Candidate struct {
	ID              string
	Name            string
	Vendor          *string
	Kind            *string
	Subtype         *string
	Scenario        *string
	Family          string
	Description     string
	PriceRub        *float64
	Confidence      *string
	SourceURL       *string
	ObjectTypes     []string
	Uses            []Use
	PayloadKg       *float64
	MassKg          *float64
	LengthMm        *float64
	WidthMm         *float64
	HeightMm        *float64
	MinAisleMm      *float64
	TurnRadiusMm    *float64
	TempMinC        *float64
	TempMaxC        *float64
	CapabilityCodes []string
	// FieldSources say where single values come from; a step reads the source of its own field.
	FieldSources map[string]catalog.FieldSource
	// Archived robots are matched only when a variant already names them.
	Archived bool
}

type Site struct {
	ObjectType  string
	AisleMm     *float64
	AisleField  string
	LightLoadKg *float64
	LightField  string
	HeavyLoadKg *float64
	HeavyField  string
	TempC       *float64
	TempField   string
	BudgetRub   *float64
	BudgetField string
	TaskCodes   []string
	// The limits below are optional: a rule runs only when its limit is set.
	CeilingMm     *float64
	FloorLoadKgM2 *float64
	LiftKg        *float64
	LiftField     string
	DoorMm        *float64
	TurnRoomMm    *float64
	TurnRoomField string
}

type Weights struct {
	Fit          float64 `json:"fit"`
	ProcessMatch float64 `json:"process_match"`
	DataQuality  float64 `json:"data_quality"`
	PriceBand    float64 `json:"price_band"`
}

type ScoreParts struct {
	Fit          float64 `json:"fit"`
	ProcessMatch float64 `json:"process_match"`
	DataQuality  float64 `json:"data_quality"`
	PriceBand    float64 `json:"price_band"`
}

type Item struct {
	SolutionID      string     `json:"solution_id"`
	Name            string     `json:"name"`
	Vendor          *string    `json:"vendor"`
	Kind            *string    `json:"kind"`
	Subtype         *string    `json:"subtype"`
	Family          string     `json:"family,omitempty"`
	PriceRub        *float64   `json:"price_rub"`
	Status          string     `json:"status"`
	Reasons         []string   `json:"reasons"`
	Hard            []Step     `json:"hard"`
	Soft            []Step     `json:"soft"`
	MissingEvidence []Step     `json:"missing_evidence"`
	Explanation     []Step     `json:"explanation"`
	ScoreParts      ScoreParts `json:"score_parts"`
	Score           float64    `json:"score"`
	Forced          bool       `json:"forced"`
	// Estimate is the robot's buy case for this project, filled by the calculation (econ.Rank).
	Estimate *Estimate `json:"estimate,omitempty"`
}

// Estimate is the fleet and buy scenario a robot would get on its own. ProcessCodes are the project processes it
// serves in that case; empty means it serves none of them.
type Estimate struct {
	FleetSize    int      `json:"fleet_size"`
	CapexRub     float64  `json:"capex_rub"`
	PaybackYears *float64 `json:"payback_years"`
	ProcessCodes []string `json:"process_codes,omitempty"`
}

type Output struct {
	Weights              Weights  `json:"weights"`
	SelectedIDs          []string `json:"selected_ids"`
	Items                []Item   `json:"items"`
	MatchVersion         string   `json:"match_version"`
	CatalogContentSHA256 string   `json:"catalog_content_sha256,omitempty"`
	TaskCodes            []string `json:"task_codes,omitempty"`
	// Best is the robot the calculation suggests and BestWhy says why, both filled by econ.Rank.
	Best    string   `json:"best,omitempty"`
	BestWhy []string `json:"best_why,omitempty"`
}

func DefaultWeights() Weights {
	return Weights{
		Fit:          WeightFit,
		ProcessMatch: WeightProcess,
		DataQuality:  WeightDataQuality,
		PriceBand:    WeightPrice,
	}
}

func Empty() Output {
	return Output{
		Weights:      DefaultWeights(),
		SelectedIDs:  []string{},
		Items:        []Item{},
		MatchVersion: Version,
	}
}

// Brief returns a copy without per-item reasons and rule steps. Lists stay empty, not null.
func (o Output) Brief() Output {
	items := make([]Item, len(o.Items))
	for i, it := range o.Items {
		it.Reasons = []string{}
		it.Hard = []Step{}
		it.Soft = []Step{}
		it.MissingEvidence = []Step{}
		it.Explanation = []Step{}
		items[i] = it
	}
	o.Items = items
	return o
}

func Classify(s Specs) DataQuality {
	missing := make([]string, 0, 5)
	reasons := make([]string, 0, 5)
	check := func(v *float64, field, reason string) {
		if v == nil {
			missing = append(missing, field)
			reasons = append(reasons, reason)
		}
	}
	check(s.PayloadKg, "payload_kg", "Нет грузоподъёмности.")
	check(s.WidthMm, "width_mm", "Нет ширины.")
	check(s.MinAisleMm, "min_aisle_mm", "Нет минимальной ширины проезда.")
	check(s.TempMinC, "temp_min_c", "Нет нижней рабочей температуры.")
	check(s.TempMaxC, "temp_max_c", "Нет верхней рабочей температуры.")
	if len(missing) == 0 {
		return DataQuality{Status: StatusOK, Missing: missing, Reasons: reasons}
	}
	return DataQuality{Status: StatusNeedsReview, Missing: missing, Reasons: reasons}
}

func Match(site Site, cands []Candidate, includeIDs []string) Output {
	want := make(map[string]struct{}, len(includeIDs))
	for _, id := range includeIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = struct{}{}
		}
	}
	present := make(map[string]struct{}, len(cands))
	items := make([]Item, 0, len(cands))
	for _, c := range cands {
		_, forced := want[c.ID]
		if c.Archived && !forced {
			continue
		}
		present[c.ID] = struct{}{}
		items = append(items, scoreOne(site, c, forced))
	}
	selected := make([]string, 0, len(includeIDs))
	seen := make(map[string]struct{}, len(includeIDs))
	for _, id := range includeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		if _, ok := present[id]; !ok {
			continue
		}
		seen[id] = struct{}{}
		selected = append(selected, id)
	}
	sort.SliceStable(items, func(i, j int) bool {
		oi, oj := statusOrder(items[i].Status), statusOrder(items[j].Status)
		if oi != oj {
			return oi < oj
		}
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].SolutionID < items[j].SolutionID
	})
	return Output{
		Weights:      DefaultWeights(),
		SelectedIDs:  selected,
		Items:        items,
		MatchVersion: Version,
		TaskCodes:    append([]string(nil), site.TaskCodes...),
	}
}

func scoreOne(site Site, c Candidate, forced bool) Item {
	if len(c.CapabilityCodes) == 0 {
		c.CapabilityCodes = catalog.DeriveCapabilities(catalog.SpecInput{
			Name:        c.Name,
			Family:      c.Family,
			Subtype:     deref(c.Subtype),
			Scenario:    deref(c.Scenario),
			ObjectTypes: c.ObjectTypes,
			PayloadKg:   c.PayloadKg,
			WidthMm:     c.WidthMm,
			MinAisleMm:  c.MinAisleMm,
			TempMinC:    c.TempMinC,
			TempMaxC:    c.TempMaxC,
		})
	}
	phys := evalPhys(site, c)
	pk, processStep := classifyProcessStep(site.ObjectType, c)
	processScore := 0.4
	steps := append([]Step(nil), phys.steps...)
	var exclude, review []string
	exclude = append(exclude, phys.exclude...)
	review = append(review, phys.review...)
	steps = append(steps, processStep)
	switch pk {
	case processMatch:
		processScore = 1
	case processUnknown:
		processScore = 0.4
		review = append(review, processStep.Text)
	case processMismatch:
		processScore = 0
		exclude = append(exclude, processStep.Text)
	}
	taskSteps := evalTasks(site, c)
	steps = append(steps, taskSteps...)
	for _, st := range taskSteps {
		if st.Outcome == OutcomeFail && st.Kind == KindHard {
			exclude = append(exclude, st.Text)
		} else if st.Outcome != OutcomePass {
			review = append(review, st.Text)
		}
	}
	dqScore, dqSteps := dataQualitySteps(site.ObjectType, c)
	steps = append(steps, dqSteps...)
	for _, st := range dqSteps {
		if st.Outcome != OutcomePass {
			review = append(review, st.Text)
		}
	}
	priceScore, priceStep := priceStep(site, c)
	steps = append(steps, priceStep)
	if priceStep.Outcome != OutcomePass {
		review = append(review, priceStep.Text)
	}
	review = dedupe(review)
	exclude = dedupe(exclude)

	status := StatusRecommended
	if len(exclude) > 0 {
		status = StatusExcluded
	} else if len(review) > 0 || pk != processMatch {
		status = StatusNeedsReview
	}

	for i := range steps {
		steps[i] = withFieldSource(c, steps[i])
	}
	hard, soft, missing := splitSteps(steps)
	parts := ScoreParts{
		Fit:          round4(phys.fit),
		ProcessMatch: round4(processScore),
		DataQuality:  round4(dqScore),
		PriceBand:    round4(priceScore),
	}
	score := round4(WeightFit*parts.Fit + WeightProcess*parts.ProcessMatch + WeightDataQuality*parts.DataQuality + WeightPrice*parts.PriceBand)
	return Item{
		SolutionID:      c.ID,
		Name:            c.Name,
		Vendor:          c.Vendor,
		Kind:            c.Kind,
		Subtype:         c.Subtype,
		Family:          c.Family,
		PriceRub:        c.PriceRub,
		Status:          status,
		Reasons:         reasonsFromSteps(status, steps),
		Hard:            hard,
		Soft:            soft,
		MissingEvidence: missing,
		Explanation:     steps,
		ScoreParts:      parts,
		Score:           score,
		Forced:          forced,
	}
}

func statusOrder(status string) int {
	switch status {
	case StatusRecommended:
		return 0
	case StatusNeedsReview:
		return 1
	default:
		return 2
	}
}

type physResult struct {
	exclude []string
	review  []string
	steps   []Step
	fit     float64
}

func evalPhys(site Site, c Candidate) physResult {
	var p physResult
	var parts []float64
	failed := false
	cleaner := isCleaner(c)

	if site.AisleMm != nil {
		aisleField := site.AisleField
		if aisleField == "" {
			aisleField = "aisle_working_m"
		}
		if c.WidthMm != nil && *c.WidthMm > *site.AisleMm {
			failed = true
			text := "Ширина робота " + fmtNum(*c.WidthMm) + " мм больше проезда объекта " + fmtNum(*site.AisleMm) + " мм. Решение не проходит. Выберите более узкую модель или добавьте к сравнению с предупреждением."
			p.exclude = append(p.exclude, text)
			p.steps = append(p.steps, aisleStep(c, aisleField, site.AisleMm, "width_mm", c.WidthMm, KindHard, OutcomeFail, text))
			parts = append(parts, 0)
		} else if c.MinAisleMm != nil && *c.MinAisleMm > *site.AisleMm {
			failed = true
			text := "Минимальный проезд робота " + fmtNum(*c.MinAisleMm) + " мм больше проезда объекта " + fmtNum(*site.AisleMm) + " мм. Решение не проходит. Выберите более узкую модель или добавьте к сравнению с предупреждением."
			p.exclude = append(p.exclude, text)
			p.steps = append(p.steps, aisleStep(c, aisleField, site.AisleMm, "min_aisle_mm", c.MinAisleMm, KindHard, OutcomeFail, text))
			parts = append(parts, 0)
		} else if c.WidthMm == nil && c.MinAisleMm == nil {
			text := "Нет ширины и минимального проезда в ТТХ. Нельзя проверить габарит. Уточните ТТХ или добавьте к сравнению с предупреждением."
			p.review = append(p.review, text)
			p.steps = append(p.steps, aisleStep(c, aisleField, site.AisleMm, "width_mm", nil, KindMissing, OutcomeUnknown, text))
			parts = append(parts, 0.4)
		} else {
			need := c.MinAisleMm
			field := "min_aisle_mm"
			if need == nil {
				need = c.WidthMm
				field = "width_mm"
			}
			margin := (*site.AisleMm - *need) / *site.AisleMm
			parts = append(parts, clamp(0.55+0.45*margin, 0, 1))
			what := "минимального проезда"
			if field == "width_mm" {
				what = "ширины"
			}
			text := "Проезд объекта " + fmtNum(*site.AisleMm) + " мм больше " + what + " робота " + fmtNum(*need) + " мм."
			p.steps = append(p.steps, aisleStep(c, aisleField, site.AisleMm, field, need, KindHard, OutcomePass, text))
			if c.MinAisleMm == nil {
				miss := "Нет минимальной ширины проезда в ТТХ. Проверили только ширину корпуса. Уточните проезд: пока решение нельзя считать рекомендованным."
				p.review = append(p.review, miss)
				p.steps = append(p.steps, aisleStep(c, aisleField, site.AisleMm, "min_aisle_mm", nil, KindMissing, OutcomeUnknown, miss))
			}
		}
	}

	if c.PayloadKg == nil {
		if !cleaner {
			text := "Нет грузоподъёмности в ТТХ. Нельзя подтвердить нагрузку. Уточните ТТХ или добавьте к сравнению с предупреждением."
			p.review = append(p.review, text)
			p.steps = append(p.steps, payloadStep(c, site, KindMissing, OutcomeUnknown, text))
			parts = append(parts, 0.4)
		}
	} else if site.LightLoadKg != nil && *c.PayloadKg < *site.LightLoadKg {
		failed = true
		text := "Грузоподъёмность " + fmtNum(*c.PayloadKg) + " кг меньше массы единицы " + fmtNum(*site.LightLoadKg) + " кг. Решение не тянет груз объекта. Выберите модель с большей грузоподъёмностью или добавьте к сравнению с предупреждением."
		p.exclude = append(p.exclude, text)
		p.steps = append(p.steps, payloadStep(c, site, KindHard, OutcomeFail, text))
		parts = append(parts, 0)
	} else if site.HeavyLoadKg != nil && *c.PayloadKg < *site.HeavyLoadKg {
		text := "Грузоподъёмность " + fmtNum(*c.PayloadKg) + " кг меньше массы паллеты или тележки " + fmtNum(*site.HeavyLoadKg) + " кг. Для тяжёлой логистики не подходит; можно сравнить как лёгкую доставку."
		p.review = append(p.review, text)
		st := payloadStep(c, site, KindSoft, OutcomeFail, text)
		st.ObjectField = site.HeavyField
		st.ObjectValue = site.HeavyLoadKg
		st.ObjectUnit = "kg"
		p.steps = append(p.steps, st)
		parts = append(parts, 0.55)
	} else if site.LightLoadKg != nil || site.HeavyLoadKg != nil {
		text := "Грузоподъёмность " + fmtNum(*c.PayloadKg) + " кг покрывает нагрузку объекта."
		p.steps = append(p.steps, payloadStep(c, site, KindHard, OutcomePass, text))
		parts = append(parts, 1)
	}

	if site.TempC != nil {
		tempField := site.TempField
		if tempField == "" {
			tempField = "apron_temp_c"
		}
		if c.TempMinC == nil || c.TempMaxC == nil {
			text := "Нет рабочего диапазона температур в ТТХ. Нельзя проверить температуру объекта. Уточните ТТХ или добавьте к сравнению с предупреждением."
			p.review = append(p.review, text)
			p.steps = append(p.steps, Step{
				RuleID: "temp_range", Kind: KindMissing, Outcome: OutcomeUnknown,
				ObjectField: tempField, ObjectValue: site.TempC, ObjectUnit: "°C",
				SolutionField: "temp_min_c", SourceURL: c.SourceURL, Confidence: c.Confidence, Text: text,
			})
			parts = append(parts, 0.4)
		} else if *site.TempC < *c.TempMinC || *site.TempC > *c.TempMaxC {
			failed = true
			text := "Температура объекта " + fmtNum(*site.TempC) + " °C вне диапазона робота " + fmtNum(*c.TempMinC) + "..." + fmtNum(*c.TempMaxC) + " °C. Для этих условий решение не подходит."
			p.exclude = append(p.exclude, text)
			p.steps = append(p.steps, Step{
				RuleID: "temp_range", Kind: KindHard, Outcome: OutcomeFail,
				ObjectField: tempField, ObjectValue: site.TempC, ObjectUnit: "°C",
				SolutionField: "temp_min_c", SolutionValue: c.TempMinC, SolutionUnit: "°C",
				SourceURL: c.SourceURL, Confidence: c.Confidence, Text: text,
			})
			parts = append(parts, 0)
		} else {
			text := "Температура объекта " + fmtNum(*site.TempC) + " °C внутри диапазона робота " + fmtNum(*c.TempMinC) + "..." + fmtNum(*c.TempMaxC) + " °C."
			p.steps = append(p.steps, Step{
				RuleID: "temp_range", Kind: KindHard, Outcome: OutcomePass,
				ObjectField: tempField, ObjectValue: site.TempC, ObjectUnit: "°C",
				SolutionField: "temp_min_c", SolutionValue: c.TempMinC, SolutionUnit: "°C",
				SourceURL: c.SourceURL, Confidence: c.Confidence, Text: text,
			})
			parts = append(parts, 1)
		}
	}

	body := bodyRules(site, c)
	p.exclude = append(p.exclude, body.exclude...)
	p.review = append(p.review, body.review...)
	p.steps = append(p.steps, body.steps...)
	if len(body.exclude) > 0 {
		failed = true
	}

	if failed {
		p.fit = 0
	} else if len(parts) == 0 {
		p.fit = 0.4
	} else {
		sum := 0.0
		for _, x := range parts {
			sum += x
		}
		p.fit = sum / float64(len(parts))
	}
	return p
}

func aisleStep(c Candidate, objectField string, objectMm *float64, solutionField string, solutionMm *float64, kind, outcome, text string) Step {
	return Step{
		RuleID: "aisle_width", Kind: kind, Outcome: outcome,
		ObjectField: objectField, ObjectValue: objectMm, ObjectUnit: "mm",
		SolutionField: solutionField, SolutionValue: solutionMm, SolutionUnit: "mm",
		SourceURL: c.SourceURL, Confidence: c.Confidence, Text: text,
	}
}

func payloadStep(c Candidate, site Site, kind, outcome, text string) Step {
	field := site.LightField
	val := site.LightLoadKg
	if field == "" {
		field = "unit_mass_kg"
	}
	return Step{
		RuleID: "payload_kg", Kind: kind, Outcome: outcome,
		ObjectField: field, ObjectValue: val, ObjectUnit: "kg",
		SolutionField: "payload_kg", SolutionValue: c.PayloadKg, SolutionUnit: "kg",
		SourceURL: c.SourceURL, Confidence: c.Confidence, Text: text,
	}
}

func dataQualitySteps(objectType string, c Candidate) (float64, []Step) {
	keys := keyFields(objectType, isCleaner(c))
	steps := make([]Step, 0, len(keys)+1)
	missingN := 0
	for _, k := range keys {
		if !hasKey(c, k) {
			missingN++
			steps = append(steps, Step{
				RuleID: "data_quality", Kind: KindMissing, Outcome: OutcomeUnknown,
				SolutionField: k, SourceURL: c.SourceURL, Confidence: c.Confidence,
				Text: keyMissingReason(k),
			})
		}
	}
	if len(keys) == 0 {
		return 1, steps
	}
	if missingN > 0 {
		return 1 - float64(missingN)/float64(len(keys)), steps
	}
	switch strings.ToLower(strings.TrimSpace(deref(c.Confidence))) {
	case "assumed":
		text := "ТТХ заполнены с пометкой оценка. Проверьте цифры перед выбором."
		steps = append(steps, Step{
			RuleID: "ttx_confidence", Kind: KindSoft, Outcome: OutcomeFail,
			SolutionField: "confidence", SourceURL: c.SourceURL, Confidence: c.Confidence, Text: text,
		})
		return 0.6, steps
	case "vendor", "measured":
		steps = append(steps, Step{
			RuleID: "ttx_confidence", Kind: KindSoft, Outcome: OutcomePass,
			SolutionField: "confidence", SourceURL: c.SourceURL, Confidence: c.Confidence,
			Text: "Источник ТТХ: " + confidenceText[strings.ToLower(strings.TrimSpace(deref(c.Confidence)))] + ".",
		})
		return 1, steps
	default:
		return 0.8, steps
	}
}

func keyFields(objectType string, cleaner bool) []string {
	var keys []string
	switch objectType {
	case "warehouse", "hospital":
		keys = []string{"payload_kg", "width_mm", "min_aisle_mm"}
	case "airport":
		keys = []string{"payload_kg", "width_mm", "temp_min_c", "temp_max_c"}
	default:
		keys = []string{"payload_kg", "width_mm"}
	}
	if !cleaner {
		return keys
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != "payload_kg" {
			out = append(out, k)
		}
	}
	return out
}

func hasKey(c Candidate, k string) bool {
	switch k {
	case "payload_kg":
		return c.PayloadKg != nil
	case "width_mm":
		return c.WidthMm != nil
	case "min_aisle_mm":
		return c.MinAisleMm != nil
	case "temp_min_c":
		return c.TempMinC != nil
	case "temp_max_c":
		return c.TempMaxC != nil
	default:
		return true
	}
}

func keyMissingReason(k string) string {
	switch k {
	case "payload_kg":
		return "Нет грузоподъёмности в ТТХ. Полнота данных недостаточна для рекомендации."
	case "width_mm":
		return "Нет ширины в ТТХ. Полнота данных недостаточна для рекомендации."
	case "min_aisle_mm":
		return "Нет минимальной ширины проезда в ТТХ. Полнота данных недостаточна для рекомендации."
	case "temp_min_c":
		return "Нет нижней рабочей температуры в ТТХ. Полнота данных недостаточна для рекомендации."
	case "temp_max_c":
		return "Нет верхней рабочей температуры в ТТХ. Полнота данных недостаточна для рекомендации."
	default:
		return "Не хватает поля " + k + " в ТТХ."
	}
}

func priceStep(site Site, c Candidate) (float64, Step) {
	field := site.BudgetField
	if field == "" {
		field = "capex_budget_mln_rub"
	}
	base := Step{
		RuleID: "price_budget", Kind: KindSoft,
		ObjectField: field, ObjectValue: site.BudgetRub, ObjectUnit: "rub",
		SolutionField: "price_rub", SolutionValue: c.PriceRub, SolutionUnit: "rub",
		SourceURL: c.SourceURL, Confidence: c.Confidence,
	}
	if c.PriceRub == nil {
		base.Outcome = OutcomeUnknown
		base.Kind = KindMissing
		base.Text = "Нет цены в каталоге. Ценовой пояс условный. Уточните цену или добавьте к сравнению с предупреждением."
		return 0.5, base
	}
	if site.BudgetRub == nil || *site.BudgetRub <= 0 {
		base.Outcome = OutcomeUnknown
		base.Text = "Нет бюджета CAPEX в параметрах. Цена не сравнивалась с лимитом."
		return 0.5, base
	}
	price := *c.PriceRub
	budget := *site.BudgetRub
	if price <= budget {
		base.Outcome = OutcomePass
		base.Text = "Цена изделия " + fmtRub(price) + " не выше бюджета CAPEX " + fmtRub(budget) + "."
		return 1, base
	}
	if price >= 3*budget {
		base.Outcome = OutcomeFail
		base.Text = "Цена изделия выше тройного бюджета CAPEX. Проверьте, что модель вообще укладывается в бюджет."
		return 0, base
	}
	base.Outcome = OutcomePass
	base.Text = "Цена изделия выше бюджета, но меньше тройного лимита."
	return 1 - (price-budget)/(2*budget), base
}

func isCleaner(c Candidate) bool {
	return catalog.IsCleaner(c.Name, deref(c.Subtype), c.Family, deref(c.Scenario))
}

func evalTasks(site Site, c Candidate) []Step {
	if len(site.TaskCodes) == 0 {
		return nil
	}
	out := make([]Step, 0, len(site.TaskCodes))
	for _, task := range site.TaskCodes {
		req := catalog.RequiredCapability(task)
		if req == "" {
			continue
		}
		st := Step{
			RuleID: "task_capability", Kind: KindSoft, TaskCode: task, CapabilityCode: req,
			SourceURL: c.SourceURL, Confidence: c.Confidence,
		}
		if catalog.HasCapability(c.CapabilityCodes, req) {
			st.Outcome = OutcomePass
			st.Text = "Задача «" + catalog.CodeLabel(task) + "» закрывается возможностью «" + catalog.CodeLabel(req) + "»."
			out = append(out, st)
			continue
		}
		if taskNeedsPayload(req) && c.PayloadKg == nil && !isCleaner(c) {
			st.Kind = KindMissing
			st.Outcome = OutcomeUnknown
			st.SolutionField = "payload_kg"
			st.Text = "Нет грузоподъёмности, нельзя подтвердить возможность «" + catalog.CodeLabel(req) + "» для задачи «" + catalog.CodeLabel(task) + "»."
			out = append(out, st)
			continue
		}
		st.Outcome = OutcomeFail
		st.Text = "Для задачи «" + catalog.CodeLabel(task) + "» нужна возможность «" + catalog.CodeLabel(req) + "». У этой модели её нет."
		out = append(out, st)
	}
	return out
}

func taskNeedsPayload(cap string) bool {
	return cap == catalog.CapPayloadPallet || cap == catalog.CapPayloadUnit
}

func dedupe(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func fmtNum(v float64) string {
	return rutext.Num(v, 2)
}

func fmtRub(v float64) string {
	return rutext.Num(math.Round(v), 0) + "\u00a0₽"
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
