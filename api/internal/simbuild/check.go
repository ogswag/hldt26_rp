package simbuild

import (
	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/projects"
)

// MapEdge is one road segment as the map panel shows it.
type MapEdge struct {
	ID             string   `json:"id"`
	LengthM        float64  `json:"length_m"`
	WidthM         float64  `json:"width_m"`
	DeclaredWidthM *float64 `json:"declared_width_m,omitempty"`
	MeasuredWidthM *float64 `json:"measured_width_m,omitempty"`
	SingleLane     bool     `json:"single_lane"`
	Blocked        []string `json:"blocked_for,omitempty"`
}

// MapClass is a robot class the map is checked against.
type MapClass struct {
	Code           string  `json:"code"`
	Label          string  `json:"label"`
	RequiredWidthM float64 `json:"required_width_m"`
	TwoLaneWidthM  float64 `json:"two_lane_width_m"`
	// Default marks the class the check assumes when no variant has a fleet.
	Default bool `json:"default,omitempty"`
}

// MapCheck is the answer of a map check: the issues, the road segments and the classes they were tried with.
type MapCheck struct {
	Issues  []maps.Issue `json:"issues"`
	Edges   []MapEdge    `json:"edges"`
	Classes []MapClass   `json:"classes"`
	Scene   *maps.Scene  `json:"scene,omitempty"`
}

// MapContext collects process codes and robot classes from every variant of the draft.
func MapContext(robots Robots, d projects.Draft) maps.Context {
	var classes []maps.ClassSpec
	seen := map[string]bool{}
	for _, v := range d.Variants {
		fleet, _, err := FleetFor(robots, v)
		if err != nil {
			continue
		}
		for _, c := range Classes(fleet) {
			key := c.Code + "|" + c.Label
			if !seen[key] {
				seen[key] = true
				classes = append(classes, c)
			}
		}
	}
	return maps.Context{ProcessCodes: ProcessCodes(d), ProcessNames: ProcessNames(d), Classes: classes, DefaultWidthM: DefaultWidthM(d)}
}

// CheckMap validates a map document and describes its road segments for the robot classes of the context.
func CheckMap(doc maps.Document, mc maps.Context) MapCheck {
	out := MapCheck{Issues: maps.Validate(doc, mc), Edges: []MapEdge{}, Classes: []MapClass{}}
	if out.Issues == nil {
		out.Issues = []maps.Issue{}
	}
	classes := mc.Classes
	assumed := len(classes) == 0
	if assumed {
		classes = []maps.ClassSpec{maps.DefaultClass}
	}
	widest := 0.0
	for _, c := range classes {
		out.Classes = append(out.Classes, MapClass{Code: c.Code, Label: c.Label, RequiredWidthM: c.RequiredWidthM(), TwoLaneWidthM: c.TwoLaneWidthM(), Default: assumed})
		if c.TwoLaneWidthM() > widest {
			widest = c.TwoLaneWidthM()
		}
	}
	for _, is := range out.Issues {
		if is.Level == maps.LevelError && (is.Code == "schema_version" || is.Code == "profile" || is.Code == "units" ||
			is.Code == "calibration" || is.Code == "limits" || is.Code == "id" || is.Code == "id_duplicate" || is.Code == "ring") {
			return out
		}
	}
	g, scene := maps.Build(doc, mc.DefaultWidthM)
	out.Scene = &scene
	for _, e := range g.Edges {
		ej := MapEdge{
			ID: e.ID, LengthM: e.LengthM, WidthM: e.WidthM, DeclaredWidthM: e.DeclaredWidthM, MeasuredWidthM: e.MeasuredWidthM,
			SingleLane: e.WidthM < widest,
		}
		for _, c := range classes {
			if e.WidthM+1e-9 < c.RequiredWidthM() {
				ej.Blocked = append(ej.Blocked, c.Label)
			}
		}
		out.Edges = append(out.Edges, ej)
	}
	return out
}
