// Package ops applies project operations (insert, set, delete, move) to collections described by
// contracts/ops/project.schema.json. It has no database or HTTP code; the TS store mirrors it.
package ops

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
)

//go:embed project.schema.json
var projectSchemaJSON []byte

const (
	KindSingleton = "singleton"
	KindList      = "list"

	TypeString      = "string"
	TypeInt         = "int"
	TypeNumber      = "number"
	TypeBool        = "bool"
	TypeEnum        = "enum"
	TypePoint       = "point"
	TypePoints      = "points"
	TypeObject      = "object"
	TypeParams      = "params"
	TypeRef         = "ref"
	TypeRefList     = "ref_list"
	TypeExternalRef = "external_ref"

	OnDeleteCascade = "cascade"
	OnDeleteSetNull = "set_null"
	OnDeleteRemove  = "remove"

	FieldID    = "id"
	FieldOrder = "order"
	// FieldObjectType selects the object type that validates the record's params fields.
	FieldObjectType = "object_type"
)

// Schema is the parsed operation schema. Build it with ParseSchema; the zero value is unusable.
type Schema struct {
	Version     int                    `json:"version"`
	Collections map[string]*Collection `json:"collections"`

	names []string
	refs  map[string][]backRef
}

type Collection struct {
	Kind    string `json:"kind"`
	Ordered bool   `json:"ordered,omitempty"`
	// LegacyIDs lets insert take ids like "p1" besides UUIDs: map features had them before operations, and
	// undoing a delete brings a record back under its id.
	LegacyIDs bool              `json:"legacy_ids,omitempty"`
	MaxItems  int               `json:"max_items,omitempty"`
	Unique    [][]string        `json:"unique,omitempty"`
	Fields    map[string]*Field `json:"fields"`
	Requires  []Require         `json:"requires,omitempty"`

	name       string
	fieldNames []string
}

type Field struct {
	Type      string   `json:"type"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	MinLength *int     `json:"min_length,omitempty"`
	MaxLength *int     `json:"max_length,omitempty"`
	Nullable  bool     `json:"nullable,omitempty"`
	Required  bool     `json:"required,omitempty"`
	Immutable bool     `json:"immutable,omitempty"`
	Values    []string `json:"values,omitempty"`
	To        string   `json:"to,omitempty"`
	Key       string   `json:"key,omitempty"`
	OnDelete  string   `json:"on_delete,omitempty"`
	Validator string   `json:"validator,omitempty"`
}

type Require struct {
	IfSet        string `json:"if_set"`
	ThenNonempty string `json:"then_nonempty"`
}

// backRef is a field that points at records of another collection.
type backRef struct {
	coll  string
	field string
}

// ProjectSchema returns the embedded project schema.
func ProjectSchema() *Schema {
	s, err := ParseSchema(projectSchemaJSON)
	if err != nil {
		panic(err)
	}
	return s
}

// ProjectSchemaJSON returns the embedded schema file as it is stored.
func ProjectSchemaJSON() []byte {
	return append([]byte(nil), projectSchemaJSON...)
}

// ParseSchema decodes a schema and rejects words outside the fixed vocabulary.
func ParseSchema(raw []byte) (*Schema, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var s Schema
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("ops.schema: %w", err)
	}
	if s.Version < 1 {
		return nil, fmt.Errorf("ops.schema: version must be positive")
	}
	if len(s.Collections) == 0 {
		return nil, fmt.Errorf("ops.schema: no collections")
	}
	for name, c := range s.Collections {
		if c == nil {
			return nil, fmt.Errorf("ops.schema: collection %s is empty", name)
		}
		c.name = name
		s.names = append(s.names, name)
		for f := range c.Fields {
			c.fieldNames = append(c.fieldNames, f)
		}
		sort.Strings(c.fieldNames)
	}
	sort.Strings(s.names)
	s.refs = map[string][]backRef{}
	for _, name := range s.names {
		c := s.Collections[name]
		if err := s.checkCollection(c); err != nil {
			return nil, err
		}
		for _, fn := range c.fieldNames {
			f := c.Fields[fn]
			if f.Type == TypeRef || f.Type == TypeRefList {
				s.refs[f.To] = append(s.refs[f.To], backRef{coll: name, field: fn})
			}
		}
	}
	return &s, nil
}

func (s *Schema) checkCollection(c *Collection) error {
	where := "ops.schema." + c.name
	switch c.Kind {
	case KindSingleton:
		if c.Ordered || c.LegacyIDs || c.MaxItems != 0 || len(c.Unique) > 0 {
			return fmt.Errorf("%s: a singleton has no order, legacy_ids, max_items or unique", where)
		}
	case KindList:
	default:
		return fmt.Errorf("%s: kind must be singleton or list", where)
	}
	if c.MaxItems < 0 {
		return fmt.Errorf("%s: max_items must not be negative", where)
	}
	for _, fn := range c.fieldNames {
		if fn == FieldID || fn == FieldOrder {
			return fmt.Errorf("%s: field name %s is reserved", where, fn)
		}
		if err := s.checkField(c, fn, c.Fields[fn]); err != nil {
			return err
		}
	}
	for _, set := range c.Unique {
		if len(set) == 0 {
			return fmt.Errorf("%s: empty unique set", where)
		}
		for _, fn := range set {
			if c.Fields[fn] == nil {
				return fmt.Errorf("%s: unique names unknown field %s", where, fn)
			}
		}
	}
	for _, r := range c.Requires {
		if c.Fields[r.IfSet] == nil || c.Fields[r.ThenNonempty] == nil {
			return fmt.Errorf("%s: requires names an unknown field", where)
		}
	}
	return nil
}

func (s *Schema) checkField(c *Collection, name string, f *Field) error {
	where := "ops.schema." + c.name + "." + name
	if f == nil {
		return fmt.Errorf("%s: empty field", where)
	}
	numeric := f.Type == TypeInt || f.Type == TypeNumber
	sized := f.Type == TypeString || f.Type == TypePoints || f.Type == TypeRefList
	if (f.Min != nil || f.Max != nil) && !numeric {
		return fmt.Errorf("%s: min and max apply to int and number", where)
	}
	if (f.MinLength != nil || f.MaxLength != nil) && !sized {
		return fmt.Errorf("%s: min_length and max_length apply to string, points and ref_list", where)
	}
	if len(f.Values) > 0 != (f.Type == TypeEnum) {
		return fmt.Errorf("%s: values belong to enum and enum needs values", where)
	}
	if (f.Validator != "") != (f.Type == TypeObject) {
		return fmt.Errorf("%s: validator belongs to object and object needs a validator", where)
	}
	isRef := f.Type == TypeRef || f.Type == TypeRefList
	if (f.To != "") != (isRef || f.Type == TypeExternalRef) {
		return fmt.Errorf("%s: to belongs to ref, ref_list and external_ref", where)
	}
	if (f.Key != "" || f.OnDelete != "") && !isRef {
		return fmt.Errorf("%s: key and on_delete belong to ref and ref_list", where)
	}
	switch f.Type {
	case TypeString, TypeInt, TypeNumber, TypeBool, TypeEnum, TypePoint, TypePoints, TypeObject, TypeParams, TypeExternalRef:
	case TypeRef, TypeRefList:
		target := s.Collections[f.To]
		if target == nil || target.Kind != KindList {
			return fmt.Errorf("%s: ref target %s must be a list collection", where, f.To)
		}
		if f.Key != "" && f.Key != FieldID {
			k := target.Fields[f.Key]
			if k == nil || k.Type != TypeString || !k.Immutable || !uniqueAlone(target, f.Key) {
				return fmt.Errorf("%s: ref key %s must be an immutable unique string", where, f.Key)
			}
		}
		switch {
		case f.Type == TypeRef && f.OnDelete == OnDeleteCascade:
		case f.Type == TypeRef && f.OnDelete == OnDeleteSetNull && f.Nullable:
		case f.Type == TypeRefList && f.OnDelete == OnDeleteRemove:
		default:
			return fmt.Errorf("%s: on_delete %q does not fit %s", where, f.OnDelete, f.Type)
		}
	default:
		return fmt.Errorf("%s: unknown type %q", where, f.Type)
	}
	return nil
}

func uniqueAlone(c *Collection, field string) bool {
	for _, set := range c.Unique {
		if len(set) == 1 && set[0] == field {
			return true
		}
	}
	return false
}

// Names returns collection names in sorted order.
func (s *Schema) Names() []string {
	return append([]string(nil), s.names...)
}

// FieldNames returns the collection's field names in sorted order.
func (c *Collection) FieldNames() []string {
	return append([]string(nil), c.fieldNames...)
}

func (c *Collection) refKey(f *Field) string {
	if f.Key == "" {
		return FieldID
	}
	return f.Key
}
