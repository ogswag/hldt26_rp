// Package simbuild turns a project draft and a catalog into a runnable sim-v2 model. It touches no database and
// no HTTP, so the API worker and the browser engine build a run the same way.
package simbuild

import (
	"errors"
	"fmt"
	"strings"

	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/rutext"
	"moscow_hackathon_2026/api/internal/sim"
	"moscow_hackathon_2026/api/internal/sim/profiles"
)

const (
	MapSourceProject  = "project"
	MapSourceTemplate = "template"

	ConfidencePreliminary = projects.ConfidencePreliminary
	ConfidenceConfigured  = projects.ConfidenceConfigured

	defaultPieceUnitsPerJob = 40
)

const (
	ModeSimulation  = ""
	ModeFleetSearch = "fleet_search"
)

// JobConfig is stored with the job and replayed by the worker.
type JobConfig struct {
	VariantID string     `json:"variant_id"`
	Sim       sim.Config `json:"sim"`
	// Suggestion is the robot a calculation suggested, run when no variant of the project has a fleet.
	Suggestion *Suggestion `json:"suggestion,omitempty"`
	// Mode fleet_search tries quantities of one fleet line. LineKey names that line.
	Mode            string         `json:"mode,omitempty"`
	LineKey         string         `json:"line_key,omitempty"`
	FleetQuantities map[string]int `json:"fleet_quantities,omitempty"`
}

// Suggestion is one robot, how many of them the calculation needs and the processes they serve.
type Suggestion struct {
	SolutionID   string   `json:"solution_id"`
	Quantity     int      `json:"quantity"`
	ProcessCodes []string `json:"process_codes,omitempty"`
}

// MapError carries blocking map issues.
type MapError struct {
	Issues []maps.Issue
}

func (e *MapError) Error() string {
	errs, _ := maps.Messages(e.Issues)
	return "Карта содержит ошибки: " + strings.Join(errs, " ")
}

// Built is a prepared model with the context needed to explain it.
// Warnings come from the builder; the model keeps its own.
type Built struct {
	Model     *sim.Model
	Scenario  sim.Scenario
	Variant   projects.Variant
	MapSource string
	MapIssues []maps.Issue
	Warnings  []string
	// VariantHash names the inputs of the variant this run simulates (VariantHash).
	VariantHash string
	Confidence  string
}

// VariantHash names what a simulation of one variant reads: the object parameters, the processes, the map and the
// fleet of that variant. A run stays current while the hash of its variant is the same.
func VariantHash(d projects.Draft, variantID string) (string, error) {
	return projects.SimInputHash(d, variantID, sim.Version)
}

// AllWarnings joins builder and model warnings.
func (b *Built) AllWarnings() []string {
	return append(append([]string{}, b.Warnings...), b.Model.Warnings()...)
}

// ProcessCodes lists draft process codes in order.
func ProcessCodes(d projects.Draft) []string {
	out := make([]string, 0, len(d.Processes))
	for _, p := range d.Processes {
		out = append(out, p.Code)
	}
	return out
}

// ProcessNames maps each draft process code to its name, for messages about the map.
func ProcessNames(d projects.Draft) map[string]string {
	out := make(map[string]string, len(d.Processes))
	for _, p := range d.Processes {
		out[p.Code] = p.Name
	}
	return out
}

// Template returns the starter map for a draft.
func Template(d projects.Draft) maps.Document {
	sp := readParams(d.Params)
	return maps.WarehouseTemplate(ProcessCodes(d), maps.TemplateWidths{MainM: sp.mainAisle, WorkingM: sp.workAisle})
}

// DefaultWidthM is the aisle width assumed for edges without a declared or measured width.
func DefaultWidthM(d projects.Draft) float64 {
	if w := readParams(d.Params).mainAisle; w > 0 {
		return w
	}
	return maps.DefaultAisleWidthM
}

// MapFor returns the draft map or the template when the draft has none.
func MapFor(d projects.Draft) (maps.Document, string, error) {
	if maps.IsEmpty(d.Map) {
		return Template(d), MapSourceTemplate, nil
	}
	doc, err := maps.Decode(d.Map)
	if err != nil {
		return maps.Document{}, "", &sim.InputError{Msg: "Карта проекта не читается. Откройте редактор карты и сохраните её заново."}
	}
	return doc, MapSourceProject, nil
}

// Build prepares a runnable model from a draft or snapshot.
func Build(robots Robots, objectType string, d projects.Draft, cfg JobConfig) (*Built, error) {
	if objectType != "warehouse" {
		return nil, &sim.InputError{Msg: "Симуляция пока доступна только для склада."}
	}
	simCfg, err := cfg.Sim.Normalize()
	if err != nil {
		return nil, err
	}
	variant, err := jobVariant(d, cfg)
	if err != nil {
		return nil, err
	}
	variantHash, err := VariantHash(d, variant.ID)
	if err != nil {
		return nil, err
	}
	fleet, warnings, err := FleetFor(robots, variant)
	if err != nil {
		return nil, err
	}
	doc, source, err := MapFor(d)
	if err != nil {
		return nil, err
	}
	codes := ProcessCodes(d)
	issues := maps.Validate(doc, maps.Context{ProcessCodes: codes, ProcessNames: ProcessNames(d), Classes: Classes(fleet), DefaultWidthM: DefaultWidthM(d)})
	if maps.HasErrors(issues) {
		return nil, &MapError{Issues: issues}
	}
	if source == MapSourceTemplate {
		warnings = append(warnings, "У проекта нет своей карты, использован шаблон склада 84 x 52 м. Разметьте план объекта.")
	}
	g, scene := maps.Build(doc, DefaultWidthM(d))
	sp := readParams(d.Params)
	active, note := sp.activeHours()
	if note != "" {
		warnings = append(warnings, note)
	}
	if sp.liftAssumed {
		warnings = append(warnings, fmt.Sprintf("Высота потолка не задана, средняя высота подъёма принята %s м.", rutext.Num(float64(defaultLiftM), 0)))
	}
	flows := map[string]maps.Flow{}
	for _, f := range doc.Layers.Flows {
		flows[f.ProcessCode] = f
	}
	var procs []sim.ProcessSpec
	for _, p := range d.Processes {
		ps := sim.ProcessSpec{
			Code:           p.Code,
			Name:           p.Name,
			TaskType:       p.TaskType,
			Unit:           p.Demand.Unit,
			UnitsPerDay:    p.Demand.UnitsPerDay,
			UnitsPerJob:    p.Demand.UnitsPerJob,
			MaxWaitS:       p.SLA.MaxWaitMin * 60,
			MaxCycleS:      p.SLA.MaxCycleMin * 60,
			Priority:       p.SLA.Priority,
			StationLoadS:   p.Durations.LoadS,
			StationUnloadS: p.Durations.UnloadS,
		}
		if ps.UnitsPerJob <= 0 {
			ps.UnitsPerJob = 1
			if p.TaskType == profiles.TaskPiecePick {
				ps.UnitsPerJob = defaultPieceUnitsPerJob
			}
		}
		if f, ok := flows[p.Code]; ok {
			ps.Pickups = f.PickupPointIDs
			ps.Drops = f.DropPointIDs
		}
		procs = append(procs, ps)
	}
	sc := sim.Scenario{
		Config:            simCfg,
		Graph:             g,
		Scene:             scene,
		Resources:         doc.Layers.Resources,
		Processes:         procs,
		Fleet:             fleet,
		ActiveHoursPerDay: active,
		PeakFactor:        sp.peak,
		Windows:           sim.ResolveWindows(simCfg, active),
		LiftM:             sp.liftM,
	}
	model, err := sim.Prepare(sc)
	if err != nil {
		return nil, err
	}
	confidence := ConfidencePreliminary
	if source == MapSourceProject {
		confidence = ConfidenceConfigured
	}
	return &Built{
		Model:       model,
		Scenario:    sc,
		Variant:     variant,
		MapSource:   source,
		MapIssues:   issues,
		Warnings:    warnings,
		VariantHash: variantHash,
		Confidence:  confidence,
	}, nil
}

// UserMessage returns the text for errors a user can fix, and false for internal faults.
func UserMessage(err error) (string, bool) {
	var ie *sim.InputError
	if errors.As(err, &ie) {
		return ie.Msg, true
	}
	var me *MapError
	if errors.As(err, &me) {
		return me.Error(), true
	}
	return "", false
}
