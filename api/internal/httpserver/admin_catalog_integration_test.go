package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"moscow_hackathon_2026/api/internal/engine"
	"moscow_hackathon_2026/api/internal/importers"
)

func adminClient(t *testing.T, env itEnv, email string) *itClient {
	t.Helper()
	c := register(t, env.h, email)
	if _, err := env.db.Exec(t.Context(), "UPDATE users SET role = 'admin' WHERE email = $1", email); err != nil {
		t.Fatal(err)
	}
	return c
}

// raw sends a body as is, the way the browser uploads a photo.
func (c *itClient) raw(method, path, contentType string, body []byte) *httptest.ResponseRecorder {
	c.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Origin", "http://localhost")
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if c.csrf != "" {
		req.Header.Set(csrfHeader, c.csrf)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec
}

func (c *itClient) get(path string, header map[string]string) *httptest.ResponseRecorder {
	c.t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec
}

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type solutionPage struct {
	Items []solutionJSON `json:"items"`
	Total int            `json:"total"`
}

func TestAdminEditsCatalog(t *testing.T) {
	env := newEnv(t)
	admin := adminClient(t, env, "catalog-admin@example.com")
	user := register(t, env.h, "catalog-user@example.com")
	path := "/api/admin/solutions/" + h1500ID

	var saved solutionUpdate
	admin.json(http.MethodPatch, path, map[string]any{"payload_kg": 1600, "family": "mobile"}, http.StatusOK, &saved)
	if saved.Solution.Specs.PayloadKg == nil || *saved.Solution.Specs.PayloadKg != 1600 {
		t.Fatalf("payload not saved: %+v", saved.Solution.Specs)
	}
	if len(saved.Changes) != 1 || saved.Changes[0].Field != "payload_kg" || saved.Changes[0].Before != 1500.0 {
		t.Fatalf("changes %+v: an unchanged family is not a change", saved.Changes)
	}
	admin.json(http.MethodPatch, path, map[string]any{"payload_kg": 1600}, http.StatusOK, &saved)
	if len(saved.Changes) != 0 {
		t.Fatalf("saving the same value changes nothing: %+v", saved.Changes)
	}
	user.json(http.MethodPatch, path, map[string]any{"payload_kg": 1}, http.StatusForbidden, nil)

	for _, body := range []map[string]any{
		{"payload_kg": -1},
		{"temp_min_c": 50, "temp_max_c": 10},
		{"name": nil},
		{"object_types": []string{"factory"}},
		{"colour": "red"},
		{"service_pct_year": 5},
	} {
		rec := admin.do(http.MethodPatch, path, body, false)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%v: status %d: %s", body, rec.Code, rec.Body.String())
		}
	}

	var created solutionUpdate
	admin.json(http.MethodPost, "/api/admin/solutions", map[string]any{
		"name": `Робот "Тест"`, "vendor": "Лаборатория", "payload_kg": 100, "object_types": []string{"hospital"},
	}, http.StatusCreated, &created)
	if created.Solution.Name != "Робот «Тест»" || len(created.Solution.ObjectTypes) != 1 {
		t.Fatalf("created %+v", created.Solution)
	}
	admin.json(http.MethodPost, "/api/admin/solutions", map[string]any{"vendor": "Без имени"}, http.StatusBadRequest, nil)
	var copied solutionJSON
	admin.json(http.MethodPost, "/api/admin/solutions/"+created.Solution.ID+"/duplicate", nil, http.StatusCreated, &copied)
	if copied.Name != "Робот «Тест» (копия)" || copied.Specs.PayloadKg == nil || *copied.Specs.PayloadKg != 100 {
		t.Fatalf("copy %+v", copied)
	}

	admin.json(http.MethodDelete, "/api/admin/solutions/"+created.Solution.ID, nil, http.StatusNoContent, nil)
	var page solutionPage
	user.json(http.MethodGet, "/api/solutions?limit=500&q="+url.QueryEscape("Робот «Тест»"), nil, http.StatusOK, &page)
	if page.Total != 1 || page.Items[0].ID != copied.ID {
		t.Fatalf("archived robot still listed: %+v", page.Items)
	}
	user.json(http.MethodGet, "/api/solutions?archived=include", nil, http.StatusForbidden, nil)
	admin.json(http.MethodGet, "/api/solutions?limit=500&archived=include&q="+url.QueryEscape("Робот «Тест»"), nil, http.StatusOK, &page)
	if page.Total != 2 {
		t.Fatalf("admin sees the archive: %d", page.Total)
	}
	rec := user.get("/api/catalog/bundle", nil)
	var cat engine.Catalog
	if err := json.Unmarshal(rec.Body.Bytes(), &cat); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range cat.Candidates {
		if c.ID == created.Solution.ID {
			found = c.Archived
		}
	}
	if !found {
		t.Fatal("calculations must still load an archived robot")
	}
	var restored solutionJSON
	admin.json(http.MethodPost, "/api/admin/solutions/"+created.Solution.ID+"/restore", nil, http.StatusOK, &restored)
	if restored.ArchivedAt != nil {
		t.Fatal("restore keeps the archive date")
	}

	rec = admin.raw(http.MethodPut, path+"/image", "image/png", testPNG(t, 1600, 800))
	if rec.Code != http.StatusOK {
		t.Fatalf("photo upload: %d %s", rec.Code, rec.Body.String())
	}
	var withPhoto solutionJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &withPhoto); err != nil || withPhoto.ImageSHA == nil {
		t.Fatalf("photo hash missing: %v", err)
	}
	img := user.get("/api/solutions/"+h1500ID+"/image?v="+*withPhoto.ImageSHA, nil)
	if img.Code != http.StatusOK || img.Header().Get("Content-Type") != "image/jpeg" ||
		!strings.Contains(img.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("photo: %d %v", img.Code, img.Header())
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(img.Body.Bytes()))
	if err != nil || cfg.Width != 1200 || cfg.Height != 600 {
		t.Fatalf("photo not scaled: %+v %v", cfg, err)
	}
	if again := user.get("/api/solutions/"+h1500ID+"/image", map[string]string{"If-None-Match": `"` + *withPhoto.ImageSHA + `"`}); again.Code != http.StatusNotModified {
		t.Fatalf("etag: %d", again.Code)
	}
	if bad := admin.raw(http.MethodPut, path+"/image", "image/png", []byte("<svg></svg>")); bad.Code != http.StatusBadRequest {
		t.Fatalf("svg accepted: %d", bad.Code)
	}
	if forbidden := user.raw(http.MethodPut, path+"/image", "image/png", testPNG(t, 10, 10)); forbidden.Code != http.StatusForbidden {
		t.Fatalf("user uploads a photo: %d", forbidden.Code)
	}
	var copiedPhoto solutionJSON
	admin.json(http.MethodPost, path+"/duplicate", nil, http.StatusCreated, &copiedPhoto)
	if copiedPhoto.ImageSHA == nil || *copiedPhoto.ImageSHA != *withPhoto.ImageSHA {
		t.Fatal("a copy keeps the photo")
	}
	admin.json(http.MethodDelete, path+"/image", nil, http.StatusOK, nil)
	if gone := user.get("/api/solutions/"+h1500ID+"/image", nil); gone.Code != http.StatusNotFound {
		t.Fatalf("deleted photo: %d", gone.Code)
	}

	var log struct {
		Items []auditItem `json:"items"`
	}
	admin.json(http.MethodGet, "/api/admin/audit?target_type=solution&target_id="+created.Solution.ID, nil, http.StatusOK, &log)
	got := map[string]bool{}
	for _, item := range log.Items {
		got[item.Action] = true
	}
	for _, want := range []string{"solution.create", "solution.archive", "solution.restore"} {
		if !got[want] {
			t.Errorf("audit has no %s: %+v", want, log.Items)
		}
	}
	if n := log.Items[0].TargetName; n == nil || *n != created.Solution.Name {
		t.Errorf("audit names the robot %v, want %q", n, created.Solution.Name)
	}
	admin.json(http.MethodGet, "/api/admin/audit?limit=2&action=solution.archive,solution.restore", nil, http.StatusOK, &log)
	if len(log.Items) != 2 || log.Items[0].Action != "solution.restore" || log.Items[1].Action != "solution.archive" {
		t.Fatalf("action filter %+v", log.Items)
	}
	var older struct {
		Items []auditItem `json:"items"`
	}
	admin.json(http.MethodGet, fmt.Sprintf("/api/admin/audit?limit=2&action=solution.archive,solution.restore&before=%d", log.Items[1].ID), nil, http.StatusOK, &older)
	if len(older.Items) != 0 {
		t.Fatalf("page after the first archive: %+v", older.Items)
	}
	admin.json(http.MethodGet, "/api/admin/audit?before=x", nil, http.StatusBadRequest, nil)
}

func TestSeedKeepsAdminEdits(t *testing.T) {
	env := newEnv(t)
	admin := adminClient(t, env, "seed-admin@example.com")
	admin.json(http.MethodPatch, "/api/admin/solutions/"+h1500ID, map[string]any{"payload_kg": nil}, http.StatusOK, nil)
	res, err := importers.Seed(t.Context(), env.db, env.srv.q, "../../../data/catalog.csv", "../../../data/seeds/robot_specs.json")
	if err != nil {
		t.Fatal(err)
	}
	if res.CatalogInserted != 0 || res.EditedSkipped != 1 {
		t.Fatalf("seed %+v", res)
	}
	var got solutionJSON
	admin.json(http.MethodGet, "/api/solutions/"+h1500ID, nil, http.StatusOK, &got)
	if got.Specs.PayloadKg != nil {
		t.Fatalf("the seed refilled a value the admin cleared: %v", *got.Specs.PayloadKg)
	}
}

type importResp struct {
	ID           string             `json:"id"`
	Mode         string             `json:"mode"`
	Status       string             `json:"status"`
	Layout       string             `json:"layout"`
	Hint         string             `json:"hint"`
	Stamp        *importStampJSON   `json:"stamp"`
	Columns      []importColumnJSON `json:"columns"`
	MappingError string             `json:"mapping_error"`
	Plan         *importers.Plan    `json:"plan"`
	Summary      *importSummary     `json:"summary"`
}

func xlsxRows(t *testing.T, body []byte, sheet string) importers.Grid {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func postImport(t *testing.T, c *itClient, mode string, sheets ...importSheet) importResp {
	t.Helper()
	var out importResp
	c.json(http.MethodPost, "/api/admin/catalog/imports", map[string]any{"file_name": "katalog.xlsx", "mode": mode, "sheets": sheets}, http.StatusCreated, &out)
	return out
}

func applyAll(t *testing.T, c *itClient, imp importResp, fields []string, want int) importResp {
	t.Helper()
	rows := []map[string]string{}
	for _, rp := range imp.Plan.Rows {
		if rp.Class != importers.ClassError && rp.Class != importers.ClassUnchanged {
			rows = append(rows, map[string]string{"key": rp.Key, "rev": rp.Rev})
		}
	}
	var out importResp
	c.json(http.MethodPost, "/api/admin/catalog/imports/"+imp.ID+"/apply", map[string]any{"rows": rows, "fields": fields}, want, &out)
	return out
}

func column(t *testing.T, g importers.Grid, header string) int {
	t.Helper()
	for i, h := range g.Header() {
		if h == header {
			return i
		}
	}
	t.Fatalf("no column %q in %v", header, g.Header())
	return -1
}

func rowOf(t *testing.T, g importers.Grid, id string) int {
	t.Helper()
	for i := 1; i < len(g); i++ {
		if g.Cell(i, 0) == id {
			return i
		}
	}
	t.Fatalf("no row for %s", id)
	return -1
}

func setCell(g importers.Grid, i, j int, v string) {
	for len(g[i]) <= j {
		g[i] = append(g[i], "")
	}
	g[i][j] = v
}

func cloneGrid(g importers.Grid) importers.Grid {
	out := make(importers.Grid, len(g))
	for i, row := range g {
		out[i] = append([]string(nil), row...)
	}
	return out
}

func onlyCounts(t *testing.T, p *importers.Plan, want map[string]int) {
	t.Helper()
	if p == nil {
		t.Fatal("no plan")
	}
	for class, n := range p.Counts {
		if n != want[class] {
			t.Fatalf("counts %v, want %v; rows: %+v", p.Counts, want, changedRows(p))
		}
	}
	for class, n := range want {
		if p.Counts[class] != n {
			t.Fatalf("counts %v, want %v; rows: %+v", p.Counts, want, changedRows(p))
		}
	}
}

func changedRows(p *importers.Plan) []importers.RowPlan {
	var out []importers.RowPlan
	for _, rp := range p.Rows {
		if rp.Class != importers.ClassUnchanged && len(out) < 5 {
			out = append(out, rp)
		}
	}
	return out
}

func TestCatalogExportUploadRoundTrip(t *testing.T) {
	env := newEnv(t)
	admin := adminClient(t, env, "files-admin@example.com")
	user := register(t, env.h, "files-user@example.com")
	if rec := user.get("/api/admin/catalog/export", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("user export: %d", rec.Code)
	}
	var page solutionPage
	admin.json(http.MethodGet, "/api/solutions?limit=500", nil, http.StatusOK, &page)
	active := page.Total

	rec := admin.get("/api/admin/catalog/export?format=xlsx", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Disposition"), "katalog-") {
		t.Fatalf("export: %d %v", rec.Code, rec.Header())
	}
	grid := xlsxRows(t, rec.Body.Bytes(), "Каталог")
	if len(grid) != active+1 {
		t.Fatalf("export rows %d, want %d", len(grid)-1, active)
	}
	imp := postImport(t, admin, importers.ModeCatalog, importSheet{Name: "Каталог", Rows: grid})
	if imp.Stamp == nil || !imp.Stamp.Found || !imp.Plan.ThreeWay {
		t.Fatalf("stamp %+v three-way %v", imp.Stamp, imp.Plan.ThreeWay)
	}
	onlyCounts(t, imp.Plan, map[string]int{importers.ClassUnchanged: active, "missing": 0})

	csv := admin.get("/api/admin/catalog/export?format=csv", nil)
	if !bytes.HasPrefix(csv.Body.Bytes(), []byte("\xef\xbb\xbfИдентификатор;Название;")) {
		t.Fatalf("csv head %q", csv.Body.String()[:60])
	}
	csvGrid, err := importers.ReadCSVGrid(bytes.NewReader(csv.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	onlyCounts(t, postImport(t, admin, importers.ModeCatalog, importSheet{Name: "katalog", Rows: csvGrid}).Plan,
		map[string]int{importers.ClassUnchanged: active, "missing": 0})

	edited := cloneGrid(grid)
	payload := column(t, edited, "Грузоподъёмность, кг")
	nav := column(t, edited, "Навигация")
	h1500 := rowOf(t, edited, h1500ID)
	stacker := rowOf(t, edited, stackerID)
	setCell(edited, h1500, payload, "1 700")
	setCell(edited, stacker, nav, "Метки на полу")
	edited = append(edited[:stacker], edited[stacker+1:]...)
	edited = append(edited[:1], append(importers.Grid{{"", "Новый робот из файла"}}, edited[1:]...)...)
	imp = postImport(t, admin, importers.ModeCatalog, importSheet{Name: "Каталог", Rows: edited})
	onlyCounts(t, imp.Plan, map[string]int{
		importers.ClassUnchanged: active - 2, importers.ClassChanged: 1, importers.ClassNew: 1, "missing": 1,
	})
	if imp.Plan.Missing[0].ID != stackerID {
		t.Fatalf("missing %+v", imp.Plan.Missing)
	}

	admin.json(http.MethodPatch, "/api/admin/solutions/"+h1500ID, map[string]any{"mass_kg": 900}, http.StatusOK, nil)
	applyAll(t, admin, imp, []string{"payload_kg", "name"}, http.StatusConflict)
	admin.json(http.MethodGet, "/api/solutions?q="+url.QueryEscape("Новый робот из файла"), nil, http.StatusOK, &page)
	if page.Total != 0 {
		t.Fatal("a failed upload must save nothing")
	}
	fresh := admin.get("/api/admin/catalog/imports/"+imp.ID, nil)
	var again importResp
	if err := json.Unmarshal(fresh.Body.Bytes(), &again); err != nil {
		t.Fatal(err)
	}
	var applied importResp
	rows := []map[string]string{}
	for _, rp := range again.Plan.Rows {
		if rp.Class == importers.ClassChanged || rp.Class == importers.ClassNew {
			rows = append(rows, map[string]string{"key": rp.Key, "rev": rp.Rev})
		}
	}
	admin.json(http.MethodPost, "/api/admin/catalog/imports/"+imp.ID+"/apply", map[string]any{
		"rows": rows, "fields": []string{"payload_kg", "name"}, "archive": []string{stackerID},
	}, http.StatusOK, &applied)
	if applied.Status != "done" || applied.Summary == nil {
		t.Fatalf("applied %+v", applied)
	}
	sum := applied.Summary.Applied
	if len(sum.Created) != 1 || len(sum.Updated) != 1 || len(sum.Archived) != 1 {
		t.Fatalf("summary %+v", sum)
	}
	var h solutionJSON
	admin.json(http.MethodGet, "/api/solutions/"+h1500ID, nil, http.StatusOK, &h)
	if *h.Specs.PayloadKg != 1700 || *h.Specs.MassKg != 900 {
		t.Fatalf("h1500 after upload: payload %v mass %v", *h.Specs.PayloadKg, *h.Specs.MassKg)
	}
	var st solutionJSON
	admin.json(http.MethodGet, "/api/solutions/"+stackerID, nil, http.StatusOK, &st)
	if st.ArchivedAt == nil {
		t.Fatal("stacker not archived")
	}
	admin.json(http.MethodPost, "/api/admin/catalog/imports/"+imp.ID+"/apply", map[string]any{"rows": rows, "fields": []string{"name"}}, http.StatusConflict, nil)

	var log struct {
		Items []auditItem `json:"items"`
	}
	admin.json(http.MethodGet, "/api/admin/audit?target_type=catalog_import&target_id="+imp.ID, nil, http.StatusOK, &log)
	if len(log.Items) != 1 || log.Items[0].Action != "catalog.import" {
		t.Fatalf("import audit %+v", log.Items)
	}
}

func TestOrganizerCatalogUploadChangesNothing(t *testing.T) {
	env := newEnv(t)
	admin := adminClient(t, env, "organizer-admin@example.com")
	f, err := os.Open("../../../data/catalog.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	grid, err := importers.ReadCSVGrid(f)
	if err != nil {
		t.Fatal(err)
	}
	imp := postImport(t, admin, importers.ModeCatalog, importSheet{Name: "catalog", Rows: grid})
	if imp.Plan.ThreeWay {
		t.Fatal("organizer file has no export stamp")
	}
	for _, rp := range imp.Plan.Rows {
		if rp.Class != importers.ClassUnchanged {
			t.Fatalf("line %d %s: %+v %+v", rp.Line, rp.Class, rp.Fields, rp.Errors)
		}
	}
}

func TestCatalogTemplateAndPhotoLinks(t *testing.T) {
	env := newEnv(t)
	admin := adminClient(t, env, "template-admin@example.com")
	photo := testPNG(t, 64, 48)
	mux := http.NewServeMux()
	mux.HandleFunc("/robot.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(photo)
	})
	site := httptest.NewServer(mux)
	defer site.Close()
	env.srv.photos = importers.NewPhotoFetcher(true)

	rec := admin.get("/api/admin/catalog/template?layout=robot&format=xlsx", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("template: %d %s", rec.Code, rec.Body.String())
	}
	form := xlsxRows(t, rec.Body.Bytes(), "Решение")
	values := map[string]string{
		"Название": "Робот из шаблона", "Грузоподъёмность": "120", "Объекты": "Склад, Больница",
		"Фото": site.URL + "/robot.png", "Сервис в год": "4,5",
	}
	for i := 1; i < len(form); i++ {
		if v, ok := values[form[i][0]]; ok {
			setCell(form, i, 1, v)
			delete(values, form[i][0])
		}
	}
	if len(values) != 0 {
		t.Fatalf("template has no rows for %v", values)
	}
	imp := postImport(t, admin, importers.ModeRobot, importSheet{Name: "Решение", Rows: form})
	if imp.Layout != importers.LayoutForm || len(imp.Plan.Rows) != 1 || imp.Plan.Rows[0].Class != importers.ClassNew {
		t.Fatalf("form upload %+v", imp)
	}
	applied := applyAll(t, admin, imp, []string{"name", "payload_kg", "object_types", "service_pct_year", "photo"}, http.StatusOK)
	id := applied.Summary.Applied.Created[0].ID
	deadline := time.Now().Add(10 * time.Second)
	for applied.Status != "done" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		admin.json(http.MethodGet, "/api/admin/catalog/imports/"+imp.ID, nil, http.StatusOK, &applied)
	}
	if applied.Status != "done" || applied.Summary.PhotosDone != 1 || len(applied.Summary.PhotoErrors) != 0 {
		t.Fatalf("photo job %+v", applied.Summary)
	}
	var robot solutionJSON
	admin.json(http.MethodGet, "/api/solutions/"+id, nil, http.StatusOK, &robot)
	if robot.ImageSHA == nil || *robot.Specs.ServicePctYear != 0.045 || len(robot.ObjectTypes) != 2 {
		t.Fatalf("robot %+v", robot)
	}

	env.srv.photos = importers.NewPhotoFetcher(false)
	table := importers.Grid{{"id", "Фото"}, {id, site.URL + "/other.png"}}
	imp = postImport(t, admin, importers.ModeCatalog, importSheet{Name: "photos", Rows: table})
	applied = applyAll(t, admin, importResp{ID: imp.ID, Plan: &importers.Plan{Rows: []importers.RowPlan{{Key: imp.Plan.Rows[0].Key, Rev: imp.Plan.Rows[0].Rev, Class: importers.ClassChanged}}}}, []string{"photo"}, http.StatusOK)
	deadline = time.Now().Add(10 * time.Second)
	for applied.Status != "done" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		admin.json(http.MethodGet, "/api/admin/catalog/imports/"+imp.ID, nil, http.StatusOK, &applied)
	}
	if len(applied.Summary.PhotoErrors) != 1 || !strings.Contains(applied.Summary.PhotoErrors[0].Message, "внутренний адрес") {
		t.Fatalf("refused link %+v", applied.Summary)
	}
}

func TestCatalogUploadErrorsAndMapping(t *testing.T) {
	env := newEnv(t)
	admin := adminClient(t, env, "mapping-admin@example.com")
	user := register(t, env.h, "mapping-user@example.com")
	sheet := importSheet{Name: "Лист1", Rows: importers.Grid{
		{"Модель", "Вес, кг", "Примечание"},
		{"Робот А", "много", "первый"},
		{"Робот Б", "80", ""},
	}}
	user.json(http.MethodPost, "/api/admin/catalog/imports", map[string]any{"mode": "catalog", "sheets": []importSheet{sheet}}, http.StatusForbidden, nil)
	imp := postImport(t, admin, importers.ModeCatalog, sheet)
	if imp.Columns[2].Field != nil || imp.Columns[0].Samples[0] != "Робот А" {
		t.Fatalf("columns %+v", imp.Columns)
	}
	onlyCounts(t, imp.Plan, map[string]int{importers.ClassError: 1, importers.ClassNew: 1, "missing": 0})

	rep := admin.get("/api/admin/catalog/imports/"+imp.ID+"/errors?format=xlsx", nil)
	errs := xlsxRows(t, rep.Body.Bytes(), "Ошибки")
	if errs[0][3] != "Ошибки" || !strings.HasPrefix(errs[1][3], "Масса: нужно число") || len(errs[2]) > 3 && errs[2][3] != "" {
		t.Fatalf("error report %v", errs)
	}

	var patched importResp
	admin.json(http.MethodPatch, "/api/admin/catalog/imports/"+imp.ID, map[string]any{
		"sheet": 0, "mapping": map[string]string{"0": "name", "1": "payload_kg", "2": "description"},
	}, http.StatusOK, &patched)
	if f := patched.Columns[2].Field; f == nil || *f != "description" {
		t.Fatalf("mapping not saved: %+v", patched.Columns)
	}
	var nameless importResp
	admin.json(http.MethodPatch, "/api/admin/catalog/imports/"+imp.ID, map[string]any{
		"sheet": 0, "mapping": map[string]string{"1": "payload_kg"},
	}, http.StatusOK, &nameless)
	if !strings.Contains(nameless.MappingError, "колонку с названием") || nameless.Plan != nil || nameless.Columns[0].Field != nil {
		t.Fatalf("mapping without a name: %+v", nameless)
	}
	rec := admin.do(http.MethodPatch, "/api/admin/catalog/imports/"+imp.ID, map[string]any{"sheet": 0, "mapping": map[string]string{"0": "name", "1": "name"}}, false)
	if rec.Code != http.StatusBadRequest || !strings.Contains(errorText(t, rec), "для двух колонок") {
		t.Fatalf("a field on two columns: %d %s", rec.Code, rec.Body.String())
	}
	var remapped importResp
	admin.json(http.MethodPatch, "/api/admin/catalog/imports/"+imp.ID, map[string]any{"sheet": 0}, http.StatusOK, &remapped)
	if f := remapped.Columns[0].Field; f == nil || *f != "name" || remapped.Columns[2].Field != nil || remapped.MappingError != "" || remapped.Plan == nil {
		t.Fatalf("sheet mapped anew: %+v", remapped)
	}
	admin.json(http.MethodDelete, "/api/admin/catalog/imports/"+imp.ID, nil, http.StatusNoContent, nil)
	admin.json(http.MethodGet, "/api/admin/catalog/imports/"+imp.ID, nil, http.StatusNotFound, nil)

	huge := strings.Repeat("я", importBodyLimit/2+10)
	big := admin.do(http.MethodPost, "/api/admin/catalog/imports", map[string]any{
		"mode": "catalog", "sheets": []importSheet{{Name: "a", Rows: importers.Grid{{"Название"}, {huge}}}},
	}, false)
	if big.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large file: %d", big.Code)
	}
	wide := importers.Grid{{"Название"}}
	for i := 0; i < importers.MaxGridRows+1; i++ {
		wide = append(wide, []string{"р"})
	}
	if rec := admin.do(http.MethodPost, "/api/admin/catalog/imports", map[string]any{"mode": "catalog", "sheets": []importSheet{{Name: "a", Rows: wide}}}, false); rec.Code != http.StatusBadRequest {
		t.Fatalf("too many rows: %d", rec.Code)
	}
}
