package maps

import (
	"fmt"
	"math"
	"moscow_hackathon_2026/api/internal/rutext"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	LevelError   = "error"
	LevelWarning = "warning"

	calibrationMismatchMax = 0.005
	checkWarnDeviation     = 0.02
	checkErrorDeviation    = 0.05
	minPlanM               = 5.0
	maxPlanM               = 2000.0
	minEdgeM               = 0.2
)

type Issue struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Ref     string `json:"ref,omitempty"`
}

// ClassSpec is the footprint of one robot class for passability checks.
type ClassSpec struct {
	Code       string
	Label      string
	WidthM     float64
	ClearanceM float64
}

func (c ClassSpec) RequiredWidthM() float64 {
	return c.WidthM + c.ClearanceM
}

func (c ClassSpec) TwoLaneWidthM() float64 {
	return TwoLaneWidthM(c.WidthM, c.ClearanceM)
}

// TwoLaneWidthM is the aisle width where two robots of this class pass each other:
// two bodies, a side clearance at each wall and half a clearance between them.
func TwoLaneWidthM(widthM, clearanceM float64) float64 {
	return 2*widthM + 1.5*clearanceM
}

type Context struct {
	ProcessCodes []string
	// ProcessNames maps a process code to the name people see; messages use it instead of the code.
	ProcessNames  map[string]string
	Classes       []ClassSpec
	DefaultWidthM float64
}

var DefaultClass = ClassSpec{Code: "amr", Label: "AMR", WidthM: 0.7, ClearanceM: 0.5}

func HasErrors(issues []Issue) bool {
	for _, is := range issues {
		if is.Level == LevelError {
			return true
		}
	}
	return false
}

func Messages(issues []Issue) (errs, warns []string) {
	errs = []string{}
	warns = []string{}
	for _, is := range issues {
		if is.Level == LevelError {
			errs = append(errs, is.Message)
		} else {
			warns = append(warns, is.Message)
		}
	}
	return errs, warns
}

type collector struct {
	items     []Issue
	names     map[string]string
	processes map[string]string
}

// process is how messages call a process: by its name, the code only when the name is unknown.
func (c *collector) process(code string) string {
	if n := strings.TrimSpace(c.processes[code]); n != "" {
		return "«" + n + "»"
	}
	return code
}

// name is how messages call a feature: by its name, with the short code the canvas prints next to a point.
func (c *collector) name(id string) string {
	if n, ok := c.names[id]; ok {
		return n
	}
	return label("", id)
}

func featureNames(d Document) map[string]string {
	n := map[string]string{}
	for _, p := range d.Layers.Points {
		n[p.ID] = label(p.Name, p.ID)
	}
	for _, r := range d.Layers.Resources {
		n[r.ID] = label(r.Name, r.ID)
	}
	for _, list := range [][]Polygon{d.Layers.Zones, d.Layers.Obstacles} {
		for _, p := range list {
			n[p.ID] = label(p.Name, p.ID)
		}
	}
	// An edge is named by the codes of its ends, as the editor names it.
	short := map[string]string{}
	for _, p := range d.Layers.Points {
		short[p.ID] = label(p.Name, p.ID)
		if featureCode.MatchString(p.ID) {
			short[p.ID] = p.ID
		}
	}
	end := func(id string) string {
		if s, ok := short[id]; ok {
			return s
		}
		return "без имени"
	}
	for _, e := range d.Layers.Edges {
		n[e.ID] = end(e.From) + " - " + end(e.To)
	}
	return n
}

func (c *collector) err(code, ref, format string, args ...any) {
	c.items = append(c.items, Issue{Level: LevelError, Code: code, Ref: ref, Message: fmt.Sprintf(format, args...)})
}

func (c *collector) warn(code, ref, format string, args ...any) {
	c.items = append(c.items, Issue{Level: LevelWarning, Code: code, Ref: ref, Message: fmt.Sprintf(format, args...)})
}

// Validate returns blocking errors and warnings that lower confidence.
func Validate(d Document, ctx Context) []Issue {
	c := &collector{names: featureNames(d), processes: ctx.ProcessNames}
	if !validateShape(d, c) {
		return c.items
	}
	validateCalibration(d, c)
	g, scene := Build(d, ctx.DefaultWidthM)
	validateExtent(scene, c)
	validatePlacement(d, c)
	validateEdges(d, g, c)
	classes := ctx.Classes
	if len(classes) == 0 {
		classes = []ClassSpec{DefaultClass}
	}
	validatePassability(g, classes, c)
	validateResources(d, g, c)
	validateFlows(d, g, ctx.ProcessCodes, classes, c)
	return c.items
}

func validID(id string) bool {
	if id == "" || len(id) > MaxIDLength {
		return false
	}
	for _, r := range id {
		ok := r == '-' || r == '_' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !ok {
			return false
		}
	}
	return true
}

func validateShape(d Document, c *collector) bool {
	if d.SchemaVersion != SchemaVersion {
		c.err("schema_version", "", "schema_version должен быть %s.", SchemaVersion)
	}
	switch d.Profile {
	case ProfileIndoor:
	case ProfileAirspace, ProfileField:
		c.err("profile", "", "Профиль карты %s пока не поддерживается. Используйте indoor.", d.Profile)
	default:
		c.err("profile", "", "profile должен быть indoor, airspace или field.")
	}
	if d.Units != "m" {
		c.err("units", "", "units должен быть m.")
	}
	mpp := d.Calibration.MetersPerPx
	if !finite(mpp) || mpp <= 0 {
		c.err("calibration", "", "Масштаб не задан. Откалибруйте план по отрезку известной длины.")
	}
	if d.Page != nil {
		switch d.Page.SourceKind {
		case "png", "jpeg", "pdf", "none":
		default:
			c.err("page", "", "page.source_kind должен быть png, jpeg, pdf или none.")
		}
		if !finite(d.Page.WidthPx) || !finite(d.Page.HeightPx) || d.Page.WidthPx < 0 || d.Page.HeightPx < 0 {
			c.err("page", "", "Размер страницы должен быть неотрицательным числом.")
		}
	}
	l := d.Layers
	if len(l.Points) > MaxPoints || len(l.Edges) > MaxEdges || len(l.Zones)+len(l.Obstacles) > MaxPolygons ||
		len(l.Resources) > MaxResources || len(l.Flows) > MaxFlows {
		c.err("limits", "", "Карта слишком большая: до %d точек, %d рёбер, %d контуров, %d ресурсов и %d потоков.",
			MaxPoints, MaxEdges, MaxPolygons, MaxResources, MaxFlows)
		return false
	}
	seen := map[string]string{}
	checkID := func(id, what string) {
		if !validID(id) {
			c.err("id", id, "Некорректный id %q у объекта %s: латиница, цифры, -, _ и точка, до %d символов.", id, what, MaxIDLength)
			return
		}
		if prev, dup := seen[id]; dup {
			c.err("id_duplicate", id, "id %q используется дважды (%s и %s).", id, prev, what)
			return
		}
		seen[id] = what
	}
	checkName := func(name, ref string) {
		if utf8.RuneCountInString(name) > MaxNameRunes {
			c.err("name", ref, "Имя объекта %s длиннее %d символов.", ref, MaxNameRunes)
		}
	}
	for _, list := range [][]Polygon{l.Zones, l.Obstacles} {
		for _, p := range list {
			checkID(p.ID, "контур")
			checkName(p.Name, p.ID)
			if len(p.Ring) < 3 || len(p.Ring) > MaxRing {
				c.err("ring", p.ID, "Контур %s должен иметь от 3 до %d вершин.", c.name(p.ID), MaxRing)
				continue
			}
			for _, v := range p.Ring {
				if !finite(v.X) || !finite(v.Y) {
					c.err("ring", p.ID, "Контур %s содержит нечисловую координату.", c.name(p.ID))
					break
				}
			}
			if polygonArea(p.Ring) < eps {
				c.err("ring", p.ID, "Контур %s имеет нулевую площадь.", c.name(p.ID))
			}
		}
	}
	for _, p := range l.Points {
		checkID(p.ID, "точка")
		checkName(p.Name, p.ID)
		switch p.Kind {
		case PointTask, PointDock, PointCharger, PointGate, PointOther:
		default:
			c.err("point_kind", p.ID, "Точка %s: kind должен быть task, dock, charger, gate или other.", c.name(p.ID))
		}
		if !finite(p.X) || !finite(p.Y) {
			c.err("point_xy", p.ID, "Точка %s имеет нечисловые координаты.", c.name(p.ID))
		}
	}
	for _, e := range l.Edges {
		checkID(e.ID, "ребро")
		if e.WidthM != nil && (!finite(*e.WidthM) || *e.WidthM <= 0 || *e.WidthM > 100) {
			c.err("edge_width", e.ID, "Ребро %s: ширина должна быть больше 0 и не больше 100 м.", c.name(e.ID))
		}
	}
	for _, r := range l.Resources {
		checkID(r.ID, "ресурс")
		checkName(r.Name, r.ID)
		switch r.Kind {
		case ResourceDock, ResourceNarrowAisle, ResourceCharger:
		default:
			c.err("resource_kind", r.ID, "Ресурс %s: kind должен быть dock, narrow_aisle или charger.", c.name(r.ID))
		}
		if r.Capacity < 1 || r.Capacity > 100 {
			c.err("resource_capacity", r.ID, "Ресурс %s: вместимость должна быть от 1 до 100.", c.name(r.ID))
		}
	}
	return !HasErrors(c.items)
}

func validateCalibration(d Document, c *collector) {
	cal := d.Calibration
	mpp := cal.MetersPerPx
	hasSource := d.Page != nil && d.Page.SourceKind != "" && d.Page.SourceKind != "none"
	if s := cal.Segment; s != nil {
		px := math.Hypot(s.X2-s.X1, s.Y2-s.Y1)
		switch {
		case px < 1:
			c.err("calibration_segment", "", "Отрезок калибровки короче 1 пикселя. Проведите его заново.")
		case !finite(s.LengthM) || s.LengthM <= 0:
			c.err("calibration_segment", "", "Длина отрезка калибровки должна быть больше 0 м.")
		default:
			want := s.LengthM / px
			if math.Abs(mpp-want)/want > calibrationMismatchMax {
				c.err("calibration_mismatch", "", "Масштаб %s м/пикс не совпадает с отрезком калибровки (%s м/пикс). Повторите калибровку.", rutext.Num(mpp, 5), rutext.Num(want, 5))
			}
		}
	} else if hasSource {
		c.warn("calibration_segment", "", "Масштаб задан без отрезка калибровки. Проведите отрезок известной длины.")
	}
	if s := cal.Check; s != nil {
		px := math.Hypot(s.X2-s.X1, s.Y2-s.Y1)
		if px < 1 || !finite(s.LengthM) || s.LengthM <= 0 {
			c.err("calibration_check", "", "Контрольное измерение задано неверно. Проведите отрезок и укажите его длину.")
			return
		}
		measured := px * mpp
		dev := math.Abs(measured-s.LengthM) / s.LengthM
		switch {
		case dev > checkErrorDeviation:
			c.err("calibration_check", "", "Контрольное измерение расходится на %s: по плану %s м, указано %s м. Масштаб неверный, откалибруйте заново.", rutext.Pct(dev*100, 1), rutext.Num(measured, 2), rutext.Num(s.LengthM, 2))
		case dev > checkWarnDeviation:
			c.warn("calibration_check", "", "Контрольное измерение расходится на %s (по плану %s м, указано %s м). Результат менее точен.", rutext.Pct(dev*100, 1), rutext.Num(measured, 2), rutext.Num(s.LengthM, 2))
		}
	} else if hasSource {
		c.warn("calibration_check", "", "Контрольное измерение не выполнено. Масштаб не проверен.")
	}
}

func validateExtent(s Scene, c *collector) {
	span := math.Max(s.WidthM, s.HeightM)
	if span < minPlanM || span > maxPlanM {
		c.warn("scale", "", "Размер плана %s x %s м выглядит неправдоподобно для склада. Проверьте масштаб.", rutext.Num(s.WidthM, 1), rutext.Num(s.HeightM, 1))
	}
}

func validatePlacement(d Document, c *collector) {
	var workspaces [][]XY
	for _, z := range d.Layers.Zones {
		if z.Kind == ZoneWorkspace {
			workspaces = append(workspaces, d.ringMeters(z.Ring))
		}
	}
	if len(workspaces) == 0 {
		c.warn("workspace", "", "Рабочая область не задана. Стены не учитываются при расчёте ширины проходов.")
	}
	obstacles := make([][]XY, 0, len(d.Layers.Obstacles))
	for _, o := range d.Layers.Obstacles {
		obstacles = append(obstacles, d.ringMeters(o.Ring))
	}
	for _, p := range d.Layers.Points {
		m := d.toMeters(p.X, p.Y)
		for i, ring := range obstacles {
			if insidePolygon(m, ring) {
				c.err("point_in_obstacle", p.ID, "Точка %s находится внутри препятствия %s.", c.name(p.ID), c.name(d.Layers.Obstacles[i].ID))
				break
			}
		}
		if len(workspaces) > 0 {
			inside := false
			for _, ring := range workspaces {
				if insidePolygon(m, ring) {
					inside = true
					break
				}
			}
			if !inside {
				c.err("point_outside", p.ID, "Точка %s вне рабочей области.", c.name(p.ID))
			}
		}
	}
}

func validateEdges(d Document, g *Graph, c *collector) {
	pts := d.pointIndex()
	pairs := map[string]string{}
	for _, e := range d.Layers.Edges {
		_, okA := pts[e.From]
		_, okB := pts[e.To]
		if !okA || !okB {
			c.err("edge_ref", e.ID, "Ребро %s ссылается на несуществующую точку.", c.name(e.ID))
			continue
		}
		if e.From == e.To {
			c.err("edge_loop", e.ID, "Ребро %s начинается и заканчивается в одной точке.", c.name(e.ID))
			continue
		}
		key := e.From + "|" + e.To
		if e.From > e.To {
			key = e.To + "|" + e.From
		}
		if prev, dup := pairs[key]; dup {
			c.warn("edge_duplicate", e.ID, "Рёбра %s и %s соединяют одни и те же точки.", c.name(prev), c.name(e.ID))
		}
		pairs[key] = e.ID
	}
	obstacles := make([][]XY, 0, len(d.Layers.Obstacles))
	for _, o := range d.Layers.Obstacles {
		obstacles = append(obstacles, d.ringMeters(o.Ring))
	}
	for _, e := range g.Edges {
		a := XY{X: g.Nodes[e.A].X, Y: g.Nodes[e.A].Y}
		b := XY{X: g.Nodes[e.B].X, Y: g.Nodes[e.B].Y}
		if e.LengthM < minEdgeM {
			c.warn("edge_short", e.ID, "Ребро %s короче %s м. Проверьте масштаб или объедините точки.", c.name(e.ID), rutext.Num(minEdgeM, 1))
		}
		for i, ring := range obstacles {
			if segmentHitsPolygon(a, b, ring) {
				c.err("edge_obstacle", e.ID, "Ребро %s пересекает препятствие %s.", c.name(e.ID), c.name(d.Layers.Obstacles[i].ID))
				break
			}
		}
		if e.DeclaredWidthM != nil && e.MeasuredWidthM != nil && *e.MeasuredWidthM+0.05 < *e.DeclaredWidthM {
			c.warn("edge_width_measured", e.ID, "Ребро %s: указана ширина %s м, по плану свободно %s м. Используется %s м.",
				c.name(e.ID), rutext.Num(*e.DeclaredWidthM, 2), rutext.Num(*e.MeasuredWidthM, 2), rutext.Num(e.WidthM, 2))
		}
		if e.DeclaredWidthM == nil && e.MeasuredWidthM == nil {
			c.warn("edge_width_missing", e.ID, "Ребро %s: ширина прохода не задана и не измерена, принято %s м.", c.name(e.ID), rutext.Num(e.WidthM, 1))
		}
	}
	for i := 0; i < len(g.Edges); i++ {
		ei := g.Edges[i]
		for j := i + 1; j < len(g.Edges); j++ {
			ej := g.Edges[j]
			if ei.A == ej.A || ei.A == ej.B || ei.B == ej.A || ei.B == ej.B {
				continue
			}
			if properCross(nodeXY(g, ei.A), nodeXY(g, ei.B), nodeXY(g, ej.A), nodeXY(g, ej.B)) {
				c.warn("edge_cross", ei.ID, "Рёбра %s и %s пересекаются без общей точки. Добавьте узел в месте пересечения.", c.name(ei.ID), c.name(ej.ID))
			}
		}
	}
}

func nodeXY(g *Graph, i int) XY {
	return XY{X: g.Nodes[i].X, Y: g.Nodes[i].Y}
}

func validatePassability(g *Graph, classes []ClassSpec, c *collector) {
	widest := classes[0]
	for _, cl := range classes {
		if cl.TwoLaneWidthM() > widest.TwoLaneWidthM() {
			widest = cl
		}
	}
	for _, e := range g.Edges {
		var blocked []string
		for _, cl := range classes {
			if e.WidthM+1e-9 < cl.RequiredWidthM() {
				blocked = append(blocked, fmt.Sprintf("%s (нужно %s м)", cl.Label, rutext.Num(cl.RequiredWidthM(), 2)))
			}
		}
		if len(blocked) > 0 {
			c.warn("edge_narrow", e.ID, "Ребро %s шириной %s м непроходимо для: %s.", c.name(e.ID), rutext.Num(e.WidthM, 2), strings.Join(blocked, ", "))
			continue
		}
		if e.WidthM < widest.TwoLaneWidthM() {
			c.warn("edge_single_lane", e.ID, "Ребро %s шириной %s м однополосное: встречные роботы ждут друг друга.", c.name(e.ID), rutext.Num(e.WidthM, 2))
		}
	}
}

func validateResources(d Document, g *Graph, c *collector) {
	pts := d.pointIndex()
	chargers := 0
	for _, r := range d.Layers.Resources {
		ids := r.Points()
		switch r.Kind {
		case ResourceDock, ResourceCharger:
			if len(ids) == 0 {
				c.err("resource_points", r.ID, "Ресурс %s не привязан к точкам.", c.name(r.ID))
			}
			if len(r.EdgeIDs) > 0 {
				c.err("resource_edges", r.ID, "Ресурс %s: рёбра задаются только для узкого участка.", c.name(r.ID))
			}
			want := PointDock
			if r.Kind == ResourceCharger {
				want = PointCharger
				chargers++
			}
			for _, id := range ids {
				p, ok := pts[id]
				if !ok {
					c.err("resource_ref", r.ID, "Ресурс %s ссылается на несуществующую точку %s.", c.name(r.ID), c.name(id))
					continue
				}
				if p.Kind != want {
					c.err("resource_ref", r.ID, "Ресурс %s: точка %s должна иметь тип %s.", c.name(r.ID), c.name(id), want)
				}
			}
		case ResourceNarrowAisle:
			if len(r.EdgeIDs) == 0 {
				c.err("resource_edges", r.ID, "Узкий участок %s не содержит рёбер.", c.name(r.ID))
			}
			if len(ids) > 0 {
				c.err("resource_points", r.ID, "Узкий участок %s задаётся рёбрами, не точками.", c.name(r.ID))
			}
			for _, id := range r.EdgeIDs {
				if _, ok := g.EdgeIndex(id); !ok {
					c.err("resource_ref", r.ID, "Узкий участок %s ссылается на несуществующее ребро %s.", c.name(r.ID), c.name(id))
				}
			}
		}
	}
	claimed := map[string]string{}
	for _, r := range d.Layers.Resources {
		keys := r.Points()
		if r.Kind == ResourceNarrowAisle {
			keys = r.EdgeIDs
		}
		for _, k := range keys {
			if prev, dup := claimed[k]; dup && prev != r.ID {
				c.err("resource_overlap", r.ID, "Объект %s входит в ресурсы %s и %s. Оставьте один.", c.name(k), c.name(prev), c.name(r.ID))
			}
			claimed[k] = r.ID
		}
	}
	for _, p := range d.Layers.Points {
		if p.Kind == PointCharger {
			if _, ok := claimed[p.ID]; !ok {
				c.warn("charger_unbound", p.ID, "Зарядная точка %s не входит в ресурс зарядки и не используется.", c.name(p.ID))
			}
		}
	}
	if chargers == 0 {
		c.warn("charger_missing", "", "Нет зарядной станции. Разряд батареи не моделируется.")
	}
}

func validateFlows(d Document, g *Graph, processCodes []string, classes []ClassSpec, c *collector) {
	if len(d.Layers.Flows) == 0 {
		c.err("flows_missing", "", "Не заданы потоки: для каждого процесса укажите точки забора и доставки.")
		return
	}
	known := map[string]bool{}
	for _, code := range processCodes {
		known[code] = true
	}
	seen := map[string]bool{}
	var chargerNodes []int
	for _, r := range d.Layers.Resources {
		if r.Kind != ResourceCharger {
			continue
		}
		for _, id := range r.Points() {
			if i, ok := g.NodeIndex(id); ok {
				chargerNodes = append(chargerNodes, i)
			}
		}
	}
	cache := map[string][]float64{}
	distFrom := func(src int, cl ClassSpec) []float64 {
		key := fmt.Sprintf("%d|%.3f", src, cl.RequiredWidthM())
		if v, ok := cache[key]; ok {
			return v
		}
		dm, _ := g.ShortestFrom(src, cl.RequiredWidthM())
		cache[key] = dm
		return dm
	}
	unreachableCharger := map[string]bool{}
	for _, f := range d.Layers.Flows {
		ref := f.ProcessCode
		if strings.TrimSpace(f.ProcessCode) == "" {
			c.err("flow_process", "", "У потока не указан код процесса.")
			continue
		}
		if seen[f.ProcessCode] {
			c.err("flow_duplicate", ref, "Поток процесса %s задан дважды.", c.process(f.ProcessCode))
			continue
		}
		seen[f.ProcessCode] = true
		if processCodes != nil && !known[f.ProcessCode] {
			c.err("flow_process", ref, "Поток ссылается на неизвестный процесс %s. Удалите поток или добавьте процесс.", c.process(f.ProcessCode))
		}
		if len(f.PickupPointIDs) == 0 || len(f.DropPointIDs) == 0 {
			c.err("flow_points", ref, "Поток %s: укажите хотя бы одну точку забора и одну точку доставки.", c.process(f.ProcessCode))
			continue
		}
		var picks, drops []int
		bad := false
		for _, list := range []struct {
			ids []string
			dst *[]int
		}{{f.PickupPointIDs, &picks}, {f.DropPointIDs, &drops}} {
			for _, id := range list.ids {
				i, ok := g.NodeIndex(id)
				if !ok {
					c.err("flow_ref", ref, "Поток %s ссылается на несуществующую точку %s.", c.process(f.ProcessCode), c.name(id))
					bad = true
					continue
				}
				if k := g.Nodes[i].Kind; k == PointCharger {
					c.err("flow_ref", ref, "Поток %s: зарядная точка %s не может быть точкой задания.", c.process(f.ProcessCode), c.name(id))
					bad = true
				}
				*list.dst = append(*list.dst, i)
			}
		}
		if bad {
			continue
		}
		var ok []string
		var failed []string
		for _, cl := range classes {
			reach := true
			check := func(from, to []int) {
				for _, p := range from {
					dm := distFrom(p, cl)
					for _, q := range to {
						if math.IsInf(dm[q], 1) {
							reach = false
							if len(failed) == 0 {
								failed = append(failed, fmt.Sprintf("%s -> %s", g.Nodes[p].ID, g.Nodes[q].ID))
							}
						}
					}
				}
			}
			check(picks, drops)
			check(drops, picks)
			if !reach {
				continue
			}
			ok = append(ok, cl.Label)
			if len(chargerNodes) > 0 {
				for _, n := range append(append([]int{}, picks...), drops...) {
					dm := distFrom(n, cl)
					found := false
					for _, ch := range chargerNodes {
						if !math.IsInf(dm[ch], 1) {
							found = true
							break
						}
					}
					if !found {
						unreachableCharger[fmt.Sprintf("%s|%s", g.Nodes[n].ID, cl.Label)] = true
					}
				}
			}
		}
		if len(ok) == 0 {
			hint := ""
			if len(failed) > 0 {
				hint = " Нет пути " + failed[0] + "."
			}
			c.err("flow_unreachable", ref, "Поток %s: точки недоступны ни для одного класса роботов.%s Проверьте рёбра и ширину проходов.", c.process(f.ProcessCode), hint)
		} else if len(ok) < len(classes) {
			c.warn("flow_partial", ref, "Поток %s доступен только для: %s.", c.process(f.ProcessCode), strings.Join(ok, ", "))
		}
	}
	keys := make([]string, 0, len(unreachableCharger))
	for k := range unreachableCharger {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts := strings.SplitN(k, "|", 2)
		c.err("charger_unreachable", parts[0], "От точки %s нет пути до зарядки для класса %s.", c.name(parts[0]), parts[1])
	}
}

// label names a feature by its name. A short code such as D1, which the canvas prints next to a point, goes
// with it; other ids (UUIDs, "zone-work" in older maps) are internal and never shown.
func label(name, id string) string {
	blank := strings.TrimSpace(name) == ""
	code := featureCode.MatchString(id)
	switch {
	case code && blank:
		return id
	case code:
		return fmt.Sprintf("%s (%s)", name, id)
	case blank:
		return "без имени"
	}
	return name
}

var featureCode = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]*$`)

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil && len(s) == 36
}
