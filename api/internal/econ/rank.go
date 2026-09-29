package econ

import (
	"math"
	"slices"
	"sort"

	"moscow_hackathon_2026/api/internal/matching"
)

// Rank gives each robot that is not excluded its own buy case and orders the robots of one verdict by payback:
// robots that pay back come first, fastest first, then the rest in match order. Best is the first robot that is
// not excluded, and BestWhy says why it leads.
//
// With project processes, a robot's case is a variant of that robot alone serving the processes it covers
// (CoveredProcesses), sized for their demand, the way Итог counts it once picked; a robot that covers none saves
// nothing. Without them,
// a robot that can do no task of the site saves nothing either.
func Rank(in Input, robots []Robot) matching.Output {
	out := in.Match
	items := append([]matching.Item(nil), out.Items...)
	byID := make(map[string]Robot, len(robots))
	for _, r := range robots {
		byID[r.ID] = r
	}
	base := in
	base.Variants = nil
	base.SharedCosts = nil
	base.Match = matching.Output{}
	siteTasks := in.Match.TaskCodes
	// NOTE: overrides that describe one robot (price, fleet, CAPEX, OPEX) do not carry over to the others.
	base.Overrides = Overrides{VolumeFactor: in.Overrides.VolumeFactor, LaborFactor: in.Overrides.LaborFactor}
	env, _, err := newVariantEnv(base)
	for i := range items {
		if items[i].Status == matching.StatusExcluded {
			continue
		}
		if r, ok := byID[items[i].SolutionID]; ok && err == nil {
			items[i].Estimate = estimate(base, env, r, items[i], siteTasks)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if ra, rb := verdictRank(a.Status), verdictRank(b.Status); ra != rb {
			return ra < rb
		}
		pa, pb := paybackOf(a), paybackOf(b)
		if (pa != nil) != (pb != nil) {
			return pa != nil
		}
		return pa != nil && *pa < *pb
	})
	out.Items = items
	out.Best, out.BestWhy = "", nil
	for i, it := range items {
		if it.Status != matching.StatusExcluded {
			out.Best = it.SolutionID
			out.BestWhy = bestWhy(items, i)
			break
		}
	}
	return out
}

func estimate(in Input, env variantEnv, r Robot, it matching.Item, siteTasks []string) *matching.Estimate {
	alone := in
	alone.Robot = &r
	res, err := evaluate(alone)
	if err != nil || len(res.Scenarios) < 2 {
		return nil
	}
	buy := res.Scenarios[1]
	if buy.FleetSize == nil || buy.CapexRub == nil {
		return nil
	}
	if len(in.Processes) == 0 {
		// Without processes the savings come from the staff of the robot's kind of work, but only for a robot the
		// match confirmed for at least one task of the site that the simulation can run it on.
		if doesNoTask(it) || !runsAnyTask(in.ObjectType, r, siteTasks) {
			return &matching.Estimate{FleetSize: *buy.FleetSize, CapexRub: *buy.CapexRub}
		}
		return &matching.Estimate{FleetSize: *buy.FleetSize, CapexRub: *buy.CapexRub, PaybackYears: buy.PaybackYears}
	}
	codes := CoveredProcesses(in.ObjectType, in.Processes, r, it)
	if len(codes) == 0 {
		return &matching.Estimate{FleetSize: *buy.FleetSize, CapexRub: *buy.CapexRub}
	}
	fleet := *buy.FleetSize
	kind := classifyWork(in.ObjectType, r)
	if peak, ok := processPeak(in, env.m, kind, codes, env.volume); ok {
		th, _ := throughput(kind, r, env.m)
		if n := fleetSize(peak, th, env.nm); n > 0 {
			fleet = n
		}
	}
	v := VariantSpec{ID: "estimate", Name: r.Name, Fleet: []FleetSpec{{SolutionID: r.ID, Quantity: fleet, TaskCodes: codes}},
		Financing: []FinancingSpec{{Kind: "buy"}}}
	vr := evalVariant(in, v, map[string]Robot{r.ID: r}, env).Result
	if len(vr.Scenarios) == 0 || vr.Scenarios[0].CapexRub == nil {
		return nil
	}
	sc := vr.Scenarios[0]
	return &matching.Estimate{FleetSize: fleet, CapexRub: *sc.CapexRub, PaybackYears: sc.PaybackYears, ProcessCodes: codes}
}

func doesNoTask(it matching.Item) bool {
	checked := false
	for _, st := range it.Explanation {
		if st.RuleID == "task_capability" {
			if st.Outcome == matching.OutcomePass {
				return false
			}
			checked = true
		}
	}
	return checked
}

// runsTask says whether the robot's kind of work includes the task (defaultTaskCodes) and, where there is a
// simulation, whether the simulation runs the robot on it.
func runsTask(kind []string, r Robot, task string) bool {
	return (len(kind) == 0 || slices.Contains(kind, task)) && (r.SimTasks == nil || slices.Contains(r.SimTasks, task))
}

func runsAnyTask(objectType string, r Robot, tasks []string) bool {
	if r.SimTasks == nil {
		return true
	}
	kind := defaultTaskCodes(classifyWork(objectType, r))
	for _, t := range tasks {
		if runsTask(kind, r, t) {
			return true
		}
	}
	return false
}

// CoveredProcesses lists the baseline processes a robot takes over: its kind of work does them, the simulation runs
// it on them (runsTask), and the match confirmed the capability their task needs, where a task needs one.
func CoveredProcesses(objectType string, processes []ProcessSpec, r Robot, it matching.Item) []string {
	unsure := map[string]bool{}
	for _, st := range it.Explanation {
		if st.RuleID == "task_capability" && st.Outcome != matching.OutcomePass {
			unsure[st.TaskCode] = true
		}
	}
	kind := defaultTaskCodes(classifyWork(objectType, r))
	var out []string
	for _, p := range processes {
		if p.IsBaseline && !unsure[p.TaskType] && processCovered(p, kind) && (r.SimTasks == nil || slices.Contains(r.SimTasks, p.TaskType)) {
			out = append(out, p.Code)
		}
	}
	return out
}

func verdictRank(status string) int {
	switch status {
	case matching.StatusRecommended:
		return 0
	case matching.StatusNeedsReview:
		return 1
	default:
		return 2
	}
}

func paybackOf(it matching.Item) *float64 {
	if it.Estimate == nil {
		return nil
	}
	return it.Estimate.PaybackYears
}

// verdictGroup names the robots of one verdict: «подходящих» and «подходящие».
type verdictGroup struct{ gen, nom string }

// bestWhy lists the checks the best robot passed, or what to check when none passed them all, and how its
// payback compares with the robots of its verdict.
func bestWhy(items []matching.Item, best int) []string {
	it := items[best]
	var why []string
	group := verdictGroup{"подходящих", "подходящие"}
	if it.Status == matching.StatusRecommended {
		seen := map[string]bool{}
		for _, st := range append(append([]matching.Step(nil), it.Hard...), it.Soft...) {
			if st.Outcome == matching.OutcomePass && st.Text != "" && !seen[st.Text] {
				seen[st.Text] = true
				why = append(why, st.Text)
			}
		}
	} else {
		group = verdictGroup{"требующих проверки", "требующие проверки"}
		why = append(why, "Ни один робот не прошёл все проверки, этот лучший из требующих проверки. Что уточнить:")
		why = append(why, it.Reasons...)
	}
	return append(why, paybackWhy(items, best, group))
}

func paybackWhy(items []matching.Item, best int, group verdictGroup) string {
	it := items[best]
	pb := paybackOf(it)
	switch {
	case it.Estimate == nil && it.PriceRub == nil:
		return "Нет цены в каталоге, поэтому окупаемость не посчитана: робот выбран по соответствию объекту."
	case it.Estimate == nil:
		return "Окупаемость не посчитана: робот выбран по соответствию объекту."
	case pb == nil:
		return "Ни один из " + group.gen + " роботов не окупается при покупке: расходы с роботами не ниже нынешних. Этот выбран по соответствию объекту."
	}
	others := 0
	for j := best + 1; j < len(items) && items[j].Status == it.Status; j++ {
		others++
		if next := paybackOf(items[j]); next != nil {
			return "Окупается " + paybackPhrase(*pb) + ", быстрее остальных " + group.gen + " роботов: следующий " +
				items[j].Name + ", " + yearsText(*next) + "."
		}
	}
	if others == 0 {
		return "Окупается " + paybackPhrase(*pb) + ". Других " + group.gen + " роботов нет."
	}
	return "Окупается " + paybackPhrase(*pb) + ". Остальные " + group.nom + " роботы не окупаются или не имеют цены."
}

// yearsText prints years to one decimal; a few weeks, which round to zero, read «меньше 0,1 года».
func yearsText(v float64) string {
	if math.Round(v*10) == 0 {
		return "меньше 0,1 года"
	}
	return formatYears(v)
}

func paybackPhrase(v float64) string {
	if math.Round(v*10) == 0 {
		return "быстрее чем за 0,1 года"
	}
	return "за " + formatYears(v)
}
