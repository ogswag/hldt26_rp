package exporters

const (
	maxDetailRows = 12
	dash          = "-"
)

func (p *pdfDoc) header(title string, h userHeader, extra ...[2]string) {
	p.bold(16)
	p.text(8, title)
	p.Ln(1)
	p.kv(append(h.rows(), extra...))
}

func (p *pdfDoc) userFinancial(r Report) {
	view := financialReport(r)
	p.header(title(r), view.Header)

	p.h2("Итог")
	p.font(10)
	p.para(view.Verdict)
	p.para(surveyNote)
	p.h3("База и варианты")
	rows := make([][]string, 0, len(view.Scenarios))
	for _, sc := range view.Scenarios {
		rows = append(rows, []string{sc.Variant, sc.Payment, formatRub(sc.Capex), formatRub(sc.Opex), formatRub(sc.AnnualEffect), paybackCell(sc), formatRub(sc.HorizonCost)})
	}
	p.table([]float64{18, 14, 16, 15, 16, 13, 16},
		[]string{"Вариант", "Оплата", "Первоначальные вложения", "Ежегодные затраты", "Годовой денежный эффект", "Простая окупаемость", "Затраты за горизонт"}, rows, 8)
	p.font(8)
	p.para(financialNote)

	if best := view.Best; best != nil {
		p.h2("Вариант с лучшей окупаемостью")
		p.kv([][2]string{
			{"Вариант", best.Variant},
			{"Оплата", best.Payment},
			{"Состав решения", emptyAsNoData(best.Fleet)},
			{"Первоначальные вложения", formatRub(best.Capex)},
			{"Ежегодные затраты", formatRub(best.Opex)},
			{"Годовой денежный эффект", formatRub(best.AnnualEffect)},
			{"Простая окупаемость", paybackCell(*best)},
			{"Затраты за горизонт", formatRub(best.HorizonCost)},
		})
		p.userDetails(detailScenario(*best), view.Details)
	}

	if len(view.SimChecks) > 0 {
		p.h2("Проверка симуляцией")
		rows := make([][]string, 0, len(view.SimChecks))
		for _, c := range view.SimChecks {
			rows = append(rows, []string{c.Variant, c.Method, c.Result})
		}
		p.table([]float64{20, 22, 58}, []string{"Вариант", "Способ", "Результат"}, rows, 8)
	}

	if len(view.Assumptions) > 0 || len(view.Warnings) > 0 {
		p.h2("Допущения и риски")
		if len(view.Assumptions) > 0 && len(view.Warnings) > 0 {
			p.h3("Допущения")
		}
		p.list(view.Assumptions)
		if len(view.Assumptions) > 0 && len(view.Warnings) > 0 {
			p.h3("Что может изменить вывод")
		}
		p.list(view.Warnings)
	}

	if len(view.Sources) > 0 {
		p.h2("Источники")
		p.list(view.Sources)
	}
	p.userParams(view.Params)
}

// userParams prints the object parameters as an appendix, two parameters to a row so the list stays short.
func (p *pdfDoc) userParams(params []paramRow) {
	if len(params) == 0 {
		return
	}
	p.h2("Параметры объекта")
	for i := 0; i < len(params); {
		group := params[i].Group
		j := i
		for j < len(params) && params[j].Group == group {
			j++
		}
		p.h3(group)
		rows := make([][]string, 0, (j-i+1)/2)
		for k := i; k < j; k += 2 {
			row := []string{params[k].name(), params[k].Value}
			if k+1 < j {
				row = append(row, params[k+1].name(), params[k+1].Value)
			}
			rows = append(rows, row)
		}
		p.table([]float64{31, 19, 31, 19}, nil, rows, 7.5)
		i = j
	}
}

func paybackCell(sc userScenario) string {
	switch {
	case sc.Baseline:
		return dash
	case sc.Payback == nil:
		return noPayback
	}
	return yearsText(*sc.Payback)
}

func (p *pdfDoc) userDetails(scenario string, details []userDetail) {
	rows := make([][]string, 0, maxDetailRows)
	for _, d := range details {
		if d.Scenario != scenario {
			continue
		}
		rows = append(rows, []string{d.Section, d.Label, rub(d.Rub), d.Note})
		if len(rows) == maxDetailRows {
			break
		}
	}
	if len(rows) == 0 {
		return
	}
	p.h3("Состав затрат")
	p.table([]float64{25, 38, 17, 20}, []string{"Раздел", "Статья", "Сумма", "Примечание"}, rows, 7.5)
}

func (p *pdfDoc) userSimulation(r Report) {
	view := simulationReport(r)
	var extra [][2]string
	if view.Variant != "" {
		extra = append(extra, [2]string{"Вариант", view.Variant})
	}
	p.header(title(r), view.Header, extra...)

	p.h2("Итог")
	p.font(10)
	p.para(view.Verdict)
	kpi := make([][]string, 0, len(view.KPI))
	for _, row := range view.KPI {
		kpi = append(kpi, []string{row[0], row[1]})
	}
	p.table([]float64{48, 52}, []string{"Показатель", "Значение"}, kpi, 8.5)

	if len(view.Processes) > 0 {
		p.h2("Процессы")
		p.table([]float64{26, 16, 14, 16, 14, 14}, []string{"Процесс", "Статус", "Выполнено", "Нарушения SLA", "Ожидание p95", "Цикл p95"}, view.Processes, 7.5)
	}

	if len(view.Bottlenecks) > 0 {
		p.h2("Узкие места")
		rows := make([][]string, 0, len(view.Bottlenecks))
		for _, b := range view.Bottlenecks {
			rows = append(rows, []string{b[0], b[1]})
		}
		p.table([]float64{30, 70}, []string{"Участок", "Что происходит"}, rows, 8)
	}

	p.h2("Допущения и риски")
	p.list(view.Notes)
	p.list(view.Limits)
}

func emptyAsNoData(value string) string {
	if value == "" {
		return noData
	}
	return value
}
