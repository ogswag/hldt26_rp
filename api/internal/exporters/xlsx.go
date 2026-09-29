package exporters

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/xuri/excelize/v2"
)

type xlsxDoc struct {
	f     *excelize.File
	head  int
	money int
	years int
	err   error
}

// XLSX renders the report as a workbook: six sheets for a calculation, four for a simulation. Money is numeric.
func XLSX(r Report) ([]byte, error) {
	if r.Econ == nil && r.Sim == nil {
		return nil, errors.New("exporters.xlsx: report has no result")
	}
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	x := &xlsxDoc{f: f}
	x.styles()
	if x.err != nil {
		return nil, x.err
	}
	props := &excelize.DocProperties{
		Title:   title(r),
		Creator: "platform",
		Created: r.generatedAt().Format("2006-01-02T15:04:05Z"),
	}
	if err := f.SetDocProps(props); err != nil {
		return nil, fmt.Errorf("exporters.xlsx.props: %w", err)
	}

	if err := f.SetSheetName("Sheet1", "Итог"); err != nil {
		return nil, fmt.Errorf("exporters.xlsx.sheet: %w", err)
	}
	if r.Econ != nil {
		x.financialSheets(r)
	} else {
		x.simulationSheets(r)
	}
	if x.err != nil {
		return nil, x.err
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("exporters.xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

func (x *xlsxDoc) styles() {
	x.head, x.err = x.f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"ECF0F6"}},
	})
	if x.err != nil {
		x.err = fmt.Errorf("exporters.xlsx.style: %w", x.err)
		return
	}
	money := `#,##0 "₽"`
	x.money, x.err = x.f.NewStyle(&excelize.Style{CustomNumFmt: &money})
	if x.err != nil {
		x.err = fmt.Errorf("exporters.xlsx.money_style: %w", x.err)
		return
	}
	years := `0.0`
	x.years, x.err = x.f.NewStyle(&excelize.Style{CustomNumFmt: &years})
	if x.err != nil {
		x.err = fmt.Errorf("exporters.xlsx.years_style: %w", x.err)
	}
}

// sheet writes a header row and data rows into a new or existing sheet. Columns in money get the ruble format and
// columns in years one decimal; both are 1-based.
func (x *xlsxDoc) sheet(name string, header []any, rows [][]any, money, years []int, widths ...float64) {
	if x.err != nil {
		return
	}
	idx, err := x.f.GetSheetIndex(name)
	if err != nil {
		x.err = fmt.Errorf("exporters.xlsx.%s: %w", name, err)
		return
	}
	if idx < 0 {
		if _, err := x.f.NewSheet(name); err != nil {
			x.err = fmt.Errorf("exporters.xlsx.%s: %w", name, err)
			return
		}
	}
	all := append([][]any{header}, rows...)
	for i, row := range all {
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			x.err = err
			return
		}
		if err := x.f.SetSheetRow(name, cell, &row); err != nil {
			x.err = fmt.Errorf("exporters.xlsx.%s: %w", name, err)
			return
		}
	}
	last, _ := excelize.CoordinatesToCellName(len(header), 1)
	if err := x.f.SetCellStyle(name, "A1", last, x.head); err != nil {
		x.err = fmt.Errorf("exporters.xlsx.%s: %w", name, err)
		return
	}
	if err := x.f.SetPanes(name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		x.err = fmt.Errorf("exporters.xlsx.%s: %w", name, err)
		return
	}
	x.style(name, len(rows), money, x.money)
	x.style(name, len(rows), years, x.years)
	for i, w := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		if err := x.f.SetColWidth(name, col, col, w); err != nil {
			x.err = fmt.Errorf("exporters.xlsx.%s: %w", name, err)
			return
		}
	}
}

func (x *xlsxDoc) style(name string, rows int, columns []int, style int) {
	if x.err != nil || rows == 0 {
		return
	}
	for _, col := range columns {
		from, _ := excelize.CoordinatesToCellName(col, 2)
		to, _ := excelize.CoordinatesToCellName(col, rows+1)
		if err := x.f.SetCellStyle(name, from, to, style); err != nil {
			x.err = fmt.Errorf("exporters.xlsx.%s: %w", name, err)
			return
		}
	}
}

func (x *xlsxDoc) financialSheets(r Report) {
	view := financialReport(r)
	rows := make([][]any, 0, len(view.Scenarios))
	for _, sc := range view.Scenarios {
		var payback any = dash
		switch {
		case sc.Baseline:
		case sc.Payback == nil:
			payback = noPayback
		default:
			payback = *sc.Payback
		}
		rows = append(rows, []any{sc.Variant, sc.Payment, sc.Fleet, nilFloat(sc.Capex), nilFloat(sc.Opex), nilFloat(sc.AnnualEffect), payback, nilFloat(sc.HorizonCost)})
	}
	x.sheet("Итог", []any{"Вариант", "Оплата", "Состав решения", "Первоначальные вложения", "Ежегодные затраты", "Годовой денежный эффект", "Простая окупаемость, лет", "Затраты за горизонт"},
		rows, []int{4, 5, 6, 8}, []int{7}, 24, 20, 44, 24, 20, 24, 24, 22)
	row := len(rows) + 3
	for _, kv := range append(view.Header.rows(), [2]string{"Вывод", view.Verdict}, [2]string{"Оговорка", surveyNote}, [2]string{"Примечание", financialNote}) {
		x.setText("Итог", row, 1, kv[0])
		x.setText("Итог", row, 2, kv[1])
		row++
	}

	details := make([][]any, 0, len(view.Details))
	for _, d := range view.Details {
		details = append(details, []any{d.Scenario, d.Section, d.Label, d.Rub, d.Note})
	}
	x.sheet("Варианты", []any{"Вариант", "Раздел", "Статья", "Сумма", "Примечание"}, details, []int{4}, nil, 34, 28, 40, 18, 54)

	notes := make([][]any, 0, len(view.Assumptions)+len(view.Warnings))
	for _, item := range view.Assumptions {
		notes = append(notes, []any{"Допущение", item})
	}
	for _, item := range view.Warnings {
		notes = append(notes, []any{"Важно", item})
	}
	for _, c := range view.SimChecks {
		notes = append(notes, []any{"Проверка симуляцией", c.Variant + ", " + lowerFirst(c.Method) + ". " + c.Result})
	}
	x.sheet("Допущения и риски", []any{"Тип", "Текст"}, notes, nil, nil, 22, 120)

	params := make([][]any, 0, len(view.Params))
	for _, pr := range view.Params {
		var value any = pr.Value
		if pr.Raw != nil {
			value = pr.Raw
		}
		params = append(params, []any{pr.Group, pr.Label, pr.Unit, value})
	}
	x.sheet("Параметры объекта", []any{"Группа", "Параметр", "Единица", "Значение"}, params, nil, nil, 30, 46, 14, 34)

	sources := make([][]any, 0, len(view.Sources))
	for _, item := range view.Sources {
		sources = append(sources, []any{item})
	}
	x.sheet("Источники", []any{"Источник"}, sources, nil, nil, 140)
	x.sensitivitySheet(r)
}

func (x *xlsxDoc) sensitivitySheet(r Report) {
	rows := [][]any{}
	if r.Econ != nil {
		for _, row := range r.Econ.Sensitivity {
			rows = append(rows, []any{
				row.VariantName, sensitivityParam(row.Param), row.DeltaPct,
				nilFloat(row.Buy.PaybackYears), nilFloat(row.Buy.AnnualEffectRub),
				nilFloat(row.Raas.PaybackYears), nilFloat(row.Raas.AnnualEffectRub),
			})
		}
	}
	x.sheet("Чувствительность", []any{"Вариант", "Параметр", "Сдвиг, %", "Покупка, окупаемость, лет", "Покупка, годовой эффект", "RaaS, окупаемость, лет", "RaaS, годовой эффект"},
		rows, []int{5, 7}, []int{4, 6}, 28, 24, 14, 28, 28, 24, 24)
}

func sensitivityParam(p string) string {
	switch p {
	case "equipment_price":
		return "Цена оборудования"
	case "volume":
		return "Объём операций"
	case "labor":
		return "ФОТ"
	default:
		return p
	}
}

func (x *xlsxDoc) simulationSheets(r Report) {
	view := simulationReport(r)
	rows := make([][]any, 0, len(view.KPI)+6)
	for _, kv := range view.Header.rows() {
		rows = append(rows, []any{kv[0], kv[1]})
	}
	if view.Variant != "" {
		rows = append(rows, []any{"Вариант", view.Variant})
	}
	rows = append(rows, []any{"Вывод", view.Verdict})
	for _, kv := range view.KPI {
		rows = append(rows, []any{kv[0], kv[1]})
	}
	x.sheet("Итог", []any{"Показатель", "Значение"}, rows, nil, nil, 28, 95)

	processes := make([][]any, 0, len(view.Processes))
	for _, p := range view.Processes {
		processes = append(processes, []any{p[0], p[1], p[2], p[3], p[4], p[5]})
	}
	x.sheet("Процессы", []any{"Процесс", "Статус", "Выполнено", "Нарушения SLA", "Ожидание p95", "Цикл p95"}, processes, nil, nil, 32, 20, 16, 20, 20, 20)

	bottlenecks := make([][]any, 0, len(view.Bottlenecks))
	for _, b := range view.Bottlenecks {
		bottlenecks = append(bottlenecks, []any{b[0], b[1]})
	}
	x.sheet("Узкие места", []any{"Участок", "Что происходит"}, bottlenecks, nil, nil, 32, 100)

	notes := make([][]any, 0, len(view.Notes)+len(view.Limits))
	for _, item := range view.Notes {
		notes = append(notes, []any{"Важно", item})
	}
	for _, item := range view.Limits {
		notes = append(notes, []any{"Ограничение модели", item})
	}
	x.sheet("Допущения и риски", []any{"Тип", "Текст"}, notes, nil, nil, 22, 120)
}

func (x *xlsxDoc) setText(sheet string, row, column int, value string) {
	if x.err != nil {
		return
	}
	cell, err := excelize.CoordinatesToCellName(column, row)
	if err != nil {
		x.err = err
		return
	}
	if err := x.f.SetCellValue(sheet, cell, value); err != nil {
		x.err = fmt.Errorf("exporters.xlsx.%s: %w", sheet, err)
	}
}

func nilFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
