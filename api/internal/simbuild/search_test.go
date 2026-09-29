package simbuild

import (
	"testing"
	"time"

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
