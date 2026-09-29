package projects

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/objects"
)

type hashPayload struct {
	AssumptionSets   []AssumptionSet `json:"assumption_sets"`
	EconOverrides    json.RawMessage `json:"econ_overrides"`
	EconVersion      string          `json:"econ_version"`
	Map              json.RawMessage `json:"map"`
	MatchSelectedIDs []string        `json:"match_selected_ids"`
	MatchVersion     string          `json:"match_version"`
	ObjectType       string          `json:"object_type"`
	Params           json.RawMessage `json:"params"`
	Processes        []Process       `json:"processes"`
	SharedCosts      []SharedCost    `json:"shared_costs"`
	SimVersion       string          `json:"sim_version"`
	Variants         []Variant       `json:"variants"`
}

// InputHash is the SHA-256 of the draft's canonical inputs and the model versions of this build.
// NOTE: a change to what the hash covers needs a migration that clears calculation_runs.draft_hash.
func InputHash(d Draft) (string, error) {
	normalized, err := cloneDraft(d)
	if err != nil {
		return "", fmt.Errorf("projects.hash.clone: %w", err)
	}
	// NOTE: params are hashed as the schema reads them, so a migration that only rewrites a format keeps the hash.
	params, err := objects.NormalizeParams(normalized.ObjectType, normalized.Params)
	if err != nil {
		return "", fmt.Errorf("projects.hash.params: %w", err)
	}
	selected := append([]string(nil), normalized.MatchSelectedIDs...)
	sort.Strings(selected)
	procs := normalized.Processes
	sort.SliceStable(procs, func(i, j int) bool {
		if procs[i].SortOrder != procs[j].SortOrder {
			return procs[i].SortOrder < procs[j].SortOrder
		}
		return procs[i].Code < procs[j].Code
	})
	for i := range procs {
		procs[i].ID = ""
		procs[i].Order = ""
		sort.Strings(procs[i].PointIDs)
	}
	vars := normalized.Variants
	sort.SliceStable(vars, func(i, j int) bool {
		if vars[i].SortOrder != vars[j].SortOrder {
			return vars[i].SortOrder < vars[j].SortOrder
		}
		return vars[i].Name < vars[j].Name
	})
	for i := range vars {
		vars[i].ID = ""
		vars[i].Order = ""
		for j := range vars[i].Fleet {
			vars[i].Fleet[j].ID = ""
			vars[i].Fleet[j].Order = ""
			sort.Strings(vars[i].Fleet[j].TaskCodes)
		}
		sort.SliceStable(vars[i].Fleet, func(a, b int) bool {
			if vars[i].Fleet[a].SortOrder != vars[i].Fleet[b].SortOrder {
				return vars[i].Fleet[a].SortOrder < vars[i].Fleet[b].SortOrder
			}
			return stringValue(vars[i].Fleet[a].SolutionID) < stringValue(vars[i].Fleet[b].SolutionID)
		})
		for j := range vars[i].Financing {
			vars[i].Financing[j].ID = ""
		}
		sort.SliceStable(vars[i].Financing, func(a, b int) bool {
			if vars[i].Financing[a].Kind != vars[i].Financing[b].Kind {
				return vars[i].Financing[a].Kind < vars[i].Financing[b].Kind
			}
			return stringValue(vars[i].Financing[a].Tariff) < stringValue(vars[i].Financing[b].Tariff)
		})
	}
	ov := normalized.EconOverrides
	if len(bytes.TrimSpace(ov)) == 0 {
		ov = json.RawMessage(`{}`)
	}
	mp := normalized.Map
	if len(bytes.TrimSpace(mp)) == 0 || bytes.Equal(bytes.TrimSpace(mp), []byte("null")) {
		mp = json.RawMessage(`null`)
	}
	costs := normalized.SharedCosts
	for i := range costs {
		costs[i].ID = ""
		costs[i].Order = ""
	}
	sort.SliceStable(costs, func(i, j int) bool {
		if costs[i].SortOrder != costs[j].SortOrder {
			return costs[i].SortOrder < costs[j].SortOrder
		}
		return costs[i].Code < costs[j].Code
	})
	sets := normalized.AssumptionSets
	for i := range sets {
		sets[i].ID = ""
		sets[i].Order = ""
	}
	sort.SliceStable(sets, func(i, j int) bool {
		if sets[i].SortOrder != sets[j].SortOrder {
			return sets[i].SortOrder < sets[j].SortOrder
		}
		return sets[i].Name < sets[j].Name
	})
	payload := hashPayload{
		AssumptionSets:   sets,
		EconOverrides:    ov,
		EconVersion:      econ.ModelVersion,
		Map:              mp,
		MatchSelectedIDs: selected,
		MatchVersion:     MatchVersion,
		ObjectType:       normalized.ObjectType,
		Params:           params,
		Processes:        procs,
		SharedCosts:      costs,
		SimVersion:       SimVersion,
		Variants:         vars,
	}
	canon, err := CanonicalJSON(payload)
	if err != nil {
		return "", fmt.Errorf("projects.hash: %w", err)
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}

func cloneDraft(d Draft) (Draft, error) {
	b, err := json.Marshal(d)
	if err != nil {
		return Draft{}, err
	}
	var out Draft
	if err := json.Unmarshal(b, &out); err != nil {
		return Draft{}, err
	}
	return out, nil
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func Stale(draftHash, runHash string) bool {
	if runHash == "" {
		return false
	}
	return draftHash != runHash
}

func CanonicalJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var n any
	if err := json.Unmarshal(b, &n); err != nil {
		return nil, err
	}
	return marshalCanon(n)
}

func marshalCanon(n any) ([]byte, error) {
	switch t := n.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var buf bytes.Buffer
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			buf.Write(kb)
			buf.WriteByte(':')
			vb, err := marshalCanon(t[k])
			if err != nil {
				return nil, err
			}
			buf.Write(vb)
		}
		buf.WriteByte('}')
		return buf.Bytes(), nil
	case []any:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, el := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			vb, err := marshalCanon(el)
			if err != nil {
				return nil, err
			}
			buf.Write(vb)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	default:
		return json.Marshal(t)
	}
}
