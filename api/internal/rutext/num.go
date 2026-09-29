// Package rutext writes Russian text: numbers with a decimal comma and digit groups, and where lines may break.
package rutext

import (
	"math"
	"strconv"
	"strings"
)

// Num prints v rounded to at most digits decimals, without trailing zeros: 17,42, 1 500, 0,5.
func Num(v float64, digits int) string {
	s := Fixed(v, digits)
	if strings.Contains(s, ",") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ",")
	}
	return s
}

// Fixed prints v with exactly digits decimals: 2,00.
func Fixed(v float64, digits int) string {
	s := strconv.FormatFloat(math.Abs(v), 'f', digits, 64)
	whole, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	if v < 0 && strings.Trim(s, "0.") != "" {
		b.WriteByte('-')
	}
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteString(NBSP)
		}
		b.WriteRune(c)
	}
	if frac != "" {
		b.WriteByte(',')
		b.WriteString(frac)
	}
	return b.String()
}

// Pct prints v percent with at most digits decimals: 6,5%.
func Pct(v float64, digits int) string {
	return Num(v, digits) + "%"
}

// Plural picks the word form that agrees with n: 1 зарядка, 2 зарядки, 5 зарядок.
func Plural(n int, one, few, many string) string {
	if n < 0 {
		n = -n
	}
	if n%100 >= 11 && n%100 <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	}
	return many
}
