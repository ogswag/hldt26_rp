package geom

import (
	"encoding/json"
	"math"
)

func parseParams(raw json.RawMessage) map[string]any {
	m := map[string]any{}
	if len(raw) == 0 {
		return m
	}
	_ = json.Unmarshal(raw, &m)
	return m
}

func num(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

func cycleGeom(objectType, kind string, m map[string]any) (pathM, handleS float64) {
	switch kind {
	case WorkPallet:
		return 2 * math.Sqrt(math.Max(num(m, "area_active_m2"), 1)), HandlePalletS
	case WorkPiece:
		p := 2 * num(m, "pick_path_m_per_line")
		if p <= 0 {
			p = 50
		}
		return p, HandlePieceS
	case WorkHospitalCart:
		p := 2 * num(m, "kitchen_to_ward_m")
		if p <= 0 {
			p = 360
		}
		return p, HandleHospitalS
	case WorkAirportRamp:
		return 2 * math.Sqrt(math.Max(num(m, "apron_area_m2"), 1)), HandleRampS
	case WorkAirportTrolley:
		return 2 * math.Sqrt(math.Max(num(m, "terminal_area_m2"), 1)), HandleTrolleyS
	case WorkCleaner, WorkAirportCleaner, WorkHospitalCleaner:
		return 0, 0
	default:
		if objectType == "airport" {
			return 2 * math.Sqrt(math.Max(num(m, "terminal_area_m2"), 1)), HandleTrolleyS
		}
		if objectType == "hospital" {
			p := 2 * num(m, "kitchen_to_ward_m")
			if p <= 0 {
				p = 360
			}
			return p, HandleHospitalS
		}
		return 2 * math.Sqrt(math.Max(num(m, "area_active_m2"), 1)), HandlePalletS
	}
}

func legTravelS(pathM, speed, handleS, econTh float64) float64 {
	if speed > 0 && pathM > 0 {
		return (pathM / 2) / speed
	}
	if econTh > 0 {
		cycle := 3600 / econTh
		h := handleS
		if h <= 0 {
			h = cycle * 0.3
		}
		if h > cycle*0.8 {
			h = cycle * 0.4
		}
		rest := cycle - h
		if rest < 2 {
			rest = 2
		}
		return rest / 2
	}
	return 40
}

func cleanerWidthM(widthMm float64) float64 {
	if widthMm > 0 {
		return widthMm / 1000
	}
	return 0.6
}
