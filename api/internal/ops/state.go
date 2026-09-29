package ops

import (
	"encoding/json"
	"reflect"
	"sort"
)

// Record is one record as decoded JSON: id, order for ordered collections, then schema fields.
// Values are string, float64, bool, nil, []any or map[string]any.
type Record map[string]any

// State holds records by collection and id. Treat it as immutable: Apply returns a new State
// that shares untouched collections and records with its input.
type State map[string]map[string]Record

// NewState returns a state with an empty map for every collection of s.
func NewState(s *Schema) State {
	st := State{}
	for _, name := range s.names {
		st[name] = map[string]Record{}
	}
	return st
}

// Get returns the record or nil.
func (st State) Get(coll, id string) Record {
	return st[coll][id]
}

// IDs returns the ids of a collection in sorted order.
func (st State) IDs(coll string) []string {
	out := make([]string, 0, len(st[coll]))
	for id := range st[coll] {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Sorted returns the records of an ordered collection by order key, then id.
func (st State) Sorted(coll string) []Record {
	out := make([]Record, 0, len(st[coll]))
	for _, id := range st.IDs(coll) {
		out = append(out, st[coll][id])
	}
	sort.SliceStable(out, func(i, j int) bool {
		oi, _ := out[i][FieldOrder].(string)
		oj, _ := out[j][FieldOrder].(string)
		return oi < oj
	})
	return out
}

func (r Record) clone() Record {
	out := make(Record, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}

// String returns a string field or "".
func (r Record) String(field string) string {
	s, _ := r[field].(string)
	return s
}

// Equal compares two decoded JSON values.
func Equal(a, b any) bool {
	return reflect.DeepEqual(a, b)
}

// Decode turns any JSON-compatible Go value into the plain decoded form used in records.
func Decode(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func cloneValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = cloneValue(x)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = cloneValue(x)
		}
		return out
	}
	return v
}
