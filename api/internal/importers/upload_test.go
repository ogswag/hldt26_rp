package importers

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/db"
)

var (
	idStacker = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	idCart    = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	idOld     = uuid.MustParse("33333333-3333-4333-8333-333333333333")
	idDrone   = uuid.MustParse("44444444-4444-4444-8444-444444444444")
)

func numeric(t *testing.T, v float64) pgtype.Numeric {
	t.Helper()
	n, err := numericFromFloat(&v)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func text(s string) *string { return &s }

func stored(id uuid.UUID, name, vendor string) db.ListSolutionsRow {
	return db.ListSolutionsRow{ID: id, Name: name, Vendor: text(vendor), Raw: json.RawMessage(`{}`)}
}

func catalogRows(t *testing.T) []db.ListSolutionsRow {
	stacker := stored(idStacker, "Штабелёр", "Ронави")
	stacker.PayloadKg = numeric(t, 1500)
	stacker.PriceRub = numeric(t, 2700000)
	stacker.NavType = text("Лидар")
	cart := stored(idCart, "Тележка", "Ронави")
	old := stored(idOld, "Старый робот", "Ронави")
	old.ArchivedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	drone := stored(idDrone, "Дрон", "Коптер")
	return []db.ListSolutionsRow{stacker, cart, old, drone}
}

func planOf(t *testing.T, g Grid, current []db.ListSolutionsRow, base map[string]Values, mode string) Plan {
	t.Helper()
	m := AutoMapping(g.Header())
	if err := m.Check(g.Width()); err != nil {
		t.Fatalf("mapping: %v", err)
	}
	return BuildPlan(ReadRows(g, m), current, base, mode, m.Has("id"))
}

func rowByLine(t *testing.T, p Plan, line int) RowPlan {
	t.Helper()
	for _, rp := range p.Rows {
		if rp.Line == line {
			return rp
		}
	}
	t.Fatalf("no row for line %d", line)
	return RowPlan{}
}

func fieldCodes(rp RowPlan) []string {
	out := []string{}
	for _, d := range rp.Fields {
		out = append(out, d.Code)
	}
	return out
}

func TestBuildPlanWithoutExport(t *testing.T) {
	g := Grid{
		{"id", "Название", "Компания", "Грузоподъёмность, кг", "Цена, ₽"},
		{idStacker.String(), "Штабелёр", "Ронави", "2 000", ""},
		{idCart.String(), "Тележка", "Ронави", "", ""},
		{"", "Новый робот", "Ронави", "50", "100 000"},
		{"", "тележка", "РОНАВИ", "", ""},
		{"", "Сломанный", "", "много", ""},
	}
	p := planOf(t, g, catalogRows(t), nil, ModeCatalog)
	if p.ThreeWay {
		t.Fatal("no export, want two-way")
	}
	stacker := rowByLine(t, p, 2)
	if stacker.Class != ClassChanged || !reflect.DeepEqual(fieldCodes(stacker), []string{"payload_kg"}) {
		t.Fatalf("stacker: %s %v", stacker.Class, fieldCodes(stacker))
	}
	if d := stacker.Fields[0]; d.Current != 1500.0 || d.File != 2000.0 || d.Conflict {
		t.Fatalf("stacker payload: %+v", d)
	}
	if stacker.Rev == "" {
		t.Fatal("stored robot without rev")
	}
	if cart := rowByLine(t, p, 3); cart.Class != ClassUnchanged {
		t.Fatalf("empty cells must keep stored values: %s %v", cart.Class, fieldCodes(cart))
	}
	fresh := rowByLine(t, p, 4)
	if fresh.Class != ClassNew || fresh.DuplicateOf != nil || fresh.Rev != "" {
		t.Fatalf("new robot: %+v", fresh)
	}
	dup := rowByLine(t, p, 5)
	if dup.Class != ClassNew || dup.DuplicateOf == nil || dup.DuplicateOf.ID != idCart.String() {
		t.Fatalf("same name and vendor must point at the stored robot: %+v", dup)
	}
	broken := rowByLine(t, p, 6)
	if broken.Class != ClassError || len(broken.Errors) != 1 || broken.Errors[0].Field != "payload_kg" {
		t.Fatalf("broken: %+v", broken)
	}
	if len(p.Missing) != 1 || p.Missing[0].ID != idDrone.String() {
		t.Fatalf("missing must list active robots absent from the file: %+v", p.Missing)
	}
	want := map[string]int{ClassChanged: 1, ClassUnchanged: 1, ClassNew: 2, ClassError: 1, "missing": 1}
	if !reflect.DeepEqual(p.Counts, want) {
		t.Fatalf("counts %v, want %v", p.Counts, want)
	}
}

func TestBuildPlanMissingNeedsCatalogModeAndIDs(t *testing.T) {
	g := Grid{{"Название"}, {"Штабелёр"}}
	if p := planOf(t, g, catalogRows(t), nil, ModeCatalog); len(p.Missing) != 0 {
		t.Fatalf("without an id column nothing is missing: %+v", p.Missing)
	}
	g = Grid{{"id", "Название"}, {idStacker.String(), "Штабелёр"}}
	if p := planOf(t, g, catalogRows(t), nil, ModeRobot); len(p.Missing) != 0 {
		t.Fatalf("one-robot upload lists nothing missing: %+v", p.Missing)
	}
}

func TestBuildPlanThreeWay(t *testing.T) {
	rows := catalogRows(t)
	base := SnapshotValues(rows)
	raw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if base, err = DecodeSnapshot(raw); err != nil {
		t.Fatal(err)
	}
	rows[0].PriceRub = numeric(t, 3000000)
	rows[0].NavType = text("Метки")
	rows[1].MinAisleMm = numeric(t, 1800)
	g := Grid{
		{"id", "Название", "Компания", "Грузоподъёмность, кг", "Цена, ₽", "Навигация", "Мин. проход, мм"},
		{idStacker.String(), "Штабелёр", "", "2000", "2 700 000", "QR-коды", ""},
		{idCart.String(), "Тележка", "Ронави", "", "", "", "1 800"},
		{idDrone.String(), "Дрон", "Коптер", "", "", "", ""},
	}
	p := planOf(t, g, rows, base, ModeCatalog)
	if !p.ThreeWay {
		t.Fatal("want three-way")
	}
	stacker := rowByLine(t, p, 2)
	if stacker.Class != ClassConflict {
		t.Fatalf("stacker class %s", stacker.Class)
	}
	got := map[string]FieldDiff{}
	for _, d := range stacker.Fields {
		got[d.Code] = d
	}
	if len(got) != 3 {
		t.Fatalf("stacker fields %v; price changed only in the catalog must be kept", fieldCodes(stacker))
	}
	if d := got["vendor"]; d.File != nil || d.Current != "Ронави" || d.Conflict {
		t.Fatalf("a cell emptied in the file clears the field: %+v", d)
	}
	if d := got["payload_kg"]; d.File != 2000.0 || d.Base != 1500.0 || d.Conflict {
		t.Fatalf("payload changed only in the file: %+v", d)
	}
	if d := got["nav_type"]; !d.Conflict || d.Base != "Лидар" || d.Current != "Метки" || d.File != "QR-коды" {
		t.Fatalf("nav changed on both sides: %+v", d)
	}
	if cart := rowByLine(t, p, 3); cart.Class != ClassUnchanged {
		t.Fatalf("the same edit on both sides is no change: %s %v", cart.Class, fieldCodes(cart))
	}
	if drone := rowByLine(t, p, 4); drone.Class != ClassUnchanged {
		t.Fatalf("drone: %s %v", drone.Class, fieldCodes(drone))
	}
}

func TestBuildPlanChecksMergedValues(t *testing.T) {
	rows := catalogRows(t)
	rows[0].TempMaxC = numeric(t, 10)
	g := Grid{{"id", "Название", "Температура от, °C"}, {idStacker.String(), "Штабелёр", "20"}}
	rp := planOf(t, g, rows, nil, ModeCatalog).Rows[0]
	if rp.Class != ClassError || len(rp.Errors) != 1 || rp.Errors[0].Field != "temp_min_c" {
		t.Fatalf("minimum above the stored maximum: %+v", rp)
	}
	g = Grid{{"Название", "Компания"}, {"", "Ронави"}}
	m := Mapping{0: "name", 1: "vendor"}
	rp = BuildPlan(ReadRows(g, m), rows, nil, ModeCatalog, false).Rows[0]
	if rp.Class != ClassError || rp.Errors[0].Field != "name" {
		t.Fatalf("a new robot needs a name: %+v", rp)
	}
}

func TestReadRowsKeepsEveryOrganizerLine(t *testing.T) {
	g := Grid{
		{"id", "Название", "Отрасль", "Сценарий", "Кейсы", "Цена изделия"},
		{idStacker.String(), "Штабелёр", "Логистика", "Паллеты", "Склад А", "2 700 000,00"},
		{idStacker.String(), "Штабелёр", "Ритейл", "Короба", "Склад Б", ""},
		{idCart.String(), "Тележка", "Медицина", "Лекарства", "", ""},
		{"", "", "", "", "", ""},
	}
	rows := ReadRows(g, AutoMapping(g.Header()))
	if len(rows) != 3 {
		t.Fatalf("rows %d, want 3", len(rows))
	}
	price := 2700000.0
	first, _ := rows[0].Values[UsesKey].([]CatalogUse)
	if !reflect.DeepEqual(first, []CatalogUse{{Industry: "Логистика", Scenario: "Паллеты", Cases: "Склад А", PriceRub: &price}}) {
		t.Fatalf("first uses %+v", first)
	}
	second, _ := rows[1].Values[UsesKey].([]CatalogUse)
	if !reflect.DeepEqual(second, []CatalogUse{{Industry: "Ритейл", Scenario: "Короба", Cases: "Склад Б"}}) {
		t.Fatalf("second uses %+v", second)
	}
	if rows[0].Values["industry"] != "Логистика" || rows[0].Values["price_rub"] != 2700000.0 || rows[1].Values["industry"] != "Ритейл" {
		t.Fatalf("each line keeps its own values: %+v %+v", rows[0].Values, rows[1].Values)
	}
	if rows[2].Line != 4 {
		t.Fatalf("line %d, want 4", rows[2].Line)
	}
}

func TestSplitRowIDs(t *testing.T) {
	line := func(n int, industry string) FileRow {
		return FileRow{Line: n, ID: idStacker.String(), Values: Values{"industry": industry, UsesKey: []CatalogUse{{Industry: industry}}}}
	}
	retail := SplitID(idStacker, "Ритейл").String()
	logistics := SplitID(idStacker, "Логистика").String()
	tests := []struct {
		name   string
		stored func(string) (string, bool)
		file   []FileRow
		want   []string
	}{
		{"nothing stored: the first line keeps the id", func(string) (string, bool) { return "", false },
			[]FileRow{line(2, "Логистика"), line(3, "Ритейл")}, []string{idStacker.String(), retail}},
		{"the stored industry keeps the id wherever it stands", func(string) (string, bool) { return "Ритейл", true },
			[]FileRow{line(2, "Логистика"), line(3, "Ритейл")}, []string{logistics, idStacker.String()}},
		{"no line of the stored industry: none keeps the id", func(string) (string, bool) { return "Медицина", true },
			[]FileRow{line(2, "Логистика"), line(3, "Ритейл")}, []string{logistics, retail}},
		{"a single line is left alone", func(string) (string, bool) { return "Ритейл", true },
			[]FileRow{line(2, "Логистика")}, []string{idStacker.String()}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitRowIDs(tc.file, tc.stored)
			ids := make([]string, len(got))
			for i, fr := range got {
				ids[i] = fr.ID
			}
			if !reflect.DeepEqual(ids, tc.want) {
				t.Fatalf("ids %v, want %v", ids, tc.want)
			}
		})
	}
	same := SplitRowIDs([]FileRow{line(2, "Ритейл"), line(3, "ритейл")}, func(string) (string, bool) { return "", false })
	if len(same[0].Errors) != 0 || len(same[1].Errors) != 1 || same[1].ID != SplitID(idStacker, "Ритейл").String() {
		t.Fatalf("a repeated industry must be refused: %+v", same)
	}
}

func TestBuildPlanMatchesRepeatedLinesToTheirSolutions(t *testing.T) {
	retail := stored(SplitID(idStacker, "Ритейл"), "Штабелёр", "Ронави")
	retail.Industry = text("Ритейл")
	retail.Raw = json.RawMessage(`{"uses":[{"industry":"Ритейл","scenario":"","cases":""}]}`)
	logistics := stored(idStacker, "Штабелёр", "Ронави")
	logistics.Industry = text("Логистика")
	logistics.Raw = json.RawMessage(`{"uses":[{"industry":"Логистика","scenario":"","cases":""}]}`)
	g := Grid{
		{"id", "Название", "Отрасль", "Кейсы"},
		{idStacker.String(), "Штабелёр", "Ритейл", ""},
		{idStacker.String(), "Штабелёр", "Логистика", ""},
	}
	p := planOf(t, g, []db.ListSolutionsRow{logistics, retail}, nil, ModeCatalog)
	for _, rp := range p.Rows {
		if rp.Class != ClassUnchanged {
			t.Fatalf("line %d: %s %v", rp.Line, rp.Class, fieldCodes(rp))
		}
	}
	if len(p.Missing) != 0 {
		t.Fatalf("missing %+v", p.Missing)
	}
}

func TestReadRowsRefusesRepeatedID(t *testing.T) {
	g := Grid{
		{"id", "Название"},
		{idStacker.String(), "Штабелёр"},
		{idStacker.String(), "Штабелёр 2"},
	}
	rows := ReadRows(g, AutoMapping(g.Header()))
	if len(rows) != 2 || len(rows[1].Errors) != 1 || rows[1].Errors[0].Field != "id" {
		t.Fatalf("rows %+v", rows)
	}
}

func TestMappingCheck(t *testing.T) {
	tests := []struct {
		m     Mapping
		width int
		ok    bool
	}{
		{Mapping{0: "name"}, 1, true},
		{Mapping{0: "id", 1: "payload_kg"}, 2, true},
		{Mapping{0: "payload_kg"}, 1, false},
		{Mapping{0: "name", 1: "name"}, 2, false},
		{Mapping{0: "name", 3: "vendor"}, 2, false},
		{Mapping{0: "name", 1: "colour"}, 2, false},
	}
	for _, tc := range tests {
		if err := tc.m.Check(tc.width); (err == nil) != tc.ok {
			t.Errorf("Check(%v, %d) = %v, want ok %v", tc.m, tc.width, err, tc.ok)
		}
	}
	if err := (Mapping{0: "payload_kg"}).Check(1); !errors.Is(err, ErrNoNameColumn) {
		t.Errorf("mapping without a name: %v", err)
	}
	m := AutoMapping([]string{"Название", "Примечание", "наименование", "Тип", "тип"})
	if !reflect.DeepEqual(m, Mapping{0: "name", 3: "family", 4: "kind"}) {
		t.Fatalf("auto mapping %v", m)
	}
}

func TestFormToTable(t *testing.T) {
	g := Grid{
		{"Поле", "Значение", "Единица", "Допустимые значения"},
		{"Название", "Штабелёр", "", ""},
		{"", "", "", ""},
		{"Грузоподъёмность", "1 500", "кг", "Число"},
	}
	if SheetLayout(g) != LayoutForm || SheetLayout(Grid{{"Название"}}) != LayoutTable {
		t.Fatal("layout")
	}
	want := Grid{{"Название", "Грузоподъёмность"}, {"Штабелёр", "1 500"}}
	if got := FormToTable(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("table %v", got)
	}
}
