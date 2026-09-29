package econ

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

func parseParams(raw json.RawMessage) (map[string]any, error) {
	m := map[string]any{}
	if len(raw) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("econ.params: %w", err)
	}
	return m, nil
}

func numOK(m map[string]any, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int32:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func num(m map[string]any, key string) float64 {
	v, ok := numOK(m, key)
	if !ok {
		return 0
	}
	return v
}

func str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprint(v)
	}
	return s
}

func isYes(s string) bool {
	t := fold(strings.TrimSpace(s))
	return strings.HasPrefix(t, "да") || t == "yes" || t == "true"
}

func boolKnownYes(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return isYes(t)
	default:
		return isYes(fmt.Sprint(t))
	}
}

func hasIT(objectType string, m map[string]any) bool {
	switch objectType {
	case "warehouse":
		return boolKnownYes(m, "has_wms")
	case "airport":
		return boolKnownYes(m, "has_fids_aodb")
	case "hospital":
		return boolKnownYes(m, "has_mis")
	default:
		return false
	}
}

func hoursPerDay(objectType string, m map[string]any) float64 {
	switch objectType {
	case "warehouse":
		h := num(m, "shifts_per_day") * num(m, "shift_hours")
		if h <= 0 {
			return 22
		}
		return h
	case "airport":
		return AirportHoursPerDay
	case "hospital":
		return HospitalHoursPerDay
	default:
		return 22
	}
}

// shiftsPerDay is how many shifts the site works: the warehouse parameter, otherwise the working day in eight-hour
// shifts, rounded up.
func shiftsPerDay(objectType string, m map[string]any) float64 {
	if objectType == "warehouse" {
		if s := num(m, "shifts_per_day"); s > 0 {
			return math.Ceil(s)
		}
	}
	return math.Ceil(hoursPerDay(objectType, m) / ShiftHours)
}

func workdays(objectType string, m map[string]any) float64 {
	if objectType == "warehouse" {
		d := num(m, "workdays_per_year")
		if d > 0 {
			return d
		}
	}
	return 365
}

func taxFactor(m map[string]any) float64 {
	t := num(m, "payroll_tax_factor")
	if t <= 0 {
		return DefaultTaxFactor
	}
	return t
}

func horizonYears(m map[string]any) float64 {
	h := num(m, "payback_horizon_years")
	if h <= 0 {
		return 5
	}
	return h
}

func defaultWork(objectType string) string {
	switch objectType {
	case "airport":
		return WorkAirportTrolley
	case "hospital":
		return WorkHospitalCart
	default:
		return WorkPallet
	}
}

func classifyWork(objectType string, r Robot) string {
	if robotCleaner(r) {
		switch objectType {
		case "airport":
			return WorkAirportCleaner
		case "hospital":
			return WorkHospitalCleaner
		default:
			return WorkCleaner
		}
	}
	switch objectType {
	case "hospital":
		return WorkHospitalCart
	case "airport":
		if robotRamp(r) {
			return WorkAirportRamp
		}
		return WorkAirportTrolley
	default:
		if robotPiece(r) {
			return WorkPiece
		}
		return WorkPallet
	}
}

func robotCleaner(r Robot) bool {
	t := fold(r.Name + " " + deref(r.Subtype) + " " + r.Family + " " + deref(r.Scenario) + " " + r.Description)
	return containsAny(t, []string{"уборщик", "уборка помещений", "поломо", "клинботикс", "mark 2"})
}

func robotRamp(r Robot) bool {
	t := fold(r.Name + " " + deref(r.Subtype) + " " + r.Family + " " + deref(r.Scenario))
	if containsAny(t, []string{"тягач", "tug", "evocargo", "cognitive"}) {
		return true
	}
	return r.SpeedMps != nil && *r.SpeedMps >= 5
}

func robotPiece(r Robot) bool {
	t := fold(r.Name + " " + deref(r.Subtype) + " " + r.Family)
	if containsAny(t, []string{"g2p", "piece", "tote", "мелкоштуч"}) {
		return true
	}
	return r.PayloadKg != nil && *r.PayloadKg > 0 && *r.PayloadKg < 200
}

func airportPeakFactor(m map[string]any, hours float64) float64 {
	day := num(m, "passengers_per_day")
	peak := num(m, "passengers_peak_per_hour")
	if hours > 0 && day > 0 && peak > 0 {
		avg := day / hours
		if avg > 0 {
			pf := peak / avg
			if pf < 1 {
				return 1
			}
			if pf > 3 {
				return 3
			}
			return pf
		}
	}
	return 1.5
}

func hospitalTrips(m map[string]any) float64 {
	meals := num(m, "meal_drop_points") * num(m, "meals_per_day")
	linen := num(m, "linen_points") * num(m, "linen_changes_per_day")
	waste := num(m, "waste_points") * num(m, "waste_trips_per_day")
	med := num(m, "med_orders_per_day")
	lab := num(m, "lab_result_trips_per_day")
	cons := num(m, "consumable_trips_per_day")
	return meals + linen + waste + med + lab + cons
}

func peakOps(objectType, kind string, m map[string]any, volume float64) (float64, string) {
	hours := hoursPerDay(objectType, m)
	if hours <= 0 {
		hours = 22
	}
	if volume <= 0 {
		volume = 1
	}
	switch kind {
	case WorkPallet:
		return dayPeak(objectType, m, num(m, "inbound_pallets_per_day")+num(m, "outbound_pallets_per_day"), volume), "поддон/ч"
	case WorkPiece:
		return dayPeak(objectType, m, num(m, "pick_lines_per_day"), volume), "строк/ч"
	case WorkCleaner:
		area := num(m, "area_active_m2")
		if area <= 0 {
			area = num(m, "area_total_m2")
		}
		return area / hours * volume, "м²/ч"
	case WorkAirportCleaner:
		return num(m, "cleaning_area_m2") / hours * volume, "м²/ч"
	case WorkAirportRamp:
		return num(m, "flights_peak_per_hour") * volume, "рейс/ч"
	case WorkAirportTrolley:
		pf := airportPeakFactor(m, hours)
		return num(m, "internal_trolley_trips_per_day") / hours * pf * volume, "рейс/ч"
	case WorkHospitalCart:
		return hospitalTrips(m) / hours * HospitalPeakFactor * volume, "рейс/ч"
	case WorkHospitalCleaner:
		return num(m, "area_total_m2") * 0.6 / hours * volume, "м²/ч"
	default:
		return 0, "1/ч"
	}
}

// dayPeak turns a warehouse daily volume into the peak hour a fleet is sized for: the working hours of the site and
// its peak factor.
func dayPeak(objectType string, m map[string]any, day, volume float64) float64 {
	hours := hoursPerDay(objectType, m)
	if hours <= 0 {
		hours = 22
	}
	if volume <= 0 {
		volume = 1
	}
	pf := num(m, "peak_load_factor")
	if pf <= 0 {
		pf = 1.5
	}
	return day / hours * pf * volume
}

// processPeak is the peak hour of the baseline processes that robots of one kind of work serve (codes, or all of
// that kind when codes is empty), in the unit of that kind. ok is false when those processes carry no demand; the
// peak then comes from the site parameters (peakOps).
func processPeak(in Input, m map[string]any, kind string, codes []string, volume float64) (float64, bool) {
	if kind != WorkPallet && kind != WorkPiece {
		return 0, false
	}
	own := defaultTaskCodes(kind)
	day := 0.0
	for _, p := range in.Processes {
		if p.IsBaseline && processCovered(p, own) && processCovered(p, codes) {
			day += p.UnitsPerDay
		}
	}
	if day <= 0 {
		return 0, false
	}
	return dayPeak(in.ObjectType, m, day, volume), true
}

func throughput(kind string, r Robot, m map[string]any) (float64, string) {
	speed := 0.0
	if r.SpeedMps != nil && *r.SpeedMps > 0 {
		speed = *r.SpeedMps
	}
	switch kind {
	case WorkPallet:
		if speed > 0 {
			path := 2 * math.Sqrt(math.Max(num(m, "area_active_m2"), 1))
			cycle := path/speed + HandlePalletS
			if cycle > 0 {
				return 3600 / cycle, ""
			}
		}
		return DefaultPalletOpsH, "Нет скорости в ТТХ, производительность паллетного AMR принята 12 операций/ч."
	case WorkPiece:
		if speed > 0 {
			path := 2 * num(m, "pick_path_m_per_line")
			if path <= 0 {
				path = 50
			}
			cycle := path/speed + HandlePieceS
			if cycle > 0 {
				return 3600 / cycle, ""
			}
		}
		return DefaultPieceOpsH, "Нет скорости в ТТХ, производительность штучного AMR принята 80 строк/ч."
	case WorkCleaner, WorkAirportCleaner, WorkHospitalCleaner:
		width := 0.6
		if r.WidthMm != nil && *r.WidthMm > 0 {
			width = *r.WidthMm / 1000
		}
		if speed > 0 {
			return width * speed * 3600 * CleanerOverlap, ""
		}
		return DefaultCleanerM2H, "Нет скорости в ТТХ, производительность уборки принята 800 м²/ч."
	case WorkHospitalCart:
		if speed > 0 {
			path := 2 * num(m, "kitchen_to_ward_m")
			if path <= 0 {
				path = 360
			}
			cycle := path/speed + HandleHospitalS
			if cycle > 0 {
				return 3600 / cycle, ""
			}
		}
		return DefaultHospitalTripsH, "Нет скорости в ТТХ, производительность внутрибольничной доставки принята 8 рейсов/ч."
	case WorkAirportRamp:
		if speed > 0 {
			path := 2 * math.Sqrt(math.Max(num(m, "apron_area_m2"), 1))
			cycle := path/speed + HandleRampS
			if cycle > 0 {
				return 3600 / cycle, ""
			}
		}
		return DefaultAirportRampOpsH, "Нет скорости в ТТХ, производительность перронного тягача принята 12 операций/ч."
	case WorkAirportTrolley:
		if speed > 0 {
			path := 2 * math.Sqrt(math.Max(num(m, "terminal_area_m2"), 1))
			cycle := path/speed + HandleTrolleyS
			if cycle > 0 {
				return 3600 / cycle, ""
			}
		}
		return DefaultAirportTrolleyOpsH, "Нет скорости в ТТХ, производительность тележки терминала принята 10 рейсов/ч."
	default:
		return DefaultPalletOpsH, "Неизвестный класс процесса, производительность принята 12 операций/ч."
	}
}

type laborPool struct {
	Headcount float64
	AnnualFot float64
	Label     string
	Assumed   bool
}

// yearFot is the yearly payroll of head people with the contributions factor tax. Pay above PayrollCapRub a year
// per person carries the reduced contribution rate instead of the full one; the injury rate inside tax stays.
func yearFot(head, wage, tax, laborFactor float64) float64 {
	if head <= 0 || wage <= 0 {
		return 0
	}
	perPerson := wage * 12 * laborFactor
	relief := math.Min(PayrollFullRate-PayrollReducedRate, math.Max(tax-1, 0))
	above := math.Max(perPerson-PayrollCapRub, 0)
	return roundRub(head * (perPerson*tax - above*relief))
}

func laborFor(objectType, kind string, m map[string]any, laborFactor float64) laborPool {
	tax := taxFactor(m)
	if laborFactor <= 0 {
		laborFactor = 1
	}
	switch kind {
	case WorkPallet:
		h := num(m, "staff_forklift")
		return laborPool{h, yearFot(h, num(m, "wage_forklift_month_rub"), tax, laborFactor), "операторы погрузчиков", false}
	case WorkPiece:
		h := num(m, "staff_pickers")
		return laborPool{h, yearFot(h, num(m, "wage_picker_month_rub"), tax, laborFactor), "отборщики", false}
	case WorkCleaner:
		h := AssumedCleanerHead
		return laborPool{h, yearFot(h, num(m, "wage_picker_month_rub"), tax, laborFactor), "уборка склада (оценка 8 чел)", true}
	case WorkAirportRamp:
		h := num(m, "staff_ramp")
		return laborPool{h, yearFot(h, num(m, "wage_ramp_month_rub"), tax, laborFactor), "персонал рампа", false}
	case WorkAirportTrolley:
		h := num(m, "staff_terminal")
		return laborPool{h, yearFot(h, num(m, "wage_cleaner_month_rub"), tax, laborFactor), "персонал терминала", false}
	case WorkAirportCleaner:
		h := num(m, "cleaning_machines")
		assumed := false
		if h <= 0 {
			h = AssumedCleanerHead
			assumed = true
		}
		return laborPool{h, yearFot(h, num(m, "wage_cleaner_month_rub"), tax, laborFactor), "уборка терминала", assumed}
	case WorkHospitalCart:
		hp := num(m, "staff_porters")
		hk := num(m, "staff_kitchen")
		hl := num(m, "staff_laundry")
		fot := yearFot(hp, num(m, "wage_porter_month_rub"), tax, laborFactor) +
			yearFot(hk, num(m, "wage_kitchen_month_rub"), tax, laborFactor) +
			yearFot(hl, num(m, "wage_porter_month_rub"), tax, laborFactor)
		return laborPool{hp + hk + hl, fot, "санитары, пищеблок, прачечная", false}
	case WorkHospitalCleaner:
		h := AssumedCleanerHead
		return laborPool{h, yearFot(h, num(m, "wage_porter_month_rub"), tax, laborFactor), "уборка (оценка 8 чел)", true}
	default:
		return laborPool{}
	}
}

func remainingHead(head float64) float64 {
	if head <= 0 {
		return 0
	}
	r := math.Ceil(RemainFrac * head)
	if r < RemainMin {
		r = RemainMin
	}
	if r > head {
		r = head
	}
	return r
}

func remainLabor(head, annualFot, laborCashShare float64) (remainHead, remainFot, cashSaved, releasedFot float64) {
	remainHead = remainingHead(head)
	if head <= 0 || annualFot <= 0 {
		return remainHead, 0, 0, 0
	}
	remainFotFull := annualFot * remainHead / head
	releasedFot = annualFot - remainFotFull
	if laborCashShare < 0 {
		laborCashShare = 0
	}
	if laborCashShare > 1 {
		laborCashShare = 1
	}
	cashSaved = roundRub(releasedFot * laborCashShare)
	remainFot = roundRub(annualFot - cashSaved)
	return remainHead, remainFot, cashSaved, roundRub(releasedFot)
}

func wageForRole(objectType, role string, m map[string]any) float64 {
	r := fold(role)
	switch objectType {
	case "airport":
		if containsAny(r, []string{"ramp", "перрон", "тягач"}) {
			return num(m, "wage_ramp_month_rub")
		}
		return num(m, "wage_cleaner_month_rub")
	case "hospital":
		if containsAny(r, []string{"kitchen", "пищ", "кухн"}) {
			return num(m, "wage_kitchen_month_rub")
		}
		return num(m, "wage_porter_month_rub")
	default:
		if containsAny(r, []string{"picker", "отбор", "комплект"}) {
			return num(m, "wage_picker_month_rub")
		}
		return num(m, "wage_forklift_month_rub")
	}
}

func laborFromProcesses(objectType string, processes []ProcessSpec, m map[string]any, laborFactor float64) laborPool {
	tax := taxFactor(m)
	if laborFactor <= 0 {
		laborFactor = 1
	}
	head := 0.0
	fot := 0.0
	labels := make([]string, 0, len(processes))
	for _, p := range processes {
		if !p.IsBaseline {
			continue
		}
		head += p.StaffHeadcount
		wage := wageForRole(objectType, p.StaffRole, m)
		fot += yearFot(p.StaffHeadcount, wage, tax, laborFactor)
		if p.Name != "" {
			labels = append(labels, p.Name)
		}
	}
	label := "базовые процессы"
	if len(labels) > 0 {
		label = strings.Join(labels, ", ")
	}
	return laborPool{Headcount: head, AnnualFot: roundRub(fot), Label: label, Assumed: false}
}

func defaultTaskCodes(kind string) []string {
	switch kind {
	case WorkPallet:
		return []string{"inbound", "putaway", "outbound", "pallet_inbound", "pallet_putaway", "pallet_outbound", "pallet_move"}
	case WorkPiece:
		return []string{"piece_pick"}
	case WorkCleaner, WorkAirportCleaner, WorkHospitalCleaner:
		return []string{"cleaning"}
	default:
		return nil
	}
}

func processCovered(p ProcessSpec, codes []string) bool {
	if len(codes) == 0 {
		return true
	}
	for _, c := range codes {
		if c == p.Code || c == p.TaskType {
			return true
		}
	}
	return false
}

func workLabel(kind string) string {
	switch kind {
	case WorkPallet:
		return "паллетная перевозка (приёмка и отгрузка)"
	case WorkPiece:
		return "штучный отбор (строки комплектации)"
	case WorkCleaner:
		return "уборка активной зоны склада"
	case WorkAirportRamp:
		return "перрон, пиковые рейсы"
	case WorkAirportTrolley:
		return "внутритерминальные рейсы тележек"
	case WorkAirportCleaner:
		return "уборка терминала"
	case WorkHospitalCart:
		return "внутрибольничные рейсы (питание, бельё, медикаменты, отходы, анализы)"
	case WorkHospitalCleaner:
		return "уборка медучреждения"
	default:
		return kind
	}
}
