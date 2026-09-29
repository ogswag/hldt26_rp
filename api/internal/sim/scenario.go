package sim

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/rutext"
	"moscow_hackathon_2026/api/internal/sim/profiles"
)

// ProcessSpec is one demand stream with its SLA and station times.
type ProcessSpec struct {
	Code           string   `json:"code"`
	Name           string   `json:"name"`
	TaskType       string   `json:"task_type"`
	Unit           string   `json:"unit,omitempty"`
	UnitsPerDay    float64  `json:"units_per_day"`
	UnitsPerJob    float64  `json:"units_per_job"`
	MaxWaitS       float64  `json:"max_wait_s"`
	MaxCycleS      float64  `json:"max_cycle_s"`
	Priority       int      `json:"priority"`
	StationLoadS   float64  `json:"station_load_s"`
	StationUnloadS float64  `json:"station_unload_s"`
	Pickups        []string `json:"pickups"`
	Drops          []string `json:"drops"`
}

// FleetItem is one fleet line of a variant.
type FleetItem struct {
	Key        string           `json:"key"`
	SolutionID string           `json:"solution_id"`
	Name       string           `json:"name"`
	Profile    profiles.Profile `json:"profile"`
	Quantity   int              `json:"quantity"`
	Serves     []string         `json:"serves,omitempty"`
}

// Scenario is everything one simulation run needs.
type Scenario struct {
	Config            Config
	Graph             *maps.Graph
	Scene             maps.Scene
	Resources         []maps.Resource
	Processes         []ProcessSpec
	Fleet             []FleetItem
	ActiveHoursPerDay float64
	PeakFactor        float64
	Windows           []Window
	LiftM             float64
}

type class struct {
	key     string
	label   string
	reqW    float64
	profile profiles.Profile
	dist    map[int][]float64
	prev    map[int][]int
}

type robotSpec struct {
	id    string
	name  string
	fleet int
	class int
	start int
}

type procModel struct {
	spec     ProcessSpec
	pickups  []int
	drops    []int
	rackPick bool
	rackDrop bool
	covered  bool
	segs     []arrivalSeg
	total    float64
}

type resModel struct {
	id       string
	kind     string
	name     string
	capacity int
	auto     bool
	nodes    []string
	edges    []string
}

type segment struct {
	from   int
	to     int
	edges  []int
	nodes  []int
	length float64
	turns  int
	group  int
}

// Model is a prepared, read-only scenario shared by all replications.
type Model struct {
	sc          Scenario
	cfg         Config
	horizonS    float64
	classes     []*class
	fleetClass  []int
	robots      []robotSpec
	procs       []*procModel
	res         []resModel
	edgeGroup   []int
	interior    []int
	nodeDock    []int
	chargers    []chargerSlot
	battery     bool
	canServe    [][]bool
	warnings    []string
	assumptions []string
}

type chargerSlot struct {
	node int
	res  int
}

// Prepare validates a scenario against the engine rules and precomputes routes.
func Prepare(sc Scenario) (*Model, error) {
	cfg, err := sc.Config.Normalize()
	if err != nil {
		return nil, err
	}
	if sc.Graph == nil || len(sc.Graph.Nodes) == 0 {
		return nil, inputErr("Карта не содержит точек. Разметьте карту или используйте шаблон.")
	}
	m := &Model{sc: sc, cfg: cfg, horizonS: cfg.HorizonH * 3600}
	if err := m.buildFleet(); err != nil {
		return nil, err
	}
	if err := m.buildProcesses(); err != nil {
		return nil, err
	}
	m.buildResources()
	m.buildRoutes()
	if err := m.checkCoverage(); err != nil {
		return nil, err
	}
	m.buildArrivals()
	m.pickStarts()
	m.collectAssumptions()
	return m, nil
}

func (m *Model) buildFleet() error {
	total := 0
	byKey := map[string]int{}
	for fi, item := range m.sc.Fleet {
		if item.Quantity < 1 {
			return inputErr("Позиция флота %s: количество должно быть не меньше 1.", item.Name)
		}
		total += item.Quantity
		if total > MaxRobots {
			return inputErr("Во флоте больше %d роботов. Уменьшите количество.", MaxRobots)
		}
		p := item.Profile
		if p.SpeedMps <= 0 || p.WidthM <= 0 {
			return inputErr("Позиция флота %s: не заданы скорость или ширина робота.", item.Name)
		}
		key := fmt.Sprintf("%s|%.3f", p.Code, p.RequiredWidthM())
		ci, ok := byKey[key]
		if !ok {
			ci = len(m.classes)
			byKey[key] = ci
			m.classes = append(m.classes, &class{
				key:     key,
				label:   fmt.Sprintf("%s %s м", p.Label, rutext.Num(p.WidthM, 2)),
				reqW:    p.RequiredWidthM(),
				profile: p,
				dist:    map[int][]float64{},
				prev:    map[int][]int{},
			})
		}
		m.fleetClass = append(m.fleetClass, ci)
		for k := 0; k < item.Quantity; k++ {
			m.robots = append(m.robots, robotSpec{
				id:    fmt.Sprintf("R%d", len(m.robots)+1),
				name:  item.Name,
				fleet: fi,
				class: ci,
			})
		}
	}
	if len(m.robots) == 0 {
		return inputErr("В варианте нет роботов. Добавьте позиции флота.")
	}
	m.battery = true
	for _, item := range m.sc.Fleet {
		if item.Profile.EnduranceH <= 0 || item.Profile.ChargeMin <= 0 {
			m.battery = false
		}
	}
	return nil
}

func (m *Model) buildProcesses() error {
	g := m.sc.Graph
	for _, ps := range m.sc.Processes {
		pm := &procModel{spec: ps}
		if ps.UnitsPerJob <= 0 {
			pm.spec.UnitsPerJob = 1
		}
		for _, list := range []struct {
			ids []string
			dst *[]int
		}{{ps.Pickups, &pm.pickups}, {ps.Drops, &pm.drops}} {
			for _, id := range list.ids {
				i, ok := g.NodeIndex(id)
				if !ok {
					return inputErr("Процесс %s ссылается на точку %s, которой нет на карте.", ps.Code, id)
				}
				*list.dst = append(*list.dst, i)
			}
		}
		pm.rackPick = ps.TaskType == profiles.TaskPalletOutbound
		pm.rackDrop = ps.TaskType == profiles.TaskPalletPutaway
		m.procs = append(m.procs, pm)
	}
	return nil
}

func (m *Model) buildResources() {
	g := m.sc.Graph
	m.edgeGroup = make([]int, len(g.Edges))
	for i := range m.edgeGroup {
		m.edgeGroup[i] = -1
	}
	m.nodeDock = make([]int, len(g.Nodes))
	for i := range m.nodeDock {
		m.nodeDock[i] = -1
	}
	for _, r := range m.sc.Resources {
		ri := len(m.res)
		rm := resModel{id: r.ID, kind: r.Kind, name: r.Name, capacity: r.Capacity, nodes: r.Points(), edges: r.EdgeIDs}
		if rm.name == "" {
			rm.name = r.ID
		}
		if rm.capacity < 1 {
			rm.capacity = 1
		}
		m.res = append(m.res, rm)
		switch r.Kind {
		case maps.ResourceDock:
			for _, id := range rm.nodes {
				if n, ok := g.NodeIndex(id); ok {
					m.nodeDock[n] = ri
				}
			}
		case maps.ResourceCharger:
			for _, id := range rm.nodes {
				if n, ok := g.NodeIndex(id); ok {
					m.chargers = append(m.chargers, chargerSlot{node: n, res: ri})
				}
			}
		case maps.ResourceNarrowAisle:
			for _, id := range rm.edges {
				if e, ok := g.EdgeIndex(id); ok {
					m.edgeGroup[e] = ri
				}
			}
		}
	}
	m.autoNarrow()
	m.interior = make([]int, len(g.Nodes))
	for n := range m.interior {
		m.interior[n] = -1
	}
	incident := make([][]int, len(g.Nodes))
	for ei, e := range g.Edges {
		incident[e.A] = append(incident[e.A], ei)
		incident[e.B] = append(incident[e.B], ei)
	}
	for n, list := range incident {
		if len(list) == 0 {
			continue
		}
		grp := m.edgeGroup[list[0]]
		same := grp >= 0
		for _, ei := range list[1:] {
			if m.edgeGroup[ei] != grp {
				same = false
				break
			}
		}
		if same {
			m.interior[n] = grp
		}
	}
	if len(m.chargers) == 0 && m.battery {
		m.battery = false
		m.warnings = append(m.warnings, "На карте нет зарядной станции: разряд батареи не моделируется.")
	}
}

// autoNarrow groups single-lane corridors into capacity-1 resources.
func (m *Model) autoNarrow() {
	g := m.sc.Graph
	widest := 0.0
	for _, c := range m.classes {
		widest = math.Max(widest, maps.TwoLaneWidthM(c.profile.WidthM, c.profile.ClearanceM))
	}
	degree := make([]int, len(g.Nodes))
	for _, e := range g.Edges {
		degree[e.A]++
		degree[e.B]++
	}
	single := make([]bool, len(g.Edges))
	for ei, e := range g.Edges {
		if m.edgeGroup[ei] >= 0 {
			continue
		}
		passable := false
		for _, c := range m.classes {
			if e.WidthM+1e-9 >= c.reqW {
				passable = true
				break
			}
		}
		if passable && e.WidthM < widest {
			single[ei] = true
		}
	}
	parent := make([]int, len(g.Edges))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	byNode := make([][]int, len(g.Nodes))
	for ei, e := range g.Edges {
		if single[ei] {
			byNode[e.A] = append(byNode[e.A], ei)
			byNode[e.B] = append(byNode[e.B], ei)
		}
	}
	for n, list := range byNode {
		if degree[n] == 2 && len(list) == 2 {
			a, b := find(list[0]), find(list[1])
			if a != b {
				parent[a] = b
			}
		}
	}
	groups := map[int]int{}
	for ei := range g.Edges {
		if !single[ei] {
			continue
		}
		root := find(ei)
		ri, ok := groups[root]
		if !ok {
			ri = len(m.res)
			groups[root] = ri
			m.res = append(m.res, resModel{
				id:       "auto-" + g.Edges[root].ID,
				kind:     maps.ResourceNarrowAisle,
				name:     "Однополосный участок " + g.Edges[root].ID,
				capacity: 1,
				auto:     true,
			})
		}
		m.edgeGroup[ei] = ri
		m.res[ri].edges = append(m.res[ri].edges, g.Edges[ei].ID)
	}
}

func (m *Model) sources() []int {
	seen := map[int]bool{}
	var out []int
	add := func(n int) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, p := range m.procs {
		for _, n := range p.pickups {
			add(n)
		}
		for _, n := range p.drops {
			add(n)
		}
	}
	for _, c := range m.chargers {
		add(c.node)
	}
	sort.Ints(out)
	return out
}

func (m *Model) buildRoutes() {
	srcs := m.sources()
	for _, c := range m.classes {
		for _, s := range srcs {
			d, p := m.sc.Graph.ShortestFrom(s, c.reqW)
			c.dist[s] = d
			c.prev[s] = p
		}
	}
}

func (m *Model) ensureSource(n int) {
	for _, c := range m.classes {
		if _, ok := c.dist[n]; ok {
			continue
		}
		d, p := m.sc.Graph.ShortestFrom(n, c.reqW)
		c.dist[n] = d
		c.prev[n] = p
	}
}

func (m *Model) reach(ci, from, to int) float64 {
	d, ok := m.classes[ci].dist[from]
	if !ok {
		return math.Inf(1)
	}
	return d[to]
}

func serves(item FleetItem, p ProcessSpec) bool {
	if !item.Profile.Can(p.TaskType) {
		return false
	}
	if len(item.Serves) == 0 {
		return true
	}
	for _, c := range item.Serves {
		if c == p.Code || c == p.TaskType {
			return true
		}
	}
	return false
}

func (m *Model) checkCoverage() error {
	m.canServe = make([][]bool, len(m.sc.Fleet))
	for fi := range m.sc.Fleet {
		m.canServe[fi] = make([]bool, len(m.procs))
	}
	covered := 0
	for pi, p := range m.procs {
		if len(p.pickups) == 0 || len(p.drops) == 0 {
			m.warnings = append(m.warnings, fmt.Sprintf("Процесс %s не привязан к точкам карты и не моделируется.", p.spec.Name))
			continue
		}
		if p.spec.UnitsPerDay <= 0 {
			m.warnings = append(m.warnings, fmt.Sprintf("У процесса %s нулевой спрос, он не моделируется.", p.spec.Name))
			continue
		}
		var blocked []string
		for fi, item := range m.sc.Fleet {
			if !serves(item, p.spec) {
				continue
			}
			ci := m.fleetClass[fi]
			ok := true
			for _, a := range p.pickups {
				for _, b := range p.drops {
					if math.IsInf(m.reach(ci, a, b), 1) || math.IsInf(m.reach(ci, b, a), 1) {
						ok = false
					}
				}
			}
			if !ok {
				blocked = append(blocked, item.Name)
				continue
			}
			m.canServe[fi][pi] = true
			p.covered = true
		}
		if p.covered {
			covered++
			continue
		}
		if len(blocked) > 0 {
			m.warnings = append(m.warnings, fmt.Sprintf("Процесс %s: роботы %s не проходят между точками процесса. Процесс не моделируется.", p.spec.Name, strings.Join(blocked, ", ")))
		} else {
			m.warnings = append(m.warnings, fmt.Sprintf("Процесс %s: во флоте нет подходящих роботов. Процесс не моделируется.", p.spec.Name))
		}
	}
	if covered == 0 {
		return inputErr("Ни один процесс не обслуживается флотом варианта. Проверьте задачи роботов, потоки на карте и ширину проходов.")
	}
	if m.battery {
		for fi, item := range m.sc.Fleet {
			ci := m.fleetClass[fi]
			for pi, p := range m.procs {
				if !m.canServe[fi][pi] {
					continue
				}
				for _, n := range append(append([]int{}, p.pickups...), p.drops...) {
					found := false
					for _, ch := range m.chargers {
						if !math.IsInf(m.reach(ci, n, ch.node), 1) {
							found = true
							break
						}
					}
					if !found {
						return inputErr("Робот %s не может доехать до зарядки от точки %s. Проверьте рёбра к зарядной станции.", item.Name, m.sc.Graph.Nodes[n].ID)
					}
				}
			}
		}
	}
	return nil
}

func (m *Model) pickStarts() {
	var candidates []int
	for _, c := range m.chargers {
		if m.interior[c.node] < 0 {
			candidates = append(candidates, c.node)
		}
	}
	if len(candidates) == 0 {
		for _, p := range m.procs {
			if !p.covered {
				continue
			}
			for _, n := range append(append([]int{}, p.pickups...), p.drops...) {
				if m.interior[n] < 0 {
					candidates = append(candidates, n)
				}
			}
		}
	}
	for i := range m.robots {
		ci := m.robots[i].class
		start := -1
		for k := 0; k < len(candidates); k++ {
			n := candidates[(i+k)%len(candidates)]
			if m.classReaches(ci, n) {
				start = n
				break
			}
		}
		if start < 0 {
			start = m.anyServedNode(m.robots[i].fleet)
		}
		m.robots[i].start = start
		m.ensureSource(start)
	}
}

func (m *Model) classReaches(ci, n int) bool {
	for fi, cls := range m.fleetClass {
		if cls != ci {
			continue
		}
		for pi, p := range m.procs {
			if m.canServe[fi][pi] && !math.IsInf(m.reach(ci, p.pickups[0], n), 1) &&
				!math.IsInf(m.reach(ci, n, p.pickups[0]), 1) {
				return true
			}
		}
	}
	return false
}

func (m *Model) anyServedNode(fi int) int {
	for pi, p := range m.procs {
		if m.canServe[fi][pi] {
			return p.pickups[0]
		}
	}
	for _, p := range m.procs {
		if len(p.pickups) > 0 {
			return p.pickups[0]
		}
	}
	return 0
}

func (m *Model) collectAssumptions() {
	a := []string{
		"Узлы графа вмещают любое число роботов, конфликты учитываются только на узких участках, доках и зарядке.",
		"Путь выбирается по кратчайшему расстоянию без учёта загрузки участков.",
		"Спрос в час = спрос в сутки / единиц в задании / активные часы суток x профиль часа.",
		"Для незавершённых заданий ожидание считается до конца горизонта.",
		"Участок однополосный, если он уже 2 x ширина + 1,5 x зазор самого широкого робота.",
	}
	if m.cfg.stochastic() {
		a = append(a,
			fmt.Sprintf("Поступление заданий пуассоновское, %d повторов.", m.cfg.Replications),
			"Длительности операций логнормальные (разброс 20%), скорости с разбросом 5%.",
		)
	} else {
		a = append(a, "Детерминированный режим: задания приходят через равные интервалы, длительности фиксированы.")
	}
	a = append(a, fmt.Sprintf("SLA выполнен, если 90-й перцентиль доли нарушений по повторам не выше %s.", rutext.Pct(m.cfg.targetPct(), 1)))
	for _, item := range m.sc.Fleet {
		if len(item.Profile.Assumed) > 0 {
			a = append(a, fmt.Sprintf("%s: значения класса %s приняты для %s.", item.Name, item.Profile.Label, strings.Join(item.Profile.Assumed, ", ")))
		}
	}
	for _, p := range m.procs {
		if p.covered && p.spec.UnitsPerJob > 1 {
			a = append(a, fmt.Sprintf("%s: одно задание перевозит %s %s.", p.spec.Name, rutext.Num(p.spec.UnitsPerJob, 0), unitOr(p.spec.Unit)))
		}
	}
	m.assumptions = a
}

func unitOr(u string) string {
	if strings.TrimSpace(u) == "" {
		return "ед"
	}
	return u
}

// Warnings lists scenario problems that lower confidence but do not block the run.
func (m *Model) Warnings() []string {
	return append([]string(nil), m.warnings...)
}

// Replications is the normalized number of repeats.
func (m *Model) Replications() int {
	return m.cfg.Replications
}

// Config returns the normalized configuration.
func (m *Model) Config() Config {
	return m.cfg
}

// segments splits a path into runs of edges that share a narrow group.
func (m *Model) segments(from int, edges []int) []segment {
	g := m.sc.Graph
	var out []segment
	cur := from
	for i := 0; i < len(edges); {
		grp := m.edgeGroup[edges[i]]
		s := segment{from: cur, group: grp, nodes: []int{cur}}
		for i < len(edges) && m.edgeGroup[edges[i]] == grp {
			ei := edges[i]
			next := g.Other(ei, cur)
			s.edges = append(s.edges, ei)
			s.nodes = append(s.nodes, next)
			s.length += g.Edges[ei].LengthM
			cur = next
			i++
		}
		s.to = cur
		s.turns = turnCount(g, s.nodes)
		out = append(out, s)
	}
	return out
}

func turnCount(g *maps.Graph, nodes []int) int {
	n := 0
	for i := 1; i+1 < len(nodes); i++ {
		a, b, c := g.Nodes[nodes[i-1]], g.Nodes[nodes[i]], g.Nodes[nodes[i+1]]
		ux, uy := b.X-a.X, b.Y-a.Y
		vx, vy := c.X-b.X, c.Y-b.Y
		lu := math.Hypot(ux, uy)
		lv := math.Hypot(vx, vy)
		if lu == 0 || lv == 0 {
			continue
		}
		if (ux*vx+uy*vy)/(lu*lv) < math.Cos(30*math.Pi/180) {
			n++
		}
	}
	return n
}

func (m *Model) path(ci, from, to int) ([]int, bool) {
	if from == to {
		return nil, true
	}
	prev, ok := m.classes[ci].prev[from]
	if !ok {
		return nil, false
	}
	edges := m.sc.Graph.PathEdges(prev, from, to)
	if edges == nil {
		return nil, false
	}
	return edges, true
}
