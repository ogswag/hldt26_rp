package econ

import (
	"encoding/json"
	"math"
	"strings"

	"moscow_hackathon_2026/api/internal/matching"
)

// NOTE: catalog prices include VAT; delivery and commissioning are extra. Model constants in rubles (energy, licenses,
// consumables) are also with VAT.
const (
	ModelVersion = "econ-v3"
	DefaultSeed  = 0

	Availability = 0.90
	Utilization  = 0.80
	Reserve      = 0.15

	DefaultLifetimeYears = 7.0
	DefaultServiceFrac   = 0.08

	InfraFrac          = 0.15
	SoftwareFrac       = 0.08
	IntegrationYesFrac = 0.12
	IntegrationNoFrac  = 0.20
	CommissioningFrac  = 0.05
	TrainingFrac       = 0.03
	ContingencyFrac    = 0.10
	DeliveryFrac       = 0.03

	RaasMonthlyFrac       = 0.025
	RaasMixFixedShare     = 0.5
	DefaultVATRate        = 0.22
	DefaultDiscountRate   = 0.15
	DefaultLaborCashShare = 1.0

	PriceSourceCatalog  = "catalog"
	PriceSourceOverride = "project_override"

	EnergyKW           = 2.0
	EnergyRubPerKWh    = 6.5
	LicensePerRobot    = 80_000.0
	ConsumablePerRobot = 20_000.0

	CommRubPerRobotYear    = 12_000.0
	RobotsPerTechnician    = 10.0
	TechnicianWageMonthRub = 90_000.0
	RobotsPerChargerNoSpec = 4.0
	ShiftHours             = 8.0
	BatteryFrac            = 0.15
	BatteryYears           = 4.0

	RemainFrac         = 0.40
	RemainMin          = 2.0
	AssumedCleanerHead = 8.0

	DefaultPalletOpsH         = 12.0
	DefaultPieceOpsH          = 80.0
	DefaultHospitalTripsH     = 8.0
	DefaultCleanerM2H         = 800.0
	DefaultAirportTrolleyOpsH = 10.0
	DefaultAirportRampOpsH    = 12.0

	HandlePalletS   = 60.0
	HandlePieceS    = 20.0
	HandleHospitalS = 120.0
	HandleRampS     = 180.0
	HandleTrolleyS  = 60.0
	CleanerOverlap  = 0.70

	AirportHoursPerDay  = 18.0
	HospitalHoursPerDay = 24.0
	HospitalPeakFactor  = 1.5
	DefaultTaxFactor    = 1.302

	PayrollCapRub      = 2_979_000.0
	PayrollReducedRate = 0.151
	PayrollFullRate    = 0.30

	WorkPallet          = "pallet"
	WorkPiece           = "piece"
	WorkCleaner         = "cleaner"
	WorkAirportRamp     = "airport_ramp"
	WorkAirportTrolley  = "airport_trolley"
	WorkAirportCleaner  = "airport_cleaner"
	WorkHospitalCart    = "hospital_cart"
	WorkHospitalCleaner = "hospital_cleaner"
)

type Scenario struct {
	Kind                string   `json:"kind"`
	SolutionID          *string  `json:"solution_id"`
	FleetSize           *int     `json:"fleet_size"`
	CapexRub            *float64 `json:"capex_rub"`
	OpexYearRub         *float64 `json:"opex_year_rub"`
	AnnualEffectRub     *float64 `json:"annual_effect_rub"`
	PaybackYears        *float64 `json:"payback_years"`
	PaybackBand         string   `json:"payback_band,omitempty"`
	RoiPct              *float64 `json:"roi_pct"`
	TcoRub              *float64 `json:"tco_rub"`
	AccountingEffectRub *float64 `json:"accounting_effect_rub,omitempty"`
	LaborCashSavedRub   *float64 `json:"labor_cash_saved_rub,omitempty"`
	VariantID           string   `json:"variant_id,omitempty"`
	VariantName         string   `json:"variant_name,omitempty"`
	Tariff              string   `json:"tariff,omitempty"`
	PriceSource         string   `json:"price_source,omitempty"`
	// DiscountedPaybackYears, NpvRub, IrrPct and CashFlow come from the yearly cash flow (cashflow.go).
	DiscountedPaybackYears *float64  `json:"discounted_payback_years,omitempty"`
	NpvRub                 *float64  `json:"npv_rub,omitempty"`
	IrrPct                 *float64  `json:"irr_pct,omitempty"`
	CashFlow               []FlowRow `json:"cash_flow,omitempty"`
}

type Line struct {
	Scenario string  `json:"scenario"`
	Bucket   string  `json:"bucket"`
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Rub      float64 `json:"rub"`
	Note     string  `json:"note,omitempty"`
}

type Formula struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Unit   string `json:"unit"`
	MathML string `json:"mathml,omitempty"`
}

type OverrideFlag struct {
	Field    string  `json:"field"`
	Value    float64 `json:"value"`
	Computed float64 `json:"computed"`
	Flag     bool    `json:"flag"`
}

type SensitivityRow struct {
	VariantID   string   `json:"variant_id,omitempty"`
	VariantName string   `json:"variant_name,omitempty"`
	Param       string   `json:"param"`
	DeltaPct    float64  `json:"delta_pct"`
	Buy         Scenario `json:"buy"`
	Raas        Scenario `json:"raas"`
}

// Ways a variant is checked by simulation: on the project map (sim-v2) or by the quick check without a map (sim-v1).
const (
	SimCheckMap   = "map"
	SimCheckQuick = "quick"
)

// SimCheck compares what a simulation delivers for one variant with what the economics counts on. A variant that
// was never simulated has no RunID for the map check. Stale means the run was made for other inputs of the variant.
type SimCheck struct {
	VariantID   string  `json:"variant_id"`
	VariantName string  `json:"variant_name"`
	Model       string  `json:"model"`
	Value       float64 `json:"value"`
	Flag        bool    `json:"flag"`
	Text        string  `json:"text,omitempty"`
	RunID       string  `json:"run_id,omitempty"`
	Stale       bool    `json:"stale,omitempty"`
}

// SimChecksFlag reports whether any check that is not stale found the simulation and the economics apart.
func SimChecksFlag(checks []SimCheck) bool {
	for _, c := range checks {
		if c.Flag && !c.Stale {
			return true
		}
	}
	return false
}

type Shared struct {
	FleetSize  int     `json:"fleet_size"`
	Chargers   int     `json:"chargers,omitempty"`
	Throughput float64 `json:"throughput"`
	PeakOps    float64 `json:"peak_ops"`
	Unit       string  `json:"unit"`
	WorkKind   string  `json:"work_kind"`
}

type Risk struct {
	ID    string `json:"id"`
	Level string `json:"level"`
	Text  string `json:"text"`
}

type Result struct {
	ModelVersion     string            `json:"model_version"`
	ObjectType       string            `json:"object_type"`
	Seed             int               `json:"seed"`
	Scenarios        []Scenario        `json:"scenarios"`
	Match            matching.Output   `json:"match"`
	Assumptions      []string          `json:"assumptions"`
	VerificationFlag bool              `json:"verification_flag"`
	Formulas         []Formula         `json:"formulas,omitempty"`
	Units            map[string]string `json:"units,omitempty"`
	Sources          []string          `json:"sources,omitempty"`
	Breakdown        []Line            `json:"breakdown,omitempty"`
	Overrides        []OverrideFlag    `json:"overrides,omitempty"`
	Sensitivity      []SensitivityRow  `json:"sensitivity,omitempty"`
	Shared           *Shared           `json:"shared,omitempty"`
	HorizonYears     float64           `json:"horizon_years,omitempty"`
	SolutionName     string            `json:"solution_name,omitempty"`
	Risks            []Risk            `json:"risks,omitempty"`
	Interpretation   []string          `json:"interpretation,omitempty"`
	Sim              *SimSummary       `json:"sim,omitempty"`
	Variants         []VariantResult   `json:"variants,omitempty"`
	Origins          []Origin          `json:"origins,omitempty"`
	AssumptionSet    *AssumptionView   `json:"assumption_set,omitempty"`
	SimChecks        []SimCheck        `json:"sim_checks,omitempty"`
}

type Origin struct {
	Metric string `json:"metric"`
	Source string `json:"source"`
	Ref    string `json:"ref,omitempty"`
	Note   string `json:"note"`
}

type AssumptionView struct {
	ID               string  `json:"id,omitempty"`
	Name             string  `json:"name"`
	VATRate          float64 `json:"vat_rate"`
	PricesIncludeVAT bool    `json:"prices_include_vat"`
	VATRecoverable   bool    `json:"vat_recoverable"`
	LaborCashShare   float64 `json:"labor_cash_share"`
	DiscountRate     float64 `json:"discount_rate"`
	// The values below are what the calculation used; Set says the set fixed it, otherwise it is the norm.
	Utilization            NormValue `json:"utilization"`
	Availability           NormValue `json:"availability"`
	Reserve                NormValue `json:"reserve"`
	ServiceShare           NormValue `json:"service_share"`
	DeliveryShare          NormValue `json:"delivery_share"`
	CommRubPerRobotYear    NormValue `json:"comm_rub_per_robot_year"`
	TechnicianWageMonthRub NormValue `json:"technician_wage_month_rub"`
}

// NormValue is a value of the calculation and whether the assumption set fixed it. The norm of a service share is
// the one for a robot without its own.
type NormValue struct {
	Value float64 `json:"value"`
	Set   bool    `json:"set"`
}

type FleetKPI struct {
	SolutionID          string   `json:"solution_id"`
	Name                string   `json:"name"`
	Quantity            int      `json:"quantity"`
	CatalogPriceRub     *float64 `json:"catalog_price_rub"`
	ProjectPriceRub     *float64 `json:"project_price_rub"`
	CashPriceRub        float64  `json:"cash_price_rub"`
	LifetimeYears       float64  `json:"lifetime_years"`
	LifetimeAssumed     bool     `json:"lifetime_assumed,omitempty"`
	PriceSource         string   `json:"price_source"`
	PriceOverrideReason string   `json:"price_override_reason,omitempty"`
	WorkKind            string   `json:"work_kind"`
	TaskCodes           []string `json:"task_codes,omitempty"`
	// PeakOps and Throughput are what the quick check holds this line to: the peak demand of the processes it serves
	// and what one robot does per hour, both in Unit.
	PeakOps    float64 `json:"peak_ops,omitempty"`
	Throughput float64 `json:"throughput,omitempty"`
	Unit       string  `json:"unit,omitempty"`
}

type VariantResult struct {
	VariantID   string     `json:"variant_id"`
	Name        string     `json:"name"`
	Chargers    int        `json:"chargers,omitempty"`
	Fleet       []FleetKPI `json:"fleet"`
	SharedCosts []Line     `json:"shared_costs,omitempty"`
	Scenarios   []Scenario `json:"scenarios"`
}

type SimSummary struct {
	Throughput     float64  `json:"throughput"`
	EconThroughput float64  `json:"econ_throughput"`
	Divergence     float64  `json:"divergence"`
	DeliveredOpsH  float64  `json:"delivered_ops_h"`
	QueueWaitS     float64  `json:"queue_wait_s"`
	ChargeShare    float64  `json:"charge_share"`
	SimulatedS     float64  `json:"simulated_s"`
	Bottleneck     string   `json:"bottleneck"`
	Unit           string   `json:"unit"`
	Assumptions    []string `json:"assumptions,omitempty"`
}

type Robot struct {
	ID             string
	Name           string
	Kind           *string
	Subtype        *string
	Family         string
	Scenario       *string
	Description    string
	PriceRub       *float64
	SpeedMps       *float64
	WidthMm        *float64
	LengthMm       *float64
	PayloadKg      *float64
	LifetimeYears  *float64
	ServicePctYear *float64
	EnduranceH     *float64
	ChargeMin      *float64
	// SimTasks are the task types the warehouse simulation runs this robot on; nil where there is no simulation.
	SimTasks []string
}

type Overrides struct {
	PriceRub     *float64 `json:"price_rub"`
	VolumeFactor *float64 `json:"volume_factor"`
	LaborFactor  *float64 `json:"labor_factor"`
	FleetSize    *int     `json:"fleet_size"`
	CapexRub     *float64 `json:"capex_rub"`
	OpexYearRub  *float64 `json:"opex_year_rub"`
}

type ProcessSpec struct {
	Code           string
	Name           string
	TaskType       string
	IsBaseline     bool
	UnitsPerDay    float64
	DemandUnit     string
	StaffHeadcount float64
	StaffRole      string
}

type FleetSpec struct {
	SolutionID          string
	Quantity            int
	TaskCodes           []string
	PriceOverrideRub    *float64
	PriceOverrideReason string
}

type FinancingSpec struct {
	Kind        string
	Tariff      string
	Assumptions json.RawMessage
}

type VariantSpec struct {
	ID        string
	Name      string
	Fleet     []FleetSpec
	Financing []FinancingSpec
}

type SharedCostSpec struct {
	Code   string
	Label  string
	Bucket string
	Rub    float64
}

type AssumptionValues struct {
	ID               string
	Name             string
	VATRate          *float64
	PricesIncludeVAT *bool
	VATRecoverable   bool
	LaborCashShare   *float64
	DiscountRate     *float64
	// The fields below are empty when the set leaves them to the norms: see Norms.with and serviceFracOf.
	Utilization            *float64
	Availability           *float64
	Reserve                *float64
	ServiceShare           *float64
	DeliveryShare          *float64
	CommRubPerRobotYear    *float64
	TechnicianWageMonthRub *float64
}

type Input struct {
	ObjectType  string
	Params      json.RawMessage
	Robot       *Robot
	Robots      []Robot
	PickReason  string
	Overrides   Overrides
	Seed        int
	Match       matching.Output
	Processes   []ProcessSpec
	Variants    []VariantSpec
	SharedCosts []SharedCostSpec
	Assumptions AssumptionValues
	Norms       Norms
}

type InputError struct {
	Msg string
}

func (e *InputError) Error() string {
	if e == nil {
		return "econ.input"
	}
	return e.Msg
}

func EmptyResult(objectType string, seed int) Result {
	return Result{
		ModelVersion: ModelVersion,
		ObjectType:   objectType,
		Seed:         seed,
		Scenarios: []Scenario{
			{Kind: "baseline"},
			{Kind: "buy"},
			{Kind: "raas"},
		},
		Match:            matching.Empty(),
		Assumptions:      []string{},
		VerificationFlag: false,
	}
}

// PickSolution returns the priced robot: selected, else recommended, else needs_review.
func PickSolution(out matching.Output) (id string, reason string) {
	for _, id := range out.SelectedIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			return id, "selected"
		}
	}
	review := ""
	for _, it := range out.Items {
		if it.Status == matching.StatusRecommended {
			return it.SolutionID, "recommended"
		}
		if review == "" && it.Status == matching.StatusNeedsReview {
			review = it.SolutionID
		}
	}
	if review != "" {
		return review, "needs_review"
	}
	return "", "none"
}

func roundRub(v float64) float64 {
	return math.Round(v)
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func fptr(v float64) *float64 {
	return &v
}

func iptr(v int) *int {
	return &v
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func fold(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	return s
}

func containsAny(text string, needles []string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(text, n) {
			return true
		}
	}
	return false
}

func (a AssumptionValues) vatRate() float64 {
	if a.VATRate == nil || *a.VATRate < 0 {
		return DefaultVATRate
	}
	return *a.VATRate
}

func (a AssumptionValues) pricesIncludeVAT() bool {
	if a.PricesIncludeVAT == nil {
		return true
	}
	return *a.PricesIncludeVAT
}

func (a AssumptionValues) laborCashShare() float64 {
	if a.LaborCashShare == nil {
		return DefaultLaborCashShare
	}
	v := *a.LaborCashShare
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// discountRate treats an unset or non-positive rate as the default, so results replayed from before the field existed
// keep the old behaviour.
func (a AssumptionValues) discountRate() float64 {
	if a.DiscountRate == nil || *a.DiscountRate <= 0 {
		return DefaultDiscountRate
	}
	return *a.DiscountRate
}

func (a AssumptionValues) displayName() string {
	if a.Name == "" {
		return "Базовый"
	}
	return a.Name
}

func (a AssumptionValues) view(n Norms) AssumptionView {
	eff := n.with(a)
	service := NormValue{Value: n.DefaultServiceFrac}
	if usable(a.ServiceShare, true, 1) {
		service = NormValue{Value: *a.ServiceShare, Set: true}
	}
	return AssumptionView{
		ID:               a.ID,
		Name:             a.displayName(),
		VATRate:          a.vatRate(),
		PricesIncludeVAT: a.pricesIncludeVAT(),
		VATRecoverable:   a.VATRecoverable,
		LaborCashShare:   a.laborCashShare(),
		DiscountRate:     a.discountRate(),

		Utilization:            NormValue{Value: eff.Utilization, Set: usable(a.Utilization, false, 1)},
		Availability:           NormValue{Value: eff.Availability, Set: usable(a.Availability, false, 1)},
		Reserve:                NormValue{Value: eff.Reserve, Set: usable(a.Reserve, true, 1)},
		ServiceShare:           service,
		DeliveryShare:          NormValue{Value: eff.DeliveryFrac, Set: usable(a.DeliveryShare, true, 1)},
		CommRubPerRobotYear:    NormValue{Value: eff.CommRubPerRobotYear, Set: usable(a.CommRubPerRobotYear, true, math.Inf(1))},
		TechnicianWageMonthRub: NormValue{Value: eff.TechnicianWageMonthRub, Set: usable(a.TechnicianWageMonthRub, true, math.Inf(1))},
	}
}

func cashPrice(list float64, a AssumptionValues) float64 {
	if list <= 0 {
		return 0
	}
	rate := a.vatRate()
	gross := list
	if !a.pricesIncludeVAT() {
		gross = list * (1 + rate)
	}
	if a.VATRecoverable && rate > -1 {
		return roundRub(gross / (1 + rate))
	}
	return roundRub(gross)
}
