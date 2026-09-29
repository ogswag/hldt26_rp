// Package sim is the discrete-event warehouse simulation (sim-v2).
package sim

import (
	"fmt"
	"math"
	"moscow_hackathon_2026/api/internal/rutext"
)

const (
	Version            = "sim-v2"
	LogSchemaVersion   = "sim-log-v1"
	EventSchemaVersion = "sim-event-v1"

	ModeDeterministic = "deterministic"
	ModeStochastic    = "stochastic"

	PolicyFIFO    = "fifo"
	PolicyNearest = "nearest"
	PolicySLA     = "sla_priority"

	DefaultReplications = 30
	MinReplications     = 2
	MaxReplications     = 30
	DefaultHorizonH     = 8.0
	MaxHorizonH         = 24.0
	DefaultSLATargetPct = 5.0
	MaxRobots           = 200
	MaxProfileValue     = 5.0

	maxEventsPerReplication = 5_000_000
	maxLogEvents            = 400_000
	checkpointEveryS        = 60.0
)

// InputError is a configuration problem the caller can fix.
type InputError struct {
	Msg string
}

func (e *InputError) Error() string { return e.Msg }

func inputErr(format string, args ...any) error {
	return &InputError{Msg: fmt.Sprintf(format, args...)}
}

// Window is an arrival window in hours from the horizon start.
type Window struct {
	StartH float64 `json:"start_h"`
	EndH   float64 `json:"end_h"`
}

// Config is the user-facing simulation request.
type Config struct {
	Mode          string    `json:"mode"`
	Replications  int       `json:"replications"`
	Seed          int64     `json:"seed"`
	Policy        string    `json:"policy"`
	HorizonH      float64   `json:"horizon_h"`
	Schedule      []Window  `json:"schedule,omitempty"`
	DemandProfile []float64 `json:"demand_profile,omitempty"`
	SLATargetPct  *float64  `json:"sla_target_pct,omitempty"`
}

// Normalize fills defaults and rejects values the engine cannot run.
func (c Config) Normalize() (Config, error) {
	if c.Mode == "" {
		c.Mode = ModeStochastic
	}
	switch c.Mode {
	case ModeDeterministic:
		c.Replications = 1
	case ModeStochastic:
		if c.Replications == 0 {
			c.Replications = DefaultReplications
		}
		if c.Replications < MinReplications || c.Replications > MaxReplications {
			return c, inputErr("Число повторов должно быть от %d до %d.", MinReplications, MaxReplications)
		}
	default:
		return c, inputErr("mode должен быть deterministic или stochastic.")
	}
	if c.Seed < 0 || c.Seed > math.MaxInt32 {
		return c, inputErr("Номер случайной выборки должен быть целым числом от 0 до %s.", rutext.Num(math.MaxInt32, 0))
	}
	if c.Policy == "" {
		c.Policy = PolicyFIFO
	}
	switch c.Policy {
	case PolicyFIFO, PolicyNearest, PolicySLA:
	default:
		return c, inputErr("policy должен быть fifo, nearest или sla_priority.")
	}
	if c.HorizonH == 0 {
		c.HorizonH = DefaultHorizonH
	}
	if !finite(c.HorizonH) || c.HorizonH < 0.25 || c.HorizonH > MaxHorizonH {
		return c, inputErr("Горизонт должен быть от 0,25 до %s ч", rutext.Num(MaxHorizonH, 0))
	}
	for i, w := range c.Schedule {
		if !finite(w.StartH) || !finite(w.EndH) || w.StartH < 0 || w.EndH <= w.StartH || w.EndH > c.HorizonH {
			return c, inputErr("Окно расписания %d должно лежать внутри горизонта и иметь конец позже начала.", i+1)
		}
		if i > 0 && w.StartH < c.Schedule[i-1].EndH {
			return c, inputErr("Окна расписания должны идти по возрастанию и не пересекаться.")
		}
	}
	if len(c.DemandProfile) > 0 {
		want := int(math.Ceil(c.HorizonH))
		if len(c.DemandProfile) != want {
			return c, inputErr("Профиль спроса должен содержать %d значений, по одному на час горизонта.", want)
		}
		for _, v := range c.DemandProfile {
			if !finite(v) || v < 0 || v > MaxProfileValue {
				return c, inputErr("Значения профиля спроса должны быть от 0 до %s.", rutext.Num(MaxProfileValue, 0))
			}
		}
	}
	target := DefaultSLATargetPct
	if c.SLATargetPct != nil {
		target = *c.SLATargetPct
	}
	if !finite(target) || target < 0 || target > 100 {
		return c, inputErr("Допустимая доля нарушений SLA должна быть от 0 до 100%%.")
	}
	c.SLATargetPct = &target
	return c, nil
}

func (c Config) targetPct() float64 {
	if c.SLATargetPct == nil {
		return DefaultSLATargetPct
	}
	return *c.SLATargetPct
}

func (c Config) stochastic() bool {
	return c.Mode == ModeStochastic
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// ReplicationSeed derives the seed of replication i.
func ReplicationSeed(base int64, i int) int64 {
	return base + int64(i)
}
