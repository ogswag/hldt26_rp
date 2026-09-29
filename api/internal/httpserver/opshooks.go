package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
)

// NOTE: tab ids come from object schemas ("site", "volumes"); the cap is well above any schema's tab count.
const tabsCap = 20

var tabID = regexp.MustCompile(`^[a-z_]{1,32}$`)

// projectHooks checks what the operation schema leaves to the server: params of the object type, named objects
// and catalog solutions.
type projectHooks struct {
	ctx       context.Context
	q         *db.Queries
	solutions map[string]bool
	err       error
}

func (h *projectHooks) Params(rec ops.Record, field string, keys []string) (string, bool) {
	raw, err := json.Marshal(rec[field])
	if err != nil {
		return "", false
	}
	return objects.CheckKeys(rec.String(ops.FieldObjectType), raw, keys)
}

func (h *projectHooks) Object(validator string, value any) bool {
	check, ok := objectValidators[validator]
	if !ok {
		return false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return check(raw)
}

// External loads the catalog's solution ids on first use. A load failure rejects the reference and is kept in err,
// so the caller answers 500 instead of trusting the rejection.
func (h *projectHooks) External(to, id string) bool {
	if to != "catalog_solutions" {
		return false
	}
	if h.solutions == nil && h.err == nil {
		ids, err := h.q.ListSolutionIDs(h.ctx)
		if err != nil {
			h.err = fmt.Errorf("ops.hooks.catalog: %w", err)
			return false
		}
		h.solutions = make(map[string]bool, len(ids))
		for _, sid := range ids {
			h.solutions[sid.String()] = true
		}
	}
	return h.solutions[id]
}

// strictDecode fills dst from a JSON object and refuses unknown keys and wrong types.
func strictDecode(raw []byte, dst any) bool {
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst) == nil
}

// storedForm accepts raw only in the form the tables keep it: the struct's own JSON, with omitted zero values
// absent and every other key present. Any other spelling would differ from the next snapshot.
func storedForm(raw []byte, dst any) bool {
	if !strictDecode(raw, dst) {
		return false
	}
	back, err := json.Marshal(dst)
	if err != nil {
		return false
	}
	var a, b any
	if json.Unmarshal(raw, &a) != nil || json.Unmarshal(back, &b) != nil {
		return false
	}
	return ops.Equal(a, b)
}

func finiteAll(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func nonNegative(vs ...float64) bool {
	for _, v := range vs {
		if v < 0 {
			return false
		}
	}
	return finiteAll(vs...)
}

// objectValidators mirror projects.ValidateProcess, the map document and econ.Overrides for single object fields.
// Fields stored through a Go struct must arrive in the stored form (storedForm).
var objectValidators = map[string]func(raw []byte) bool{
	"process_demand": func(raw []byte) bool {
		var d projects.Demand
		return storedForm(raw, &d) && nonNegative(d.UnitsPerDay, d.UnitsPerJob) &&
			d.UnitsPerDay <= projects.MaxUnitsPerDay && d.UnitsPerJob <= projects.MaxUnitsPerJob
	},
	"process_sla": func(raw []byte) bool {
		var s projects.SLA
		if !storedForm(raw, &s) || !nonNegative(s.MaxWaitMin, s.MaxCycleMin) {
			return false
		}
		if s.MaxWaitMin > 0 && s.MaxCycleMin > 0 && s.MaxWaitMin > s.MaxCycleMin {
			return false
		}
		return s.Priority >= 0 && s.Priority <= projects.MaxPriority
	},
	"process_durations": func(raw []byte) bool {
		var d projects.Durations
		return storedForm(raw, &d) && nonNegative(d.LoadS, d.UnloadS, d.TravelS)
	},
	"process_staff": func(raw []byte) bool {
		var s projects.Staff
		return storedForm(raw, &s) && nonNegative(s.Headcount)
	},
	"financing_assumptions": func(raw []byte) bool {
		var m map[string]any
		return bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) && json.Unmarshal(raw, &m) == nil
	},
	"econ_overrides": func(raw []byte) bool {
		var o econ.Overrides
		if !strictDecode(raw, &o) {
			return false
		}
		for _, p := range []*float64{o.PriceRub, o.VolumeFactor, o.LaborFactor, o.CapexRub, o.OpexYearRub} {
			if p != nil && !nonNegative(*p) {
				return false
			}
		}
		return o.FleetSize == nil || *o.FleetSize >= 1
	},
	"solution_ids": func(raw []byte) bool {
		var ids []string
		if json.Unmarshal(raw, &ids) != nil || len(ids) > solutionsCap {
			return false
		}
		for _, id := range ids {
			if _, err := uuid.Parse(id); err != nil {
				return false
			}
		}
		return true
	},
	"tab_ids": func(raw []byte) bool {
		var ids []string
		if json.Unmarshal(raw, &ids) != nil || len(ids) > tabsCap {
			return false
		}
		for _, id := range ids {
			if !tabID.MatchString(id) {
				return false
			}
		}
		return true
	},
	"map_page": func(raw []byte) bool {
		var p maps.Page
		if !storedForm(raw, &p) || !nonNegative(p.WidthPx, p.HeightPx) {
			return false
		}
		switch p.SourceKind {
		case "png", "jpeg", "pdf", "none":
			return true
		}
		return false
	},
	"map_segment": func(raw []byte) bool {
		var s maps.Segment
		return storedForm(raw, &s) && finiteAll(s.X1, s.Y1, s.X2, s.Y2) && s.LengthM > 0 && finiteAll(s.LengthM)
	},
}
