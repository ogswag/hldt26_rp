package econ

import (
	"fmt"
	"moscow_hackathon_2026/api/internal/rutext"
)

func validateOverrides(ov Overrides) error {
	if ov.VolumeFactor != nil && *ov.VolumeFactor <= 0 {
		return &InputError{Msg: "volume_factor должен быть больше 0."}
	}
	if ov.LaborFactor != nil && *ov.LaborFactor <= 0 {
		return &InputError{Msg: "labor_factor должен быть больше 0."}
	}
	if ov.PriceRub != nil && *ov.PriceRub < 0 {
		return &InputError{Msg: "price_rub не может быть отрицательным."}
	}
	if ov.FleetSize != nil && *ov.FleetSize < 0 {
		return &InputError{Msg: "fleet_size не может быть отрицательным."}
	}
	if ov.CapexRub != nil && *ov.CapexRub < 0 {
		return &InputError{Msg: "capex_rub не может быть отрицательным."}
	}
	if ov.OpexYearRub != nil && *ov.OpexYearRub < 0 {
		return &InputError{Msg: "opex_year_rub не может быть отрицательным."}
	}
	return nil
}

func evaluate(in Input) (Result, error) {
	if in.Seed < 0 {
		return Result{}, &InputError{Msg: "Номер случайной выборки должен быть целым числом, 0 или больше."}
	}
	switch in.ObjectType {
	case "warehouse", "airport", "hospital":
	default:
		return Result{}, fmt.Errorf("econ.calculate: unknown object type")
	}
	if err := validateOverrides(in.Overrides); err != nil {
		return Result{}, err
	}
	m, err := parseParams(in.Params)
	if err != nil {
		return Result{}, err
	}

	volume := 1.0
	if in.Overrides.VolumeFactor != nil {
		volume = *in.Overrides.VolumeFactor
	}
	laborFactor := 1.0
	if in.Overrides.LaborFactor != nil {
		laborFactor = *in.Overrides.LaborFactor
	}

	horizon := horizonYears(m)
	hoursDay := hoursPerDay(in.ObjectType, m)
	hoursYear := hoursDay * workdays(in.ObjectType, m)
	nm := in.norms()
	assumptions := baseAssumptions(horizon, in.Assumptions, nm)

	kind := defaultWork(in.ObjectType)
	if in.Robot != nil {
		kind = classifyWork(in.ObjectType, *in.Robot)
	}
	lab := laborFor(in.ObjectType, kind, m, laborFactor)
	if len(in.Processes) > 0 {
		lab = laborFromProcesses(in.ObjectType, in.Processes, m, laborFactor)
	}
	if lab.Assumed {
		assumptions = append(assumptions, "Численность уборки не задана в параметрах, принята оценка "+rutext.Num(AssumedCleanerHead, 1)+" чел")
	}
	if in.ObjectType == "hospital" && kind == WorkHospitalCart {
		assumptions = append(assumptions, "Зарплата транспорта белья принята равной зарплате санитара: отдельной ставки в параметрах нет.")
	}
	if in.ObjectType == "airport" {
		assumptions = append(assumptions, "Рабочий день аэропорта принят 18 ч, если иное не задано схемой.")
	}
	if in.ObjectType == "hospital" {
		assumptions = append(assumptions, "Медучреждение считается круглосуточным (24 ч). Пик логистики 1,5 к среднечасовой.")
	}

	laborShare := in.Assumptions.laborCashShare()
	baselineOpex := lab.AnnualFot
	remain, remainFot, laborCashSaved, _ := remainLabor(lab.Headcount, lab.AnnualFot, laborShare)

	res := Result{
		ModelVersion:     ModelVersion,
		ObjectType:       in.ObjectType,
		Seed:             in.Seed,
		Match:            in.Match,
		VerificationFlag: false,
		HorizonYears:     horizon,
		Formulas:         formulas(nm),
		Units:            units(),
		Sources:          sources(nm),
		Breakdown: []Line{
			{Scenario: "baseline", Bucket: "opex", ID: "fot", Label: "ФОТ целевых ролей с начислениями", Rub: baselineOpex, Note: lab.Label},
		},
	}

	baseSc, _ := scenarioFrom(scenarioIn{Kind: "baseline", Opex: baselineOpex, Horizon: horizon})
	res.Scenarios = []Scenario{baseSc, {Kind: "buy"}, {Kind: "raas"}}
	res.AssumptionSet = ptrAssumption(in.Assumptions.view(in.baseNorms()))
	res.Origins = metricOrigins(PriceSourceCatalog, len(in.Processes) > 0)

	if in.Robot == nil {
		assumptions = append(assumptions, pickMessage(in.PickReason))
		res.Assumptions = assumptions
		attachView(&res, in, horizon, false, false, false)
		return res, nil
	}

	assumptions = append(assumptions, pickMessage(in.PickReason))
	assumptions = append(assumptions, "Процесс расчёта: "+workLabel(kind)+".")

	catalogList := derefFloat(in.Robot.PriceRub)
	priceSource := PriceSourceCatalog
	listPrice := catalogList
	if in.Overrides.PriceRub != nil {
		listPrice = *in.Overrides.PriceRub
		priceSource = PriceSourceOverride
	}
	price := cashPrice(listPrice, in.Assumptions)
	if price <= 0 {
		assumptions = append(assumptions, "Нет цены изделия. Сценарии покупки и RaaS не посчитаны. Укажите цену в каталоге или в overrides.price_rub.")
		res.Assumptions = assumptions
		attachView(&res, in, horizon, false, false, true)
		return res, nil
	}

	peak, unit := peakOps(in.ObjectType, kind, m, volume)
	th, thNote := throughput(kind, *in.Robot, m)
	if thNote != "" {
		assumptions = append(assumptions, thNote)
	}
	computedFleet := fleetSize(peak, th, nm)
	fleet := computedFleet
	if in.Overrides.FleetSize != nil {
		fleet = *in.Overrides.FleetSize
		res.Overrides = append(res.Overrides, OverrideFlag{
			Field: "fleet_size", Value: float64(fleet), Computed: float64(computedFleet), Flag: true,
		})
	}

	integFrac := nm.IntegrationNoFrac
	if hasIT(in.ObjectType, m) {
		integFrac = nm.IntegrationYesFrac
	} else {
		assumptions = append(assumptions, noITAssumption(nm))
	}

	item := newFleetItem(*in.Robot, fleet, listPrice, in.Assumptions, nm)
	cm := buildCosts(costInput{
		Items: []fleetItem{item}, HoursYear: hoursYear, IntegFrac: integFrac, SharedKey: "buy", Assumptions: in.Assumptions,
		Norms: nm, Shifts: shiftsPerDay(in.ObjectType, m), PayrollTax: taxFactor(m), LaborFactor: laborFactor,
	})
	if item.LifeAssumed {
		assumptions = append(assumptions, lifeAssumption(nm, ""))
	}
	if item.ServiceAssumed {
		assumptions = append(assumptions, serviceAssumption(nm, ""))
	}

	fee := raasFee("fixed", cm.Equipment, 0, 1, parseRaas(nil, nm)).Fee
	computedBuyCapex, computedRaasCapex := cm.BuyCapex, cm.RaasCapex
	computedBuyOpex := cm.buyOpex(remainFot)
	raasOpex := cm.raasOpex(remainFot, fee)

	buyCapex := computedBuyCapex
	buyOpex := computedBuyOpex
	if in.Overrides.CapexRub != nil {
		buyCapex = roundRub(*in.Overrides.CapexRub)
		res.Overrides = append(res.Overrides, OverrideFlag{
			Field: "capex_rub", Value: buyCapex, Computed: computedBuyCapex, Flag: true,
		})
	}
	if in.Overrides.OpexYearRub != nil {
		buyOpex = roundRub(*in.Overrides.OpexYearRub)
		res.Overrides = append(res.Overrides, OverrideFlag{
			Field: "opex_year_rub", Value: buyOpex, Computed: computedBuyOpex, Flag: true,
		})
	}
	if in.Overrides.PriceRub != nil {
		res.Overrides = append(res.Overrides, OverrideFlag{
			Field: "price_rub", Value: listPrice, Computed: catalogList, Flag: true,
		})
	}
	if in.Overrides.VolumeFactor != nil {
		res.Overrides = append(res.Overrides, OverrideFlag{
			Field: "volume_factor", Value: volume, Computed: 1, Flag: true,
		})
	}
	if in.Overrides.LaborFactor != nil {
		res.Overrides = append(res.Overrides, OverrideFlag{
			Field: "labor_factor", Value: laborFactor, Computed: 1, Flag: true,
		})
	}
	sid := in.Robot.ID
	discount := in.Assumptions.discountRate()
	buySc, buyFlow := scenarioFrom(scenarioIn{
		Kind: "buy", SolutionID: &sid, Fleet: fleet, Capex: buyCapex, Opex: buyOpex, Baseline: baselineOpex,
		LaborCashSaved: laborCashSaved, Horizon: horizon, Discount: discount, Purchases: cm.purchases(), Depreciation: cm.Depreciation,
		BatteryFrac: nm.BatteryFrac, BatteryYears: nm.BatteryYears,
	})
	buySc.PriceSource = priceSource
	raasSc, _ := scenarioFrom(scenarioIn{
		Kind: "raas", SolutionID: &sid, Fleet: fleet, Capex: computedRaasCapex, Opex: raasOpex, Baseline: baselineOpex,
		LaborCashSaved: laborCashSaved, Horizon: horizon, Discount: discount, Depreciation: cm.RaasDepreciation,
	})
	raasSc.PriceSource = priceSource
	raasSc.Tariff = "fixed"
	res.Scenarios[1] = buySc
	res.Scenarios[2] = raasSc

	res.Breakdown = append(res.Breakdown,
		Line{Scenario: "buy", Bucket: "capex", ID: "equipment", Label: "Оборудование (цена x флот)", Rub: cm.Equipment, Note: vatNote(in.Assumptions)},
		cm.delivery("buy", nm),
		Line{Scenario: "buy", Bucket: "capex", ID: "infrastructure", Label: "Инфраструктура " + shareOfEquipment(nm.InfraFrac), Rub: cm.Infra},
		Line{Scenario: "buy", Bucket: "capex", ID: "software", Label: "ПО " + shareOfEquipment(nm.SoftwareFrac), Rub: cm.Software},
		Line{Scenario: "buy", Bucket: "capex", ID: "integration", Label: "Интеграция", Rub: cm.Integration, Note: pctNote(integFrac)},
		Line{Scenario: "buy", Bucket: "capex", ID: "commissioning", Label: "Пусконаладка " + shareOfEquipment(nm.CommissioningFrac), Rub: cm.Commissioning},
		Line{Scenario: "buy", Bucket: "capex", ID: "training", Label: "Обучение " + shareOfEquipment(nm.TrainingFrac), Rub: cm.Training},
		Line{Scenario: "buy", Bucket: "capex", ID: "contingency", Label: "Резерв " + rutext.Pct(nm.ContingencyFrac*100, 1) + " суммы статей", Rub: cm.BuyCont},
		Line{Scenario: "buy", Bucket: "opex", ID: "fot_remaining", Label: "ФОТ оставшегося целевого персонала", Rub: remainFot, Note: rutext.Num(remain, 1) + " чел, денежная доля " + rutext.Num(laborShare*100, 1) + "%"},
		Line{Scenario: "buy", Bucket: "opex", ID: "service", Label: "Сервис и ремонт", Rub: cm.Service},
		Line{Scenario: "buy", Bucket: "opex", ID: "licenses", Label: "Лицензии", Rub: cm.Licenses},
	)
	res.Breakdown = append(res.Breakdown, cm.operationLines("buy", nm)...)
	res.Breakdown = append(res.Breakdown, replacementLines("buy", buyFlow, nm)...)
	res.Breakdown = append(res.Breakdown,
		Line{Scenario: "raas", Bucket: "capex", ID: "infrastructure", Label: "Инфраструктура " + shareOfEquipment(nm.InfraFrac), Rub: cm.Infra},
		Line{Scenario: "raas", Bucket: "capex", ID: "software", Label: "ПО " + shareOfEquipment(nm.SoftwareFrac), Rub: cm.Software},
		Line{Scenario: "raas", Bucket: "capex", ID: "integration", Label: "Интеграция", Rub: cm.Integration},
		Line{Scenario: "raas", Bucket: "capex", ID: "commissioning", Label: "Пусконаладка " + shareOfEquipment(nm.CommissioningFrac), Rub: cm.Commissioning},
		Line{Scenario: "raas", Bucket: "capex", ID: "training", Label: "Обучение " + shareOfEquipment(nm.TrainingFrac), Rub: cm.Training},
		Line{Scenario: "raas", Bucket: "capex", ID: "contingency", Label: "Резерв " + rutext.Pct(nm.ContingencyFrac*100, 1) + " суммы статей RaaS", Rub: cm.RaasCont},
		Line{Scenario: "raas", Bucket: "opex", ID: "fot_remaining", Label: "ФОТ оставшегося целевого персонала", Rub: remainFot},
		Line{Scenario: "raas", Bucket: "opex", ID: "raas_fee", Label: "Плата RaaS (доля цены в месяц x 12 x цена x флот)", Rub: fee, Note: rutext.Num(nm.RaasMonthlyFrac*100, 1) + "% цены в месяц"},
	)
	res.Breakdown = append(res.Breakdown, cm.operationLines("raas", nm)...)

	res.Shared = &Shared{
		FleetSize:  fleet,
		Chargers:   cm.Chargers,
		Throughput: round4(th),
		PeakOps:    round4(peak),
		Unit:       unit,
		WorkKind:   kind,
	}
	res.Assumptions = assumptions
	res.AssumptionSet = ptrAssumption(in.Assumptions.view(in.baseNorms()))
	res.Origins = metricOrigins(priceSource, len(in.Processes) > 0)
	attachView(&res, in, horizon, item.LifeAssumed, item.ServiceAssumed, false)
	return res, nil
}

type scenarioIn struct {
	Kind           string
	SolutionID     *string
	Fleet          int
	Capex          float64
	Opex           float64
	Baseline       float64
	LaborCashSaved float64
	Horizon        float64
	Discount       float64
	Purchases      []purchase
	Depreciation   float64
	BatteryFrac    float64
	BatteryYears   float64
}

// scenarioFrom reads every metric of a scenario off its cash flow, so payback, NPV, ROI and TCO come from one schedule.
func scenarioFrom(s scenarioIn) (Scenario, flowResult) {
	capex := roundRub(s.Capex)
	opex := roundRub(s.Opex)
	sc := Scenario{
		Kind:              s.Kind,
		SolutionID:        s.SolutionID,
		FleetSize:         iptr(s.Fleet),
		CapexRub:          fptr(capex),
		OpexYearRub:       fptr(opex),
		LaborCashSavedRub: fptr(roundRub(s.LaborCashSaved)),
	}
	if s.Kind == "baseline" {
		sc.TcoRub = fptr(roundRub(opex * s.Horizon))
		sc.AnnualEffectRub = fptr(0)
		return sc, flowResult{}
	}
	effect := roundRub(s.Baseline - opex)
	fr := runCase(cashCase{Capex: capex, Effect: effect, Purchases: s.Purchases, Horizon: s.Horizon, Discount: s.Discount, BatteryFrac: s.BatteryFrac, BatteryYears: s.BatteryYears})
	sc.AnnualEffectRub = fptr(effect)
	sc.AccountingEffectRub = fptr(roundRub(float64(effect) - s.Depreciation))
	sc.TcoRub = fptr(roundRub(capex + opex*s.Horizon + fr.BatteryRub + fr.ReplacementRub))
	sc.PaybackYears = fr.PaybackYears
	sc.PaybackBand = paybackBand(fr.PaybackYears)
	sc.DiscountedPaybackYears = fr.DiscountedPaybackYears
	sc.RoiPct = fr.RoiPct
	sc.NpvRub = fptr(fr.NpvRub)
	sc.IrrPct = fr.IrrPct
	sc.CashFlow = fr.Rows
	return sc, fr
}

func replacementLines(scenario string, fr flowResult, n Norms) []Line {
	var out []Line
	if fr.BatteryRub > 0 {
		out = append(out, Line{Scenario: scenario, Bucket: "replacement", ID: "battery", Label: "Замена батарей за горизонт", Rub: fr.BatteryRub,
			Note: batteryRule(n) + " внутри срока службы"})
	}
	if fr.ReplacementRub > 0 {
		out = append(out, Line{Scenario: scenario, Bucket: "replacement", ID: "robot_replacement", Label: "Повторная закупка роботов за горизонт", Rub: fr.ReplacementRub,
			Note: "робот покупается снова, когда его срок службы кончается внутри горизонта"})
	}
	return out
}

func batteryRule(n Norms) string {
	return rutext.Pct(n.BatteryFrac*100, 1) + " цены робота каждые " + formatYears(n.BatteryYears)
}

func noITAssumption(n Norms) string {
	return "Нет признака WMS/AODB/МИС, интеграция принята " + rutext.Pct(n.IntegrationNoFrac*100, 1) + " от оборудования. Иначе " +
		rutext.Pct(n.IntegrationYesFrac*100, 1) + "."
}

func lifeAssumption(n Norms, names string) string {
	text := "Срок службы не задан в ТТХ, принят " + formatYears(n.DefaultLifetimeYears)
	if names != "" {
		text += ": " + names
	}
	return text + ". По нему считаются повторная закупка робота и амортизация."
}

func serviceAssumption(n Norms, names string) string {
	text := "Доля сервиса не задана в ТТХ, принята " + rutext.Pct(n.DefaultServiceFrac*100, 1) + " от стоимости оборудования в год"
	if names != "" {
		text += ": " + names
	}
	return text + "."
}

func vatNote(a AssumptionValues) string {
	if a.VATRecoverable {
		return "без НДС, НДС " + rutext.Pct(a.vatRate()*100, 1) + " к вычету"
	}
	return ""
}

func ptrAssumption(v AssumptionView) *AssumptionView {
	return &v
}

func metricOrigins(priceSource string, fromProcesses bool) []Origin {
	priceNote := "Цена из каталога с НДС."
	if priceSource == PriceSourceOverride {
		priceNote = "Проектная цена. Источник или причина указаны в варианте."
	}
	baseNote := "ФОТ целевых ролей из параметров объекта."
	if fromProcesses {
		baseNote = "ФОТ базовых процессов проекта, один раз на все варианты."
	}
	return []Origin{
		{Metric: "price_rub", Source: priceSource, Note: priceNote},
		{Metric: "opex_year_rub", Source: "process", Ref: "baseline_opex", Note: baseNote + " Замены батарей и роботов сюда не входят, они разовые."},
		{Metric: "annual_effect_rub", Source: "formula", Ref: "cash_effect", Note: "Денежный эффект: OPEX базы минус постоянный OPEX сценария, без амортизации."},
		{Metric: "payback_years", Source: "formula", Ref: "payback", Note: "Простая окупаемость: момент, когда накопленный денежный поток достигает нуля. Без замен до него равна CAPEX / денежный эффект."},
		{Metric: "discounted_payback_years", Source: "formula", Ref: "discounted_payback", Note: "Та же окупаемость по потокам, приведённым по ставке дисконтирования набора допущений."},
		{Metric: "npv_rub", Source: "formula", Ref: "npv", Note: "Чистая приведённая стоимость за горизонт, в постоянных ценах, без остаточной стоимости оборудования."},
		{Metric: "irr_pct", Source: "formula", Ref: "irr", Note: "Ставка, при которой NPV за горизонт равен нулю."},
		{Metric: "roi_pct", Source: "formula", Ref: "roi", Note: "Классический ROI за горизонт: (сумма потоков - CAPEX) / CAPEX, без остаточной стоимости оборудования."},
		{Metric: "tco_rub", Source: "formula", Ref: "tco", Note: "Затраты за горизонт в ценах базового года, без дисконта: CAPEX, OPEX, замены. Остаточная стоимость оборудования не учитывается."},
		{Metric: "labor_cash_saved_rub", Source: "assumption", Ref: "labor_cash_share", Note: "Денежная экономия труда: высвобождённый ФОТ x доля денежной экономии."},
	}
}

func pickMessage(reason string) string {
	switch reason {
	case "selected":
		return "Для расчёта взят робот, выбранный в проекте."
	case "recommended":
		return "Взят предложенный робот, лучший из подходящих. Почему он, сказано на вкладке Роботы."
	case "needs_review":
		return "Ни один робот не прошёл все проверки, взят лучший из требующих проверки. Что уточнить, сказано на вкладке Роботы."
	default:
		return "Ни один робот из каталога не подходит к объекту, покупка и RaaS не посчитаны. Причины на вкладке Роботы."
	}
}

func baseAssumptions(horizon float64, a AssumptionValues, n Norms) []string {
	out := []string{
		"Собственные средства, без кредита и лизинга.",
		"Расчёт по годам в постоянных ценах базового года: инвестиции в нулевой год, эффекты и платежи в конце года, без инфляции и роста зарплат.",
		"Главная метрика: простая окупаемость, момент, когда накопленный денежный поток достигает нуля. NPV, IRR и дисконтированная окупаемость даны справочно, ставка дисконтирования " + rutext.Pct(a.discountRate()*100, 1) + " в год из набора допущений.",
		"Налог на прибыль и амортизационный налоговый щит не моделируются. Амортизация показана для бухгалтерского эффекта и не входит в денежный поток.",
		"Срок расчёта равен горизонту: " + formatYears(horizon) + ". Робот, чей срок службы кончается внутри горизонта, покупается снова. Остаточная стоимость оборудования на конец горизонта не учитывается: затраты за горизонт, NPV и ROI консервативны, особенно если робота пришлось купить снова незадолго до конца горизонта.",
		"Замена батареи: " + batteryRule(n) + " внутри срока службы, разовым платежом в своём году.",
		"RaaS: по умолчанию " + rutext.Pct(n.RaasMonthlyFrac*100, 1) + " цены в месяц, переменный тариф растёт с годовым объёмом, смешанный берёт половину от каждого. Срок контракта равен горизонту. Оборудование, доставка, батареи и замена роботов входят в плату. Ставки заданы командой, не договором.",
		"Доступность " + rutext.Pct(n.Availability*100, 1) + ", загрузка " + rutext.Pct(n.Utilization*100, 1) + ", резерв " + rutext.Pct(n.Reserve*100, 1) + ". Зарядка учтена в доступности. По нормативу загрузка 80% взята из середины диапазона AMR 70-85% легенды датасета.",
		"Денежная экономия труда задаётся долей высвобождённого ФОТ.",
		"Начисления на ФОТ заданы параметром объекта, взносы выше предельной базы " + rutext.Num(PayrollCapRub, 0) + " ₽ на человека в год считаются по ставке " + rutext.Pct(PayrollReducedRate*100, 1) + ".",
		"Энергия: " + rutext.Num(n.EnergyKW, 1) + " кВт на робота, " + rutext.Num(n.EnergyRubPerKWh, 1) + " ₽/кВт·ч. Лицензии " + rutext.Num(n.LicensePerRobot, 0) + " ₽, расходники " + rutext.Num(n.ConsumablePerRobot, 0) + " ₽ и связь " + rutext.Num(n.CommRubPerRobotYear, 0) + " ₽ на робота в год. Это суммы с НДС.",
		"Сервис и ремонт: доля стоимости оборудования в год, отдельной статьи на ремонт нет. Доставка " + rutext.Pct(n.DeliveryFrac*100, 1) + " оборудования входит в CAPEX покупки.",
		"Персонал эксплуатации: один техник на " + rutext.Num(n.RobotsPerTechnician, 0) + " роботов в каждую смену, зарплата " + rutext.Num(n.TechnicianWageMonthRub, 0) + " ₽ в месяц, начисления как у остальных ролей. Зарплата НДС не облагается.",
		"Число зарядных станций считается по времени работы и зарядки роботов из ТТХ. Для робота без этих данных принята одна станция на " + rutext.Num(n.RobotsPerCharger, 0) + " робота. Стоимость станций входит в инфраструктуру, отдельной статьи на них нет.",
		"Нет прочих текущих затрат процесса в параметрах, кроме ФОТ целевых ролей.",
		"Общая инфраструктура, ПО, интеграция, ПНР и обучение считаются один раз на вариант, не на каждую позицию флота.",
		vatAssumption(a),
	}
	return out
}

func vatAssumption(a AssumptionValues) string {
	rate := rutext.Pct(a.vatRate()*100, 1)
	switch {
	case a.VATRecoverable:
		return "НДС " + rate + " принят к вычету: в денежный расчёт идёт цена без НДС, включая энергию, лицензии, расходники и общие статьи. Зарплата НДС не облагается. Вычет требует, чтобы покупатель был плательщиком НДС и оборудование использовалось в облагаемой деятельности."
	case !a.pricesIncludeVAT():
		return "НДС " + rate + " добавлен к ценам без НДС, к вычету не принимается: в денежный расчёт идёт цена с НДС."
	default:
		return "НДС " + rate + " входит в цены и не принимается к вычету: в денежный расчёт идёт цена с НДС."
	}
}

func formulas(n Norms) []Formula {
	pct := func(v float64) string { return rutext.Pct(v*100, 1) }
	out := []Formula{
		{ID: "fleet", Text: "флот = пик / (производительность x " + rutext.Fixed(n.Availability, 2) + " x " + rutext.Fixed(n.Utilization, 2) + "), вверх до целого, x " + rutext.Fixed(1+n.Reserve, 2) + ", вверх до целого", Unit: "шт"},
		{ID: "chargers", Text: "зарядные станции = для позиции с ТТХ вверх до целого от количества x время зарядки / (время работы + время зарядки), без ТТХ вверх до целого от количества / " + rutext.Num(n.RobotsPerCharger, 0) + ", у флота не меньше одной", Unit: "шт"},
		{ID: "capex_buy", Text: "CAPEX покупка = (оборудование + доставка + инфраструктура + ПО + интеграция + ПНР + обучение) x " + rutext.Fixed(1+n.ContingencyFrac, 2), Unit: "₽"},
		{ID: "capex_raas", Text: "CAPEX RaaS = (инфраструктура + ПО + интеграция + ПНР + обучение) x " + rutext.Fixed(1+n.ContingencyFrac, 2) + ", без оборудования и доставки", Unit: "₽"},
		{ID: "delivery", Text: "доставка = " + pct(n.DeliveryFrac) + " оборудования, только в CAPEX покупки", Unit: "₽"},
		{ID: "opex_buy", Text: "OPEX покупка = ФОТ остатка + сервис и ремонт + лицензии + энергия + расходники + связь + персонал эксплуатации, без разовых замен", Unit: "₽/год"},
		{ID: "opex_raas", Text: "OPEX RaaS = ФОТ остатка + плата тарифа + энергия + расходники + связь + персонал эксплуатации", Unit: "₽/год"},
		{ID: "technicians", Text: "персонал эксплуатации = вверх до целого от флота / " + rutext.Num(n.RobotsPerTechnician, 0) + ", x число смен x зарплата x 12 x (1 + начисления)", Unit: "₽/год"},
		{ID: "baseline_opex", Text: "OPEX базы = сумма ФОТ базовых процессов (или целевых ролей) x коэффициент начислений", Unit: "₽/год"},
		{ID: "payroll", Text: "ФОТ = штат x оклад x 12 x (1 + начисления), взносы выше предельной базы 2 979 000 ₽ на человека в год по ставке 15,1%", Unit: "₽/год"},
		{ID: "vat", Text: "цена в денежном расчёте = цена с НДС / (1 + ставка НДС) при вычете НДС, иначе цена с НДС", Unit: "₽"},
		{ID: "cash_effect", Text: "денежный эффект = OPEX базы - OPEX сценария", Unit: "₽/год"},
		{ID: "battery", Text: "замена батареи = " + batteryRule(n) + ", пока это внутри срока службы и горизонта", Unit: "₽"},
		{ID: "replacement", Text: "повторная покупка = цена робота в год, когда кончается его срок службы, если он внутри горизонта", Unit: "₽"},
		{ID: "cash_flow", Text: "денежный поток года t = денежный эффект - замена батарей - повторная покупка. Год 0 = -CAPEX", Unit: "₽"},
		{ID: "payback", Text: "простая окупаемость = момент, когда накопленный денежный поток достигает нуля. Без замен до этого момента = CAPEX / денежный эффект", Unit: "лет"},
		{ID: "discounted_payback", Text: "дисконтированная окупаемость = тот же момент для потоков, поделённых на (1 + r) в степени t", Unit: "лет"},
		{ID: "npv", Text: "NPV = -CAPEX + сумма по годам t от 1 до горизонта: денежный поток года t / (1 + r) в степени t", Unit: "₽"},
		{ID: "irr", Text: "IRR = ставка r, при которой NPV равен нулю", Unit: "%"},
		{ID: "roi", Text: "ROI = (сумма денежных потоков за горизонт - CAPEX) / CAPEX x 100", Unit: "%"},
		{ID: "tco", Text: "Затраты за горизонт (TCO) = CAPEX + OPEX за год x горизонт + замены батарей + повторные покупки", Unit: "₽"},
		{ID: "depreciation", Text: "амортизация линейная: робот = цена / его срок службы, доставка, инфраструктура, ПО, интеграция и ПНР = стоимость / " + formatYears(n.DefaultLifetimeYears) + ". Обучение и резерв не амортизируются", Unit: "₽/год"},
		{ID: "accounting_effect", Text: "бухгалтерский эффект = денежный эффект - амортизация", Unit: "₽/год"},
		{ID: "labor_cash", Text: "денежная экономия труда = высвобождённый ФОТ x доля денежной экономии", Unit: "₽/год"},
		{ID: "raas_fixed", Text: "RaaS фиксированный = доля цены в месяц x 12 x цена x флот", Unit: "₽/год"},
		{ID: "raas_variable", Text: "RaaS переменный = ставка за операцию x годовой объём. Без своей ставки берётся ставка, выведенная из фиксированной платы при плановом объёме", Unit: "₽/год"},
		{ID: "raas_mixed", Text: "RaaS смешанный = доля фиксированной части x фиксированный + (1 - доля) x переменный", Unit: "₽/год"},
	}
	for i := range out {
		out[i].MathML = FormulaMathML(out[i].ID, n)
	}
	return out
}

func units() map[string]string {
	return map[string]string{
		"capex_rub":                "₽",
		"opex_year_rub":            "₽/год",
		"annual_effect_rub":        "₽/год",
		"tco_rub":                  "₽",
		"payback_years":            "лет",
		"discounted_payback_years": "лет",
		"npv_rub":                  "₽",
		"irr_pct":                  "%",
		"roi_pct":                  "%",
		"accounting_effect_rub":    "₽/год",
		"labor_cash_saved_rub":     "₽/год",
		"fleet_size":               "шт",
		"price_rub":                "₽",
	}
}

func sources(n Norms) []string {
	return []string{
		"Цена изделия: каталог роботов, с НДС.",
		"ФОТ, режим, объёмы, горизонт: параметры объекта (демо-колонка датасета).",
		"Начисления на ФОТ: параметр объекта «Начисления на ФОТ», по умолчанию 1,302. Предельная база взносов 2 979 000 ₽ на человека в год и ставка 15,1% выше неё: правила 2026 года.",
		"Доступность и загрузка: набор допущений, а по нормативу легенда датасета (AMR 70-85%): середина " + rutext.Pct(n.Utilization*100, 0) + " и доступность " + rutext.Pct(n.Availability*100, 0) + ".",
		"Статьи CAPEX сверх изделия и доставка: допущение команды, значения по нормативам модели.",
		"Связь, персонал эксплуатации, число зарядных станций без ТТХ: допущение команды, значения по нормативам модели.",
		"Ставка RaaS: допущение команды. Фиксированный " + rutext.Pct(n.RaasMonthlyFrac*100, 1) + " в месяц, переменный от объёма, смешанный 50/50.",
		"НДС: ставка и вычет задаются набором допущений. Цена из каталога с НДС, при вычете в денежный расчёт идёт цена без НДС.",
		"Ставка дисконтирования: набор допущений, по умолчанию 15% в год, постоянные цены.",
		"Срок службы и доля сервиса: ТТХ робота, при пропуске допущение модели: " + formatYears(n.DefaultLifetimeYears) + " и " + rutext.Pct(n.DefaultServiceFrac*100, 1) + " в год.",
	}
}

func pctNote(frac float64) string {
	return rutext.Num(frac*100, 1) + "% от оборудования"
}

func derefFloat(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
