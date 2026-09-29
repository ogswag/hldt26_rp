package exporters

import (
	"encoding/json"
	"math"
	"time"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/rutext"
)

func objectLabel(t string) string {
	switch t {
	case objects.Warehouse:
		return "Склад"
	case objects.Airport:
		return "Аэропорт"
	case objects.Hospital:
		return "Медучреждение"
	default:
		return t
	}
}

func scenarioLabel(kind string) string {
	switch kind {
	case "baseline":
		return "База"
	case "buy":
		return "Покупка"
	case "raas":
		return "RaaS"
	default:
		return kind
	}
}

func tariffLabel(t string) string {
	switch t {
	case "fixed":
		return "фиксированный"
	case "variable":
		return "переменный"
	case "mixed":
		return "смешанный"
	default:
		return t
	}
}

func financingLabel(kind, tariff string) string {
	if kind == "raas" && tariff != "" {
		return "RaaS, " + tariffLabel(tariff)
	}
	return scenarioLabel(kind)
}

func verdictLabel(v string) string {
	switch v {
	case "pass":
		return "SLA выполняется"
	case "fail":
		return "SLA нарушается"
	default:
		return v
	}
}

func yesNo(v bool) string {
	if v {
		return "да"
	}
	return "нет"
}

const noData = "нет данных"

func formatRub(v *float64) string {
	if v == nil {
		return noData
	}
	return rub(*v)
}

// rub prints whole rubles with the sign; a no-break space keeps the sign next to the amount.
func rub(v float64) string {
	return rutext.Num(math.Round(v), 0) + "\u00a0₽"
}

// yearsText prints years to one decimal with the agreeing word, e.g. 2,5 года or 5 лет.
func yearsText(v float64) string {
	r := math.Round(v*10) / 10
	return rutext.Num(r, 1) + " " + econ.YearsWord(r)
}

func formatNum(v *float64, digits int) string {
	if v == nil {
		return noData
	}
	return rutext.Num(*v, digits)
}

func num(v float64, digits int) string {
	return rutext.Num(v, digits)
}

func pct(v float64) string {
	return rutext.Pct(v*100, 1)
}

func duration(s float64) string {
	switch {
	case s < 90:
		return num(s, 0) + " с"
	case s < 5400:
		return num(s/60, 1) + " мин"
	default:
		return num(s/3600, 1) + " ч"
	}
}

func paramsMap(raw json.RawMessage) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

// reportDate prints the date of the report as people write it in Russia.
func reportDate(t time.Time) string {
	return t.UTC().Format("02.01.2006")
}
