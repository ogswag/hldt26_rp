package catalog

import "encoding/json"

// FieldSource says where one value of a robot comes from. A value with no entry falls back to the source of the card.
type FieldSource struct {
	SourceURL  string `json:"source_url,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Note       string `json:"note,omitempty"`
	SourcedAt  string `json:"sourced_at,omitempty"`
}

// ParseFieldSources reads a solution's field_sources. A payload it cannot read gives an empty map.
func ParseFieldSources(raw json.RawMessage) map[string]FieldSource {
	out := map[string]FieldSource{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	if out == nil {
		out = map[string]FieldSource{}
	}
	return out
}
