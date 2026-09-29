package importers

import "testing"

func TestTextNormalizer(t *testing.T) {
	n, err := newTextNormalizer()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		`ООО "Ронави Роботикс"`:                       "ООО «Ронави Роботикс»",
		`ООО "Компания "Имя""`:                        "ООО «Компания „Имя“»",
		"Ronavi H1500 (грузоподъемность до 1 500 кг)": "Ronavi H1500 (грузоподъёмность до 1 500 кг)",
		"Внедрен на складе. Еще 3 штабелера":          "Внедрён на складе. Ещё 3 штабелёра",
		"Все роботы":                                  "Все роботы",
		"AMR, без кириллицы":                          "AMR, без кириллицы",
	}
	for in, want := range cases {
		if got := n.Text(in); got != want {
			t.Errorf("Text(%q) = %q, want %q", in, got, want)
		}
	}
}
