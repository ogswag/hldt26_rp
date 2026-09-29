package sim

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/sim/profiles"
)

var allCodes = []string{"inbound", "putaway", "piece_pick", "outbound"}

func processByCode(code string) ProcessSpec {
	switch code {
	case "inbound":
		return ProcessSpec{Code: code, Name: "Приёмка", TaskType: profiles.TaskPalletInbound, UnitsPerDay: 1000, UnitsPerJob: 1,
			MaxWaitS: 30 * 60, MaxCycleS: 45 * 60, StationLoadS: 90, StationUnloadS: 60}
	case "putaway":
		return ProcessSpec{Code: code, Name: "Размещение", TaskType: profiles.TaskPalletPutaway, UnitsPerDay: 1000, UnitsPerJob: 1,
			MaxWaitS: 20 * 60, MaxCycleS: 40 * 60, StationLoadS: 60, StationUnloadS: 60}
	case "piece_pick":
		return ProcessSpec{Code: code, Name: "Отбор", TaskType: profiles.TaskPiecePick, UnitsPerDay: 100000, UnitsPerJob: 40,
			MaxWaitS: 15 * 60, MaxCycleS: 25 * 60, StationLoadS: 20, StationUnloadS: 15}
	default:
		return ProcessSpec{Code: code, Name: "Отгрузка", TaskType: profiles.TaskPalletOutbound, UnitsPerDay: 1000, UnitsPerJob: 1,
			MaxWaitS: 25 * 60, MaxCycleS: 40 * 60, StationLoadS: 60, StationUnloadS: 90}
	}
}

func amr(qty int) FleetItem {
	speed, width, payload, endurance, charge := 1.5, 654.0, 1500.0, 6.0, 18.0
	p := profiles.Resolve(profiles.AMR, profiles.Specs{SpeedMps: &speed, WidthMm: &width, PayloadKg: &payload, EnduranceH: &endurance, ChargeMin: &charge})
	return FleetItem{Key: "amr", SolutionID: "h1500", Name: "Ronavi H1500", Profile: p, Quantity: qty}
}

func stacker(qty int) FleetItem {
	return FleetItem{Key: "stacker", SolutionID: "robocv", Name: "RoboCV stacker", Profile: profiles.Resolve(profiles.Pallet, profiles.Specs{}), Quantity: qty}
}

func scenarioFor(t *testing.T, codes []string, fleet []FleetItem, cfg Config, edit func(*maps.Document)) Scenario {
	t.Helper()
	doc := maps.WarehouseTemplate(codes, maps.TemplateWidths{MainM: 3.5, WorkingM: 2.8})
	if edit != nil {
		edit(&doc)
	}
	var classes []maps.ClassSpec
	for _, f := range fleet {
		classes = append(classes, maps.ClassSpec{Code: f.Profile.Code, Label: f.Profile.Label, WidthM: f.Profile.WidthM, ClearanceM: f.Profile.ClearanceM})
	}
	issues := maps.Validate(doc, maps.Context{ProcessCodes: codes, Classes: classes})
	if maps.HasErrors(issues) {
		t.Fatalf("template invalid: %+v", issues)
	}
	g, scene := maps.Build(doc, 3.0)
	var procs []ProcessSpec
	for _, code := range codes {
		p := processByCode(code)
		for _, f := range doc.Layers.Flows {
			if f.ProcessCode == code {
				p.Pickups = f.PickupPointIDs
				p.Drops = f.DropPointIDs
			}
		}
		procs = append(procs, p)
	}
	return Scenario{
		Config:            cfg,
		Graph:             g,
		Scene:             scene,
		Resources:         doc.Layers.Resources,
		Processes:         procs,
		Fleet:             fleet,
		ActiveHoursPerDay: 14,
		PeakFactor:        1.5,
		LiftM:             4,
	}
}

func runAll(t *testing.T, sc Scenario) (Result, []Metrics, *Log) {
	t.Helper()
	m, err := Prepare(sc)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	res, reps, log, err := m.RunAll(context.Background(), nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return res, reps, log
}

func setEdgeWidth(id string, w float64) func(*maps.Document) {
	return func(d *maps.Document) {
		for i := range d.Layers.Edges {
			if d.Layers.Edges[i].ID == id {
				v := w
				d.Layers.Edges[i].WidthM = &v
			}
		}
	}
}

func setCapacity(id string, c int) func(*maps.Document) {
	return func(d *maps.Document) {
		for i := range d.Layers.Resources {
			if d.Layers.Resources[i].ID == id {
				d.Layers.Resources[i].Capacity = c
			}
		}
	}
}

func widenNarrowPassage(d *maps.Document) {
	setEdgeWidth("e-narrow", 4)(d)
	var res []maps.Resource
	for _, r := range d.Layers.Resources {
		if r.ID != "res-narrow" {
			res = append(res, r)
		}
	}
	d.Layers.Resources = res
	var obs []maps.Polygon
	for _, o := range d.Layers.Obstacles {
		if o.ID != "wall-n1" && o.ID != "wall-n2" {
			obs = append(obs, o)
		}
	}
	d.Layers.Obstacles = obs
}

func resourceByID(t *testing.T, res Result, id string) ResourceResult {
	t.Helper()
	for _, r := range res.Resources {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("resource %s missing in %+v", id, res.Resources)
	return ResourceResult{}
}

var quick = Config{Mode: ModeStochastic, Replications: 6, HorizonH: 4}

func TestInsufficientFleetFailsSufficientPasses(t *testing.T) {
	small, _, _ := runAll(t, scenarioFor(t, []string{"putaway"}, []FleetItem{stacker(2)}, quick, nil))
	if small.Verdict != VerdictFail || small.SLAOK {
		t.Fatalf("2 stackers must violate SLA: %s", small.VerdictText)
	}
	if len(small.Bottlenecks) == 0 || small.Bottlenecks[0].Kind != "fleet" || !small.Bottlenecks[0].Primary {
		t.Fatalf("primary cause must be the fleet: %+v", small.Bottlenecks)
	}
	if small.KPI.QueueMax.Median < 50 || small.KPI.WaitMeanS.Median < 30*60 {
		t.Fatalf("queue must show the shortage: %+v", small.KPI)
	}
	big, _, _ := runAll(t, scenarioFor(t, []string{"putaway"}, []FleetItem{stacker(12)}, quick, nil))
	if big.Verdict != VerdictPass {
		t.Fatalf("12 stackers must pass: %s", big.VerdictText)
	}
	if big.KPI.ThroughputPerH.Median <= small.KPI.ThroughputPerH.Median {
		t.Fatalf("throughput %v <= %v", big.KPI.ThroughputPerH.Median, small.KPI.ThroughputPerH.Median)
	}
	for _, b := range big.Bottlenecks {
		if b.Primary {
			t.Fatalf("passing run has a primary bottleneck: %+v", b)
		}
	}
}

func TestDockCapacityIsTheVisibleCause(t *testing.T) {
	sc := scenarioFor(t, []string{"inbound"}, []FleetItem{amr(12)}, quick, setCapacity("res-docks", 1))
	res, _, _ := runAll(t, sc)
	if res.Verdict != VerdictFail {
		t.Fatalf("one dock slot must break SLA: %s", res.VerdictText)
	}
	if res.Bottlenecks[0].ID != "res-docks" || !res.Bottlenecks[0].Saturated {
		t.Fatalf("primary cause must be the dock: %+v", res.Bottlenecks)
	}
	dock := resourceByID(t, res, "res-docks")
	if dock.Utilization.Median < 0.85 || dock.QueueMax.Median < 2 {
		t.Fatalf("dock load %+v", dock)
	}
	ok, _, _ := runAll(t, scenarioFor(t, []string{"inbound"}, []FleetItem{amr(12)}, quick, setCapacity("res-docks", 4)))
	if ok.Verdict != VerdictPass {
		t.Fatalf("four dock slots must pass: %s", ok.VerdictText)
	}
}

func distancePerJob(reps []Metrics) float64 {
	s := 0.0
	for _, r := range reps {
		s += r.DistanceKM / float64(r.Completed)
	}
	return s / float64(len(reps))
}

func TestGeometryChangesRouteQueueAndThroughput(t *testing.T) {
	fleet := []FleetItem{amr(10)}
	base, baseReps, _ := runAll(t, scenarioFor(t, []string{"piece_pick"}, fleet, quick, nil))
	blocked, blockedReps, _ := runAll(t, scenarioFor(t, []string{"piece_pick"}, fleet, quick, setEdgeWidth("e-narrow", 1.0)))
	wide, wideReps, _ := runAll(t, scenarioFor(t, []string{"piece_pick"}, fleet, quick, widenNarrowPassage))

	if d0, d1 := distancePerJob(baseReps), distancePerJob(blockedReps); d1 < 1.5*d0 {
		t.Fatalf("blocked passage must force the bypass: %.4f vs %.4f km per job", d1, d0)
	}
	if blocked.KPI.ThroughputPerH.Median >= base.KPI.ThroughputPerH.Median {
		t.Fatalf("detour must cut throughput: %v vs %v", blocked.KPI.ThroughputPerH.Median, base.KPI.ThroughputPerH.Median)
	}
	if narrow := resourceByID(t, blocked, "res-narrow"); narrow.Utilization.Median != 0 {
		t.Fatalf("blocked passage still used: %+v", narrow)
	}

	narrow := resourceByID(t, base, "res-narrow")
	if narrow.QueueMax.Median < 2 || narrow.WaitTotalS.Median < 600 {
		t.Fatalf("narrow passage queue not visible: %+v", narrow)
	}
	if base.Verdict != VerdictFail || base.Bottlenecks[0].ID != "res-narrow" {
		t.Fatalf("narrow passage must be the primary cause: %s %+v", base.VerdictText, base.Bottlenecks)
	}
	if wide.Verdict != VerdictPass {
		t.Fatalf("wide passage must pass: %s", wide.VerdictText)
	}
	if distancePerJob(wideReps) > distancePerJob(baseReps)*1.02 {
		t.Fatal("widening must not lengthen routes")
	}
	if wide.KPI.CycleP95S.Median >= base.KPI.CycleP95S.Median {
		t.Fatalf("cycle p95 %v >= %v", wide.KPI.CycleP95S.Median, base.KPI.CycleP95S.Median)
	}
}

func TestReplicationsAreDeterministic(t *testing.T) {
	sc := scenarioFor(t, allCodes, []FleetItem{amr(6), stacker(6)}, quick, nil)
	a, repsA, logA := runAll(t, sc)
	b, _, logB := runAll(t, sc)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatal("same seed gave different results")
	}
	la, _ := json.Marshal(logA)
	lb, _ := json.Marshal(logB)
	if string(la) != string(lb) {
		t.Fatal("same seed gave different journals")
	}
	if a.Replications != 6 || len(a.Seeds) != 6 || a.Seeds[5] != 5 {
		t.Fatalf("seeds %v", a.Seeds)
	}
	m, err := Prepare(sc)
	if err != nil {
		t.Fatal(err)
	}
	plain, _, err := m.Run(a.Representative, a.RepresentativeSeed, false)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Completed != repsA[a.Representative].Completed || plain.Violated != repsA[a.Representative].Violated {
		t.Fatal("representative rerun diverged")
	}
	recorded, _, err := m.Run(a.Representative, a.RepresentativeSeed, true)
	if err != nil {
		t.Fatal(err)
	}
	jp, _ := json.Marshal(plain)
	jr, _ := json.Marshal(recorded)
	if string(jp) != string(jr) {
		t.Fatal("recording changed the metrics")
	}
	c := sc
	c.Config.Seed = 7
	other, _, _ := runAll(t, c)
	if other.KPI.Completed == a.KPI.Completed && other.KPI.WaitMeanS == a.KPI.WaitMeanS {
		t.Fatal("different seed gave identical aggregates")
	}
}

func TestJournalHasEveryEventType(t *testing.T) {
	cfg := Config{Mode: ModeStochastic, Replications: 2, HorizonH: 4}
	_, _, log := runAll(t, scenarioFor(t, []string{"inbound", "putaway"}, []FleetItem{amr(2), stacker(3)}, cfg, nil))
	seen := map[string]int{}
	lastT := 0.0
	for i, e := range log.Events {
		seen[e.Type]++
		if e.Seq != i || e.T < lastT {
			t.Fatalf("event order broken at %d", i)
		}
		lastT = e.T
		if e.Type == "route_start" && (e.Payload == nil || len(e.Payload.Path) < 2 || e.Payload.DurS == nil) {
			t.Fatalf("route_start without path: %+v", e)
		}
	}
	for _, typ := range []string{"job_arrival", "dispatch", "route_start", "route_end", "resource_acquire", "resource_release",
		"load", "unload", "charge_start", "charge_end", "sla_reached", "sla_violated"} {
		if seen[typ] == 0 {
			t.Fatalf("no %s in journal: %v", typ, seen)
		}
	}
	if log.SchemaVersion != LogSchemaVersion || log.SimVersion != Version || len(log.Robots) != 5 || len(log.Nodes) == 0 {
		t.Fatalf("header %+v", log.Robots)
	}
	if len(log.Checkpoints) != 241 || log.Checkpoints[240].T != 14400 {
		t.Fatalf("checkpoints %d", len(log.Checkpoints))
	}
	if log.Scene.WidthM != 84 || len(log.Scene.Obstacles) == 0 {
		t.Fatalf("scene %+v", log.Scene)
	}
}

func TestDeterministicModeMatchesExpectedDemand(t *testing.T) {
	cfg := Config{Mode: ModeDeterministic, Replications: 30, HorizonH: 4}
	sc := scenarioFor(t, []string{"inbound"}, []FleetItem{amr(12)}, cfg, nil)
	m, err := Prepare(sc)
	if err != nil {
		t.Fatal(err)
	}
	if m.Replications() != 1 {
		t.Fatalf("deterministic replications %d", m.Replications())
	}
	res, reps, _, err := m.RunAll(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := m.ExpectedArrivals()["inbound"]
	got := float64(reps[0].Arrived)
	if got < want-1 || got > want+1 {
		t.Fatalf("arrivals %v, expected %v", got, want)
	}
	if res.KPI.Arrived.Min != res.KPI.Arrived.Max {
		t.Fatal("one replication must have a zero range")
	}
	if res.VerdictText == "" || res.DemandProfile == nil || len(res.DemandProfile) != 4 {
		t.Fatalf("result %+v", res)
	}
}

func TestSLAPriorityProtectsUrgentProcess(t *testing.T) {
	codes := []string{"inbound", "piece_pick"}
	run := func(policy string) Result {
		sc := scenarioFor(t, codes, []FleetItem{amr(7)}, Config{Mode: ModeStochastic, Replications: 6, HorizonH: 4, Policy: policy}, widenNarrowPassage)
		for i := range sc.Processes {
			if sc.Processes[i].Code == "piece_pick" {
				sc.Processes[i].Priority = 5
			}
		}
		res, _, _ := runAll(t, sc)
		return res
	}
	pick := func(r Result) ProcessResult {
		for _, p := range r.Processes {
			if p.Code == "piece_pick" {
				return p
			}
		}
		t.Fatal("piece_pick missing")
		return ProcessResult{}
	}
	fifo := run(PolicyFIFO)
	sla := run(PolicySLA)
	if pick(sla).ViolationRate.Median >= pick(fifo).ViolationRate.Median {
		t.Fatalf("priority did not help: sla %v fifo %v", pick(sla).ViolationRate, pick(fifo).ViolationRate)
	}
}

func TestChooseRobotByPolicy(t *testing.T) {
	for _, tc := range []struct {
		policy string
		want   string
	}{{PolicyFIFO, "R1"}, {PolicyNearest, "R2"}, {PolicySLA, "R2"}} {
		cfg := Config{Mode: ModeDeterministic, HorizonH: 1, Policy: tc.policy}
		m, err := Prepare(scenarioFor(t, []string{"inbound"}, []FleetItem{amr(2)}, cfg, nil))
		if err != nil {
			t.Fatal(err)
		}
		r := &runner{m: m, horizon: m.horizonS}
		r.init()
		far, _ := m.sc.Graph.NodeIndex("C1")
		near, _ := m.sc.Graph.NodeIndex("B1")
		pickup, _ := m.sc.Graph.NodeIndex("D1")
		drop := near
		m.ensureSource(far)
		m.ensureSource(near)
		r.robots[0].node, r.robots[0].idleSince = far, 10
		r.robots[1].node, r.robots[1].idleSince = near, 20
		got := r.chooseRobot(r.robots, 0, &job{pickup: pickup, drop: drop})
		if got == nil || got.spec.id != tc.want {
			t.Fatalf("%s picked %+v, want %s", tc.policy, got, tc.want)
		}
	}
}

func TestUncoveredProcessWarnsAndSkips(t *testing.T) {
	res, _, _ := runAll(t, scenarioFor(t, []string{"inbound", "putaway"}, []FleetItem{amr(4)}, quick, nil))
	var putaway ProcessResult
	for _, p := range res.Processes {
		if p.Code == "putaway" {
			putaway = p
		}
	}
	if putaway.Covered || putaway.Arrived.Median != 0 {
		t.Fatalf("AMR must not serve putaway: %+v", putaway)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("missing warning for skipped process")
	}
	if _, err := Prepare(scenarioFor(t, []string{"putaway"}, []FleetItem{amr(4)}, quick, nil)); err == nil {
		t.Fatal("no covered process must be an input error")
	} else {
		var ie *InputError
		if !errors.As(err, &ie) {
			t.Fatalf("error type %T", err)
		}
	}
}

func TestCancelStopsRun(t *testing.T) {
	m, err := Prepare(scenarioFor(t, allCodes, []FleetItem{amr(6), stacker(6)}, Config{Mode: ModeStochastic, Replications: 30, HorizonH: 8}, nil))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, _, _, err = m.RunAll(ctx, func(done float64) error {
		calls++
		if done >= 2 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if calls == 0 {
		t.Fatal("no progress reported")
	}
}

func TestConfigValidation(t *testing.T) {
	neg := -1.0
	bad := []Config{
		{Mode: "fast"},
		{Mode: ModeStochastic, Replications: 31},
		{Mode: ModeStochastic, Replications: 1},
		{Policy: "random"},
		{HorizonH: 25},
		{Seed: -1},
		{HorizonH: 2, DemandProfile: []float64{1}},
		{HorizonH: 2, DemandProfile: []float64{1, -1}},
		{HorizonH: 2, Schedule: []Window{{StartH: 1, EndH: 3}}},
		{SLATargetPct: &neg},
	}
	for i, c := range bad {
		if _, err := c.Normalize(); err == nil {
			t.Fatalf("case %d accepted: %+v", i, c)
		}
	}
	c, err := Config{}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeStochastic || c.Replications != DefaultReplications || c.Policy != PolicyFIFO || c.HorizonH != DefaultHorizonH || *c.SLATargetPct != DefaultSLATargetPct {
		t.Fatalf("defaults %+v", c)
	}
}

func TestDefaultProfileKeepsDailyDemand(t *testing.T) {
	windows := []Window{{StartH: 0, EndH: 8}}
	p := DefaultProfile(8, windows, 1.5)
	sum := 0.0
	peak := 0.0
	for _, v := range p {
		sum += v
		if v > peak {
			peak = v
		}
	}
	if sum < 7.99 || sum > 8.01 {
		t.Fatalf("mean must be 1, sum %v", sum)
	}
	if peak < 1.2 {
		t.Fatalf("peak %v", peak)
	}
	off := DefaultProfile(4, []Window{{StartH: 0, EndH: 2}}, 1.5)
	if len(off) != 4 {
		t.Fatal(off)
	}
}
