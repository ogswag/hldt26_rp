package econ

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"moscow_hackathon_2026/api/internal/rutext"
)

type raasParams struct {
	MonthlyFrac   float64 `json:"monthly_frac"`
	MixFixedShare float64 `json:"mix_fixed_share"`
	PerOpRub      float64 `json:"per_op_rub"`
}

func compareProject(in Input) (Result, error) {
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
	if len(in.Variants) > 6 {
		return Result{}, &InputError{Msg: "Сравниваются не больше шести вариантов. Удалите лишние."}
	}
	env, lab, err := newVariantEnv(in)
	if err != nil {
		return Result{}, err
	}
	horizon, laborShare, baselineOpex := env.horizon, env.laborShare, env.baselineOpex
	nm := env.nm
	assumptions := baseAssumptions(horizon, in.Assumptions, nm)
	if laborShare != 1 {
		assumptions = append(assumptions, "Доля денежной экономии труда: "+rutext.Num(laborShare*100, 1)+"%. Остальной высвобождённый ФОТ не считается живыми деньгами.")
	}

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
		AssumptionSet:    ptrAssumption(in.Assumptions.view(in.baseNorms())),
		Origins:          metricOrigins(PriceSourceCatalog, len(in.Processes) > 0),
		Breakdown: []Line{
			{Scenario: "baseline", Bucket: "opex", ID: "fot", Label: "ФОТ базовых процессов с начислениями", Rub: baselineOpex, Note: lab.Label},
		},
	}
	baseSc, _ := scenarioFrom(scenarioIn{Kind: "baseline", Opex: baselineOpex, Horizon: horizon})
	res.Scenarios = []Scenario{baseSc}

	robots := map[string]Robot{}
	for _, r := range in.Robots {
		robots[r.ID] = r
	}
	if in.Robot != nil {
		robots[in.Robot.ID] = *in.Robot
	}

	if env.integFrac == nm.IntegrationNoFrac {
		assumptions = append(assumptions, noITAssumption(nm))
	}

	var lifeNames, svcNames []string

	for _, v := range in.Variants {
		ev := evalVariant(in, v, robots, env)
		vr := ev.Result
		res.Variants = append(res.Variants, vr)
		res.Breakdown = append(res.Breakdown, ev.Lines...)
		assumptions = append(assumptions, ev.Notes...)
		lifeNames = appendNew(lifeNames, ev.LifeAssumedNames...)
		svcNames = appendNew(svcNames, ev.ServiceAssumedNames...)
		for _, sc := range vr.Scenarios {
			res.Scenarios = append(res.Scenarios, sc)
		}
		if res.Shared == nil && len(vr.Fleet) > 0 && len(vr.Scenarios) > 0 {
			fleet := 0
			if vr.Scenarios[0].FleetSize != nil {
				fleet = *vr.Scenarios[0].FleetSize
			}
			wk := vr.Fleet[0].WorkKind
			peak, unit := peakOps(in.ObjectType, wk, env.m, env.volume)
			// The peak the check holds the fleet to is the demand of the processes its robots of this kind serve.
			var codes []string
			for _, f := range vr.Fleet {
				if f.WorkKind == wk {
					codes = append(codes, f.TaskCodes...)
				}
			}
			if p, ok := processPeak(in, env.m, wk, codes, env.volume); ok && len(codes) > 0 {
				peak = p
			}
			th := 0.0
			if robot, ok := robots[vr.Fleet[0].SolutionID]; ok {
				th, _ = throughput(wk, robot, env.m)
			}
			res.Shared = &Shared{
				FleetSize:  fleet,
				Chargers:   vr.Chargers,
				Throughput: round4(th),
				PeakOps:    round4(peak),
				WorkKind:   wk,
				Unit:       unit,
			}
			if len(vr.Fleet) == 1 {
				res.SolutionName = vr.Fleet[0].Name
			} else if len(vr.Fleet) > 1 {
				res.SolutionName = v.Name
			}
		}
	}
	if len(lifeNames) > 0 {
		assumptions = append(assumptions, lifeAssumption(nm, strings.Join(lifeNames, ", ")))
	}
	if len(svcNames) > 0 {
		assumptions = append(assumptions, serviceAssumption(nm, strings.Join(svcNames, ", ")))
	}
	res.Assumptions = assumptions
	attachView(&res, in, horizon, len(lifeNames) > 0, len(svcNames) > 0, false)
	res.Sensitivity = sensitivityVariants(in, robots, env)
	if len(res.Sensitivity) > 0 {
		res.Assumptions = append(res.Assumptions, volumeFleetAssumption())
	}
	return res, nil
}

// variantEnv is what the variants of one calculation share: the parameters and the baseline.
type variantEnv struct {
	m                                                    map[string]any
	volume, laborFactor, laborShare, hoursYear, workDays float64
	horizon, baselineOpex, integFrac                     float64
	shifts, payrollTax                                   float64
	// nm is what the calculation runs on: the norms with the assumption set applied.
	nm Norms
}

func newVariantEnv(in Input) (variantEnv, laborPool, error) {
	m, err := parseParams(in.Params)
	if err != nil {
		return variantEnv{}, laborPool{}, err
	}
	nm := in.norms()
	env := variantEnv{m: m, volume: 1, laborFactor: 1, integFrac: nm.IntegrationNoFrac, nm: nm}
	if in.Overrides.VolumeFactor != nil {
		env.volume = *in.Overrides.VolumeFactor
	}
	if in.Overrides.LaborFactor != nil {
		env.laborFactor = *in.Overrides.LaborFactor
	}
	env.horizon = horizonYears(m)
	env.workDays = workdays(in.ObjectType, m)
	env.hoursYear = hoursPerDay(in.ObjectType, m) * env.workDays
	env.laborShare = in.Assumptions.laborCashShare()
	env.shifts = shiftsPerDay(in.ObjectType, m)
	env.payrollTax = taxFactor(m)
	if hasIT(in.ObjectType, m) {
		env.integFrac = nm.IntegrationYesFrac
	}
	lab := laborFromProcesses(in.ObjectType, in.Processes, m, env.laborFactor)
	if len(in.Processes) == 0 {
		kind := defaultWork(in.ObjectType)
		if in.Robot != nil {
			kind = classifyWork(in.ObjectType, *in.Robot)
		}
		lab = laborFor(in.ObjectType, kind, m, env.laborFactor)
	}
	env.baselineOpex = lab.AnnualFot
	return env, lab, nil
}

type variantEval struct {
	Result              VariantResult
	Lines               []Line
	Notes               []string
	LifeAssumedNames    []string
	ServiceAssumedNames []string
}

func evalVariant(in Input, v VariantSpec, robots map[string]Robot, env variantEnv) variantEval {
	m, volume, laborFactor, laborShare := env.m, env.volume, env.laborFactor, env.laborShare
	workDays, horizon, baselineOpex := env.workDays, env.horizon, env.baselineOpex
	out := VariantResult{VariantID: v.ID, Name: v.Name, Fleet: []FleetKPI{}, Scenarios: []Scenario{}}
	ev := variantEval{Result: out}
	notes := []string{}
	if v.Name != "" {
		notes = append(notes, "Вариант \""+v.Name+"\": технический состав один, финансирование считается отдельно.")
	}
	var items []fleetItem
	covered := map[string]bool{}
	primarySID := (*string)(nil)

	for _, f := range v.Fleet {
		if f.Quantity < 1 || f.SolutionID == "" {
			continue
		}
		r, ok := robots[f.SolutionID]
		if !ok {
			notes = append(notes, "Позиция флота "+f.SolutionID+" не найдена в каталоге и пропущена.")
			continue
		}
		catalog := derefFloat(r.PriceRub)
		source := PriceSourceCatalog
		list := catalog
		if f.PriceOverrideRub != nil {
			list = *f.PriceOverrideRub
			source = PriceSourceOverride
		} else if in.Overrides.PriceRub != nil && len(v.Fleet) == 1 {
			list = *in.Overrides.PriceRub
			source = PriceSourceOverride
		}
		item := newFleetItem(r, f.Quantity, list, in.Assumptions, env.nm)
		if item.CashRub <= 0 {
			notes = append(notes, "Нет цены для "+r.Name+". Позиция не вошла в CAPEX.")
			continue
		}
		kind := classifyWork(in.ObjectType, r)
		codes := f.TaskCodes
		if len(codes) == 0 {
			codes = defaultTaskCodes(kind)
		}
		for _, c := range codes {
			covered[c] = true
		}
		if item.LifeAssumed {
			ev.LifeAssumedNames = append(ev.LifeAssumedNames, r.Name)
		}
		if item.ServiceAssumed {
			ev.ServiceAssumedNames = append(ev.ServiceAssumedNames, r.Name)
		}
		sid := r.ID
		if primarySID == nil {
			primarySID = &sid
		}
		items = append(items, item)
		proj := (*float64)(nil)
		if source == PriceSourceOverride {
			proj = fptr(list)
		}
		th, _ := throughput(kind, r, m)
		peak, unit := peakOps(in.ObjectType, kind, m, volume)
		if p, ok := processPeak(in, m, kind, codes, volume); ok && len(codes) > 0 {
			peak = p
		}
		out.Fleet = append(out.Fleet, FleetKPI{
			SolutionID:          r.ID,
			Name:                r.Name,
			Quantity:            f.Quantity,
			CatalogPriceRub:     fptr(catalog),
			ProjectPriceRub:     proj,
			CashPriceRub:        item.CashRub,
			LifetimeYears:       item.Life,
			LifetimeAssumed:     item.LifeAssumed,
			PriceSource:         source,
			PriceOverrideReason: f.PriceOverrideReason,
			WorkKind:            kind,
			TaskCodes:           codes,
			PeakOps:             round4(peak),
			Throughput:          round4(th),
			Unit:                unit,
		})
	}

	if len(items) == 0 {
		ev.Result, ev.Notes = out, notes
		return ev
	}

	remainFot, cashSaved := variantLabor(in, covered, m, laborFactor, laborShare)
	nm := env.nm
	cm := buildCosts(costInput{
		Items: items, HoursYear: env.hoursYear, IntegFrac: env.integFrac, SharedKey: v.ID, Shared: in.SharedCosts, Assumptions: in.Assumptions,
		Norms: nm, Shifts: env.shifts, PayrollTax: env.payrollTax, LaborFactor: laborFactor,
	})
	out.SharedCosts = cm.SharedLines
	out.Chargers = cm.Chargers
	discount := in.Assumptions.discountRate()
	buyOpex := cm.buyOpex(remainFot)
	annualOps := variantAnnualOps(in, covered, m, volume, workDays)

	fins := v.Financing
	if len(fins) == 0 {
		fins = []FinancingSpec{{Kind: "buy"}, {Kind: "raas", Tariff: "fixed"}}
	}

	buyIn := scenarioIn{
		Kind: "buy", SolutionID: primarySID, Fleet: cm.Fleet, Capex: cm.BuyCapex, Opex: buyOpex, Baseline: baselineOpex,
		LaborCashSaved: cashSaved, Horizon: horizon, Discount: discount, Purchases: cm.purchases(), Depreciation: cm.Depreciation,
		BatteryFrac: nm.BatteryFrac, BatteryYears: nm.BatteryYears,
	}
	_, buyFlow := scenarioFrom(buyIn)
	key := v.ID + ":buy"
	lines := []Line{
		{Scenario: key, Bucket: "capex", ID: "equipment", Label: "Оборудование смешанного флота", Rub: cm.Equipment, Note: vatNote(in.Assumptions)},
	}
	lines = append(lines, cm.delivery(key, nm))
	lines = append(lines, cm.SharedLines...)
	lines = append(lines,
		Line{Scenario: key, Bucket: "capex", ID: "contingency", Label: "Резерв " + rutext.Pct(nm.ContingencyFrac*100, 1) + " суммы статей", Rub: cm.BuyCont},
		Line{Scenario: key, Bucket: "opex", ID: "fot_remaining", Label: "ФОТ с учётом денежной доли экономии труда", Rub: remainFot},
		Line{Scenario: key, Bucket: "opex", ID: "service", Label: "Сервис и ремонт", Rub: cm.Service},
		Line{Scenario: key, Bucket: "opex", ID: "licenses", Label: "Лицензии", Rub: cm.Licenses},
	)
	lines = append(lines, cm.operationLines(key, nm)...)
	lines = append(lines, replacementLines(key, buyFlow, nm)...)

	priceSource := PriceSourceCatalog
	for _, it := range out.Fleet {
		if it.PriceSource == PriceSourceOverride {
			priceSource = PriceSourceOverride
			break
		}
	}

	for _, f := range fins {
		kind := f.Kind
		if kind != "buy" && kind != "raas" {
			continue
		}
		tariff := f.Tariff
		si := buyIn
		if kind == "raas" {
			if tariff == "" {
				tariff = "fixed"
			}
			rp := parseRaas(f.Assumptions, nm)
			q := raasFee(tariff, cm.Equipment, annualOps, volume, rp)
			if q.Skipped {
				notes = append(notes, "RaaS ("+tariffLabel(tariff)+") в варианте \""+v.Name+"\" не посчитан: "+q.Note)
				continue
			}
			notes = append(notes, "RaaS ("+tariffLabel(tariff)+"): "+q.Note)
			si.Kind, si.Capex, si.Opex, si.Purchases, si.Depreciation = "raas", cm.RaasCapex, cm.raasOpex(remainFot, q.Fee), nil, cm.RaasDepreciation
			lines = append(lines, Line{
				Scenario: v.ID + ":raas:" + tariff, Bucket: "opex", ID: "raas_fee",
				Label: "Плата RaaS (" + tariffLabel(tariff) + ")", Rub: q.Fee, Note: q.Note,
			})
		}
		sc, _ := scenarioFrom(si)
		sc.VariantID = v.ID
		sc.VariantName = v.Name
		sc.Tariff = tariff
		sc.PriceSource = priceSource
		out.Scenarios = append(out.Scenarios, sc)
	}
	ev.Result, ev.Lines, ev.Notes = out, lines, notes
	return ev
}

func variantLabor(in Input, covered map[string]bool, m map[string]any, laborFactor, laborShare float64) (remainFot, cashSaved float64) {
	if len(in.Processes) == 0 {
		kind := defaultWork(in.ObjectType)
		if in.Robot != nil {
			kind = classifyWork(in.ObjectType, *in.Robot)
		}
		lab := laborFor(in.ObjectType, kind, m, laborFactor)
		_, remainFot, cashSaved, _ = remainLabor(lab.Headcount, lab.AnnualFot, laborShare)
		return remainFot, cashSaved
	}
	tax := taxFactor(m)
	if laborFactor <= 0 {
		laborFactor = 1
	}
	anyCover := len(covered) > 0
	for _, p := range in.Processes {
		if !p.IsBaseline {
			continue
		}
		wage := wageForRole(in.ObjectType, p.StaffRole, m)
		fot := yearFot(p.StaffHeadcount, wage, tax, laborFactor)
		hit := processCovered(p, keys(covered))
		if !anyCover || hit {
			_, rf, cs, _ := remainLabor(p.StaffHeadcount, fot, laborShare)
			remainFot += rf
			cashSaved += cs
			continue
		}
		remainFot += fot
	}
	return roundRub(remainFot), roundRub(cashSaved)
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k, ok := range m {
		if ok {
			out = append(out, k)
		}
	}
	return out
}

func variantAnnualOps(in Input, covered map[string]bool, m map[string]any, volume, workDays float64) float64 {
	if volume <= 0 {
		volume = 1
	}
	if workDays <= 0 {
		workDays = 365
	}
	if len(in.Processes) == 0 {
		return 0
	}
	sum := 0.0
	anyCover := len(covered) > 0
	for _, p := range in.Processes {
		if !p.IsBaseline {
			continue
		}
		if anyCover && !processCovered(p, keys(covered)) {
			continue
		}
		sum += p.UnitsPerDay * workDays * volume
	}
	return sum
}

func parseRaas(raw json.RawMessage, n Norms) raasParams {
	p := raasParams{MonthlyFrac: n.RaasMonthlyFrac, MixFixedShare: n.RaasMixFixedShare}
	if len(raw) == 0 {
		return p
	}
	_ = json.Unmarshal(raw, &p)
	if p.MonthlyFrac <= 0 {
		p.MonthlyFrac = n.RaasMonthlyFrac
	}
	if p.MixFixedShare < 0 || p.MixFixedShare > 1 {
		p.MixFixedShare = n.RaasMixFixedShare
	}
	return p
}

// raasQuote is a RaaS payment with the rate behind it, so the page can print what was actually used.
type raasQuote struct {
	Fee     float64
	PerOp   float64
	Implied bool
	Skipped bool
	Note    string
}

// raasFee prices one year of a RaaS contract. The variable part needs a rate per operation: the one in the
// tariff assumptions, or else the rate that makes the fixed payment equal at the planned volume (annualOps at volume
// factor volume), so a variable tariff reacts to the volume factor and nothing else.
func raasFee(tariff string, equipment, annualOps, volume float64, p raasParams) raasQuote {
	fixed := roundRub(p.MonthlyFrac * 12 * equipment)
	fixedNote := rutext.Pct(p.MonthlyFrac*100, 1) + " цены в месяц"
	if tariff != "variable" && tariff != "mixed" {
		return raasQuote{Fee: fixed, Note: fixedNote}
	}
	if annualOps <= 0 {
		return raasQuote{Skipped: true, Note: "у проекта нет процессов с объёмом, ставку за операцию не от чего считать."}
	}
	if volume <= 0 {
		volume = 1
	}
	perOp, implied := p.PerOpRub, false
	if perOp <= 0 {
		perOp, implied = fixed/(annualOps/volume), true
	}
	variable := roundRub(perOp * annualOps)
	rate := rutext.Num(perOp, 2) + " ₽ за операцию"
	if implied {
		rate += " (выведена из фиксированной платы при плановом объёме)"
	}
	if tariff == "variable" {
		return raasQuote{Fee: variable, PerOp: perOp, Implied: implied, Note: rate}
	}
	note := "доля фиксированной части " + rutext.Pct(p.MixFixedShare*100, 1) + " (" + fixedNote + "), остальное по ставке " + rate
	return raasQuote{Fee: roundRub(p.MixFixedShare*fixed + (1-p.MixFixedShare)*variable), PerOp: perOp, Implied: implied, Note: note}
}

func tariffLabel(t string) string {
	switch t {
	case "variable":
		return "переменный"
	case "mixed":
		return "смешанный"
	default:
		return "фиксированный"
	}
}

func appendNew(list []string, names ...string) []string {
	for _, n := range names {
		if !slices.Contains(list, n) {
			list = append(list, n)
		}
	}
	return list
}
