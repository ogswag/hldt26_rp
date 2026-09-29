package httpserver

import (
	"math"
	"testing"

	"moscow_hackathon_2026/api/internal/econ"
)

func TestNormsRangeErrorAcceptsTheDefaults(t *testing.T) {
	if msg := normsRangeError(econ.DefaultNorms()); msg != "" {
		t.Fatalf("the default norms were refused: %s", msg)
	}
}

func TestNormsRangeErrorRefusesValuesThatBreakTheFleetSize(t *testing.T) {
	cases := []struct {
		name string
		edit func(*econ.Norms)
	}{
		{"zero availability", func(n *econ.Norms) { n.Availability = 0 }},
		{"zero utilization", func(n *econ.Norms) { n.Utilization = 0 }},
		{"availability over 100%", func(n *econ.Norms) { n.Availability = 1.2 }},
		{"negative reserve", func(n *econ.Norms) { n.Reserve = -0.1 }},
		{"reserve over 100%", func(n *econ.Norms) { n.Reserve = 1.5 }},
		{"NaN utilization", func(n *econ.Norms) { n.Utilization = math.NaN() }},
		{"infinite wage", func(n *econ.Norms) { n.TechnicianWageMonthRub = math.Inf(1) }},
		{"no robots per technician", func(n *econ.Norms) { n.RobotsPerTechnician = 0 }},
		{"no robots per charger", func(n *econ.Norms) { n.RobotsPerCharger = 0 }},
		{"discount rate under 1%", func(n *econ.Norms) { n.DiscountRate = 0.005 }},
	}
	for _, c := range cases {
		n := econ.DefaultNorms()
		c.edit(&n)
		if msg := normsRangeError(n); msg == "" {
			t.Errorf("%s was accepted", c.name)
		}
	}
}

func TestNormsRangeErrorKeepsTheEdgesOfTheRange(t *testing.T) {
	n := econ.DefaultNorms()
	n.Availability, n.Utilization, n.Reserve = 0.01, 1, 0
	n.DeliveryFrac, n.CommRubPerRobotYear = 0, 0
	if msg := normsRangeError(n); msg != "" {
		t.Fatalf("the edges of the range were refused: %s", msg)
	}
}

func TestNormsMessagesSpeakInPercent(t *testing.T) {
	n := econ.DefaultNorms()
	n.Availability = 0
	if msg := normsRangeError(n); msg != "Доступность и загрузка должны быть от 1% до 100%." {
		t.Fatalf("message %q", msg)
	}
}
