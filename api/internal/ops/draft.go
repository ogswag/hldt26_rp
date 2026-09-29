package ops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/projects"
)

const (
	CollProject        = "project"
	CollProcesses      = "processes"
	CollVariants       = "variants"
	CollFleetItems     = "fleet_items"
	CollFinancing      = "financing"
	CollSharedCosts    = "shared_costs"
	CollAssumptionSets = "assumption_sets"
	CollMap            = "map"
	CollMapPoints      = "map_points"
	CollMapEdges       = "map_edges"
	CollMapZones       = "map_zones"
	CollMapObstacles   = "map_obstacles"
	CollMapResources   = "map_resources"
	CollMapFlows       = "map_flows"
)

// MapCollections are the collections that ToDraft folds into the map document.
var MapCollections = []string{CollMap, CollMapPoints, CollMapEdges, CollMapZones, CollMapObstacles, CollMapResources, CollMapFlows}

var flowNamespace = uuid.MustParse("7d0c5e0a-51f6-4d3e-9a55-2f1d6b8c4e10")

var defaultPage = maps.Page{WidthPx: 1000, HeightPx: 700, SourceKind: "none"}

const defaultMetersPerPx = 0.1

// FromDraft turns a stored project draft into operation state. The map exists when map.page is set.
func FromDraft(name string, d projects.Draft) (State, error) {
	st := State{}
	for _, c := range []string{CollProject, CollProcesses, CollVariants, CollFleetItems, CollFinancing, CollSharedCosts,
		CollAssumptionSets, CollMap, CollMapPoints, CollMapEdges, CollMapZones, CollMapObstacles, CollMapResources, CollMapFlows} {
		st[c] = map[string]Record{}
	}
	params, err := decodeObject(d.Params)
	if err != nil {
		return nil, fmt.Errorf("ops.from.params: %w", err)
	}
	overrides, err := decodeRaw(d.EconOverrides)
	if err != nil {
		return nil, fmt.Errorf("ops.from.econ_overrides: %w", err)
	}
	proj := Record{
		FieldID: CollProject, "name": name, "object_type": d.ObjectType, "params": params,
		"match_selected_ids": strList(d.MatchSelectedIDs), "econ_overrides": overrides, "active_assumption_set_id": nil,
		"reviewed_tabs": strList(d.ReviewedTabs),
	}
	st[CollProject][CollProject] = proj

	procs := append([]projects.Process(nil), d.Processes...)
	pkeys := orderKeys(len(procs), func(i int) (string, int) { return procs[i].Order, procs[i].SortOrder })
	for i, p := range procs {
		if p.ID == "" {
			return nil, fmt.Errorf("ops.from: process %s has no id", p.Code)
		}
		st[CollProcesses][p.ID] = Record{
			FieldID: p.ID, FieldOrder: pkeys[i], "code": p.Code, "name": p.Name, "task_type": p.TaskType,
			"is_baseline": p.IsBaseline, "demand": mustDecode(p.Demand), "sla": mustDecode(p.SLA),
			"point_ids": strList(p.PointIDs), "durations": mustDecode(p.Durations), "baseline_staff": mustDecode(p.BaselineStaff),
		}
	}

	vars := d.Variants
	vkeys := orderKeys(len(vars), func(i int) (string, int) { return vars[i].Order, vars[i].SortOrder })
	for i, v := range vars {
		if v.ID == "" {
			return nil, fmt.Errorf("ops.from: variant %s has no id", v.Name)
		}
		st[CollVariants][v.ID] = Record{FieldID: v.ID, FieldOrder: vkeys[i], "name": v.Name, "status": v.Status, "notes": v.Notes}
		fleet := v.Fleet
		fkeys := orderKeys(len(fleet), func(j int) (string, int) { return fleet[j].Order, fleet[j].SortOrder })
		for j, f := range fleet {
			if f.ID == "" {
				return nil, fmt.Errorf("ops.from: fleet item of %s has no id", v.Name)
			}
			st[CollFleetItems][f.ID] = Record{
				FieldID: f.ID, FieldOrder: fkeys[j], "variant_id": v.ID, "solution_id": strPtr(f.SolutionID),
				"quantity": float64(f.Quantity), "price_override_rub": floatPtr(f.PriceOverrideRub),
				"price_override_reason": f.PriceOverrideReason, "task_codes": strList(f.TaskCodes),
			}
		}
		for _, fin := range v.Financing {
			if fin.ID == "" {
				return nil, fmt.Errorf("ops.from: financing of %s has no id", v.Name)
			}
			ass, err := decodeObject(fin.Assumptions)
			if err != nil {
				return nil, fmt.Errorf("ops.from.financing: %w", err)
			}
			st[CollFinancing][fin.ID] = Record{
				FieldID: fin.ID, "variant_id": v.ID, "kind": fin.Kind, "tariff": strPtr(fin.Tariff), "assumptions": ass,
			}
		}
	}

	costs := d.SharedCosts
	ckeys := orderKeys(len(costs), func(i int) (string, int) { return costs[i].Order, costs[i].SortOrder })
	for i, c := range costs {
		if c.ID == "" {
			return nil, fmt.Errorf("ops.from: shared cost %s has no id", c.Code)
		}
		st[CollSharedCosts][c.ID] = Record{FieldID: c.ID, FieldOrder: ckeys[i], "code": c.Code, "label": c.Label, "bucket": c.Bucket, "rub": c.Rub}
	}

	sets := d.AssumptionSets
	skeys := orderKeys(len(sets), func(i int) (string, int) { return sets[i].Order, sets[i].SortOrder })
	for i, a := range sets {
		if a.ID == "" {
			return nil, fmt.Errorf("ops.from: assumption set %s has no id", a.Name)
		}
		st[CollAssumptionSets][a.ID] = Record{
			FieldID: a.ID, FieldOrder: skeys[i], "name": a.Name, "vat_rate": a.VATRate,
			"prices_include_vat": a.PricesIncludeVAT, "vat_recoverable": a.VATRecoverable, "labor_cash_share": a.LaborCashShare,
			"discount_rate": projects.DiscountOrDefault(a.DiscountRate), "utilization": floatPtr(a.Utilization), "availability": floatPtr(a.Availability),
			"reserve": floatPtr(a.Reserve), "service_share": floatPtr(a.ServiceShare), "delivery_share": floatPtr(a.DeliveryShare),
			"comm_rub_per_robot_year": floatPtr(a.CommRubPerRobotYear), "technician_wage_month_rub": floatPtr(a.TechnicianWageMonthRub),
		}
		if a.ID == d.ActiveAssumptionSetID || (d.ActiveAssumptionSetID == "" && a.IsActive && proj["active_assumption_set_id"] == nil) {
			proj["active_assumption_set_id"] = a.ID
		}
	}

	if err := fromMap(st, d.Map); err != nil {
		return nil, err
	}
	return st, nil
}

func fromMap(st State, raw json.RawMessage) error {
	m := Record{FieldID: CollMap, "page": nil, "profile": maps.ProfileIndoor, "meters_per_px": defaultMetersPerPx, "segment": nil, "check": nil}
	st[CollMap][CollMap] = m
	if maps.IsEmpty(raw) {
		return nil
	}
	doc, err := maps.Decode(raw)
	if err != nil {
		return fmt.Errorf("ops.from.map: %w", err)
	}
	page := defaultPage
	if doc.Page != nil {
		page = *doc.Page
	}
	m["page"] = mustDecode(page)
	m["profile"] = doc.Profile
	m["meters_per_px"] = doc.Calibration.MetersPerPx
	m["segment"] = segmentValue(doc.Calibration.Segment)
	m["check"] = segmentValue(doc.Calibration.Check)
	// NOTE: a stored map with a repeated id keeps the first feature; such a map already failed validation.
	put := func(coll, id string, rec Record) {
		if _, dup := st[coll][id]; !dup {
			st[coll][id] = rec
		}
	}
	for _, p := range doc.Layers.Points {
		put(CollMapPoints, p.ID, Record{
			FieldID: p.ID, "kind": p.Kind, "name": p.Name, "pos": map[string]any{"x": p.X, "y": p.Y}, "process_code": emptyNil(p.ProcessCode),
		})
	}
	for _, e := range doc.Layers.Edges {
		var bi, width any
		if e.Bidirectional != nil {
			bi = *e.Bidirectional
		}
		if e.WidthM != nil {
			width = *e.WidthM
		}
		put(CollMapEdges, e.ID, Record{FieldID: e.ID, "from": e.From, "to": e.To, "bidirectional": bi, "width_m": width})
	}
	for coll, list := range map[string][]maps.Polygon{CollMapZones: doc.Layers.Zones, CollMapObstacles: doc.Layers.Obstacles} {
		keys, _ := KeysAfter("", len(list))
		for i, p := range list {
			put(coll, p.ID, Record{FieldID: p.ID, FieldOrder: keys[i], "kind": p.Kind, "name": p.Name, "ring": mustDecode(p.Ring)})
		}
	}
	for _, r := range doc.Layers.Resources {
		put(CollMapResources, r.ID, Record{
			FieldID: r.ID, "kind": r.Kind, "name": r.Name, "capacity": float64(r.Capacity), "point_id": emptyNil(r.PointID),
			"point_ids": strList(r.PointIDs), "edge_ids": strList(r.EdgeIDs),
		})
	}
	for _, f := range doc.Layers.Flows {
		id := f.ID
		if id == "" {
			id = uuid.NewSHA1(flowNamespace, []byte(f.ProcessCode)).String()
		}
		put(CollMapFlows, id, Record{
			FieldID: id, "process_code": f.ProcessCode, "pickup_point_ids": strList(f.PickupPointIDs), "drop_point_ids": strList(f.DropPointIDs),
		})
	}
	return nil
}

// ToDraft rebuilds the draft and the project name. CatalogRevisionID is left for the caller.
func ToDraft(st State) (projects.Draft, string, error) {
	proj := st.Get(CollProject, CollProject)
	if proj == nil {
		return projects.Draft{}, "", fmt.Errorf("ops.to: no project record")
	}
	var d projects.Draft
	d.SchemaVersion = projects.DraftSchemaVersion
	d.ObjectType = proj.String("object_type")
	var err error
	if d.Params, err = json.Marshal(proj["params"]); err != nil {
		return projects.Draft{}, "", fmt.Errorf("ops.to.params: %w", err)
	}
	d.MatchSelectedIDs = toStrings(proj["match_selected_ids"])
	d.ReviewedTabs = toStrings(proj["reviewed_tabs"])
	if proj["econ_overrides"] != nil {
		if d.EconOverrides, err = json.Marshal(proj["econ_overrides"]); err != nil {
			return projects.Draft{}, "", fmt.Errorf("ops.to.econ_overrides: %w", err)
		}
	}

	d.Processes = []projects.Process{}
	for i, r := range st.Sorted(CollProcesses) {
		p := projects.Process{
			ID: r.String(FieldID), Order: r.String(FieldOrder), SortOrder: i, Code: r.String("code"), Name: r.String("name"),
			TaskType: r.String("task_type"), PointIDs: toStrings(r["point_ids"]),
		}
		p.IsBaseline, _ = r["is_baseline"].(bool)
		if err := remarshal(r["demand"], &p.Demand); err != nil {
			return projects.Draft{}, "", err
		}
		if err := remarshal(r["sla"], &p.SLA); err != nil {
			return projects.Draft{}, "", err
		}
		if err := remarshal(r["durations"], &p.Durations); err != nil {
			return projects.Draft{}, "", err
		}
		if err := remarshal(r["baseline_staff"], &p.BaselineStaff); err != nil {
			return projects.Draft{}, "", err
		}
		d.Processes = append(d.Processes, p)
	}

	fleetBy := map[string][]Record{}
	for _, r := range st.Sorted(CollFleetItems) {
		fleetBy[r.String("variant_id")] = append(fleetBy[r.String("variant_id")], r)
	}
	finBy := map[string][]Record{}
	for _, id := range st.IDs(CollFinancing) {
		r := st[CollFinancing][id]
		finBy[r.String("variant_id")] = append(finBy[r.String("variant_id")], r)
	}
	d.Variants = []projects.Variant{}
	for i, r := range st.Sorted(CollVariants) {
		v := projects.Variant{
			ID: r.String(FieldID), Order: r.String(FieldOrder), SortOrder: i, Name: r.String("name"),
			Status: r.String("status"), Notes: r.String("notes"), Fleet: []projects.FleetItem{}, Financing: []projects.Financing{},
		}
		for j, f := range fleetBy[v.ID] {
			item := projects.FleetItem{
				ID: f.String(FieldID), Order: f.String(FieldOrder), SortOrder: j, SolutionID: toStrPtr(f["solution_id"]),
				Quantity: toInt(f["quantity"]), PriceOverrideRub: toFloatPtr(f["price_override_rub"]),
				PriceOverrideReason: f.String("price_override_reason"), TaskCodes: toStrings(f["task_codes"]),
			}
			v.Fleet = append(v.Fleet, item)
		}
		fins := finBy[v.ID]
		sort.SliceStable(fins, func(a, b int) bool {
			if fins[a].String("kind") != fins[b].String("kind") {
				return fins[a].String("kind") < fins[b].String("kind")
			}
			ta, tb := fins[a]["tariff"], fins[b]["tariff"]
			if (ta == nil) != (tb == nil) {
				return tb == nil
			}
			return fins[a].String("tariff") < fins[b].String("tariff")
		})
		for _, f := range fins {
			ass, err := json.Marshal(f["assumptions"])
			if err != nil {
				return projects.Draft{}, "", fmt.Errorf("ops.to.financing: %w", err)
			}
			v.Financing = append(v.Financing, projects.Financing{ID: f.String(FieldID), Kind: f.String("kind"), Tariff: toStrPtr(f["tariff"]), Assumptions: ass})
		}
		d.Variants = append(d.Variants, v)
	}

	d.SharedCosts = []projects.SharedCost{}
	for i, r := range st.Sorted(CollSharedCosts) {
		d.SharedCosts = append(d.SharedCosts, projects.SharedCost{
			ID: r.String(FieldID), Order: r.String(FieldOrder), SortOrder: i, Code: r.String("code"), Label: r.String("label"),
			Bucket: r.String("bucket"), Rub: toFloat(r["rub"]),
		})
	}
	active, _ := proj["active_assumption_set_id"].(string)
	d.AssumptionSets = []projects.AssumptionSet{}
	for i, r := range st.Sorted(CollAssumptionSets) {
		a := projects.AssumptionSet{
			ID: r.String(FieldID), Order: r.String(FieldOrder), SortOrder: i, Name: r.String("name"),
			VATRate: toFloat(r["vat_rate"]), LaborCashShare: toFloat(r["labor_cash_share"]), DiscountRate: projects.DiscountOrDefault(toFloat(r["discount_rate"])), IsActive: r.String(FieldID) == active,
			Utilization: toFloatPtr(r["utilization"]), Availability: toFloatPtr(r["availability"]), Reserve: toFloatPtr(r["reserve"]),
			ServiceShare: toFloatPtr(r["service_share"]), DeliveryShare: toFloatPtr(r["delivery_share"]),
			CommRubPerRobotYear: toFloatPtr(r["comm_rub_per_robot_year"]), TechnicianWageMonthRub: toFloatPtr(r["technician_wage_month_rub"]),
		}
		a.PricesIncludeVAT, _ = r["prices_include_vat"].(bool)
		a.VATRecoverable, _ = r["vat_recoverable"].(bool)
		if a.IsActive {
			d.ActiveAssumptionSetID = a.ID
		}
		d.AssumptionSets = append(d.AssumptionSets, a)
	}

	if d.Map, err = toMap(st); err != nil {
		return projects.Draft{}, "", err
	}
	return d, proj.String("name"), nil
}

// ToMap builds the map document without server messages, or nil when the project has no map.
func toMap(st State) (json.RawMessage, error) {
	m := st.Get(CollMap, CollMap)
	if m == nil || m["page"] == nil {
		return nil, nil
	}
	doc := maps.Document{SchemaVersion: maps.SchemaVersion, Profile: m.String("profile"), Units: "m"}
	var page maps.Page
	if err := remarshal(m["page"], &page); err != nil {
		return nil, err
	}
	doc.Page = &page
	doc.Calibration.MetersPerPx = toFloat(m["meters_per_px"])
	var err error
	if doc.Calibration.Segment, err = toSegment(m["segment"]); err != nil {
		return nil, err
	}
	if doc.Calibration.Check, err = toSegment(m["check"]); err != nil {
		return nil, err
	}
	l := &doc.Layers
	l.Points = []maps.PointFeature{}
	for _, id := range st.IDs(CollMapPoints) {
		r := st[CollMapPoints][id]
		pos, _ := r["pos"].(map[string]any)
		l.Points = append(l.Points, maps.PointFeature{
			ID: id, Kind: r.String("kind"), Name: r.String("name"), X: toFloat(pos["x"]), Y: toFloat(pos["y"]), ProcessCode: r.String("process_code"),
		})
	}
	l.Edges = []maps.Edge{}
	for _, id := range st.IDs(CollMapEdges) {
		r := st[CollMapEdges][id]
		e := maps.Edge{ID: id, From: r.String("from"), To: r.String("to"), WidthM: toFloatPtr(r["width_m"])}
		if b, ok := r["bidirectional"].(bool); ok {
			e.Bidirectional = &b
		}
		l.Edges = append(l.Edges, e)
	}
	for _, pair := range []struct {
		coll string
		dst  *[]maps.Polygon
	}{{CollMapZones, &l.Zones}, {CollMapObstacles, &l.Obstacles}} {
		*pair.dst = []maps.Polygon{}
		for _, r := range st.Sorted(pair.coll) {
			p := maps.Polygon{ID: r.String(FieldID), Kind: r.String("kind"), Name: r.String("name")}
			if err := remarshal(r["ring"], &p.Ring); err != nil {
				return nil, err
			}
			*pair.dst = append(*pair.dst, p)
		}
	}
	l.Resources = []maps.Resource{}
	for _, id := range st.IDs(CollMapResources) {
		r := st[CollMapResources][id]
		l.Resources = append(l.Resources, maps.Resource{
			ID: id, Kind: r.String("kind"), Name: r.String("name"), Capacity: toInt(r["capacity"]), PointID: r.String("point_id"),
			PointIDs: nilIfEmpty(toStrings(r["point_ids"])), EdgeIDs: nilIfEmpty(toStrings(r["edge_ids"])),
		})
	}
	flows := st.IDs(CollMapFlows)
	sort.SliceStable(flows, func(i, j int) bool {
		return st[CollMapFlows][flows[i]].String("process_code") < st[CollMapFlows][flows[j]].String("process_code")
	})
	for _, id := range flows {
		r := st[CollMapFlows][id]
		l.Flows = append(l.Flows, maps.Flow{
			ID: id, ProcessCode: r.String("process_code"),
			PickupPointIDs: toStrings(r["pickup_point_ids"]), DropPointIDs: toStrings(r["drop_point_ids"]),
		})
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("ops.to.map: %w", err)
	}
	return raw, nil
}

// orderKeys keeps stored keys when they are valid, distinct and agree with sort_order; otherwise it
// assigns fresh keys in sort_order.
func orderKeys(n int, at func(i int) (string, int)) []string {
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		_, sa := at(idx[a])
		_, sb := at(idx[b])
		return sa < sb
	})
	keys := make([]string, n)
	ok := true
	prev := ""
	for _, i := range idx {
		k, _ := at(i)
		if !ValidOrderKey(k) || (prev != "" && k <= prev) {
			ok = false
			break
		}
		keys[i] = k
		prev = k
	}
	if ok {
		return keys
	}
	fresh, _ := KeysAfter("", n)
	for rank, i := range idx {
		keys[i] = fresh[rank]
	}
	return keys
}

func decodeObject(raw json.RawMessage) (map[string]any, error) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || bytes.Equal(t, []byte("null")) {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(t, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func decodeRaw(raw json.RawMessage) (any, error) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || bytes.Equal(t, []byte("null")) {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(t, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func mustDecode(v any) any {
	out, err := Decode(v)
	if err != nil {
		panic(err)
	}
	return out
}

func remarshal(v any, dst any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("ops.to: %w", err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("ops.to: %w", err)
	}
	return nil
}

func segmentValue(s *maps.Segment) any {
	if s == nil {
		return nil
	}
	return mustDecode(s)
}

func toSegment(v any) (*maps.Segment, error) {
	if v == nil {
		return nil, nil
	}
	var s maps.Segment
	if err := remarshal(v, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func strList(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

func toStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

func emptyNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func strPtr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func floatPtr(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func toStrPtr(v any) *string {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	return &s
}

func toFloatPtr(v any) *float64 {
	f, ok := v.(float64)
	if !ok {
		return nil
	}
	return &f
}

func toFloat(v any) float64 {
	f, _ := v.(float64)
	return f
}

func toInt(v any) int {
	f, _ := v.(float64)
	return int(f)
}
