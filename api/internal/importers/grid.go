package importers

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Grid is one sheet of a file as rows of cells; the first row is the header.
type Grid [][]string

// Limits of an uploaded sheet.
const (
	MaxGridRows  = 5000
	MaxGridCols  = 80
	MaxCellRunes = 4000
)

var ErrGridTooLarge = errors.New("importers.grid: sheet over the limits")

// ReadCSVGrid reads a file separated by semicolons, as the organizer delivers it, dropping a UTF-8 BOM.
func ReadCSVGrid(r io.Reader) (Grid, error) {
	cr := csv.NewReader(r)
	cr.Comma = ';'
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	var g Grid
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("importers.csv row %d: %w", len(g)+1, err)
		}
		g = append(g, rec)
	}
	if len(g) > 0 && len(g[0]) > 0 {
		g[0][0] = strings.TrimPrefix(g[0][0], "\ufeff")
	}
	return g, nil
}

// Header is the first row, or nil for an empty sheet.
func (g Grid) Header() []string {
	if len(g) == 0 {
		return nil
	}
	return g[0]
}

// Cell is the cell at row i, column j, or "" past the end of a short row.
func (g Grid) Cell(i, j int) string {
	if i < 0 || i >= len(g) || j < 0 || j >= len(g[i]) {
		return ""
	}
	return g[i][j]
}

// Width is the number of columns of the widest row.
func (g Grid) Width() int {
	w := 0
	for _, row := range g {
		w = max(w, len(row))
	}
	return w
}

// Check rejects a sheet over the upload limits.
func (g Grid) Check() error {
	if len(g) > MaxGridRows+1 {
		return fmt.Errorf("%w: more than %d rows", ErrGridTooLarge, MaxGridRows)
	}
	if g.Width() > MaxGridCols {
		return fmt.Errorf("%w: more than %d columns", ErrGridTooLarge, MaxGridCols)
	}
	for _, row := range g {
		for _, c := range row {
			if len(c) > MaxCellRunes && len([]rune(c)) > MaxCellRunes {
				return fmt.Errorf("%w: a cell longer than %d characters", ErrGridTooLarge, MaxCellRunes)
			}
		}
	}
	return nil
}

// Trimmed drops trailing empty rows and the empty cells at the end of each row.
func (g Grid) Trimmed() Grid {
	out := make(Grid, 0, len(g))
	for _, row := range g {
		end := len(row)
		for end > 0 && strings.TrimSpace(row[end-1]) == "" {
			end--
		}
		out = append(out, row[:end])
	}
	for len(out) > 1 && len(out[len(out)-1]) == 0 {
		out = out[:len(out)-1]
	}
	return out
}
