package exporters

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWrapLines(t *testing.T) {
	cases := []struct {
		text  string
		width int
		want  string
	}{
		{"CAPEX 55 212 300 ₽", 30, "CAPEX 55_212_300 ₽"},
		{"CAPEX 55 212 300 ₽", 14, "CAPEX|55_212_300 ₽"},
		{"CAPEX 55 212 300 ₽", 8, "CAPEX|55 212|300 ₽"},
		{"Флот 13 шт на складе", 12, "Флот 13 шт|на_складе"},
		{"Флот 13 шт", 8, "Флот|13 шт"},
		{"срок до 3 лет", 9, "срок|до 3 лет"},
		{"ошибок нет\nпредупреждений 1", 40, "ошибок нет|предупреждений 1"},
		{"грузоподъёмность", 6, "грузоп|одъёмн|ость"},
		{"", 10, ""},
	}
	for _, c := range cases {
		fits := func(s string) bool { return utf8.RuneCountInString(s) <= c.width }
		got := strings.ReplaceAll(strings.Join(wrapLines(c.text, fits), "|"), "\u00a0", "_")
		if got != c.want {
			t.Errorf("wrapLines(%q, %d) = %q, want %q", c.text, c.width, got, c.want)
		}
	}
}
