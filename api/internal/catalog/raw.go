package catalog

import "encoding/json"

// Use is one industry and scenario the catalog row names for a solution.
type Use struct {
	Industry string `json:"industry"`
	Scenario string `json:"scenario"`
	Cases    string `json:"cases"`
}

// Raw is what the engine reads from a solution's raw import payload.
type Raw struct {
	Uses        []Use    `json:"uses"`
	SourceID    string   `json:"source_id"`
	ObjectTypes []string `json:"object_types"`
	Family      string   `json:"Тип"`
	Description string   `json:"описание"`
}

// ParseRaw reads a solution's raw payload. A payload it cannot read gives an empty Raw.
func ParseRaw(raw json.RawMessage) Raw {
	var r Raw
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &r)
	}
	return r
}
