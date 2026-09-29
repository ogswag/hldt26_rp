package econ

import "moscow_hackathon_2026/api/internal/rutext"

// fleetItem is one line of a fleet with its own price, life and service share, so a mixed fleet does not depend on
// the order of its robots.
type fleetItem struct {
	Robot          Robot
	Qty            int
	ListRub        float64
	CashRub        float64
	Life           float64
	LifeAssumed    bool
	ServiceFrac    float64
	ServiceAssumed bool
}

func newFleetItem(r Robot, qty int, list float64, a AssumptionValues, n Norms) fleetItem {
	life, lifeAssumed := lifetimeOf(r, n)
	svc, svcAssumed := serviceFracOf(r, n, a)
	return fleetItem{
		Robot: r, Qty: qty, ListRub: list, CashRub: cashPrice(list, a),
		Life: life, LifeAssumed: lifeAssumed, ServiceFrac: svc, ServiceAssumed: svcAssumed,
	}
}

func (f fleetItem) equipment() float64 { return roundRub(f.CashRub * float64(f.Qty)) }

// vatPolicy is the one place the assumption set touches a rub amount.
// NOTE: model constants (energy, licenses, consumables) are declared with VAT, like catalog prices.
type vatPolicy struct {
	rate        float64
	recoverable bool
}

func newVATPolicy(a AssumptionValues) vatPolicy {
	return vatPolicy{rate: a.vatRate(), recoverable: a.VATRecoverable}
}

// constant is what a model constant that includes VAT costs the customer in cash.
func (p vatPolicy) constant(withVAT float64) float64 {
	if p.recoverable && p.rate > -1 {
		return withVAT / (1 + p.rate)
	}
	return withVAT
}

// costModel is the CAPEX and yearly cost of one technical fleet, shared by the single-robot calculation and the
// variants so the two cannot drift apart.
type costModel struct {
	Items         []fleetItem
	Fleet         int
	Equipment     float64
	Infra         float64
	Software      float64
	Integration   float64
	Commissioning float64
	Training      float64
	Delivery      float64
	SharedLines   []Line
	SharedCapex   float64
	SharedOpex    float64
	BuyCont       float64
	RaasCont      float64
	BuyCapex      float64
	RaasCapex     float64
	Service       float64
	Licenses      float64
	Energy        float64
	Consumables   float64
	// Comms is the yearly communications charge of the fleet. Technicians is the yearly pay of the staff that keeps
	// it running, TechHeads their number over all shifts, and Chargers the charging stations the fleet needs.
	Comms       float64
	Technicians float64
	TechHeads   float64
	Chargers    int
	// DepreciableBase is what the customer capitalises apart from the robots: infrastructure, software, integration,
	// commissioning and extra CAPEX lines. Training is an expense and the reserve is not an asset.
	DepreciableBase float64
	// Depreciation is the yearly straight-line charge: robots by their own life, the rest by the default life.
	Depreciation     float64
	RaasDepreciation float64
}

type costInput struct {
	Items       []fleetItem
	HoursYear   float64
	IntegFrac   float64
	SharedKey   string
	Shared      []SharedCostSpec
	Assumptions AssumptionValues
	// Norms are the values the calculation runs on, the assumption set already applied.
	Norms       Norms
	Shifts      float64
	PayrollTax  float64
	LaborFactor float64
}

func buildCosts(in costInput) costModel {
	if in.Norms == (Norms{}) {
		in.Norms = DefaultNorms()
	}
	vat := newVATPolicy(in.Assumptions)
	cm := costModel{Items: in.Items}
	robotDep, service := 0.0, 0.0
	for _, it := range in.Items {
		eq := it.equipment()
		cm.Equipment += eq
		cm.Fleet += it.Qty
		service += it.ServiceFrac * eq
		robotDep += eq / it.Life
	}
	cm.Service = roundRub(service)
	n := in.Norms
	cm.Infra = roundRub(n.InfraFrac * cm.Equipment)
	cm.Software = roundRub(n.SoftwareFrac * cm.Equipment)
	cm.Integration = roundRub(in.IntegFrac * cm.Equipment)
	cm.Commissioning = roundRub(n.CommissioningFrac * cm.Equipment)
	cm.Training = roundRub(n.TrainingFrac * cm.Equipment)
	cm.Delivery = roundRub(n.DeliveryFrac * cm.Equipment)
	cm.SharedLines, cm.SharedCapex, cm.SharedOpex = sharedLines(in, cm)

	// NOTE: delivery is paid by a buyer only; under RaaS it is inside the fee.
	buySub := cm.Equipment + cm.Delivery + cm.SharedCapex
	cm.BuyCont = roundRub(n.ContingencyFrac * buySub)
	cm.BuyCapex = buySub + cm.BuyCont
	raasSub := cm.SharedCapex
	cm.RaasCont = roundRub(n.ContingencyFrac * raasSub)
	cm.RaasCapex = raasSub + cm.RaasCont

	fleet := float64(cm.Fleet)
	cm.Licenses = roundRub(vat.constant(n.LicensePerRobot * fleet))
	cm.Energy = roundRub(vat.constant(n.EnergyKW * in.HoursYear * n.EnergyRubPerKWh * fleet))
	cm.Consumables = roundRub(vat.constant(n.ConsumablePerRobot * fleet))
	cm.Comms = roundRub(vat.constant(n.CommRubPerRobotYear * fleet))
	cm.TechHeads = technicians(cm.Fleet, in.Shifts, n)
	cm.Technicians = roundRub(yearFot(cm.TechHeads, n.TechnicianWageMonthRub, in.PayrollTax, in.LaborFactor))
	cm.Chargers = chargers(in.Items, n)

	for _, l := range cm.SharedLines {
		if l.Bucket == "capex" && l.ID != "training" {
			cm.DepreciableBase += l.Rub
		}
	}
	cm.RaasDepreciation = roundRub(cm.DepreciableBase / n.DefaultLifetimeYears)
	cm.Depreciation = roundRub(robotDep + (cm.DepreciableBase+cm.Delivery)/n.DefaultLifetimeYears)
	return cm
}

// sharedLines are the once-per-technical-variant lines, the fixed shares of the equipment and the extra lines the
// assumptions page adds. The extra lines are entered like prices, so the same VAT flags apply to them.
func sharedLines(in costInput, cm costModel) ([]Line, float64, float64) {
	key, n := in.SharedKey, in.Norms
	lines := []Line{
		{Scenario: key, Bucket: "capex", ID: "infrastructure", Label: "Общая инфраструктура " + shareOfEquipment(n.InfraFrac), Rub: cm.Infra},
		{Scenario: key, Bucket: "capex", ID: "software", Label: "ПО " + shareOfEquipment(n.SoftwareFrac), Rub: cm.Software},
		{Scenario: key, Bucket: "capex", ID: "integration", Label: "Интеграция", Rub: cm.Integration, Note: pctNote(in.IntegFrac)},
		{Scenario: key, Bucket: "capex", ID: "commissioning", Label: "Пусконаладка " + shareOfEquipment(n.CommissioningFrac), Rub: cm.Commissioning},
		{Scenario: key, Bucket: "capex", ID: "training", Label: "Обучение " + shareOfEquipment(n.TrainingFrac), Rub: cm.Training},
	}
	capex := cm.Infra + cm.Software + cm.Integration + cm.Commissioning + cm.Training
	opex := 0.0
	for _, c := range in.Shared {
		if c.Rub < 0 {
			continue
		}
		bucket := c.Bucket
		if bucket == "" {
			bucket = "capex"
		}
		code := c.Code
		if code == "" {
			code = "shared"
		}
		label := c.Label
		if label == "" {
			label = "Общая статья"
		}
		rub := cashPrice(c.Rub, in.Assumptions)
		lines = append(lines, Line{Scenario: key, Bucket: bucket, ID: code, Label: label, Rub: rub, Note: "общая статья варианта, не на позицию флота"})
		if bucket == "capex" {
			capex += rub
		} else if bucket == "opex" {
			opex += rub
		}
	}
	return lines, capex, opex
}

func (cm costModel) purchases() []purchase {
	out := make([]purchase, 0, len(cm.Items))
	for _, it := range cm.Items {
		out = append(out, purchase{Rub: it.equipment(), Life: it.Life})
	}
	return out
}

func (cm costModel) buyOpex(remainFot float64) float64 {
	return remainFot + cm.Service + cm.Licenses + cm.Energy + cm.Consumables + cm.Comms + cm.Technicians + cm.SharedOpex
}

func (cm costModel) raasOpex(remainFot, fee float64) float64 {
	return remainFot + fee + cm.Energy + cm.Consumables + cm.Comms + cm.Technicians + cm.SharedOpex
}

func (cm costModel) delivery(key string, n Norms) Line {
	return Line{Scenario: key, Bucket: "capex", ID: "delivery", Label: "Доставка " + shareOfEquipment(n.DeliveryFrac), Rub: cm.Delivery,
		Note: "не вносите доставку ещё раз общей статьёй"}
}

// operationLines are the yearly costs of a fleet that a buyer and a RaaS customer both pay.
func (cm costModel) operationLines(key string, n Norms) []Line {
	return []Line{
		{Scenario: key, Bucket: "opex", ID: "energy", Label: "Энергия", Rub: cm.Energy},
		{Scenario: key, Bucket: "opex", ID: "consumables", Label: "Расходники", Rub: cm.Consumables},
		{Scenario: key, Bucket: "opex", ID: "communications", Label: "Связь", Rub: cm.Comms, Note: rutext.Num(n.CommRubPerRobotYear, 0) + " ₽ в год на робота"},
		{Scenario: key, Bucket: "opex", ID: "technicians", Label: "Персонал эксплуатации", Rub: cm.Technicians, Note: rutext.Num(cm.TechHeads, 0) + " чел, по одному на " + rutext.Num(n.RobotsPerTechnician, 0) + " роботов в смену"},
	}
}

func shareOfEquipment(frac float64) string {
	return rutext.Pct(frac*100, 1) + " оборудования"
}

func (cm costModel) anyLifeAssumed() bool {
	for _, it := range cm.Items {
		if it.LifeAssumed {
			return true
		}
	}
	return false
}

func (cm costModel) anyServiceAssumed() bool {
	for _, it := range cm.Items {
		if it.ServiceAssumed {
			return true
		}
	}
	return false
}

func (cm costModel) minLife() float64 {
	life := 0.0
	for i, it := range cm.Items {
		if i == 0 || it.Life < life {
			life = it.Life
		}
	}
	return life
}
