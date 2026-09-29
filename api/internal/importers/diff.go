package importers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"moscow_hackathon_2026/api/internal/db"
)

// Row classes of an upload preview.
const (
	ClassNew       = "new"
	ClassChanged   = "changed"
	ClassConflict  = "conflict"
	ClassUnchanged = "unchanged"
	ClassError     = "error"
)

// FieldDiff is one field of a robot that the upload would change. Base is the value at the export the file came
// from, when there was one.
type FieldDiff struct {
	Code     string `json:"code"`
	Base     any    `json:"base"`
	Current  any    `json:"current"`
	File     any    `json:"file"`
	Conflict bool   `json:"conflict,omitempty"`
}

// RobotRef names a robot in a preview.
type RobotRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RowPlan is what the upload would do with one robot of the file.
type RowPlan struct {
	Key   string `json:"key"`
	Line  int    `json:"line"`
	Class string `json:"class"`
	// ID is the robot the row updates, or the id a new robot gets.
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	// Rev fingerprints the stored robot, so applying after someone else's edit fails instead of overwriting it.
	Rev         string       `json:"rev,omitempty"`
	Archived    bool         `json:"archived,omitempty"`
	DuplicateOf *RobotRef    `json:"duplicate_of,omitempty"`
	Photo       string       `json:"photo,omitempty"`
	Fields      []FieldDiff  `json:"fields"`
	Errors      []FieldError `json:"errors,omitempty"`
}

// Plan is the preview of an upload.
type Plan struct {
	ThreeWay bool           `json:"three_way"`
	Rows     []RowPlan      `json:"rows"`
	Missing  []RobotRef     `json:"missing"`
	Counts   map[string]int `json:"counts"`
}

// BuildPlan compares the file's robots with the catalog. With base, the values of the export the file was made
// from, it merges three ways: a field changed only in the file is taken, one changed only in the catalog is kept,
// one changed on both sides differently is a conflict. Without base an empty cell keeps the stored value.
func BuildPlan(file []FileRow, current []db.ListSolutionsRow, base map[string]Values, mode string, idMapped bool) Plan {
	byID := make(map[string]db.ListSolutionsRow, len(current))
	byName := map[string]RobotRef{}
	for _, row := range current {
		id := row.ID.String()
		byID[id] = row
		key := nameKey(row.Name, deref(row.Vendor))
		if _, ok := byName[key]; !ok || !row.ArchivedAt.Valid {
			byName[key] = RobotRef{ID: id, Name: row.Name}
		}
	}
	file = SplitRowIDs(file, func(id string) (string, bool) {
		row, ok := byID[id]
		return deref(row.Industry), ok
	})
	plan := Plan{ThreeWay: base != nil, Rows: make([]RowPlan, 0, len(file)), Missing: []RobotRef{}, Counts: map[string]int{}}
	inFile := map[string]bool{}
	for _, fr := range file {
		rp := planRow(fr, byID, byName, base)
		if fr.ID != "" {
			inFile[fr.ID] = true
		}
		plan.Rows = append(plan.Rows, rp)
		plan.Counts[rp.Class]++
	}
	if mode == ModeCatalog && idMapped {
		for _, row := range current {
			id := row.ID.String()
			if !row.ArchivedAt.Valid && !inFile[id] {
				plan.Missing = append(plan.Missing, RobotRef{ID: id, Name: row.Name})
			}
		}
	}
	plan.Counts["missing"] = len(plan.Missing)
	return plan
}

func planRow(fr FileRow, byID map[string]db.ListSolutionsRow, byName map[string]RobotRef, base map[string]Values) RowPlan {
	rp := RowPlan{Key: fmt.Sprintf("L%d", fr.Line), Line: fr.Line, ID: fr.ID, Photo: fr.Photo, Fields: []FieldDiff{}}
	name, _ := fr.Values["name"].(string)
	rp.Name = name
	target, exists := byID[fr.ID]
	if len(fr.Errors) > 0 {
		rp.Class, rp.Errors = ClassError, fr.Errors
		if exists && rp.Name == "" {
			rp.Name = target.Name
		}
		return rp
	}
	if !exists {
		rp.Class = ClassNew
		after := Values{}
		for _, code := range fileCodes(fr.Values) {
			if v := fr.Values[code]; v != nil {
				after[code] = v
				rp.Fields = append(rp.Fields, FieldDiff{Code: code, File: v})
			}
		}
		if fe := Check(after); fe != nil {
			rp.Class, rp.Errors = ClassError, []FieldError{*fe}
			return rp
		}
		if ref, ok := byName[nameKey(name, str(fr.Values["vendor"]))]; ok {
			rp.DuplicateOf = &ref
		}
		return rp
	}
	cur := RowValues(target)
	rp.Rev = Rev(target)
	rp.Archived = target.ArchivedAt.Valid
	if rp.Name == "" {
		rp.Name = target.Name
	}
	b, threeWay := base[fr.ID]
	after := cloneValues(cur)
	conflict := false
	for _, code := range fileCodes(fr.Values) {
		f, c := fr.Values[code], cur[code]
		if Equal(code, f, c) {
			continue
		}
		d := FieldDiff{Code: code, Current: c, File: f}
		switch {
		case threeWay && Equal(code, f, b[code]):
			continue
		case threeWay && Equal(code, c, b[code]):
			d.Base = b[code]
		case threeWay:
			d.Base, d.Conflict = b[code], true
			conflict = true
		case f == nil:
			continue
		}
		if !d.Conflict {
			setValue(after, code, f)
		}
		rp.Fields = append(rp.Fields, d)
	}
	if fe := Check(after); fe != nil {
		rp.Class, rp.Errors = ClassError, []FieldError{*fe}
		return rp
	}
	switch {
	case conflict:
		rp.Class = ClassConflict
	case len(rp.Fields) > 0:
		rp.Class = ClassChanged
	default:
		rp.Class = ClassUnchanged
	}
	return rp
}

// fileCodes lists the codes a file row carries in field order, organizer uses last.
func fileCodes(v Values) []string {
	var out []string
	for _, f := range fields {
		if _, ok := v[f.Code]; ok && f.Editable {
			out = append(out, f.Code)
		}
	}
	if _, ok := v[UsesKey]; ok {
		out = append(out, UsesKey)
	}
	return out
}

func setValue(v Values, code string, x any) {
	if x == nil {
		delete(v, code)
		return
	}
	v[code] = x
}

func cloneValues(v Values) Values {
	out := make(Values, len(v))
	for k, x := range v {
		out[k] = x
	}
	return out
}

// Rev fingerprints a stored robot's values and archive state.
func Rev(row db.ListSolutionsRow) string {
	b, _ := json.Marshal(struct {
		V Values `json:"v"`
		A bool   `json:"a"`
	}{RowValues(row), row.ArchivedAt.Valid})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// nameKey matches likely duplicates: the same name and vendor, ignoring case, ё and punctuation.
func nameKey(name, vendor string) string {
	return headerKey(name) + "|" + headerKey(vendor)
}

func str(x any) string {
	s, _ := x.(string)
	return s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// SnapshotValues is what a catalog export stores for the three-way merge: every robot's values by id.
func SnapshotValues(rows []db.ListSolutionsRow) map[string]Values {
	out := make(map[string]Values, len(rows))
	for _, row := range rows {
		out[row.ID.String()] = RowValues(row)
	}
	return out
}

// DecodeSnapshot reads a stored export snapshot back into typed values.
func DecodeSnapshot(raw []byte) (map[string]Values, error) {
	var doc map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("importers.snapshot: %w", err)
	}
	out := make(map[string]Values, len(doc))
	for id, m := range doc {
		v := Values{}
		for code, x := range m {
			if code == UsesKey {
				var uses []CatalogUse
				if json.Unmarshal(x, &uses) == nil && len(uses) > 0 {
					v[code] = uses
				}
				continue
			}
			f, ok := FieldByCode(code)
			if !ok {
				continue
			}
			switch f.Kind {
			case FieldNumber:
				var n float64
				if json.Unmarshal(x, &n) == nil {
					v[code] = n
				}
			case FieldChoices:
				var list []string
				if json.Unmarshal(x, &list) == nil && len(list) > 0 {
					v[code] = list
				}
			default:
				var s string
				if json.Unmarshal(x, &s) == nil && s != "" {
					v[code] = s
				}
			}
		}
		out[id] = v
	}
	return out, nil
}
