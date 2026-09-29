package httpserver

import (
	"encoding/json"
	"testing"

	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
)

func TestObjectValidatorsCoverSchema(t *testing.T) {
	for name, c := range ops.ProjectSchema().Collections {
		for fn, f := range c.Fields {
			if f.Type == ops.TypeObject && objectValidators[f.Validator] == nil {
				t.Errorf("%s.%s: no server validator %q", name, fn, f.Validator)
			}
		}
	}
}

// Stored defaults must pass the validators, or the first edit of a fresh project would be rejected.
func TestDefaultsPassObjectValidators(t *testing.T) {
	h := &projectHooks{}
	for _, objectType := range []string{"warehouse", "airport", "hospital"} {
		for _, p := range projects.DefaultProcesses(objectType) {
			for validator, v := range map[string]any{
				"process_demand": p.Demand, "process_sla": p.SLA, "process_durations": p.Durations, "process_staff": p.BaselineStaff,
			} {
				raw, _ := json.Marshal(v)
				var decoded any
				_ = json.Unmarshal(raw, &decoded)
				if !h.Object(validator, decoded) {
					t.Errorf("%s %s: %s rejects %s", objectType, p.Code, validator, raw)
				}
			}
		}
	}
	for validator, raw := range map[string]string{
		"financing_assumptions": `{}`,
		"econ_overrides":        `{"price_rub":null,"volume_factor":1.2,"labor_factor":null,"fleet_size":2,"capex_rub":null,"opex_year_rub":null}`,
		"solution_ids":          `["5760e938-9a43-45a7-b8e8-f4f2e6383930"]`,
		"tab_ids":               `["site","volumes"]`,
		"map_page":              `{"width_px":1000,"height_px":700,"source_kind":"none"}`,
		"map_segment":           `{"x1":0,"y1":0,"x2":100,"y2":0,"length_m":10}`,
	} {
		var v any
		_ = json.Unmarshal([]byte(raw), &v)
		if !h.Object(validator, v) {
			t.Errorf("%s rejects %s", validator, raw)
		}
	}
	for _, c := range [][2]string{
		{"process_sla", `{"max_wait_min":30,"max_cycle_min":10}`},
		{"process_demand", `{"units_per_day":-1}`},
		{"econ_overrides", `{"fleet_size":0}`},
		{"solution_ids", `["not-a-uuid"]`},
		{"tab_ids", `["Site"]`},
		{"tab_ids", `[1]`},
		{"map_page", `{"width_px":1,"height_px":1,"source_kind":"gif"}`},
		{"map_segment", `{"x1":0,"y1":0,"x2":1,"y2":0,"length_m":0}`},
		// Not the stored form: a zero value the struct omits, or a key the struct always writes.
		{"process_staff", `{"headcount":2,"role":""}`},
		{"map_page", `{"width_px":1000,"height_px":700}`},
	} {
		validator, raw := c[0], c[1]
		var v any
		_ = json.Unmarshal([]byte(raw), &v)
		if h.Object(validator, v) {
			t.Errorf("%s accepts %s", validator, raw)
		}
	}
}
