package importers

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestNumericFromFloat(t *testing.T) {
	v := 2700000.0
	n, err := numericFromFloat(&v)
	if err != nil {
		t.Fatal(err)
	}
	if !n.Valid {
		t.Fatal("want valid numeric")
	}
	got, err := n.Float64Value()
	if err != nil || !got.Valid || got.Float64 != v {
		t.Fatalf("got %+v err %v", got, err)
	}
	half := 1.5
	n, err = numericFromFloat(&half)
	if err != nil {
		t.Fatal(err)
	}
	got, err = n.Float64Value()
	if err != nil || !got.Valid || got.Float64 != half {
		t.Fatalf("1.5: got %+v err %v", got, err)
	}
	n, err = numericFromFloat(nil)
	if err != nil || n.Valid {
		t.Fatalf("nil: valid=%v err=%v", n.Valid, err)
	}
}

func TestParsePrice(t *testing.T) {
	tests := []struct {
		in      string
		want    float64
		wantNil bool
		wantErr bool
	}{
		{in: "2 700 000,00", want: 2700000},
		{in: "950 000,00", want: 950000},
		{in: "", wantNil: true},
		{in: "  ", wantNil: true},
		{in: "abc", wantErr: true},
	}
	for _, tc := range tests {
		got, err := ParsePrice(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("ParsePrice(%q): want error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParsePrice(%q): %v", tc.in, err)
		}
		if tc.wantNil {
			if got != nil {
				t.Fatalf("ParsePrice(%q): want nil, got %v", tc.in, *got)
			}
			continue
		}
		if got == nil || *got != tc.want {
			t.Fatalf("ParsePrice(%q): got %v want %v", tc.in, got, tc.want)
		}
	}
}

func TestReadCatalogCSVKeepsEveryRow(t *testing.T) {
	path := filepath.Join("testdata", "catalog_dups.csv")
	recs, err := ReadCatalogCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("rows: got %d want 3", len(recs))
	}
	h1500 := recs[0]
	if h1500.ID.String() != "5760e938-9a43-45a7-b8e8-f4f2e6383930" || h1500.ID != h1500.SourceID {
		t.Fatalf("the first row keeps the delivered id: %s", h1500.ID)
	}
	if len(h1500.Uses) != 1 || h1500.Uses[0].Industry != "Торговля и услуги" || h1500.Uses[0].Scenario != "Внутрискладская логистика" {
		t.Fatalf("first use: %+v", h1500.Uses)
	}
	if h1500.Row["Отрасль"] != "Торговля и услуги" || !h1500.Grouped {
		t.Fatalf("first row %q grouped %v", h1500.Row["Отрасль"], h1500.Grouped)
	}
	second := recs[1]
	if second.SourceID != h1500.SourceID || second.ID == second.SourceID || !second.Grouped {
		t.Fatalf("second row %+v", second)
	}
	if second.ID != SplitID(h1500.SourceID, "Промышленность") {
		t.Fatalf("second row id %s", second.ID)
	}
	if len(second.Uses) != 1 || second.Uses[0].Industry != "Промышленность" || second.Uses[0].Scenario != "Внутрипроизводственная логистика" {
		t.Fatalf("second use: %+v", second.Uses)
	}
	solo := recs[2]
	if solo.Grouped || solo.ID != solo.SourceID || solo.Uses[0].PriceRub == nil || *solo.Uses[0].PriceRub != 1000000 {
		t.Fatalf("solo row %+v", solo)
	}
}

func TestSplitIDDoesNotDependOnOrder(t *testing.T) {
	src := uuid.MustParse("5760e938-9a43-45a7-b8e8-f4f2e6383930")
	if SplitID(src, "Промышленность") != SplitID(src, " промышленность ") {
		t.Fatal("case and spaces must not change the id")
	}
	if SplitID(src, "Промышленность") == SplitID(src, "Торговля и услуги") {
		t.Fatal("two industries share an id")
	}
	other := uuid.MustParse("8f3c1e20-6d4a-4b91-9c2e-7b0a11d4e8f2")
	if SplitID(src, "Промышленность") == SplitID(other, "Промышленность") {
		t.Fatal("two robots share an id")
	}
}

func TestInventoryFromRecords(t *testing.T) {
	path := filepath.Join("testdata", "catalog_dups.csv")
	recs, err := ReadCatalogCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	inv := InventoryFromRecords(recs)
	if inv.DataRows != 3 || inv.UniqueIDs != 2 || inv.DuplicatedIDs != 1 || inv.ExtraRows != 1 {
		t.Fatalf("inventory %+v", inv)
	}
	if inv.PriceConflicts != 0 {
		t.Fatalf("price conflicts %d", inv.PriceConflicts)
	}
}

func TestCatalogCSVInventory(t *testing.T) {
	path := filepath.Join("..", "..", "..", "data", "catalog.csv")
	recs, err := ReadCatalogCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	inv := InventoryFromRecords(recs)
	if inv.DataRows != 223 || inv.UniqueIDs != 187 || inv.DuplicatedIDs != 23 || inv.ExtraRows != 36 || inv.PriceConflicts != 3 {
		t.Fatalf("catalog inventory %+v want 223/187/23/36, 3 price conflicts", inv)
	}
	ids := map[uuid.UUID]bool{}
	for _, r := range recs {
		if ids[r.ID] {
			t.Fatalf("two rows share solution id %s", r.ID)
		}
		ids[r.ID] = true
	}
	if len(ids) != 223 {
		t.Fatalf("solution ids %d want 223", len(ids))
	}
	if got := FormatInventory(inv); got == "" {
		t.Fatal("empty report")
	}
}

func TestLoadSpecsFile(t *testing.T) {
	path := filepath.Join("..", "..", "..", "data", "seeds", "robot_specs.json")
	doc, err := LoadSpecsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Robots) != 58 {
		t.Fatalf("robots: got %d want 58", len(doc.Robots))
	}
	described := 0
	for _, r := range doc.Robots {
		if r.SourceURL == "" && r.Confidence == "vendor" && r.InCatalog {
			t.Errorf("%s: vendor values without a source link", r.Name)
		}
		if r.Description != "" {
			described++
		}
	}
	if described != 23 {
		t.Fatalf("descriptions: got %d want 23", described)
	}
	var h1500 *robotSeed
	var pudu *robotSeed
	for i := range doc.Robots {
		r := &doc.Robots[i]
		if r.ID == "5760e938-9a43-45a7-b8e8-f4f2e6383930" {
			h1500 = r
		}
		if r.ID == "8f3c1e20-6d4a-4b91-9c2e-7b0a11d4e8f2" {
			pudu = r
		}
	}
	if h1500 == nil || h1500.Specs.PayloadKg == nil || *h1500.Specs.PayloadKg != 1500 {
		t.Fatalf("H1500 payload: %+v", h1500)
	}
	if h1500.Confidence != "vendor" {
		t.Fatalf("H1500 confidence %q", h1500.Confidence)
	}
	if len(h1500.ObjectTypes) != 2 {
		t.Fatalf("H1500 object_types %v", h1500.ObjectTypes)
	}
	if h1500.Specs.MinAisleMm == nil || *h1500.Specs.MinAisleMm != 750 {
		t.Fatalf("H1500 aisle from the vendor page: %+v", h1500.Specs.MinAisleMm)
	}
	if pudu == nil || pudu.InCatalog || pudu.Solution == nil {
		t.Fatalf("PuduBot 2 extra row: %+v", pudu)
	}
	if pudu.Specs.MinAisleMm == nil || *pudu.Specs.MinAisleMm != 800 {
		t.Fatalf("PuduBot aisle: %+v", pudu.Specs.MinAisleMm)
	}
}
