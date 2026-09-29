package ops

import (
	"encoding/json"
	"sort"

	"github.com/google/uuid"
)

// pointsAt lists the collections a collection's reference fields target.
func (s *Schema) pointsAt(name string) []string {
	seen := map[string]bool{}
	c := s.Collections[name]
	for _, fn := range c.fieldNames {
		f := c.Fields[fn]
		if (f.Type == TypeRef || f.Type == TypeRefList) && f.To != "" && f.To != name {
			seen[f.To] = true
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// DeleteOrder lists the list collections so that every collection comes before the ones it points at. Emptying
// a project in this order never reaches a record a cascade has already taken away, which would be a rejection.
func (s *Schema) DeleteOrder() []string {
	var post []string
	seen := map[string]bool{}
	var visit func(name string)
	visit = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		for _, to := range s.pointsAt(name) {
			visit(to)
		}
		post = append(post, name)
	}
	for _, name := range s.names {
		visit(name)
	}
	out := make([]string, 0, len(post))
	for i := len(post) - 1; i >= 0; i-- {
		if s.Collections[post[i]].Kind == KindList {
			out = append(out, post[i])
		}
	}
	return out
}

func rawValue(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return raw
}

// Replace builds the operations that turn from into to. Every record of from goes, every record of to arrives,
// and the fields of the singleton records are set last, when whatever their references point at already exists.
// The result is one transaction: the project either becomes to or stays exactly as it was.
func Replace(s *Schema, from, to State) []Op {
	order := s.DeleteOrder()
	list := []Op{}
	for _, coll := range order {
		for _, id := range from.IDs(coll) {
			list = append(list, Op{Op: OpDelete, Coll: coll, ID: id})
		}
	}
	// Inserting runs the other way round, so a record arrives after the records it points at.
	for i := len(order) - 1; i >= 0; i-- {
		coll := order[i]
		for _, rec := range to.Sorted(coll) {
			value := map[string]any{}
			for k, v := range rec {
				if k != FieldID {
					value[k] = v
				}
			}
			list = append(list, Op{Op: OpInsert, Coll: coll, ID: rec.String(FieldID), Value: rawValue(value)})
		}
	}
	for _, name := range s.names {
		c := s.Collections[name]
		if c.Kind != KindSingleton {
			continue
		}
		rec := to.Get(name, name)
		if rec == nil {
			continue
		}
		for _, fn := range c.fieldNames {
			if fn == FieldID {
				continue
			}
			list = append(list, Op{Op: OpSet, Coll: name, ID: name, Path: fn, Value: rawValue(rec[fn])})
		}
	}
	return list
}

// Rekey returns a copy of st in which every record identified by a UUID gets a fresh one, and every reference
// that points at such a record by its id follows. Identifiers of a project are its own: importing a snapshot
// under new ones lets the same snapshot be saved twice, and never collides with the project it came from.
// Map features named the old way ("T4", "res-1") keep their names, which mean nothing outside their project.
func Rekey(s *Schema, st State) State {
	fresh := map[string]map[string]string{}
	for _, name := range s.names {
		if s.Collections[name].Kind != KindList {
			continue
		}
		m := map[string]string{}
		for _, id := range st.IDs(name) {
			if validUUID(id) {
				m[id] = uuid.NewString()
			}
		}
		fresh[name] = m
	}
	out := State{}
	for _, name := range s.names {
		c := s.Collections[name]
		out[name] = map[string]Record{}
		for _, id := range st.IDs(name) {
			rec := st[name][id].clone()
			next := id
			if to, ok := fresh[name][id]; ok {
				next = to
				rec[FieldID] = to
			}
			for _, fn := range c.fieldNames {
				f := c.Fields[fn]
				if c.refKey(f) != FieldID || fresh[f.To] == nil {
					continue
				}
				switch f.Type {
				case TypeRef:
					if old, ok := rec[fn].(string); ok {
						if to, ok := fresh[f.To][old]; ok {
							rec[fn] = to
						}
					}
				case TypeRefList:
					list, ok := rec[fn].([]any)
					if !ok {
						continue
					}
					next := make([]any, 0, len(list))
					for _, v := range list {
						if old, ok := v.(string); ok {
							if to, ok := fresh[f.To][old]; ok {
								v = to
							}
						}
						next = append(next, v)
					}
					rec[fn] = next
				}
			}
			out[name][next] = rec
		}
	}
	return out
}
