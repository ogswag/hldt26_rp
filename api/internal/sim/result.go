package sim

import (
	"context"
	"fmt"
	"math"
	"moscow_hackathon_2026/api/internal/rutext"
	"sort"
	"strings"
)

const (
	VerdictPass = "pass"
	VerdictFail = "fail"

	bottleneckMinWaitS   = 60.0
	minFleetShare        = 0.03
	saturatedUtilization = 0.85
	saturatedQueue       = 0.5
	chargingHeavyShare   = 0.25
	fleetIdleSaturated   = 0.10
)

type FleetResult struct {
	Key          string   `json:"key"`
	SolutionID   string   `json:"solution_id"`
	Name         string   `json:"name"`
	Profile      string   `json:"profile"`
	ProfileLabel string   `json:"profile_label"`
	Quantity     int      `json:"quantity"`
	Serves       []string `json:"serves,omitempty"`
	Processes    []string `json:"processes"`
	SpeedMps     float64  `json:"speed_mps"`
	WidthM       float64  `json:"width_m"`
	EnduranceH   float64  `json:"endurance_h"`
	ChargeMin    float64  `json:"charge_min"`
	Assumed      []string `json:"assumed,omitempty"`
}

type ProcessResult struct {
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	TaskType       string  `json:"task_type"`
	Covered        bool    `json:"covered"`
	Priority       int     `json:"priority"`
	UnitsPerJob    float64 `json:"units_per_job"`
	ExpectedJobs   float64 `json:"expected_jobs"`
	MaxWaitS       float64 `json:"max_wait_s"`
	MaxCycleS      float64 `json:"max_cycle_s"`
	Arrived        Stat    `json:"arrived"`
	Completed      Stat    `json:"completed"`
	Violated       Stat    `json:"violated"`
	ViolationRate  Stat    `json:"violation_rate"`
	ThroughputPerH Stat    `json:"throughput_per_h"`
	WaitMeanS      Stat    `json:"wait_mean_s"`
	WaitP95S       Stat    `json:"wait_p95_s"`
	CycleMeanS     Stat    `json:"cycle_mean_s"`
	CycleP95S      Stat    `json:"cycle_p95_s"`
	QueueMean      Stat    `json:"queue_mean"`
	QueueMax       Stat    `json:"queue_max"`
}

type ResourceResult struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Capacity    int    `json:"capacity"`
	Auto        bool   `json:"auto,omitempty"`
	Utilization Stat   `json:"utilization"`
	QueueMean   Stat   `json:"queue_mean"`
	QueueMax    Stat   `json:"queue_max"`
	WaitMeanS   Stat   `json:"wait_mean_s"`
	WaitMaxS    Stat   `json:"wait_max_s"`
	WaitTotalS  Stat   `json:"wait_total_s"`
}

type KPI struct {
	Arrived          Stat `json:"arrived"`
	Completed        Stat `json:"completed"`
	Violated         Stat `json:"violated"`
	ViolationRate    Stat `json:"violation_rate"`
	ThroughputPerH   Stat `json:"throughput_per_h"`
	WaitMeanS        Stat `json:"wait_mean_s"`
	WaitP95S         Stat `json:"wait_p95_s"`
	CycleMeanS       Stat `json:"cycle_mean_s"`
	CycleP95S        Stat `json:"cycle_p95_s"`
	QueueMean        Stat `json:"queue_mean"`
	QueueMax         Stat `json:"queue_max"`
	FleetUtilization Stat `json:"fleet_utilization"`
	ChargingShare    Stat `json:"charging_share"`
	IdleShare        Stat `json:"idle_share"`
	DistanceKM       Stat `json:"distance_km"`
	BatteryDepleted  Stat `json:"battery_depleted"`
	DispatchWaitS    Stat `json:"dispatch_wait_s"`
}

// Bottleneck is one cause of delay. WaitS is robot-seconds for resources and job-seconds for the fleet.
// FleetShare is the share of fleet time lost in the resource queue, or the busy share for the fleet.
type Bottleneck struct {
	Kind        string  `json:"kind"`
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Primary     bool    `json:"primary"`
	Saturated   bool    `json:"saturated"`
	WaitS       float64 `json:"wait_s"`
	FleetShare  float64 `json:"fleet_share"`
	Utilization float64 `json:"utilization"`
	QueueMax    float64 `json:"queue_max"`
	Text        string  `json:"text"`
}

// Result is the aggregate of all replications stored with the run.
type Result struct {
	SimVersion         string           `json:"sim_version"`
	Config             Config           `json:"config"`
	Replications       int              `json:"replications"`
	Seeds              []int64          `json:"seeds"`
	HorizonS           float64          `json:"horizon_s"`
	Windows            []Window         `json:"windows"`
	DemandProfile      []float64        `json:"demand_profile"`
	ActiveHoursPerDay  float64          `json:"active_hours_per_day"`
	SLATargetPct       float64          `json:"sla_target_pct"`
	SLAOK              bool             `json:"sla_ok"`
	Verdict            string           `json:"verdict"`
	VerdictText        string           `json:"verdict_text"`
	BatteryModeled     bool             `json:"battery_modeled"`
	KPI                KPI              `json:"kpi"`
	Processes          []ProcessResult  `json:"processes"`
	Resources          []ResourceResult `json:"resources"`
	Fleet              []FleetResult    `json:"fleet"`
	Robots             []RobotMetrics   `json:"robots"`
	Bottlenecks        []Bottleneck     `json:"bottlenecks"`
	Representative     int              `json:"representative"`
	RepresentativeSeed int64            `json:"representative_seed"`
	Warnings           []string         `json:"warnings"`
	Assumptions        []string         `json:"assumptions"`
}

// Progress receives completed replications as a fraction, for example 3.5 of 30.
type Progress func(done float64) error

// RunAll runs every replication, re-runs the representative one with a journal and aggregates.
func (m *Model) RunAll(ctx context.Context, progress Progress) (Result, []Metrics, *Log, error) {
	n := m.cfg.Replications
	reps := make([]Metrics, 0, n)
	for i := 0; i < n; i++ {
		if err := ctx.Err(); err != nil {
			return Result{}, nil, nil, err
		}
		seed := ReplicationSeed(m.cfg.Seed, i)
		base := float64(i)
		tick := func(frac float64) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if progress != nil {
				return progress(base + frac)
			}
			return nil
		}
		met, _, err := m.run(i, seed, false, tick)
		if err != nil {
			return Result{}, nil, nil, err
		}
		reps = append(reps, met)
		if progress != nil {
			if err := progress(float64(i + 1)); err != nil {
				return Result{}, nil, nil, err
			}
		}
	}
	rep := medianIndex(reps)
	seed := ReplicationSeed(m.cfg.Seed, rep)
	_, log, err := m.run(rep, seed, true, func(float64) error { return ctx.Err() })
	if err != nil {
		return Result{}, nil, nil, err
	}
	return m.aggregate(reps, rep), reps, log, nil
}

// RunN runs n replications without a journal. Fleet search uses it to try sizes.
func (m *Model) RunN(ctx context.Context, n int, progress Progress) (Result, []Metrics, error) {
	if n < 1 {
		n = 1
	}
	reps := make([]Metrics, 0, n)
	for i := 0; i < n; i++ {
		if err := ctx.Err(); err != nil {
			return Result{}, nil, err
		}
		seed := ReplicationSeed(m.cfg.Seed, i)
		met, _, err := m.Run(i, seed, false)
		if err != nil {
			return Result{}, nil, err
		}
		reps = append(reps, met)
		if progress != nil {
			if err := progress(float64(i + 1)); err != nil {
				return Result{}, nil, err
			}
		}
	}
	return m.aggregate(reps, medianIndex(reps)), reps, nil
}

func (m *Model) aggregate(reps []Metrics, rep int) Result {
	res := Result{
		SimVersion:         Version,
		Config:             m.cfg,
		Replications:       len(reps),
		HorizonS:           m.horizonS,
		Windows:            m.sc.Windows,
		ActiveHoursPerDay:  m.sc.ActiveHoursPerDay,
		SLATargetPct:       m.cfg.targetPct(),
		BatteryModeled:     m.battery,
		Representative:     rep,
		RepresentativeSeed: ReplicationSeed(m.cfg.Seed, rep),
		Warnings:           append([]string{}, m.warnings...),
		Assumptions:        append([]string{}, m.assumptions...),
	}
	res.DemandProfile = m.cfg.DemandProfile
	if len(res.DemandProfile) == 0 {
		res.DemandProfile = DefaultProfile(int(math.Ceil(m.cfg.HorizonH)), m.sc.Windows, m.sc.PeakFactor)
	}
	for i := range reps {
		res.Seeds = append(res.Seeds, reps[i].Seed)
	}
	res.KPI = KPI{
		Arrived:          collect(reps, func(x Metrics) float64 { return float64(x.Arrived) }),
		Completed:        collect(reps, func(x Metrics) float64 { return float64(x.Completed) }),
		Violated:         collect(reps, func(x Metrics) float64 { return float64(x.Violated) }),
		ViolationRate:    collect(reps, func(x Metrics) float64 { return x.ViolationRate }),
		ThroughputPerH:   collect(reps, func(x Metrics) float64 { return x.ThroughputPerH }),
		WaitMeanS:        collect(reps, func(x Metrics) float64 { return x.WaitMeanS }),
		WaitP95S:         collect(reps, func(x Metrics) float64 { return x.WaitP95S }),
		CycleMeanS:       collect(reps, func(x Metrics) float64 { return x.CycleMeanS }),
		CycleP95S:        collect(reps, func(x Metrics) float64 { return x.CycleP95S }),
		QueueMean:        collect(reps, func(x Metrics) float64 { return x.QueueMean }),
		QueueMax:         collect(reps, func(x Metrics) float64 { return float64(x.QueueMax) }),
		FleetUtilization: collect(reps, func(x Metrics) float64 { return x.FleetUtilization }),
		ChargingShare:    collect(reps, func(x Metrics) float64 { return x.ChargingShare }),
		IdleShare:        collect(reps, func(x Metrics) float64 { return x.IdleShare }),
		DistanceKM:       collect(reps, func(x Metrics) float64 { return x.DistanceKM }),
		BatteryDepleted:  collect(reps, func(x Metrics) float64 { return float64(x.BatteryDepleted) }),
		DispatchWaitS:    collect(reps, func(x Metrics) float64 { return x.DispatchWaitS }),
	}
	expected := m.ExpectedArrivals()
	for _, pm := range m.procs {
		pr := ProcessResult{
			Code: pm.spec.Code, Name: pm.spec.Name, TaskType: pm.spec.TaskType, Covered: pm.covered,
			Priority: pm.spec.Priority, UnitsPerJob: pm.spec.UnitsPerJob, ExpectedJobs: round3(expected[pm.spec.Code]),
			MaxWaitS: pm.spec.MaxWaitS, MaxCycleS: pm.spec.MaxCycleS,
		}
		if pm.covered {
			pick := func(f func(ProcessMetrics) float64) Stat {
				return collect(reps, func(x Metrics) float64 {
					for _, p := range x.Processes {
						if p.Code == pm.spec.Code {
							return f(p)
						}
					}
					return 0
				})
			}
			pr.Arrived = pick(func(p ProcessMetrics) float64 { return float64(p.Arrived) })
			pr.Completed = pick(func(p ProcessMetrics) float64 { return float64(p.Completed) })
			pr.Violated = pick(func(p ProcessMetrics) float64 { return float64(p.Violated) })
			pr.ViolationRate = pick(func(p ProcessMetrics) float64 { return p.ViolationRate })
			pr.ThroughputPerH = pick(func(p ProcessMetrics) float64 { return p.ThroughputPerH })
			pr.WaitMeanS = pick(func(p ProcessMetrics) float64 { return p.WaitMeanS })
			pr.WaitP95S = pick(func(p ProcessMetrics) float64 { return p.WaitP95S })
			pr.CycleMeanS = pick(func(p ProcessMetrics) float64 { return p.CycleMeanS })
			pr.CycleP95S = pick(func(p ProcessMetrics) float64 { return p.CycleP95S })
			pr.QueueMean = pick(func(p ProcessMetrics) float64 { return p.QueueMean })
			pr.QueueMax = pick(func(p ProcessMetrics) float64 { return float64(p.QueueMax) })
		}
		res.Processes = append(res.Processes, pr)
	}
	for ri, rm := range m.res {
		pick := func(f func(ResourceMetrics) float64) Stat {
			return collect(reps, func(x Metrics) float64 { return f(x.Resources[ri]) })
		}
		res.Resources = append(res.Resources, ResourceResult{
			ID: rm.id, Kind: rm.kind, Name: rm.name, Capacity: rm.capacity, Auto: rm.auto,
			Utilization: pick(func(r ResourceMetrics) float64 { return r.Utilization }),
			QueueMean:   pick(func(r ResourceMetrics) float64 { return r.QueueMean }),
			QueueMax:    pick(func(r ResourceMetrics) float64 { return float64(r.QueueMax) }),
			WaitMeanS:   pick(func(r ResourceMetrics) float64 { return r.WaitMeanS }),
			WaitMaxS:    pick(func(r ResourceMetrics) float64 { return r.WaitMaxS }),
			WaitTotalS:  pick(func(r ResourceMetrics) float64 { return r.WaitTotalS }),
		})
	}
	for fi, item := range m.sc.Fleet {
		fr := FleetResult{
			Key: item.Key, SolutionID: item.SolutionID, Name: item.Name, Profile: item.Profile.Code,
			ProfileLabel: item.Profile.Label, Quantity: item.Quantity, Serves: item.Serves,
			SpeedMps: item.Profile.SpeedMps, WidthM: item.Profile.WidthM, EnduranceH: item.Profile.EnduranceH,
			ChargeMin: item.Profile.ChargeMin, Assumed: item.Profile.Assumed, Processes: []string{},
		}
		for pi, pm := range m.procs {
			if m.canServe[fi][pi] {
				fr.Processes = append(fr.Processes, pm.spec.Code)
			}
		}
		res.Fleet = append(res.Fleet, fr)
	}
	if rep < len(reps) {
		res.Robots = reps[rep].Robots
	}
	p90 := res.KPI.ViolationRate.P90 * 100
	res.SLAOK = p90 <= res.SLATargetPct+1e-9
	res.Bottlenecks = m.bottlenecks(res)
	if res.SLAOK {
		res.Verdict = VerdictPass
		res.VerdictText = fmt.Sprintf("SLA выполняется: в 90%% повторов нарушений не больше %s (факт %s).", rutext.Pct(res.SLATargetPct, 1), rutext.Pct(p90, 1))
	} else {
		res.Verdict = VerdictFail
		res.VerdictText = fmt.Sprintf("SLA не выполняется: в 90%% повторов нарушений до %s при допуске %s.", rutext.Pct(p90, 1), rutext.Pct(res.SLATargetPct, 1))
		if len(res.Bottlenecks) > 0 {
			res.VerdictText += " Главная причина: " + lowerFirst(res.Bottlenecks[0].Text)
		}
	}
	if len(reps) == 1 {
		res.VerdictText = strings.Replace(res.VerdictText, "в 90% повторов ", "", 1)
	}
	return res
}

// bottlenecks ranks delay causes. A saturated resource comes first: more robots do not help there.
// Otherwise a long job queue with busy robots points at the fleet size.
func (m *Model) bottlenecks(res Result) []Bottleneck {
	robotTime := float64(len(m.robots)) * m.horizonS
	k := res.KPI
	items := []Bottleneck{}
	for _, r := range res.Resources {
		share := ratio(r.WaitTotalS.Median, robotTime)
		saturated := r.Utilization.Median >= saturatedUtilization || r.QueueMean.Median >= saturatedQueue*float64(r.Capacity)
		var text string
		switch r.Kind {
		case "dock":
			text = fmt.Sprintf("Док %s занят на %s, роботы ждали его %s в сумме, очередь до %s. Добавьте ворота или разнесите приёмку и отгрузку по времени.",
				r.Name, rutext.Pct(r.Utilization.Median*100, 0), humanDur(r.WaitTotalS.Median), rutext.Num(r.QueueMax.Median, 0))
		case "charger":
			text = fmt.Sprintf("Зарядка %s занята на %s, роботы ждали её %s в сумме, очередь до %s. Добавьте зарядные места.",
				r.Name, rutext.Pct(r.Utilization.Median*100, 0), humanDur(r.WaitTotalS.Median), rutext.Num(r.QueueMax.Median, 0))
		default:
			text = fmt.Sprintf("Узкий участок %s занят на %s, роботы ждали его %s в сумме, очередь до %s. Расширьте проход или добавьте объезд.",
				r.Name, rutext.Pct(r.Utilization.Median*100, 0), humanDur(r.WaitTotalS.Median), rutext.Num(r.QueueMax.Median, 0))
		}
		if !saturated && (share < minFleetShare || r.WaitTotalS.Median < bottleneckMinWaitS) {
			continue
		}
		items = append(items, Bottleneck{
			Kind: r.Kind, ID: r.ID, Name: r.Name, Saturated: saturated, WaitS: r.WaitTotalS.Median,
			FleetShare: share, Utilization: r.Utilization.Median, QueueMax: r.QueueMax.Median, Text: text,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Saturated != items[j].Saturated {
			return items[i].Saturated
		}
		if items[i].WaitS != items[j].WaitS {
			return items[i].WaitS > items[j].WaitS
		}
		return items[i].ID < items[j].ID
	})
	jobsWaiting := k.WaitMeanS.Median >= bottleneckMinWaitS || k.ViolationRate.P90*100 > res.SLATargetPct
	fleetBusy := k.IdleShare.Median < fleetIdleSaturated
	if jobsWaiting && (fleetBusy || len(items) == 0) {
		text := fmt.Sprintf("Задания ждали свободного робота в среднем %s, роботы заняты на %s, свободны %s времени. Роботов не хватает для потока заданий.",
			humanDur(k.WaitMeanS.Median), rutext.Pct(k.FleetUtilization.Median*100, 0), rutext.Pct(k.IdleShare.Median*100, 0))
		if k.ChargingShare.Median >= chargingHeavyShare {
			text = fmt.Sprintf("Задания ждали свободного робота в среднем %s, роботы %s времени заняты зарядкой. Нужны роботы с большей автономностью или больше роботов.",
				humanDur(k.WaitMeanS.Median), rutext.Pct(k.ChargingShare.Median*100, 0))
		} else if !fleetBusy {
			text = fmt.Sprintf("Задания ждали назначения в среднем %s, хотя роботы свободны %s времени. Проверьте, какие роботы назначены на процессы.",
				humanDur(k.WaitMeanS.Median), rutext.Pct(k.IdleShare.Median*100, 0))
		}
		fleet := Bottleneck{
			Kind: "fleet", ID: "fleet", Name: "Флот", WaitS: k.DispatchWaitS.Median, Saturated: fleetBusy,
			FleetShare: k.FleetUtilization.Median, Utilization: k.FleetUtilization.Median, QueueMax: k.QueueMax.Median, Text: text,
		}
		pos := 0
		for pos < len(items) && items[pos].Saturated {
			pos++
		}
		if fleetBusy && pos > 0 && items[0].FleetShare < minFleetShare {
			pos = 0
		}
		items = append(items[:pos], append([]Bottleneck{fleet}, items[pos:]...)...)
	}
	if len(items) > 0 && !res.SLAOK {
		items[0].Primary = true
	}
	return items
}

func humanDur(s float64) string {
	switch {
	case s < 90:
		return rutext.Num(s, 0) + " с"
	case s < 5400:
		return rutext.Num(s/60, 1) + " мин"
	default:
		return rutext.Num(s/3600, 1) + " ч"
	}
}

func upperFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToLower(string(r[0])) + string(r[1:])
}
