// Package exporters renders the PDF and XLSX reports for the person who decides. A saved report is built from one
// immutable run; the run snapshot itself stays in the system and is not exported.
package exporters

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/sim"
)

const (
	KindGuest       = "guest"
	KindCalculation = "calculation"
	KindSimulation  = "simulation"

	LevelHigh    = "high"
	LevelWarning = "warning"
	LevelInfo    = "info"
)

// Report is what a PDF or XLSX file is built from. Run is nil for a guest calculation.
type Report struct {
	GeneratedAt time.Time
	ProjectName string
	ObjectType  string
	Run         *RunInfo
	Inputs      projects.Draft
	Econ        *econ.Result
	Sim         *SimReport
	Catalog     CatalogInfo
}

// RunInfo identifies the stored run the report was built from.
type RunInfo struct {
	ID              string
	Kind            string
	VersionNo       int
	InputHash       string
	EconVersion     string
	SimVersion      string
	ConfidenceLevel string
	StaleVsDraft    bool
	// CreatedAt is when the run was made; zero when unknown.
	CreatedAt time.Time
}

// CatalogInfo lists the solutions the variants use, as the catalog holds them now.
type CatalogInfo struct {
	Items []CatalogItem
}

type CatalogItem struct {
	SolutionID string
	Name       string
	PriceRub   *float64
	SourceURL  string
	// Confidence is the catalog code of where the specs came from: measured, vendor or assumed.
	Confidence string
	SourcedAt  *time.Time
}

// SimReport is the stored summary of a simulation run.
type SimReport struct {
	VariantName string
	MapWarnings []string
	EconCheck   *EconCheck
	Result      sim.Result
}

// EconCheck compares the jobs the fleet did in the simulation with the jobs the economics expects.
type EconCheck struct {
	Flag bool
	Text string
}

// Notice is one line of the warnings list.
type Notice struct {
	Level string
	Text  string
}

// Kind reports whether the report comes from a guest calculation or from a stored run.
func (r Report) Kind() string {
	if r.Run == nil {
		return KindGuest
	}
	return r.Run.Kind
}

// ConfidenceLevel is the run level; a guest calculation is always preliminary.
func (r Report) ConfidenceLevel() string {
	if r.Run == nil || r.Run.ConfidenceLevel == "" {
		return projects.ConfidencePreliminary
	}
	return r.Run.ConfidenceLevel
}

func (r Report) generatedAt() time.Time {
	if r.GeneratedAt.IsZero() {
		return time.Now().UTC()
	}
	return r.GeneratedAt.UTC()
}

// ConfidenceLabel returns the Russian name and meaning of a confidence level.
func ConfidenceLabel(level string) (name, meaning string) {
	switch level {
	case projects.ConfidenceConfigured:
		return "Настроенный", "Маршруты, спрос и длительности введены пользователем. Параметры не оценены по журналам или замерам."
	case projects.ConfidenceCalibrated:
		return "Калиброванный", "Параметры оценены по журналам или замерам объекта."
	case projects.ConfidenceValidated:
		return "Подтверждённый", "Прогноз сопоставлен с фактическим процессом и прошёл порог ошибки."
	default:
		return "Предварительный", "Каталожные данные и нормативы. Фактические маршруты, спрос и длительности не подтверждены."
	}
}

func title(r Report) string {
	switch r.Kind() {
	case KindSimulation:
		return "Отчёт по симуляции"
	default:
		return "Финансовая оценка роботизации"
	}
}

// Filename names a report by its object and the version of the project it was calculated from.
func Filename(r Report, ext string) string {
	ot := strings.TrimSpace(r.ObjectType)
	if ot == "" {
		ot = "object"
	}
	if r.Run == nil {
		return fmt.Sprintf("ocenka-%s.%s", ot, ext)
	}
	prefix := "raschet"
	if r.Run.Kind == KindSimulation {
		prefix = "simulyaciya"
	}
	return fmt.Sprintf("%s-%s-%d.%s", prefix, ot, r.Run.VersionNo, ext)
}

func levelRank(level string) int {
	switch level {
	case LevelHigh:
		return 0
	case LevelWarning:
		return 1
	default:
		return 2
	}
}

// Notices collects the warnings of the run, the economics, the fleet check, the simulation and the map, most severe
// first. The reports keep the high and warning levels.
func Notices(r Report) []Notice {
	var out []Notice
	seen := map[string]bool{}
	add := func(level, text string) {
		text = strings.TrimSpace(text)
		if text == "" || seen[text] {
			return
		}
		seen[text] = true
		out = append(out, Notice{Level: level, Text: text})
	}
	if r.Run != nil {
		if r.Run.StaleVsDraft {
			add(LevelWarning, "Черновик проекта изменён после этого запуска. Отчёт описывает исторический запуск, а не текущие входы.")
		}
		if r.Run.Kind == KindSimulation && r.Run.SimVersion != sim.Version {
			add(LevelWarning, "Запуск посчитан по прежней версии модели симуляции. Повторите запуск, чтобы получить цифры по текущим правилам.")
		}
		if r.Run.Kind == KindCalculation && r.Run.EconVersion != econ.ModelVersion {
			add(LevelWarning, "Запуск посчитан по прежней версии модели расчёта. Пересчитайте проект, чтобы получить цифры по текущим правилам.")
		}
	}
	if r.Econ != nil {
		for _, rk := range r.Econ.Risks {
			if rk.Level == LevelHigh || rk.Level == LevelWarning {
				add(rk.Level, rk.Text)
			}
		}
		for _, c := range r.Econ.SimChecks {
			switch {
			case c.Stale:
				add(LevelWarning, "Проверка симуляцией по варианту «"+c.VariantName+"» устарела: вариант или объект изменились после запуска. Повторите симуляцию.")
			case c.Flag:
				add(LevelHigh, "Вариант «"+c.VariantName+"»: "+c.Text)
			}
		}
		for _, n := range skippedFleet(r) {
			add(LevelWarning, n)
		}
	}
	if r.Sim != nil {
		res := r.Sim.Result
		if res.Verdict == sim.VerdictFail {
			add(LevelHigh, res.VerdictText)
		}
		if r.Sim.EconCheck != nil && r.Sim.EconCheck.Flag {
			add(LevelHigh, r.Sim.EconCheck.Text)
		}
		for _, b := range res.Bottlenecks {
			if b.Primary {
				add(LevelWarning, "Узкое место: "+b.Text)
			}
		}
		for _, w := range res.Warnings {
			add(LevelWarning, w)
		}
		for _, w := range r.Sim.MapWarnings {
			add(LevelWarning, w)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return levelRank(out[i].Level) < levelRank(out[j].Level) })
	return out
}

// skippedFleet names the variant positions the economics could not price.
func skippedFleet(r Report) []string {
	if r.Econ == nil || len(r.Econ.Variants) == 0 {
		return nil
	}
	items := map[string]CatalogItem{}
	for _, it := range r.Catalog.Items {
		items[it.SolutionID] = it
	}
	var out []string
	for _, v := range r.Inputs.Variants {
		used := map[string]bool{}
		for _, vr := range r.Econ.Variants {
			if vr.VariantID == v.ID || (vr.VariantID == "" && vr.Name == v.Name) {
				for _, f := range vr.Fleet {
					used[f.SolutionID] = true
				}
			}
		}
		for _, f := range v.Fleet {
			if f.SolutionID == nil || *f.SolutionID == "" || f.Quantity < 1 || used[*f.SolutionID] {
				continue
			}
			it, ok := items[*f.SolutionID]
			switch {
			case !ok:
				out = append(out, fmt.Sprintf("Вариант %s: одного из роботов нет в каталоге, позиция не вошла в расчёт.", v.Name))
			case it.PriceRub == nil && f.PriceOverrideRub == nil:
				out = append(out, fmt.Sprintf("Вариант %s: у решения %s нет цены, позиция не вошла в первоначальные вложения.", v.Name, it.Name))
			}
		}
	}
	return out
}
