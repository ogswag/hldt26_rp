package projects

import (
	"encoding/json"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/sim/geom"
)

const (
	DraftSchemaVersion    = "draft-v1"
	SnapshotSchemaVersion = "snapshot-v1"
	MapSchemaVersion      = "map-v1"
	EventSchemaVersion    = "sim-event-v1"
	MatchVersion          = "match-v4"
	SimVersion            = geom.Version
	MaxVariants           = 6
)

// A project without picked robots is calculated and simulated on the suggestion, as a variant of this id and name.
const (
	SuggestionVariantID = "suggestion"
	SuggestionName      = "Предложение расчёта"
)

// Confidence levels of a run, from catalog norms up to a forecast checked against the real process.
const (
	ConfidencePreliminary = "preliminary"
	ConfidenceConfigured  = "configured"
	ConfidenceCalibrated  = "calibrated"
	ConfidenceValidated   = "validated"
)

// SnapshotModel names the model and confidence level a snapshot was taken for.
type SnapshotModel struct {
	SimVersion      string
	ConfidenceLevel string
}

// CalculationModel is the model of an econ-v1 calculation run. It uses catalog data and norms only.
func CalculationModel() SnapshotModel {
	return SnapshotModel{SimVersion: SimVersion, ConfidenceLevel: ConfidencePreliminary}
}

type Demand struct {
	UnitsPerDay float64 `json:"units_per_day,omitempty"`
	Unit        string  `json:"unit,omitempty"`
	UnitsPerJob float64 `json:"units_per_job,omitempty"`
}

type SLA struct {
	MaxWaitMin  float64 `json:"max_wait_min,omitempty"`
	MaxCycleMin float64 `json:"max_cycle_min,omitempty"`
	Priority    int     `json:"priority,omitempty"`
}

type Durations struct {
	LoadS   float64 `json:"load_s,omitempty"`
	UnloadS float64 `json:"unload_s,omitempty"`
	TravelS float64 `json:"travel_s,omitempty"`
}

type Staff struct {
	Headcount float64 `json:"headcount,omitempty"`
	Role      string  `json:"role,omitempty"`
}

type Process struct {
	ID            string    `json:"id,omitempty"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	TaskType      string    `json:"task_type"`
	IsBaseline    bool      `json:"is_baseline"`
	Demand        Demand    `json:"demand"`
	SLA           SLA       `json:"sla"`
	PointIDs      []string  `json:"point_ids,omitempty"`
	Durations     Durations `json:"durations"`
	BaselineStaff Staff     `json:"baseline_staff"`
	SortOrder     int       `json:"sort_order"`
	Order         string    `json:"order,omitempty"`
}

type FleetItem struct {
	ID                  string   `json:"id,omitempty"`
	SolutionID          *string  `json:"solution_id"`
	Quantity            int      `json:"quantity"`
	TaskCodes           []string `json:"task_codes,omitempty"`
	PriceOverrideRub    *float64 `json:"price_override_rub"`
	PriceOverrideReason string   `json:"price_override_reason,omitempty"`
	SortOrder           int      `json:"sort_order"`
	Order               string   `json:"order,omitempty"`
}

type Financing struct {
	ID          string          `json:"id,omitempty"`
	Kind        string          `json:"kind"`
	Tariff      *string         `json:"tariff"`
	Assumptions json.RawMessage `json:"assumptions,omitempty"`
}

type Variant struct {
	ID        string      `json:"id,omitempty"`
	Name      string      `json:"name"`
	Status    string      `json:"status"`
	Notes     string      `json:"notes,omitempty"`
	SortOrder int         `json:"sort_order"`
	Order     string      `json:"order,omitempty"`
	Fleet     []FleetItem `json:"fleet"`
	Financing []Financing `json:"financing"`
}

type SharedCost struct {
	ID        string  `json:"id,omitempty"`
	Code      string  `json:"code"`
	Label     string  `json:"label"`
	Bucket    string  `json:"bucket"`
	Rub       float64 `json:"rub"`
	SortOrder int     `json:"sort_order"`
	Order     string  `json:"order,omitempty"`
}

// AssumptionSet holds the rates of one calculation case. A nil pointer field means the norm applies.
type AssumptionSet struct {
	ID                     string   `json:"id,omitempty"`
	Name                   string   `json:"name"`
	IsActive               bool     `json:"is_active"`
	VATRate                float64  `json:"vat_rate"`
	PricesIncludeVAT       bool     `json:"prices_include_vat"`
	VATRecoverable         bool     `json:"vat_recoverable"`
	LaborCashShare         float64  `json:"labor_cash_share"`
	DiscountRate           float64  `json:"discount_rate"`
	Utilization            *float64 `json:"utilization,omitempty"`
	Availability           *float64 `json:"availability,omitempty"`
	Reserve                *float64 `json:"reserve,omitempty"`
	ServiceShare           *float64 `json:"service_share,omitempty"`
	DeliveryShare          *float64 `json:"delivery_share,omitempty"`
	CommRubPerRobotYear    *float64 `json:"comm_rub_per_robot_year,omitempty"`
	TechnicianWageMonthRub *float64 `json:"technician_wage_month_rub,omitempty"`
	SortOrder              int      `json:"sort_order"`
	Order                  string   `json:"order,omitempty"`
}

type Draft struct {
	SchemaVersion         string          `json:"schema_version"`
	ObjectType            string          `json:"object_type"`
	Params                json.RawMessage `json:"params"`
	Processes             []Process       `json:"processes"`
	Variants              []Variant       `json:"variants"`
	SharedCosts           []SharedCost    `json:"shared_costs,omitempty"`
	AssumptionSets        []AssumptionSet `json:"assumption_sets,omitempty"`
	ActiveAssumptionSetID string          `json:"active_assumption_set_id,omitempty"`
	Map                   json.RawMessage `json:"map"`
	MatchSelectedIDs      []string        `json:"match_selected_ids"`
	EconOverrides         json.RawMessage `json:"econ_overrides,omitempty"`
	// NOTE: reviewed_tabs steers the UI only; it stays out of InputHash, so reviewing never makes a run stale.
	ReviewedTabs []string `json:"reviewed_tabs,omitempty"`
}

type Snapshot struct {
	SchemaVersion   string `json:"schema_version"`
	ProjectID       string `json:"project_id"`
	VersionNo       int    `json:"version_no"`
	Name            string `json:"name"`
	ObjectType      string `json:"object_type"`
	Draft           Draft  `json:"draft"`
	InputHash       string `json:"input_hash"`
	MatchVersion    string `json:"match_version"`
	EconVersion     string `json:"econ_version"`
	SimVersion      string `json:"sim_version"`
	ConfidenceLevel string `json:"confidence_level"`
}

func NewDraft(objectType string, params json.RawMessage, processes []Process, variants []Variant, selected []string) Draft {
	if params == nil {
		params = json.RawMessage(`{}`)
	}
	if processes == nil {
		processes = []Process{}
	}
	if variants == nil {
		variants = []Variant{}
	}
	if selected == nil {
		selected = []string{}
	}
	return Draft{
		SchemaVersion:    DraftSchemaVersion,
		ObjectType:       objectType,
		Params:           params,
		Processes:        processes,
		Variants:         variants,
		SharedCosts:      []SharedCost{},
		AssumptionSets:   DefaultAssumptionSets(),
		Map:              nil,
		MatchSelectedIDs: selected,
	}
}

func NewSnapshot(projectID, name, objectType string, versionNo int, draft Draft, hash string, model SnapshotModel) Snapshot {
	if model.SimVersion == "" {
		model.SimVersion = SimVersion
	}
	if model.ConfidenceLevel == "" {
		model.ConfidenceLevel = ConfidencePreliminary
	}
	return Snapshot{
		SchemaVersion:   SnapshotSchemaVersion,
		ProjectID:       projectID,
		VersionNo:       versionNo,
		Name:            name,
		ObjectType:      objectType,
		Draft:           draft,
		InputHash:       hash,
		MatchVersion:    MatchVersion,
		EconVersion:     econ.ModelVersion,
		SimVersion:      model.SimVersion,
		ConfidenceLevel: model.ConfidenceLevel,
	}
}
