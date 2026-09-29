package objects

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDefaultsPassValidation(t *testing.T) {
	for _, typ := range []string{Warehouse, Airport, Hospital} {
		def, err := Defaults(typ)
		if err != nil {
			t.Fatalf("%s defaults: %v", typ, err)
		}
		raw, err := json.Marshal(def)
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(typ, raw); err != nil {
			t.Fatalf("%s defaults invalid: %v", typ, err)
		}
	}
}

func TestValidateRejectsOutOfRange(t *testing.T) {
	def, err := Defaults(Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	def["aisle_working_m"] = 1.0
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	err = Validate(Warehouse, raw)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	found := false
	for _, d := range ve.Details {
		if d.Field == "aisle_working_m" {
			found = true
		}
	}
	if !found {
		t.Fatalf("details: %+v", ve.Details)
	}
}

func TestValidateRejectsUnknownKey(t *testing.T) {
	def, err := Defaults(Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	def["not_a_field"] = 1
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	err = Validate(Warehouse, raw)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	found := false
	for _, d := range ve.Details {
		if d.Field == "not_a_field" {
			found = true
		}
	}
	if !found {
		t.Fatalf("details: %+v", ve.Details)
	}
}

func TestValidateRejectsMissingAndWrongType(t *testing.T) {
	if err := Validate(Warehouse, json.RawMessage(`{"area_total_m2":"big"}`)); err == nil {
		t.Fatal("want error")
	}
	if err := Validate(Airport, json.RawMessage(`{}`)); err == nil {
		t.Fatal("empty airport: want error")
	}
	def, err := Defaults(Airport)
	if err != nil {
		t.Fatal(err)
	}
	def["terminals"] = 1.5
	b, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	err = Validate(Airport, b)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	found := false
	for _, d := range ve.Details {
		if d.Field == "terminals" {
			found = true
		}
	}
	if !found {
		t.Fatalf("details: %+v", ve.Details)
	}
}

func TestSchemaUnknownType(t *testing.T) {
	_, err := SchemaFor("factory")
	if !errors.Is(err, ErrUnknownType) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateBooleanEnumUnknown(t *testing.T) {
	def, err := Defaults(Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	def["has_wms"] = nil
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(Warehouse, raw); err != nil {
		t.Fatalf("unknown WMS should pass: %v", err)
	}
	delete(def, "has_wms")
	raw, err = json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(Warehouse, raw); err == nil {
		t.Fatal("missing nullable required field should fail")
	}
	def["has_wms"] = true
	def["floor_type"] = "дерево"
	raw, err = json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	err = Validate(Warehouse, raw)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	found := false
	for _, d := range ve.Details {
		if d.Field == "floor_type" {
			found = true
		}
	}
	if !found {
		t.Fatalf("details: %+v", ve.Details)
	}
	def, err = Defaults(Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	def["task_mix"] = []string{"inbound"}
	def["shift_window"] = TimeRange{Start: "07:00", End: "19:00"}
	raw, err = json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(Warehouse, raw); err != nil {
		t.Fatalf("task mix and window: %v", err)
	}
	delete(def, "aisle_working_m")
	raw, err = json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(Warehouse, raw); err == nil {
		t.Fatal("missing required aisle should fail")
	}
}

func TestNormalizeClock(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"08:00", "08:00", true},
		{"8:00", "08:00", true},
		{"08:00:00", "08:00", true},
		{"08:00:00.500", "08:00", true},
		{"22:00:00", "22:00", true},
		{"25:00", "", false},
		{"08:00:99", "", false},
		{"", "", false},
		{"08-00", "", false},
	}
	for _, tc := range cases {
		got, ok := NormalizeClock(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("%q: got %q %v want %q %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestFillDefaultsMissingAndClocks(t *testing.T) {
	def, err := Defaults(Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	delete(def, "floor_type")
	delete(def, "has_wms")
	delete(def, "shift_window")
	delete(def, "task_mix")
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(Warehouse, raw); err == nil {
		t.Fatal("missing fields should fail Validate")
	}
	filled, err := FillDefaults(Warehouse, raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(Warehouse, filled); err != nil {
		t.Fatalf("filled: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(filled, &got); err != nil {
		t.Fatal(err)
	}
	if got["floor_type"] != "Промышленный бетон" {
		t.Fatalf("floor_type %v", got["floor_type"])
	}
	if got["has_wms"] != true {
		t.Fatalf("has_wms %v", got["has_wms"])
	}

	def, err = Defaults(Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	def["shift_window"] = map[string]any{"start": "08:00:00", "end": "22:00:00.0"}
	raw, err = json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	norm, err := NormalizeParams(Warehouse, raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(Warehouse, norm); err != nil {
		t.Fatalf("clocks: %v", err)
	}
	if err := json.Unmarshal(norm, &got); err != nil {
		t.Fatal(err)
	}
	win, ok := got["shift_window"].(map[string]any)
	if !ok || win["start"] != "08:00" || win["end"] != "22:00" {
		t.Fatalf("window %v", got["shift_window"])
	}
}

func TestReferenceWarehouseValidates(t *testing.T) {
	path := filepath.Join("..", "..", "..", "data", "fixtures", "warehouse-reference.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if err := Validate(Warehouse, doc.Params); err != nil {
		t.Fatalf("reference params: %v", err)
	}
}

func TestSeedsMatchSchema(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data", "seeds")
	cases := []string{Warehouse, Airport, Hospital}
	for _, typ := range cases {
		path := filepath.Join(root, typ+".json")
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if err := Validate(typ, b); err != nil {
			t.Fatalf("%s seed: %v", typ, err)
		}
		var seed map[string]any
		if err := json.Unmarshal(b, &seed); err != nil {
			t.Fatal(err)
		}
		s, err := SchemaFor(typ)
		if err != nil {
			t.Fatal(err)
		}
		fields := Fields(s)
		if len(seed) != len(fields) {
			t.Fatalf("%s seed keys %d schema fields %d", typ, len(seed), len(fields))
		}
		for _, f := range fields {
			if _, ok := seed[f.ID]; !ok {
				t.Fatalf("%s seed missing %s", typ, f.ID)
			}
		}
	}
}

func TestSchemaTabsCoverGroups(t *testing.T) {
	for _, ti := range Types() {
		s, err := SchemaFor(ti.Type)
		if err != nil {
			t.Fatal(err)
		}
		used := map[string]int{}
		for _, g := range s.Groups {
			used[g.Tab]++
		}
		declared := map[string]bool{}
		keys := 0
		for _, tab := range s.Tabs {
			if tab.ID == "" || tab.Label == "" || declared[tab.ID] {
				t.Fatalf("%s: bad or duplicate tab %+v", ti.Type, tab)
			}
			declared[tab.ID] = true
			if used[tab.ID] == 0 {
				t.Fatalf("%s: tab %s has no groups", ti.Type, tab.ID)
			}
			if tab.Key {
				keys++
			}
		}
		for _, g := range s.Groups {
			if !declared[g.Tab] {
				t.Fatalf("%s: group %s is on undeclared tab %q", ti.Type, g.ID, g.Tab)
			}
		}
		if keys == 0 {
			t.Fatalf("%s: no key tab", ti.Type)
		}
	}
}

func TestSchemaLabelsOmitUnits(t *testing.T) {
	periods := []string{"сутки", "/сут", " в час", "/год"}
	for _, ti := range Types() {
		s, err := SchemaFor(ti.Type)
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]string{}
		for _, g := range s.Groups {
			for _, f := range g.Fields {
				if f.Unit != "-" && strings.Contains(f.Label, "("+f.Unit+")") {
					t.Errorf("%s.%s: label %q repeats unit %q", ti.Type, f.ID, f.Label, f.Unit)
				}
				for _, p := range periods {
					if strings.Contains(f.Unit, "/") && strings.Contains(f.Label, p) {
						t.Errorf("%s.%s: label %q repeats the rate of unit %q", ti.Type, f.ID, f.Label, f.Unit)
						break
					}
				}
				for _, name := range append([]string{f.Label}, f.Aliases...) {
					key := strings.ToLower(strings.ReplaceAll(name, "ё", "е"))
					if other, ok := names[key]; ok && other != f.ID {
						t.Errorf("%s: %q names both %s and %s", ti.Type, name, other, f.ID)
					}
					names[key] = f.ID
				}
			}
		}
	}
}

func TestParseDimensions(t *testing.T) {
	cases := []struct {
		in   string
		want Dimensions
		ok   bool
	}{
		{"1200x800x1600", Dimensions{1200, 800, 1600}, true},
		{"1200 X 800 X 1600", Dimensions{1200, 800, 1600}, true},
		{"300\u00d7200\u00d7150", Dimensions{300, 200, 150}, true},
		{"300х200х150,5", Dimensions{300, 200, 150.5}, true},
		{"1200x800", Dimensions{}, false},
		{"1200x0x1600", Dimensions{}, false},
		{"axbxc", Dimensions{}, false},
		{"", Dimensions{}, false},
	}
	for _, c := range cases {
		got, ok := ParseDimensions(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("ParseDimensions(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestDimensionsField(t *testing.T) {
	def, err := Defaults(Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		value any
		ok    bool
	}{
		{"object", map[string]any{"length": 1200, "width": 1000, "height": 1800}, true},
		{"part missing", map[string]any{"length": 1200, "width": 1000}, false},
		{"extra key", map[string]any{"length": 1200, "width": 1000, "height": 1800, "depth": 1}, false},
		{"below min", map[string]any{"length": 1200, "width": 100, "height": 1800}, false},
		{"above max", map[string]any{"length": 1200, "width": 800, "height": 9000}, false},
		{"text part", map[string]any{"length": "1200", "width": 800, "height": 1600}, false},
		{"legacy string", "1200x800x1600", false},
	}
	for _, c := range cases {
		params := map[string]any{}
		for k, v := range def {
			params[k] = v
		}
		params["pallet_size_mm"] = c.value
		raw, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(Warehouse, raw); (err == nil) != c.ok {
			t.Errorf("%s: err %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestNormalizeParamsReadsLegacyDimensions(t *testing.T) {
	raw := json.RawMessage(`{"pallet_size_mm":"1200x1000x1600","unit_size_mm":"bad"}`)
	norm, err := NormalizeParams(Warehouse, raw)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(norm, &got); err != nil {
		t.Fatal(err)
	}
	pallet, ok := got["pallet_size_mm"].(map[string]any)
	if !ok || pallet["length"] != 1200.0 || pallet["width"] != 1000.0 || pallet["height"] != 1600.0 {
		t.Fatalf("pallet %v", got["pallet_size_mm"])
	}
	if got["unit_size_mm"] != "bad" {
		t.Fatalf("an unreadable string must stay for Validate to refuse: %v", got["unit_size_mm"])
	}
}

// The form shows the short label on one line, so every label it shows stays short, and every short label names
// a field that exists.
func TestShortLabelsFitOneLine(t *testing.T) {
	shorts := map[string]map[string]string{Warehouse: warehouseShort, Airport: airportShort, Hospital: hospitalShort}
	for _, ti := range Types() {
		s, err := SchemaFor(ti.Type)
		if err != nil {
			t.Fatal(err)
		}
		ids := map[string]bool{}
		for _, f := range Fields(s) {
			ids[f.ID] = true
			shown := f.Label
			if f.Short != "" {
				shown = f.Short
			}
			if n := utf8.RuneCountInString(shown); n > 28 {
				t.Errorf("%s.%s: label %q is %d characters; add a short label of 28 or fewer", ti.Type, f.ID, shown, n)
			}
		}
		for id := range shorts[ti.Type] {
			if !ids[id] {
				t.Errorf("%s: short label for unknown field %s", ti.Type, id)
			}
		}
	}
}

func TestHelpOverridesMatchSchema(t *testing.T) {
	help := map[string]map[string]string{
		Warehouse: warehouseHelp,
		Airport:   airportHelp,
		Hospital:  hospitalHelp,
	}
	for _, ti := range Types() {
		s, err := SchemaFor(ti.Type)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, f := range Fields(s) {
			want, ok := help[ti.Type][f.ID]
			if !ok {
				if f.Help != nil {
					t.Errorf("%s.%s: unexpected help override", ti.Type, f.ID)
				}
				continue
			}
			seen[f.ID] = true
			if f.Help == nil || *f.Help != want {
				t.Errorf("%s.%s: help override was not applied", ti.Type, f.ID)
			}
		}
		for id := range help[ti.Type] {
			if !seen[id] {
				t.Errorf("%s: help override for unknown field %s", ti.Type, id)
			}
		}
	}
}
