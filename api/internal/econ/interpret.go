package econ

import (
	"fmt"
	"math"
	"strings"

	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/rutext"
)

const (
	BandNone      = "none"
	BandUpTo3     = "up_to_3"
	BandFrom3To5  = "from_3_to_5"
	BandOver5     = "over_5"
	BandLimitLow  = 3.0
	BandLimitHigh = 5.0

	Disclaimer = "Это предварительная оценка, не решение go/no-go. Главная метрика: окупаемость простая, без дисконта. NPV и IRR даны справочно. Перед внедрением нужно обследование площадки."
)

func paybackBand(years *float64) string {
	if years == nil {
		return BandNone
	}
	v := *years
	if v <= BandLimitLow {
		return BandUpTo3
	}
	if v <= BandLimitHigh {
		return BandFrom3To5
	}
	return BandOver5
}

func bandLabelRU(band string) string {
	switch band {
	case BandUpTo3:
		return "до 3 лет"
	case BandFrom3To5:
		return "от 3 до 5 лет"
	case BandOver5:
		return "более 5 лет"
	default:
		return "нет срока окупаемости"
	}
}

func attachView(res *Result, in Input, horizon float64, lifeAssumed, svcAssumed, noPrice bool) {
	res.HorizonYears = horizon
	if in.Robot != nil {
		res.SolutionName = in.Robot.Name
	}
	res.Risks = collectRisks(in, *res, lifeAssumed, svcAssumed, noPrice)
	res.Interpretation = interpret(in, *res, horizon, noPrice)
}

func ApplyVerification(res *Result) {
	if res == nil {
		return
	}
	if res.Sim != nil {
		note := fmt.Sprintf(
			"Симуляция пикового окна %s с: производительность %s против экономики %s, расхождение %s.",
			rutext.Num(res.Sim.SimulatedS, 0),
			rutext.Num(res.Sim.Throughput, 2),
			rutext.Num(res.Sim.EconThroughput, 2),
			rutext.Pct(res.Sim.Divergence*100, 1),
		)
		res.Assumptions = append(res.Assumptions, note)
	}
	if !res.VerificationFlag {
		return
	}
	for _, r := range res.Risks {
		if r.ID == "verification" {
			return
		}
	}
	res.Risks = append(res.Risks, Risk{
		ID:    "verification",
		Level: "high",
		Text:  "Симуляция и экономика разошлись больше чем на 15%. Не скрывайте этот флаг.",
	})
}

func collectRisks(in Input, res Result, lifeAssumed, svcAssumed, noPrice bool) []Risk {
	out := []Risk{{
		ID:    "simple_payback",
		Level: "info",
		Text:  "Окупаемость простая: момент, когда накопленный денежный поток достигает нуля, без дисконта. NPV, IRR и дисконтированная окупаемость считаются по ставке набора допущений в постоянных ценах. Налог на прибыль и инфляция не моделируются.",
	}}
	if in.Robot == nil && !hasVariantFleet(in) {
		out = append(out, Risk{
			ID:    "no_robot",
			Level: "warning",
			Text:  "Ни один робот из каталога не подходит к объекту, покупка и RaaS не посчитаны. Причины на вкладке Роботы.",
		})
		return out
	}
	if in.Robot == nil && hasVariantFleet(in) {
		return append(out, variantRisks(in, res)...)
	}
	if noPrice {
		out = append(out, Risk{
			ID:    "no_price",
			Level: "warning",
			Text:  "Нет цены изделия. Сценарии покупки и RaaS пустые, пока цена не задана в каталоге или в поле цены.",
		})
		return out
	}
	nm := in.norms()
	out = append(out,
		Risk{
			ID:    "vat_price",
			Level: "info",
			Text:  "Цена из каталога с НДС. Доставка " + rutext.Pct(nm.DeliveryFrac*100, 1) + " оборудования и остальные статьи CAPEX сверх изделия (инфраструктура, ПО, интеграция, пусконаладка, обучение) добавлены отдельными статьями.",
		},
		Risk{
			ID:    "remaining_staff",
			Level: "warning",
			Text:  "В новом OPEX остаётся не меньше 40% целевого персонала (и не меньше 2 чел). Прочий персонал в расчёт не входит.",
		},
		Risk{
			ID:    "raas_fee",
			Level: "warning",
			Text:  "Ставка RaaS " + rutext.Pct(nm.RaasMonthlyFrac*100, 1) + " цены изделия в месяц задана командой, не договором. Выкуп не моделируется. Доставка, сервис и ремонт, лицензии и батарея считаются входящими в ставку.",
		},
	)
	item := findMatchItem(in.Match, in.Robot.ID)
	if in.PickReason == "needs_review" || (item != nil && item.Status != matching.StatusRecommended) {
		out = append(out, Risk{
			ID:    "not_recommended",
			Level: "warning",
			Text:  "В расчёт взят робот, который не прошёл все проверки. Что уточнить, сказано на вкладке Роботы.",
		})
	}
	if lifeAssumed || svcAssumed || (item != nil && item.Status == matching.StatusNeedsReview) {
		out = append(out, Risk{
			ID:    "missing_ttx",
			Level: "warning",
			Text:  "По роботу не хватает ТТХ или статуса «подходит». Срок службы и доля сервиса при пробелах приняты допущением модели.",
		})
	}
	out = append(out, horizonRisk(res.HorizonYears, singleLife(in.Robot, in.norms()))...)
	out = append(out, vatRisk(in.Assumptions)...)
	buy := scenarioByKind(res, "buy")
	if buy.AnnualEffectRub != nil && *buy.AnnualEffectRub <= 0 {
		out = append(out, Risk{
			ID:    "no_effect",
			Level: "high",
			Text:  "Годовой эффект покупки не положительный, срока окупаемости нет.",
		})
	}
	if res.VerificationFlag {
		out = append(out, Risk{
			ID:    "verification",
			Level: "high",
			Text:  "Симуляция и экономика разошлись больше чем на 15%. Не скрывайте этот флаг.",
		})
	}
	return out
}

func singleLife(r *Robot, n Norms) float64 {
	if r == nil {
		return 0
	}
	life, _ := lifetimeOf(*r, n)
	return life
}

func variantsMinLife(res Result) float64 {
	life := 0.0
	for _, v := range res.Variants {
		for _, f := range v.Fleet {
			if f.LifetimeYears > 0 && (life == 0 || f.LifetimeYears < life) {
				life = f.LifetimeYears
			}
		}
	}
	return life
}

func horizonRisk(horizon, life float64) []Risk {
	if life <= 0 || horizon <= life {
		return nil
	}
	return []Risk{{
		ID:    "horizon_over_life",
		Level: "warning",
		Text: "Горизонт " + formatYears(horizon) + " длиннее срока службы робота (" + formatYears(life) +
			"): в покупке робот покупается снова, остаток срока службы последней покупки не учитывается. В RaaS замена входит в плату.",
	}}
}

func vatRisk(a AssumptionValues) []Risk {
	if !a.VATRecoverable {
		return nil
	}
	return []Risk{{
		ID:    "vat_deduction_conditions",
		Level: "info",
		Text:  "НДС принят к вычету: в денежный расчёт идёт цена без НДС. Вычет возможен, если покупатель плательщик НДС, есть счета-фактуры и оборудование используется в облагаемой деятельности.",
	}}
}

func hasVariantFleet(in Input) bool {
	for _, v := range in.Variants {
		if len(v.Fleet) > 0 {
			return true
		}
	}
	return false
}

func variantRisks(in Input, res Result) []Risk {
	out := []Risk{
		{ID: "vat_price", Level: "info", Text: "Цена по каталогу с НДС, если набор допущений не включает вычет НДС. Доставка " + rutext.Pct(in.norms().DeliveryFrac*100, 1) + " оборудования добавлена отдельной статьёй CAPEX покупки."},
		{ID: "remaining_staff", Level: "warning", Text: "В новом OPEX остаётся не меньше 40% персонала покрытых процессов (и не меньше 2 чел). Денежная доля экономии задаётся набором допущений."},
		{ID: "raas_fee", Level: "warning", Text: "Три тарифа RaaS: фиксированный, переменный и смешанный. Ставки заданы командой, не договором. Выкуп не моделируется."},
		{ID: "shared_infra", Level: "info", Text: "Общая инфраструктура считается один раз на технический вариант, покупка и RaaS делят один состав флота."},
	}
	out = append(out, pickRisks(in, res)...)
	out = append(out, vatRisk(in.Assumptions)...)
	out = append(out, horizonRisk(res.HorizonYears, variantsMinLife(res))...)
	buy := scenarioByKind(res, "buy")
	if buy.AnnualEffectRub != nil && *buy.AnnualEffectRub <= 0 {
		out = append(out, Risk{ID: "no_effect", Level: "high", Text: "Годовой денежный эффект покупки не положительный, срока окупаемости нет."})
	}
	return out
}

// pickRisks warns about every fleet line whose robot the matching did not recommend, once per variant.
func pickRisks(in Input, res Result) []Risk {
	var out []Risk
	for _, v := range res.Variants {
		for _, f := range v.Fleet {
			item := findMatchItem(in.Match, f.SolutionID)
			if item == nil {
				continue
			}
			reasons := strings.TrimSpace(strings.Join(item.Reasons, " "))
			switch item.Status {
			case matching.StatusExcluded:
				out = append(out, Risk{
					ID:    "pick_excluded:" + v.VariantID + ":" + f.SolutionID,
					Level: "high",
					Text:  strings.TrimSpace(fmt.Sprintf("%s, %s: подбор исключил робота, но он взят в расчёт. %s", v.Name, f.Name, reasons)),
				})
			case matching.StatusNeedsReview:
				out = append(out, Risk{
					ID:    "pick_needs_review:" + v.VariantID + ":" + f.SolutionID,
					Level: "warning",
					Text:  strings.TrimSpace(fmt.Sprintf("%s, %s: подбор требует проверки. %s", v.Name, f.Name, reasons)),
				})
			}
		}
	}
	return out
}

func interpret(in Input, res Result, horizon float64, noPrice bool) []string {
	if in.Robot == nil && !hasVariantFleet(in) {
		return []string{
			"Сценарии покупки и RaaS не посчитаны: ни один робот из каталога не подходит к объекту. Причины на вкладке Роботы.",
			Disclaimer,
		}
	}
	if in.Robot == nil && hasVariantFleet(in) {
		n := 0
		for _, v := range res.Variants {
			if len(v.Fleet) > 0 {
				n++
			}
		}
		text := "Покупка и RaaS посчитаны для одного состава флота на общей базе процессов."
		if n > 1 {
			word := "технических вариантов"
			if n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14) {
				word = "технических варианта"
			}
			text = fmt.Sprintf("Сравнение %d %s на общей базе процессов. Покупка и RaaS не копируют состав флота.", n, word)
		}
		return []string{text, Disclaimer}
	}
	if noPrice {
		return []string{
			"Сценарии покупки и RaaS не посчитаны: нет цены изделия. Укажите цену в каталоге или в поле цены и повторите расчёт.",
			Disclaimer,
		}
	}
	name := in.Robot.Name
	if name == "" {
		name = "выбранное решение"
	}
	fleet := 0
	buy := scenarioByKind(res, "buy")
	if buy.FleetSize != nil {
		fleet = *buy.FleetSize
	}
	return []string{
		fmt.Sprintf("Расчёт для %s, флот %d шт, горизонт %s.", name, fleet, formatYears(horizon)),
		scenarioSentence("Покупка", buy),
		scenarioSentence("RaaS", scenarioByKind(res, "raas")),
		Disclaimer,
	}
}

func scenarioSentence(label string, sc Scenario) string {
	if sc.CapexRub == nil {
		return label + ": нет данных."
	}
	tco := "нет данных"
	if sc.TcoRub != nil {
		tco = formatRub(*sc.TcoRub)
	}
	if sc.PaybackYears == nil {
		if sc.AnnualEffectRub != nil && *sc.AnnualEffectRub > 0 {
			return fmt.Sprintf("%s: CAPEX %s, не окупается за %d лет с учётом замен. Затраты за горизонт %s.",
				label, formatRub(*sc.CapexRub), PaybackMaxYears, tco)
		}
		return fmt.Sprintf("%s: CAPEX %s, денежный эффект не положительный, срока окупаемости нет. Затраты за горизонт %s.",
			label, formatRub(*sc.CapexRub), tco)
	}
	roi := "нет данных"
	if sc.RoiPct != nil {
		roi = rutext.Pct(*sc.RoiPct, 1)
	}
	npv := "нет данных"
	if sc.NpvRub != nil {
		npv = formatRub(*sc.NpvRub)
	}
	return fmt.Sprintf("%s: CAPEX %s, окупаемость %s (%s), ROI за горизонт %s, NPV %s, затраты за горизонт %s.",
		label, formatRub(*sc.CapexRub), formatYears(*sc.PaybackYears), bandLabelRU(sc.PaybackBand), roi, npv, tco)
}

func scenarioByKind(res Result, kind string) Scenario {
	for _, sc := range res.Scenarios {
		if sc.Kind == kind {
			return sc
		}
	}
	return Scenario{Kind: kind}
}

func findMatchItem(out matching.Output, id string) *matching.Item {
	if id == "" {
		return nil
	}
	for i := range out.Items {
		if out.Items[i].SolutionID == id {
			return &out.Items[i]
		}
	}
	return nil
}

// formatYears prints years to one decimal with the agreeing word, e.g. 2,5 года or 5 лет.
func formatYears(v float64) string {
	r := math.Round(v*10) / 10
	return rutext.Num(r, 1) + " " + YearsWord(r)
}

// YearsWord agrees «год» with v. A fractional v takes «года», as in 2,5 года.
func YearsWord(v float64) string {
	if v != math.Trunc(v) {
		return "года"
	}
	n := int64(math.Abs(v))
	switch {
	case n%10 == 1 && n%100 != 11:
		return "год"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return "года"
	default:
		return "лет"
	}
}

func formatRub(v float64) string {
	return rutext.Num(math.Round(v), 0) + rubSign
}

// NOTE: a no-break space keeps the sign on the same line as the amount.
const rubSign = "\u00a0₽"
