package simbuild

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/rutext"
)

const defaultLiftM = 3

type siteParams struct {
	peak        float64
	windowH     float64
	shiftsH     float64
	mainAisle   float64
	workAisle   float64
	liftM       float64
	liftAssumed bool
}

func readParams(raw json.RawMessage) siteParams {
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	num := func(k string) float64 {
		if v, ok := m[k].(float64); ok && !math.IsNaN(v) {
			return v
		}
		return 0
	}
	p := siteParams{
		peak:      num("peak_load_factor"),
		shiftsH:   num("shifts_per_day") * num("shift_hours"),
		mainAisle: num("aisle_main_m"),
		workAisle: num("aisle_working_m"),
	}
	if p.peak < 1 {
		p.peak = 1.5
	}
	if w, ok := m["shift_window"].(map[string]any); ok {
		s, _ := w["start"].(string)
		e, _ := w["end"].(string)
		if a, okA := clockHours(s); okA {
			if b, okB := clockHours(e); okB {
				if b <= a {
					b += 24
				}
				p.windowH = b - a
			}
		}
	}
	if h := num("ceiling_height_m"); h > 0 {
		p.liftM = math.Min(math.Max((h-2)/2, 0.5), 6)
	} else {
		p.liftM = defaultLiftM
		p.liftAssumed = true
	}
	return p
}

func clockHours(s string) (float64, bool) {
	norm, ok := objects.NormalizeClock(s)
	if !ok {
		return 0, false
	}
	h, err1 := strconv.Atoi(norm[:2])
	m, err2 := strconv.Atoi(norm[3:])
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return float64(h) + float64(m)/60, true
}

// activeHours picks the working hours per day that spread daily demand.
func (p siteParams) activeHours() (float64, string) {
	switch {
	case p.windowH > 0:
		note := ""
		if p.shiftsH > 0 && math.Abs(p.shiftsH-p.windowH) > 1 {
			note = fmt.Sprintf("Окно смены %s ч не совпадает со сменами %s ч в сутки. Спрос распределён по окну смены.", rutext.Num(p.windowH, 1), rutext.Num(p.shiftsH, 1))
		}
		return p.windowH, note
	case p.shiftsH > 0:
		return math.Min(p.shiftsH, 24), ""
	default:
		return 24, "Рабочее окно не задано, спрос распределён на 24 ч"
	}
}
