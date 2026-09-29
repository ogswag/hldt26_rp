package exporters

import (
	"strings"
	"testing"

	"moscow_hackathon_2026/api/internal/econ"
)

func scenarioOf(name, payment string, payback, effect *float64) userScenario {
	return userScenario{Variant: name, Payment: payment, Payback: payback, AnnualEffect: effect}
}

func TestBestScenario(t *testing.T) {
	baseline := userScenario{Variant: "База", Baseline: true, Payback: fptr(0.1), AnnualEffect: fptr(1)}
	late := "Ни один вариант не окупается за горизонт оценки (5 лет). Отчёт не выделяет лучший вариант."
	cases := []struct {
		name     string
		items    []userScenario
		horizon  float64
		want     string
		contains string
	}{
		{"no scenarios", nil, 5, "", noVariants},
		{"baseline only", []userScenario{baseline}, 5, "", noVariants},
		{"shortest payback wins", []userScenario{
			baseline,
			scenarioOf("Один", "Покупка", fptr(3.2), fptr(10)),
			scenarioOf("Два", "RaaS, фиксированный", fptr(2.4), fptr(8)),
			scenarioOf("Три", "Покупка", fptr(4), fptr(20)),
		}, 5, "Два", "оплата: RaaS, фиксированный, 2,4 года"},
		{"baseline is never the best", []userScenario{baseline, scenarioOf("Один", "Покупка", fptr(3), fptr(10))}, 5, "Один", "оплата: покупка, 3 года"},
		{"equal payback picks none", []userScenario{
			scenarioOf("Один", "Покупка", fptr(2.5), fptr(10)),
			scenarioOf("Два", "RaaS, фиксированный", fptr(2.5), fptr(30)),
		}, 5, "", "одинаковую простую окупаемость: 2,5 года"},
		{"a longer third payback does not break the tie", []userScenario{
			scenarioOf("Один", "Покупка", fptr(2.5), fptr(10)),
			scenarioOf("Два", "Покупка", fptr(2.5), fptr(30)),
			scenarioOf("Три", "Покупка", fptr(6), fptr(30)),
		}, 5, "", "одинаковую"},
		{"a shorter payback after a tie wins", []userScenario{
			scenarioOf("Один", "Покупка", fptr(2.5), fptr(10)),
			scenarioOf("Два", "Покупка", fptr(2.5), fptr(30)),
			scenarioOf("Три", "Покупка", fptr(1), fptr(30)),
		}, 5, "Три", "1 год"},
		{"no payback", []userScenario{scenarioOf("Один", "Покупка", nil, fptr(10))}, 5, "", noPayback},
		{"a negative effect is not a candidate", []userScenario{
			scenarioOf("Один", "Покупка", fptr(1), fptr(-5)),
			scenarioOf("Два", "Покупка", fptr(4), fptr(5)),
		}, 5, "Два", "«Два»"},
		{"zero effect is not a candidate", []userScenario{scenarioOf("Один", "Покупка", fptr(1), fptr(0))}, 5, "", noPayback},
		{"payback beyond the horizon is not recommended", []userScenario{
			scenarioOf("Один", "Покупка", fptr(12), fptr(10)),
			scenarioOf("Два", "RaaS, фиксированный", fptr(8), fptr(10)),
		}, 5, "", late},
		{"a variant beyond the horizon is skipped", []userScenario{
			scenarioOf("Один", "Покупка", fptr(7), fptr(10)),
			scenarioOf("Два", "Покупка", fptr(4.9), fptr(10)),
		}, 5, "Два", "«Два»"},
		{"payback at the horizon counts", []userScenario{scenarioOf("Один", "Покупка", fptr(5), fptr(10))}, 5, "Один", "5 лет"},
		{"a tie beyond the horizon says nothing about a tie", []userScenario{
			scenarioOf("Один", "Покупка", fptr(9), fptr(10)),
			scenarioOf("Два", "Покупка", fptr(9), fptr(10)),
		}, 5, "", late},
		{"no payback next to a late one still speaks of the horizon", []userScenario{
			scenarioOf("Один", "Покупка", nil, fptr(10)),
			scenarioOf("Два", "Покупка", fptr(9), fptr(10)),
		}, 5, "", late},
		{"an unknown horizon sets no limit", []userScenario{scenarioOf("Один", "Покупка", fptr(25), fptr(10))}, 0, "Один", "25 лет"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			best, verdict := bestScenario(c.items, c.horizon)
			got := ""
			if best != nil {
				got = best.Variant
			}
			if got != c.want {
				t.Errorf("best %q, want %q", got, c.want)
			}
			if !strings.Contains(verdict, c.contains) {
				t.Errorf("verdict %q does not contain %q", verdict, c.contains)
			}
		})
	}
}

func TestNoPaybackVerdictSaysNothingIsRecommended(t *testing.T) {
	for _, item := range []userScenario{scenarioOf("Один", "Покупка", nil, fptr(10)), scenarioOf("Один", "Покупка", fptr(12), fptr(10))} {
		_, verdict := bestScenario([]userScenario{item}, 5)
		mustNotContain(t, "verdict", verdict, "Лучшая", "рекоменду")
	}
}

func TestPaybackBeyondHorizonIsNotHighlighted(t *testing.T) {
	r := calcReport(t)
	for i := range r.Econ.Scenarios {
		if r.Econ.Scenarios[i].Kind != "baseline" {
			r.Econ.Scenarios[i].PaybackYears = fptr(12.3)
		}
	}
	view := financialReport(r)
	if view.Best != nil {
		t.Fatalf("best %+v, want none", view.Best)
	}
	mustContain(t, "verdict", view.Verdict, "Ни один вариант не окупается за горизонт оценки (5 лет)")
	text, _ := pdfText(t, r)
	mustContain(t, "pdf", text, view.Verdict, "12,3 года")
	mustNotContain(t, "pdf", text, "Вариант с лучшей окупаемостью", "Состав затрат")
	summary := sheetText(t, openWorkbook(t, r), "Итог")
	mustContain(t, "workbook", summary, "Вывод\t"+view.Verdict)
}

func TestFinancialReportSlice(t *testing.T) {
	r := calcReport(t)
	view := financialReport(r)
	if len(view.Scenarios) != len(r.Econ.Scenarios) {
		t.Fatalf("%d scenarios, econ has %d", len(view.Scenarios), len(r.Econ.Scenarios))
	}
	if !view.Scenarios[0].Baseline || view.Scenarios[0].Variant != "База" || view.Scenarios[0].Payment != "Текущий процесс" {
		t.Fatalf("baseline row %+v", view.Scenarios[0])
	}
	if view.Best == nil || view.Best.Baseline {
		t.Fatalf("best %+v", view.Best)
	}
	for _, sc := range view.Scenarios {
		if sc.Baseline || sc.Payback == nil || sc.AnnualEffect == nil || *sc.AnnualEffect <= 0 {
			continue
		}
		if *sc.Payback < *view.Best.Payback {
			t.Errorf("%s pays back in %v, faster than the best %v", sc.Variant, *sc.Payback, *view.Best.Payback)
		}
	}
	if view.Best.Fleet != "AMR H1500, 4 шт.; Штабелёр S20, 2 шт.; Зарядных станций: 2" {
		t.Errorf("fleet %q", view.Best.Fleet)
	}
	if len(view.Details) == 0 {
		t.Fatal("cost lines missing")
	}
	sections := map[string]bool{}
	for _, d := range view.Details {
		sections[d.Section] = true
	}
	for _, s := range []string{"Первоначальные вложения", "Ежегодные затраты"} {
		if !sections[s] {
			t.Errorf("no cost lines in section %q", s)
		}
	}
}

func TestFinancialAssumptionsKeepDecisionFields(t *testing.T) {
	res := &econ.Result{HorizonYears: 5, AssumptionSet: &econ.AssumptionView{VATRate: 0.22, PricesIncludeVAT: true, VATRecoverable: false, LaborCashShare: 0.8, DiscountRate: 0.15}}
	got := strings.Join(financialAssumptions(res), "\n")
	mustContain(t, "assumptions", got, "Горизонт оценки: 5 лет.", "НДС 22%, цены с НДС, НДС к вычету: нет.", "Денежная доля экономии труда: 80%.")
	mustNotContain(t, "assumptions", got, "исконт", "15%")
	if n := len(financialAssumptions(res)); n != 3 {
		t.Errorf("%d assumption lines, want 3", n)
	}
}

func TestFinancialWarningsAreShortAndCapped(t *testing.T) {
	r := calcReport(t)
	warnings := financialWarnings(r)
	if len(warnings) == 0 {
		t.Fatal("the fixture must warn")
	}
	joined := strings.Join(warnings, "\n")
	mustContain(t, "warnings", joined, "Черновик проекта изменён после этого запуска", "Не заданы параметры объекта")
	mustContain(t, "warnings", joined, "Процесс «"+r.Inputs.Processes[3].Name+"»: спрос не задан")
	mustNotContain(t, "warnings", joined, internalMarks()...)

	long := strings.Repeat("Предложение о том, что проверить. ", 40)
	r.Econ.Risks = nil
	for i := 0; i < 20; i++ {
		r.Econ.Risks = append(r.Econ.Risks, econ.Risk{Level: LevelHigh, Text: long + string(rune('а'+i))})
	}
	capped := financialWarnings(r)
	if len(capped) != maxWarnings+1 {
		t.Fatalf("%d warnings, want %d and a tail", len(capped), maxWarnings+1)
	}
	if !strings.HasPrefix(capped[maxWarnings], "Ещё предупреждений: ") {
		t.Errorf("tail %q", capped[maxWarnings])
	}
	for _, w := range capped[:maxWarnings] {
		if len([]rune(w)) > warningLength+3 {
			t.Errorf("warning is %d characters: %q", len([]rune(w)), w)
		}
	}
}

func TestCriticalWarningsReachBothFiles(t *testing.T) {
	r := calcReport(t)
	r.Econ.Risks = []econ.Risk{{Level: LevelHigh, Text: "Окупаемость зависит от одного допущения о численности."}, {Level: LevelInfo, Text: "Справочная строка без последствий."}}
	text, _ := pdfText(t, r)
	mustContain(t, "pdf", text, "Окупаемость зависит от одного допущения о численности.")
	mustNotContain(t, "pdf", text, "Справочная строка без последствий.")
	notes := sheetText(t, openWorkbook(t, r), "Допущения и риски")
	mustContain(t, "workbook", notes, "Важно\tОкупаемость зависит от одного допущения о численности.")
	mustNotContain(t, "workbook", notes, "Справочная строка без последствий.")
}

func TestShorten(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"short text stays", "Коротко.", 20, "Коротко."},
		{"cut at the last sentence", "Первое предложение. Второе предложение. Третье предложение.", 45, "Первое предложение. Второе предложение."},
		{"no sentence end", "Одно очень длинное предложение без точки внутри", 12, "Одно очень д..."},
		{"multibyte text is cut by characters", "ёёёёёёёёёё", 4, "ёёёё..."},
	}
	for _, c := range cases {
		if got := shorten(c.in, c.n); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestScenarioDetailsFollowTableOrder(t *testing.T) {
	view := financialReport(calcReport(t))
	last := -1
	order := map[string]int{}
	for i, sc := range view.Scenarios {
		order[detailScenario(sc)] = i
	}
	for _, d := range view.Details {
		i, ok := order[d.Scenario]
		if !ok {
			t.Fatalf("detail of an unknown scenario %q", d.Scenario)
		}
		if i < last {
			t.Fatalf("details are not in table order at %q", d.Scenario)
		}
		last = i
	}
}

func TestSimVerdict(t *testing.T) {
	cases := []struct{ verdict, text, want string }{
		{"pass", "", "SLA выполняется"},
		{"pass", "Нарушений нет.", "SLA выполняется. Нарушений нет."},
		{"fail", "SLA нарушается: 12% заданий дольше допуска.", "SLA нарушается: 12% заданий дольше допуска."},
		{"fail", "12% заданий дольше допуска.", "SLA нарушается. 12% заданий дольше допуска."},
	}
	for _, c := range cases {
		if got := simVerdict(c.verdict, c.text); got != c.want {
			t.Errorf("%s %q: %q, want %q", c.verdict, c.text, got, c.want)
		}
	}
}

func TestSimulationReportSlice(t *testing.T) {
	view := simulationReport(simReportFixture(t))
	if view.Verdict != "SLA нарушается: 12% заданий дольше допуска." {
		t.Errorf("verdict %q", view.Verdict)
	}
	if len(view.KPI) != 4 || len(view.Processes) != 2 || len(view.Bottlenecks) != 1 || len(view.Limits) != len(simLimits) {
		t.Fatalf("slice shape: %d kpi, %d processes, %d bottlenecks, %d limits", len(view.KPI), len(view.Processes), len(view.Bottlenecks), len(view.Limits))
	}
	notes := strings.Join(view.Notes, "\n")
	mustContain(t, "notes", notes, "Флот выполнил 75% ожидаемых заданий", "Высота потолка не задана.", "Контрольное измерение масштаба")
	mustNotContain(t, "notes", notes, "SLA нарушается: 12%", "Узкое место: ")
}

func TestHorizonCostIsGrossOfResidualValue(t *testing.T) {
	r := calcReport(t)
	view := financialReport(r)
	checked := 0
	for i, sc := range r.Econ.Scenarios {
		if sc.Kind != "buy" {
			continue
		}
		gross := *sc.CapexRub + *sc.OpexYearRub*r.Econ.HorizonYears
		for _, row := range sc.CashFlow {
			gross += row.BatteryRub + row.ReplacementRub
		}
		if got := *view.Scenarios[i].HorizonCost; got != gross {
			t.Errorf("%s: horizon cost %v, want capex + annual costs + batteries + replacements = %v", scenarioName(sc), got, gross)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no purchase scenario in the fixture")
	}
	text, _ := pdfText(t, r)
	mustContain(t, "pdf", text, "Остаточная стоимость оборудования не учитывается")
	f := openWorkbook(t, r)
	for _, sheet := range f.GetSheetList() {
		mustNotContain(t, sheet, sheetText(t, f, sheet), "Остаточная стоимость роботов", "за вычетом остаточной")
	}
	for _, d := range view.Details {
		mustNotContain(t, "cost lines", d.Label+d.Section+d.Note, "Остаточная")
	}
}
