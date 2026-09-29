package catalog

import "strings"

// Family codes group robots for colors, icons and filters. They follow the organizer's «Тип» column.
const (
	FamilyMobile      = "mobile"
	FamilyUAV         = "uav"
	FamilyGround      = "ground"
	FamilyMarine      = "marine"
	FamilyStationary  = "stationary"
	FamilyManipulator = "manipulator"
	FamilyHumanoid    = "humanoid"
	FamilySoftware    = "software"
	FamilyOther       = "other"
)

// familyTypes maps the organizer's «Тип» labels, folded, to family codes.
var familyTypes = map[string]string{
	"мобильные роботы": FamilyMobile,
	"бас":              FamilyUAV,
	"автономные наземные транспортные средства": FamilyGround,
	"морские роботы": FamilyMarine,
	"стационарные роботизированные системы": FamilyStationary,
	"роботы-манипуляторы":                   FamilyManipulator,
	"мобильные манипуляторы":                FamilyManipulator,
	"антропоморфные роботы":                 FamilyHumanoid,
	"по брс": FamilySoftware,
	"по":     FamilySoftware,
	"другое": FamilyOther,
}

// familyLabels are the «Тип» labels written back to a solution's raw payload, which matching reads.
var familyLabels = map[string]string{
	FamilyMobile:      "Мобильные роботы",
	FamilyUAV:         "БАС",
	FamilyGround:      "Автономные наземные транспортные средства",
	FamilyMarine:      "Морские роботы",
	FamilyStationary:  "Стационарные роботизированные системы",
	FamilyManipulator: "Роботы-манипуляторы",
	FamilyHumanoid:    "Антропоморфные роботы",
	FamilySoftware:    "ПО БРС",
	FamilyOther:       "Другое",
}

// FamilyCodes lists every family code in display order.
func FamilyCodes() []string {
	return []string{
		FamilyMobile, FamilyUAV, FamilyGround, FamilyMarine, FamilyStationary,
		FamilyManipulator, FamilyHumanoid, FamilySoftware, FamilyOther,
	}
}

// ValidFamily reports whether code is a family code.
func ValidFamily(code string) bool {
	_, ok := familyLabels[code]
	return ok
}

// FamilyLabel is the organizer's «Тип» label of a family code, or "" for an unknown code.
func FamilyLabel(code string) string {
	return familyLabels[code]
}

// FamilyCode maps a «Тип» label and a kind code to a family. An empty label follows the kind; "" means unknown.
func FamilyCode(typeLabel, kind string) string {
	t := strings.Join(strings.Fields(fold(typeLabel)), " ")
	if t == "" {
		switch kind {
		case "bas":
			return FamilyUAV
		case "software":
			return FamilySoftware
		}
		return ""
	}
	if code, ok := familyTypes[t]; ok {
		return code
	}
	return FamilyOther
}
