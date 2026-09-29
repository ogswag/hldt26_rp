package exporters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/sim"
)

const (
	testRunID  = "0f5c2a1e-7b1d-4c9a-9a51-3c1f7a9d2b10"
	testHash   = "9d4e1c7a5b3f2e1d0c9b8a7f6e5d4c3b2a1f0e9d8c7b6a5f4e3d2c1b0a9f8e7d"
	robotID    = "5760e938-9a43-45a7-b8e8-f4f2e6383930"
	stackerID  = "2ffc706d-fe43-4c2b-baad-a624a95ad3ce"
	missingSID = "11111111-2222-4333-8444-555555555555"
)

func fptr(v float64) *float64 { return &v }

func testParams(t *testing.T) json.RawMessage {
	t.Helper()
	def, err := objects.Defaults(objects.Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	def["power_kw"] = nil
	def["has_wms"] = nil
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func testDraft(t *testing.T, raw json.RawMessage) projects.Draft {
	t.Helper()
	robot, stacker, missing := robotID, stackerID, missingSID
	tariff := "variable"
	procs := projects.DefaultProcesses(objects.Warehouse)
	procs[3].Demand.UnitsPerDay = 0
	d := projects.NewDraft(objects.Warehouse, raw, procs, []projects.Variant{
		{ID: "v1", Name: "Смешанный флот", Status: "draft", Fleet: []projects.FleetItem{
			{SolutionID: &robot, Quantity: 4, TaskCodes: []string{"piece_pick"}, PriceOverrideRub: fptr(2_500_000), PriceOverrideReason: "КП поставщика от 01.09"},
			{SolutionID: &stacker, Quantity: 2, TaskCodes: []string{"pallet_putaway"}},
			{SolutionID: &missing, Quantity: 1},
		}, Financing: []projects.Financing{{Kind: "buy"}, {Kind: "raas", Tariff: &tariff, Assumptions: json.RawMessage(`{"per_op_rub":3}`)}}},
		{ID: "v2", Name: "Пустой", Status: "draft", Fleet: []projects.FleetItem{}},
	}, []string{robotID})
	return d
}

var sourcedAt = time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)

func calcReport(t *testing.T) Report {
	t.Helper()
	raw := testParams(t)
	d := testDraft(t, raw)
	robots := []econ.Robot{
		{ID: robotID, Name: "AMR H1500", PriceRub: fptr(3_000_000), SpeedMps: fptr(1.5), WidthMm: fptr(900), PayloadKg: fptr(1500)},
		{ID: stackerID, Name: "Штабелёр S20", PriceRub: fptr(4_500_000), PayloadKg: fptr(2000)},
	}
	match := matching.Output{
		MatchVersion:         matching.Version,
		CatalogContentSHA256: strings.Repeat("ab", 32),
		SelectedIDs:          []string{robotID},
		Items: []matching.Item{
			{SolutionID: robotID, Name: "AMR H1500", Status: matching.StatusRecommended, Reasons: []string{"Подходит."}},
			{SolutionID: stackerID, Name: "Штабелёр S20", Status: matching.StatusNeedsReview, Reasons: []string{"Нет минимальной ширины проезда."},
				MissingEvidence: []matching.Step{{RuleID: "aisle", Kind: matching.KindMissing, Outcome: matching.OutcomeUnknown, Text: "Нет минимальной ширины проезда."}}},
		},
	}
	res, err := econ.Calculate(econ.Input{
		ObjectType: objects.Warehouse,
		Params:     raw,
		Robots:     robots,
		PickReason: "selected",
		Match:      match,
		Processes: []econ.ProcessSpec{
			{Code: "piece_pick", TaskType: "piece_pick", IsBaseline: true, UnitsPerDay: 100000, StaffHeadcount: 100, StaffRole: "отборщик"},
			{Code: "putaway", TaskType: "pallet_putaway", IsBaseline: true, UnitsPerDay: 1000, StaffHeadcount: 6, StaffRole: "погрузчик"},
		},
		Variants: []econ.VariantSpec{{ID: "v1", Name: "Смешанный флот", Fleet: []econ.FleetSpec{
			{SolutionID: robotID, Quantity: 4, TaskCodes: []string{"piece_pick"}, PriceOverrideRub: fptr(2_500_000), PriceOverrideReason: "КП поставщика от 01.09"},
			{SolutionID: stackerID, Quantity: 2, TaskCodes: []string{"pallet_putaway"}},
		}, Financing: []econ.FinancingSpec{{Kind: "buy"}, {Kind: "raas", Tariff: "variable"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	res.Match = match
	return Report{
		GeneratedAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
		ProjectName: "Демо склад",
		ObjectType:  objects.Warehouse,
		Inputs:      d,
		Econ:        &res,
		Run: &RunInfo{
			ID: testRunID, Kind: KindCalculation, VersionNo: 3, InputHash: testHash,
			EconVersion: econ.ModelVersion, SimVersion: projects.SimVersion,
			ConfidenceLevel: projects.ConfidencePreliminary, StaleVsDraft: true,
			CreatedAt: time.Date(2026, 9, 25, 9, 30, 0, 0, time.UTC),
		},
		Catalog: CatalogInfo{Items: []CatalogItem{
			{SolutionID: robotID, Name: "AMR H1500", PriceRub: fptr(3_000_000), SourceURL: "https://ronavi.example/h1500", Confidence: "vendor", SourcedAt: &sourcedAt},
			{SolutionID: stackerID, Name: "Штабелёр S20", PriceRub: fptr(4_500_000)},
		}},
	}
}

func simReportFixture(t *testing.T) Report {
	t.Helper()
	r := calcReport(t)
	r.Econ = nil
	r.Run.Kind = KindSimulation
	r.Run.SimVersion = sim.Version
	r.Run.ConfidenceLevel = projects.ConfidenceConfigured
	r.Run.StaleVsDraft = false
	stat := func(v float64) sim.Stat {
		return sim.Stat{Median: v, Min: v * 0.9, Max: v * 1.1, P10: v * 0.92, P90: v * 1.08}
	}
	r.Sim = &SimReport{
		VariantName: "Смешанный флот",
		MapWarnings: []string{"Контрольное измерение масштаба не выполнено."},
		EconCheck:   &EconCheck{Flag: true, Text: "Флот выполнил 75% ожидаемых заданий за горизонт."},
		Result: sim.Result{
			SimVersion: sim.Version, Config: sim.Config{Mode: sim.ModeStochastic, Replications: 4, Seed: 3, Policy: sim.PolicyFIFO, HorizonH: 2},
			Replications: 4, Seeds: []int64{3, 4, 5, 6}, HorizonS: 7200, Windows: []sim.Window{{StartH: 0, EndH: 2}},
			ActiveHoursPerDay: 14, SLATargetPct: 5, Verdict: sim.VerdictFail, VerdictText: "SLA нарушается: 12% заданий дольше допуска.",
			KPI: sim.KPI{Arrived: stat(400), Completed: stat(300), ViolationRate: stat(0.12), ThroughputPerH: stat(150)},
			Processes: []sim.ProcessResult{
				{Code: "inbound", Name: "Приёмка", Covered: true, ExpectedJobs: 100, Arrived: stat(100), Completed: stat(80), ViolationRate: stat(0.2), MaxWaitS: 1800},
				{Code: "outbound", Name: "Отгрузка", Covered: false},
			},
			Resources:          []sim.ResourceResult{{ID: "dock-1", Kind: "dock", Name: "Док", Capacity: 1, Utilization: stat(0.93)}},
			Fleet:              []sim.FleetResult{{SolutionID: robotID, Name: "AMR H1500", Profile: "amr", ProfileLabel: "AMR", Quantity: 4, Assumed: []string{"endurance_h", "charge_min", "speed_cap"}}},
			Bottlenecks:        []sim.Bottleneck{{Kind: "dock", ID: "dock-1", Name: "Док", Primary: true, Saturated: true, Text: "Док загружен на 93%."}},
			RepresentativeSeed: 5, Representative: 2,
			Warnings:    []string{"Высота потолка не задана."},
			Assumptions: []string{"Отказы не моделируются."},
		},
	}
	return r
}

func sheetText(t *testing.T, f *excelize.File, sheet string) string {
	t.Helper()
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("sheet %s: %v", sheet, err)
	}
	var b strings.Builder
	for _, row := range rows {
		b.WriteString(strings.Join(row, "\t"))
		b.WriteByte('\n')
	}
	return b.String()
}

func mustContain(t *testing.T, where, text string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(text, p) {
			t.Fatalf("%s does not contain %q:\n%s", where, p, text)
		}
	}
}

func pdfText(t *testing.T, r Report) (string, int) {
	t.Helper()
	b, said, err := renderPDF(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("not a pdf: %q", b[:8])
	}
	return strings.Join(said, "\n"), bytes.Count(b, []byte("/Type /Page\n"))
}

func mustNotContain(t *testing.T, where, text string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if strings.Contains(text, p) {
			t.Fatalf("%s exposes %q:\n%s", where, p, text)
		}
	}
}

// internalMarks are strings a person deciding on a project must never see in a report.
func internalMarks() []string {
	return []string{
		testRunID, testHash, robotID, stackerID, missingSID, "seed", "econ-v", "sim-v", "match-v", "solution_id", "LINESTRING", "POINT(",
		"{", "NPV", "IRR", "ROI", "Дисконтир", "sha256", "SHA-256",
	}
}

func openWorkbook(t *testing.T, r Report) *excelize.File {
	t.Helper()
	b, err := XLSX(r)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestFinancialPDF(t *testing.T) {
	r := calcReport(t)
	text, pages := pdfText(t, r)
	if pages < 1 || pages > 3 {
		t.Fatalf("a typical project fits three pages with the appendix, got %d", pages)
	}
	mustContain(t, "pdf", text,
		"Финансовая оценка роботизации", "Проект", "Демо склад", "Объект", "Склад", "Дата расчёта", "25.09.2026", "Дата отчёта", "27.09.2026", "Достоверность", "Предварительный",
		"Итог", "База и варианты", "Первоначальные вложения", "Ежегодные затраты", "Годовой денежный эффект", "Простая окупаемость", "Затраты за горизонт",
		"База", "Текущий процесс", "Смешанный флот", "Покупка", "RaaS, переменный",
		"Вариант с лучшей окупаемостью", "Состав решения", "AMR H1500, 4 шт.", "Состав затрат", "Допущения и риски",
		"не является бухгалтерской отчётностью", "Горизонт оценки", "НДС", "Денежная доля экономии труда",
		surveyNote,
		"Источники", "AMR H1500: https://ronavi.example/h1500. Источник ТТХ: данные производителя, 20.09.2026.", "Штабелёр S20: ссылка на источник не указана.",
		"Параметры объекта", "Мощность электроснабжения (доступная), кВт", "Неизвестно", "Горизонт расчёта окупаемости, лет",
	)
	mustNotContain(t, "pdf", text, internalMarks()...)
	mustNotContain(t, "pdf", text, "Денежный поток", "Остаточная стоимость по годам", "Формулы")
}

func TestPDFWithoutVariantsSaysSo(t *testing.T) {
	r := calcReport(t)
	r.Econ.Scenarios = r.Econ.Scenarios[:1]
	r.Econ.Variants = nil
	text, _ := pdfText(t, r)
	mustContain(t, "pdf", text, noVariants)
	mustNotContain(t, "pdf", text, "Вариант с лучшей окупаемостью")
}

func TestSimulationPDF(t *testing.T) {
	r := simReportFixture(t)
	if Filename(r, "pdf") != "simulyaciya-warehouse-3.pdf" {
		t.Fatalf("filename %s", Filename(r, "pdf"))
	}
	text, pages := pdfText(t, r)
	if pages > 2 {
		t.Fatalf("simulation report is short, got %d pages", pages)
	}
	mustContain(t, "pdf", text,
		"Отчёт по симуляции", "Смешанный фло", "Итог", "SLA нарушается", "Допуск нарушений SLA", "Производительность", "Процессы", "Приёмка", "Не моделируется",
		"Узкие места", "Док загружен на 93%", "Допущения и риски", "Путь робота выбирается по кратчайшему расстоянию",
	)
	mustNotContain(t, "pdf", text, internalMarks()...)
	mustNotContain(t, "pdf", text, "Репликаци", "Профиль", "Карта", "Распределени")
}

func TestFinancialWorkbook(t *testing.T) {
	r := calcReport(t)
	f := openWorkbook(t, r)
	want := "Итог,Варианты,Допущения и риски,Параметры объекта,Источники,Чувствительность"
	if got := strings.Join(f.GetSheetList(), ","); got != want {
		t.Fatalf("sheets %s, want %s", got, want)
	}
	props, err := f.GetDocProps()
	if err != nil || props.Identifier != "" || strings.Contains(props.Keywords, testHash) {
		t.Fatalf("doc props %+v %v", props, err)
	}
	summary := sheetText(t, f, "Итог")
	mustContain(t, "summary", summary,
		"Вариант\tОплата\tСостав решения\tПервоначальные вложения\tЕжегодные затраты\tГодовой денежный эффект\tПростая окупаемость, лет\tЗатраты за горизонт",
		"База\tТекущий процесс", "Смешанный флот\tПокупка", "Проект\tДемо склад", "Дата расчёта\t25.09.2026", "Дата отчёта\t27.09.2026", "Вывод\t", "Оговорка\t"+surveyNote, "Примечание\t",
	)
	view := financialReport(r)
	params := sheetText(t, f, "Параметры объекта")
	mustContain(t, "params", params,
		"Группа\tПараметр\tЕдиница\tЗначение",
		"Мощность электроснабжения (доступная)\tкВт\tНеизвестно",
		"Горизонт расчёта окупаемости\tлет\t5",
	)
	sources := sheetText(t, f, "Источники")
	mustContain(t, "sources", sources, "AMR H1500: https://ronavi.example/h1500. Источник ТТХ: данные производителя, 20.09.2026.")
	all := summary + sheetText(t, f, "Варианты") + sheetText(t, f, "Допущения и риски") + params + sources
	mustNotContain(t, "workbook", all, internalMarks()...)
	mustNotContain(t, "sources", sources, "Ставка дисконтирования")

	for i, sc := range r.Econ.Scenarios {
		row := i + 2
		for _, c := range []struct {
			col string
			v   *float64
		}{{"D", sc.CapexRub}, {"E", sc.OpexYearRub}, {"F", sc.AnnualEffectRub}, {"H", view.Scenarios[i].HorizonCost}} {
			cell := fmt.Sprintf("%s%d", c.col, row)
			if c.v == nil {
				continue
			}
			typ, err := f.GetCellType("Итог", cell)
			if err != nil {
				t.Fatal(err)
			}
			if typ == excelize.CellTypeSharedString || typ == excelize.CellTypeInlineString {
				t.Errorf("%s of %s is text, want a number", cell, scenarioName(sc))
			}
			raw, err := f.GetCellValue("Итог", cell, excelize.Options{RawCellValue: true})
			if got, perr := strconv.ParseFloat(raw, 64); err != nil || perr != nil || got != *c.v {
				t.Errorf("%s holds %q, want %v", cell, raw, *c.v)
			}
			shown, err := f.GetCellValue("Итог", cell)
			if err != nil || !strings.Contains(shown, "₽") {
				t.Errorf("%s shows %q, want the ruble format", cell, shown)
			}
		}
	}
}

func TestWorkbookPaybackCells(t *testing.T) {
	r := calcReport(t)
	f := openWorkbook(t, r)
	for i, sc := range r.Econ.Scenarios {
		cell := fmt.Sprintf("G%d", i+2)
		got, err := f.GetCellValue("Итог", cell, excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case sc.Kind == "baseline":
			if got != dash {
				t.Errorf("baseline payback %q, want %q", got, dash)
			}
		case sc.PaybackYears == nil:
			if got != noPayback {
				t.Errorf("%s payback %q, want %q", scenarioName(sc), got, noPayback)
			}
		default:
			if v, err := strconv.ParseFloat(got, 64); err != nil || v != *sc.PaybackYears {
				t.Errorf("%s payback %q, want %v", scenarioName(sc), got, *sc.PaybackYears)
			}
		}
	}
}

func TestSimulationWorkbook(t *testing.T) {
	r := simReportFixture(t)
	f := openWorkbook(t, r)
	want := "Итог,Процессы,Узкие места,Допущения и риски"
	if got := strings.Join(f.GetSheetList(), ","); got != want {
		t.Fatalf("sheets %s, want %s", got, want)
	}
	mustContain(t, "summary", sheetText(t, f, "Итог"), "Вывод\tSLA нарушается", "Нарушения SLA", "Производительность")
	mustContain(t, "processes", sheetText(t, f, "Процессы"), "Приёмка\tМоделируется", "Отгрузка\tНе моделируется")
	mustContain(t, "bottlenecks", sheetText(t, f, "Узкие места"), "Док\tДок загружен")
	mustContain(t, "notes", sheetText(t, f, "Допущения и риски"), "Ограничение модели\tПуть робота")
	var all string
	for _, sheet := range f.GetSheetList() {
		all += sheetText(t, f, sheet)
	}
	mustNotContain(t, "workbook", all, internalMarks()...)
}

func TestGuestReport(t *testing.T) {
	raw := testParams(t)
	res, err := econ.Calculate(econ.Input{ObjectType: objects.Warehouse, Params: raw})
	if err != nil {
		t.Fatal(err)
	}
	r := Report{GeneratedAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC), ObjectType: objects.Warehouse, Inputs: projects.Draft{Params: raw}, Econ: &res}
	if r.ConfidenceLevel() != projects.ConfidencePreliminary || r.Kind() != KindGuest {
		t.Fatalf("guest kind %s level %s", r.Kind(), r.ConfidenceLevel())
	}
	if Filename(r, "xlsx") != "ocenka-warehouse.xlsx" {
		t.Fatalf("guest filename %s", Filename(r, "xlsx"))
	}
	text, _ := pdfText(t, r)
	mustContain(t, "pdf", text, "Не сохранён", "База и варианты", "Дата расчёта", "Дата отчёта", surveyNote)
	f := openWorkbook(t, r)
	if got := strings.Join(f.GetSheetList(), ","); got != "Итог,Варианты,Допущения и риски,Параметры объекта,Источники,Чувствительность" {
		t.Fatalf("guest sheets %s", got)
	}
	mustContain(t, "summary", sheetText(t, f, "Итог"), "База\tТекущий процесс", "Проект\tНе сохранён", "Дата расчёта\t27.09.2026", "Дата отчёта\t27.09.2026")
}

func TestReportWithoutResultIsAnError(t *testing.T) {
	r := Report{ObjectType: objects.Warehouse}
	if _, err := PDF(r); err == nil {
		t.Fatal("pdf of an empty report must fail")
	}
	if _, err := XLSX(r); err == nil {
		t.Fatal("xlsx of an empty report must fail")
	}
}

func TestNoticesOrderAndLevels(t *testing.T) {
	r := calcReport(t)
	items := Notices(r)
	if len(items) == 0 {
		t.Fatal("the fixture must raise warnings")
	}
	last := -1
	for _, n := range items {
		rank := levelRank(n.Level)
		if rank < last {
			t.Fatalf("notices not sorted by level: %+v", items)
		}
		last = rank
	}
	r.Run.StaleVsDraft = false
	r.Run.EconVersion = "econ-v0"
	text := noticeText(Notices(r))
	mustContain(t, "notices", text, "Запуск посчитан по прежней версии модели расчёта")
	mustNotContain(t, "notices", text, "econ-v")
}

func TestNoticesNameSkippedFleetWithoutIDs(t *testing.T) {
	text := noticeText(Notices(calcReport(t)))
	mustContain(t, "notices", text, "Вариант Смешанный флот: одного из роботов нет в каталоге")
	mustNotContain(t, "notices", text, missingSID)
}

func noticeText(items []Notice) string {
	var b strings.Builder
	for _, n := range items {
		b.WriteString(n.Level + " " + n.Text + "\n")
	}
	return b.String()
}

func TestConfidenceLabels(t *testing.T) {
	seen := map[string]bool{}
	for _, lv := range []string{projects.ConfidencePreliminary, projects.ConfidenceConfigured, projects.ConfidenceCalibrated, projects.ConfidenceValidated} {
		name, meaning := ConfidenceLabel(lv)
		if name == "" || meaning == "" || seen[name] {
			t.Fatalf("level %s: %q %q", lv, name, meaning)
		}
		seen[name] = true
	}
}

func TestReportShowsSimulationChecksOfEveryVariant(t *testing.T) {
	r := calcReport(t)
	r.Econ.SimChecks = []econ.SimCheck{
		{VariantID: "a", VariantName: "Без запуска", Model: econ.SimCheckMap},
		{VariantID: "b", VariantName: "С расхождением", Model: econ.SimCheckMap, RunID: "run-b", Flag: true, Value: 0.6, Text: "Флот выполнил 60% ожидаемых заданий за горизонт."},
		{VariantID: "c", VariantName: "Устаревший", Model: econ.SimCheckMap, RunID: "run-c", Stale: true, Flag: true, Text: "Флот выполнил 60% ожидаемых заданий за горизонт."},
		{VariantID: "d", VariantName: "Без карты", Model: econ.SimCheckQuick, Value: 1, Text: "AMR H1500: 12 поддон/ч в симуляции против 12 в экономике, расхождение 0%."},
	}
	text, pages := pdfText(t, r)
	if pages > 3 {
		t.Fatalf("checks must not push the report past three pages, got %d", pages)
	}
	mustContain(t, "pdf", text,
		"Проверка симуляцией", "Симуляция на карте", "Быстрая проверка без карты",
		"Не запускалась. Запустите симуляцию варианта на вкладке «Симуляция».",
		"Расхождение. Флот выполнил 60%", "Устарела: вариант или объект изменились после запуска.", "Сходится. AMR H1500",
	)
	notes := noticeText(Notices(r))
	mustContain(t, "notices", notes,
		"high Вариант «С расхождением»: Флот выполнил 60%",
		"warning Проверка симуляцией по варианту «Устаревший» устарела",
	)
	mustNotContain(t, "notices", notes, "Вариант «Устаревший»: Флот выполнил", "Без запуска")
	mustNotContain(t, "pdf", text, "run-b", "run-c")
	f := openWorkbook(t, r)
	mustContain(t, "notes", sheetText(t, f, "Допущения и риски"), "Проверка симуляцией\tС расхождением, симуляция на карте. Расхождение.")
}
