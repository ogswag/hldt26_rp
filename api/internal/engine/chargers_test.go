package engine_test

import (
	"encoding/json"
	"strings"
	"testing"

	"moscow_hackathon_2026/api/internal/engine"
	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/projects"
)

func TestMapChargerRisksNameEachVariant(t *testing.T) {
	a := cartA
	d := projects.NewDraft(objects.Warehouse, defaults(t, objects.Warehouse), nil, []projects.Variant{
		{ID: "v1", Name: "Большой флот", Status: "draft", Fleet: []projects.FleetItem{{SolutionID: &a, Quantity: 30}}},
		{ID: "v2", Name: "Средний флот", Status: "draft", Fleet: []projects.FleetItem{{SolutionID: &a, Quantity: 20}}},
		{ID: "v3", Name: "Малый флот", Status: "draft", Fleet: []projects.FleetItem{{SolutionID: &a, Quantity: 9}}},
	}, nil)
	raw, err := json.Marshal(maps.WarehouseTemplate(nil, maps.TemplateWidths{}))
	if err != nil {
		t.Fatal(err)
	}
	d.Map = raw
	res, _, err := engine.Calculate(cartCatalog(objects.Warehouse), engine.Request{
		ObjectType: objects.Warehouse, Params: d.Params, Draft: d, HasDraft: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var texts []string
	for _, r := range res.Risks {
		if !strings.HasPrefix(r.ID, "map_chargers") {
			continue
		}
		if seen[r.ID] {
			t.Errorf("risk id %q repeats", r.ID)
		}
		seen[r.ID] = true
		texts = append(texts, r.Text)
	}
	if len(texts) != 2 || !seen["map_chargers:v1"] || !seen["map_chargers:v2"] {
		t.Fatalf("two variants need more places than the map has: %q", texts)
	}
	if !strings.Contains(texts[0], "Большой флот") || strings.Contains(strings.Join(texts, " "), "Малый флот") {
		t.Errorf("the risks must name the variants that lack places: %q", texts)
	}
}
