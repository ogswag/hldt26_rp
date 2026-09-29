package econ

import "math"

// Norms are the numbers the model takes where nobody set a value: the shares and rates of the fleet, the CAPEX
// lines, the yearly costs, the replacements and the RaaS tariff. The zero value of Input.Norms means DefaultNorms.
type Norms struct {
	Availability       float64 `json:"availability"`
	Utilization        float64 `json:"utilization"`
	Reserve            float64 `json:"reserve"`
	RobotsPerCharger   float64 `json:"robots_per_charger"`
	InfraFrac          float64 `json:"infra_frac"`
	SoftwareFrac       float64 `json:"software_frac"`
	IntegrationYesFrac float64 `json:"integration_yes_frac"`
	IntegrationNoFrac  float64 `json:"integration_no_frac"`
	CommissioningFrac  float64 `json:"commissioning_frac"`
	TrainingFrac       float64 `json:"training_frac"`
	ContingencyFrac    float64 `json:"contingency_frac"`
	DeliveryFrac       float64 `json:"delivery_frac"`

	EnergyKW               float64 `json:"energy_kw"`
	EnergyRubPerKWh        float64 `json:"energy_rub_per_kwh"`
	LicensePerRobot        float64 `json:"license_rub_per_robot"`
	ConsumablePerRobot     float64 `json:"consumable_rub_per_robot"`
	CommRubPerRobotYear    float64 `json:"comm_rub_per_robot_year"`
	RobotsPerTechnician    float64 `json:"robots_per_technician"`
	TechnicianWageMonthRub float64 `json:"technician_wage_month_rub"`
	DefaultServiceFrac     float64 `json:"default_service_frac"`

	DefaultLifetimeYears float64 `json:"default_lifetime_years"`
	BatteryFrac          float64 `json:"battery_frac"`
	BatteryYears         float64 `json:"battery_years"`

	RaasMonthlyFrac   float64 `json:"raas_monthly_frac"`
	RaasMixFixedShare float64 `json:"raas_mix_fixed_share"`
}

// DefaultNorms are the values the team fixed for the model.
func DefaultNorms() Norms {
	return Norms{
		Availability:       Availability,
		Utilization:        Utilization,
		Reserve:            Reserve,
		RobotsPerCharger:   RobotsPerChargerNoSpec,
		InfraFrac:          InfraFrac,
		SoftwareFrac:       SoftwareFrac,
		IntegrationYesFrac: IntegrationYesFrac,
		IntegrationNoFrac:  IntegrationNoFrac,
		CommissioningFrac:  CommissioningFrac,
		TrainingFrac:       TrainingFrac,
		ContingencyFrac:    ContingencyFrac,
		DeliveryFrac:       DeliveryFrac,

		EnergyKW:               EnergyKW,
		EnergyRubPerKWh:        EnergyRubPerKWh,
		LicensePerRobot:        LicensePerRobot,
		ConsumablePerRobot:     ConsumablePerRobot,
		CommRubPerRobotYear:    CommRubPerRobotYear,
		RobotsPerTechnician:    RobotsPerTechnician,
		TechnicianWageMonthRub: TechnicianWageMonthRub,
		DefaultServiceFrac:     DefaultServiceFrac,

		DefaultLifetimeYears: DefaultLifetimeYears,
		BatteryFrac:          BatteryFrac,
		BatteryYears:         BatteryYears,

		RaasMonthlyFrac:   RaasMonthlyFrac,
		RaasMixFixedShare: RaasMixFixedShare,
	}
}

// with applies the values an assumption set fixed. An empty field keeps the norm, and so does a value outside its
// range: a stored value that a later rule rejects must not stop a calculation.
func (n Norms) with(a AssumptionValues) Norms {
	if usable(a.Utilization, false, 1) {
		n.Utilization = *a.Utilization
	}
	if usable(a.Availability, false, 1) {
		n.Availability = *a.Availability
	}
	if usable(a.Reserve, true, 1) {
		n.Reserve = *a.Reserve
	}
	if usable(a.DeliveryShare, true, 1) {
		n.DeliveryFrac = *a.DeliveryShare
	}
	if usable(a.CommRubPerRobotYear, true, math.Inf(1)) {
		n.CommRubPerRobotYear = *a.CommRubPerRobotYear
	}
	if usable(a.TechnicianWageMonthRub, true, math.Inf(1)) {
		n.TechnicianWageMonthRub = *a.TechnicianWageMonthRub
	}
	return n
}

// usable says a field of the set carries a value inside (0, max], or [0, max] when zero is allowed.
func usable(v *float64, zeroOK bool, max float64) bool {
	if v == nil || *v > max || *v < 0 || math.IsNaN(*v) {
		return false
	}
	return zeroOK || *v > 0
}

// baseNorms are the norms of the input, or the defaults when it carries none.
func (in Input) baseNorms() Norms {
	if in.Norms == (Norms{}) {
		return DefaultNorms()
	}
	return in.Norms
}

// norms is what the calculation runs on: the norms with the assumption set on top.
func (in Input) norms() Norms {
	return in.baseNorms().with(in.Assumptions)
}

// serviceFrac is the yearly service share of the equipment price: the assumption set, else the robot, else the norm.
func serviceFracOf(r Robot, n Norms, a AssumptionValues) (frac float64, assumed bool) {
	if usable(a.ServiceShare, true, 1) {
		return *a.ServiceShare, false
	}
	if r.ServicePctYear != nil && *r.ServicePctYear > 0 {
		v := *r.ServicePctYear
		if v > 1 {
			v = v / 100
		}
		return v, false
	}
	return n.DefaultServiceFrac, true
}

func lifetimeOf(r Robot, n Norms) (float64, bool) {
	if r.LifetimeYears != nil && *r.LifetimeYears > 0 {
		return *r.LifetimeYears, false
	}
	return n.DefaultLifetimeYears, true
}

// fleetSize is the robots needed for a peak: throughput cut by availability and utilization, rounded up, then the
// reserve on top, rounded up again.
func fleetSize(peak, th float64, n Norms) int {
	if peak <= 0 || th <= 0 {
		return 0
	}
	eff := th * n.Availability * n.Utilization
	if eff <= 0 {
		return 0
	}
	raw := math.Ceil(peak / eff)
	return int(math.Ceil(raw * (1 + n.Reserve)))
}

// chargers counts the charging stations of a fleet. A line with runtime and charging time in the specs needs the
// share of its robots that charge at any moment; a line without them needs one station per RobotsPerCharger. A
// fleet needs at least one.
func chargers(items []fleetItem, n Norms) int {
	total, robots := 0.0, 0
	for _, it := range items {
		q := float64(it.Qty)
		robots += it.Qty
		endurance, charge := derefFloat(it.Robot.EnduranceH), derefFloat(it.Robot.ChargeMin)/60
		if endurance > 0 && charge > 0 {
			total += math.Ceil(q * charge / (endurance + charge))
			continue
		}
		total += math.Ceil(q / n.RobotsPerCharger)
	}
	if robots > 0 && total < 1 {
		total = 1
	}
	return int(total)
}

// technicians is the staff that keeps a fleet running: one person per RobotsPerTechnician robots on every shift.
func technicians(fleet int, shifts float64, n Norms) float64 {
	if fleet <= 0 || n.RobotsPerTechnician <= 0 || shifts <= 0 {
		return 0
	}
	return math.Ceil(float64(fleet)/n.RobotsPerTechnician) * shifts
}
