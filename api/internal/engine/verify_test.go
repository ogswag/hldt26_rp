package engine_test

import (
	"testing"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/engine"
	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/projects"
)

const (
	cartA = "5760e938-9a43-45a7-b8e8-f4f2e6383930"
	cartB = "2ffc706d-fe43-4c2b-baad-a624a95ad3ce"
)

func cartCatalog(objectType string) engine.Catalog {
	kind, scen := "amr", objectType
	robot := func(id, name string, speed float64) econ.Robot {
		return econ.Robot{
			ID: id, Name: name, Kind: &kind, Family: "AMR", Scenario: &scen, PriceRub: ptr(2_000_000.0),
			SpeedMps: ptr(speed), WidthMm: ptr(700.0), PayloadKg: ptr(300.0), LifetimeYears: ptr(7.0),
			EnduranceH: ptr(8.0), ChargeMin: ptr(60.0),
		}
	}
	cand := func(id, name string) matching.Candidate {
		return matching.Candidate{
			ID: id, Name: name, Kind: &kind, Scenario: &scen, Family: "AMR", PriceRub: ptr(2_000_000.0),
			ObjectTypes: []string{objectType}, PayloadKg: ptr(300.0), WidthMm: ptr(700.0),
		}
	}
	return engine.Catalog{
		ContentSHA256: "verify",
		Candidates:    []matching.Candidate{cand(cartA, "Тележка А"), cand(cartB, "Тележка Б")},
		Robots:        []econ.Robot{robot(cartA, "Тележка А", 1.2), robot(cartB, "Тележка Б", 0.8)},
	}
}

func variantsDraft(t *testing.T, objectType string) projects.Draft {
	t.Helper()
	a, b := cartA, cartB
	d := projects.NewDraft(objectType, defaults(t, objectType), nil, []projects.Variant{
		{ID: "v1", Name: "Одна модель", Status: "draft", Fleet: []projects.FleetItem{{SolutionID: &a, Quantity: 3}}},
		{ID: "v2", Name: "Две модели", Status: "draft", Fleet: []projects.FleetItem{{SolutionID: &a, Quantity: 2}, {SolutionID: &b, Quantity: 2}}},
		{ID: "v3", Name: "Пустой", Status: "draft", Fleet: []projects.FleetItem{}},
	}, nil)
	return d
}

func TestQuickCheckRunsPerVariantForAirportAndHospital(t *testing.T) {
	for _, objectType := range []string{objects.Airport, objects.Hospital} {
		t.Run(objectType, func(t *testing.T) {
			d := variantsDraft(t, objectType)
			res, _, err := engine.Calculate(cartCatalog(objectType), engine.Request{
				ObjectType: objectType, Params: d.Params, Draft: d, HasDraft: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.SimChecks) != 2 {
				t.Fatalf("one check per variant with a fleet, got %+v", res.SimChecks)
			}
			for i, want := range []struct{ id, name string }{{"v1", "Одна модель"}, {"v2", "Две модели"}} {
				c := res.SimChecks[i]
				if c.VariantID != want.id || c.VariantName != want.name || c.Model != econ.SimCheckQuick || c.Text == "" {
					t.Errorf("check %d: %+v", i, c)
				}
				if c.Value <= 0 {
					t.Errorf("check %d has no simulated output: %+v", i, c)
				}
			}
			if res.VerificationFlag != econ.SimChecksFlag(res.SimChecks) {
				t.Errorf("the verification flag %v must follow the checks %+v", res.VerificationFlag, res.SimChecks)
			}
			if res.Sim == nil {
				t.Error("the run keeps the summary of its worst line")
			}
			for _, v := range res.Variants {
				for _, f := range v.Fleet {
					if f.Throughput <= 0 || f.Unit == "" {
						t.Errorf("fleet line %s of %s has no check input: %+v", f.Name, v.Name, f)
					}
				}
			}
		})
	}
}

func TestWarehouseHasNoQuickCheck(t *testing.T) {
	d := variantsDraft(t, objects.Warehouse)
	res, _, err := engine.Calculate(cartCatalog(objects.Warehouse), engine.Request{
		ObjectType: objects.Warehouse, Params: d.Params, Draft: d, HasDraft: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.SimChecks) != 0 || res.Sim != nil || res.VerificationFlag {
		t.Fatalf("a warehouse is checked by simulation runs on its map, not here: %+v", res.SimChecks)
	}
}
