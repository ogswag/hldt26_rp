package econ

import (
	"math"
	"testing"
)

func flowNets(rows []FlowRow) []float64 {
	out := make([]float64, len(rows))
	for i, r := range rows {
		out[i] = r.NetRub
	}
	return out
}

func TestRunCaseTable(t *testing.T) {
	cases := []struct {
		name        string
		in          cashCase
		nets        []float64
		payback     *float64
		discounted  *float64
		npv         float64
		irr         *float64
		roi         *float64
		battery     float64
		replacement float64
	}{
		{
			name:       "no purchases, annuity",
			in:         cashCase{Capex: 1000, Effect: 400, Horizon: 5, Discount: 0.12},
			nets:       []float64{-1000, 400, 400, 400, 400, 400},
			payback:    fptr(2.5),
			discounted: fptr(3.1545),
			npv:        442,
			irr:        fptr(28.65),
			roi:        fptr(100),
		},
		{
			name:        "battery lands in year 4 and moves payback that ends after it",
			in:          cashCase{Capex: 1000, Effect: 300, Horizon: 6, Discount: 0.10, Purchases: []purchase{{Rub: 1000, Life: 10}}},
			nets:        []float64{-1000, 300, 300, 300, 150, 300, 300},
			payback:     fptr(3.6667),
			discounted:  fptr(4.8133),
			npv:         204,
			irr:         fptr(16.87),
			roi:         fptr(65),
			battery:     150,
			replacement: 0,
		},
		{
			name:       "battery after the payback does not move it",
			in:         cashCase{Capex: 1000, Effect: 500, Horizon: 6, Discount: 0.10, Purchases: []purchase{{Rub: 1000, Life: 10}}},
			nets:       []float64{-1000, 500, 500, 500, 350, 500, 500},
			payback:    fptr(2),
			discounted: fptr(2.352),
			battery:    150,
			npv:        1075,
			irr:        fptr(42.48),
			roi:        fptr(185),
		},
		{
			name:        "horizon longer than life buys the robot again",
			in:          cashCase{Capex: 1000, Effect: 300, Horizon: 9, Discount: 0.10, Purchases: []purchase{{Rub: 1000, Life: 7}}},
			nets:        []float64{-1000, 300, 300, 300, 150, 300, 300, -700, 300, 300},
			payback:     fptr(3.6667),
			npv:         112,
			irr:         fptr(13.83),
			roi:         fptr(55),
			battery:     150,
			replacement: 1000,
		},
		{
			name:    "fractional horizon scales the last year",
			in:      cashCase{Capex: 1000, Effect: 400, Horizon: 2.5, Discount: 0.10},
			nets:    []float64{-1000, 400, 400, 200},
			payback: fptr(2.5),
			npv:     -156,
			roi:     fptr(0),
		},
		{
			name: "effect too small to ever pay back",
			in:   cashCase{Capex: 1000, Effect: 10, Horizon: 5, Discount: 0.10},
			nets: []float64{-1000, 10, 10, 10, 10, 10},
			npv:  -962,
			roi:  fptr(-95),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runCase(c.in)
			if nets := flowNets(got.Rows); len(nets) != len(c.nets) {
				t.Fatalf("nets %v, want %v", nets, c.nets)
			} else {
				for i := range nets {
					if math.Abs(nets[i]-c.nets[i]) > 1e-9 {
						t.Fatalf("nets %v, want %v", nets, c.nets)
					}
				}
			}
			same := func(name string, a, b *float64) {
				t.Helper()
				if (a == nil) != (b == nil) || (a != nil && math.Abs(*a-*b) > 1e-9) {
					t.Errorf("%s got %v want %v", name, derefFloat(a), derefFloat(b))
				}
			}
			same("payback", got.PaybackYears, c.payback)
			if c.discounted != nil {
				same("discounted payback", got.DiscountedPaybackYears, c.discounted)
			}
			if c.irr != nil {
				same("irr", got.IrrPct, c.irr)
			}
			same("roi", got.RoiPct, c.roi)
			if got.NpvRub != c.npv {
				t.Errorf("npv %v, want %v", got.NpvRub, c.npv)
			}
			if got.BatteryRub != c.battery || got.ReplacementRub != c.replacement {
				t.Errorf("lumps battery %v replacement %v, want %v %v", got.BatteryRub, got.ReplacementRub, c.battery, c.replacement)
			}
		})
	}
}

func TestDiscountedPaybackNotBeforeSimple(t *testing.T) {
	for _, effect := range []float64{250, 400, 900} {
		got := runCase(cashCase{Capex: 1000, Effect: effect, Horizon: 5, Discount: 0.15})
		if got.PaybackYears == nil || got.DiscountedPaybackYears == nil {
			t.Fatalf("effect %v: no payback", effect)
		}
		if *got.DiscountedPaybackYears < *got.PaybackYears {
			t.Errorf("effect %v: discounted %v before simple %v", effect, *got.DiscountedPaybackYears, *got.PaybackYears)
		}
	}
}

func TestNPVAtZeroRateIsTheSum(t *testing.T) {
	flows := []float64{-1000, 300, 300, 150, 300, 700}
	if got := npv(0, flows); got != 750 {
		t.Fatalf("npv at 0 = %v, want 750", got)
	}
}

func TestIRRTable(t *testing.T) {
	cases := []struct {
		name  string
		flows []float64
		want  *float64
	}{
		{"one period", []float64{-100, 110}, fptr(10)},
		{"never positive", []float64{-100, -10, -10}, nil},
		{"never negative", []float64{100, 10}, nil},
	}
	for _, c := range cases {
		got := irr(c.flows)
		if (got == nil) != (c.want == nil) || (got != nil && math.Abs(*got-*c.want) > 1e-9) {
			t.Errorf("%s: irr %v, want %v", c.name, derefFloat(got), derefFloat(c.want))
		}
	}
}

func TestNPVAtIRRIsZero(t *testing.T) {
	got := runCase(cashCase{Capex: 1000, Effect: 300, Horizon: 9, Discount: 0.10, Purchases: []purchase{{Rub: 1000, Life: 7}}})
	if got.IrrPct == nil {
		t.Fatal("no irr")
	}
	if v := npv(*got.IrrPct/100, flowNets(got.Rows)); math.Abs(v) > 5 {
		t.Fatalf("npv at irr = %v", v)
	}
}

func TestLumpsPerPurchaseCycle(t *testing.T) {
	battery, replacement := lumps([]purchase{{Rub: 1000, Life: 5}}, 12, BatteryFrac, BatteryYears)
	wantBattery := map[int]float64{4: 150, 9: 150}
	wantReplacement := map[int]float64{5: 1000, 10: 1000}
	for y, v := range wantBattery {
		if battery[y] != v {
			t.Errorf("battery year %d = %v, want %v", y, battery[y], v)
		}
	}
	if len(battery) != len(wantBattery) {
		t.Errorf("battery %v", battery)
	}
	for y, v := range wantReplacement {
		if replacement[y] != v {
			t.Errorf("replacement year %d = %v, want %v", y, replacement[y], v)
		}
	}
	if len(replacement) != len(wantReplacement) {
		t.Errorf("replacement %v", replacement)
	}
}
