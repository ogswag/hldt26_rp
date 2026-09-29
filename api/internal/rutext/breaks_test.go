package rutext

import (
	"strings"
	"testing"
)

// show writes a layout compactly: [a b] is a keep, a~b a no-break join, 1_200 a digit group.
func show(text string) string {
	var one func(p Piece) string
	one = func(p Piece) string {
		if p.Parts == nil {
			return strings.ReplaceAll(p.Word, NBSP, "_")
		}
		out := make([]string, len(p.Parts))
		for i, q := range p.Parts {
			out[i] = one(q)
		}
		if p.Join {
			return strings.Join(out, "~")
		}
		return "[" + strings.Join(out, " ") + "]"
	}
	var out []string
	for _, p := range Layout(text) {
		out = append(out, one(p))
	}
	return strings.Join(out, " ")
}

func TestLayout(t *testing.T) {
	cases := []struct{ text, want string }{
		{"Проверка карты: ошибок 0, предупреждений 1", "Проверка карты: [ошибок 0,] [предупреждений 1]"},
		{"13 шт", "[13 шт]"},
		{"1 500 кг", "[1_500 кг]"},
		{"1\u00a0500 кг", "[1_500 кг]"},
		{"0,5 года", "[0,5 года]"},
		{"Флот 13 шт", "[Флот [13 шт]]"},
		{"Персонал базы 12 чел", "Персонал [базы [12 чел]]"},
		{"Производительность экономики 1 200 строк/ч,", "Производительность [экономики [1_200 строк/ч,]]"},
		{"Горизонт: 5 лет.", "Горизонт: [5 лет.]"},
		{"Склад. 5 шт", "Склад. [5 шт]"},
		{"цена 1,2 млн ₽", "[цена [1,2 млн ₽]]"},
		{"15 000 ₽/мес", "[15_000 ₽/мес]"},
		{"больше чем на 15%", "больше чем на~15%"},
		{"Ожидание до 15 мин, цикл до 40 мин", "Ожидание [до [15 мин,]] цикл [до [40 мин]]"},
		{"1 200 в сутки, единица: паллета", "1_200 в~сутки, единица: паллета"},
		{"сбора ягод с использованием группы", "сбора ягод с~использованием группы"},
		{"и в отделения", "и~в~отделения"},
		{"В расчёте 3 варианта", "[В [расчёте 3]] варианта"},
		{"итого 1\u00a0200\u00a0000\u00a0₽", "итого 1_200_000_₽"},
		{"Ronavi H1500 (грузоподъёмность до 1 500 кг).", "Ronavi H1500 (грузоподъёмность [до [1_500 кг).]]"},
		{"CAPEX 55 212 300 ₽", "[CAPEX [55_212_300 ₽]]"},
		{"", ""},
	}
	for _, c := range cases {
		if got := show(c.text); got != c.want {
			t.Errorf("Layout(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

func TestPieceText(t *testing.T) {
	got := make([]string, 0)
	for _, p := range Layout("Флот 13 шт и в отделения") {
		got = append(got, p.Text())
	}
	want := []string{"Флот 13 шт", "и\u00a0в\u00a0отделения"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Text = %q, want %q", got, want)
	}
}
