package geom

import "math/rand"

const (
	stIdle   = "idle"
	stTravel = "travel"
	stQueue  = "queue"
	stOp     = "op"
	stCharge = "charge"
)

type pool struct {
	cap  int
	used int
	wait []int
	pos  []Point
}

func (p *pool) acquire(id int) bool {
	if p.used < p.cap {
		p.used++
		return true
	}
	p.wait = append(p.wait, id)
	return false
}

func (p *pool) release() {
	if p.used > 0 {
		p.used--
	}
}

type bot struct {
	id         int
	x, y       float64
	state      string
	phase      int
	tLeft      float64
	travelS    float64
	from, to   Point
	prog       float64
	cycleStart float64
	completed  int
	cycleSum   float64
	queueSum   float64
	chargeSum  float64
	workSum    float64
	enduranceS float64
	chargeNeed float64
}

func runTransport(in Input, lay Layout, pathM, handleS float64, loadN, unloadN int) Summary {
	sum := emptySummary(in)
	if handleS <= 0 {
		handleS = HandlePalletS
	}
	leg := legTravelS(pathM, in.SpeedMps, handleS, in.Throughput)
	if leg < 1 {
		leg = 1
	}
	half := handleS / 2
	if half < 1 {
		half = 1
	}

	loads := nodesOf(lay.OpPoints, "load")
	unloads := nodesOf(lay.OpPoints, "unload")
	if len(loads) == 0 {
		loads = []Point{{X: 20, Y: 40}}
	}
	if len(unloads) == 0 {
		unloads = []Point{{X: 140, Y: 40}}
	}
	loadPool := pool{cap: loadN, pos: loads}
	unloadPool := pool{cap: unloadN, pos: unloads}
	chargers := chargerPoints(lay)

	rng := rand.New(rand.NewSource(in.Seed))
	n := in.FleetSize
	bots := make([]*bot, n)
	for i := 0; i < n; i++ {
		ch := chargers[i%len(chargers)]
		b := &bot{
			id:         i,
			x:          ch.X + (rng.Float64()-0.5)*2,
			y:          ch.Y + (rng.Float64()-0.5)*2,
			state:      stIdle,
			enduranceS: in.EnduranceH * 3600,
			chargeNeed: in.ChargeMin * 60,
		}
		bots[i] = b
		startLoadTravel(b, loads[i%len(loads)], leg, 0)
	}

	now := 0.0
	for now < HorizonS {
		dt := TickS
		if now+dt > HorizonS {
			dt = HorizonS - now
		}
		now += dt
		stepTransport(bots, &loadPool, &unloadPool, loads, unloads, chargers, leg, half, now, dt)
	}
	fillTransportSummary(&sum, in, bots, now)
	return sum
}

func startLoadTravel(b *bot, dest Point, leg, now float64) {
	b.phase = 0
	b.state = stTravel
	b.from = Point{X: b.x, Y: b.y}
	b.to = dest
	b.prog = 0
	b.travelS = leg
	b.cycleStart = now
}

func stepTransport(bots []*bot, loadP, unloadP *pool, loads, unloads, chargers []Point, leg, half, now, dt float64) {
	for _, b := range bots {
		if b.state == stQueue {
			b.queueSum += dt
			b.workSum += dt
			continue
		}
		if b.state == stCharge {
			b.chargeSum += dt
			b.tLeft -= dt
			if b.tLeft <= 0 {
				b.workSum = 0
				startLoadTravel(b, loads[b.id%len(loads)], leg, now)
			}
			continue
		}
		if b.state == stOp {
			b.workSum += dt
			b.tLeft -= dt
			if b.tLeft > 0 {
				continue
			}
			if b.phase == 1 {
				loadP.release()
				b.phase = 2
				b.state = stTravel
				b.from = Point{X: b.x, Y: b.y}
				b.to = unloads[b.id%len(unloads)]
				b.prog = 0
				b.travelS = leg
				continue
			}
			unloadP.release()
			b.completed++
			b.cycleSum += now - b.cycleStart
			if b.enduranceS > 0 && b.workSum >= b.enduranceS && b.chargeNeed > 0 {
				ch := chargers[b.id%len(chargers)]
				b.state = stTravel
				b.phase = 4
				b.from = Point{X: b.x, Y: b.y}
				b.to = ch
				b.prog = 0
				b.travelS = mathMax(leg*0.4, 8)
				continue
			}
			startLoadTravel(b, loads[b.id%len(loads)], leg, now)
			continue
		}
		if b.state != stTravel {
			continue
		}
		b.workSum += dt
		b.prog += dt / b.travelS
		if b.prog >= 1 {
			b.prog = 1
			b.x = b.to.X
			b.y = b.to.Y
			if b.phase == 4 {
				b.state = stCharge
				b.tLeft = b.chargeNeed
				if b.tLeft < 1 {
					b.tLeft = 1
				}
				continue
			}
			if b.phase == 0 {
				if loadP.acquire(b.id) {
					beginOp(b, b.to, half)
					b.phase = 1
				} else {
					b.state = stQueue
				}
				continue
			}
			if b.phase == 2 {
				if unloadP.acquire(b.id) {
					beginOp(b, b.to, half)
					b.phase = 3
				} else {
					b.state = stQueue
				}
			}
			continue
		}
		b.x = b.from.X + (b.to.X-b.from.X)*b.prog
		b.y = b.from.Y + (b.to.Y-b.from.Y)*b.prog
	}

	drainQueue(bots, loadP, half, true)
	drainQueue(bots, unloadP, half, false)
}

func drainQueue(bots []*bot, p *pool, half float64, load bool) {
	for p.used < p.cap && len(p.wait) > 0 {
		id := p.wait[0]
		p.wait = p.wait[1:]
		if id < 0 || id >= len(bots) {
			continue
		}
		p.used++
		b := bots[id]
		if load {
			b.phase = 1
		} else {
			b.phase = 3
		}
		beginOp(b, b.to, half)
	}
}

func beginOp(b *bot, at Point, half float64) {
	b.state = stOp
	b.tLeft = half
	b.x = at.X
	b.y = at.Y
}

func fillTransportSummary(sum *Summary, in Input, bots []*bot, now float64) {
	sum.FleetSize = len(bots)
	sum.EconThroughput = round4(in.Throughput)
	completed := 0
	cycleSum := 0.0
	queueSum := 0.0
	chargeSum := 0.0
	for _, b := range bots {
		completed += b.completed
		cycleSum += b.cycleSum
		queueSum += b.queueSum
		chargeSum += b.chargeSum
	}
	simTh := 0.0
	meanCycle := 0.0
	if completed > 0 && cycleSum > 0 {
		meanCycle = cycleSum / float64(completed)
		simTh = 3600 / meanCycle
	}
	sum.Throughput = round4(simTh)
	div := 0.0
	if in.Throughput > 0 {
		div = abs(simTh-in.Throughput) / in.Throughput
	}
	sum.Divergence = round4(div)
	sum.VerificationFlag = in.Throughput > 0 && div > DivergenceMax
	if now > 0 {
		sum.DeliveredOpsH = round4(float64(completed) / (now / 3600))
	}
	if completed > 0 {
		sum.QueueWaitS = round4(queueSum / float64(completed))
	}
	if len(bots) > 0 && now > 0 {
		sum.ChargeShare = round4(chargeSum / (float64(len(bots)) * now))
	}
	sum.Bottleneck = "none"
	if meanCycle > 0 && sum.QueueWaitS > 0.08*meanCycle {
		sum.Bottleneck = "op_points"
	} else if sum.ChargeShare > 0.10 {
		sum.Bottleneck = "chargers"
	}
	if in.LoadSlots == 1 || in.UnloadSlots == 1 {
		sum.Assumptions = append(sum.Assumptions, "Точки операций ограничены для проверки очереди.")
	}
}

func runCleaner(in Input, lay Layout, m map[string]any) Summary {
	sum := emptySummary(in)
	sum.FleetSize = in.FleetSize
	sum.EconThroughput = round4(in.Throughput)
	zone := activeZone(lay, in.ObjectType)
	width := cleanerWidthM(in.WidthMm)
	speed := in.SpeedMps
	rate := 0.0
	if speed > 0 {
		rate = width * speed * 3600 * CleanerOverlap
	} else {
		rate = in.Throughput
		if speed <= 0 && in.Throughput > 0 {
			speed = 1.0
		}
	}
	n := in.FleetSize
	bots := make([]*bot, n)
	rng := rand.New(rand.NewSource(in.Seed))
	for i := 0; i < n; i++ {
		y := zone.Y + 4 + float64(i+1)*zone.H/float64(n+2)
		x0 := zone.X + 4
		bots[i] = &bot{
			id: i, x: x0 + rng.Float64()*6, y: y, state: stTravel,
			from: Point{X: x0, Y: y}, to: Point{X: zone.X + zone.W - 4, Y: y},
			travelS: mathMax((zone.W-8)/mathMax(speed, 0.3), 8),
		}
	}
	covered := 0.0
	now := 0.0
	perSec := rate / 3600
	for now < HorizonS {
		dt := TickS
		if now+dt > HorizonS {
			dt = HorizonS - now
		}
		now += dt
		covered += perSec * dt * float64(n)
		for _, b := range bots {
			b.prog += dt / b.travelS
			if b.prog >= 1 {
				b.x, b.y = b.to.X, b.to.Y
				b.from, b.to = b.to, b.from
				b.prog = 0
			} else {
				b.x = b.from.X + (b.to.X-b.from.X)*b.prog
				b.y = b.from.Y + (b.to.Y-b.from.Y)*b.prog
			}
		}
	}
	simTh := 0.0
	if n > 0 && now > 0 {
		simTh = covered / (now / 3600) / float64(n)
	}
	sum.Throughput = round4(simTh)
	div := 0.0
	if in.Throughput > 0 {
		div = abs(simTh-in.Throughput) / in.Throughput
	}
	sum.Divergence = round4(div)
	sum.VerificationFlag = in.Throughput > 0 && div > DivergenceMax
	if now > 0 {
		sum.DeliveredOpsH = round4(covered / (now / 3600))
	}
	sum.Bottleneck = "none"
	_ = m
	return sum
}

func emptySummary(in Input) Summary {
	return Summary{
		FleetSize:      in.FleetSize,
		EconThroughput: round4(in.Throughput),
		Bottleneck:     "none",
		Assumptions:    []string{},
	}
}

func nodesOf(nodes []Node, kind string) []Point {
	out := []Point{}
	for _, n := range nodes {
		if n.Kind == kind {
			out = append(out, Point{X: n.X, Y: n.Y})
		}
	}
	return out
}

func chargerPoints(lay Layout) []Point {
	out := []Point{}
	for _, n := range lay.Chargers {
		out = append(out, Point{X: n.X, Y: n.Y})
	}
	if len(out) == 0 {
		out = []Point{{X: 80, Y: 80}}
	}
	return out
}

func activeZone(lay Layout, objectType string) Zone {
	want := "storage"
	if objectType == "airport" {
		want = "terminal"
	}
	if objectType == "hospital" {
		want = "floor"
	}
	for _, z := range lay.Zones {
		if z.Kind == want {
			return z
		}
	}
	if len(lay.Zones) > 0 {
		return lay.Zones[0]
	}
	return Zone{X: 10, Y: 10, W: 80, H: 40}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
