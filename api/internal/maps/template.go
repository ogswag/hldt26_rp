package maps

import "fmt"

const (
	templatePxPerM  = 10.0
	templateWidthM  = 84.0
	templateHeightM = 52.0
)

// TemplateWidths are aisle widths taken from object params.
type TemplateWidths struct {
	MainM    float64
	WorkingM float64
}

// WarehouseTemplate returns an 84 x 52 m warehouse with docks, racks, a narrow passage,
// a bypass, a pick area and a charging station. Flows are added for known process codes.
func WarehouseTemplate(processCodes []string, w TemplateWidths) Document {
	if w.MainM <= 0 {
		w.MainM = 3.5
	}
	if w.WorkingM <= 0 {
		w.WorkingM = 2.8
	}
	px := func(m float64) float64 { return m * templatePxPerM }
	rect := func(id, kind, name string, x1, y1, x2, y2 float64) Polygon {
		return Polygon{ID: id, Kind: kind, Name: name, Ring: []XY{
			{X: px(x1), Y: px(y1)}, {X: px(x2), Y: px(y1)}, {X: px(x2), Y: px(y2)}, {X: px(x1), Y: px(y2)},
		}}
	}
	point := func(id, kind, name string, x, y float64) PointFeature {
		return PointFeature{ID: id, Kind: kind, Name: name, X: px(x), Y: px(y)}
	}
	width := func(v float64) *float64 { return &v }
	open := width(5.0)
	spur := width(4.0)
	d := Document{
		SchemaVersion: SchemaVersion,
		Profile:       ProfileIndoor,
		Units:         "m",
		Page:          &Page{WidthPx: px(templateWidthM), HeightPx: px(templateHeightM), SourceKind: "none"},
		Calibration: Calibration{
			MetersPerPx: 1 / templatePxPerM,
			Segment:     &Segment{X1: 0, Y1: 0, X2: px(templateWidthM), Y2: 0, LengthM: templateWidthM},
			Check:       &Segment{X1: 0, Y1: 0, X2: 0, Y2: px(templateHeightM), LengthM: templateHeightM},
		},
	}
	l := &d.Layers
	l.Zones = []Polygon{
		rect("zone-work", ZoneWorkspace, "Рабочая область", 0, 0, templateWidthM, templateHeightM),
		rect("zone-docks", "dock_area", "Доки", 0.5, 8, 6, 46),
		rect("zone-storage", "storage", "Хранение", 20, 2, 48, 50),
		rect("zone-pick", "pick", "Отбор", 70, 14, 83.5, 38),
		rect("zone-charge", "charge", "Зарядка", 56, 1, 68, 9),
	}
	aisles := []float64{25.9, 31.3, 36.7, 42.1}
	for k := 0; k < 5; k++ {
		x := 22 + float64(k)*5.4
		l.Obstacles = append(l.Obstacles,
			rect(fmt.Sprintf("rack-u%d", k+1), "rack", fmt.Sprintf("Стеллаж В%d", k+1), x, 4, x+2.4, 24),
			rect(fmt.Sprintf("rack-l%d", k+1), "rack", fmt.Sprintf("Стеллаж Н%d", k+1), x, 28, x+2.4, 48),
		)
	}
	l.Obstacles = append(l.Obstacles,
		rect("wall-n1", "wall", "Стена прохода", 56, 20, 68, 25),
		rect("wall-n2", "wall", "Стена прохода", 56, 27, 68, 32),
	)
	l.Points = []PointFeature{
		point("D1", PointDock, "Ворота 1", 3, 12),
		point("D2", PointDock, "Ворота 2", 3, 18),
		point("D3", PointDock, "Ворота 3", 3, 36),
		point("D4", PointDock, "Ворота 4", 3, 42),
		point("B1", PointTask, "Буфер приёмки 1", 8, 12),
		point("B2", PointTask, "Буфер приёмки 2", 8, 18),
		point("O1", PointGate, "Отгрузка 1", 8, 36),
		point("O2", PointGate, "Отгрузка 2", 8, 42),
		point("L1", PointOther, "", 14, 12),
		point("L2", PointOther, "", 14, 18),
		point("M0", PointOther, "", 14, 26),
		point("L3", PointOther, "", 14, 36),
		point("L4", PointOther, "", 14, 42),
		point("M1", PointOther, "", 50, 26),
		point("N1", PointOther, "Вход в узкий проход", 56, 26),
		point("N2", PointOther, "Выход из узкого прохода", 68, 26),
		point("M2", PointOther, "", 72, 26),
		point("P1", PointTask, "Станция отбора 1", 76, 20),
		point("P2", PointTask, "Станция отбора 2", 76, 32),
		point("Y1", PointOther, "", 50, 50),
		point("Y2", PointOther, "", 72, 50),
		point("U1", PointOther, "", 50, 10),
		point("K1", PointOther, "", 62, 10),
		point("C1", PointCharger, "Зарядка 1", 60, 5),
		point("C2", PointCharger, "Зарядка 2", 64, 5),
	}
	for k, x := range aisles {
		l.Points = append(l.Points,
			point(fmt.Sprintf("A%d", k+1), PointOther, "", x, 26),
			point(fmt.Sprintf("S%d", k+1), PointTask, fmt.Sprintf("Ячейки В%d", k+1), x, 14),
			point(fmt.Sprintf("S%d", k+5), PointTask, fmt.Sprintf("Ячейки Н%d", k+1), x, 38),
		)
	}
	edge := func(id, a, b string, wm *float64) Edge {
		return Edge{ID: id, From: a, To: b, WidthM: wm}
	}
	l.Edges = []Edge{
		edge("e-d1", "D1", "B1", open),
		edge("e-d2", "D2", "B2", open),
		edge("e-d3", "D3", "O1", open),
		edge("e-d4", "D4", "O2", open),
		edge("e-b1", "B1", "L1", open),
		edge("e-b2", "B2", "L2", open),
		edge("e-o1", "O1", "L3", open),
		edge("e-o2", "O2", "L4", open),
		edge("e-l12", "L1", "L2", width(w.MainM)),
		edge("e-l2m", "L2", "M0", width(w.MainM)),
		edge("e-ml3", "M0", "L3", width(w.MainM)),
		edge("e-l34", "L3", "L4", width(w.MainM)),
		edge("e-ma1", "M0", "A1", width(w.MainM)),
		edge("e-a12", "A1", "A2", width(w.MainM)),
		edge("e-a23", "A2", "A3", width(w.MainM)),
		edge("e-a34", "A3", "A4", width(w.MainM)),
		edge("e-a4m", "A4", "M1", width(w.MainM)),
		edge("e-mn1", "M1", "N1", width(w.MainM)),
		edge("e-narrow", "N1", "N2", width(2.0)),
		edge("e-n2m", "N2", "M2", width(w.MainM)),
		edge("e-p1", "M2", "P1", spur),
		edge("e-p2", "M2", "P2", spur),
		edge("e-by1", "M1", "Y1", spur),
		edge("e-by2", "Y1", "Y2", spur),
		edge("e-by3", "Y2", "M2", spur),
		edge("e-u1", "M1", "U1", spur),
		edge("e-k1", "U1", "K1", spur),
		edge("e-c1", "K1", "C1", spur),
		edge("e-c2", "K1", "C2", spur),
	}
	for k := range aisles {
		a := fmt.Sprintf("A%d", k+1)
		l.Edges = append(l.Edges,
			edge(fmt.Sprintf("e-s%d", k+1), a, fmt.Sprintf("S%d", k+1), width(w.WorkingM)),
			edge(fmt.Sprintf("e-s%d", k+5), a, fmt.Sprintf("S%d", k+5), width(w.WorkingM)),
		)
	}
	l.Resources = []Resource{
		{ID: "res-docks", Kind: ResourceDock, Name: "Доки", Capacity: 2, PointIDs: []string{"D1", "D2", "D3", "D4"}},
		{ID: "res-narrow", Kind: ResourceNarrowAisle, Name: "Узкий проход", Capacity: 1, EdgeIDs: []string{"e-narrow"}},
		{ID: "res-charge", Kind: ResourceCharger, Name: "Зарядная станция", Capacity: 2, PointIDs: []string{"C1", "C2"}},
	}
	storage := []string{"S1", "S2", "S3", "S4", "S5", "S6", "S7", "S8"}
	known := map[string]Flow{
		"inbound":    {ProcessCode: "inbound", PickupPointIDs: []string{"D1", "D2"}, DropPointIDs: []string{"B1", "B2"}},
		"putaway":    {ProcessCode: "putaway", PickupPointIDs: []string{"B1", "B2"}, DropPointIDs: storage},
		"piece_pick": {ProcessCode: "piece_pick", PickupPointIDs: storage, DropPointIDs: []string{"P1", "P2"}},
		"outbound":   {ProcessCode: "outbound", PickupPointIDs: storage, DropPointIDs: []string{"D3", "D4"}},
	}
	l.Flows = []Flow{}
	for _, code := range processCodes {
		if f, ok := known[code]; ok {
			l.Flows = append(l.Flows, f)
		}
	}
	return d
}
