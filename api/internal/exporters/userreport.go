package exporters

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/rutext"
)

// The PDF and the XLSX of a run are drawn from the same slice built here, so the two files never disagree.

const (
	maxWarnings   = 8
	warningLength = 320

	financialNote = "Оценка недисконтированная и не является бухгалтерской отчётностью. Затраты за горизонт даны в ценах базового года: " +
		"первоначальные вложения, ежегодные затраты за весь горизонт, замены батарей и повторная закупка роботов. Остаточная стоимость оборудования не учитывается."

	surveyNote = "Результат является предварительной оценкой и требует проверки при обследовании объекта."

	noPayback  = "Не окупается на заданных условиях"
	noVariants = "В проекте нет вариантов для сравнения. Добавьте роботов в вариант и пересчитайте проект."
)

type userHeader struct {
	Project string
	Object  string
	// RunDate is when the calculation or the simulation was made; RunLabel names it for that kind of report.
	RunLabel   string
	RunDate    string
	Date       string
	Confidence string
}

func headerOf(r Report) userHeader {
	name, meaning := ConfidenceLabel(r.ConfidenceLevel())
	project := r.ProjectName
	if project == "" {
		project = "Не сохранён"
	}
	run, label := r.generatedAt(), "Дата расчёта"
	if r.Run != nil && !r.Run.CreatedAt.IsZero() {
		run = r.Run.CreatedAt
	}
	if r.Kind() == KindSimulation {
		label = "Дата запуска"
	}
	return userHeader{
		Project: project, Object: objectLabel(r.ObjectType), RunLabel: label, RunDate: reportDate(run),
		Date: reportDate(r.generatedAt()), Confidence: name + ". " + meaning,
	}
}

func (h userHeader) rows() [][2]string {
	return [][2]string{{"Проект", h.Project}, {"Объект", h.Object}, {h.RunLabel, h.RunDate}, {"Дата отчёта", h.Date}, {"Достоверность", h.Confidence}}
}

// userScenario is the baseline or one variant with one way to pay for it.
type userScenario struct {
	Key          string
	Variant      string
	Payment      string
	Fleet        string
	Baseline     bool
	Capex        *float64
	Opex         *float64
	AnnualEffect *float64
	Payback      *float64
	HorizonCost  *float64
}

type userDetail struct {
	Scenario string
	Section  string
	Label    string
	Rub      float64
	Note     string
}

type userFinancialReport struct {
	Header      userHeader
	Scenarios   []userScenario
	Best        *userScenario
	Verdict     string
	Details     []userDetail
	Assumptions []string
	Warnings    []string
	Params      []paramRow
	Sources     []string
	SimChecks   []userSimCheck
}

// userSimCheck is the check of one variant by simulation, in the words the report prints.
type userSimCheck struct {
	Variant string
	Method  string
	Result  string
}

func simCheckRows(checks []econ.SimCheck) []userSimCheck {
	out := make([]userSimCheck, 0, len(checks))
	for _, c := range checks {
		method := "Симуляция на карте"
		if c.Model == econ.SimCheckQuick {
			method = "Быстрая проверка без карты"
		}
		var result string
		switch {
		case c.RunID == "" && c.Model == econ.SimCheckMap:
			result = "Не запускалась. Запустите симуляцию варианта на вкладке «Симуляция»."
		case c.Stale:
			result = "Устарела: вариант или объект изменились после запуска. " + c.Text
		case c.Flag:
			result = "Расхождение. " + c.Text
		default:
			result = "Сходится. " + c.Text
		}
		out = append(out, userSimCheck{Variant: c.VariantName, Method: method, Result: strings.TrimSpace(result)})
	}
	return out
}

func financialReport(r Report) userFinancialReport {
	if r.Econ == nil {
		return userFinancialReport{Header: headerOf(r)}
	}
	res := r.Econ
	variants := map[string]econ.VariantResult{}
	for _, v := range res.Variants {
		variants[v.VariantID] = v
	}
	out := userFinancialReport{
		Header: headerOf(r), Assumptions: financialAssumptions(res), Warnings: financialWarnings(r),
		Params: objectParams(r), Sources: financialSources(r), SimChecks: simCheckRows(res.SimChecks),
	}
	for _, sc := range res.Scenarios {
		item := userScenario{
			Key:          scenarioKey(sc),
			Variant:      scenarioName(sc),
			Payment:      financingLabel(sc.Kind, sc.Tariff),
			Baseline:     sc.Kind == "baseline",
			Capex:        sc.CapexRub,
			Opex:         sc.OpexYearRub,
			AnnualEffect: sc.AnnualEffectRub,
			Payback:      sc.PaybackYears,
			HorizonCost:  sc.TcoRub,
		}
		switch v, ok := variants[sc.VariantID]; {
		case ok:
			item.Fleet = fleetText(v)
		case sc.VariantID == "" && res.SolutionName != "" && sc.FleetSize != nil && *sc.FleetSize > 0:
			item.Fleet = res.SolutionName + ", " + strconv.Itoa(*sc.FleetSize) + " шт."
		}
		if item.Baseline {
			item.Variant = "База"
			item.Payment = "Текущий процесс"
		}
		out.Scenarios = append(out.Scenarios, item)
	}
	out.Best, out.Verdict = bestScenario(out.Scenarios, res.HorizonYears)
	out.Details = scenarioDetails(res, out.Scenarios)
	return out
}

func scenarioName(sc econ.Scenario) string {
	if sc.Kind == "baseline" {
		return "База проекта"
	}
	if sc.VariantName != "" {
		return sc.VariantName
	}
	return "Выбранное решение"
}

func scenarioKey(sc econ.Scenario) string {
	if sc.Kind == "baseline" {
		return "baseline"
	}
	key := sc.VariantID
	if key == "" {
		key = sc.Kind
	}
	key += ":" + sc.Kind
	if sc.Kind == "raas" && sc.Tariff != "" {
		key += ":" + sc.Tariff
	}
	return key
}

func scenarioVariantKey(key string) string {
	if i := strings.IndexByte(key, ':'); i >= 0 {
		return key[:i]
	}
	return key
}

func fleetText(v econ.VariantResult) string {
	parts := make([]string, 0, len(v.Fleet))
	for _, f := range v.Fleet {
		if f.Name == "" || f.Quantity < 1 {
			continue
		}
		parts = append(parts, f.Name+", "+strconv.Itoa(f.Quantity)+" шт.")
	}
	text := strings.Join(parts, "; ")
	if v.Chargers > 0 {
		line := "Зарядных станций: " + strconv.Itoa(v.Chargers)
		if text == "" {
			return line
		}
		return text + ". " + line
	}
	return text
}

// bestScenario picks the non-baseline scenario with a positive annual effect and the shortest simple payback within
// the horizon. Equal paybacks pick none: the report says so instead of choosing one variant arbitrarily. A horizon of
// 0 means no limit.
func bestScenario(items []userScenario, horizon float64) (*userScenario, string) {
	var best *userScenario
	tied, compared, late := false, false, false
	for i := range items {
		item := &items[i]
		if item.Baseline {
			continue
		}
		compared = true
		if item.Payback == nil || item.AnnualEffect == nil || *item.AnnualEffect <= 0 {
			continue
		}
		if horizon > 0 && *item.Payback > horizon+1e-9 {
			late = true
			continue
		}
		switch {
		case best == nil || *item.Payback < *best.Payback-1e-9:
			best, tied = item, false
		case math.Abs(*item.Payback-*best.Payback) <= 1e-9:
			tied = true
		}
	}
	switch {
	case !compared:
		return nil, noVariants
	case best == nil && late:
		return nil, "Ни один вариант не окупается за горизонт оценки (" + yearsText(horizon) + "). Отчёт не выделяет лучший вариант."
	case best == nil:
		return nil, noPayback + "."
	case tied:
		return nil, "Несколько вариантов имеют одинаковую простую окупаемость: " + yearsText(*best.Payback) + ". Отчёт не выделяет один из них."
	}
	return best, "Лучшая простая окупаемость у варианта «" + best.Variant + "», оплата: " + lowerFirst(best.Payment) + ", " + yearsText(*best.Payback) + "."
}

// lowerFirst lowers the first letter of a word in a sentence and keeps the acronym RaaS.
func lowerFirst(s string) string {
	if s == "" || strings.HasPrefix(s, "RaaS") {
		return s
	}
	r := []rune(s)
	return strings.ToLower(string(r[0])) + string(r[1:])
}

var sectionOrder = map[string]int{
	"Первоначальные вложения": 0,
	"Ежегодные затраты":       1,
	"Замены за горизонт":      2,
}

// scenarioDetails lists the cost lines of every variant by section, in the order of the comparison table.
func scenarioDetails(res *econ.Result, scenarios []userScenario) []userDetail {
	var out []userDetail
	seen := map[string]bool{}
	for _, sc := range scenarios {
		if sc.Baseline {
			continue
		}
		for _, line := range res.Breakdown {
			if line.Scenario != sc.Key && line.Scenario != scenarioVariantKey(sc.Key) {
				continue
			}
			key := sc.Key + "\x00" + line.Bucket + "\x00" + line.Label
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, userDetail{Scenario: detailScenario(sc), Section: sectionOf(line.Bucket), Label: line.Label, Rub: line.Rub, Note: line.Note})
		}
	}
	order := map[string]int{}
	for i, sc := range scenarios {
		order[detailScenario(sc)] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := order[out[i].Scenario], order[out[j].Scenario]; a != b {
			return a < b
		}
		return sectionOrder[out[i].Section] < sectionOrder[out[j].Section]
	})
	return out
}

func detailScenario(sc userScenario) string {
	return sc.Variant + ", " + sc.Payment
}

func sectionOf(bucket string) string {
	switch bucket {
	case "capex":
		return "Первоначальные вложения"
	case "replacement":
		return "Замены за горизонт"
	default:
		return "Ежегодные затраты"
	}
}

// financialAssumptions keeps only what can change the decision: the horizon, VAT and the share of labor savings
// that turns into cash.
func financialAssumptions(res *econ.Result) []string {
	var items []string
	if res.HorizonYears > 0 {
		items = append(items, "Горизонт оценки: "+yearsText(res.HorizonYears)+".")
	}
	if a := res.AssumptionSet; a != nil {
		prices := "без НДС"
		if a.PricesIncludeVAT {
			prices = "с НДС"
		}
		vat, share := a.VATRate*100, a.LaborCashShare*100
		items = append(items,
			"НДС "+formatNum(&vat, 0)+"%, цены "+prices+", НДС к вычету: "+yesNo(a.VATRecoverable)+".",
			"Денежная доля экономии труда: "+formatNum(&share, 0)+"%.",
		)
	}
	return items
}

// financialWarnings lists what can change the conclusion: the run and fleet warnings, processes that give no saving
// and object parameters nobody has entered. The list is short; the rest stays in the project.
func financialWarnings(r Report) []string {
	var out []string
	for _, n := range Notices(r) {
		if n.Level == LevelHigh || n.Level == LevelWarning {
			out = append(out, shorten(n.Text, warningLength))
		}
	}
	for _, p := range r.Inputs.Processes {
		if !p.IsBaseline {
			continue
		}
		if p.Demand.UnitsPerDay <= 0 {
			out = append(out, "Процесс «"+p.Name+"»: спрос не задан, процесс не даёт экономии.")
		} else if p.BaselineStaff.Headcount <= 0 {
			out = append(out, "Процесс «"+p.Name+"»: базовая численность не задана, экономия труда по нему не считается.")
		}
	}
	if line := unknownParams(r); line != "" {
		out = append(out, line)
	}
	if len(out) > maxWarnings {
		rest := len(out) - maxWarnings
		out = append(out[:maxWarnings], "Ещё предупреждений: "+rutext.Num(float64(rest), 0)+". Полный список в расчёте проекта.")
	}
	return out
}

// paramRow is one object parameter as the report prints it. Value is the text a person reads, Raw the number when the
// parameter is one.
type paramRow struct {
	Group string
	Label string
	Unit  string
	Value string
	Raw   any
}

// name is the label with its unit, as the PDF prints it.
func (p paramRow) name() string {
	if p.Unit == "" {
		return p.Label
	}
	return p.Label + ", " + p.Unit
}

// objectParams lists every parameter of the object in the order of the form, with the value the project holds.
func objectParams(r Report) []paramRow {
	schema, err := objects.SchemaFor(r.ObjectType)
	if err != nil {
		return nil
	}
	vals := paramsMap(r.Inputs.Params)
	var out []paramRow
	for _, g := range schema.Groups {
		for _, f := range g.Fields {
			row := paramRow{Group: g.Label, Label: f.Label}
			if f.Unit != "" && f.Unit != "-" {
				row.Unit = f.Unit
			}
			v, ok := vals[f.ID]
			row.Value = paramText(f, v, ok)
			if n, isNum := v.(float64); isNum && (f.Type == objects.TypeNumber || f.Type == objects.TypeInteger) {
				row.Raw = n
			}
			out = append(out, row)
		}
	}
	return out
}

// paramText writes a value the way the object form does: grouped digits, the label of a choice, Да or Нет.
func paramText(f objects.Field, v any, present bool) string {
	label := func(value string) string {
		for _, o := range f.Options {
			if o.Value == value {
				return o.Label
			}
		}
		return value
	}
	switch x := v.(type) {
	case nil:
		if !present {
			return "не задано"
		}
		return objects.UnknownLabel
	case bool:
		if x {
			return "Да"
		}
		return "Нет"
	case float64:
		return rutext.Num(x, 6)
	case string:
		if f.Type == objects.TypeEnum {
			return label(x)
		}
		return x
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				parts = append(parts, label(s))
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		if start, ok := x["start"].(string); ok {
			end, _ := x["end"].(string)
			return start + " - " + end
		}
		size := func(key string) string {
			n, _ := x[key].(float64)
			return rutext.Num(n, 6)
		}
		return size("length") + " x " + size("width") + " x " + size("height")
	}
	return ""
}

// financialSources lists where the numbers come from: the model's own sources and, for every robot in the
// comparison, the page its specs were taken from.
func financialSources(r Report) []string {
	var out []string
	for _, s := range r.Econ.Sources {
		if !strings.Contains(strings.ToLower(s), "дисконтир") {
			out = append(out, s)
		}
	}
	for _, it := range r.Catalog.Items {
		out = append(out, robotSource(it))
	}
	return out
}

var confidenceNames = map[string]string{
	"measured": "измерения",
	"vendor":   "данные производителя",
	"assumed":  "оценка",
}

func robotSource(it CatalogItem) string {
	line := it.Name + ": "
	if it.SourceURL == "" {
		line += "ссылка на источник не указана"
	} else {
		line += it.SourceURL
	}
	if name := confidenceNames[strings.ToLower(strings.TrimSpace(it.Confidence))]; name != "" {
		line += ". Источник ТТХ: " + name
		if it.SourcedAt != nil {
			line += ", " + reportDate(*it.SourcedAt)
		}
	}
	return line + "."
}

const shownParams = 4

func unknownParams(r Report) string {
	schema, err := objects.SchemaFor(r.ObjectType)
	if err != nil {
		return ""
	}
	vals := paramsMap(r.Inputs.Params)
	var names []string
	for _, f := range objects.Fields(schema) {
		v, ok := vals[f.ID]
		if (ok && v != nil) || (!ok && !f.Required) {
			continue
		}
		names = append(names, f.Label)
	}
	if len(names) == 0 {
		return ""
	}
	shown := names
	if len(shown) > shownParams {
		shown = shown[:shownParams]
	}
	line := "Не заданы параметры объекта: " + strings.Join(shown, ", ")
	if rest := len(names) - len(shown); rest > 0 {
		line += " и ещё " + rutext.Num(float64(rest), 0)
	}
	return line + ". Уточните их: от них зависит расчёт."
}

// shorten cuts a text at the last sentence end within n characters, or at n with «...».
func shorten(text string, n int) string {
	runes := []rune(text)
	if len(runes) <= n {
		return text
	}
	head := string(runes[:n])
	if i := strings.LastIndex(head, ". "); i > len(head)/3 {
		return head[:i+1]
	}
	return strings.TrimSpace(head) + "..."
}

type userSimReport struct {
	Header      userHeader
	Variant     string
	Verdict     string
	KPI         [][2]string
	Processes   [][]string
	Bottlenecks [][2]string
	Notes       []string
	Limits      []string
}

// simLimits are the limits of the movement model a reader of a simulation needs to know.
var simLimits = []string{
	"Путь робота выбирается по кратчайшему расстоянию без учёта загрузки участков.",
	"Конфликты между роботами учитываются только на узких участках, доках и зарядке.",
}

// simVerdict puts the verdict word before its explanation unless the explanation already starts with it.
func simVerdict(verdict, text string) string {
	label := verdictLabel(verdict)
	switch {
	case text == "":
		return label
	case strings.HasPrefix(text, label):
		return text
	}
	return label + ". " + text
}

func simulationReport(r Report) userSimReport {
	s := r.Sim
	if s == nil {
		return userSimReport{Header: headerOf(r)}
	}
	res := s.Result
	k := res.KPI
	out := userSimReport{
		Header:  headerOf(r),
		Variant: s.VariantName,
		Verdict: simVerdict(res.Verdict, res.VerdictText),
		KPI: [][2]string{
			{"Допуск нарушений SLA", pct(res.SLATargetPct / 100)},
			{"Выполнено заданий", num(k.Completed.Median, 0)},
			{"Нарушения SLA", pct(k.ViolationRate.Median)},
			{"Производительность", num(k.ThroughputPerH.Median, 1) + " заданий/ч"},
		},
	}
	for _, p := range res.Processes {
		status := "Не моделируется"
		if p.Covered {
			status = "Моделируется"
		}
		out.Processes = append(out.Processes, []string{p.Name, status, num(p.Completed.Median, 0), pct(p.ViolationRate.Median), duration(p.WaitP95S.Median), duration(p.CycleP95S.Median)})
	}
	for _, b := range res.Bottlenecks {
		out.Bottlenecks = append(out.Bottlenecks, [2]string{b.Name, b.Text})
	}
	for _, n := range Notices(r) {
		if (n.Level == LevelHigh || n.Level == LevelWarning) && n.Text != res.VerdictText && !strings.HasPrefix(n.Text, "Узкое место: ") {
			out.Notes = append(out.Notes, n.Text)
		}
	}
	out.Limits = simLimits
	return out
}
