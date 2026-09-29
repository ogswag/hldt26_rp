package ops

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	OpInsert = "insert"
	OpSet    = "set"
	OpDelete = "delete"
	OpMove   = "move"
	// OpReset marks a legacy write in the journal. Clients reload the snapshot when they see it.
	OpReset = "reset"

	StatusApplied  = "applied"
	StatusRejected = "rejected"

	ReasonTargetMissing  = "target_missing"
	ReasonRefMissing     = "ref_missing"
	ReasonUnique         = "unique"
	ReasonLimit          = "limit"
	ReasonInvalidValue   = "invalid_value"
	ReasonImmutable      = "immutable"
	ReasonForbidden      = "forbidden"
	ReasonSchemaVersion  = "schema_version"
	ReasonProjectDeleted = "project_deleted"

	ChangeInsert = "insert"
	ChangeUpdate = "update"
	ChangeDelete = "delete"
)

// Op is one primitive. Set addresses a field or, for object and params fields, "field.key".
type Op struct {
	Op    string          `json:"op"`
	Coll  string          `json:"coll,omitempty"`
	ID    string          `json:"id,omitempty"`
	Path  string          `json:"path,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
	Order string          `json:"order,omitempty"`
}

// Tx is one user action. It applies whole or not at all.
type Tx struct {
	TxID  string `json:"tx_id"`
	Label string `json:"label,omitempty"`
	Ops   []Op   `json:"ops"`
}

type Outcome struct {
	Status  string   `json:"status"`
	Reason  string   `json:"reason,omitempty"`
	Details *Details `json:"details,omitempty"`
}

// Details points at the operation and field that failed.
type Details struct {
	Op    int    `json:"op"`
	Coll  string `json:"coll,omitempty"`
	ID    string `json:"id,omitempty"`
	Field string `json:"field,omitempty"`
}

// Change is the net effect of a transaction on one record, cascades included.
type Change struct {
	Coll   string `json:"coll"`
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Before Record `json:"before,omitempty"`
	After  Record `json:"after,omitempty"`
}

// Hooks check what the schema cannot: object type params, named object validators and external refs.
type Hooks interface {
	// Params returns the first bad key of rec[field]. keys nil means every key.
	Params(rec Record, field string, keys []string) (badKey string, ok bool)
	Object(validator string, value any) bool
	External(to, id string) bool
}

type recKey struct{ coll, id string }

type touch struct {
	before    Record
	fields    map[string]bool
	allParams map[string]bool
	params    map[string]map[string]bool
	lastOp    int
}

type violation struct {
	reason string
	d      Details
}

type applier struct {
	s       *Schema
	h       Hooks
	st      State
	owned   map[string]bool
	order   []recKey
	touched map[recKey]*touch
	checked map[string]int
	inserts map[string]string
	op      int
}

// Apply runs tx against st. On rejection it returns st unchanged and no changes.
func Apply(s *Schema, st State, tx Tx, h Hooks) (State, []Change, Outcome) {
	work := State{}
	for k, v := range st {
		work[k] = v
	}
	a := &applier{
		s: s, h: h, st: work, owned: map[string]bool{}, touched: map[recKey]*touch{},
		checked: map[string]int{}, inserts: map[string]string{},
	}
	for i, op := range tx.Ops {
		a.op = i
		if v := a.apply(op); v != nil {
			v.d.Op = i
			return st, nil, Outcome{Status: StatusRejected, Reason: v.reason, Details: &v.d}
		}
	}
	if v := a.finish(); v != nil {
		return st, nil, Outcome{Status: StatusRejected, Reason: v.reason, Details: &v.d}
	}
	return a.st, a.changes(), Outcome{Status: StatusApplied}
}

func reject(reason, coll, id, field string) *violation {
	return &violation{reason: reason, d: Details{Coll: coll, ID: id, Field: field}}
}

func (a *applier) apply(op Op) *violation {
	c := a.s.Collections[op.Coll]
	switch op.Op {
	case OpInsert, OpSet, OpDelete, OpMove:
	default:
		return reject(ReasonInvalidValue, op.Coll, op.ID, "")
	}
	if c == nil {
		return reject(ReasonInvalidValue, op.Coll, op.ID, "")
	}
	switch op.Op {
	case OpInsert:
		return a.insert(c, op)
	case OpSet:
		return a.set(c, op)
	case OpDelete:
		return a.delete(c, op)
	}
	return a.move(c, op)
}

func (a *applier) write(coll, id string, rec Record) {
	if !a.owned[coll] {
		src := a.st[coll]
		cp := make(map[string]Record, len(src)+1)
		for k, v := range src {
			cp[k] = v
		}
		a.st[coll] = cp
		a.owned[coll] = true
	}
	k := recKey{coll, id}
	t := a.touched[k]
	if t == nil {
		t = &touch{before: a.st[coll][id], fields: map[string]bool{}, allParams: map[string]bool{}, params: map[string]map[string]bool{}}
		a.touched[k] = t
		a.order = append(a.order, k)
	}
	t.lastOp = a.op
	if rec == nil {
		delete(a.st[coll], id)
		return
	}
	a.st[coll][id] = rec
}

// mark records a field set by an explicit operation. Only such fields get reference and params checks.
func (a *applier) mark(coll, id, field, key string, keyed bool) {
	t := a.touched[recKey{coll, id}]
	t.fields[field] = true
	if keyed {
		if t.params[field] == nil {
			t.params[field] = map[string]bool{}
		}
		t.params[field][key] = true
	} else {
		t.allParams[field] = true
	}
	a.checked[coll] = a.op
}

func (a *applier) insert(c *Collection, op Op) *violation {
	if c.Kind == KindSingleton {
		return reject(ReasonImmutable, c.name, op.ID, "")
	}
	if !validUUID(op.ID) && !(c.LegacyIDs && validLegacyID(op.ID)) {
		return reject(ReasonInvalidValue, c.name, op.ID, FieldID)
	}
	if a.st[c.name][op.ID] != nil {
		return reject(ReasonUnique, c.name, op.ID, FieldID)
	}
	raw, ok := decodeValue(op.Value)
	obj, isObj := raw.(map[string]any)
	if !ok || !isObj {
		return reject(ReasonInvalidValue, c.name, op.ID, "")
	}
	for _, k := range sortedKeys(obj) {
		if k == FieldOrder && c.Ordered {
			continue
		}
		if c.Fields[k] == nil {
			return reject(ReasonInvalidValue, c.name, op.ID, k)
		}
	}
	rec := Record{FieldID: op.ID}
	if c.Ordered {
		o, ok := obj[FieldOrder].(string)
		if !ok || !ValidOrderKey(o) {
			return reject(ReasonInvalidValue, c.name, op.ID, FieldOrder)
		}
		rec[FieldOrder] = o
	}
	for _, fn := range c.fieldNames {
		f := c.Fields[fn]
		v, present := obj[fn]
		if !present {
			if f.Required {
				return reject(ReasonInvalidValue, c.name, op.ID, fn)
			}
			v = zeroValue(f)
		}
		if !checkValue(f, v) || !a.objectOK(f, v) {
			return reject(ReasonInvalidValue, c.name, op.ID, fn)
		}
		rec[fn] = v
	}
	a.write(c.name, op.ID, rec)
	for _, fn := range c.fieldNames {
		a.mark(c.name, op.ID, fn, "", false)
	}
	a.inserts[c.name] = op.ID
	return nil
}

func (a *applier) set(c *Collection, op Op) *violation {
	id := op.ID
	if c.Kind == KindSingleton && id == "" {
		id = c.name
	}
	rec := a.st[c.name][id]
	if rec == nil {
		return reject(ReasonTargetMissing, c.name, id, "")
	}
	field, key, keyed := strings.Cut(op.Path, ".")
	f := c.Fields[field]
	if f == nil || (keyed && ((f.Type != TypeObject && f.Type != TypeParams) || key == "" || strings.Contains(key, "."))) {
		return reject(ReasonInvalidValue, c.name, id, op.Path)
	}
	v, ok := decodeValue(op.Value)
	if !ok {
		return reject(ReasonInvalidValue, c.name, id, op.Path)
	}
	if f.Immutable {
		return reject(ReasonImmutable, c.name, id, field)
	}
	next := v
	if keyed {
		cur, isObj := rec[field].(map[string]any)
		if rec[field] != nil && !isObj {
			return reject(ReasonInvalidValue, c.name, id, op.Path)
		}
		m := make(map[string]any, len(cur)+1)
		for k, x := range cur {
			m[k] = x
		}
		m[key] = v
		next = m
	}
	if !checkValue(f, next) || !a.objectOK(f, next) {
		return reject(ReasonInvalidValue, c.name, id, op.Path)
	}
	if Equal(rec[field], next) {
		return nil
	}
	nr := rec.clone()
	nr[field] = next
	a.write(c.name, id, nr)
	a.mark(c.name, id, field, key, keyed)
	return nil
}

func (a *applier) delete(c *Collection, op Op) *violation {
	if c.Kind == KindSingleton {
		return reject(ReasonImmutable, c.name, op.ID, "")
	}
	if a.st[c.name][op.ID] == nil {
		return reject(ReasonTargetMissing, c.name, op.ID, "")
	}
	a.remove(c.name, op.ID)
	return nil
}

// remove deletes a record and applies on_delete rules to every record that points at it.
func (a *applier) remove(coll, id string) {
	rec := a.st[coll][id]
	a.write(coll, id, nil)
	for _, br := range a.s.refs[coll] {
		f := a.s.Collections[br.coll].Fields[br.field]
		target := rec.String(refKey(f))
		if target == "" {
			continue
		}
		var hits []string
		for rid, r := range a.st[br.coll] {
			if refers(f, r[br.field], target) {
				hits = append(hits, rid)
			}
		}
		sort.Strings(hits)
		for _, rid := range hits {
			r := a.st[br.coll][rid]
			if r == nil {
				continue
			}
			switch {
			case f.Type == TypeRef && f.OnDelete == OnDeleteCascade:
				a.remove(br.coll, rid)
			case f.Type == TypeRef:
				nr := r.clone()
				nr[br.field] = nil
				a.write(br.coll, rid, nr)
			default:
				list, _ := r[br.field].([]any)
				kept := make([]any, 0, len(list))
				for _, x := range list {
					if x != target {
						kept = append(kept, x)
					}
				}
				nr := r.clone()
				nr[br.field] = kept
				a.write(br.coll, rid, nr)
			}
		}
	}
}

func refers(f *Field, v any, target string) bool {
	if f.Type == TypeRef {
		return v == target
	}
	list, _ := v.([]any)
	for _, x := range list {
		if x == target {
			return true
		}
	}
	return false
}

func (a *applier) move(c *Collection, op Op) *violation {
	if !c.Ordered {
		return reject(ReasonInvalidValue, c.name, op.ID, FieldOrder)
	}
	rec := a.st[c.name][op.ID]
	if rec == nil {
		return reject(ReasonTargetMissing, c.name, op.ID, "")
	}
	if !ValidOrderKey(op.Order) {
		return reject(ReasonInvalidValue, c.name, op.ID, FieldOrder)
	}
	if rec[FieldOrder] == op.Order {
		return nil
	}
	nr := rec.clone()
	nr[FieldOrder] = op.Order
	a.write(c.name, op.ID, nr)
	return nil
}

func (a *applier) objectOK(f *Field, v any) bool {
	return f.Type != TypeObject || v == nil || a.h.Object(f.Validator, v)
}

func (a *applier) finish() *violation {
	for _, k := range a.order {
		rec := a.st[k.coll][k.id]
		t := a.touched[k]
		if rec == nil || len(t.fields) == 0 {
			continue
		}
		if v := a.checkRecord(a.s.Collections[k.coll], rec, t); v != nil {
			v.d.Op = t.lastOp
			return v
		}
	}
	names := make([]string, 0, len(a.checked))
	for n := range a.checked {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		if v := a.checkCollection(a.s.Collections[name]); v != nil {
			return v
		}
	}
	return nil
}

func (a *applier) checkRecord(c *Collection, rec Record, t *touch) *violation {
	id := rec.String(FieldID)
	for _, fn := range c.fieldNames {
		f := c.Fields[fn]
		// Params are valid only for an object type, so a new type re-checks them in full.
		retyped := f.Type == TypeParams && t.fields[FieldObjectType]
		if !t.fields[fn] && !retyped {
			continue
		}
		switch f.Type {
		case TypeRef:
			if s, ok := rec[fn].(string); ok && !a.exists(f, s) {
				return reject(ReasonRefMissing, c.name, id, fn)
			}
		case TypeRefList:
			list, _ := rec[fn].([]any)
			for _, x := range list {
				if s, _ := x.(string); !a.exists(f, s) {
					return reject(ReasonRefMissing, c.name, id, fn)
				}
			}
		case TypeExternalRef:
			if s, ok := rec[fn].(string); ok && !a.h.External(f.To, s) {
				return reject(ReasonRefMissing, c.name, id, fn)
			}
		case TypeParams:
			var keys []string
			if !t.allParams[fn] && !retyped {
				keys = sortedSet(t.params[fn])
			}
			if bad, ok := a.h.Params(rec, fn, keys); !ok {
				path := fn
				if bad != "" {
					path = fn + "." + bad
				}
				return reject(ReasonInvalidValue, c.name, id, path)
			}
		}
	}
	for _, r := range c.Requires {
		if !t.fields[r.IfSet] && !t.fields[r.ThenNonempty] {
			continue
		}
		if rec[r.IfSet] != nil && !nonEmpty(rec[r.ThenNonempty]) {
			return reject(ReasonInvalidValue, c.name, id, r.ThenNonempty)
		}
	}
	return nil
}

func (a *applier) checkCollection(c *Collection) *violation {
	recs := a.st[c.name]
	if last, ok := a.inserts[c.name]; ok && c.MaxItems > 0 && len(recs) > c.MaxItems {
		v := reject(ReasonLimit, c.name, last, "")
		v.d.Op = a.touched[recKey{c.name, last}].lastOp
		return v
	}
	for _, set := range c.Unique {
		count := make(map[string]int, len(recs))
		for _, r := range recs {
			count[tupleKey(r, set)]++
		}
		for _, k := range a.order {
			rec := recs[k.id]
			t := a.touched[k]
			if k.coll != c.name || rec == nil || !anyTouched(t, set) {
				continue
			}
			if count[tupleKey(rec, set)] > 1 {
				v := reject(ReasonUnique, c.name, k.id, set[0])
				v.d.Op = t.lastOp
				return v
			}
		}
	}
	return nil
}

func (a *applier) exists(f *Field, v string) bool {
	if v == "" {
		return false
	}
	key := refKey(f)
	if key == FieldID {
		return a.st[f.To][v] != nil
	}
	for _, r := range a.st[f.To] {
		if r[key] == v {
			return true
		}
	}
	return false
}

func (a *applier) changes() []Change {
	out := make([]Change, 0, len(a.order))
	for _, k := range a.order {
		before := a.touched[k].before
		after := a.st[k.coll][k.id]
		switch {
		case before == nil && after == nil:
		case before == nil:
			out = append(out, Change{Coll: k.coll, ID: k.id, Kind: ChangeInsert, After: after})
		case after == nil:
			out = append(out, Change{Coll: k.coll, ID: k.id, Kind: ChangeDelete, Before: before})
		case !Equal(before, after):
			out = append(out, Change{Coll: k.coll, ID: k.id, Kind: ChangeUpdate, Before: before, After: after})
		}
	}
	return out
}

func refKey(f *Field) string {
	if f.Key == "" {
		return FieldID
	}
	return f.Key
}

func anyTouched(t *touch, fields []string) bool {
	for _, f := range fields {
		if t.fields[f] {
			return true
		}
	}
	return false
}

func tupleKey(rec Record, fields []string) string {
	vals := make([]any, len(fields))
	for i, f := range fields {
		vals[i] = rec[f]
	}
	b, _ := json.Marshal(vals)
	return string(b)
}

func decodeValue(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, false
	}
	return v, true
}

func zeroValue(f *Field) any {
	if f.Nullable {
		return nil
	}
	switch f.Type {
	case TypeString:
		return ""
	case TypeInt, TypeNumber:
		return 0.0
	case TypeBool:
		return false
	case TypePoint:
		return map[string]any{"x": 0.0, "y": 0.0}
	case TypePoints, TypeRefList:
		return []any{}
	case TypeParams:
		return map[string]any{}
	}
	return nil
}

func checkValue(f *Field, v any) bool {
	if v == nil {
		return f.Nullable
	}
	switch f.Type {
	case TypeString:
		s, ok := v.(string)
		return ok && lengthOK(f, utf8.RuneCountInString(s))
	case TypeInt:
		n, ok := v.(float64)
		return ok && n == math.Trunc(n) && numberOK(f, n)
	case TypeNumber:
		n, ok := v.(float64)
		return ok && numberOK(f, n)
	case TypeBool:
		_, ok := v.(bool)
		return ok
	case TypeEnum:
		s, ok := v.(string)
		if !ok {
			return false
		}
		for _, x := range f.Values {
			if x == s {
				return true
			}
		}
		return false
	case TypePoint:
		return isPoint(v)
	case TypePoints:
		list, ok := v.([]any)
		if !ok || !lengthOK(f, len(list)) {
			return false
		}
		for _, p := range list {
			if !isPoint(p) {
				return false
			}
		}
		return true
	case TypeObject:
		return true
	case TypeParams:
		_, ok := v.(map[string]any)
		return ok
	case TypeRef, TypeExternalRef:
		s, ok := v.(string)
		return ok && s != ""
	case TypeRefList:
		list, ok := v.([]any)
		if !ok || !lengthOK(f, len(list)) {
			return false
		}
		seen := map[string]bool{}
		for _, x := range list {
			s, ok := x.(string)
			if !ok || s == "" || seen[s] {
				return false
			}
			seen[s] = true
		}
		return true
	}
	return false
}

func numberOK(f *Field, n float64) bool {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return false
	}
	return (f.Min == nil || n >= *f.Min) && (f.Max == nil || n <= *f.Max)
}

func lengthOK(f *Field, n int) bool {
	return (f.MinLength == nil || n >= *f.MinLength) && (f.MaxLength == nil || n <= *f.MaxLength)
}

func isPoint(v any) bool {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 2 {
		return false
	}
	x, okx := m["x"].(float64)
	y, oky := m["y"].(float64)
	return okx && oky && !math.IsNaN(x) && !math.IsInf(x, 0) && !math.IsNaN(y) && !math.IsInf(y, 0)
}

func nonEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(t) != ""
	case []any:
		return len(t) > 0
	}
	return true
}

// validLegacyID accepts the ids map features had before operations: 1 to 64 Latin letters, digits, "-", "_"
// and ".", as maps.Validate allows.
func validLegacyID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '-' || c == '_' || c == '.' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
				return false
			}
		}
	}
	return true
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
