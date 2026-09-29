package geom

import "encoding/json"

// Version names the geometric peak-window check used by econ verification.
const Version = "sim-v1"

const (
	TickS         = 1.0
	HorizonS      = 900.0
	DivergenceMax = 0.15
	MaxFleet      = 200

	HandlePalletS   = 60.0
	HandlePieceS    = 20.0
	HandleHospitalS = 120.0
	HandleRampS     = 180.0
	HandleTrolleyS  = 60.0
	CleanerOverlap  = 0.70

	WorkPallet          = "pallet"
	WorkPiece           = "piece"
	WorkCleaner         = "cleaner"
	WorkAirportRamp     = "airport_ramp"
	WorkAirportTrolley  = "airport_trolley"
	WorkAirportCleaner  = "airport_cleaner"
	WorkHospitalCart    = "hospital_cart"
	WorkHospitalCleaner = "hospital_cleaner"
)

type Input struct {
	ObjectType   string
	WorkKind     string
	FleetSize    int
	Throughput   float64
	PeakOps      float64
	Unit         string
	Seed         int64
	ScenarioKind string
	Params       json.RawMessage
	SpeedMps     float64
	WidthMm      float64
	EnduranceH   float64
	ChargeMin    float64
	LoadSlots    int
	UnloadSlots  int
}

type Point struct {
	X float64
	Y float64
}

type Zone struct {
	Kind string
	X    float64
	Y    float64
	W    float64
	H    float64
}

type Node struct {
	Kind string
	X    float64
	Y    float64
}

type Layout struct {
	Zones    []Zone
	OpPoints []Node
	Chargers []Node
}

type Summary struct {
	ModelVersion     string   `json:"model_version"`
	ObjectType       string   `json:"object_type"`
	WorkKind         string   `json:"work_kind"`
	ScenarioKind     string   `json:"scenario_kind"`
	Seed             int64    `json:"seed"`
	FleetSize        int      `json:"fleet_size"`
	EconThroughput   float64  `json:"econ_throughput"`
	Throughput       float64  `json:"throughput"`
	Divergence       float64  `json:"divergence"`
	VerificationFlag bool     `json:"verification_flag"`
	PeakOps          float64  `json:"peak_ops"`
	DeliveredOpsH    float64  `json:"delivered_ops_h"`
	Unit             string   `json:"unit"`
	QueueWaitS       float64  `json:"queue_wait_s"`
	ChargeShare      float64  `json:"charge_share"`
	SimulatedS       float64  `json:"simulated_s"`
	Bottleneck       string   `json:"bottleneck"`
	Assumptions      []string `json:"assumptions"`
}

func Run(in Input) Summary {
	if in.ScenarioKind == "" {
		in.ScenarioKind = "buy"
	}
	if in.Seed < 0 {
		in.Seed = 0
	}
	if in.FleetSize < 0 {
		in.FleetSize = 0
	}
	if in.FleetSize > MaxFleet {
		in.FleetSize = MaxFleet
	}
	if in.WorkKind == "" {
		in.WorkKind = defaultWork(in.ObjectType)
	}
	m := parseParams(in.Params)
	pathM, handleS := cycleGeom(in.ObjectType, in.WorkKind, m)
	lay := makeLayout(in)
	loadN, unloadN := slotCounts(in, lay)

	var sum Summary
	if isCleaner(in.WorkKind) {
		sum = runCleaner(in, lay, m)
	} else {
		sum = runTransport(in, lay, pathM, handleS, loadN, unloadN)
	}
	sum.ModelVersion = Version
	sum.ObjectType = in.ObjectType
	sum.WorkKind = in.WorkKind
	sum.ScenarioKind = in.ScenarioKind
	sum.Seed = in.Seed
	sum.PeakOps = round4(in.PeakOps)
	sum.Unit = in.Unit
	if sum.Unit == "" {
		sum.Unit = "1/ч"
	}
	sum.SimulatedS = HorizonS
	sum.Assumptions = append([]string{
		"Схема синтетическая, не обмер площадки.",
		"Производительность симуляции: 3600 / (движение + обработка + очередь) на робота. Зарядка в этот знаменатель не входит.",
		"Покупка и RaaS делят одну физику флота, деньги разные.",
	}, sum.Assumptions...)
	return sum
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

func isCleaner(kind string) bool {
	return kind == WorkCleaner || kind == WorkAirportCleaner || kind == WorkHospitalCleaner
}

func slotCounts(in Input, lay Layout) (int, int) {
	load, unload := 0, 0
	for _, n := range lay.OpPoints {
		if n.Kind == "load" {
			load++
		}
		if n.Kind == "unload" {
			unload++
		}
	}
	if in.LoadSlots > 0 {
		load = in.LoadSlots
	}
	if in.UnloadSlots > 0 {
		unload = in.UnloadSlots
	}
	if load < 1 {
		load = 1
	}
	if unload < 1 {
		unload = 1
	}
	return load, unload
}

func round4(v float64) float64 {
	return float64(int(v*10000+0.5)) / 10000
}
