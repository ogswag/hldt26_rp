package importers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"moscow_hackathon_2026/api/internal/catalog"
	"moscow_hackathon_2026/api/internal/db"
)

// Result reports what a startup seed did.
type Result struct {
	CatalogInserted int
	// SplitInserted counts rows of a repeated id added to a catalog that was seeded before rows became solutions.
	SplitInserted int
	ExtraInserted int
	EditedSkipped int
	Inventory     Inventory
}

type Inventory struct {
	DataRows       int
	UniqueIDs      int
	DuplicatedIDs  int
	ExtraRows      int
	PriceConflicts int
}

type CatalogUse struct {
	Industry string   `json:"industry"`
	Scenario string   `json:"scenario"`
	Cases    string   `json:"cases"`
	PriceRub *float64 `json:"price_rub"`
}

// CatalogRecord is one data row of the organizer file. A row whose id was seen before gets its own solution id
// (SplitID), so every row is a solution and none is merged into another.
type CatalogRecord struct {
	ID uuid.UUID
	// SourceID is the id the file delivered; it equals ID for the first row of an id.
	SourceID uuid.UUID
	// Grouped marks a row whose id the file repeats.
	Grouped bool
	Row     map[string]string
	Uses    []CatalogUse
}

// splitNamespace is the UUID namespace of the ids that repeated rows get.
var splitNamespace = uuid.MustParse("6f1d6b1e-7c1a-4d0e-9a55-0c3f5e2b8a17")

// SplitID is the solution id of a row that repeats a source id. It depends only on the id and the industry, so it
// does not move with the order of the file.
func SplitID(source uuid.UUID, industry string) uuid.UUID {
	return uuid.NewSHA1(splitNamespace, []byte(source.String()+"|"+headerKey(industry)))
}

// Seed fills an empty catalog from the organizer file and then fills gaps from the seed files. It never overwrites
// a stored value and leaves robots an admin has edited alone.
func Seed(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, csvPath, specsPath string) (Result, error) {
	records, err := ReadCatalogCSV(csvPath)
	if err != nil {
		return Result{}, err
	}
	doc, err := LoadSpecsFile(specsPath)
	if err != nil {
		return Result{}, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("importers.begin: %w", err)
	}
	defer tx.Rollback(ctx)
	qt := q.WithTx(tx)

	out := Result{Inventory: InventoryFromRecords(records)}
	n, err := qt.CountSolutions(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("importers.count: %w", err)
	}
	if n == 0 {
		if out.CatalogInserted, err = insertRecords(ctx, qt, records); err != nil {
			return Result{}, err
		}
	} else if out.SplitInserted, err = alignSplitRows(ctx, qt, records); err != nil {
		return Result{}, err
	}
	ids, err := qt.ListEditedSolutionIDs(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("importers.edited: %w", err)
	}
	edited := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		edited[id] = true
	}
	if err := fillUses(ctx, qt, records, edited); err != nil {
		return Result{}, err
	}
	if out.ExtraInserted, err = seedSpecs(ctx, qt, doc, edited, splitSiblings(records)); err != nil {
		return Result{}, err
	}
	out.EditedSkipped = len(ids)
	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("importers.commit: %w", err)
	}
	return out, nil
}

func ReadCatalogCSV(path string) ([]CatalogRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("importers.open: %w", err)
	}
	defer f.Close()
	grid, err := ReadCSVGrid(f)
	if err != nil {
		return nil, err
	}
	if len(grid) == 0 {
		return nil, fmt.Errorf("importers.header: empty file")
	}
	header := grid.Header()

	norm, err := sharedText()
	if err != nil {
		return nil, err
	}
	out := make([]CatalogRecord, 0, len(grid)-1)
	first := make(map[uuid.UUID]int)
	taken := make(map[uuid.UUID]int)
	for i, rec := range grid[1:] {
		line := i + 2
		row := mapRow(header, rec)
		for _, col := range textColumns {
			if v, ok := row[col]; ok {
				row[col] = norm.Text(v)
			}
		}
		source, err := uuid.Parse(strings.TrimSpace(row["id"]))
		if err != nil {
			return nil, fmt.Errorf("importers.row %d: invalid id: %w", line, err)
		}
		name := strings.TrimSpace(row["Название"])
		if name == "" {
			return nil, fmt.Errorf("importers.row %d: empty name", line)
		}
		price, err := ParsePrice(row["Цена изделия"])
		if err != nil {
			return nil, fmt.Errorf("importers.row %d: price: %w", line, err)
		}
		use := CatalogUse{
			Industry: strings.TrimSpace(row["Отрасль"]),
			Scenario: strings.TrimSpace(row["Сценарий"]),
			Cases:    strings.TrimSpace(row["Кейсы"]),
			PriceRub: price,
		}
		rc := CatalogRecord{ID: source, SourceID: source, Row: row, Uses: []CatalogUse{use}}
		if at, seen := first[source]; seen {
			rc.ID = SplitID(source, use.Industry)
			rc.Grouped = true
			out[at].Grouped = true
			if prev, dup := taken[rc.ID]; dup {
				return nil, fmt.Errorf("importers.row %d: repeats id %s and industry %q of row %d", line, source, use.Industry, prev)
			}
		} else {
			first[source] = len(out)
		}
		taken[rc.ID] = line
		out = append(out, rc)
	}
	return out, nil
}

func insertRecords(ctx context.Context, q *db.Queries, records []CatalogRecord) (int, error) {
	inserted := 0
	for _, rec := range records {
		p, err := recordToSolution(rec)
		if err != nil {
			return 0, err
		}
		n, err := q.InsertSolution(ctx, p)
		if err != nil {
			return 0, fmt.Errorf("importers.insert %s: %w", rec.ID, err)
		}
		if err := q.InsertSolutionSpecs(ctx, rec.ID); err != nil {
			return 0, fmt.Errorf("importers.specs %s: %w", rec.ID, err)
		}
		inserted += int(n)
	}
	return inserted, nil
}

func recordToSolution(rec CatalogRecord) (db.InsertSolutionParams, error) {
	row := rec.Row
	name := strings.TrimSpace(row["Название"])
	price, err := ParsePrice(row["Цена изделия"])
	if err != nil {
		return db.InsertSolutionParams{}, fmt.Errorf("importers.insert %s: price: %w", rec.ID, err)
	}
	rawMap := make(map[string]any, len(row)+1)
	for k, v := range row {
		rawMap[k] = v
	}
	rawMap["uses"] = rec.Uses
	if rec.Grouped {
		rawMap["source_id"] = rec.SourceID.String()
	}
	raw, err := json.Marshal(rawMap)
	if err != nil {
		return db.InsertSolutionParams{}, fmt.Errorf("importers.raw %s: %w", rec.ID, err)
	}
	priceNum, err := numericFromFloat(price)
	if err != nil {
		return db.InsertSolutionParams{}, fmt.Errorf("importers.insert %s: price: %w", rec.ID, err)
	}
	return db.InsertSolutionParams{
		ID:       rec.ID,
		Name:     name,
		Vendor:   optString(row["компания"]),
		Kind:     optString(row["тип"]),
		Subtype:  optString(row["Подтип"]),
		Status:   optString(row["статус"]),
		Industry: optString(row["Отрасль"]),
		Scenario: optString(row["Сценарий"]),
		PriceRub: priceNum,
		Raw:      raw,
		Family:   optString(catalog.FamilyCode(row["Тип"], row["тип"])),
	}, nil
}

// fillUses stores the organizer's uses and first price where a robot has none.
func fillUses(ctx context.Context, q *db.Queries, records []CatalogRecord, edited map[uuid.UUID]bool) error {
	for _, rec := range records {
		if edited[rec.ID] {
			continue
		}
		raw, err := json.Marshal(rec.Uses)
		if err != nil {
			return fmt.Errorf("importers.uses %s: %w", rec.ID, err)
		}
		if err := q.FillSolutionUses(ctx, db.FillSolutionUsesParams{ID: rec.ID, Replacement: raw}); err != nil {
			return fmt.Errorf("importers.uses %s: %w", rec.ID, err)
		}
		var price *float64
		if len(rec.Uses) > 0 {
			price = rec.Uses[0].PriceRub
		}
		if price == nil {
			continue
		}
		priceNum, err := numericFromFloat(price)
		if err != nil {
			return fmt.Errorf("importers.price %s: %w", rec.ID, err)
		}
		if err := q.FillSolutionPrice(ctx, db.FillSolutionPriceParams{ID: rec.ID, PriceRub: priceNum}); err != nil {
			return fmt.Errorf("importers.price %s: %w", rec.ID, err)
		}
		if err := fillFieldSources(ctx, q, rec.ID, map[string]catalog.FieldSource{"price_rub": {Note: "Цена из каталога организаторов."}}); err != nil {
			return err
		}
	}
	return nil
}

// alignSplitRows brings a catalog seeded while repeated ids were merged up to date: it adds the row of each repeated
// id that is missing and cuts the uses of the first row down to its own. Robots an admin edited keep their uses.
func alignSplitRows(ctx context.Context, q *db.Queries, records []CatalogRecord) (int, error) {
	added := 0
	for _, rec := range records {
		if !rec.Grouped {
			continue
		}
		if rec.ID == rec.SourceID {
			uses, err := json.Marshal(rec.Uses)
			if err != nil {
				return 0, fmt.Errorf("importers.align %s: %w", rec.ID, err)
			}
			if err := q.CutSolutionUses(ctx, db.CutSolutionUsesParams{ID: rec.ID, Uses: uses, SourceID: rec.SourceID.String()}); err != nil {
				return 0, fmt.Errorf("importers.align %s: %w", rec.ID, err)
			}
			continue
		}
		p, err := recordToSolution(rec)
		if err != nil {
			return 0, err
		}
		n, err := q.InsertSolution(ctx, p)
		if err != nil {
			return 0, fmt.Errorf("importers.align %s: %w", rec.ID, err)
		}
		if n == 0 {
			continue
		}
		if err := q.InsertSolutionSpecs(ctx, rec.ID); err != nil {
			return 0, fmt.Errorf("importers.align.specs %s: %w", rec.ID, err)
		}
		added++
	}
	return added, nil
}

// splitSiblings maps a source id to the ids of the other rows that repeat it.
func splitSiblings(records []CatalogRecord) map[uuid.UUID][]uuid.UUID {
	out := make(map[uuid.UUID][]uuid.UUID)
	for _, rec := range records {
		if rec.ID != rec.SourceID {
			out[rec.SourceID] = append(out[rec.SourceID], rec.ID)
		}
	}
	return out
}

// InventoryFromRecords counts the rows of the file, its ids and the ids it repeats.
func InventoryFromRecords(records []CatalogRecord) Inventory {
	inv := Inventory{DataRows: len(records)}
	byID := make(map[uuid.UUID][]int, len(records))
	for i, rec := range records {
		byID[rec.SourceID] = append(byID[rec.SourceID], i)
	}
	inv.UniqueIDs = len(byID)
	for _, rows := range byID {
		if len(rows) < 2 {
			continue
		}
		inv.DuplicatedIDs++
		inv.ExtraRows += len(rows) - 1
		first := records[rows[0]].Uses[0].PriceRub
		for _, r := range rows[1:] {
			if priceConflict(first, records[r].Uses[0].PriceRub) {
				inv.PriceConflicts++
				break
			}
		}
	}
	return inv
}

func priceConflict(a, b *float64) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil || b == nil {
		return true
	}
	return *a != *b
}

func FormatInventory(inv Inventory) string {
	return fmt.Sprintf(
		"CSV: %d rows, %d unique ids, %d repeated ids (%d extra rows, each kept as its own solution). Price conflicts: %d.",
		inv.DataRows, inv.UniqueIDs, inv.DuplicatedIDs, inv.ExtraRows, inv.PriceConflicts,
	)
}

func ParsePrice(s string) (*float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		switch r {
		case ',', '.':
			b.WriteByte('.')
		case ' ', '\u00a0', '\u202f':
		default:
			return nil, fmt.Errorf("unexpected %q", string(r))
		}
	}
	v, err := strconv.ParseFloat(b.String(), 64)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// textColumns are the catalog columns written for people; codes, numbers and links are left as delivered.
var textColumns = []string{"Название", "компания", "описание", "Тип", "Подтип", "Сценарий", "Кейсы", "Регион", "Отрасль"}

func mapRow(header, rec []string) map[string]string {
	out := make(map[string]string, len(header))
	for i, h := range header {
		if i < len(rec) {
			out[h] = rec[i]
		} else {
			out[h] = ""
		}
	}
	return out
}

func optString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func numericFromFloat(p *float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if p == nil {
		return n, nil
	}
	s := strconv.FormatFloat(*p, 'f', -1, 64)
	if err := n.Scan(s); err != nil {
		return n, err
	}
	return n, nil
}
