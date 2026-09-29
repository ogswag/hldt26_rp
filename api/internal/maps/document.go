package maps

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	SchemaVersion = "map-v1"

	ProfileIndoor   = "indoor"
	ProfileAirspace = "airspace"
	ProfileField    = "field"

	PointTask    = "task"
	PointDock    = "dock"
	PointCharger = "charger"
	PointGate    = "gate"
	PointOther   = "other"

	ResourceDock        = "dock"
	ResourceNarrowAisle = "narrow_aisle"
	ResourceCharger     = "charger"

	ZoneWorkspace = "workspace"

	MaxPoints    = 1000
	MaxEdges     = 3000
	MaxPolygons  = 500
	MaxRing      = 200
	MaxResources = 100
	MaxFlows     = 50
	MaxIDLength  = 64
	MaxNameRunes = 120
)

// Document is the semantic warehouse map. Feature coordinates are page pixels; meters = px * meters_per_px.
type Document struct {
	SchemaVersion string      `json:"schema_version"`
	Profile       string      `json:"profile"`
	Units         string      `json:"units"`
	Page          *Page       `json:"page,omitempty"`
	Calibration   Calibration `json:"calibration"`
	Layers        Layers      `json:"layers"`
	Errors        []string    `json:"errors,omitempty"`
	Warnings      []string    `json:"warnings,omitempty"`
}

type Page struct {
	WidthPx    float64 `json:"width_px"`
	HeightPx   float64 `json:"height_px"`
	SourceKind string  `json:"source_kind"`
}

type Calibration struct {
	MetersPerPx float64  `json:"meters_per_px"`
	Segment     *Segment `json:"segment,omitempty"`
	Check       *Segment `json:"check,omitempty"`
}

type Segment struct {
	X1      float64 `json:"x1"`
	Y1      float64 `json:"y1"`
	X2      float64 `json:"x2"`
	Y2      float64 `json:"y2"`
	LengthM float64 `json:"length_m"`
}

type Layers struct {
	Zones     []Polygon      `json:"zones"`
	Obstacles []Polygon      `json:"obstacles"`
	Points    []PointFeature `json:"points"`
	Resources []Resource     `json:"resources"`
	Edges     []Edge         `json:"edges"`
	Flows     []Flow         `json:"flows,omitempty"`
}

type XY struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Polygon struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
	Ring []XY   `json:"ring"`
}

type PointFeature struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Name        string  `json:"name,omitempty"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	ProcessCode string  `json:"process_code,omitempty"`
}

type Resource struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name,omitempty"`
	Capacity int      `json:"capacity"`
	PointID  string   `json:"point_id,omitempty"`
	PointIDs []string `json:"point_ids,omitempty"`
	EdgeIDs  []string `json:"edge_ids,omitempty"`
}

// Points returns point_ids plus the legacy single point_id.
func (r Resource) Points() []string {
	out := make([]string, 0, len(r.PointIDs)+1)
	if r.PointID != "" {
		out = append(out, r.PointID)
	}
	for _, id := range r.PointIDs {
		if id != "" && id != r.PointID {
			out = append(out, id)
		}
	}
	return out
}

type Edge struct {
	ID            string   `json:"id"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	Bidirectional *bool    `json:"bidirectional,omitempty"`
	WidthM        *float64 `json:"width_m,omitempty"`
}

func (e Edge) TwoWay() bool {
	return e.Bidirectional == nil || *e.Bidirectional
}

// Flow binds a project process to its pickup and drop points. ID keeps the flow addressable by operations.
type Flow struct {
	ID             string   `json:"id,omitempty"`
	ProcessCode    string   `json:"process_code"`
	PickupPointIDs []string `json:"pickup_point_ids"`
	DropPointIDs   []string `json:"drop_point_ids"`
}

// ErrShape reports a document that does not match map-v1.
var ErrShape = errors.New("maps: document shape")

// Decode parses a stored or submitted document strictly.
func Decode(raw []byte) (Document, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return Document{}, fmt.Errorf("%w: empty document", ErrShape)
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	var d Document
	if err := dec.Decode(&d); err != nil {
		return Document{}, fmt.Errorf("%w: %v", ErrShape, err)
	}
	return d, nil
}

// IsEmpty reports whether raw holds no map.
func IsEmpty(raw []byte) bool {
	t := bytes.TrimSpace(raw)
	return len(t) == 0 || bytes.Equal(t, []byte("null"))
}

func (d Document) pointIndex() map[string]PointFeature {
	out := make(map[string]PointFeature, len(d.Layers.Points))
	for _, p := range d.Layers.Points {
		out[p.ID] = p
	}
	return out
}

func (d Document) toMeters(x, y float64) XY {
	return XY{X: x * d.Calibration.MetersPerPx, Y: y * d.Calibration.MetersPerPx}
}

func (d Document) ringMeters(ring []XY) []XY {
	out := make([]XY, len(ring))
	for i, p := range ring {
		out[i] = d.toMeters(p.X, p.Y)
	}
	return out
}
