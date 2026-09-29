package econ

import (
	"strings"
	"testing"

	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/objects"
)

func priced(id, name string, price float64) Robot {
	r := h1500()
	r.ID, r.Name = id, name
	if price > 0 {
		r.PriceRub = &price
	} else {
		r.PriceRub = nil
	}
	return r
}

func item(r Robot, status string, score float64) matching.Item {
	return matching.Item{
		SolutionID: r.ID, Name: r.Name, PriceRub: r.PriceRub, Status: status, Score: score,
		Reasons: []string{"Причина " + r.Name + "."},
		Hard:    []matching.Step{{Outcome: matching.OutcomePass, Text: "Проезд подходит."}},
	}
}

func rank(t *testing.T, robots []Robot, items []matching.Item) matching.Output {
	t.Helper()
	return Rank(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Match:      matching.Output{Items: items},
	}, robots)
}

func order(out matching.Output) string {
	names := make([]string, len(out.Items))
	for i, it := range out.Items {
		names[i] = it.Name
	}
	return strings.Join(names, ", ")
}

func TestRankOrdersByVerdictThenPayback(t *testing.T) {
	h := priced("h", "Ronavi H1500", 2_700_000)
	dear := priced("d", "Дорогой", 5_400_000)
	cheap := priced("c", "Дешёвый", 1_350_000)
	noPrice := priced("n", "Без цены", 0)
	review := priced("r", "Проверить", 1_000_000)
	out := rank(t, []Robot{h, dear, cheap, noPrice, review}, []matching.Item{
		item(dear, matching.StatusRecommended, 0.9),
		item(noPrice, matching.StatusRecommended, 0.85),
		item(h, matching.StatusRecommended, 0.8),
		item(cheap, matching.StatusRecommended, 0.7),
		item(review, matching.StatusNeedsReview, 0.95),
		item(priced("x", "Исключён", 1_000_000), matching.StatusExcluded, 0.99),
	})
	if got, want := order(out), "Дешёвый, Ronavi H1500, Дорогой, Без цены, Проверить, Исключён"; got != want {
		t.Fatalf("order %q, want %q", got, want)
	}
	if out.Best != "c" {
		t.Fatalf("best %q", out.Best)
	}
	// The estimate of one robot is the buy scenario it gets alone (TestWarehouseH1500Golden).
	est := out.Items[1].Estimate
	if est == nil || est.FleetSize != 13 || est.CapexRub != 56_370_600 || est.PaybackYears == nil || *est.PaybackYears != 3.4941 {
		t.Fatalf("H1500 estimate %+v", est)
	}
	if out.Items[3].Estimate != nil || out.Items[5].Estimate != nil {
		t.Fatal("a robot without price or an excluded one got an estimate")
	}
	last := out.BestWhy[len(out.BestWhy)-1]
	if !strings.HasPrefix(last, "Окупается за 1,5 года, быстрее остальных подходящих роботов: следующий Ronavi H1500, 3,5 года.") {
		t.Fatalf("why %q", last)
	}
	if out.BestWhy[0] != "Проезд подходит." {
		t.Fatalf("checks %q", out.BestWhy)
	}
}

func TestRankWhy(t *testing.T) {
	h := priced("h", "Ronavi H1500", 2_700_000)
	noPrice := priced("n", "Без цены", 0)
	cases := []struct {
		name  string
		items []matching.Item
		first string
		last  string
	}{
		{
			name:  "alone",
			items: []matching.Item{item(h, matching.StatusRecommended, 0.8)},
			first: "Проезд подходит.",
			last:  "Окупается за 3,5 года. Других подходящих роботов нет.",
		},
		{
			name:  "the others have no price",
			items: []matching.Item{item(noPrice, matching.StatusRecommended, 0.9), item(h, matching.StatusRecommended, 0.8)},
			first: "Проезд подходит.",
			last:  "Окупается за 3,5 года. Остальные подходящие роботы не окупаются или не имеют цены.",
		},
		{
			name:  "no price at all",
			items: []matching.Item{item(noPrice, matching.StatusRecommended, 0.9)},
			first: "Проезд подходит.",
			last:  "Нет цены в каталоге, поэтому окупаемость не посчитана: робот выбран по соответствию объекту.",
		},
		{
			name:  "nothing passed every check",
			items: []matching.Item{item(h, matching.StatusNeedsReview, 0.8), item(noPrice, matching.StatusExcluded, 0.9)},
			first: "Ни один робот не прошёл все проверки, этот лучший из требующих проверки. Что уточнить:",
			last:  "Окупается за 3,5 года. Других требующих проверки роботов нет.",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := rank(t, []Robot{h, noPrice}, c.items)
			if len(out.BestWhy) < 2 || out.BestWhy[0] != c.first || out.BestWhy[len(out.BestWhy)-1] != c.last {
				t.Fatalf("why %q", out.BestWhy)
			}
		})
	}
}

func TestRankHasNoBestWhenEverythingIsExcluded(t *testing.T) {
	h := priced("h", "Ronavi H1500", 2_700_000)
	out := rank(t, []Robot{h}, []matching.Item{item(h, matching.StatusExcluded, 0.8)})
	if out.Best != "" || out.BestWhy != nil || out.Items[0].Estimate != nil {
		t.Fatalf("best %q why %q estimate %+v", out.Best, out.BestWhy, out.Items[0].Estimate)
	}
}

func failTasks(it matching.Item, tasks ...string) matching.Item {
	for _, task := range tasks {
		it.Explanation = append(it.Explanation, matching.Step{RuleID: "task_capability", TaskCode: task, Outcome: matching.OutcomeFail})
	}
	return it
}

// A robot saves only the staff of the processes whose tasks it can do, the way a variant with it is counted.
func TestRankCountsOnlyCoveredProcesses(t *testing.T) {
	pallet := priced("p", "Паллетный", 2_700_000)
	cleaner := priced("c", "Уборщик", 300_000)
	processes := warehouseProcessSpecs()
	out := Rank(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Processes:  processes,
		Match: matching.Output{Items: []matching.Item{
			failTasks(item(cleaner, matching.StatusNeedsReview, 0.9), "pallet_inbound", "pallet_putaway", "piece_pick", "pallet_outbound"),
			failTasks(item(pallet, matching.StatusNeedsReview, 0.8), "piece_pick"),
		}},
	}, []Robot{pallet, cleaner})
	if got := order(out); got != "Паллетный, Уборщик" {
		t.Fatalf("order %q", got)
	}
	if e := out.Items[1].Estimate; e == nil || e.PaybackYears != nil || e.CapexRub <= 0 {
		t.Fatalf("a robot that does none of the tasks must not pay back: %+v", e)
	}
	p := out.Items[0].Estimate
	if p == nil || p.PaybackYears == nil || strings.Join(p.ProcessCodes, " ") != "inbound putaway outbound" {
		t.Fatalf("pallet estimate %+v", p)
	}
	// The same robot as a variant serving the pallet processes gives the same case.
	res, err := Calculate(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Processes:  processes,
		Robots:     []Robot{pallet},
		Variants: []VariantSpec{{ID: "v", Name: "Паллетный", Financing: []FinancingSpec{{Kind: "buy"}},
			Fleet: []FleetSpec{{SolutionID: "p", Quantity: p.FleetSize, TaskCodes: p.ProcessCodes}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	buy := res.Variants[0].Scenarios[0]
	if *buy.CapexRub != p.CapexRub || *buy.PaybackYears != *p.PaybackYears {
		t.Fatalf("variant capex %v payback %v, estimate %+v", *buy.CapexRub, *buy.PaybackYears, p)
	}
}

func TestRankWithoutProcessesDropsRobotsThatDoNoTask(t *testing.T) {
	pallet := priced("p", "Паллетный", 2_700_000)
	cheap := priced("c", "Дешёвый", 300_000)
	passed := item(pallet, matching.StatusNeedsReview, 0.8)
	passed.Explanation = []matching.Step{{RuleID: "task_capability", TaskCode: "pallet_inbound", Outcome: matching.OutcomePass}}
	out := rank(t, []Robot{pallet, cheap}, []matching.Item{failTasks(item(cheap, matching.StatusNeedsReview, 0.9), "pallet_inbound"), passed})
	if got := order(out); got != "Паллетный, Дешёвый" {
		t.Fatalf("order %q", got)
	}
	if e := out.Items[1].Estimate; e == nil || e.PaybackYears != nil {
		t.Fatalf("a robot that does no task of the site must not pay back: %+v", e)
	}
}

// Where the simulation runs, a robot covers only the processes it can be simulated on: a pallet AMR takes
// приёмка, a stacker takes every pallet process.
func TestRankFollowsSimulationTasks(t *testing.T) {
	amr := priced("a", "Паллетный AMR", 2_700_000)
	amr.SimTasks = []string{"piece_pick", "pallet_inbound", "pallet_move"}
	stacker := priced("s", "Штабелёр", 4_800_000)
	stacker.SimTasks = []string{"pallet_inbound", "pallet_putaway", "pallet_outbound", "pallet_move"}
	cleaner := priced("c", "Уборщик", 300_000)
	cleaner.SimTasks = []string{}
	out := Rank(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Processes:  warehouseProcessSpecs(),
		Match: matching.Output{Items: []matching.Item{
			item(amr, matching.StatusNeedsReview, 0.9),
			item(stacker, matching.StatusNeedsReview, 0.8),
			item(cleaner, matching.StatusNeedsReview, 0.7),
		}},
	}, []Robot{amr, stacker, cleaner})
	codes := map[string]string{}
	for _, it := range out.Items {
		if it.Estimate != nil {
			codes[it.Name] = strings.Join(it.Estimate.ProcessCodes, " ")
		}
	}
	if codes["Паллетный AMR"] != "inbound" || codes["Штабелёр"] != "inbound putaway outbound" || codes["Уборщик"] != "" {
		t.Fatalf("covered processes %q", codes)
	}
	if out.Items[len(out.Items)-1].Name != "Уборщик" {
		t.Fatalf("a robot the simulation cannot run must not lead: %q", order(out))
	}
}

// A robot's fleet is sized for the demand of the processes it takes over: 1 000 pallets a day of приёмка need
// 7 robots, 3 000 a day of приёмка, размещение and отгрузка need 19 (the site parameters alone give 13 for 2 000).
func TestRankSizesFleetByCoveredProcesses(t *testing.T) {
	amr := priced("a", "Паллетный AMR", 2_700_000)
	amr.SimTasks = []string{"piece_pick", "pallet_inbound", "pallet_move"}
	stacker := priced("s", "Штабелёр", 2_700_000)
	stacker.SimTasks = []string{"pallet_inbound", "pallet_putaway", "pallet_outbound", "pallet_move"}
	out := Rank(Input{
		ObjectType: objects.Warehouse,
		Params:     warehouseParams(t),
		Processes:  warehouseProcessSpecs(),
		Match:      matching.Output{Items: []matching.Item{item(amr, matching.StatusNeedsReview, 0.9), item(stacker, matching.StatusNeedsReview, 0.8)}},
	}, []Robot{amr, stacker})
	got := map[string]matching.Estimate{}
	for _, it := range out.Items {
		got[it.Name] = *it.Estimate
	}
	if e := got["Паллетный AMR"]; e.FleetSize != 7 || e.CapexRub != 30_353_400 {
		t.Fatalf("AMR for приёмка %+v", e)
	}
	if e := got["Штабелёр"]; e.FleetSize != 19 || e.CapexRub != 82_387_800 {
		t.Fatalf("stacker for three processes %+v", e)
	}
}
