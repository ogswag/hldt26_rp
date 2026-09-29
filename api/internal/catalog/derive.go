package catalog

type SpecInput struct {
	Name        string
	Family      string
	Subtype     string
	Scenario    string
	ObjectTypes []string
	PayloadKg   *float64
	WidthMm     *float64
	MinAisleMm  *float64
	TempMinC    *float64
	TempMaxC    *float64
}

func DeriveCapabilities(in SpecInput) []string {
	seen := make(map[string]struct{}, 8)
	add := func(code string) {
		if code == "" {
			return
		}
		if _, ok := seen[code]; ok {
			return
		}
		seen[code] = struct{}{}
	}
	for _, t := range in.ObjectTypes {
		if t == "warehouse" {
			add(CapWarehouseIndoor)
		}
	}
	if in.WidthMm != nil || in.MinAisleMm != nil {
		add(CapAisleRated)
	}
	if in.TempMinC != nil && in.TempMaxC != nil {
		add(CapTempRated)
	}
	if IsCleaner(in.Name, in.Subtype, in.Family, in.Scenario) {
		add(CapCleaning)
	}
	if in.PayloadKg != nil && *in.PayloadKg > 0 {
		add(CapPayloadUnit)
		if *in.PayloadKg >= PalletPayloadMinKg {
			add(CapPayloadPallet)
		}
	}
	text := fold(in.Name + " " + in.Subtype + " " + in.Family + " " + in.Scenario)
	if containsAny(text, []string{"amr", "мобильн"}) {
		add(CapAMR)
	}
	if containsAny(text, []string{"штабел"}) {
		add(CapStacker)
	}
	if containsAny(text, []string{"погрузчик", "forklift"}) {
		add(CapForklift)
	}
	out := make([]string, 0, len(seen))
	for _, e := range Capabilities() {
		if _, ok := seen[e.Code]; ok {
			out = append(out, e.Code)
		}
	}
	return out
}
