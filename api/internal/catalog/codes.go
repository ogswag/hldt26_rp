package catalog

import "strings"

const (
	TaskPalletInbound  = "pallet_inbound"
	TaskPalletPutaway  = "pallet_putaway"
	TaskPalletOutbound = "pallet_outbound"
	TaskPalletMove     = "pallet_move"
	TaskPiecePick      = "piece_pick"
	TaskCleaning       = "cleaning"

	CapWarehouseIndoor = "warehouse_indoor"
	CapPayloadPallet   = "payload_pallet"
	CapPayloadUnit     = "payload_unit"
	CapAisleRated      = "aisle_rated"
	CapTempRated       = "temp_rated"
	CapCleaning        = "cleaning"
	CapAMR             = "amr"
	CapStacker         = "stacker"
	CapForklift        = "forklift"

	PalletPayloadMinKg = 400
)

type Entry struct {
	Kind       string `json:"kind"`
	Code       string `json:"code"`
	Label      string `json:"label"`
	ObjectType string `json:"object_type,omitempty"`
}

type TaskRule struct {
	CapabilityCode string `json:"capability_code"`
	TaskCode       string `json:"task_code"`
	Relation       string `json:"relation"`
}

func Tasks() []Entry {
	return []Entry{
		{Kind: "task", Code: TaskPalletInbound, Label: "Приёмка паллет", ObjectType: "warehouse"},
		{Kind: "task", Code: TaskPalletPutaway, Label: "Размещение паллет", ObjectType: "warehouse"},
		{Kind: "task", Code: TaskPalletOutbound, Label: "Отгрузка паллет", ObjectType: "warehouse"},
		{Kind: "task", Code: TaskPalletMove, Label: "Паллетное перемещение", ObjectType: "warehouse"},
		{Kind: "task", Code: TaskPiecePick, Label: "Мелкоштучный отбор", ObjectType: "warehouse"},
		{Kind: "task", Code: TaskCleaning, Label: "Уборка", ObjectType: "warehouse"},
	}
}

func Capabilities() []Entry {
	return []Entry{
		{Kind: "capability", Code: CapWarehouseIndoor, Label: "Наземная работа на складе", ObjectType: "warehouse"},
		{Kind: "capability", Code: CapPayloadPallet, Label: "Паллетная нагрузка", ObjectType: "warehouse"},
		{Kind: "capability", Code: CapPayloadUnit, Label: "Штучная или контейнерная нагрузка", ObjectType: "warehouse"},
		{Kind: "capability", Code: CapAisleRated, Label: "Есть ширина или минимальный проезд", ObjectType: "warehouse"},
		{Kind: "capability", Code: CapTempRated, Label: "Есть рабочий диапазон температур", ObjectType: "warehouse"},
		{Kind: "capability", Code: CapCleaning, Label: "Уборка помещений", ObjectType: "warehouse"},
		{Kind: "capability", Code: CapAMR, Label: "Автономный мобильный робот", ObjectType: "warehouse"},
		{Kind: "capability", Code: CapStacker, Label: "Штабелёр", ObjectType: "warehouse"},
		{Kind: "capability", Code: CapForklift, Label: "Погрузчик", ObjectType: "warehouse"},
	}
}

// CodeLabel is the Russian name of a task or capability code, or the code itself when it has none.
func CodeLabel(code string) string {
	for _, list := range [][]Entry{Tasks(), Capabilities()} {
		for _, e := range list {
			if e.Code == code {
				return e.Label
			}
		}
	}
	return code
}

func TaskRules() []TaskRule {
	return []TaskRule{
		{CapabilityCode: CapPayloadPallet, TaskCode: TaskPalletInbound, Relation: "required"},
		{CapabilityCode: CapPayloadPallet, TaskCode: TaskPalletPutaway, Relation: "required"},
		{CapabilityCode: CapPayloadPallet, TaskCode: TaskPalletOutbound, Relation: "required"},
		{CapabilityCode: CapPayloadPallet, TaskCode: TaskPalletMove, Relation: "required"},
		{CapabilityCode: CapPayloadUnit, TaskCode: TaskPiecePick, Relation: "required"},
		{CapabilityCode: CapCleaning, TaskCode: TaskCleaning, Relation: "required"},
	}
}

func RequiredCapability(taskCode string) string {
	for _, r := range TaskRules() {
		if r.TaskCode == taskCode && r.Relation == "required" {
			return r.CapabilityCode
		}
	}
	return ""
}

func ValidTask(code string) bool {
	for _, e := range Tasks() {
		if e.Code == code {
			return true
		}
	}
	return false
}

func HasCapability(codes []string, want string) bool {
	if want == "" {
		return true
	}
	for _, c := range codes {
		if c == want {
			return true
		}
	}
	if want == CapPayloadUnit {
		for _, c := range codes {
			if c == CapPayloadPallet {
				return true
			}
		}
	}
	return false
}

func IsCleaner(name, subtype, family, scenario string) bool {
	t := fold(name + " " + subtype + " " + family + " " + scenario)
	return containsAny(t, []string{"уборщик", "уборка помещений", "поломо", "клинботикс", "mark 2"})
}

func fold(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	return s
}

func containsAny(text string, needles []string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(text, n) {
			return true
		}
	}
	return false
}
