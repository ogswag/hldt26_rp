package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"moscow_hackathon_2026/api/internal/config"
	"moscow_hackathon_2026/api/internal/objects"
)

func testHandler() http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(config.Config{PublicOrigin: "http://localhost"}, log, nil, nil, nil, nil)
}

func TestObjectTypesAndSchema(t *testing.T) {
	h := testHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/object-types", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("object-types status %d body %s", rec.Code, rec.Body.Bytes())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/object-types/warehouse/schema", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("schema status %d body %s", rec.Code, rec.Body.Bytes())
	}
	var schema objects.Schema
	if err := json.Unmarshal(rec.Body.Bytes(), &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Type != objects.Warehouse || len(objects.Fields(schema)) == 0 {
		t.Fatalf("schema %+v", schema)
	}
	var sawBool, sawEnum, sawMulti, sawRange bool
	for _, f := range objects.Fields(schema) {
		switch f.Type {
		case objects.TypeBoolean:
			sawBool = true
		case objects.TypeEnum:
			sawEnum = true
		case objects.TypeMultiEnum:
			sawMulti = true
		case objects.TypeTimeRange:
			sawRange = true
		}
	}
	if !sawBool || !sawEnum || !sawMulti || !sawRange {
		t.Fatalf("warehouse schema types bool=%v enum=%v multi=%v range=%v", sawBool, sawEnum, sawMulti, sawRange)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/object-types/factory/schema", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown type status %d", rec.Code)
	}
}

func TestGuestCannotCreateProject(t *testing.T) {
	h := testHandler()
	body := bytes.NewBufferString(`{"name":"p","object_type":"warehouse"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/projects", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
}

func TestGuestCalculateStubAndRange(t *testing.T) {
	h := testHandler()
	def, err := objects.Defaults(objects.Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"object_type": objects.Warehouse,
		"params":      def,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/guest/calculate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["model_version"] != "econ-v3" {
		t.Fatalf("model %v", got["model_version"])
	}
	if got["object_type"] != "warehouse" {
		t.Fatalf("type %v", got["object_type"])
	}
	if got["verification_flag"] != false {
		t.Fatalf("flag %v", got["verification_flag"])
	}
	scenarios, ok := got["scenarios"].([]any)
	if !ok || len(scenarios) != 3 {
		t.Fatalf("scenarios %v", got["scenarios"])
	}
	base, ok := scenarios[0].(map[string]any)
	if !ok {
		t.Fatalf("baseline %v", scenarios[0])
	}
	if base["opex_year_rub"] == nil {
		t.Fatalf("baseline opex empty %v", base)
	}

	zero := 0.0
	badOv, err := json.Marshal(map[string]any{
		"object_type": objects.Warehouse,
		"params":      def,
		"overrides":   map[string]any{"volume_factor": zero},
	})
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/guest/calculate", bytes.NewReader(badOv))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("override status %d body %s", rec.Code, rec.Body.Bytes())
	}

	def["aisle_working_m"] = 0.5
	bad, err := json.Marshal(map[string]any{
		"object_type": objects.Warehouse,
		"params":      def,
	})
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/guest/calculate", bytes.NewReader(bad))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("range status %d body %s", rec.Code, rec.Body.Bytes())
	}
	var errBody errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatal(err)
	}
	if errBody.Code != "validation_error" || len(errBody.Details) == 0 {
		t.Fatalf("error body %+v", errBody)
	}
}

func TestJSONBodyRejectsTrailingValue(t *testing.T) {
	h := testHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/guest/calculate", bytes.NewBufferString(`{} {}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
}

func TestGuestMatchValidation(t *testing.T) {
	h := testHandler()
	def, err := objects.Defaults(objects.Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"object_type": objects.Warehouse,
		"params":      def,
		"include_ids": []string{"not-a-uuid"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/guest/match", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("include_ids status %d body %s", rec.Code, rec.Body.Bytes())
	}

	okBody, err := json.Marshal(map[string]any{
		"object_type": objects.Warehouse,
		"params":      def,
	})
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/guest/match", bytes.NewReader(okBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("guest match status %d body %s", rec.Code, rec.Body.Bytes())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["weights"]; !ok {
		t.Fatalf("weights missing %v", got)
	}
	items, ok := got["items"].([]any)
	if !ok || len(items) != 0 {
		t.Fatalf("nil db items %v", got["items"])
	}

	for view, want := range map[string]int{"brief": http.StatusOK, "full": http.StatusOK, "short": http.StatusBadRequest} {
		req = httptest.NewRequest(http.MethodPost, "/api/guest/match?view="+view, bytes.NewReader(okBody))
		req.Header.Set("Content-Type", "application/json")
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("view=%s status %d, want %d, body %s", view, rec.Code, want, rec.Body.Bytes())
		}
	}
}

func TestGuestFillsMissingParamsAndClocks(t *testing.T) {
	h := testHandler()
	def, err := objects.Defaults(objects.Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	delete(def, "floor_type")
	delete(def, "has_wms")
	delete(def, "shift_window")
	payload, err := json.Marshal(map[string]any{
		"object_type": objects.Warehouse,
		"params":      def,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/guest/match", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fill match status %d body %s", rec.Code, rec.Body.Bytes())
	}

	full, err := objects.Defaults(objects.Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	full["shift_window"] = map[string]any{"start": "08:00:00", "end": "22:00:00"}
	clockBody, err := json.Marshal(map[string]any{
		"object_type": objects.Warehouse,
		"params":      full,
	})
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/guest/calculate", bytes.NewReader(clockBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("clock calculate status %d body %s", rec.Code, rec.Body.Bytes())
	}

	full["has_wms"] = "Да"
	bad, err := json.Marshal(map[string]any{
		"object_type": objects.Warehouse,
		"params":      full,
	})
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/guest/match", bytes.NewReader(bad))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("string bool status %d body %s", rec.Code, rec.Body.Bytes())
	}
}

func TestGuestMatchRejectsUnknownTaskCode(t *testing.T) {
	h := testHandler()
	def, err := objects.Defaults(objects.Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"object_type": objects.Warehouse,
		"params":      def,
		"task_codes":  []string{"unknown_task"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/guest/match", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
}

func TestCatalogQueryValidation(t *testing.T) {
	h := testHandler()
	cases := []string{
		"/api/solutions?limit=bad",
		"/api/solutions?limit=0",
		"/api/solutions?offset=-1",
		"/api/solutions?min_payload_kg=NaN",
		"/api/solutions?max_width_mm=-1",
		"/api/solutions?object_type=factory",
		"/api/solutions?sort=vendor",
		"/api/solutions?limit=501",
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
			}
		})
	}
}

func TestGuestCannotUseSimulationsOrMaps(t *testing.T) {
	h := testHandler()
	const id = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	cases := []struct {
		method, path, body string
	}{
		{http.MethodPost, "/api/projects/" + id + "/simulations", `{}`},
		{http.MethodGet, "/api/projects/" + id + "/simulations", ""},
		{http.MethodGet, "/api/simulation-jobs/" + id, ""},
		{http.MethodDelete, "/api/simulation-jobs/" + id, ""},
		{http.MethodPost, "/api/simulation-jobs/" + id + "/retry", ""},
		{http.MethodGet, "/api/simulation-runs/" + id, ""},
		{http.MethodGet, "/api/simulation-runs/" + id + "/events", ""},
		{http.MethodPut, "/api/simulation-runs/" + id + "/pin", `{"pinned":true}`},
		{http.MethodPost, "/api/projects/" + id + "/transactions", `{"client_id":"guest","schema_version":1,"txs":[]}`},
		{http.MethodGet, "/api/projects/" + id + "/snapshot", ""},
		{http.MethodPost, "/api/projects/" + id + "/map/validate", `{}`},
		{http.MethodGet, "/api/projects/" + id + "/map/template", ""},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, bytes.NewBufferString(c.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s: status %d body %s", c.method, c.path, rec.Code, rec.Body.Bytes())
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+id+"/sim", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("removed sim-v1 project route still answers: %d", rec.Code)
	}
}

func TestAcceptsGzip(t *testing.T) {
	for header, want := range map[string]bool{
		"gzip, deflate, br": true,
		"br;q=1, gzip;q=0":  false,
		"gzip;q=0.5":        true,
		"identity":          false,
		"*":                 true,
		"":                  false,
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept-Encoding", header)
		if got := acceptsGzip(req); got != want {
			t.Fatalf("%q: got %v", header, got)
		}
	}
}

func TestGuestExportPDFAndXLSX(t *testing.T) {
	h := testHandler()
	def, err := objects.Defaults(objects.Warehouse)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"object_type": objects.Warehouse,
		"params":      def,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/guest/export?format=pdf", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("pdf status %d body %s", rec.Code, rec.Body.Bytes())
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF")) {
		t.Fatalf("not pdf %q", rec.Body.Bytes()[:8])
	}
	req = httptest.NewRequest(http.MethodPost, "/api/guest/export?format=xlsx", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("xlsx status %d body %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Body.Len() < 100 {
		t.Fatalf("short xlsx %d", rec.Body.Len())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/guest/export?format=doc", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("format status %d", rec.Code)
	}
}

func TestGuestCannotProjectExport(t *testing.T) {
	h := testHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/projects/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/export", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
}

func TestGuestCannotProjectMatch(t *testing.T) {
	h := testHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/projects/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/match?view=brief", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
}

func TestUnknownRoutesAnswerJSON(t *testing.T) {
	h := testHandler()
	cases := []struct {
		name   string
		method string
		path   string
		status int
		code   string
	}{
		{"unknown api path", http.MethodGet, "/api/nope", http.StatusNotFound, "not_found"},
		{"unknown root path", http.MethodGet, "/nope", http.StatusNotFound, "not_found"},
		{"wrong method", http.MethodGet, "/api/auth/login", http.StatusMethodNotAllowed, "method_not_allowed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.path, nil)
			req.Header.Set("X-Request-Id", "req-42")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.status {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("content type %q", ct)
			}
			if got := rec.Header().Get("X-Request-Id"); got != "req-42" {
				t.Fatalf("request id header %q", got)
			}
			var body errorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != c.code || body.RequestID != "req-42" || body.Error == "" {
				t.Fatalf("body %+v", body)
			}
		})
	}
}
