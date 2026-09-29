package econ

import "math"

// PaybackMaxYears is how far the schedule runs to find a payback beyond the horizon.
const PaybackMaxYears = 30

const minPurchaseLife = 0.5

// FlowRow is one year of a scenario cash flow against the baseline. Year 0 is the investment.
type FlowRow struct {
	Year           int     `json:"year"`
	EffectRub      float64 `json:"effect_rub"`
	BatteryRub     float64 `json:"battery_rub,omitempty"`
	ReplacementRub float64 `json:"replacement_rub,omitempty"`
	NetRub         float64 `json:"net_rub"`
	CumulativeRub  float64 `json:"cumulative_rub"`
}

// purchase is the equipment a scenario buys: its price and how long it lives before it is bought again.
type purchase struct {
	Rub  float64
	Life float64
}

type cashCase struct {
	Capex     float64
	Effect    float64
	Purchases []purchase
	Horizon   float64
	Discount  float64
	// BatteryFrac and BatteryYears are the norms of the battery change; zero takes the defaults.
	BatteryFrac  float64
	BatteryYears float64
}

type flowResult struct {
	Rows                   []FlowRow
	PaybackYears           *float64
	DiscountedPaybackYears *float64
	NpvRub                 float64
	IrrPct                 *float64
	RoiPct                 *float64
	BatteryRub             float64
	ReplacementRub         float64
}

func yearOf(t float64) int {
	return int(math.Ceil(t - 1e-9))
}

// lumps places the battery changes and the repeat purchases of every robot in the year they fall, up to until years.
func lumps(purchases []purchase, until, batteryFrac, batteryYears float64) (battery, replacement map[int]float64) {
	battery, replacement = map[int]float64{}, map[int]float64{}
	for _, p := range purchases {
		if p.Rub <= 0 {
			continue
		}
		life := math.Max(p.Life, minPurchaseLife)
		for start := 0.0; start < until-1e-9; start += life {
			if start > 0 {
				replacement[yearOf(start)] += p.Rub
			}
			for t := start + batteryYears; t < start+life-1e-9 && t < until-1e-9; t += batteryYears {
				battery[yearOf(t)] += batteryFrac * p.Rub
			}
		}
	}
	return battery, replacement
}

// crossing is the moment a running total that starts at -capex first reaches zero, linear inside the year.
func crossing(capex float64, flows []float64) *float64 {
	cum := -capex
	for i, f := range flows {
		prev := cum
		cum += f
		if cum >= 0 && f > 0 {
			return fptr(round4(float64(i) + (-prev)/f))
		}
	}
	return nil
}

func npv(rate float64, flows []float64) float64 {
	sum := 0.0
	for t, f := range flows {
		sum += f / math.Pow(1+rate, float64(t))
	}
	return sum
}

// irr is the rate where the flows (flows[0] is the investment) have zero value, nil when there is none between
// -90% and 500%.
func irr(flows []float64) *float64 {
	lo, hi := -0.9, 5.0
	flo, fhi := npv(lo, flows), npv(hi, flows)
	if flo == 0 {
		return fptr(round2(lo * 100))
	}
	if fhi == 0 {
		return fptr(round2(hi * 100))
	}
	if (flo > 0) == (fhi > 0) {
		return nil
	}
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		fm := npv(mid, flows)
		if (fm > 0) == (flo > 0) {
			lo, flo = mid, fm
		} else {
			hi = mid
		}
	}
	return fptr(round2((lo + hi) / 2 * 100))
}

func discounted(flows []float64, rate float64) []float64 {
	out := make([]float64, len(flows))
	for t, f := range flows {
		out[t] = f / math.Pow(1+rate, float64(t+1))
	}
	return out
}

// runCase lays the case out by year: the effect every year, batteries and repeat purchases when they fall, then reads
// payback, NPV, IRR and ROI off it. Payback runs past the horizon up to
// PaybackMaxYears, the other metrics stop at the horizon.
func runCase(c cashCase) flowResult {
	horizon := c.Horizon
	if horizon <= 0 {
		horizon = 5
	}
	years := yearOf(horizon)
	frac, every := c.BatteryFrac, c.BatteryYears
	if frac <= 0 {
		frac = BatteryFrac
	}
	if every <= 0 {
		every = BatteryYears
	}
	battery, replacement := lumps(c.Purchases, horizon, frac, every)

	res := flowResult{Rows: []FlowRow{{Year: 0, NetRub: -c.Capex, CumulativeRub: -c.Capex}}}
	cum := -c.Capex
	net := []float64{-c.Capex}
	for y := 1; y <= years; y++ {
		share := math.Min(1, horizon-float64(y-1))
		row := FlowRow{
			Year:           y,
			EffectRub:      roundRub(c.Effect * share),
			BatteryRub:     roundRub(battery[y]),
			ReplacementRub: roundRub(replacement[y]),
		}
		row.NetRub = row.EffectRub - row.BatteryRub - row.ReplacementRub
		cum += row.NetRub
		row.CumulativeRub = cum
		res.Rows = append(res.Rows, row)
		net = append(net, row.NetRub)
		res.BatteryRub += row.BatteryRub
		res.ReplacementRub += row.ReplacementRub
	}
	res.NpvRub = roundRub(npv(c.Discount, net))
	res.IrrPct = irr(net)
	if c.Capex > 0 {
		gain := cum + c.Capex
		res.RoiPct = fptr(round2((gain - c.Capex) / c.Capex * 100))
	}

	if c.Capex <= 0 {
		return res
	}
	longBattery, longReplacement := lumps(c.Purchases, PaybackMaxYears, frac, every)
	long := make([]float64, PaybackMaxYears)
	for y := 1; y <= PaybackMaxYears; y++ {
		long[y-1] = roundRub(c.Effect) - roundRub(longBattery[y]) - roundRub(longReplacement[y])
	}
	res.PaybackYears = crossing(c.Capex, long)
	res.DiscountedPaybackYears = crossing(c.Capex, discounted(long, c.Discount))
	return res
}
