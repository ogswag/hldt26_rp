// Package profiles holds robot class models for the event simulation.
package profiles

import (
	"math"
	"strings"
)

const (
	AMR    = "amr"
	Pallet = "pallet"

	TaskPalletInbound  = "pallet_inbound"
	TaskPalletPutaway  = "pallet_putaway"
	TaskPalletOutbound = "pallet_outbound"
	TaskPalletMove     = "pallet_move"
	TaskPiecePick      = "piece_pick"

	palletPayloadMinKg = 400
)

// Profile is the action, geometry and energy model of one robot class.
type Profile struct {
	Code  string `json:"code"`
	Label string `json:"label"`

	SpeedMps          float64 `json:"speed_mps"`
	LoadedSpeedFactor float64 `json:"loaded_speed_factor"`
	WidthM            float64 `json:"width_m"`
	LengthM           float64 `json:"length_m"`
	ClearanceM        float64 `json:"clearance_m"`
	PayloadKg         float64 `json:"payload_kg"`

	EnduranceH  float64 `json:"endurance_h"`
	ChargeMin   float64 `json:"charge_min"`
	ChargeBelow float64 `json:"charge_below"`
	ChargeTo    float64 `json:"charge_to"`

	// AMR: latch or lift under the load. Pallet: approach, align and insert forks.
	PickS float64 `json:"pick_s"`
	DropS float64 `json:"drop_s"`
	// Pallet only: fork travel time per meter of lift height and extra alignment at racks.
	LiftSPerM   float64 `json:"lift_s_per_m"`
	RackAlignS  float64 `json:"rack_align_s"`
	TurnPenalty float64 `json:"turn_penalty_s"`

	DrawIdle       float64 `json:"draw_idle"`
	DrawMove       float64 `json:"draw_move"`
	DrawMoveLoaded float64 `json:"draw_move_loaded"`
	DrawHandle     float64 `json:"draw_handle"`

	Tasks []string `json:"tasks"`

	// Assumed lists the fields filled from class defaults because the catalog has no value.
	Assumed []string `json:"assumed,omitempty"`
}

// Specs are catalog values that override class defaults when present.
type Specs struct {
	SpeedMps   *float64
	WidthMm    *float64
	LengthMm   *float64
	PayloadKg  *float64
	EnduranceH *float64
	ChargeMin  *float64
}

func baseAMR() Profile {
	return Profile{
		Code:              AMR,
		Label:             "AMR",
		SpeedMps:          1.5,
		LoadedSpeedFactor: 0.8,
		WidthM:            0.7,
		LengthM:           1.0,
		ClearanceM:        0.5,
		PayloadKg:         600,
		EnduranceH:        8,
		ChargeMin:         60,
		ChargeBelow:       0.30,
		ChargeTo:          0.90,
		PickS:             12,
		DropS:             10,
		TurnPenalty:       2,
		DrawIdle:          0.15,
		DrawMove:          1.0,
		DrawMoveLoaded:    1.3,
		DrawHandle:        0.8,
	}
}

func basePallet() Profile {
	return Profile{
		Code:              Pallet,
		Label:             "Паллетный робот",
		SpeedMps:          1.2,
		LoadedSpeedFactor: 0.65,
		WidthM:            1.2,
		LengthM:           2.4,
		ClearanceM:        0.6,
		PayloadKg:         1400,
		EnduranceH:        8,
		ChargeMin:         75,
		ChargeBelow:       0.25,
		ChargeTo:          0.95,
		PickS:             25,
		DropS:             20,
		LiftSPerM:         4,
		RackAlignS:        15,
		TurnPenalty:       5,
		DrawIdle:          0.10,
		DrawMove:          0.9,
		DrawMoveLoaded:    1.4,
		DrawHandle:        1.4,
	}
}

// Classify picks a class from catalog text and capability codes.
// ok is false for classes sim-v2 does not model yet (cleaners, tugs).
func Classify(name, family, subtype, kind string, capabilities []string) (code string, ok bool) {
	caps := map[string]bool{}
	for _, c := range capabilities {
		caps[c] = true
	}
	text := strings.ToLower(strings.Join([]string{name, family, subtype, kind}, " "))
	switch {
	case caps["cleaning"] || strings.Contains(text, "уборщ") || strings.Contains(text, "поломо"):
		return "", false
	case strings.Contains(text, "тягач") || strings.Contains(text, "tug"):
		return "", false
	case caps["stacker"] || caps["forklift"] || strings.Contains(text, "штабел") || strings.Contains(text, "погрузчик") ||
		strings.Contains(text, "stacker") || strings.Contains(text, "forklift"):
		return Pallet, true
	default:
		return AMR, true
	}
}

// Resolve builds a profile for class code with catalog overrides.
func Resolve(code string, s Specs) Profile {
	var p Profile
	if code == Pallet {
		p = basePallet()
		p.Tasks = []string{TaskPalletInbound, TaskPalletPutaway, TaskPalletOutbound, TaskPalletMove}
	} else {
		p = baseAMR()
	}
	p.Assumed = []string{}
	take := func(field string, v *float64, dst *float64, scale float64) {
		if v != nil && *v > 0 && !math.IsNaN(*v) && !math.IsInf(*v, 0) {
			*dst = *v * scale
			return
		}
		p.Assumed = append(p.Assumed, field)
	}
	take("speed_mps", s.SpeedMps, &p.SpeedMps, 1)
	take("width_mm", s.WidthMm, &p.WidthM, 0.001)
	take("length_mm", s.LengthMm, &p.LengthM, 0.001)
	take("payload_kg", s.PayloadKg, &p.PayloadKg, 1)
	take("endurance_h", s.EnduranceH, &p.EnduranceH, 1)
	take("charge_min", s.ChargeMin, &p.ChargeMin, 1)
	if p.SpeedMps > 3 {
		p.SpeedMps = 3
		p.Assumed = append(p.Assumed, "speed_cap")
	}
	if code != Pallet {
		p.Tasks = []string{TaskPiecePick}
		if p.PayloadKg >= palletPayloadMinKg {
			p.Tasks = append(p.Tasks, TaskPalletInbound, TaskPalletMove)
		}
	}
	return p
}

// Can reports whether the class performs the task type.
func (p Profile) Can(taskType string) bool {
	for _, t := range p.Tasks {
		if t == taskType {
			return true
		}
	}
	return false
}

func (p Profile) RequiredWidthM() float64 {
	return p.WidthM + p.ClearanceM
}

// SpeedFor returns travel speed in m/s.
func (p Profile) SpeedFor(loaded bool) float64 {
	if loaded {
		return p.SpeedMps * p.LoadedSpeedFactor
	}
	return p.SpeedMps
}

// PickTime is the mechanical handling time to take a load. Rack cells add fork travel up and down.
func (p Profile) PickTime(liftM float64, rack bool) float64 {
	t := p.PickS
	if rack {
		t += 2*p.LiftSPerM*liftM + p.RackAlignS
	}
	return t
}

// DropTime is the mechanical handling time to leave a load.
func (p Profile) DropTime(liftM float64, rack bool) float64 {
	t := p.DropS
	if rack {
		t += 2*p.LiftSPerM*liftM + p.RackAlignS
	}
	return t
}

// DrainPerS is the battery share used per second at draw multiplier k.
func (p Profile) DrainPerS(k float64) float64 {
	if p.EnduranceH <= 0 {
		return 0
	}
	return k / (p.EnduranceH * 3600)
}

// ChargePerS is the battery share restored per second on a charger.
func (p Profile) ChargePerS() float64 {
	if p.ChargeMin <= 0 {
		return 0
	}
	return 1 / (p.ChargeMin * 60)
}
