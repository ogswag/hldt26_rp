package catalog

import "testing"

func TestFamilyCode(t *testing.T) {
	tests := []struct {
		typeLabel, kind, want string
	}{
		{"Мобильные роботы", "brs", FamilyMobile},
		{"  мобильные   роботы ", "brs", FamilyMobile},
		{"БАС", "bas", FamilyUAV},
		{"Автономные наземные транспортные средства", "brs", FamilyGround},
		{"Морские роботы", "brs", FamilyMarine},
		{"Стационарные роботизированные системы", "brs", FamilyStationary},
		{"Роботы-манипуляторы", "brs", FamilyManipulator},
		{"Мобильные манипуляторы", "brs", FamilyManipulator},
		{"Антропоморфные роботы", "brs", FamilyHumanoid},
		{"ПО БРС", "software", FamilySoftware},
		{"ПО", "brs", FamilySoftware},
		{"Другое", "brs", FamilyOther},
		{"Экзоскелеты", "brs", FamilyOther},
		{"", "bas", FamilyUAV},
		{"", "software", FamilySoftware},
		{"", "brs", ""},
		{"", "", ""},
	}
	for _, tc := range tests {
		if got := FamilyCode(tc.typeLabel, tc.kind); got != tc.want {
			t.Errorf("FamilyCode(%q, %q) = %q, want %q", tc.typeLabel, tc.kind, got, tc.want)
		}
	}
}

func TestFamilyLabelsCoverCodes(t *testing.T) {
	for _, code := range FamilyCodes() {
		if !ValidFamily(code) || FamilyLabel(code) == "" {
			t.Errorf("family %q has no label", code)
		}
		if got := FamilyCode(FamilyLabel(code), ""); got != code {
			t.Errorf("label of %q maps back to %q", code, got)
		}
	}
	if ValidFamily("robot") {
		t.Error("unknown code accepted")
	}
}
