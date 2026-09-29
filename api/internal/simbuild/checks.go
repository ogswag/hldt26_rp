package simbuild

import (
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
)

// RunBrief is what the check of a variant needs of a finished simulation run.
type RunBrief struct {
	RunID       string     `json:"run_id"`
	VariantID   string     `json:"variant_id"`
	VariantHash string     `json:"variant_hash"`
	EconCheck   *EconCheck `json:"econ_check,omitempty"`
}

const noDemandText = "Спрос процессов не задан, сверка с экономикой невозможна."

// Checks compares the simulation of each variant that has a fleet with what the economics counts on. runs go newest
// first. A variant is checked by its latest run made for the current inputs of that variant; when only older runs
// exist the check is stale, and a variant nobody simulated gets a check without a run.
func Checks(d projects.Draft, runs []RunBrief) ([]econ.SimCheck, error) {
	var out []econ.SimCheck
	for _, v := range d.Variants {
		if !hasFleetLine(v) {
			continue
		}
		hash, err := VariantHash(d, v.ID)
		if err != nil {
			return nil, err
		}
		var fresh, latest *RunBrief
		for i := range runs {
			r := &runs[i]
			if r.VariantID != v.ID {
				continue
			}
			if latest == nil {
				latest = r
			}
			if r.VariantHash == hash {
				fresh = r
				break
			}
		}
		check := econ.SimCheck{VariantID: v.ID, VariantName: v.Name, Model: econ.SimCheckMap}
		pick := fresh
		if pick == nil {
			pick = latest
			check.Stale = latest != nil
		}
		if pick == nil {
			out = append(out, check)
			continue
		}
		check.RunID = pick.RunID
		if ec := pick.EconCheck; ec != nil {
			check.Value, check.Flag, check.Text = ec.Coverage, ec.Flag, ec.Text
		} else {
			check.Text = noDemandText
		}
		out = append(out, check)
	}
	return out, nil
}

// LocalChecks is Checks for a project that lives in the browser: its records and the runs kept there, newest first.
func LocalChecks(state ops.State, runs []RunBrief) ([]econ.SimCheck, error) {
	d, err := DraftOf(state)
	if err != nil {
		return nil, err
	}
	return Checks(d, runs)
}

// LocalVariantHashes names the inputs of every variant of a project in the browser, so a page can tell which of its
// kept runs are stale.
func LocalVariantHashes(state ops.State) (map[string]string, error) {
	d, err := DraftOf(state)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(d.Variants))
	for _, v := range d.Variants {
		if out[v.ID], err = VariantHash(d, v.ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func hasFleetLine(v projects.Variant) bool {
	for _, f := range v.Fleet {
		if f.SolutionID != nil && *f.SolutionID != "" && f.Quantity > 0 {
			return true
		}
	}
	return false
}
