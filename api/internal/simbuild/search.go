package simbuild

import (
	"context"
	"fmt"
	"math"
	"time"

	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/sim"
)

const (
	searchCoverage = 0.85
	searchReps     = 5
	searchCap      = 200
)

var errSearchTimeout = fmt.Errorf("search timeout")

// SearchFleet finds the smallest quantity of one fleet line that covers its processes. It tries sizes without a
// journal, then runs the best size with a journal.
func SearchFleet(ctx context.Context, robots Robots, objectType string, d projects.Draft, cfg JobConfig, progress sim.Progress, deadline time.Time) (*Built, sim.Result, []sim.Metrics, *sim.Log, FleetSearch, error) {
	if cfg.LineKey == "" {
		return nil, sim.Result{}, nil, nil, FleetSearch{}, &sim.InputError{Msg: "Не указана позиция флота для подбора."}
	}
	v, err := jobVariant(d, cfg)
	if err != nil {
		return nil, sim.Result{}, nil, nil, FleetSearch{}, err
	}
	q0, ok := lineQuantity(v, cfg.LineKey)
	if !ok {
		return nil, sim.Result{}, nil, nil, FleetSearch{}, &sim.InputError{Msg: "В варианте нет такой позиции флота."}
	}
	limit := searchLimit(q0)
	probed := 0
	total := searchStepCap(q0)
	probe := func(q int) (FleetSearchRow, bool, error) {
		if err := ctx.Err(); err != nil {
			return FleetSearchRow{}, false, err
		}
		jc := cfg
		jc.Mode = ""
		jc.FleetQuantities = map[string]int{cfg.LineKey: q}
		jc.Sim.Replications = searchReps
		built, err := Build(robots, objectType, d, jc)
		if err != nil {
			return FleetSearchRow{}, false, err
		}
		res, _, err := built.Model.RunN(ctx, searchReps, nil)
		if err != nil {
			return FleetSearchRow{}, false, err
		}
		row, pass := lineScore(res, cfg.LineKey)
		row.Quantity = q
		probed++
		if progress != nil {
			_ = progress(float64(probed) / float64(total) * float64(searchReps))
		}
		return row, pass, nil
	}
	best, stopped, rows, err := searchSizes(q0, limit, deadline, time.Now, probe)
	if err != nil && err != errSearchTimeout {
		return nil, sim.Result{}, nil, nil, FleetSearch{LineKey: cfg.LineKey, Rows: rows, Best: best, Stopped: stopped}, err
	}
	runQ, out := closingRun(cfg.LineKey, q0, best, rows, stopped)
	jc := cfg
	jc.Mode = ""
	jc.FleetQuantities = map[string]int{cfg.LineKey: runQ}
	built, err := Build(robots, objectType, d, jc)
	if err != nil {
		return nil, sim.Result{}, nil, nil, out, err
	}
	res, reps, journal, err := built.Model.RunAll(ctx, progress)
	if err != nil {
		return nil, sim.Result{}, nil, nil, out, err
	}
	return built, res, reps, journal, out, nil
}

// closingRun picks the size of the run that ends a search and builds the table the page shows. When no size passed,
// the run takes the last size tried so its bottleneck shows, and Best stays 0: the page offers no size.
func closingRun(lineKey string, q0, best int, rows []FleetSearchRow, stopped string) (int, FleetSearch) {
	out := FleetSearch{LineKey: lineKey, Rows: rows, Best: best, Stopped: stopped}
	if best >= 1 {
		return best, out
	}
	size := q0
	if len(rows) > 0 {
		size = rows[len(rows)-1].Quantity
	}
	if out.Stopped == "" {
		out.Stopped = "ни один размер не прошёл"
	}
	return size, out
}

// LineQuantity is the current count of the fleet line named by key.
func LineQuantity(v projects.Variant, key string) (int, bool) {
	return lineQuantity(v, key)
}

func lineQuantity(v projects.Variant, key string) (int, bool) {
	for i, f := range v.Fleet {
		if fleetKey(f, i) == key && f.Quantity > 0 {
			return f.Quantity, true
		}
	}
	return 0, false
}

func searchLimit(q0 int) int {
	if q0 < 1 {
		q0 = 1
	}
	n := q0 * 2
	if n < q0+20 {
		n = q0 + 20
	}
	if n > searchCap {
		n = searchCap
	}
	return n
}

// SearchStepCap is the upper bound of sizes a search of q0 will try, for the job progress bar.
func SearchStepCap(q0 int) int {
	return searchStepCap(q0)
}

func searchStepCap(q0 int) int {
	limit := searchLimit(q0)
	n := 2
	for step := 1; q0+step < limit; step *= 2 {
		n++
	}
	n += 12
	return n
}

func searchSizes(q0, limit int, deadline time.Time, now func() time.Time, probe func(int) (FleetSearchRow, bool, error)) (best int, stopped string, rows []FleetSearchRow, err error) {
	if q0 < 1 {
		q0 = 1
	}
	seen := map[int]bool{}
	run := func(q int) (bool, error) {
		if q < 1 {
			q = 1
		}
		if q > limit {
			q = limit
		}
		if seen[q] {
			for _, row := range rows {
				if row.Quantity == q {
					return row.Coverage+1e-12 >= searchCoverage && row.SLAOK, nil
				}
			}
		}
		if !deadline.IsZero() && now().After(deadline) {
			return false, errSearchTimeout
		}
		row, pass, err := probe(q)
		if err != nil {
			return false, err
		}
		seen[q] = true
		rows = append(rows, row)
		if pass && (best == 0 || q < best) {
			best = q
		}
		return pass, nil
	}
	ok0, err := run(q0)
	if err == errSearchTimeout {
		return best, "поиск остановлен по времени", rows, nil
	}
	if err != nil {
		return best, stopped, rows, err
	}
	if !ok0 {
		lo, hi := q0, 0
		for step := 1; ; step *= 2 {
			q := q0 + step
			if q > limit {
				q = limit
			}
			ok, err := run(q)
			if err == errSearchTimeout {
				return best, "поиск остановлен по времени", rows, nil
			}
			if err != nil {
				return best, stopped, rows, err
			}
			if ok {
				hi = q
				break
			}
			lo = q
			if q == limit {
				break
			}
		}
		if hi == 0 {
			return 0, "ни один размер не прошёл", rows, nil
		}
		for lo+1 < hi {
			mid := lo + (hi-lo)/2
			ok, err := run(mid)
			if err == errSearchTimeout {
				return best, "поиск остановлен по времени", rows, nil
			}
			if err != nil {
				return best, stopped, rows, err
			}
			if ok {
				hi = mid
			} else {
				lo = mid
			}
		}
		return hi, "", rows, nil
	}
	lo, hi := 0, q0
	for step := 1; ; step *= 2 {
		q := q0 - step
		if q < 1 {
			q = 1
		}
		ok, err := run(q)
		if err == errSearchTimeout {
			return best, "поиск остановлен по времени", rows, nil
		}
		if err != nil {
			return best, stopped, rows, err
		}
		if !ok {
			lo = q
			break
		}
		hi = q
		if q == 1 {
			return 1, "", rows, nil
		}
	}
	for lo+1 < hi {
		mid := lo + (hi-lo)/2
		ok, err := run(mid)
		if err == errSearchTimeout {
			return best, "поиск остановлен по времени", rows, nil
		}
		if err != nil {
			return best, stopped, rows, err
		}
		if ok {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi, "", rows, nil
}

func lineScore(res sim.Result, lineKey string) (FleetSearchRow, bool) {
	var line *sim.FleetResult
	for i := range res.Fleet {
		if res.Fleet[i].Key == lineKey {
			line = &res.Fleet[i]
			break
		}
	}
	row := FleetSearchRow{SLAOK: true}
	if line == nil {
		return row, false
	}
	served := map[string]bool{}
	for _, c := range line.Processes {
		served[c] = true
	}
	expected, completed := 0.0, 0.0
	for _, p := range res.Processes {
		if !served[p.Code] || p.ExpectedJobs <= 0 {
			continue
		}
		expected += p.ExpectedJobs
		completed += p.Completed.Median
		if p.ViolationRate.P90*100 > res.SLATargetPct+1e-9 {
			row.SLAOK = false
		}
	}
	if expected <= 0 {
		return row, false
	}
	row.Coverage = math.Round(completed/expected*1000) / 1000
	return row, row.Coverage+1e-12 >= searchCoverage && row.SLAOK
}
