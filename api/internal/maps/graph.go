package maps

import (
	"container/heap"
	"math"
)

const (
	DefaultAisleWidthM = 3.0
	wallProbeM         = 8.0
)

type Node struct {
	ID   string  `json:"id"`
	Kind string  `json:"kind"`
	Name string  `json:"name,omitempty"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

type GraphEdge struct {
	ID             string   `json:"id"`
	A              int      `json:"-"`
	B              int      `json:"-"`
	From           string   `json:"from"`
	To             string   `json:"to"`
	LengthM        float64  `json:"length_m"`
	WidthM         float64  `json:"width_m"`
	DeclaredWidthM *float64 `json:"declared_width_m,omitempty"`
	MeasuredWidthM *float64 `json:"measured_width_m,omitempty"`
	TwoWay         bool     `json:"two_way"`
}

type arc struct {
	edge int
	to   int
}

// Graph is the route graph in meters.
type Graph struct {
	Nodes []Node
	Edges []GraphEdge
	nodes map[string]int
	edges map[string]int
	out   [][]arc
}

type ScenePolygon struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
	Ring []XY   `json:"ring"`
}

// Scene is the static drawing of a map in meters.
type Scene struct {
	WidthM    float64        `json:"width_m"`
	HeightM   float64        `json:"height_m"`
	Zones     []ScenePolygon `json:"zones"`
	Obstacles []ScenePolygon `json:"obstacles"`
}

func (g *Graph) NodeIndex(id string) (int, bool) {
	i, ok := g.nodes[id]
	return i, ok
}

func (g *Graph) EdgeIndex(id string) (int, bool) {
	i, ok := g.edges[id]
	return i, ok
}

func walls(d Document) [][2]XY {
	var out [][2]XY
	for _, o := range d.Layers.Obstacles {
		out = append(out, ringWalls(d.ringMeters(o.Ring))...)
	}
	for _, z := range d.Layers.Zones {
		if z.Kind == ZoneWorkspace {
			out = append(out, ringWalls(d.ringMeters(z.Ring))...)
		}
	}
	return out
}

// Build converts a structurally valid document into a route graph and a scene.
// Edge width is the declared width capped by the free corridor measured against obstacles.
func Build(d Document, defaultWidthM float64) (*Graph, Scene) {
	if defaultWidthM <= 0 {
		defaultWidthM = DefaultAisleWidthM
	}
	g := &Graph{
		nodes: make(map[string]int, len(d.Layers.Points)),
		edges: make(map[string]int, len(d.Layers.Edges)),
	}
	for _, p := range d.Layers.Points {
		if _, dup := g.nodes[p.ID]; dup {
			continue
		}
		m := d.toMeters(p.X, p.Y)
		g.nodes[p.ID] = len(g.Nodes)
		g.Nodes = append(g.Nodes, Node{ID: p.ID, Kind: p.Kind, Name: p.Name, X: m.X, Y: m.Y})
	}
	g.out = make([][]arc, len(g.Nodes))
	ws := walls(d)
	for _, e := range d.Layers.Edges {
		a, okA := g.nodes[e.From]
		b, okB := g.nodes[e.To]
		if !okA || !okB || a == b {
			continue
		}
		if _, dup := g.edges[e.ID]; dup {
			continue
		}
		pa := XY{X: g.Nodes[a].X, Y: g.Nodes[a].Y}
		pb := XY{X: g.Nodes[b].X, Y: g.Nodes[b].Y}
		ge := GraphEdge{
			ID:      e.ID,
			A:       a,
			B:       b,
			From:    e.From,
			To:      e.To,
			LengthM: dist(pa, pb),
			TwoWay:  e.TwoWay(),
		}
		width := defaultWidthM
		if e.WidthM != nil {
			v := *e.WidthM
			ge.DeclaredWidthM = &v
			width = v
		}
		if len(ws) > 0 {
			measured := clearWidth(pa, pb, ws, wallProbeM)
			if measured < 2*wallProbeM {
				mv := round3(measured)
				ge.MeasuredWidthM = &mv
				if e.WidthM == nil || measured < width {
					width = measured
				}
			}
		}
		ge.WidthM = round3(width)
		idx := len(g.Edges)
		g.edges[e.ID] = idx
		g.Edges = append(g.Edges, ge)
		g.out[a] = append(g.out[a], arc{edge: idx, to: b})
		if ge.TwoWay {
			g.out[b] = append(g.out[b], arc{edge: idx, to: a})
		}
	}
	return g, buildScene(d, g)
}

func buildScene(d Document, g *Graph) Scene {
	s := Scene{Zones: []ScenePolygon{}, Obstacles: []ScenePolygon{}}
	maxX, maxY := 0.0, 0.0
	grow := func(p XY) {
		maxX = math.Max(maxX, p.X)
		maxY = math.Max(maxY, p.Y)
	}
	for _, z := range d.Layers.Zones {
		ring := d.ringMeters(z.Ring)
		for _, p := range ring {
			grow(p)
		}
		s.Zones = append(s.Zones, ScenePolygon{ID: z.ID, Kind: z.Kind, Name: z.Name, Ring: roundRing(ring)})
	}
	for _, o := range d.Layers.Obstacles {
		ring := d.ringMeters(o.Ring)
		for _, p := range ring {
			grow(p)
		}
		s.Obstacles = append(s.Obstacles, ScenePolygon{ID: o.ID, Kind: o.Kind, Name: o.Name, Ring: roundRing(ring)})
	}
	for _, n := range g.Nodes {
		grow(XY{X: n.X, Y: n.Y})
	}
	if d.Page != nil && d.Page.WidthPx > 0 && d.Page.HeightPx > 0 {
		grow(d.toMeters(d.Page.WidthPx, d.Page.HeightPx))
	}
	s.WidthM = round3(maxX)
	s.HeightM = round3(maxY)
	return s
}

func roundRing(ring []XY) []XY {
	out := make([]XY, len(ring))
	for i, p := range ring {
		out[i] = XY{X: round3(p.X), Y: round3(p.Y)}
	}
	return out
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

type pqItem struct {
	node int
	d    float64
}

type pq []pqItem

func (q pq) Len() int { return len(q) }
func (q pq) Less(i, j int) bool {
	if q[i].d != q[j].d {
		return q[i].d < q[j].d
	}
	return q[i].node < q[j].node
}
func (q pq) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *pq) Push(x any)   { *q = append(*q, x.(pqItem)) }
func (q *pq) Pop() any {
	old := *q
	it := old[len(old)-1]
	*q = old[:len(old)-1]
	return it
}

// ShortestFrom runs Dijkstra over edges at least minWidthM wide.
// prevEdge[v] is the edge used to enter v, or -1.
func (g *Graph) ShortestFrom(src int, minWidthM float64) (distM []float64, prevEdge []int) {
	n := len(g.Nodes)
	distM = make([]float64, n)
	prevEdge = make([]int, n)
	for i := range distM {
		distM[i] = math.Inf(1)
		prevEdge[i] = -1
	}
	if src < 0 || src >= n {
		return distM, prevEdge
	}
	distM[src] = 0
	q := &pq{{node: src}}
	for q.Len() > 0 {
		it := heap.Pop(q).(pqItem)
		if it.d > distM[it.node] {
			continue
		}
		for _, a := range g.out[it.node] {
			e := g.Edges[a.edge]
			if e.WidthM+1e-9 < minWidthM {
				continue
			}
			nd := it.d + e.LengthM
			if nd < distM[a.to]-1e-9 {
				distM[a.to] = nd
				prevEdge[a.to] = a.edge
				heap.Push(q, pqItem{node: a.to, d: nd})
			}
		}
	}
	return distM, prevEdge
}

// PathEdges walks prevEdge back from dst to src and returns edges in travel order.
func (g *Graph) PathEdges(prevEdge []int, src, dst int) []int {
	var rev []int
	cur := dst
	for cur != src {
		ei := prevEdge[cur]
		if ei < 0 {
			return nil
		}
		rev = append(rev, ei)
		e := g.Edges[ei]
		if e.B == cur {
			cur = e.A
		} else {
			cur = e.B
		}
		if len(rev) > len(g.Edges) {
			return nil
		}
	}
	out := make([]int, len(rev))
	for i := range rev {
		out[i] = rev[len(rev)-1-i]
	}
	return out
}

// Other returns the far end of edge ei seen from node.
func (g *Graph) Other(ei, node int) int {
	e := g.Edges[ei]
	if e.A == node {
		return e.B
	}
	return e.A
}
