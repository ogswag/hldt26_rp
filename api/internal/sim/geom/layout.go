package geom

func makeLayout(in Input) Layout {
	switch in.ObjectType {
	case "airport":
		return airportLayout(in)
	case "hospital":
		return hospitalLayout(in)
	default:
		return warehouseLayout(in)
	}
}

func chargerCount(fleet int) int {
	n := 4
	if fleet > 0 {
		n = (fleet + 3) / 4
	}
	if n < 2 {
		n = 2
	}
	if n > 8 {
		n = 8
	}
	return n
}

func warehouseLayout(in Input) Layout {
	lay := Layout{Zones: []Zone{{Kind: "storage", X: 40, Y: 10, W: 80, H: 62}}}
	for _, y := range []float64{28, 44, 60} {
		lay.OpPoints = append(lay.OpPoints, Node{Kind: "load", X: 18, Y: y})
	}
	for _, y := range []float64{28, 44, 60} {
		lay.OpPoints = append(lay.OpPoints, Node{Kind: "unload", X: 142, Y: y})
	}
	nCh := chargerCount(in.FleetSize)
	span := 70.0
	for i := 0; i < nCh; i++ {
		x := 50 + span*float64(i)/float64(max(nCh-1, 1))
		lay.Chargers = append(lay.Chargers, Node{Kind: "charge", X: x, Y: 85})
	}
	return lay
}

func airportLayout(in Input) Layout {
	lay := Layout{Zones: []Zone{{Kind: "terminal", X: 88, Y: 8, W: 78, H: 72}}}
	for _, y := range []float64{22, 40, 58} {
		lay.OpPoints = append(lay.OpPoints, Node{Kind: "load", X: 28, Y: y})
	}
	for _, y := range []float64{22, 40, 58} {
		lay.OpPoints = append(lay.OpPoints, Node{Kind: "unload", X: 130, Y: y})
	}
	nCh := chargerCount(in.FleetSize)
	span := 140.0
	for i := 0; i < nCh; i++ {
		x := 16 + span*float64(i)/float64(max(nCh-1, 1))
		lay.Chargers = append(lay.Chargers, Node{Kind: "charge", X: x, Y: 90})
	}
	return lay
}

func hospitalLayout(in Input) Layout {
	m := parseParams(in.Params)
	floors := int(num(m, "floors"))
	if floors < 1 {
		floors = 3
	}
	if floors > 4 {
		floors = 4
	}
	lay := Layout{}
	y0 := 8.0
	for i := 0; i < floors; i++ {
		y := y0 + float64(floors-1-i)*18
		lay.Zones = append(lay.Zones, Zone{Kind: "floor", X: 4, Y: y, W: 152, H: 16})
	}
	groundY := y0 + float64(floors-1)*18
	lay.OpPoints = []Node{
		{Kind: "load", X: 22, Y: groundY + 8},
		{Kind: "load", X: 22, Y: groundY + 12},
		{Kind: "unload", X: 136, Y: y0 + 8},
		{Kind: "unload", X: 136, Y: y0 + 14},
		{Kind: "unload", X: 148, Y: y0 + 8},
	}
	nCh := chargerCount(in.FleetSize)
	if nCh > 4 {
		nCh = 4
	}
	for i := 0; i < nCh; i++ {
		lay.Chargers = append(lay.Chargers, Node{Kind: "charge", X: 44 + float64(i)*5, Y: groundY + 8})
	}
	return lay
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
