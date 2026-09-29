package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/audit"
	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/exporters"
	"moscow_hackathon_2026/api/internal/importers"
	"moscow_hackathon_2026/api/internal/rutext"
)

const (
	importBodyLimit  = 10 << 20
	importMaxSheets  = 20
	importDraftTTL   = 24 * time.Hour
	importPhotoLimit = 200
	importPhotoTotal = 10 * time.Minute
	importSampleRows = 3
)

var (
	errImportNotFound = errors.New("catalog import not found")
	errImportApplied  = errors.New("catalog import already applied")
)

type importSheet struct {
	Name string         `json:"name"`
	Rows importers.Grid `json:"rows"`
}

type importGrid struct {
	Sheets []importSheet `json:"sheets"`
}

type importMapping struct {
	Sheet   int               `json:"sheet"`
	Columns importers.Mapping `json:"columns"`
}

type importColumnJSON struct {
	Index   int      `json:"index"`
	Header  string   `json:"header"`
	Field   *string  `json:"field"`
	Samples []string `json:"samples"`
}

type importStampJSON struct {
	ExportID  string     `json:"export_id"`
	CreatedAt *time.Time `json:"created_at"`
	Found     bool       `json:"found"`
}

type importJSON struct {
	ID           string             `json:"id"`
	FileName     string             `json:"file_name"`
	Mode         string             `json:"mode"`
	Status       string             `json:"status"`
	Sheets       []string           `json:"sheets"`
	Sheet        int                `json:"sheet"`
	Layout       string             `json:"layout"`
	Hint         string             `json:"hint,omitempty"`
	Stamp        *importStampJSON   `json:"stamp"`
	Columns      []importColumnJSON `json:"columns"`
	MappingError string             `json:"mapping_error,omitempty"`
	Plan         *importers.Plan    `json:"plan,omitempty"`
	Summary      json.RawMessage    `json:"summary,omitempty"`
}

// importSummary is what an applied upload stored, and the progress of its photo links.
type importSummary struct {
	Applied        importers.Applied `json:"applied"`
	PhotosTotal    int               `json:"photos_total"`
	PhotosDone     int               `json:"photos_done"`
	PhotoErrors    []photoError      `json:"photo_errors"`
	PhotosSkipped  int               `json:"photos_skipped,omitempty"`
	PhotosStopped  bool              `json:"photos_interrupted,omitempty"`
	AppliedAt      time.Time         `json:"applied_at"`
	UnchangedCount int               `json:"unchanged"`
}

type photoError struct {
	SolutionID string `json:"solution_id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	Message    string `json:"message"`
}

func downloadHeaders(w http.ResponseWriter, name, format string) {
	ct := "text/csv; charset=utf-8"
	if format == exporters.FormatXLSX {
		ct = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+"."+format+`"`)
	w.Header().Set("Cache-Control", "no-store")
}

func fileFormat(w http.ResponseWriter, r *http.Request) (string, bool) {
	switch f := r.URL.Query().Get("format"); f {
	case "", exporters.FormatXLSX:
		return exporters.FormatXLSX, true
	case exporters.FormatCSV:
		return f, true
	}
	writeError(w, r, http.StatusBadRequest, "Формат файла: xlsx или csv.")
	return "", false
}

// adminExportCatalog writes the active catalog and keeps its values, so an upload of the edited file merges three
// ways.
func (s *Server) adminExportCatalog(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	format, ok := fileFormat(w, r)
	if !ok {
		return
	}
	var file []byte
	err := s.inTx(r.Context(), func(tx *Server) error {
		rows, err := tx.q.ListSolutions(r.Context(), solutionsCap)
		if err != nil {
			return err
		}
		active := rows[:0]
		for _, row := range rows {
			if !row.ArchivedAt.Valid {
				active = append(active, row)
			}
		}
		snapshot, err := json.Marshal(importers.SnapshotValues(active))
		if err != nil {
			return err
		}
		actor := auth.FromRequest(r).UserID
		exp, err := tx.q.InsertCatalogExport(r.Context(), db.InsertCatalogExportParams{CreatedBy: &actor, Snapshot: snapshot})
		if err != nil {
			return err
		}
		file, err = exporters.CatalogFile(active, exp.ID.String(), format)
		return err
	})
	if err != nil {
		s.internal(w, r, "admin.catalog.export", err, "Не удалось выгрузить каталог. Повторите.")
		return
	}
	downloadHeaders(w, "katalog-"+time.Now().Format("2006-01-02"), format)
	_, _ = w.Write(file)
}

func (s *Server) adminCatalogTemplate(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	format, ok := fileFormat(w, r)
	if !ok {
		return
	}
	layout, name := importers.LayoutTable, "shablon-kataloga"
	switch r.URL.Query().Get("layout") {
	case "", "catalog":
	case "robot":
		layout, name = importers.LayoutForm, "shablon-resheniya"
	default:
		writeError(w, r, http.StatusBadRequest, "Шаблон бывает для одного решения или для каталога.")
		return
	}
	file, err := exporters.CatalogTemplate(layout, format)
	if err != nil {
		s.internal(w, r, "admin.catalog.template", err, "Не удалось подготовить шаблон. Повторите.")
		return
	}
	downloadHeaders(w, name, format)
	_, _ = w.Write(file)
}

type createImportBody struct {
	FileName string        `json:"file_name"`
	Mode     string        `json:"mode"`
	Sheets   []importSheet `json:"sheets"`
}

func (s *Server) adminCreateImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var body createImportBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, importBodyLimit))
	if err := dec.Decode(&body); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, r, http.StatusRequestEntityTooLarge, "Файл больше 10 МБ. Разбейте его на части и загрузите по очереди.")
			return
		}
		writeError(w, r, http.StatusBadRequest, "Файл не прочитался. Сохраните его как XLSX или CSV и загрузите снова.")
		return
	}
	if body.Mode != importers.ModeRobot && body.Mode != importers.ModeCatalog {
		writeError(w, r, http.StatusBadRequest, "Выберите, что загружаете: одно решение или каталог.")
		return
	}
	body.FileName = strings.TrimSpace(body.FileName)
	if body.FileName == "" || utf8.RuneCountInString(body.FileName) > 200 {
		body.FileName = "file"
	}
	if len(body.Sheets) == 0 {
		writeError(w, r, http.StatusBadRequest, "В файле нет листов с данными. Проверьте файл.")
		return
	}
	if len(body.Sheets) > importMaxSheets {
		writeError(w, r, http.StatusBadRequest, fmt.Sprintf(
			"В файле больше %d листов с данными. Оставьте лист с каталогом и загрузите файл снова.", importMaxSheets))
		return
	}
	for i := range body.Sheets {
		body.Sheets[i].Rows = body.Sheets[i].Rows.Trimmed()
		if err := body.Sheets[i].Rows.Check(); err != nil {
			writeError(w, r, http.StatusBadRequest, fmt.Sprintf(
				"Лист «%s» больше допустимого: до %d строк, %d колонок и %d знаков в ячейке. Разбейте файл на части.",
				body.Sheets[i].Name, importers.MaxGridRows, importers.MaxGridCols, importers.MaxCellRunes))
			return
		}
	}
	grid := importGrid{Sheets: body.Sheets}
	sheet := pickSheet(grid)
	m := importMapping{Sheet: sheet, Columns: importers.AutoMapping(sheetTable(grid, sheet).Header())}
	gridJSON, err := json.Marshal(grid)
	if err != nil {
		s.internal(w, r, "admin.catalog.import", err, "Не удалось сохранить файл. Повторите.")
		return
	}
	var out importJSON
	err = s.inTx(r.Context(), func(tx *Server) error {
		if _, err := tx.q.DeleteStaleCatalogImportDrafts(r.Context(), pgtype.Timestamptz{Time: time.Now().Add(-importDraftTTL), Valid: true}); err != nil {
			return err
		}
		actor := auth.FromRequest(r).UserID
		exportID, _ := tx.stampExport(r.Context(), sheetTable(grid, sheet), m.Columns)
		mappingJSON, err := json.Marshal(m)
		if err != nil {
			return err
		}
		id, err := tx.q.InsertCatalogImport(r.Context(), db.InsertCatalogImportParams{
			CreatedBy: &actor, FileName: body.FileName, Mode: body.Mode, ExportID: exportID,
			Grid: gridJSON, Mapping: mappingJSON,
		})
		if err != nil {
			return err
		}
		out, err = tx.importPreview(r.Context(), id)
		return err
	})
	if err != nil {
		s.importFailed(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// pickSheet prefers the first sheet with a name column.
func pickSheet(g importGrid) int {
	for i := range g.Sheets {
		m := importers.AutoMapping(sheetTable(g, i).Header())
		if m.Has("name") {
			return i
		}
	}
	return 0
}

// sheetTable is the sheet as a table: the one-robot form turns into one header row and one value row.
func sheetTable(g importGrid, sheet int) importers.Grid {
	if sheet < 0 || sheet >= len(g.Sheets) {
		return nil
	}
	rows := g.Sheets[sheet].Rows
	if importers.SheetLayout(rows) == importers.LayoutForm {
		return importers.FormToTable(rows)
	}
	return rows
}

// stampExport finds the export a file came from by its «Выгрузка» column.
func (s *Server) stampExport(ctx context.Context, table importers.Grid, m importers.Mapping) (*uuid.UUID, bool) {
	col := -1
	for c, code := range m {
		if code == importers.StampField {
			col = c
		}
	}
	if col < 0 {
		return nil, false
	}
	for i := 1; i < len(table); i++ {
		id, err := uuid.Parse(strings.TrimSpace(table.Cell(i, col)))
		if err != nil {
			continue
		}
		if _, err := s.q.GetCatalogExport(ctx, id); err != nil {
			return nil, true
		}
		return &id, true
	}
	return nil, false
}

type patchImportBody struct {
	Sheet   int               `json:"sheet"`
	Mapping map[string]string `json:"mapping"`
}

func (s *Server) adminPatchImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, ok := importIDParam(w, r)
	if !ok {
		return
	}
	var body patchImportBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	var out importJSON
	err := s.inTx(r.Context(), func(tx *Server) error {
		imp, grid, _, err := tx.loadImport(r.Context(), id, true)
		if err != nil {
			return err
		}
		if imp.Status != "draft" {
			return errImportApplied
		}
		if body.Sheet < 0 || body.Sheet >= len(grid.Sheets) {
			return &importers.FieldError{Field: "sheet", Message: "В файле нет такого листа. Выберите лист из списка."}
		}
		table := sheetTable(grid, body.Sheet)
		m := importMapping{Sheet: body.Sheet, Columns: importers.Mapping{}}
		if body.Mapping == nil {
			m.Columns = importers.AutoMapping(table.Header())
		}
		for key, code := range body.Mapping {
			var col int
			if _, err := fmt.Sscan(key, &col); err != nil || code == "" {
				continue
			}
			m.Columns[col] = code
		}
		if err := m.Columns.Check(table.Width()); err != nil && !errors.Is(err, importers.ErrNoNameColumn) {
			return &importers.FieldError{Field: "mapping", Message: strings.ToUpper(err.Error()[:1]) + err.Error()[1:] + "."}
		}
		exportID, _ := tx.stampExport(r.Context(), table, m.Columns)
		mappingJSON, err := json.Marshal(m)
		if err != nil {
			return err
		}
		if err := tx.q.SetCatalogImportMapping(r.Context(), db.SetCatalogImportMappingParams{ID: id, Mapping: mappingJSON, ExportID: exportID}); err != nil {
			return err
		}
		out, err = tx.importPreview(r.Context(), id)
		return err
	})
	if err != nil {
		s.importFailed(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) adminGetImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, ok := importIDParam(w, r)
	if !ok {
		return
	}
	out, err := s.importPreview(r.Context(), id)
	if err != nil {
		s.importFailed(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) adminDeleteImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, ok := importIDParam(w, r)
	if !ok {
		return
	}
	if _, err := s.q.DeleteCatalogImport(r.Context(), id); err != nil {
		s.internal(w, r, "admin.catalog.import.delete", err, "Не удалось удалить загрузку. Повторите.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func importIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "importId"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "Некорректный идентификатор загрузки.")
		return uuid.Nil, false
	}
	return id, true
}

func (s *Server) importFailed(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errImportNotFound):
		writeError(w, r, http.StatusNotFound, "Загрузка не найдена: черновики хранятся сутки. Загрузите файл снова.")
	case errors.Is(err, errImportApplied):
		writeError(w, r, http.StatusConflict, "Эта загрузка уже сохранена. Чтобы загрузить файл ещё раз, начните новую загрузку.")
	case errors.Is(err, importers.ErrStale):
		writeCode(w, r, http.StatusConflict, "catalog_changed", "Каталог изменился после проверки, проверьте ещё раз.")
	default:
		if writeSaveError(w, r, err) {
			return
		}
		s.internal(w, r, "admin.catalog.import", err, "Не удалось обработать файл. Повторите.")
	}
}

func (s *Server) loadImport(ctx context.Context, id uuid.UUID, lock bool) (db.CatalogImport, importGrid, importMapping, error) {
	if lock {
		if _, err := s.q.LockCatalogImport(ctx, id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return db.CatalogImport{}, importGrid{}, importMapping{}, errImportNotFound
			}
			return db.CatalogImport{}, importGrid{}, importMapping{}, err
		}
	}
	imp, err := s.q.GetCatalogImport(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.CatalogImport{}, importGrid{}, importMapping{}, errImportNotFound
		}
		return db.CatalogImport{}, importGrid{}, importMapping{}, err
	}
	var grid importGrid
	var m importMapping
	if err := json.Unmarshal(imp.Grid, &grid); err != nil {
		return imp, grid, m, fmt.Errorf("catalog.import.grid: %w", err)
	}
	if err := json.Unmarshal(imp.Mapping, &m); err != nil {
		return imp, grid, m, fmt.Errorf("catalog.import.mapping: %w", err)
	}
	return imp, grid, m, nil
}

// importPlan reads the file rows and compares them with the live catalog.
func (s *Server) importPlan(ctx context.Context, imp db.CatalogImport, grid importGrid, m importMapping) (importers.Plan, importers.Grid, []importers.FileRow, error) {
	table := sheetTable(grid, m.Sheet)
	rows := importers.ReadRows(table, m.Columns)
	if imp.Mode == importers.ModeRobot && len(rows) > 1 {
		rows = rows[:1]
	}
	current, err := s.q.ListSolutions(ctx, solutionsCap)
	if err != nil {
		return importers.Plan{}, nil, nil, err
	}
	var base map[string]importers.Values
	if imp.ExportID != nil {
		exp, err := s.q.GetCatalogExport(ctx, *imp.ExportID)
		if err == nil {
			base, err = importers.DecodeSnapshot(exp.Snapshot)
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return importers.Plan{}, nil, nil, err
		}
	}
	plan := importers.BuildPlan(rows, current, base, imp.Mode, m.Columns.Has("id"))
	return plan, table, rows, nil
}

func (s *Server) importPreview(ctx context.Context, id uuid.UUID) (importJSON, error) {
	imp, grid, m, err := s.loadImport(ctx, id, false)
	if err != nil {
		return importJSON{}, err
	}
	out := importJSON{
		ID: imp.ID.String(), FileName: imp.FileName, Mode: imp.Mode, Status: imp.Status,
		Sheets: make([]string, len(grid.Sheets)), Sheet: m.Sheet, Columns: []importColumnJSON{},
	}
	for i, sh := range grid.Sheets {
		out.Sheets[i] = sh.Name
	}
	if m.Sheet >= 0 && m.Sheet < len(grid.Sheets) {
		out.Layout = importers.SheetLayout(grid.Sheets[m.Sheet].Rows)
	}
	table := sheetTable(grid, m.Sheet)
	header := table.Header()
	for c := 0; c < table.Width(); c++ {
		col := importColumnJSON{Index: c, Samples: []string{}}
		if c < len(header) {
			col.Header = strings.TrimSpace(header[c])
		}
		if code, ok := m.Columns[c]; ok {
			col.Field = &code
		}
		for i := 1; i < len(table) && len(col.Samples) < importSampleRows; i++ {
			if v := strings.TrimSpace(table.Cell(i, c)); v != "" {
				col.Samples = append(col.Samples, clipSample(v))
			}
		}
		out.Columns = append(out.Columns, col)
	}
	if imp.ExportID != nil {
		out.Stamp = &importStampJSON{ExportID: imp.ExportID.String()}
		if exp, err := s.q.GetCatalogExport(ctx, *imp.ExportID); err == nil {
			out.Stamp.Found = true
			out.Stamp.CreatedAt = timestamptzPtr(exp.CreatedAt)
		}
	}
	if imp.Status != "draft" {
		out.Summary = imp.Summary
		return out, nil
	}
	dataRows := len(table) - 1
	switch {
	case imp.Mode == importers.ModeRobot && out.Layout == importers.LayoutTable && dataRows > 1:
		out.Hint = fmt.Sprintf("Для одного решения берётся первая строка, а строк с данными в файле %s. Чтобы загрузить все, выберите «Каталог целиком».",
			rutext.Num(float64(dataRows), 0))
	case imp.Mode == importers.ModeCatalog && out.Layout == importers.LayoutForm:
		out.Hint = "Файл заполнен по шаблону одного решения. Выберите «Один робот», чтобы увидеть его поля рядом с каталогом."
	}
	if err := m.Columns.Check(table.Width()); err != nil {
		msg := err.Error()
		out.MappingError = strings.ToUpper(msg[:1]) + msg[1:] + "."
		return out, nil
	}
	plan, _, _, err := s.importPlan(ctx, imp, grid, m)
	if err != nil {
		return importJSON{}, err
	}
	out.Plan = &plan
	return out, nil
}

func clipSample(s string) string {
	if utf8.RuneCountInString(s) <= 60 {
		return s
	}
	return string([]rune(s)[:60]) + "..."
}

type applyRow struct {
	Key string `json:"key"`
	Rev string `json:"rev"`
}

type applyImportBody struct {
	Rows    []applyRow                   `json:"rows"`
	Fields  []string                     `json:"fields"`
	Picks   map[string]map[string]string `json:"picks"`
	Archive []string                     `json:"archive"`
}

func (s *Server) adminApplyImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, ok := importIDParam(w, r)
	if !ok {
		return
	}
	var body applyImportBody
	if !decodeJSONLimit(w, r, &body, false, importBodyLimit) {
		return
	}
	sel := importers.Selection{Rows: map[string]string{}, Fields: body.Fields, Picks: body.Picks, Archive: body.Archive}
	for _, row := range body.Rows {
		sel.Rows[row.Key] = row.Rev
	}
	actor := auth.FromRequest(r).UserID
	var summary importSummary
	err := s.inTx(r.Context(), func(tx *Server) error {
		imp, grid, m, err := tx.loadImport(r.Context(), id, true)
		if err != nil {
			return err
		}
		if imp.Status != "draft" {
			return errImportApplied
		}
		if err := m.Columns.Check(sheetTable(grid, m.Sheet).Width()); err != nil {
			return &importers.FieldError{Field: "mapping", Message: "Проверьте колонки: " + err.Error() + "."}
		}
		plan, _, _, err := tx.importPlan(r.Context(), imp, grid, m)
		if err != nil {
			return err
		}
		applied, err := importers.Apply(r.Context(), tx.q, plan, sel)
		if err != nil {
			return err
		}
		if len(applied.Photos) > importPhotoLimit {
			summary.PhotosSkipped = len(applied.Photos) - importPhotoLimit
			applied.Photos = applied.Photos[:importPhotoLimit]
		}
		summary.Applied = applied
		summary.PhotosTotal = len(applied.Photos)
		summary.PhotoErrors = []photoError{}
		summary.AppliedAt = time.Now().UTC()
		summary.UnchangedCount = plan.Counts[importers.ClassUnchanged]
		if err := tx.recordImportAudit(r, actor, imp, applied); err != nil {
			return err
		}
		status := "done"
		if len(applied.Photos) > 0 {
			status = "photos"
		}
		raw, err := json.Marshal(summary)
		if err != nil {
			return err
		}
		return tx.q.SetCatalogImportResult(r.Context(), db.SetCatalogImportResultParams{ID: id, Status: status, Summary: raw})
	})
	if err != nil {
		s.importFailed(w, r, err)
		return
	}
	if len(summary.Applied.Photos) > 0 {
		go s.fetchImportPhotos(id, actor, summary)
	}
	out, err := s.importPreview(r.Context(), id)
	if err != nil {
		s.importFailed(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) recordImportAudit(r *http.Request, actor uuid.UUID, imp db.CatalogImport, a importers.Applied) error {
	reqID := requestID(r)
	importID := imp.ID.String()
	for _, list := range []struct {
		action string
		items  []importers.RobotChange
	}{{audit.ActionSolutionCreate, a.Created}, {audit.ActionSolutionUpdate, a.Updated}} {
		for _, c := range list.items {
			if err := s.recordAudit(r.Context(), reqID, audit.Event{
				Actor: &actor, Action: list.action, TargetType: audit.TargetSolution, TargetID: c.ID,
				Meta: map[string]any{"name": c.Name, "changes": c.Changes, "import_id": importID},
			}); err != nil {
				return err
			}
		}
	}
	for _, ref := range a.Archived {
		if err := s.recordAudit(r.Context(), reqID, audit.Event{
			Actor: &actor, Action: audit.ActionSolutionArchive, TargetType: audit.TargetSolution, TargetID: ref.ID,
			Meta: map[string]any{"name": ref.Name, "import_id": importID},
		}); err != nil {
			return err
		}
	}
	return s.recordAudit(r.Context(), reqID, audit.Event{
		Actor: &actor, Action: audit.ActionCatalogImport, TargetType: audit.TargetCatalogImport, TargetID: importID,
		Meta: map[string]any{
			"file_name": imp.FileName, "mode": imp.Mode,
			"created": len(a.Created), "updated": len(a.Updated), "archived": len(a.Archived), "photos": len(a.Photos),
		},
	})
}

// fetchImportPhotos downloads the photo links of an applied upload one by one and reports progress in its summary.
func (s *Server) fetchImportPhotos(id, actor uuid.UUID, summary importSummary) {
	ctx, cancel := context.WithTimeout(context.Background(), importPhotoTotal)
	defer cancel()
	log := s.log.With("op", "catalog.import.photos", "import_id", id.String())
	save := func(status string) {
		raw, err := json.Marshal(summary)
		if err != nil {
			log.Error("catalog.import.photos.summary", "err", err)
			return
		}
		wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer wcancel()
		if err := s.q.SetCatalogImportResult(wctx, db.SetCatalogImportResultParams{ID: id, Status: status, Summary: raw}); err != nil {
			log.Error("catalog.import.photos.save", "err", err)
		}
	}
	for _, task := range summary.Applied.Photos {
		if err := s.importPhoto(ctx, actor, id, task); err != nil {
			summary.PhotoErrors = append(summary.PhotoErrors, photoError{
				SolutionID: task.SolutionID, Name: task.Name, URL: task.URL, Message: importers.PhotoErrorText(err),
			})
		}
		summary.PhotosDone++
		save("photos")
	}
	sort.SliceStable(summary.PhotoErrors, func(i, j int) bool { return summary.PhotoErrors[i].Name < summary.PhotoErrors[j].Name })
	save("done")
}

func (s *Server) importPhoto(ctx context.Context, actor, importID uuid.UUID, task importers.PhotoTask) error {
	if s.photos == nil {
		return importers.ErrPhotoAddress
	}
	photo, err := s.photos.Fetch(ctx, task.URL)
	if err != nil {
		return err
	}
	sid, err := uuid.Parse(task.SolutionID)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *Server) error {
		if err := tx.q.UpsertSolutionImage(ctx, db.UpsertSolutionImageParams{
			SolutionID: sid, ContentType: photo.ContentType, Bytes: photo.Bytes, Sha256: photo.SHA256,
		}); err != nil {
			return err
		}
		return audit.Record(ctx, tx.q, audit.Event{
			Actor: &actor, Action: audit.ActionSolutionImage, TargetType: audit.TargetSolution, TargetID: task.SolutionID,
			Meta: map[string]any{"name": task.Name, "image_sha": photo.SHA256, "import_id": importID.String()},
		})
	})
}

// adminImportErrors returns the uploaded sheet with an «Ошибки» column for the rows that could not be read.
func (s *Server) adminImportErrors(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, ok := importIDParam(w, r)
	if !ok {
		return
	}
	format, ok := fileFormat(w, r)
	if !ok {
		return
	}
	imp, grid, m, err := s.loadImport(r.Context(), id, false)
	if err != nil {
		s.importFailed(w, r, err)
		return
	}
	plan, _, _, err := s.importPlan(r.Context(), imp, grid, m)
	if err != nil {
		s.importFailed(w, r, err)
		return
	}
	sheet := grid.Sheets[m.Sheet].Rows
	messages := map[int][]string{}
	form := importers.SheetLayout(sheet) == importers.LayoutForm
	for _, rp := range plan.Rows {
		for _, fe := range rp.Errors {
			line := rp.Line
			if form {
				line = formLine(sheet, fe.Field)
			}
			messages[line] = append(messages[line], fe.Message)
		}
	}
	file, err := exporters.ErrorReport(sheet, messages, format)
	if err != nil {
		s.internal(w, r, "admin.catalog.import.errors", err, "Не удалось подготовить отчёт. Повторите.")
		return
	}
	downloadHeaders(w, "oshibki-zagruzki", format)
	_, _ = w.Write(file)
}

// formLine finds the line of a field in the one-robot form; errors of unknown fields go to the header line.
func formLine(sheet importers.Grid, code string) int {
	for i := 1; i < len(sheet); i++ {
		if f, ok := importers.FieldForHeader(sheet.Cell(i, 0)); ok && f.Code == code {
			return i + 1
		}
	}
	return 1
}
