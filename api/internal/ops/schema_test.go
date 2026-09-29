package ops

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestEmbeddedSchemaIsFresh(t *testing.T) {
	canonical, err := os.ReadFile("../../../contracts/ops/project.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, ProjectSchemaJSON()) {
		t.Fatal("api/internal/ops/project.schema.json differs from contracts/ops/project.schema.json: run node scripts/gen-ops-schema.mjs")
	}
}

func TestSchemaRejectsWordsOutsideVocabulary(t *testing.T) {
	for name, doc := range map[string]string{
		"unknown collection key": `{"version":1,"collections":{"a":{"kind":"list","fields":{},"color":"red"}}}`,
		"unknown field key":      `{"version":1,"collections":{"a":{"kind":"list","fields":{"x":{"type":"string","default":"y"}}}}}`,
		"unknown type":           `{"version":1,"collections":{"a":{"kind":"list","fields":{"x":{"type":"date"}}}}}`,
		"unknown kind":           `{"version":1,"collections":{"a":{"kind":"map","fields":{}}}}`,
		"ordered singleton":      `{"version":1,"collections":{"a":{"kind":"singleton","ordered":true,"fields":{}}}}`,
		"ref to nowhere":         `{"version":1,"collections":{"a":{"kind":"list","fields":{"x":{"type":"ref","to":"b","on_delete":"cascade"}}}}}`,
		"set_null not nullable":  `{"version":1,"collections":{"a":{"kind":"list","fields":{"x":{"type":"ref","to":"a","on_delete":"set_null"}}}}}`,
		"remove on ref":          `{"version":1,"collections":{"a":{"kind":"list","fields":{"x":{"type":"ref","to":"a","on_delete":"remove"}}}}}`,
		"key not unique":         `{"version":1,"collections":{"a":{"kind":"list","fields":{"c":{"type":"string","immutable":true},"x":{"type":"ref_list","to":"a","key":"c","on_delete":"remove"}}}}}`,
		"object without check":   `{"version":1,"collections":{"a":{"kind":"list","fields":{"x":{"type":"object"}}}}}`,
		"min on string":          `{"version":1,"collections":{"a":{"kind":"list","fields":{"x":{"type":"string","min":1}}}}}`,
		"reserved field":         `{"version":1,"collections":{"a":{"kind":"list","fields":{"order":{"type":"string"}}}}}`,
		"unique unknown field":   `{"version":1,"collections":{"a":{"kind":"list","unique":[["y"]],"fields":{}}}}`,
		"requires unknown field": `{"version":1,"collections":{"a":{"kind":"list","requires":[{"if_set":"x","then_nonempty":"y"}],"fields":{"x":{"type":"int"}}}}}`,
	} {
		if _, err := ParseSchema([]byte(doc)); err == nil {
			t.Errorf("%s: schema accepted", name)
		}
	}
}

func TestProjectSchemaParses(t *testing.T) {
	s := ProjectSchema()
	if s.Version != 1 || len(s.Names()) != 14 {
		t.Fatalf("version %d, collections %v", s.Version, s.Names())
	}
	if !strings.Contains(strings.Join(s.Names(), ","), "map_flows") {
		t.Fatal("map_flows missing")
	}
}
