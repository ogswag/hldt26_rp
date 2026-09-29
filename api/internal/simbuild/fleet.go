package simbuild

import (
	"fmt"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/engine"
	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/sim"
	"moscow_hackathon_2026/api/internal/sim/profiles"
)

// Robot is what a run needs to know about one catalog robot.
type Robot struct {
	ID           string
	Name         string
	Family       string
	Kind         *string
	Subtype      *string
	Capabilities []string
	Specs        profiles.Specs
}

// Robots is the catalog as a run reads it, by solution id.
type Robots map[string]Robot

// NewRobot joins a candidate with its priced robot, which carries the specs.
func NewRobot(c matching.Candidate, r econ.Robot) Robot {
	return Robot{
		ID: c.ID, Name: c.Name, Family: c.Family, Kind: c.Kind, Subtype: c.Subtype, Capabilities: c.CapabilityCodes,
		Specs: profiles.Specs{
			SpeedMps: r.SpeedMps, WidthMm: r.WidthMm, LengthMm: r.LengthMm,
			PayloadKg: r.PayloadKg, EnduranceH: r.EnduranceH, ChargeMin: r.ChargeMin,
		},
	}
}

// RobotsOf joins the candidates and the priced robots of an engine catalog by id.
func RobotsOf(cat engine.Catalog) Robots {
	priced := make(map[string]econ.Robot, len(cat.Robots))
	for _, r := range cat.Robots {
		priced[r.ID] = r
	}
	out := make(Robots, len(cat.Candidates))
	for _, c := range cat.Candidates {
		out[c.ID] = NewRobot(c, priced[c.ID])
	}
	return out
}

// SolutionIDs lists the robots a run of the draft may need: every fleet line of every variant, and the suggestion.
func SolutionIDs(d projects.Draft, s *Suggestion) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, v := range d.Variants {
		for _, f := range v.Fleet {
			if f.SolutionID != nil {
				add(*f.SolutionID)
			}
		}
	}
	if s != nil {
		add(s.SolutionID)
	}
	return out
}

// HasFleet says whether any variant of the draft has a robot in it.
func HasFleet(d projects.Draft) bool {
	for _, v := range d.Variants {
		if variantHasFleet(v) {
			return true
		}
	}
	return false
}

func variantHasFleet(v projects.Variant) bool {
	for _, f := range v.Fleet {
		if f.SolutionID != nil && *f.SolutionID != "" && f.Quantity > 0 {
			return true
		}
	}
	return false
}

func jobVariant(d projects.Draft, cfg JobConfig) (projects.Variant, error) {
	var v projects.Variant
	var err error
	if s := cfg.Suggestion; s != nil {
		id := s.SolutionID
		v = projects.Variant{Name: projects.SuggestionName, Fleet: []projects.FleetItem{{SolutionID: &id, Quantity: s.Quantity, TaskCodes: s.ProcessCodes}}}
	} else {
		v, err = pickVariant(d, cfg.VariantID)
		if err != nil {
			return v, err
		}
	}
	if len(cfg.FleetQuantities) == 0 {
		return v, nil
	}
	fleet := make([]projects.FleetItem, len(v.Fleet))
	copy(fleet, v.Fleet)
	for i := range fleet {
		key := fleetKey(fleet[i], i)
		if q, ok := cfg.FleetQuantities[key]; ok && q > 0 {
			fleet[i].Quantity = q
		}
	}
	v.Fleet = fleet
	return v, nil
}

func fleetKey(f projects.FleetItem, i int) string {
	if f.ID != "" {
		return f.ID
	}
	return fmt.Sprintf("fleet-%d", i+1)
}

func pickVariant(d projects.Draft, id string) (projects.Variant, error) {
	if id != "" {
		for _, v := range d.Variants {
			if v.ID == id {
				if !variantHasFleet(v) {
					return v, &sim.InputError{Msg: fmt.Sprintf("В варианте «%s» нет роботов. Выберите вариант с флотом.", v.Name)}
				}
				return v, nil
			}
		}
		return projects.Variant{}, &sim.InputError{Msg: "Вариант не найден в проекте. Обновите страницу и выберите вариант заново."}
	}
	for _, v := range d.Variants {
		if variantHasFleet(v) {
			return v, nil
		}
	}
	return projects.Variant{}, &sim.InputError{Msg: "Ни в одном варианте нет роботов. Выберите робота на вкладке Роботы."}
}

// FleetFor resolves variant fleet lines into robot profiles from the catalog.
func FleetFor(robots Robots, v projects.Variant) ([]sim.FleetItem, []string, error) {
	var out []sim.FleetItem
	var warnings []string
	for i, f := range v.Fleet {
		if f.SolutionID == nil || *f.SolutionID == "" || f.Quantity < 1 {
			continue
		}
		sid, err := uuid.Parse(*f.SolutionID)
		if err != nil {
			return nil, nil, &sim.InputError{Msg: fmt.Sprintf("Позиция флота %d: некорректный идентификатор решения.", i+1)}
		}
		robot, ok := robots[sid.String()]
		if !ok {
			return nil, nil, &sim.InputError{Msg: fmt.Sprintf("Позиция флота %d: робота нет в каталоге. Уберите позицию из флота.", i+1)}
		}
		code, ok := profiles.Classify(robot.Name, robot.Family, deref(robot.Subtype), deref(robot.Kind), robot.Capabilities)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s: этот класс роботов симуляция пока не моделирует, позиция пропущена.", robot.Name))
			continue
		}
		key := fleetKey(f, i)
		out = append(out, sim.FleetItem{
			Key:        key,
			SolutionID: robot.ID,
			Name:       robot.Name,
			Profile:    profiles.Resolve(code, robot.Specs),
			Quantity:   f.Quantity,
			Serves:     f.TaskCodes,
		})
	}
	if len(out) == 0 {
		return nil, warnings, &sim.InputError{Msg: "В варианте нет роботов, которые умеет моделировать симуляция (AMR и паллетные роботы)."}
	}
	return out, warnings, nil
}

// Classes returns footprint specs for map checks.
func Classes(fleet []sim.FleetItem) []maps.ClassSpec {
	seen := map[string]bool{}
	var out []maps.ClassSpec
	for _, f := range fleet {
		key := fmt.Sprintf("%s|%.3f|%.3f", f.Profile.Code, f.Profile.WidthM, f.Profile.ClearanceM)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, maps.ClassSpec{
			Code:       f.Profile.Code,
			Label:      fmt.Sprintf("%s (%s)", f.Name, f.Profile.Label),
			WidthM:     f.Profile.WidthM,
			ClearanceM: f.Profile.ClearanceM,
		})
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
