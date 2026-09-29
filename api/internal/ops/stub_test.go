package ops

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"
)

// stubHooks implements contracts/ops/fixtures/hooks.json. web/src/store/apply.fixtures.test.ts has the same stub.
type stubHooks struct {
	ParamSpec  map[string]map[string]stubParam `json:"params"`
	ObjectSpec map[string]json.RawMessage      `json:"objects"`
	Externals  map[string][]string             `json:"external"`
}

type stubParam struct {
	Type     string   `json:"type"`
	Min      *float64 `json:"min"`
	Max      *float64 `json:"max"`
	Nullable bool     `json:"nullable"`
}

func loadStub(t *testing.T) *stubHooks {
	t.Helper()
	raw, err := os.ReadFile(fixturesDir + "/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	var h stubHooks
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatal(err)
	}
	return &h
}

func (h *stubHooks) Params(rec Record, field string, keys []string) (string, bool) {
	params, _ := rec[field].(map[string]any)
	spec := h.ParamSpec[rec.String("object_type")]
	if keys == nil {
		keys = sortedKeys(params)
	}
	for _, k := range keys {
		p, known := spec[k]
		if !known || !p.ok(params[k]) {
			return k, false
		}
	}
	return "", true
}

func (p stubParam) ok(v any) bool {
	if v == nil {
		return p.Nullable
	}
	switch p.Type {
	case "bool":
		_, ok := v.(bool)
		return ok
	case "number", "int":
		n, ok := v.(float64)
		if !ok || (p.Type == "int" && n != math.Trunc(n)) {
			return false
		}
		return (p.Min == nil || n >= *p.Min) && (p.Max == nil || n <= *p.Max)
	}
	return false
}

func (h *stubHooks) Object(validator string, value any) bool {
	raw, known := h.ObjectSpec[validator]
	if !known {
		return false
	}
	var list string
	if json.Unmarshal(raw, &list) == nil {
		items, ok := value.([]any)
		if !ok {
			return false
		}
		for _, x := range items {
			if _, ok := x.(string); !ok {
				return false
			}
		}
		return true
	}
	var spec map[string]string
	if err := json.Unmarshal(raw, &spec); err != nil {
		return false
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return false
	}
	keys := sortedKeys(obj)
	sort.Strings(keys)
	for _, k := range keys {
		kind, known := spec[k]
		if !known {
			kind, known = spec["*"]
		}
		if !known {
			return false
		}
		switch kind {
		case "number":
			n, ok := obj[k].(float64)
			if !ok || n < 0 {
				return false
			}
		case "string":
			if _, ok := obj[k].(string); !ok {
				return false
			}
		}
	}
	return true
}

func (h *stubHooks) External(to, id string) bool {
	for _, x := range h.Externals[to] {
		if x == id {
			return true
		}
	}
	return false
}
