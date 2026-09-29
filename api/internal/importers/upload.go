package importers

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// Upload modes: one robot from the vertical template, or a catalog table that adds and updates robots.
const (
	ModeRobot   = "robot"
	ModeCatalog = "catalog"
)

// Sheet layouts: a table with a robot per row, or the one-robot form with a field per row.
const (
	LayoutTable = "table"
	LayoutForm  = "form"
)

// ErrNoNameColumn is a mapping that finds robots neither by name nor by id. It is a step of editing a mapping, so
// it is kept and reported rather than refused.
var ErrNoNameColumn = errors.New("выберите колонку с названием решения")

// Mapping maps a column index to a field code. Columns it leaves out are not loaded.
type Mapping map[int]string

// AutoMapping maps every header that names a field; a field named twice keeps its first column.
func AutoMapping(header []string) Mapping {
	m := Mapping{}
	taken := map[string]bool{}
	for i, h := range header {
		f, ok := FieldForHeader(h)
		if !ok || taken[f.Code] {
			continue
		}
		m[i] = f.Code
		taken[f.Code] = true
	}
	return m
}

// Columns lists mapped column indexes in order.
func (m Mapping) Columns() []int {
	cols := make([]int, 0, len(m))
	for c := range m {
		cols = append(cols, c)
	}
	slices.Sort(cols)
	return cols
}

// Has reports whether some column is mapped to the field.
func (m Mapping) Has(code string) bool {
	for _, c := range m {
		if c == code {
			return true
		}
	}
	return false
}

// Check rejects a mapping with unknown fields, a field mapped twice or no name column.
func (m Mapping) Check(width int) error {
	seen := map[string]bool{}
	for col, code := range m {
		if col < 0 || col >= width {
			return fmt.Errorf("в файле нет колонки %d", col+1)
		}
		f, ok := FieldByCode(code)
		if !ok {
			return fmt.Errorf("неизвестное поле. Выберите поле из списка")
		}
		if seen[code] {
			return fmt.Errorf("поле «%s» выбрано для двух колонок. Оставьте одну", f.Label)
		}
		seen[code] = true
	}
	if !seen["name"] && !seen["id"] {
		return ErrNoNameColumn
	}
	return nil
}

// SheetLayout tells the one-robot form, whose header is «Поле | Значение», from a table.
func SheetLayout(g Grid) string {
	h := g.Header()
	if len(h) >= 2 && headerKey(h[0]) == "поле" && headerKey(h[1]) == "значение" {
		return LayoutForm
	}
	return LayoutTable
}

// FormToTable turns the one-robot form into a table: field labels become the header, values the only row.
func FormToTable(g Grid) Grid {
	var header, values []string
	for i := 1; i < len(g); i++ {
		label := strings.TrimSpace(g.Cell(i, 0))
		if label == "" {
			continue
		}
		header = append(header, label)
		values = append(values, g.Cell(i, 1))
	}
	return Grid{header, values}
}

// FileRow is one robot read from a sheet.
type FileRow struct {
	// Line is the sheet line of the robot's first row; the header is line 1.
	Line   int
	ID     string
	Stamp  string
	Photo  string
	Values Values
	Errors []FieldError
}

// ReadRows parses the data rows of a table. A mapped empty cell gives a nil value, an unmapped column no key. A
// table with the organizer's «Кейсы» column lists a robot once per use: each line is a solution of its own, and
// BuildPlan gives the lines of a repeated id their ids (SplitRowIDs).
func ReadRows(g Grid, m Mapping) []FileRow {
	organizer := m.Has("cases")
	cols := m.Columns()
	var out []FileRow
	byID := map[string]int{}
	for i := 1; i < len(g); i++ {
		empty := true
		for _, c := range cols {
			if strings.TrimSpace(g.Cell(i, c)) != "" {
				empty = false
				break
			}
		}
		if empty {
			continue
		}
		row := FileRow{Line: i + 1, Values: Values{}}
		var cases string
		for _, c := range cols {
			f, _ := FieldByCode(m[c])
			v, err := f.Parse(g.Cell(i, c))
			if err != nil {
				var fe *FieldError
				if !errors.As(err, &fe) {
					fe = f.fail("значение не читается.")
				}
				row.Errors = append(row.Errors, *fe)
				continue
			}
			s, _ := v.(string)
			switch f.Code {
			case "id":
				row.ID = s
			case StampField:
				row.Stamp = s
			case "photo":
				row.Photo = s
			case "cases":
				cases = s
			default:
				row.Values[f.Code] = v
			}
		}
		if organizer {
			use := CatalogUse{Cases: cases}
			use.Industry, _ = row.Values["industry"].(string)
			use.Scenario, _ = row.Values["scenario"].(string)
			if p, ok := row.Values["price_rub"].(float64); ok {
				use.PriceRub = &p
			}
			row.Values[UsesKey] = []CatalogUse{use}
		} else if at, ok := byID[row.ID]; ok && row.ID != "" {
			row.Errors = append(row.Errors, FieldError{Field: "id", Message: fmt.Sprintf(
				"Строка повторяет решение из строки %d. Оставьте одну строку на решение.", out[at].Line)})
		}
		if row.ID != "" {
			if _, ok := byID[row.ID]; !ok {
				byID[row.ID] = len(out)
			}
		}
		out = append(out, row)
	}
	return out
}

// SplitRowIDs gives every line of an id the file repeats its own solution id, the way the catalog seed does: the line
// of the industry the stored robot has keeps the id, the others get SplitID. Without a stored robot the first line
// keeps it. A line that repeats the id and industry of an earlier one is an error.
func SplitRowIDs(file []FileRow, industryOf func(id string) (string, bool)) []FileRow {
	groups := map[string][]int{}
	for i, fr := range file {
		if _, ok := fr.Values[UsesKey]; ok && fr.ID != "" {
			groups[fr.ID] = append(groups[fr.ID], i)
		}
	}
	out := append([]FileRow(nil), file...)
	for id, at := range groups {
		if len(at) < 2 {
			continue
		}
		source, err := uuid.Parse(id)
		if err != nil {
			continue
		}
		keeper := at[0]
		if stored, ok := industryOf(id); ok {
			keeper = -1
			for _, i := range at {
				if headerKey(useIndustry(file[i])) == headerKey(stored) {
					keeper = i
					break
				}
			}
		}
		seen := map[string]int{}
		for _, i := range at {
			fr := out[i]
			fr.Values = cloneValues(fr.Values)
			industry := headerKey(useIndustry(fr))
			if i != keeper {
				fr.ID = SplitID(source, useIndustry(fr)).String()
			}
			if prev, dup := seen[industry]; dup {
				fr.Errors = append(append([]FieldError(nil), fr.Errors...), FieldError{Field: "id", Message: fmt.Sprintf(
					"Строка повторяет идентификатор и отрасль строки %d. Оставьте одну строку на отрасль.", file[prev].Line)})
			}
			seen[industry] = i
			out[i] = fr
		}
	}
	return out
}

func useIndustry(fr FileRow) string {
	uses, _ := fr.Values[UsesKey].([]CatalogUse)
	if len(uses) == 0 {
		return ""
	}
	return uses[0].Industry
}
