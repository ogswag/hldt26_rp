package econ

// Calculate fills baseline, buy, and raas KPIs for model econ-v3.
func Calculate(in Input) (Result, error) {
	if len(in.Variants) > 0 {
		return compareProject(in)
	}
	res, err := evaluate(in)
	if err != nil {
		return Result{}, err
	}
	if res.Shared == nil {
		return res, nil
	}
	res.Sensitivity = sensitivity(in)
	res.Assumptions = append(res.Assumptions, "В чувствительности сдвиг объёма меняет и флот: число роботов считается от нового пика. База ФОТ не меняется.")
	return res, nil
}

func sensitivity(in Input) []SensitivityRow {
	type step struct {
		param string
		delta float64
		apply func(Overrides, float64) Overrides
	}
	steps := []step{
		{"equipment_price", -20, func(ov Overrides, d float64) Overrides {
			base := robotPrice(in)
			if ov.PriceRub != nil {
				base = *ov.PriceRub
			}
			p := base * (1 + d/100)
			ov.PriceRub = &p
			return ov
		}},
		{"equipment_price", 20, func(ov Overrides, d float64) Overrides {
			base := robotPrice(in)
			if ov.PriceRub != nil {
				base = *ov.PriceRub
			}
			p := base * (1 + d/100)
			ov.PriceRub = &p
			return ov
		}},
		{"volume", -20, func(ov Overrides, d float64) Overrides {
			vf := 1.0
			if ov.VolumeFactor != nil {
				vf = *ov.VolumeFactor
			}
			x := vf * (1 + d/100)
			ov.VolumeFactor = &x
			return ov
		}},
		{"volume", 20, func(ov Overrides, d float64) Overrides {
			vf := 1.0
			if ov.VolumeFactor != nil {
				vf = *ov.VolumeFactor
			}
			x := vf * (1 + d/100)
			ov.VolumeFactor = &x
			return ov
		}},
		{"labor", -20, func(ov Overrides, d float64) Overrides {
			lf := 1.0
			if ov.LaborFactor != nil {
				lf = *ov.LaborFactor
			}
			x := lf * (1 + d/100)
			ov.LaborFactor = &x
			return ov
		}},
		{"labor", 20, func(ov Overrides, d float64) Overrides {
			lf := 1.0
			if ov.LaborFactor != nil {
				lf = *ov.LaborFactor
			}
			x := lf * (1 + d/100)
			ov.LaborFactor = &x
			return ov
		}},
	}
	out := make([]SensitivityRow, 0, len(steps))
	for _, st := range steps {
		clone := in
		clone.Overrides = st.apply(in.Overrides, st.delta)
		got, err := evaluate(clone)
		if err != nil || len(got.Scenarios) < 3 {
			continue
		}
		out = append(out, SensitivityRow{
			Param:    st.param,
			DeltaPct: st.delta,
			Buy:      got.Scenarios[1],
			Raas:     got.Scenarios[2],
		})
	}
	return out
}

func robotPrice(in Input) float64 {
	if in.Robot == nil || in.Robot.PriceRub == nil {
		return 0
	}
	return *in.Robot.PriceRub
}
