package econ

import "math"

func volumeFleetAssumption() string {
	return "В чувствительности сдвиг объёма меняет и флот: количество каждой позиции умножается на тот же коэффициент и округляется вверх. База ФОТ не меняется."
}

func sensitivityVariants(in Input, robots map[string]Robot, env variantEnv) []SensitivityRow {
	var out []SensitivityRow
	for _, v := range in.Variants {
		if !specHasFleet(v) {
			continue
		}
		out = append(out, variantSteps(in, v, robots, env)...)
	}
	return out
}

func specHasFleet(v VariantSpec) bool {
	for _, f := range v.Fleet {
		if f.Quantity > 0 && f.SolutionID != "" {
			return true
		}
	}
	return false
}

func variantSteps(in Input, v VariantSpec, robots map[string]Robot, env variantEnv) []SensitivityRow {
	type step struct {
		param string
		delta float64
		price float64
		volume float64
		labor float64
	}
	steps := []step{
		{"equipment_price", -20, 0.8, 1, 1},
		{"equipment_price", 20, 1.2, 1, 1},
		{"volume", -20, 1, 0.8, 1},
		{"volume", 20, 1, 1.2, 1},
		{"labor", -20, 1, 1, 0.8},
		{"labor", 20, 1, 1, 1.2},
	}
	out := make([]SensitivityRow, 0, len(steps))
	for _, st := range steps {
		spec := scaleFleet(v, robots, st.price, st.volume)
		next := env
		next.volume *= st.volume
		next.laborFactor *= st.labor
		ev := evalVariant(in, spec, robots, next)
		buy, raas, ok := buyAndRaas(ev.Result.Scenarios)
		if !ok {
			continue
		}
		out = append(out, SensitivityRow{
			VariantID: v.ID, VariantName: v.Name, Param: st.param, DeltaPct: st.delta, Buy: buy, Raas: raas,
		})
	}
	return out
}

func scaleFleet(v VariantSpec, robots map[string]Robot, price, qty float64) VariantSpec {
	fleet := make([]FleetSpec, len(v.Fleet))
	copy(fleet, v.Fleet)
	for i := range fleet {
		f := &fleet[i]
		if price != 1 {
			base := 0.0
			if r, ok := robots[f.SolutionID]; ok {
				base = derefFloat(r.PriceRub)
			}
			if f.PriceOverrideRub != nil {
				base = *f.PriceOverrideRub
			}
			p := base * price
			f.PriceOverrideRub = &p
		}
		if qty != 1 && f.Quantity > 0 {
			n := int(math.Ceil(float64(f.Quantity) * qty))
			if n < 1 {
				n = 1
			}
			f.Quantity = n
		}
	}
	v.Fleet = fleet
	return v
}

func buyAndRaas(sc []Scenario) (buy, raas Scenario, ok bool) {
	for _, s := range sc {
		if s.Kind == "buy" && buy.Kind == "" {
			buy = s
		}
		if s.Kind == "raas" && raas.Kind == "" {
			raas = s
		}
	}
	return buy, raas, buy.Kind == "buy"
}
