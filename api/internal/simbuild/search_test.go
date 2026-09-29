package simbuild

import (
	"testing"
	"time"

	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/sim"
)

func simResultForLine(key string, coverage, violationP90 float64) sim.Result {
	return sim.Result{
		SLATargetPct: 5,
		Fleet:        []sim.FleetResult{{Key: key, Processes: []string{"inbound"}}},
		Processes: []sim.ProcessResult{{
			Code: "inbound", ExpectedJobs: 100, Completed: sim.Stat{Median: coverage * 100},
			ViolationRate: sim.Stat{P90: violationP90},
		}},
	}
}

func TestSearchSizesFindsSmallestPassingUp(t *testing.T) {
	passFrom := 13
	probe := func(q int) (FleetSearchRow, bool, error) {
		return FleetSearchRow{Quantity: q, Coverage: 0.9, SLAOK: q >= passFrom}, q >= passFrom, nil
	}
	best, stopped, rows, err := searchSizes(10, searchLimit(10), time.Time{}, time.Now, probe)
	if err != nil || stopped != "" || best != 13 {
		t.Fatalf("best %d stopped %q err %v rows %d", best, stopped, err, len(rows))
	}
}

func TestSearchSizesFindsSmallestPassingDown(t *testing.T) {
	passFrom := 7
	probe := func(q int) (FleetSearchRow, bool, error) {
		return FleetSearchRow{Quantity: q, Coverage: 0.9, SLAOK: q >= passFrom}, q >= passFrom, nil
	}
	best, stopped, _, err := searchSizes(10, searchLimit(10), time.Time{}, time.Now, probe)
	if err != nil || stopped != "" || best != 7 {
		t.Fatalf("best %d stopped %q err %v", best, stopped, err)
	}
}

func TestSearchSizesStopsOnDeadline(t *testing.T) {
	now := time.Now()
	deadline := now.Add(-time.Second)
	n := 0
	probe := func(q int) (FleetSearchRow, bool, error) {
		n++
		return FleetSearchRow{Quantity: q}, false, nil
	}
	_, stopped, _, err := searchSizes(10, searchLimit(10), deadline, func() time.Time { return time.Now() }, probe)
	if err != nil || stopped != "поиск остановлен по времени" {
		t.Fatalf("stopped %q err %v probes %d", stopped, err, n)
	}
}

func TestSearchSizesNonePass(t *testing.T) {
	probe := func(q int) (FleetSearchRow, bool, error) {
		return FleetSearchRow{Quantity: q, Coverage: 0.1, SLAOK: false}, false, nil
	}
	best, stopped, rows, err := searchSizes(3, searchLimit(3), time.Time{}, time.Now, probe)
	if err != nil || best != 0 || stopped != "ни один размер не прошёл" || len(rows) == 0 {
		t.Fatalf("best %d stopped %q err %v rows %d", best, stopped, err, len(rows))
	}
}

func TestLineScoreNeedsCoverageAndSLA(t *testing.T) {
	res := simResultForLine("fleet-1", 0.9, 0.02)
	row, pass := lineScore(res, "fleet-1")
	if !pass || row.Coverage < 0.85 || !row.SLAOK {
		t.Fatalf("row %+v pass %v", row, pass)
	}
	res = simResultForLine("fleet-1", 0.5, 0.02)
	if _, pass := lineScore(res, "fleet-1"); pass {
		t.Fatal("low coverage must fail")
	}
	res = simResultForLine("fleet-1", 0.95, 0.2)
	if _, pass := lineScore(res, "fleet-1"); pass {
		t.Fatal("SLA miss must fail")
	}
}

func TestClosingRunOffersNoSizeWhenNonePassed(t *testing.T) {
	rows := []FleetSearchRow{{Quantity: 10}, {Quantity: 11}, {Quantity: 40}}
	size, out := closingRun("f1", 10, 0, rows, "")
	if size != 40 || out.Best != 0 || out.Stopped != "ни один размер не прошёл" || out.LineKey != "f1" {
		t.Fatalf("size %d, table %+v", size, out)
	}
	size, out = closingRun("f1", 10, 0, rows, "поиск остановлен по времени")
	if size != 40 || out.Best != 0 || out.Stopped != "поиск остановлен по времени" {
		t.Fatalf("stopped by time: size %d, table %+v", size, out)
	}
	if size, out = closingRun("f1", 10, 0, nil, ""); size != 10 || out.Best != 0 {
		t.Fatalf("no rows: size %d, table %+v", size, out)
	}
	if size, out = closingRun("f1", 10, 13, rows, ""); size != 13 || out.Best != 13 || out.Stopped != "" {
		t.Fatalf("passing size: size %d, table %+v", size, out)
	}
}

func TestSearchRunHashesTheSizeItSimulated(t *testing.T) {
	line := func(q int) []projects.Variant {
		return []projects.Variant{{ID: "v1", Name: "AMR", Fleet: []projects.FleetItem{{ID: "f1", SolutionID: ptr(fixtureRobot), Quantity: q}}}}
	}
	d, err := DraftOf(fixtureState(t, func(d *projects.Draft) { d.Variants = line(2) }))
	if err != nil {
		t.Fatal(err)
	}
	taken := d
	taken.Variants = line(4)
	before, _ := VariantHash(d, "v1")
	after, _ := VariantHash(taken, "v1")
	robots := RobotsOf(fixtureCatalog())

	plain, err := Build(robots, "warehouse", d, JobConfig{VariantID: "v1", Sim: quick()})
	if err != nil || plain.VariantHash != before {
		t.Fatalf("a plain run hashes the draft as it is: %v %v", plain, err)
	}
	built, err := Build(robots, "warehouse", d, JobConfig{VariantID: "v1", Sim: quick(), FleetQuantities: map[string]int{"f1": 4}})
	if err != nil {
		t.Fatal(err)
	}
	if built.VariantHash != after || built.VariantHash == before {
		t.Fatalf("a search run at 4 robots must hash the variant with 4 robots: got %s, with 2 %s, with 4 %s", built.VariantHash, before, after)
	}
	if got, _ := VariantHash(d, "v1"); got != before {
		t.Fatal("building a search run changed the draft")
	}

	runs := []RunBrief{{RunID: "search", VariantID: "v1", VariantHash: built.VariantHash, EconCheck: &EconCheck{Coverage: 1, Text: "ok"}}}
	checks, err := Checks(d, runs)
	if err != nil || len(checks) != 1 || !checks[0].Stale {
		t.Fatalf("before the size is taken the run is not the check of the variant: %+v %v", checks, err)
	}
	checks, err = Checks(taken, runs)
	if err != nil || len(checks) != 1 || checks[0].Stale || checks[0].RunID != "search" {
		t.Fatalf("after the size is taken the run is the check of the variant: %+v %v", checks, err)
	}
}
