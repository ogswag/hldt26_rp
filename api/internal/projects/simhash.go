package projects

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"moscow_hackathon_2026/api/internal/objects"
)

type simFleetLine struct {
	SolutionID string   `json:"solution_id"`
	Quantity   int      `json:"quantity"`
	TaskCodes  []string `json:"task_codes"`
}

type simHashPayload struct {
	Fleet      []simFleetLine  `json:"fleet"`
	Map        json.RawMessage `json:"map"`
	ObjectType string          `json:"object_type"`
	Params     json.RawMessage `json:"params"`
	Processes  []Process       `json:"processes"`
	SimVersion string          `json:"sim_version"`
}

// SimInputHash is the SHA-256 of what a simulation of one variant reads: the object parameters, the processes, the
// map and the fleet of that variant, plus the version of the simulation model. Costs, financing and the other
// variants stay out, so editing them does not make the run stale. A variant the draft does not have hashes with an
// empty fleet.
func SimInputHash(d Draft, variantID, simVersion string) (string, error) {
	normalized, err := cloneDraft(d)
	if err != nil {
		return "", fmt.Errorf("projects.sim_hash.clone: %w", err)
	}
	// NOTE: defaults are filled first, as a simulation run does, so a draft with gaps and the same draft with its
	// defaults written out hash alike.
	params, err := objects.FillDefaults(normalized.ObjectType, normalized.Params)
	if err != nil {
		return "", fmt.Errorf("projects.sim_hash.params: %w", err)
	}
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
	fleet := []simFleetLine{}
	for _, v := range normalized.Variants {
		if v.ID != variantID {
			continue
		}
		for _, f := range v.Fleet {
			codes := append([]string{}, f.TaskCodes...)
			sort.Strings(codes)
			fleet = append(fleet, simFleetLine{SolutionID: stringValue(f.SolutionID), Quantity: f.Quantity, TaskCodes: codes})
		}
	}
	sort.SliceStable(fleet, func(i, j int) bool {
		if fleet[i].SolutionID != fleet[j].SolutionID {
			return fleet[i].SolutionID < fleet[j].SolutionID
		}
		return fmt.Sprint(fleet[i].TaskCodes) < fmt.Sprint(fleet[j].TaskCodes)
	})
	mp := normalized.Map
	if len(bytes.TrimSpace(mp)) == 0 || bytes.Equal(bytes.TrimSpace(mp), []byte("null")) {
		mp = json.RawMessage(`null`)
	}
	canon, err := CanonicalJSON(simHashPayload{
		Fleet:      fleet,
		Map:        mp,
		ObjectType: normalized.ObjectType,
		Params:     params,
		Processes:  procs,
		SimVersion: simVersion,
	})
	if err != nil {
		return "", fmt.Errorf("projects.sim_hash: %w", err)
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}
