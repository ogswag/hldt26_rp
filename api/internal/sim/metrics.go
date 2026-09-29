package sim

import "math"

type ProcessMetrics struct {
	Code           string  `json:"code"`
	Arrived        int     `json:"arrived"`
	Completed      int     `json:"completed"`
	Violated       int     `json:"violated"`
	Open           int     `json:"open"`
	ViolationRate  float64 `json:"violation_rate"`
	ThroughputPerH float64 `json:"throughput_per_h"`
	WaitMeanS      float64 `json:"wait_mean_s"`
	WaitP95S       float64 `json:"wait_p95_s"`
	CycleMeanS     float64 `json:"cycle_mean_s"`
	CycleP95S      float64 `json:"cycle_p95_s"`
	QueueMean      float64 `json:"queue_mean"`
	QueueMax       int     `json:"queue_max"`
	WaitTotalS     float64 `json:"wait_total_s"`
}

type ResourceMetrics struct {
	ID          string  `json:"id"`
	Utilization float64 `json:"utilization"`
	QueueMean   float64 `json:"queue_mean"`
	QueueMax    int     `json:"queue_max"`
	Acquired    int     `json:"acquired"`
	WaitMeanS   float64 `json:"wait_mean_s"`
	WaitMaxS    float64 `json:"wait_max_s"`
	WaitTotalS  float64 `json:"wait_total_s"`
}

type RobotMetrics struct {
	ID            string             `json:"id"`
	Jobs          int                `json:"jobs"`
	DistanceKM    float64            `json:"distance_km"`
	Utilization   float64            `json:"utilization"`
	ChargingShare float64            `json:"charging_share"`
	IdleShare     float64            `json:"idle_share"`
	Battery       float64            `json:"battery_end"`
	Depleted      int                `json:"depleted"`
	States        map[string]float64 `json:"states_s"`
}

// Metrics are the results of one replication.
type Metrics struct {
	Replication      int               `json:"replication"`
	Seed             int64             `json:"seed"`
	Arrived          int               `json:"arrived"`
	Completed        int               `json:"completed"`
	Violated         int               `json:"violated"`
	Open             int               `json:"open"`
	ViolationRate    float64           `json:"violation_rate"`
	ThroughputPerH   float64           `json:"throughput_per_h"`
	WaitMeanS        float64           `json:"wait_mean_s"`
	WaitP95S         float64           `json:"wait_p95_s"`
	CycleMeanS       float64           `json:"cycle_mean_s"`
	CycleP95S        float64           `json:"cycle_p95_s"`
	QueueMean        float64           `json:"queue_mean"`
	QueueMax         int               `json:"queue_max"`
	FleetUtilization float64           `json:"fleet_utilization"`
	ChargingShare    float64           `json:"charging_share"`
	IdleShare        float64           `json:"idle_share"`
	DistanceKM       float64           `json:"distance_km"`
	BatteryDepleted  int               `json:"battery_depleted"`
	DispatchWaitS    float64           `json:"dispatch_wait_s"`
	Events           int               `json:"events"`
	Processes        []ProcessMetrics  `json:"processes"`
	Resources        []ResourceMetrics `json:"resources"`
	Robots           []RobotMetrics    `json:"robots"`
}

func (r *runner) finish() Metrics {
	m := r.m
	hz := r.horizon
	hours := hz / 3600
	for _, rb := range r.robots {
		r.setState(rb, rb.state)
	}
	for pi := range r.procs {
		r.touchQueues(pi)
	}
	for _, rs := range r.res {
		r.touchRes(rs)
		for _, w := range rs.queue {
			wait := hz - w.since
			rs.waitTotal += wait
			rs.waitMax = math.Max(rs.waitMax, wait)
		}
	}
	out := Metrics{Events: r.steps - r.ckpts}
	var allWait, allCycle []float64
	for pi, pm := range m.procs {
		if !pm.covered {
			continue
		}
		pmx := ProcessMetrics{Code: pm.spec.Code}
		var waits, cycles []float64
		openViolated := 0
		for _, j := range r.jobs {
			if j.proc != pi {
				continue
			}
			pmx.Arrived++
			w := hz - j.tArr
			if j.dispatched {
				w = j.tDisp - j.tArr
			}
			waits = append(waits, w)
			pmx.WaitTotalS += w
			if j.done {
				pmx.Completed++
				cycles = append(cycles, j.tDone-j.tArr)
			} else {
				pmx.Open++
				if j.violated {
					openViolated++
				}
			}
			if j.violated {
				pmx.Violated++
			}
		}
		pmx.ViolationRate = ratio(float64(pmx.Violated), float64(pmx.Completed+openViolated))
		pmx.ThroughputPerH = round3(float64(pmx.Completed) / hours)
		pmx.WaitMeanS, pmx.WaitP95S = meanP95(waits)
		pmx.CycleMeanS, pmx.CycleP95S = meanP95(cycles)
		pmx.QueueMean = round3(r.procs[pi].queueArea / hz)
		pmx.QueueMax = r.procs[pi].queueMax
		pmx.WaitTotalS = f1v(pmx.WaitTotalS)
		out.Arrived += pmx.Arrived
		out.Completed += pmx.Completed
		out.Violated += pmx.Violated
		out.Open += pmx.Open
		out.DispatchWaitS += pmx.WaitTotalS
		allWait = append(allWait, waits...)
		allCycle = append(allCycle, cycles...)
		out.Processes = append(out.Processes, pmx)
	}
	openViolated := 0
	for _, j := range r.jobs {
		if !j.done && j.violated && m.procs[j.proc].covered {
			openViolated++
		}
	}
	out.ViolationRate = ratio(float64(out.Violated), float64(out.Completed+openViolated))
	out.ThroughputPerH = round3(float64(out.Completed) / hours)
	out.WaitMeanS, out.WaitP95S = meanP95(allWait)
	out.CycleMeanS, out.CycleP95S = meanP95(allCycle)
	out.QueueMean = round3(r.queueArea / hz)
	out.QueueMax = r.queueMax
	out.DispatchWaitS = f1v(out.DispatchWaitS)

	var busy, charging, idle, total, dist float64
	for _, rb := range r.robots {
		rm := RobotMetrics{ID: rb.spec.id, Jobs: rb.jobs, DistanceKM: round3(rb.distM / 1000), Battery: round3(rb.battery), Depleted: rb.depleted,
			States: map[string]float64{}}
		t := 0.0
		for s, v := range rb.timeIn {
			t += v
			if v > 0 {
				rm.States[stateNames[s]] = f1v(v)
			}
		}
		b := rb.timeIn[stTravelEmpty] + rb.timeIn[stTravelLoaded] + rb.timeIn[stHandling] + rb.timeIn[stWaitResource]
		c := rb.timeIn[stToCharger] + rb.timeIn[stWaitCharger] + rb.timeIn[stCharging]
		rm.Utilization = ratio(b, t)
		rm.ChargingShare = ratio(c, t)
		rm.IdleShare = ratio(rb.timeIn[stIdle], t)
		busy += b
		charging += c
		idle += rb.timeIn[stIdle]
		total += t
		dist += rb.distM
		out.BatteryDepleted += rb.depleted
		out.Robots = append(out.Robots, rm)
	}
	out.FleetUtilization = ratio(busy, total)
	out.ChargingShare = ratio(charging, total)
	out.IdleShare = ratio(idle, total)
	out.DistanceKM = round3(dist / 1000)
	for _, rs := range r.res {
		capacity := float64(rs.model.capacity)
		out.Resources = append(out.Resources, ResourceMetrics{
			ID:          rs.model.id,
			Utilization: round3(rs.useArea / (hz * capacity)),
			QueueMean:   round3(rs.queueArea / hz),
			QueueMax:    rs.queueMax,
			Acquired:    rs.acquired,
			WaitMeanS:   f1v(rs.waitTotal / math.Max(1, float64(rs.acquired+len(rs.queue)))),
			WaitMaxS:    f1v(rs.waitMax),
			WaitTotalS:  f1v(rs.waitTotal),
		})
	}
	return out
}

func ratio(a, b float64) float64 {
	if b <= 0 {
		return 0
	}
	return round3(a / b)
}

func meanP95(v []float64) (float64, float64) {
	if len(v) == 0 {
		return 0, 0
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	sorted := sortedFloats(v)
	return f1v(s / float64(len(v))), f1v(quantile(sorted, 0.95))
}

func quantile(sorted []float64, q float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	pos := q * float64(n-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if hi >= n {
		hi = n - 1
	}
	frac := pos - float64(lo)
	return sorted[lo] + (sorted[hi]-sorted[lo])*frac
}

// Stat summarizes one metric across replications.
type Stat struct {
	Median float64 `json:"median"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	P10    float64 `json:"p10"`
	P90    float64 `json:"p90"`
}

func statOf(v []float64) Stat {
	if len(v) == 0 {
		return Stat{}
	}
	s := sortedFloats(v)
	r := func(x float64) float64 { return math.Round(x*1000) / 1000 }
	return Stat{
		Median: r(quantile(s, 0.5)),
		Min:    r(s[0]),
		Max:    r(s[len(s)-1]),
		P10:    r(quantile(s, 0.1)),
		P90:    r(quantile(s, 0.9)),
	}
}

func collect[T any](items []T, f func(T) float64) Stat {
	v := make([]float64, len(items))
	for i, it := range items {
		v[i] = f(it)
	}
	return statOf(v)
}

// medianIndex returns the replication whose violation count and throughput are closest to the medians.
func medianIndex(reps []Metrics) int {
	if len(reps) == 0 {
		return 0
	}
	viol := collect(reps, func(m Metrics) float64 { return float64(m.Violated) }).Median
	th := collect(reps, func(m Metrics) float64 { return m.ThroughputPerH }).Median
	best := 0
	bestScore := math.Inf(1)
	for i, m := range reps {
		score := math.Abs(float64(m.Violated)-viol)/math.Max(1, viol) + math.Abs(m.ThroughputPerH-th)/math.Max(1, th)
		if score < bestScore-1e-12 {
			best = i
			bestScore = score
		}
	}
	return best
}
