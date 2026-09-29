package projects

import (
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/econ"
)

const (
	MaxPriority    = 9
	MaxUnitsPerJob = 10000
	MaxUnitsPerDay = 10_000_000
)

func DefaultProcesses(objectType string) []Process {
	if objectType != "warehouse" {
		return []Process{}
	}
	return []Process{
		{
			Code: "inbound", Name: "Приёмка", TaskType: "pallet_inbound", IsBaseline: true,
			Demand:        Demand{UnitsPerDay: 1000, Unit: "поддон"},
			SLA:           SLA{MaxWaitMin: 30, MaxCycleMin: 45},
			Durations:     Durations{LoadS: 90, UnloadS: 60, TravelS: 180},
			BaselineStaff: Staff{Headcount: 8, Role: "приёмка"},
			SortOrder:     0,
		},
		{
			Code: "putaway", Name: "Размещение", TaskType: "pallet_putaway", IsBaseline: true,
			Demand:        Demand{UnitsPerDay: 1000, Unit: "поддон"},
			SLA:           SLA{MaxWaitMin: 20, MaxCycleMin: 40},
			Durations:     Durations{LoadS: 60, UnloadS: 60, TravelS: 240},
			BaselineStaff: Staff{Headcount: 6, Role: "погрузчик"},
			SortOrder:     1,
		},
		{
			Code: "piece_pick", Name: "Мелкоштучный отбор", TaskType: "piece_pick", IsBaseline: true,
			Demand:        Demand{UnitsPerDay: 100000, Unit: "строка"},
			SLA:           SLA{MaxWaitMin: 15, MaxCycleMin: 25},
			Durations:     Durations{LoadS: 20, UnloadS: 15, TravelS: 90},
			BaselineStaff: Staff{Headcount: 100, Role: "отборщик"},
			SortOrder:     2,
		},
		{
			Code: "outbound", Name: "Отгрузка", TaskType: "pallet_outbound", IsBaseline: true,
			Demand:        Demand{UnitsPerDay: 1000, Unit: "поддон"},
			SLA:           SLA{MaxWaitMin: 25, MaxCycleMin: 40},
			Durations:     Durations{LoadS: 60, UnloadS: 90, TravelS: 180},
			BaselineStaff: Staff{Headcount: 10, Role: "отгрузка"},
			SortOrder:     3,
		},
	}
}

func DefaultVariants() []Variant {
	buy := "buy"
	raas := "raas"
	fixed := "fixed"
	return []Variant{
		emptyVariant("Вариант 1", 0, buy, raas, fixed),
		emptyVariant("Вариант 2", 1, buy, raas, fixed),
		emptyVariant("Вариант 3", 2, buy, raas, fixed),
	}
}

func emptyVariant(name string, order int, buy, raas, tariff string) Variant {
	t := tariff
	return Variant{
		Name:      name,
		Status:    "draft",
		SortOrder: order,
		Fleet:     []FleetItem{},
		Financing: []Financing{
			{Kind: buy, Tariff: nil, Assumptions: []byte(`{}`)},
			{Kind: raas, Tariff: &t, Assumptions: []byte(`{}`)},
		},
	}
}

func ValidateProcess(p Process) string {
	if strings.TrimSpace(p.Code) == "" {
		return "Укажите код процесса."
	}
	if strings.TrimSpace(p.Name) == "" {
		return "Укажите имя процесса."
	}
	switch p.TaskType {
	case "pallet_inbound", "pallet_putaway", "pallet_outbound", "piece_pick", "pallet_move", "cleaning":
	default:
		return "task_type должен быть pallet_inbound, pallet_putaway, pallet_outbound, piece_pick, pallet_move или cleaning."
	}
	if p.SortOrder < 0 {
		return "sort_order не может быть отрицательным."
	}
	if p.Demand.UnitsPerDay < 0 {
		return "Спрос процесса не может быть отрицательным."
	}
	if p.SLA.MaxWaitMin < 0 || p.SLA.MaxCycleMin < 0 {
		return "Параметры SLA процесса не могут быть отрицательными."
	}
	if p.SLA.MaxWaitMin > 0 && p.SLA.MaxCycleMin > 0 && p.SLA.MaxWaitMin > p.SLA.MaxCycleMin {
		return "Максимальное ожидание не может быть больше максимального цикла."
	}
	if p.SLA.Priority < 0 || p.SLA.Priority > MaxPriority {
		return fmt.Sprintf("Приоритет процесса должен быть от 0 до %d.", MaxPriority)
	}
	if p.Demand.UnitsPerJob < 0 || p.Demand.UnitsPerJob > MaxUnitsPerJob {
		return fmt.Sprintf("Единиц в одном задании должно быть от 0 до %d. 0 означает значение по умолчанию.", MaxUnitsPerJob)
	}
	if p.Demand.UnitsPerDay > MaxUnitsPerDay {
		return fmt.Sprintf("Спрос процесса не может превышать %d единиц в сутки.", MaxUnitsPerDay)
	}
	if p.Durations.LoadS < 0 || p.Durations.UnloadS < 0 || p.Durations.TravelS < 0 {
		return "Длительности процесса не могут быть отрицательными."
	}
	if p.BaselineStaff.Headcount < 0 {
		return "Численность процесса не может быть отрицательной."
	}
	return ""
}

func DefaultAssumptionSets() []AssumptionSet {
	return []AssumptionSet{{
		Name:             "Базовый",
		IsActive:         true,
		VATRate:          0.22,
		PricesIncludeVAT: true,
		VATRecoverable:   false,
		LaborCashShare:   1,
		DiscountRate:     DefaultDiscountRate,
		SortOrder:        0,
	}}
}

const DefaultDiscountRate = econ.DefaultDiscountRate

// DiscountOrDefault is the discount rate of a set, or the default for a set saved before the field existed.
func DiscountOrDefault(rate float64) float64 {
	if rate <= 0 {
		return DefaultDiscountRate
	}
	return rate
}

func DefaultSharedCosts() []SharedCost {
	return []SharedCost{}
}

func ValidateSharedCost(c SharedCost) string {
	if c.Code == "" {
		return "Укажите код общей статьи."
	}
	if c.Label == "" {
		return "Укажите имя общей статьи."
	}
	if c.Bucket != "capex" && c.Bucket != "opex" {
		return "bucket общей статьи должен быть capex или opex."
	}
	if c.Rub < 0 {
		return "Сумма общей статьи не может быть отрицательной."
	}
	if c.SortOrder < 0 {
		return "sort_order общей статьи не может быть отрицательным."
	}
	return ""
}

func ValidateAssumptionSet(a AssumptionSet) string {
	if a.Name == "" {
		return "Укажите имя набора допущений."
	}
	if a.VATRate < 0 || a.VATRate > 1 {
		return "Ставка НДС должна быть от 0 до 1."
	}
	if a.LaborCashShare < 0 || a.LaborCashShare > 1 {
		return "Доля денежной экономии труда должна быть от 0 до 1."
	}
	if a.DiscountRate != 0 && (a.DiscountRate < 0.01 || a.DiscountRate > 1) {
		return "Ставка дисконтирования должна быть от 1% до 100%."
	}
	for _, f := range []struct {
		v        *float64
		min, max float64
		text     string
	}{
		{a.Utilization, 0.01, 1, "Загрузка робота должна быть от 1% до 100%."},
		{a.Availability, 0.01, 1, "Доступность должна быть от 1% до 100%."},
		{a.Reserve, 0, 1, "Резерв флота должен быть от 0 до 100%."},
		{a.ServiceShare, 0, 1, "Доля сервиса должна быть от 0 до 100%."},
		{a.DeliveryShare, 0, 1, "Доля доставки должна быть от 0 до 100%."},
		{a.CommRubPerRobotYear, 0, 10_000_000, "Связь на робота в год должна быть от 0 до 10 000 000 ₽."},
		{a.TechnicianWageMonthRub, 0, 10_000_000, "Зарплата техника в месяц должна быть от 0 до 10 000 000 ₽."},
	} {
		if f.v != nil && (*f.v < f.min || *f.v > f.max || math.IsNaN(*f.v)) {
			return f.text
		}
	}
	if a.SortOrder < 0 {
		return "sort_order набора допущений не может быть отрицательным."
	}
	return ""
}

func ActiveAssumptionSet(sets []AssumptionSet) AssumptionSet {
	for _, s := range sets {
		if s.IsActive {
			return s
		}
	}
	if len(sets) > 0 {
		return sets[0]
	}
	d := DefaultAssumptionSets()
	return d[0]
}

func ValidateVariant(v Variant) string {
	if strings.TrimSpace(v.Name) == "" {
		return "Укажите имя варианта."
	}
	if v.Status != "draft" && v.Status != "ready" {
		return "status варианта должен быть draft или ready."
	}
	if v.SortOrder < 0 {
		return "sort_order варианта не может быть отрицательным."
	}
	for _, item := range v.Fleet {
		if item.SolutionID != nil && *item.SolutionID != "" {
			if _, err := uuid.Parse(*item.SolutionID); err != nil {
				return "solution_id позиции флота должен быть UUID решения из каталога."
			}
		}
		if item.Quantity < 1 {
			return "Количество в позиции флота должно быть целым числом >= 1."
		}
		if item.PriceOverrideRub != nil && strings.TrimSpace(item.PriceOverrideReason) == "" {
			return "Для проектной цены укажите источник или причину."
		}
		if item.PriceOverrideRub != nil && *item.PriceOverrideRub < 0 {
			return "Проектная цена не может быть отрицательной."
		}
		if item.SortOrder < 0 {
			return "sort_order позиции флота не может быть отрицательным."
		}
		for _, code := range item.TaskCodes {
			if strings.TrimSpace(code) == "" {
				return "Код процесса в позиции флота не должен быть пустым."
			}
		}
	}
	seen := make(map[string]bool)
	for _, f := range v.Financing {
		if f.Kind != "buy" && f.Kind != "raas" {
			return "kind финансирования должен быть buy или raas."
		}
		if f.Kind == "raas" && f.Tariff != nil {
			switch *f.Tariff {
			case "fixed", "variable", "mixed":
			default:
				return "tariff RaaS должен быть fixed, variable или mixed."
			}
		}
		if f.Kind == "buy" && f.Tariff != nil {
			return "Для покупки tariff должен быть null."
		}
		key := f.Kind
		if f.Kind == "raas" {
			key += ":fixed"
			if f.Tariff != nil {
				key = f.Kind + ":" + *f.Tariff
			}
		}
		if seen[key] {
			return "Сценарии финансирования не должны повторяться."
		}
		seen[key] = true
	}
	return ""
}
