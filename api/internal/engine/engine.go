// Package engine runs matching, economics and the verification simulation. It takes the catalog it needs as
// data, so the same code serves the API, a worker and the browser build. Nothing here touches the database,
// HTTP or the clock.
package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"moscow_hackathon_2026/api/internal/catalog"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/rutext"
	"moscow_hackathon_2026/api/internal/sim/geom"
	"moscow_hackathon_2026/api/internal/sim/profiles"
)

// CatalogFormat changes when the bundle gains fields, so a browser holding an older bundle of the same content
// takes the new one.
const CatalogFormat = "3"

// Catalog is what matching picks from and what economics prices. ContentSHA256 names its content.
type Catalog struct {
	ContentSHA256 string               `json:"content_sha256"`
	NormsSHA256   string               `json:"norms_sha256,omitempty"`
	Norms         econ.Norms           `json:"norms,omitempty"`
	Candidates    []matching.Candidate `json:"candidates"`
	Robots        []econ.Robot         `json:"robots"`
}

// Request is one calculation. Draft is empty for the guest path, which has no processes or variants.
type Request struct {
	ObjectType string          `json:"object_type"`
	Params     json.RawMessage `json:"params"`
	IncludeIDs []string        `json:"include_ids,omitempty"`
	TaskCodes  []string        `json:"task_codes,omitempty"`
	Seed       int             `json:"seed"`
	Overrides  econ.Overrides  `json:"overrides,omitempty"`
	Draft      projects.Draft  `json:"draft,omitempty"`
	HasDraft   bool            `json:"has_draft,omitempty"`
}

// OverridesOf returns the what-if of a request: its own, else the one stored in the draft.
func OverridesOf(own *econ.Overrides, d projects.Draft) (econ.Overrides, error) {
	if own != nil {
		return *own, nil
	}
	ov := econ.Overrides{}
	if len(d.EconOverrides) > 0 && string(d.EconOverrides) != "null" {
		if err := json.Unmarshal(d.EconOverrides, &ov); err != nil {
			return ov, fmt.Errorf("engine.overrides: %w", err)
		}
	}
	return ov, nil
}

// Match picks robots for the site. An empty catalog answers with an empty result, as a server without a
// catalog does.
func Match(cat Catalog, objectType string, params json.RawMessage, includeIDs, taskCodes []string) (matching.Output, error) {
	if len(cat.Candidates) == 0 {
		out := matching.Empty()
		stamp(&out, cat)
		out.TaskCodes = taskCodes
		return out, nil
	}
	site, err := matching.SiteFromParams(objectType, params)
	if err != nil {
		return matching.Output{}, err
	}
	site.TaskCodes = taskCodes
	out := matching.Match(site, cat.Candidates, includeIDs)
	stamp(&out, cat)
	return out, nil
}

func stamp(out *matching.Output, cat Catalog) {
	out.CatalogContentSHA256 = cat.ContentSHA256
	out.NormsSHA256 = cat.NormsSHA256
}

// Calculate matches, prices and verifies with the simulation. The robot it returns is the one the simulation
// ran on, or nil when nothing was picked.
func Calculate(cat Catalog, req Request) (econ.Result, *econ.Robot, error) {
	taskCodes := req.TaskCodes
	if len(taskCodes) == 0 {
		taskCodes = TaskCodes(req.ObjectType, req.Draft.Processes, nil)
	}
	out, err := Match(cat, req.ObjectType, req.Params, req.IncludeIDs, taskCodes)
	if err != nil {
		return econ.Result{}, nil, err
	}
	processes := toProcessSpecs(req.Draft.Processes)
	assumptions := econ.AssumptionValues{}
	if req.HasDraft {
		assumptions = toAssumptionValues(projects.ActiveAssumptionSet(req.Draft.AssumptionSets))
	}
	robots := cat.Robots
	if req.ObjectType == "warehouse" {
		robots = withSimTasks(cat)
	}
	out = econ.Rank(econ.Input{
		ObjectType:  req.ObjectType,
		Params:      req.Params,
		Overrides:   req.Overrides,
		Seed:        req.Seed,
		Match:       out,
		Processes:   processes,
		Assumptions: assumptions,
		Norms:       cat.Norms,
	}, robots)
	if req.HasDraft && draftHasFleet(req.Draft) {
		return calculateDraft(cat, req, out)
	}
	// A project that has picked no robot runs on the suggestion, counted as a variant of it alone, so Итог shows
	// the same case as the suggestion on Роботы and as the variant it becomes once picked.
	if v, ok := suggestionVariant(out); ok && req.HasDraft && len(processes) > 0 && len(out.SelectedIDs) == 0 {
		sreq := req
		sreq.Draft.Variants = []projects.Variant{v}
		return calculateDraft(cat, sreq, out)
	}
	res, robot, err := calculateFromMatch(cat, req, out, processes, assumptions)
	if err != nil {
		return econ.Result{}, nil, err
	}
	verify(&res, req, robot)
	return res, robot, nil
}

func calculateFromMatch(cat Catalog, req Request, out matching.Output, processes []econ.ProcessSpec, assumptions econ.AssumptionValues) (econ.Result, *econ.Robot, error) {
	id, reason := econ.PickSolution(out)
	robot := robotByID(cat.Robots, id)
	res, err := econ.Calculate(econ.Input{
		ObjectType:  req.ObjectType,
		Params:      req.Params,
		Robot:       robot,
		PickReason:  reason,
		Overrides:   req.Overrides,
		Seed:        req.Seed,
		Match:       out,
		Processes:   processes,
		Assumptions: assumptions,
		Norms:       cat.Norms,
	})
	if err != nil {
		return econ.Result{}, nil, err
	}
	res.Match = out
	return res, robot, nil
}

func calculateDraft(cat Catalog, req Request, out matching.Output) (econ.Result, *econ.Robot, error) {
	d := req.Draft
	res, err := econ.Calculate(econ.Input{
		ObjectType:  req.ObjectType,
		Params:      req.Params,
		Robots:      cat.Robots,
		PickReason:  "selected",
		Overrides:   req.Overrides,
		Seed:        req.Seed,
		Match:       out,
		Processes:   toProcessSpecs(d.Processes),
		Variants:    toVariantSpecs(d.Variants),
		SharedCosts: toSharedSpecs(d.SharedCosts),
		Assumptions: toAssumptionValues(projects.ActiveAssumptionSet(d.AssumptionSets)),
		Norms:       cat.Norms,
	})
	if err != nil {
		return econ.Result{}, nil, err
	}
	res.Match = out
	verifyVariants(&res, req, cat.Robots)
	mapChargerRisks(&res, req)
	return res, robotByID(cat.Robots, firstFleetSolution(res)), nil
}

// mapChargerRisks warns when the warehouse map has fewer charging places than the fleet of a variant needs by its
// specs. A project without a map runs on the template, which the calculation does not judge.
func mapChargerRisks(res *econ.Result, req Request) {
	if req.ObjectType != "warehouse" || maps.IsEmpty(req.Draft.Map) {
		return
	}
	doc, err := maps.Decode(req.Draft.Map)
	if err != nil {
		return
	}
	places := 0
	for _, r := range doc.Layers.Resources {
		if r.Kind == maps.ResourceCharger {
			places += r.Capacity
		}
	}
	many := len(res.Variants) > 1
	for _, v := range res.Variants {
		if v.Chargers <= places {
			continue
		}
		lead := "На карте"
		if many {
			lead = v.Name + ": на карте"
		}
		text := fmt.Sprintf("%s %d %s, по ТТХ флота нужно %d. Добавьте зарядку на карту или проверьте время работы и зарядки роботов.",
			lead, places, rutext.Plural(places, "зарядка", "зарядки", "зарядок"), v.Chargers)
		res.Risks = append(res.Risks, econ.Risk{ID: "map_chargers:" + v.VariantID, Level: "warning", Text: text})
	}
}

// verify runs the quick check on the one robot of a calculation without variants. A warehouse has no quick check: its
// variants are checked by simulation runs on the project map (simbuild.Checks).
func verify(res *econ.Result, req Request, robot *econ.Robot) {
	if req.ObjectType == "warehouse" || res.Shared == nil || robot == nil {
		return
	}
	out := geom.Run(SimInput(req.ObjectType, req.Params, robot, *res, req.Seed, "buy"))
	name := res.SolutionName
	if name == "" {
		name = robot.Name
	}
	res.Sim = ToEconSim(out)
	res.SimChecks = []econ.SimCheck{quickCheck("", name, robot.Name, out)}
	res.VerificationFlag = out.VerificationFlag
	econ.ApplyVerification(res)
}

// verifyVariants runs the quick check on every fleet line of every variant and keeps the worst line of each variant.
func verifyVariants(res *econ.Result, req Request, robots []econ.Robot) {
	if req.ObjectType == "warehouse" {
		return
	}
	var worst *geom.Summary
	for _, v := range res.Variants {
		var line *geom.Summary
		robotName := ""
		for _, f := range v.Fleet {
			robot := robotByID(robots, f.SolutionID)
			if robot == nil || f.Throughput <= 0 || f.Quantity < 1 {
				continue
			}
			out := geom.Run(lineInput(req, robot, f))
			if line == nil || out.Divergence > line.Divergence {
				o := out
				line, robotName = &o, robot.Name
			}
		}
		if line == nil {
			continue
		}
		res.SimChecks = append(res.SimChecks, quickCheck(v.VariantID, v.Name, robotName, *line))
		if worst == nil || line.Divergence > worst.Divergence {
			worst = line
		}
	}
	if worst == nil {
		return
	}
	res.Sim = ToEconSim(*worst)
	res.VerificationFlag = econ.SimChecksFlag(res.SimChecks)
	econ.ApplyVerification(res)
}

// lineInput is the quick check of one fleet line: its robots alone, held to what the economics counts for them.
func lineInput(req Request, robot *econ.Robot, f econ.FleetKPI) geom.Input {
	in := geom.Input{
		ObjectType:   req.ObjectType,
		ScenarioKind: "buy",
		Params:       req.Params,
		Seed:         int64(req.Seed),
		WorkKind:     f.WorkKind,
		FleetSize:    f.Quantity,
		Throughput:   f.Throughput,
		PeakOps:      f.PeakOps,
		Unit:         f.Unit,
	}
	if robot.SpeedMps != nil {
		in.SpeedMps = *robot.SpeedMps
	}
	if robot.WidthMm != nil {
		in.WidthMm = *robot.WidthMm
	}
	if robot.EnduranceH != nil {
		in.EnduranceH = *robot.EnduranceH
	}
	if robot.ChargeMin != nil {
		in.ChargeMin = *robot.ChargeMin
	}
	return in
}

// quickCheck words the outcome of one quick run: the robot's simulated output against what the economics counts.
func quickCheck(variantID, variantName, robotName string, out geom.Summary) econ.SimCheck {
	value := 0.0
	if out.EconThroughput > 0 {
		value = out.Throughput / out.EconThroughput
	}
	text := fmt.Sprintf("%s: %s %s в час в симуляции против %s в экономике, расхождение %s.",
		robotName, rutext.Num(out.Throughput, 1), out.Unit, rutext.Num(out.EconThroughput, 1), rutext.Pct(out.Divergence*100, 1))
	if out.VerificationFlag {
		text += " Производительность из экономики нельзя считать подтверждённой."
	}
	return econ.SimCheck{
		VariantID:   variantID,
		VariantName: variantName,
		Model:       econ.SimCheckQuick,
		Value:       math.Round(value*1000) / 1000,
		Flag:        out.VerificationFlag,
		Text:        text,
	}
}

func robotByID(robots []econ.Robot, id string) *econ.Robot {
	if id == "" {
		return nil
	}
	for i := range robots {
		if robots[i].ID == id {
			r := robots[i]
			return &r
		}
	}
	return nil
}

func firstFleetSolution(res econ.Result) string {
	for _, v := range res.Variants {
		if len(v.Fleet) > 0 && len(v.Scenarios) > 0 {
			return v.Fleet[0].SolutionID
		}
	}
	return ""
}

// withSimTasks gives each robot the task types the warehouse simulation runs it on, the way a simulation job
// classifies its fleet (simbuild.FleetFor), so the ranking counts only processes the simulation can model with it.
func withSimTasks(cat Catalog) []econ.Robot {
	byID := make(map[string]matching.Candidate, len(cat.Candidates))
	for _, c := range cat.Candidates {
		byID[c.ID] = c
	}
	out := make([]econ.Robot, len(cat.Robots))
	for i, r := range cat.Robots {
		out[i] = r
		out[i].SimTasks = []string{}
		c := byID[r.ID]
		caps := c.CapabilityCodes
		if len(caps) == 0 {
			caps = catalog.DeriveCapabilities(catalog.SpecInput{
				Name: c.Name, Family: c.Family, Subtype: deref(c.Subtype), Scenario: deref(c.Scenario),
				ObjectTypes: c.ObjectTypes, PayloadKg: c.PayloadKg, WidthMm: c.WidthMm, MinAisleMm: c.MinAisleMm,
				TempMinC: c.TempMinC, TempMaxC: c.TempMaxC,
			})
		}
		if code, ok := profiles.Classify(r.Name, r.Family, deref(r.Subtype), deref(r.Kind), caps); ok {
			out[i].SimTasks = profiles.Resolve(code, profiles.Specs{
				SpeedMps: r.SpeedMps, WidthMm: r.WidthMm, PayloadKg: r.PayloadKg, EnduranceH: r.EnduranceH, ChargeMin: r.ChargeMin,
			}).Tasks
		}
	}
	return out
}

func suggestionVariant(out matching.Output) (projects.Variant, bool) {
	for _, it := range out.Items {
		if it.SolutionID != out.Best {
			continue
		}
		if it.Estimate == nil || len(it.Estimate.ProcessCodes) == 0 {
			return projects.Variant{}, false
		}
		id, fixed := it.SolutionID, "fixed"
		return projects.Variant{
			ID:     projects.SuggestionVariantID,
			Name:   projects.SuggestionName,
			Status: "draft",
			Fleet:  []projects.FleetItem{{SolutionID: &id, Quantity: it.Estimate.FleetSize, TaskCodes: it.Estimate.ProcessCodes}},
			Financing: []projects.Financing{
				{Kind: "buy", Assumptions: []byte(`{}`)},
				{Kind: "raas", Tariff: &fixed, Assumptions: []byte(`{}`)},
			},
		}, true
	}
	return projects.Variant{}, false
}

func draftHasFleet(d projects.Draft) bool {
	for _, v := range d.Variants {
		for _, f := range v.Fleet {
			if f.SolutionID != nil && *f.SolutionID != "" && f.Quantity > 0 {
				return true
			}
		}
	}
	return false
}

// SimInput is what the simulation needs from a finished calculation.
func SimInput(objectType string, params json.RawMessage, robot *econ.Robot, res econ.Result, seed int, scenario string) geom.Input {
	in := geom.Input{
		ObjectType:   objectType,
		ScenarioKind: scenario,
		Params:       params,
		Seed:         int64(seed),
	}
	if res.Shared != nil {
		in.WorkKind = res.Shared.WorkKind
		in.FleetSize = res.Shared.FleetSize
		in.Throughput = res.Shared.Throughput
		in.PeakOps = res.Shared.PeakOps
		in.Unit = res.Shared.Unit
	}
	if robot != nil {
		if robot.SpeedMps != nil {
			in.SpeedMps = *robot.SpeedMps
		}
		if robot.WidthMm != nil {
			in.WidthMm = *robot.WidthMm
		}
		if robot.EnduranceH != nil {
			in.EnduranceH = *robot.EnduranceH
		}
		if robot.ChargeMin != nil {
			in.ChargeMin = *robot.ChargeMin
		}
	}
	return in
}

func ToEconSim(s geom.Summary) *econ.SimSummary {
	return &econ.SimSummary{
		Throughput:     s.Throughput,
		EconThroughput: s.EconThroughput,
		Divergence:     s.Divergence,
		DeliveredOpsH:  s.DeliveredOpsH,
		QueueWaitS:     s.QueueWaitS,
		ChargeShare:    s.ChargeShare,
		SimulatedS:     s.SimulatedS,
		Bottleneck:     s.Bottleneck,
		Unit:           s.Unit,
		Assumptions:    s.Assumptions,
	}
}

// TaskCodes is what the project asks robots to do: the explicit list, the task types of its processes, or the
// default processes of the object type.
func TaskCodes(objectType string, processes []projects.Process, explicit []string) []string {
	if len(explicit) > 0 {
		return explicit
	}
	if len(processes) > 0 {
		seen := make(map[string]struct{}, len(processes))
		out := make([]string, 0, len(processes))
		for _, p := range processes {
			code := strings.TrimSpace(p.TaskType)
			if code == "" {
				continue
			}
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			out = append(out, code)
		}
		return out
	}
	if objectType != "warehouse" {
		return nil
	}
	defs := projects.DefaultProcesses(objectType)
	out := make([]string, 0, len(defs))
	for _, p := range defs {
		out = append(out, p.TaskType)
	}
	return out
}

func toProcessSpecs(items []projects.Process) []econ.ProcessSpec {
	out := make([]econ.ProcessSpec, 0, len(items))
	for _, p := range items {
		out = append(out, econ.ProcessSpec{
			Code:           p.Code,
			Name:           p.Name,
			TaskType:       p.TaskType,
			IsBaseline:     p.IsBaseline,
			UnitsPerDay:    p.Demand.UnitsPerDay,
			DemandUnit:     p.Demand.Unit,
			StaffHeadcount: p.BaselineStaff.Headcount,
			StaffRole:      p.BaselineStaff.Role,
		})
	}
	return out
}

func toVariantSpecs(items []projects.Variant) []econ.VariantSpec {
	out := make([]econ.VariantSpec, 0, len(items))
	for _, v := range items {
		vs := econ.VariantSpec{ID: v.ID, Name: v.Name}
		for _, f := range v.Fleet {
			sid := ""
			if f.SolutionID != nil {
				sid = *f.SolutionID
			}
			vs.Fleet = append(vs.Fleet, econ.FleetSpec{
				SolutionID:          sid,
				Quantity:            f.Quantity,
				TaskCodes:           f.TaskCodes,
				PriceOverrideRub:    f.PriceOverrideRub,
				PriceOverrideReason: f.PriceOverrideReason,
			})
		}
		for _, f := range v.Financing {
			tariff := ""
			if f.Tariff != nil {
				tariff = *f.Tariff
			}
			vs.Financing = append(vs.Financing, econ.FinancingSpec{Kind: f.Kind, Tariff: tariff, Assumptions: f.Assumptions})
		}
		out = append(out, vs)
	}
	return out
}

func toSharedSpecs(items []projects.SharedCost) []econ.SharedCostSpec {
	out := make([]econ.SharedCostSpec, 0, len(items))
	for _, c := range items {
		out = append(out, econ.SharedCostSpec{Code: c.Code, Label: c.Label, Bucket: c.Bucket, Rub: c.Rub})
	}
	return out
}

func toAssumptionValues(a projects.AssumptionSet) econ.AssumptionValues {
	vat := a.VATRate
	inc := a.PricesIncludeVAT
	share := a.LaborCashShare
	discount := a.DiscountRate
	return econ.AssumptionValues{
		ID:               a.ID,
		Name:             a.Name,
		VATRate:          &vat,
		PricesIncludeVAT: &inc,
		VATRecoverable:   a.VATRecoverable,
		LaborCashShare:   &share,
		DiscountRate:     &discount,

		Utilization:            a.Utilization,
		Availability:           a.Availability,
		Reserve:                a.Reserve,
		ServiceShare:           a.ServiceShare,
		DeliveryShare:          a.DeliveryShare,
		CommRubPerRobotYear:    a.CommRubPerRobotYear,
		TechnicianWageMonthRub: a.TechnicianWageMonthRub,
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
