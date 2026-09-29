package sim

import (
	"container/heap"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
)

const (
	stIdle = iota
	stTravelEmpty
	stTravelLoaded
	stHandling
	stWaitResource
	stToCharger
	stWaitCharger
	stCharging
	stateCount
)

var stateNames = [stateCount]string{
	"idle", "travel_empty", "travel_loaded", "handling", "wait_resource", "to_charger", "wait_charger", "charging",
}

const tickEvery = 20000

// ErrTooManyEvents stops a replication that would not finish in reasonable time.
var ErrTooManyEvents = errors.New("sim: event limit reached")

type event struct {
	t   float64
	seq uint64
	fn  func()
}

type eventHeap []*event

func (h eventHeap) Len() int { return len(h) }
func (h eventHeap) Less(i, j int) bool {
	if h[i].t != h[j].t {
		return h[i].t < h[j].t
	}
	return h[i].seq < h[j].seq
}
func (h eventHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *eventHeap) Push(x any)   { *h = append(*h, x.(*event)) }
func (h *eventHeap) Pop() any {
	old := *h
	e := old[len(old)-1]
	*h = old[:len(old)-1]
	return e
}

type job struct {
	idx        int
	proc       int
	pickup     int
	drop       int
	tArr       float64
	tDisp      float64
	tDone      float64
	dispatched bool
	done       bool
	violated   bool
	robot      int
}

type robot struct {
	spec         robotSpec
	idx          int
	class        int
	node         int
	state        int
	lastT        float64
	idleSince    float64
	battery      float64
	startBattery float64
	job          *job
	held         int
	timeIn       [stateCount]float64
	distM        float64
	jobs         int
	depleted     int
}

type waiter struct {
	rb    *robot
	since float64
	then  func()
}

type resState struct {
	model     resModel
	inUse     int
	queue     []waiter
	lastT     float64
	useArea   float64
	queueArea float64
	queueMax  int
	acquired  int
	waitTotal float64
	waitMax   float64
}

type procState struct {
	queue     []*job
	lastT     float64
	queueArea float64
	queueMax  int
	target    float64
	count     int
}

type runner struct {
	m          *Model
	rng        *rand.Rand
	stochastic bool
	now        float64
	horizon    float64
	seq        uint64
	events     eventHeap
	steps      int
	robots     []*robot
	jobs       []*job
	procs      []*procState
	res        []*resState
	rec        *recorder
	queueLen   int
	queueArea  float64
	queueMax   int
	queueLastT float64
	completed  int
	violated   int
	ckpts      int
	err        error
}

// Run executes replication rep with seed. When record is true it also returns the event log.
func (m *Model) Run(rep int, seed int64, record bool) (Metrics, *Log, error) {
	return m.run(rep, seed, record, nil)
}

func (m *Model) run(rep int, seed int64, record bool, tick func(frac float64) error) (Metrics, *Log, error) {
	r := &runner{
		m:          m,
		rng:        rand.New(rand.NewSource(seed)),
		stochastic: m.cfg.stochastic(),
		horizon:    m.horizonS,
	}
	if record {
		r.rec = &recorder{}
	}
	r.init()
	for r.events.Len() > 0 && r.err == nil {
		ev := heap.Pop(&r.events).(*event)
		if ev.t > r.horizon {
			break
		}
		r.now = ev.t
		ev.fn()
		r.steps++
		if r.steps > maxEventsPerReplication {
			r.err = ErrTooManyEvents
		}
		if tick != nil && r.steps%tickEvery == 0 {
			if err := tick(r.now / r.horizon); err != nil {
				r.err = err
			}
		}
	}
	if r.err != nil {
		return Metrics{}, nil, fmt.Errorf("sim.run replication %d: %w", rep, r.err)
	}
	r.now = r.horizon
	met := r.finish()
	met.Replication = rep
	met.Seed = seed
	var log *Log
	if record {
		log = r.buildLog(rep, seed)
	}
	return met, log, nil
}

func (r *runner) at(t float64, fn func()) {
	if t < r.now {
		t = r.now
	}
	r.seq++
	heap.Push(&r.events, &event{t: t, seq: r.seq, fn: fn})
}

func (r *runner) noise(sigma float64) float64 {
	if !r.stochastic || sigma <= 0 {
		return 1
	}
	return math.Exp(r.rng.NormFloat64()*sigma - sigma*sigma/2)
}

func (r *runner) init() {
	m := r.m
	for i, spec := range m.robots {
		p := m.sc.Fleet[spec.fleet].Profile
		b := 1.0
		if r.stochastic {
			b = 0.6 + 0.4*r.rng.Float64()
		}
		if p.ChargeTo > 0 && b > p.ChargeTo && !r.stochastic {
			b = p.ChargeTo
		}
		rb := &robot{spec: spec, idx: i, class: spec.class, node: spec.start, battery: b, startBattery: b, held: -1}
		r.robots = append(r.robots, rb)
	}
	for _, rm := range m.res {
		r.res = append(r.res, &resState{model: rm})
	}
	for pi, p := range m.procs {
		ps := &procState{}
		r.procs = append(r.procs, ps)
		if !p.covered {
			continue
		}
		if r.stochastic {
			ps.target = r.rng.ExpFloat64()
		} else {
			ps.target = float64(pi+1) / float64(len(m.procs)+1)
		}
		r.scheduleArrival(pi)
	}
	if r.rec != nil {
		r.at(0, r.checkpoint)
	}
	r.at(0, r.dispatch)
}

func (r *runner) scheduleArrival(pi int) {
	ps := r.procs[pi]
	t, ok := r.m.procs[pi].arrivalAt(ps.target)
	if !ok || t > r.horizon {
		return
	}
	r.at(t, func() { r.arrive(pi) })
}

func (r *runner) arrive(pi int) {
	pm := r.m.procs[pi]
	ps := r.procs[pi]
	j := &job{idx: len(r.jobs), proc: pi, tArr: r.now, robot: -1}
	if r.stochastic {
		j.pickup = pm.pickups[r.rng.Intn(len(pm.pickups))]
		j.drop = pm.drops[r.rng.Intn(len(pm.drops))]
	} else {
		j.pickup = pm.pickups[ps.count%len(pm.pickups)]
		j.drop = pm.drops[ps.count%len(pm.drops)]
	}
	ps.count++
	r.jobs = append(r.jobs, j)
	r.touchQueues(pi)
	ps.queue = append(ps.queue, j)
	r.queueLen++
	if len(ps.queue) > ps.queueMax {
		ps.queueMax = len(ps.queue)
	}
	if r.queueLen > r.queueMax {
		r.queueMax = r.queueLen
	}
	prio := pm.spec.Priority
	r.emit(Event{Type: "job_arrival", JobID: jobID(j), ProcessCode: pm.spec.Code, FromID: r.nodeID(j.pickup), ToID: r.nodeID(j.drop), Priority: &prio,
		Payload: &Payload{Queue: intPtr(len(ps.queue))}})
	if s := pm.spec.MaxWaitS; s > 0 {
		r.at(j.tArr+s, func() {
			if !j.dispatched {
				r.violate(j, "wait")
			}
		})
	}
	if s := pm.spec.MaxCycleS; s > 0 {
		r.at(j.tArr+s, func() {
			if !j.done {
				r.violate(j, "cycle")
			}
		})
	}
	if r.stochastic {
		ps.target += r.rng.ExpFloat64()
	} else {
		ps.target++
	}
	r.scheduleArrival(pi)
	r.dispatch()
}

func (r *runner) violate(j *job, kind string) {
	if !j.violated {
		j.violated = true
		r.violated++
	}
	pm := r.m.procs[j.proc]
	elapsed := r.now - j.tArr
	ev := Event{Type: "sla_violated", JobID: jobID(j), ProcessCode: pm.spec.Code, Payload: &Payload{Kind: kind, WaitS: f1(elapsed)}}
	if j.robot >= 0 {
		ev.RobotID = r.robots[j.robot].spec.id
	}
	r.emit(ev)
}

func (r *runner) touchQueues(pi int) {
	dt := r.now - r.queueLastT
	if dt > 0 {
		r.queueArea += float64(r.queueLen) * dt
		r.queueLastT = r.now
	}
	ps := r.procs[pi]
	if d := r.now - ps.lastT; d > 0 {
		ps.queueArea += float64(len(ps.queue)) * d
		ps.lastT = r.now
	}
}

func (r *runner) batteryNow(rb *robot) float64 {
	if !r.m.battery {
		return 1
	}
	dt := r.now - rb.lastT
	b := rb.battery - r.drain(rb, rb.state)*dt
	return clamp01(b)
}

func (r *runner) drain(rb *robot, state int) float64 {
	p := r.m.sc.Fleet[rb.spec.fleet].Profile
	switch state {
	case stTravelEmpty, stToCharger:
		return p.DrainPerS(p.DrawMove)
	case stTravelLoaded:
		return p.DrainPerS(p.DrawMoveLoaded)
	case stHandling:
		return p.DrainPerS(p.DrawHandle)
	case stCharging:
		return -p.ChargePerS()
	default:
		return p.DrainPerS(p.DrawIdle)
	}
}

func (r *runner) setState(rb *robot, s int) {
	dt := r.now - rb.lastT
	if dt > 0 {
		rb.timeIn[rb.state] += dt
		if r.m.battery {
			before := rb.battery
			rb.battery = clamp01(rb.battery - r.drain(rb, rb.state)*dt)
			if before > 0 && rb.battery == 0 {
				rb.depleted++
			}
		}
	}
	rb.lastT = r.now
	rb.state = s
}

func (r *runner) needsCharge(rb *robot) bool {
	if !r.m.battery {
		return false
	}
	p := r.m.sc.Fleet[rb.spec.fleet].Profile
	return r.batteryNow(rb) < p.ChargeBelow
}

type candidate struct {
	pi int
	j  *job
}

func (r *runner) dispatch() {
	for {
		var idle []*robot
		for _, rb := range r.robots {
			if rb.state != stIdle || rb.job != nil {
				continue
			}
			if r.needsCharge(rb) && r.goCharge(rb) {
				continue
			}
			idle = append(idle, rb)
		}
		if len(idle) == 0 {
			return
		}
		var best *candidate
		var bestRobot *robot
		for pi, ps := range r.procs {
			if len(ps.queue) == 0 {
				continue
			}
			head := ps.queue[0]
			rb := r.chooseRobot(idle, pi, head)
			if rb == nil {
				continue
			}
			c := &candidate{pi: pi, j: head}
			if best == nil || r.before(c, best) {
				best = c
				bestRobot = rb
			}
		}
		if best == nil {
			return
		}
		r.assign(bestRobot, best.pi)
	}
}

func (r *runner) before(a, b *candidate) bool {
	if r.m.cfg.Policy == PolicySLA {
		pa := r.m.procs[a.pi].spec
		pb := r.m.procs[b.pi].spec
		if pa.Priority != pb.Priority {
			return pa.Priority > pb.Priority
		}
		da, db := deadline(a.j, pa), deadline(b.j, pb)
		if da != db {
			return da < db
		}
	}
	if a.j.tArr != b.j.tArr {
		return a.j.tArr < b.j.tArr
	}
	return a.j.idx < b.j.idx
}

func deadline(j *job, p ProcessSpec) float64 {
	d := math.Inf(1)
	if p.MaxWaitS > 0 {
		d = j.tArr + p.MaxWaitS
	}
	if p.MaxCycleS > 0 {
		d = math.Min(d, j.tArr+p.MaxCycleS)
	}
	return d
}

func (r *runner) chooseRobot(idle []*robot, pi int, j *job) *robot {
	var pick *robot
	pickDist := math.Inf(1)
	for _, rb := range idle {
		if !r.m.canServe[rb.spec.fleet][pi] {
			continue
		}
		toPick := r.m.reach(rb.class, rb.node, j.pickup)
		if math.IsInf(toPick, 1) || math.IsInf(r.m.reach(rb.class, j.pickup, j.drop), 1) {
			continue
		}
		if r.m.cfg.Policy == PolicyFIFO {
			if pick == nil || rb.idleSince < pick.idleSince {
				pick = rb
			}
			continue
		}
		if pick == nil || toPick < pickDist-1e-9 {
			pick = rb
			pickDist = toPick
		}
	}
	return pick
}

func (r *runner) assign(rb *robot, pi int) {
	ps := r.procs[pi]
	r.touchQueues(pi)
	j := ps.queue[0]
	ps.queue[0] = nil
	ps.queue = ps.queue[1:]
	r.queueLen--
	j.dispatched = true
	j.tDisp = r.now
	j.robot = rb.idx
	rb.job = j
	pm := r.m.procs[pi]
	prio := pm.spec.Priority
	r.emit(Event{Type: "dispatch", JobID: jobID(j), RobotID: rb.spec.id, ProcessCode: pm.spec.Code,
		FromID: r.nodeID(rb.node), ToID: r.nodeID(j.pickup), Priority: &prio,
		Payload: &Payload{WaitS: f1(r.now - j.tArr), Queue: intPtr(len(ps.queue))}})
	r.moveTo(rb, j.pickup, false, stTravelEmpty, func() { r.atPickup(rb, j) })
}

func (r *runner) moveTo(rb *robot, dest int, loaded bool, state int, then func()) {
	if rb.node == dest {
		then()
		return
	}
	edges, ok := r.m.path(rb.class, rb.node, dest)
	if !ok {
		r.err = fmt.Errorf("no path for robot %s from %s to %s", rb.spec.id, r.nodeID(rb.node), r.nodeID(dest))
		return
	}
	segs := r.m.segments(rb.node, edges)
	r.runSegments(rb, segs, 0, loaded, state, then)
}

func (r *runner) runSegments(rb *robot, segs []segment, i int, loaded bool, state int, then func()) {
	if i == len(segs) {
		then()
		return
	}
	s := segs[i]
	next := func() { r.runSegments(rb, segs, i+1, loaded, state, then) }
	if s.group < 0 {
		if rb.held >= 0 {
			r.release(rb.held, rb)
			rb.held = -1
		}
		r.drive(rb, s, loaded, state, next)
		return
	}
	afterGroup := func() {
		if r.m.interior[s.to] != s.group {
			r.release(s.group, rb)
			rb.held = -1
		}
		next()
	}
	if rb.held == s.group {
		r.drive(rb, s, loaded, state, afterGroup)
		return
	}
	if rb.held >= 0 {
		r.release(rb.held, rb)
		rb.held = -1
	}
	r.acquire(s.group, rb, stWaitResource, func() {
		rb.held = s.group
		r.drive(rb, s, loaded, state, afterGroup)
	})
}

func (r *runner) drive(rb *robot, s segment, loaded bool, state int, then func()) {
	p := r.m.sc.Fleet[rb.spec.fleet].Profile
	speed := p.SpeedFor(loaded)
	dur := (s.length/speed + float64(s.turns)*p.TurnPenalty) * r.noise(0.05)
	r.setState(rb, state)
	var jid string
	if rb.job != nil {
		jid = jobID(rb.job)
	}
	if r.rec != nil {
		path := make([]string, len(s.nodes))
		for i, n := range s.nodes {
			path[i] = r.nodeID(n)
		}
		r.emit(Event{Type: "route_start", RobotID: rb.spec.id, JobID: jid, FromID: r.nodeID(s.from), ToID: r.nodeID(s.to),
			Payload: &Payload{Path: path, DistM: f1(s.length), DurS: f1(dur), Loaded: loaded, Battery: f3(r.batteryNow(rb))}})
	}
	r.at(r.now+dur, func() {
		rb.node = s.to
		rb.distM += s.length
		r.setState(rb, state)
		r.emit(Event{Type: "route_end", RobotID: rb.spec.id, JobID: jid, FromID: r.nodeID(s.from), ToID: r.nodeID(s.to),
			Payload: &Payload{Battery: f3(rb.battery)}})
		then()
	})
}

func (r *runner) acquire(ri int, rb *robot, waitState int, then func()) {
	rs := r.res[ri]
	r.touchRes(rs)
	if rs.inUse < rs.model.capacity && len(rs.queue) == 0 {
		rs.inUse++
		rs.acquired++
		r.emit(Event{Type: "resource_acquire", RobotID: rb.spec.id, ResourceID: rs.model.id, JobID: robotJob(rb),
			Payload: &Payload{Kind: rs.model.kind, WaitS: f1(0), Queue: intPtr(0)}})
		then()
		return
	}
	rs.queue = append(rs.queue, waiter{rb: rb, since: r.now, then: then})
	if len(rs.queue) > rs.queueMax {
		rs.queueMax = len(rs.queue)
	}
	r.setState(rb, waitState)
}

func (r *runner) release(ri int, rb *robot) {
	rs := r.res[ri]
	r.touchRes(rs)
	if rs.inUse > 0 {
		rs.inUse--
	}
	r.emit(Event{Type: "resource_release", RobotID: rb.spec.id, ResourceID: rs.model.id, JobID: robotJob(rb),
		Payload: &Payload{Kind: rs.model.kind}})
	if len(rs.queue) == 0 || rs.inUse >= rs.model.capacity {
		return
	}
	w := rs.queue[0]
	rs.queue[0] = waiter{}
	rs.queue = rs.queue[1:]
	rs.inUse++
	rs.acquired++
	wait := r.now - w.since
	rs.waitTotal += wait
	rs.waitMax = math.Max(rs.waitMax, wait)
	r.emit(Event{Type: "resource_acquire", RobotID: w.rb.spec.id, ResourceID: rs.model.id, JobID: robotJob(w.rb),
		Payload: &Payload{Kind: rs.model.kind, WaitS: f1(wait), Queue: intPtr(len(rs.queue))}})
	r.at(r.now, w.then)
}

func (r *runner) touchRes(rs *resState) {
	dt := r.now - rs.lastT
	if dt > 0 {
		rs.useArea += float64(rs.inUse) * dt
		rs.queueArea += float64(len(rs.queue)) * dt
		rs.lastT = r.now
	}
}

func (r *runner) atPickup(rb *robot, j *job) {
	pm := r.m.procs[j.proc]
	p := r.m.sc.Fleet[rb.spec.fleet].Profile
	dock := r.m.nodeDock[j.pickup]
	load := func() {
		dur := (pm.spec.StationLoadS + p.PickTime(r.m.sc.LiftM, pm.rackPick)) * r.noise(0.2)
		r.setState(rb, stHandling)
		r.emit(Event{Type: "load", RobotID: rb.spec.id, JobID: jobID(j), ProcessCode: pm.spec.Code, ToID: r.nodeID(j.pickup),
			Payload: &Payload{DurS: f1(dur)}})
		r.at(r.now+dur, func() {
			if dock >= 0 {
				r.release(dock, rb)
			}
			r.moveTo(rb, j.drop, true, stTravelLoaded, func() { r.atDrop(rb, j) })
		})
	}
	if dock >= 0 {
		r.acquire(dock, rb, stWaitResource, load)
		return
	}
	load()
}

func (r *runner) atDrop(rb *robot, j *job) {
	pm := r.m.procs[j.proc]
	p := r.m.sc.Fleet[rb.spec.fleet].Profile
	dock := r.m.nodeDock[j.drop]
	unload := func() {
		dur := (pm.spec.StationUnloadS + p.DropTime(r.m.sc.LiftM, pm.rackDrop)) * r.noise(0.2)
		r.setState(rb, stHandling)
		r.emit(Event{Type: "unload", RobotID: rb.spec.id, JobID: jobID(j), ProcessCode: pm.spec.Code, ToID: r.nodeID(j.drop),
			Payload: &Payload{DurS: f1(dur)}})
		r.at(r.now+dur, func() {
			if dock >= 0 {
				r.release(dock, rb)
			}
			r.complete(rb, j)
		})
	}
	if dock >= 0 {
		r.acquire(dock, rb, stWaitResource, unload)
		return
	}
	unload()
}

func (r *runner) complete(rb *robot, j *job) {
	j.done = true
	j.tDone = r.now
	r.completed++
	rb.jobs++
	rb.job = nil
	pm := r.m.procs[j.proc]
	if !j.violated {
		r.emit(Event{Type: "sla_reached", RobotID: rb.spec.id, JobID: jobID(j), ProcessCode: pm.spec.Code,
			Payload: &Payload{WaitS: f1(j.tDisp - j.tArr), CycleS: f1(j.tDone - j.tArr)}})
	}
	if !r.needsCharge(rb) || !r.goCharge(rb) {
		r.becomeIdle(rb)
	}
	r.dispatch()
}

func (r *runner) becomeIdle(rb *robot) {
	r.setState(rb, stIdle)
	rb.idleSince = r.now
}

// goCharge sends rb to the nearest charger, preferring a free one. It returns false when none is reachable.
func (r *runner) goCharge(rb *robot) bool {
	slot := -1
	bestFree := math.Inf(1)
	bestAny := math.Inf(1)
	anySlot := -1
	for i, ch := range r.m.chargers {
		d := r.m.reach(rb.class, rb.node, ch.node)
		if math.IsInf(d, 1) {
			continue
		}
		rs := r.res[ch.res]
		if rs.inUse < rs.model.capacity && len(rs.queue) == 0 && d < bestFree {
			bestFree = d
			slot = i
		}
		if d < bestAny {
			bestAny = d
			anySlot = i
		}
	}
	if slot < 0 {
		slot = anySlot
	}
	if slot < 0 {
		return false
	}
	ch := r.m.chargers[slot]
	r.setState(rb, stToCharger)
	r.moveTo(rb, ch.node, false, stToCharger, func() {
		r.acquire(ch.res, rb, stWaitCharger, func() {
			p := r.m.sc.Fleet[rb.spec.fleet].Profile
			r.setState(rb, stCharging)
			b := rb.battery
			r.emit(Event{Type: "charge_start", RobotID: rb.spec.id, ResourceID: r.res[ch.res].model.id, ToID: r.nodeID(ch.node),
				Payload: &Payload{Battery: f3(b)}})
			dur := 0.0
			if rate := p.ChargePerS(); rate > 0 && p.ChargeTo > b {
				dur = (p.ChargeTo - b) / rate
			}
			r.at(r.now+dur, func() {
				r.setState(rb, stCharging)
				rb.battery = math.Max(rb.battery, p.ChargeTo)
				r.emit(Event{Type: "charge_end", RobotID: rb.spec.id, ResourceID: r.res[ch.res].model.id, ToID: r.nodeID(ch.node),
					Payload: &Payload{Battery: f3(rb.battery), DurS: f1(dur)}})
				r.release(ch.res, rb)
				r.becomeIdle(rb)
				r.dispatch()
			})
		})
	})
	return true
}

func (r *runner) checkpoint() {
	cp := Checkpoint{T: f1v(r.now), Queue: r.queueLen, Completed: r.completed, Violated: r.violated}
	for _, rb := range r.robots {
		switch rb.state {
		case stIdle:
			cp.Idle++
		case stToCharger, stWaitCharger, stCharging:
			cp.Charging++
		default:
			cp.Busy++
		}
	}
	for _, rs := range r.res {
		cp.Resources = append(cp.Resources, ResourceLoad{ID: rs.model.id, InUse: rs.inUse, Queue: len(rs.queue)})
	}
	for _, ps := range r.procs {
		cp.Processes = append(cp.Processes, len(ps.queue))
	}
	r.rec.checkpoints = append(r.rec.checkpoints, cp)
	r.ckpts++
	next := r.now + checkpointEveryS
	if next <= r.horizon {
		r.at(next, r.checkpoint)
	}
}

func (r *runner) nodeID(n int) string {
	if n < 0 || n >= len(r.m.sc.Graph.Nodes) {
		return ""
	}
	return r.m.sc.Graph.Nodes[n].ID
}

func jobID(j *job) string {
	if j == nil || j.idx < 0 {
		return ""
	}
	return fmt.Sprintf("J%d", j.idx+1)
}

func robotJob(rb *robot) string {
	return jobID(rb.job)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func sortedFloats(v []float64) []float64 {
	out := append([]float64(nil), v...)
	sort.Float64s(out)
	return out
}
