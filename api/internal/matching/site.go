package matching

import (
	"encoding/json"
	"fmt"
)

func SiteFromParams(objectType string, params json.RawMessage) (Site, error) {
	var m map[string]any
	if len(params) > 0 {
		if err := json.Unmarshal(params, &m); err != nil {
			return Site{}, fmt.Errorf("matching.site: %w", err)
		}
	}
	s := Site{ObjectType: objectType, BudgetField: "capex_budget_mln_rub"}
	switch objectType {
	case "warehouse":
		s.AisleMm, s.AisleField = minMetersToMm(m, "aisle_working_m", "aisle_main_m")
		s.LightLoadKg = jsonNum(m, "unit_mass_kg")
		s.LightField = "unit_mass_kg"
		s.HeavyLoadKg = jsonNum(m, "pallet_mass_kg")
		s.HeavyField = "pallet_mass_kg"
		s.BudgetRub = millionsToRub(m, "capex_budget_mln_rub")
		s.CeilingMm = metersToMm(m, "ceiling_height_m")
		s.FloorLoadKgM2 = jsonNum(m, "floor_load_kg_m2")
		if floors := jsonNum(m, "floors"); floors != nil && *floors > 1 {
			s.LiftKg, s.LiftField = jsonNum(m, "lift_capacity_kg"), "lift_capacity_kg"
		}
		s.DoorMm = metersToMm(m, "door_width_m")
		s.TurnRoomMm = metersToMm(m, "aisle_main_m")
		s.TurnRoomField = "aisle_main_m"
	case "airport":
		s.LightLoadKg = jsonNum(m, "baggage_mass_kg")
		s.LightField = "baggage_mass_kg"
		s.TempC = jsonNum(m, "apron_temp_c")
		s.TempField = "apron_temp_c"
		s.BudgetRub = millionsToRub(m, "capex_budget_mln_rub")
	case "hospital":
		s.AisleMm, s.AisleField = minMetersToMm(m, "corridor_width_m")
		s.HeavyLoadKg = jsonNum(m, "meal_cart_mass_kg")
		s.HeavyField = "meal_cart_mass_kg"
		s.BudgetRub = millionsToRub(m, "capex_budget_mln_rub")
		s.LiftKg, s.LiftField = jsonNum(m, "elevator_capacity_kg"), "elevator_capacity_kg"
		s.DoorMm = metersToMm(m, "door_width_m")
		s.TurnRoomMm = metersToMm(m, "corridor_width_m")
		s.TurnRoomField = "corridor_width_m"
	default:
		return Site{}, fmt.Errorf("matching.site: unknown object type")
	}
	return s, nil
}

func jsonNum(m map[string]any, key string) *float64 {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case float64:
		return &t
	case int:
		f := float64(t)
		return &f
	case int32:
		f := float64(t)
		return &f
	case int64:
		f := float64(t)
		return &f
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return nil
		}
		return &f
	default:
		return nil
	}
}

func minMetersToMm(m map[string]any, keys ...string) (*float64, string) {
	var min *float64
	field := ""
	for _, k := range keys {
		v := jsonNum(m, k)
		if v == nil {
			continue
		}
		mm := *v * 1000
		if min == nil || mm < *min {
			x := mm
			min = &x
			field = k
		}
	}
	return min, field
}

func metersToMm(m map[string]any, key string) *float64 {
	v := jsonNum(m, key)
	if v == nil {
		return nil
	}
	x := *v * 1000
	return &x
}

func millionsToRub(m map[string]any, key string) *float64 {
	v := jsonNum(m, key)
	if v == nil {
		return nil
	}
	x := *v * 1_000_000
	return &x
}
