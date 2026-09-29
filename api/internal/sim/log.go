package sim

import (
	"math"

	"moscow_hackathon_2026/api/internal/maps"
)

// Event is one row of the compact journal (sim-event-v1).
type Event struct {
	Seq         int      `json:"seq"`
	T           float64  `json:"t_s"`
	Type        string   `json:"type"`
	JobID       string   `json:"job_id,omitempty"`
	RobotID     string   `json:"robot_id,omitempty"`
	ProcessCode string   `json:"process_code,omitempty"`
	ResourceID  string   `json:"resource_id,omitempty"`
	FromID      string   `json:"from_id,omitempty"`
	ToID        string   `json:"to_id,omitempty"`
	Priority    *int     `json:"priority,omitempty"`
	Payload     *Payload `json:"payload,omitempty"`
}

type Payload struct {
	Path    []string `json:"path,omitempty"`
	DistM   *float64 `json:"dist_m,omitempty"`
	DurS    *float64 `json:"dur_s,omitempty"`
	Loaded  bool     `json:"loaded,omitempty"`
	Battery *float64 `json:"battery,omitempty"`
	WaitS   *float64 `json:"wait_s,omitempty"`
	CycleS  *float64 `json:"cycle_s,omitempty"`
	Kind    string   `json:"kind,omitempty"`
	Queue   *int     `json:"queue,omitempty"`
}

type ResourceLoad struct {
	ID    string `json:"id"`
	InUse int    `json:"in_use"`
	Queue int    `json:"queue"`
}

// Checkpoint is a system state sample for the timeline.
type Checkpoint struct {
	T         float64        `json:"t_s"`
	Queue     int            `json:"queue"`
	Busy      int            `json:"busy"`
	Idle      int            `json:"idle"`
	Charging  int            `json:"charging"`
	Completed int            `json:"completed"`
	Violated  int            `json:"violated"`
	Processes []int          `json:"processes"`
	Resources []ResourceLoad `json:"resources"`
}

type LogRobot struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Profile  string  `json:"profile"`
	WidthM   float64 `json:"width_m"`
	LengthM  float64 `json:"length_m"`
	FleetKey string  `json:"fleet_key"`
	StartID  string  `json:"start_id"`
	Battery  float64 `json:"battery"`
}

type LogResource struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Capacity int      `json:"capacity"`
	Auto     bool     `json:"auto,omitempty"`
	NodeIDs  []string `json:"node_ids,omitempty"`
	EdgeIDs  []string `json:"edge_ids,omitempty"`
}

type LogProcess struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Covered  bool   `json:"covered"`
	Priority int    `json:"priority"`
}

// Log is the replay artifact of one replication.
type Log struct {
	SchemaVersion string           `json:"schema_version"`
	EventSchema   string           `json:"event_schema"`
	SimVersion    string           `json:"sim_version"`
	Replication   int              `json:"replication"`
	Seed          int64            `json:"seed"`
	HorizonS      float64          `json:"horizon_s"`
	Scene         maps.Scene       `json:"scene"`
	Nodes         []maps.Node      `json:"nodes"`
	Edges         []maps.GraphEdge `json:"edges"`
	Resources     []LogResource    `json:"resources"`
	Robots        []LogRobot       `json:"robots"`
	Processes     []LogProcess     `json:"processes"`
	Events        []Event          `json:"events"`
	Checkpoints   []Checkpoint     `json:"checkpoints"`
	Truncated     bool             `json:"truncated"`
}

type recorder struct {
	events      []Event
	checkpoints []Checkpoint
	truncated   bool
}

func (r *runner) emit(e Event) {
	if r.rec == nil {
		return
	}
	if len(r.rec.events) >= maxLogEvents {
		r.rec.truncated = true
		return
	}
	e.Seq = len(r.rec.events)
	e.T = f1v(r.now)
	r.rec.events = append(r.rec.events, e)
}

func (r *runner) buildLog(rep int, seed int64) *Log {
	m := r.m
	l := &Log{
		SchemaVersion: LogSchemaVersion,
		EventSchema:   EventSchemaVersion,
		SimVersion:    Version,
		Replication:   rep,
		Seed:          seed,
		HorizonS:      m.horizonS,
		Scene:         m.sc.Scene,
		Nodes:         m.sc.Graph.Nodes,
		Edges:         m.sc.Graph.Edges,
		Events:        r.rec.events,
		Checkpoints:   r.rec.checkpoints,
		Truncated:     r.rec.truncated,
	}
	for _, rm := range m.res {
		l.Resources = append(l.Resources, LogResource{
			ID: rm.id, Kind: rm.kind, Name: rm.name, Capacity: rm.capacity, Auto: rm.auto, NodeIDs: rm.nodes, EdgeIDs: rm.edges,
		})
	}
	for i, spec := range m.robots {
		item := m.sc.Fleet[spec.fleet]
		l.Robots = append(l.Robots, LogRobot{
			ID:       spec.id,
			Name:     spec.name,
			Profile:  item.Profile.Code,
			WidthM:   item.Profile.WidthM,
			LengthM:  item.Profile.LengthM,
			FleetKey: item.Key,
			StartID:  r.nodeID(spec.start),
			Battery:  round3(r.robots[i].startBattery),
		})
	}
	for _, p := range m.procs {
		l.Processes = append(l.Processes, LogProcess{Code: p.spec.Code, Name: p.spec.Name, Covered: p.covered, Priority: p.spec.Priority})
	}
	if l.Events == nil {
		l.Events = []Event{}
	}
	return l
}

func f1v(v float64) float64 {
	return math.Round(v*10) / 10
}

func f1(v float64) *float64 {
	x := f1v(v)
	return &x
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

func f3(v float64) *float64 {
	x := round3(v)
	return &x
}

func intPtr(v int) *int {
	return &v
}
