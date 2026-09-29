package maps

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

var codes = []string{"inbound", "putaway", "piece_pick", "outbound"}

func template() Document {
	return WarehouseTemplate(codes, TemplateWidths{MainM: 3.5, WorkingM: 2.8})
}

var stacker = ClassSpec{Code: "pallet", Label: "Штабелёр", WidthM: 1.2, ClearanceM: 0.6}

func hasCode(issues []Issue, level, code string) bool {
	for _, is := range issues {
		if is.Level == level && is.Code == code {
			return true
		}
	}
	return false
}

func TestTemplateIsValid(t *testing.T) {
	issues := Validate(template(), Context{ProcessCodes: codes, Classes: []ClassSpec{DefaultClass, stacker}})
	if HasErrors(issues) {
		t.Fatalf("template errors: %+v", issues)
	}
	if !hasCode(issues, LevelWarning, "edge_single_lane") {
		t.Fatal("narrow passage must be reported as single lane")
	}
	raw, err := json.Marshal(template())
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Layers.Flows) != 4 || back.Calibration.Check == nil {
		t.Fatalf("round trip lost data: %+v", back.Layers.Flows)
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	if _, err := Decode([]byte(`{"schema_version":"map-v1","extra":1}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := Decode([]byte(`null`)); err == nil {
		t.Fatal("null accepted")
	}
	if !IsEmpty([]byte(" null ")) || IsEmpty([]byte(`{}`)) {
		t.Fatal("IsEmpty")
	}
}

func TestCalibrationChecks(t *testing.T) {
	d := template()
	d.Calibration.MetersPerPx = 0.11
	if !hasCode(Validate(d, Context{}), LevelError, "calibration_mismatch") {
		t.Fatal("scale that disagrees with the segment must fail")
	}
	d = template()
	d.Calibration.Check.LengthM = 53.5
	issues := Validate(d, Context{})
	if !hasCode(issues, LevelWarning, "calibration_check") || HasErrors(issues) {
		t.Fatalf("2.9%% deviation must warn: %+v", issues)
	}
	d.Calibration.Check.LengthM = 60
	if !hasCode(Validate(d, Context{}), LevelError, "calibration_check") {
		t.Fatal("15% deviation must fail")
	}
	d = template()
	d.Page.SourceKind = "png"
	d.Calibration.Check = nil
	if !hasCode(Validate(d, Context{}), LevelWarning, "calibration_check") {
		t.Fatal("missing control measurement must warn for an uploaded plan")
	}
	d = template()
	d.Calibration.MetersPerPx = 0
	d.Calibration.Segment = nil
	if !hasCode(Validate(d, Context{}), LevelError, "calibration") {
		t.Fatal("zero scale must fail")
	}
	d = template()
	d.Calibration.MetersPerPx = 10
	d.Calibration.Segment = nil
	if !hasCode(Validate(d, Context{}), LevelWarning, "scale") {
		t.Fatal("8 km plan must warn about scale")
	}
}

func movePoint(d *Document, id string, x, y float64) {
	for i := range d.Layers.Points {
		if d.Layers.Points[i].ID == id {
			d.Layers.Points[i].X = x * templatePxPerM
			d.Layers.Points[i].Y = y * templatePxPerM
		}
	}
}

func TestPlacementAndIntersections(t *testing.T) {
	d := template()
	movePoint(&d, "S1", 23, 14)
	issues := Validate(d, Context{ProcessCodes: codes})
	if !hasCode(issues, LevelError, "point_in_obstacle") {
		t.Fatalf("point inside rack: %+v", issues)
	}
	d = template()
	movePoint(&d, "P1", 90, 20)
	if !hasCode(Validate(d, Context{ProcessCodes: codes}), LevelError, "point_outside") {
		t.Fatal("point outside workspace must fail")
	}
	d = template()
	d.Layers.Edges = append(d.Layers.Edges, Edge{ID: "e-cut", From: "S1", To: "S2"})
	if !hasCode(Validate(d, Context{ProcessCodes: codes}), LevelError, "edge_obstacle") {
		t.Fatal("edge through a rack must fail")
	}
	d = template()
	d.Layers.Edges = append(d.Layers.Edges, Edge{ID: "e-x1", From: "O1", To: "L4"}, Edge{ID: "e-x2", From: "O2", To: "L3"})
	if !hasCode(Validate(d, Context{ProcessCodes: codes}), LevelWarning, "edge_cross") {
		t.Fatal("crossing edges without a node must warn")
	}
	d = template()
	d.Layers.Edges = append(d.Layers.Edges, Edge{ID: "e-bad", From: "S1", To: "nope"})
	if !hasCode(Validate(d, Context{ProcessCodes: codes}), LevelError, "edge_ref") {
		t.Fatal("dangling edge must fail")
	}
}

func TestMeasuredWidthCapsDeclared(t *testing.T) {
	d := template()
	for i := range d.Layers.Edges {
		if d.Layers.Edges[i].ID == "e-narrow" {
			w := 3.5
			d.Layers.Edges[i].WidthM = &w
		}
	}
	g, _ := Build(d, 3)
	ei, _ := g.EdgeIndex("e-narrow")
	if e := g.Edges[ei]; math.Abs(e.WidthM-2.0) > 0.01 || e.MeasuredWidthM == nil {
		t.Fatalf("walls 2 m apart must cap width: %+v", e)
	}
	if !hasCode(Validate(d, Context{ProcessCodes: codes}), LevelWarning, "edge_width_measured") {
		t.Fatal("declared width above measured must warn")
	}
	ai, _ := g.EdgeIndex("e-s1")
	if e := g.Edges[ai]; math.Abs(e.WidthM-2.8) > 0.01 || *e.MeasuredWidthM < 2.99 {
		t.Fatalf("aisle %+v", e)
	}
}

func TestUnreachableFlowFails(t *testing.T) {
	d := template()
	var edges []Edge
	for _, e := range d.Layers.Edges {
		if e.ID != "e-narrow" && e.ID != "e-by3" {
			edges = append(edges, e)
		}
	}
	d.Layers.Edges = edges
	issues := Validate(d, Context{ProcessCodes: codes})
	if !hasCode(issues, LevelError, "flow_unreachable") {
		t.Fatalf("pick stations cut off: %+v", issues)
	}
	d = template()
	for i := range d.Layers.Edges {
		if d.Layers.Edges[i].ID == "e-s1" {
			w := 1.5
			d.Layers.Edges[i].WidthM = &w
		}
	}
	issues = Validate(d, Context{ProcessCodes: codes, Classes: []ClassSpec{stacker}})
	if !hasCode(issues, LevelWarning, "edge_narrow") || !hasCode(issues, LevelError, "flow_unreachable") {
		t.Fatalf("1.5 m aisle must block the stacker: %+v", issues)
	}
	issues = Validate(d, Context{ProcessCodes: codes, Classes: []ClassSpec{DefaultClass, stacker}})
	if !hasCode(issues, LevelWarning, "flow_partial") {
		t.Fatalf("AMR still reaches S1: %+v", issues)
	}
}

func TestFlowAndResourceReferences(t *testing.T) {
	d := template()
	issues := Validate(d, Context{ProcessCodes: []string{"inbound"}})
	if !hasCode(issues, LevelError, "flow_process") {
		t.Fatal("flow for unknown process must fail")
	}
	d = template()
	d.Layers.Flows = nil
	if !hasCode(Validate(d, Context{}), LevelError, "flows_missing") {
		t.Fatal("missing flows must fail")
	}
	d = template()
	d.Layers.Resources = append(d.Layers.Resources, Resource{ID: "res-bad", Kind: ResourceDock, Capacity: 1, PointIDs: []string{"B1"}})
	if !hasCode(Validate(d, Context{ProcessCodes: codes}), LevelError, "resource_ref") {
		t.Fatal("dock resource on a task point must fail")
	}
	d = template()
	d.Layers.Resources[0].Capacity = 0
	if !hasCode(Validate(d, Context{ProcessCodes: codes}), LevelError, "resource_capacity") {
		t.Fatal("zero capacity must fail")
	}
	d = template()
	d.Layers.Resources = d.Layers.Resources[:2]
	if !hasCode(Validate(d, Context{ProcessCodes: codes}), LevelWarning, "charger_missing") {
		t.Fatal("missing charger must warn")
	}
	d = template()
	d.Layers.Points = append(d.Layers.Points, PointFeature{ID: "S1", Kind: PointTask, X: 10, Y: 10})
	if !hasCode(Validate(d, Context{}), LevelError, "id_duplicate") {
		t.Fatal("duplicate id must fail")
	}
	d = template()
	d.Layers.Points[0].ID = "док 1"
	if !hasCode(Validate(d, Context{}), LevelError, "id") {
		t.Fatal("non-ascii id must fail")
	}
}

func TestShortestPathRespectsWidth(t *testing.T) {
	g, _ := Build(template(), 3)
	src, _ := g.NodeIndex("A4")
	dst, _ := g.NodeIndex("P1")
	d, prev := g.ShortestFrom(src, 1.2)
	path := g.PathEdges(prev, src, dst)
	var ids []string
	for _, e := range path {
		ids = append(ids, g.Edges[e].ID)
	}
	if !strings.Contains(strings.Join(ids, ","), "e-narrow") {
		t.Fatalf("AMR path must use the passage: %v", ids)
	}
	wide, prevWide := g.ShortestFrom(src, 2.1)
	ids = ids[:0]
	for _, e := range g.PathEdges(prevWide, src, dst) {
		ids = append(ids, g.Edges[e].ID)
	}
	if strings.Contains(strings.Join(ids, ","), "e-narrow") || wide[dst] <= d[dst] {
		t.Fatalf("wide robot must take the bypass: %v %.1f vs %.1f", ids, wide[dst], d[dst])
	}
}

func message(issues []Issue, code string) string {
	for _, is := range issues {
		if is.Code == code {
			return is.Message
		}
	}
	return ""
}

// Features drawn after operations have UUID ids; messages name them by name and name their edges by the ends.
func TestMessagesNameNewFeatures(t *testing.T) {
	const point, rack, edge = "0b7e3f7a-8a4e-4b1f-9c2d-3e4f5a6b7c8d", "1c8f4a8b-9b5f-4c2a-8d3e-4f5a6b7c8d9e", "2d9a5b9c-8c6a-4d3b-9e4f-5a6b7c8d9e0f"
	d := template()
	d.Layers.Obstacles[0].ID = rack
	d.Layers.Obstacles[0].Name = "o7"
	ring := d.Layers.Obstacles[0].Ring
	d.Layers.Points = append(d.Layers.Points, PointFeature{ID: point, Kind: "task", Name: "T9", X: (ring[0].X + ring[2].X) / 2, Y: (ring[0].Y + ring[2].Y) / 2})
	d.Layers.Edges = append(d.Layers.Edges, Edge{ID: edge, From: point, To: "S1"})
	issues := Validate(d, Context{ProcessCodes: codes})
	if got, want := message(issues, "point_in_obstacle"), "Точка T9 находится внутри препятствия o7."; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := message(issues, "edge_obstacle"); !strings.HasPrefix(got, "Ребро T9 - S1 ") {
		t.Fatalf("edge message %q", got)
	}
}
