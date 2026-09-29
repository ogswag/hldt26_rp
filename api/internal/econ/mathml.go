package econ

import (
	"encoding/json"
	"html"
	"strings"

	"moscow_hackathon_2026/api/internal/rutext"
)

// NOTE: MathML for the web page, which the browser draws without a library. Formula.Text stays the plain form
// for PDF and XLSX; both describe the same formula and change together.

func mlMath(parts ...string) string {
	return `<math display="block">` + mlRow(parts...) + "</math>"
}

func mlRow(parts ...string) string { return "<mrow>" + strings.Join(parts, "") + "</mrow>" }

func mlText(s string) string { return "<mtext>" + html.EscapeString(s) + "</mtext>" }

// NOTE: the space around an operator is written out as mspace; browsers apply dictionary spacing unevenly (Chrome
// gives none to a minus between two mtext runs).
func mlOp(s string) string {
	space := `<mspace width="0.2222em"/>`
	if s == "=" {
		space = `<mspace width="0.2778em"/>`
	}
	return space + `<mo lspace="0" rspace="0">` + html.EscapeString(s) + "</mo>" + space
}

func mlNum(s string) string { return "<mn>" + s + "</mn>" }

func mlFrac(num, den string) string { return "<mfrac>" + num + den + "</mfrac>" }

func mlSub(base, sub string) string { return "<msub>" + base + sub + "</msub>" }

func mlParen(parts ...string) string {
	return mlRow(`<mo fence="true">(</mo>`, strings.Join(parts, ""), `<mo fence="true">)</mo>`)
}

func mlCeil(parts ...string) string {
	return mlRow(`<mo fence="true">⌈</mo>`, strings.Join(parts, ""), `<mo fence="true">⌉</mo>`)
}

// mlSum joins terms with an operator: mlSum("+", a, b, c) is a + b + c.
func mlSum(op string, terms ...string) string {
	out := make([]string, 0, 2*len(terms))
	for i, t := range terms {
		if i > 0 {
			out = append(out, mlOp(op))
		}
		out = append(out, t)
	}
	return mlRow(out...)
}

func mlTexts(words ...string) []string {
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = mlText(w)
	}
	return out
}

// FormulaMathML is the MathML of one formula of the model, empty for an id it does not know.
func FormulaMathML(id string, n Norms) string {
	eq := func(name string, rhs ...string) string { return mlMath(mlText(name), mlOp("="), mlRow(rhs...)) }
	switch id {
	case "fleet":
		return eq("Флот", mlCeil(
			mlCeil(mlFrac(mlText("Пик"), mlSum("×", mlText("Производительность"), mlNum(rutext.Fixed(n.Availability, 2)), mlNum(rutext.Fixed(n.Utilization, 2))))),
			mlOp("×"), mlNum(rutext.Fixed(1+n.Reserve, 2))))
	case "chargers":
		return eq("Зарядные станции",
			mlText("с ТТХ: "), mlCeil(mlSum("×", mlText("Количество"), mlFrac(mlText("Время зарядки"), mlSum("+", mlTexts("Время работы", "Время зарядки")...)))),
			mlText("; без ТТХ: "), mlCeil(mlFrac(mlText("Количество"), mlNum(rutext.Num(n.RobotsPerCharger, 0)))))
	case "capex_buy":
		return eq("CAPEX покупки", mlParen(mlSum("+", mlTexts("Оборудование", "Доставка", "Инфраструктура", "ПО", "Интеграция", "ПНР", "Обучение")...)), mlOp("×"), mlNum(rutext.Fixed(1+n.ContingencyFrac, 2)))
	case "capex_raas":
		return eq("CAPEX RaaS", mlParen(mlSum("+", mlTexts("Инфраструктура", "ПО", "Интеграция", "ПНР", "Обучение")...)), mlOp("×"), mlNum(rutext.Fixed(1+n.ContingencyFrac, 2)))
	case "delivery":
		return eq("Доставка", mlSum("×", mlNum(rutext.Pct(n.DeliveryFrac*100, 1)), mlText("Оборудование")), mlText(", только в CAPEX покупки"))
	case "opex_buy":
		return eq("OPEX покупки", mlSum("+", mlTexts("ФОТ остатка", "Сервис и ремонт", "Лицензии", "Энергия", "Расходники", "Связь", "Персонал эксплуатации")...))
	case "opex_raas":
		return eq("OPEX RaaS", mlSum("+", mlTexts("ФОТ остатка", "Плата по тарифу", "Энергия", "Расходники", "Связь", "Персонал эксплуатации")...))
	case "technicians":
		return eq("Персонал эксплуатации", mlCeil(mlFrac(mlText("Флот"), mlNum(rutext.Num(n.RobotsPerTechnician, 0)))), mlOp("×"), mlSum("×", mlTexts("Смены", "Оклад")...), mlOp("×"), mlNum("12"), mlOp("×"), mlParen(mlSum("+", mlNum("1"), mlText("Начисления"))))
	case "baseline_opex":
		return eq("OPEX базы", mlOp("∑"), mlText("ФОТ базовых процессов"), mlOp("×"), mlText("Коэффициент начислений"))
	case "payroll":
		return eq("ФОТ", mlSum("×", mlTexts("Штат", "Оклад")...), mlOp("×"), mlNum("12"), mlOp("×"), mlParen(mlSum("+", mlNum("1"), mlText("Начисления"))), mlText(", выше предельной базы 2 979 000 ₽ ставка 15,1%"))
	case "vat":
		return eq("Цена в расчёте", mlFrac(mlText("Цена с НДС"), mlParen(mlSum("+", mlNum("1"), mlText("Ставка НДС")))), mlText(", если НДС к вычету"))
	case "cash_effect":
		return eq("Денежный эффект", mlSum("−", mlTexts("OPEX базы", "OPEX сценария")...))
	case "battery":
		return eq("Замена батареи", mlSum("×", mlNum(rutext.Pct(n.BatteryFrac*100, 1)), mlText("Цена робота")), mlText(", каждые "+formatYears(n.BatteryYears)+" внутри срока службы"))
	case "replacement":
		return eq("Повторная покупка", mlText("Цена робота"), mlText(", когда срок службы кончается внутри горизонта"))
	case "cash_flow":
		return eq("Поток года t", mlSum("−", mlTexts("Эффект", "Замена батарей", "Повторная покупка")...))
	case "payback":
		return eq("Простая окупаемость", mlFrac(mlText("CAPEX"), mlText("Денежный эффект")), mlText(", пока нет разовых замен"))
	case "discounted_payback":
		return eq("Дисконтированная окупаемость", mlText("год, когда накопленный дисконтированный поток достигает нуля"))
	case "npv":
		return eq("NPV", mlOp("−"), mlText("CAPEX"), mlOp("+"), mlOp("∑"), mlFrac(mlSub(mlText("Поток"), mlText("t")), mlRow(mlParen(mlSum("+", mlNum("1"), mlText("r"))), mlText("t"))))
	case "irr":
		return eq("IRR", mlText("ставка r, при которой NPV равен нулю"))
	case "roi":
		return eq("ROI", mlFrac(mlSum("−", mlText("Сумма потоков за горизонт"), mlText("CAPEX")), mlText("CAPEX")), mlOp("×"), mlNum("100"))
	case "tco":
		return eq("Затраты за горизонт (TCO)", mlSum("+", mlText("CAPEX"), mlRow(mlSub(mlText("OPEX"), mlText("год")), mlOp("×"), mlText("Горизонт")), mlText("Замены")))
	case "depreciation":
		return eq("Амортизация", mlFrac(mlText("Стоимость актива"), mlText("Срок службы")), mlText(", обучение и резерв не амортизируются"))
	case "accounting_effect":
		return eq("Бухгалтерский эффект", mlSum("−", mlTexts("Денежный эффект", "Амортизация")...))
	case "labor_cash":
		return eq("Денежная экономия труда", mlSum("×", mlTexts("Высвобождённый ФОТ", "Доля денежной экономии")...))
	case "raas_fixed":
		return eq("RaaS, фиксированный", mlSum("×", mlText("Доля цены в месяц"), mlNum("12"), mlText("Цена"), mlText("Флот")))
	case "raas_variable":
		return eq("RaaS, переменный", mlSum("×", mlTexts("Ставка за операцию", "Годовой объём")...))
	case "raas_mixed":
		return eq("RaaS, смешанный", mlSum("+", mlRow(mlText("Доля фиксированной части"), mlOp("×"), mlText("Фиксированный")), mlRow(mlText("Остальная доля"), mlOp("×"), mlText("Переменный"))))
	default:
		return ""
	}
}

// WithMathML adds the MathML form to the formulas of a result saved before results carried it. Anything it
// cannot read goes back unchanged.
func WithMathML(raw json.RawMessage) json.RawMessage {
	var doc map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &doc) != nil {
		return raw
	}
	var fs []Formula
	if json.Unmarshal(doc["formulas"], &fs) != nil || len(fs) == 0 || fs[0].MathML != "" {
		return raw
	}
	for i := range fs {
		fs[i].MathML = FormulaMathML(fs[i].ID, DefaultNorms())
	}
	b, err := json.Marshal(fs)
	if err != nil {
		return raw
	}
	doc["formulas"] = b
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return out
}
