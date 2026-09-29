package projects

import (
	"encoding/json"
	"testing"

	"moscow_hackathon_2026/api/internal/objects"
)

func simDraft(t *testing.T) Draft {
	t.Helper()
	a, b := "5760e938-9a43-45a7-b8e8-f4f2e6383930", "2ffc706d-fe43-4c2b-baad-a624a95ad3ce"
	d := NewDraft("warehouse", json.RawMessage(`{"area_total_m2":20000}`), DefaultProcesses("warehouse"), []Variant{
		{ID: "v1", Name: "Первый", Status: "draft", Fleet: []FleetItem{{SolutionID: &a, Quantity: 4, TaskCodes: []string{"inbound", "putaway"}}}, Financing: []Financing{{Kind: "buy"}}},
		{ID: "v2", Name: "Второй", Status: "draft", Fleet: []FleetItem{{SolutionID: &b, Quantity: 2, TaskCodes: []string{"outbound"}}}, Financing: []Financing{{Kind: "buy"}}},
	}, nil)
	return d
}

func simHash(t *testing.T, d Draft, variant string) string {
	t.Helper()
	h, err := SimInputHash(d, variant, "sim-v2")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestSimInputHashIgnoresWhatTheSimulationDoesNotRead(t *testing.T) {
	base := simDraft(t)
	want := simHash(t, base, "v1")

	other := simDraft(t)
	other.Variants[1].Fleet[0].Quantity = 9
	other.Variants[0].Fleet[0].PriceOverrideRub = new(float64)
	other.Variants[0].Financing = []Financing{{Kind: "raas"}}
	other.SharedCosts = []SharedCost{{Code: "wms", Label: "WMS", Bucket: "capex", Rub: 1_000_000}}
	other.AssumptionSets = []AssumptionSet{{Name: "Иной", IsActive: true, VATRate: 0.1}}
	other.ReviewedTabs = []string{"general"}
	if got := simHash(t, other, "v1"); got != want {
		t.Fatalf("the other variant, costs, financing and assumptions must not change the hash of v1: %s vs %s", got, want)
	}
}

func TestSimInputHashFollowsWhatTheSimulationReads(t *testing.T) {
	base := simDraft(t)
	want := simHash(t, base, "v1")
	changes := map[string]func(d *Draft){
		"fleet quantity": func(d *Draft) { d.Variants[0].Fleet[0].Quantity = 5 },
		"fleet tasks":    func(d *Draft) { d.Variants[0].Fleet[0].TaskCodes = []string{"inbound"} },
		"process demand": func(d *Draft) { d.Processes[0].Demand.UnitsPerDay += 100 },
		"parameters":     func(d *Draft) { d.Params = json.RawMessage(`{"area_total_m2":21000}`) },
		"map":            func(d *Draft) { d.Map = json.RawMessage(`{"schema_version":"map-v1"}`) },
	}
	for name, change := range changes {
		d := simDraft(t)
		change(&d)
		if got := simHash(t, d, "v1"); got == want {
			t.Errorf("%s must change the hash", name)
		}
	}
	if simHash(t, base, "v1") == simHash(t, base, "v2") {
		t.Error("two variants with different fleets must hash differently")
	}
	if h, _ := SimInputHash(base, "v1", "sim-v3"); h == want {
		t.Error("the model version must change the hash")
	}
}

func TestSimInputHashIgnoresGapsFilledByDefaults(t *testing.T) {
	gaps := simDraft(t)
	gaps.Params = json.RawMessage(`{}`)
	filled := simDraft(t)
	def, err := objects.FillDefaults("warehouse", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	filled.Params = def
	if a, b := simHash(t, gaps, "v1"), simHash(t, filled, "v1"); a != b {
		t.Fatalf("a draft with gaps and the same draft with defaults must hash alike: %s vs %s", a, b)
	}
}
