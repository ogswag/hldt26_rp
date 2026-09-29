package rutext

import "testing"

func TestNum(t *testing.T) {
	cases := []struct {
		v      float64
		digits int
		want   string
	}{
		{17.4172, 2, "17,42"},
		{1500, 0, "1\u00a0500"},
		{2.5, 1, "2,5"},
		{2, 2, "2"},
		{0.1, 5, "0,1"},
		{-1234567.891, 1, "-1\u00a0234\u00a0567,9"},
		{-0.001, 1, "0"},
		{954.72, 1, "954,7"},
		{0, 0, "0"},
		{999, 0, "999"},
		{-2500000, 0, "-2\u00a0500\u00a0000"},
	}
	for _, c := range cases {
		if got := Num(c.v, c.digits); got != c.want {
			t.Errorf("Num(%v, %d) = %q, want %q", c.v, c.digits, got, c.want)
		}
	}
	if got := Fixed(2, 2); got != "2,00" {
		t.Errorf("Fixed(2, 2) = %q", got)
	}
	if got := Pct(6.46, 1); got != "6,5%" {
		t.Errorf("Pct = %q", got)
	}
}

func TestPlural(t *testing.T) {
	cases := map[int]string{0: "зарядок", 1: "зарядка", 2: "зарядки", 4: "зарядки", 5: "зарядок", 11: "зарядок", 12: "зарядок", 14: "зарядок", 21: "зарядка", 22: "зарядки", 111: "зарядок", 101: "зарядка"}
	for n, want := range cases {
		if got := Plural(n, "зарядка", "зарядки", "зарядок"); got != want {
			t.Errorf("Plural(%d) = %q, want %q", n, got, want)
		}
	}
}
