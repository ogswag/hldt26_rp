package sim

import (
	"math"
)

type arrivalSeg struct {
	start float64
	end   float64
	rate  float64
	cum0  float64
}

// DefaultProfile returns hourly demand multipliers with two peaks that reach peakFactor
// before normalization to a mean of 1 over the active hours.
func DefaultProfile(hours int, windows []Window, peakFactor float64) []float64 {
	if hours < 1 {
		return nil
	}
	if peakFactor < 1 {
		peakFactor = 1
	}
	out := make([]float64, hours)
	span := 0.0
	for _, w := range windows {
		span = math.Max(span, w.EndH)
	}
	if span <= 0 {
		span = float64(hours)
	}
	tri := func(x, c, half float64) float64 {
		v := 1 - math.Abs(x-c)/half
		if v < 0 {
			return 0
		}
		return v
	}
	sum, n := 0.0, 0
	for h := 0; h < hours; h++ {
		x := float64(h) + 0.5
		bump := math.Max(tri(x, 0.3*span, 0.2*span), tri(x, 0.7*span, 0.2*span))
		out[h] = 0.75 + (peakFactor-0.75)*bump
		if activeShare(windows, float64(h), float64(h+1)) > 0 {
			sum += out[h]
			n++
		}
	}
	if n > 0 && sum > 0 {
		mean := sum / float64(n)
		for h := range out {
			out[h] = math.Round(out[h]/mean*1000) / 1000
		}
	}
	return out
}

func activeShare(windows []Window, a, b float64) float64 {
	if b <= a {
		return 0
	}
	cover := 0.0
	for _, w := range windows {
		lo := math.Max(a, w.StartH)
		hi := math.Min(b, w.EndH)
		if hi > lo {
			cover += hi - lo
		}
	}
	return cover / (b - a)
}

// ResolveWindows returns config windows or a single window of activeH hours clipped to the horizon.
func ResolveWindows(cfg Config, activeH float64) []Window {
	if len(cfg.Schedule) > 0 {
		return append([]Window(nil), cfg.Schedule...)
	}
	end := cfg.HorizonH
	if activeH > 0 && activeH < end {
		end = activeH
	}
	return []Window{{StartH: 0, EndH: end}}
}

func (m *Model) buildArrivals() {
	windows := m.sc.Windows
	if len(windows) == 0 {
		windows = ResolveWindows(m.cfg, m.sc.ActiveHoursPerDay)
		m.sc.Windows = windows
	}
	hours := int(math.Ceil(m.cfg.HorizonH))
	profile := m.cfg.DemandProfile
	if len(profile) == 0 {
		profile = DefaultProfile(hours, windows, m.sc.PeakFactor)
	}
	active := m.sc.ActiveHoursPerDay
	if active <= 0 || active > 24 {
		active = 24
	}
	for _, p := range m.procs {
		if !p.covered {
			continue
		}
		perHour := p.spec.UnitsPerDay / p.spec.UnitsPerJob / active
		cum := 0.0
		for _, w := range windows {
			for h := int(math.Floor(w.StartH)); h < hours && float64(h) < w.EndH; h++ {
				lo := math.Max(float64(h), w.StartH)
				hi := math.Min(float64(h+1), w.EndH)
				if hi <= lo {
					continue
				}
				rate := perHour * profile[h] / 3600
				seg := arrivalSeg{start: lo * 3600, end: hi * 3600, rate: rate, cum0: cum}
				cum += rate * (seg.end - seg.start)
				p.segs = append(p.segs, seg)
			}
		}
		p.total = cum
	}
}

// arrivalAt inverts the cumulative intensity: the time when target expected arrivals are reached.
func (p *procModel) arrivalAt(target float64) (float64, bool) {
	if target >= p.total {
		return 0, false
	}
	for _, s := range p.segs {
		cum1 := s.cum0 + s.rate*(s.end-s.start)
		if target < cum1 && s.rate > 0 {
			return s.start + (target-s.cum0)/s.rate, true
		}
	}
	return 0, false
}

// ExpectedArrivals is the mean number of jobs per process over the horizon.
func (m *Model) ExpectedArrivals() map[string]float64 {
	out := map[string]float64{}
	for _, p := range m.procs {
		if p.covered {
			out[p.spec.Code] = p.total
		}
	}
	return out
}
