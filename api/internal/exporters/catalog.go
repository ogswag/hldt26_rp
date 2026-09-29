package exporters

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/importers"
	"moscow_hackathon_2026/api/internal/rutext"
)

// Catalog file formats.
const (
	FormatXLSX = "xlsx"
	FormatCSV  = "csv"
)

const (
	catalogSheet = "Каталог"
	helpSheet    = "Справка"
	formSheet    = "Решение"
	errorsHeader = "Ошибки"
)

// catalogColumns are the columns of a catalog file: the id, every field a person edits, the export stamp.
func catalogColumns(stamp bool) []importers.Field {
	var out []importers.Field
	for _, f := range importers.Fields() {
		if f.Code == "id" || f.Editable || (stamp && f.Code == importers.StampField) {
			out = append(out, f)
		}
	}
	return out
}

// templateColumns are the columns a person fills in a template: the catalog columns and a photo link.
func templateColumns() []importers.Field {
	var out []importers.Field
	for _, f := range importers.Fields() {
		if f.Code == "id" || f.Editable || f.Code == "photo" {
			out = append(out, f)
		}
	}
	return out
}

// CatalogFile writes the catalog with one robot per row. The export id goes into the «Выгрузка» column (hidden in
// XLSX), so an upload of the edited file can tell edits in the file from edits made in the catalog since.
func CatalogFile(rows []db.ListSolutionsRow, exportID, format string) ([]byte, error) {
	cols := catalogColumns(true)
	table := make([][]any, 0, len(rows))
	for _, row := range rows {
		v := importers.RowValues(row)
		line := make([]any, len(cols))
		for i, f := range cols {
			switch f.Code {
			case "id":
				line[i] = row.ID.String()
			case importers.StampField:
				line[i] = exportID
			default:
				line[i] = cellValue(f, v[f.Code])
			}
		}
		table = append(table, line)
	}
	if format == FormatCSV {
		return csvTable(headers(cols), table)
	}
	return xlsxTable(cols, table)
}

// CatalogTemplate writes an empty file to fill: the catalog table with a «Справка» sheet, or the one-robot form
// with a field per row.
func CatalogTemplate(layout, format string) ([]byte, error) {
	cols := templateColumns()
	if layout == importers.LayoutForm {
		rows := [][]any{}
		for _, f := range cols {
			rows = append(rows, []any{f.Label, "", unitText(f), allowedText(f)})
		}
		head := []string{"Поле", "Значение", "Единица", "Допустимые значения"}
		if format == FormatCSV {
			return csvTable(head, rows)
		}
		return xlsxForm(head, cols, rows)
	}
	if format == FormatCSV {
		return csvTable(headers(cols), nil)
	}
	return xlsxTable(cols, nil)
}

// ErrorReport writes an uploaded sheet back with an «Ошибки» column; messages are keyed by sheet line, the header
// being line 1.
func ErrorReport(g importers.Grid, messages map[int][]string, format string) ([]byte, error) {
	width := g.Width()
	head := make([]string, width+1)
	copy(head, g.Header())
	head[width] = errorsHeader
	rows := make([][]any, 0, len(g))
	for i := 1; i < len(g); i++ {
		line := make([]any, width+1)
		for j := 0; j < width; j++ {
			line[j] = g.Cell(i, j)
		}
		line[width] = strings.Join(messages[i+1], " ")
		rows = append(rows, line)
	}
	if format == FormatCSV {
		return csvTable(head, rows)
	}
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	x := &sheetWriter{f: f, name: errorsHeader}
	f.SetSheetName("Sheet1", x.name)
	x.header(head)
	x.rows(rows, nil)
	x.widths(func(int) float64 { return 20 })
	return x.bytes()
}

func headers(cols []importers.Field) []string {
	out := make([]string, len(cols))
	for i, f := range cols {
		out[i] = f.Header()
	}
	return out
}

// cellValue is what an XLSX cell holds: numbers stay numbers (shares in percent), codes become Russian labels.
func cellValue(f importers.Field, v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case float64:
		if f.Percent {
			return math.Round(x*100*1e6) / 1e6
		}
		return x
	}
	return f.Format(v)
}

func unitText(f importers.Field) string {
	return f.Unit
}

// allowedText says in Russian what a field accepts.
func allowedText(f importers.Field) string {
	switch f.Kind {
	case importers.FieldID:
		if f.Code == "id" {
			return "Идентификатор из выгрузки каталога. Для нового решения оставьте пустым."
		}
		return ""
	case importers.FieldNumber:
		lo, hi := *f.Min, *f.Max
		if f.Percent {
			lo, hi = lo*100, hi*100
		}
		kind := "Число"
		if f.Integer {
			kind = "Целое число"
		}
		return fmt.Sprintf("%s от %s до %s.", kind, rutext.Num(lo, 2), rutext.Num(hi, 2))
	case importers.FieldChoice:
		return "Одно из значений: " + labels(f) + "."
	case importers.FieldChoices:
		return "Одно или несколько значений через запятую: " + labels(f) + "."
	case importers.FieldURL:
		if f.Code == "photo" {
			return "Ссылка на фото JPEG, PNG, WebP или GIF до 8 МБ. Фото загрузится после сохранения."
		}
		return "Ссылка, которая начинается с http:// или https://."
	case importers.FieldDate:
		return "Дата, например 28.09.2026."
	}
	text := fmt.Sprintf("Текст до %s знаков.", rutext.Num(float64(f.MaxLen), 0))
	if f.Required {
		text = "Обязательное поле. " + text
	}
	return text
}

func labels(f importers.Field) string {
	out := make([]string, len(f.Choices))
	for i, c := range f.Choices {
		out[i] = c.Label
	}
	return strings.Join(out, ", ")
}

// csvTable writes rows separated by semicolons with a UTF-8 BOM, the way spreadsheet programs open Russian text.
// Numbers get a decimal comma.
func csvTable(head []string, rows [][]any) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\ufeff")
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	w.UseCRLF = true
	if err := w.Write(head); err != nil {
		return nil, fmt.Errorf("exporters.catalog.csv: %w", err)
	}
	for _, row := range rows {
		rec := make([]string, len(row))
		for i, c := range row {
			rec[i] = csvCell(c)
		}
		if err := w.Write(rec); err != nil {
			return nil, fmt.Errorf("exporters.catalog.csv: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("exporters.catalog.csv: %w", err)
	}
	return buf.Bytes(), nil
}

func csvCell(c any) string {
	switch x := c.(type) {
	case nil:
		return ""
	case float64:
		return strings.Replace(strconv.FormatFloat(math.Round(x*1e6)/1e6, 'f', -1, 64), ".", ",", 1)
	case string:
		return x
	}
	return fmt.Sprint(c)
}

func xlsxTable(cols []importers.Field, rows [][]any) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	x := &sheetWriter{f: f, name: catalogSheet}
	f.SetSheetName("Sheet1", x.name)
	x.header(headers(cols))
	money := -1
	stamp := -1
	for i, c := range cols {
		switch c.Code {
		case "price_rub":
			money = i
		case importers.StampField:
			stamp = i
		}
	}
	x.rows(rows, map[int]bool{money: true})
	x.widths(func(i int) float64 {
		switch cols[i].Code {
		case "name", "scenario":
			return 36
		case "description":
			return 60
		case "id":
			return 38
		}
		return 16
	})
	if stamp >= 0 && x.err == nil {
		name, _ := excelize.ColumnNumberToName(stamp + 1)
		x.err = f.SetColVisible(x.name, name, false)
	}
	if rows == nil {
		x.help(cols)
	}
	return x.bytes()
}

func xlsxForm(head []string, cols []importers.Field, rows [][]any) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	x := &sheetWriter{f: f, name: formSheet}
	f.SetSheetName("Sheet1", x.name)
	x.header(head)
	x.rows(rows, nil)
	x.widths(func(i int) float64 { return []float64{30, 40, 12, 70}[i] })
	for i, c := range cols {
		if x.err != nil || (c.Kind != importers.FieldChoice) {
			continue
		}
		cell := fmt.Sprintf("B%d", i+2)
		dv := excelize.NewDataValidation(true)
		dv.SetSqref(cell)
		opts := make([]string, len(c.Choices))
		for j, ch := range c.Choices {
			opts[j] = ch.Label
		}
		if x.err = dv.SetDropList(opts); x.err == nil {
			x.err = f.AddDataValidation(x.name, dv)
		}
	}
	return x.bytes()
}

type sheetWriter struct {
	f     *excelize.File
	name  string
	width int
	err   error
}

func (x *sheetWriter) header(head []string) {
	if x.err != nil {
		return
	}
	x.width = len(head)
	style, err := x.f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		x.err = err
		return
	}
	for i, h := range head {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if x.err = x.f.SetCellStr(x.name, cell, h); x.err != nil {
			return
		}
	}
	last, _ := excelize.CoordinatesToCellName(max(1, len(head)), 1)
	if x.err = x.f.SetCellStyle(x.name, "A1", last, style); x.err != nil {
		return
	}
	x.err = x.f.SetPanes(x.name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
}

func (x *sheetWriter) rows(rows [][]any, money map[int]bool) {
	if x.err != nil {
		return
	}
	moneyFormat := `#,##0 "₽"`
	moneyStyle, err := x.f.NewStyle(&excelize.Style{CustomNumFmt: &moneyFormat})
	if err != nil {
		x.err = err
		return
	}
	for r, row := range rows {
		for c, v := range row {
			if v == nil {
				continue
			}
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			switch t := v.(type) {
			case string:
				x.err = x.f.SetCellStr(x.name, cell, t)
			default:
				x.err = x.f.SetCellValue(x.name, cell, t)
			}
			if x.err == nil && money[c] {
				x.err = x.f.SetCellStyle(x.name, cell, cell, moneyStyle)
			}
			if x.err != nil {
				return
			}
		}
	}
}

func (x *sheetWriter) widths(w func(int) float64) {
	for i := 0; i < x.width && x.err == nil; i++ {
		name, _ := excelize.ColumnNumberToName(i + 1)
		x.err = x.f.SetColWidth(x.name, name, name, w(i))
	}
}

// help adds the «Справка» sheet: what each column accepts.
func (x *sheetWriter) help(cols []importers.Field) {
	if x.err != nil {
		return
	}
	if _, x.err = x.f.NewSheet(helpSheet); x.err != nil {
		return
	}
	h := &sheetWriter{f: x.f, name: helpSheet}
	h.header([]string{"Колонка", "Единица", "Что писать"})
	rows := make([][]any, 0, len(cols))
	for _, c := range cols {
		rows = append(rows, []any{c.Header(), unitText(c), allowedText(c)})
	}
	h.rows(rows, nil)
	h.widths(func(i int) float64 { return []float64{32, 12, 90}[i] })
	x.err = h.err
}

func (x *sheetWriter) bytes() ([]byte, error) {
	if x.err != nil {
		return nil, fmt.Errorf("exporters.catalog.xlsx: %w", x.err)
	}
	var buf bytes.Buffer
	if err := x.f.Write(&buf); err != nil {
		return nil, fmt.Errorf("exporters.catalog.xlsx: %w", err)
	}
	return buf.Bytes(), nil
}
