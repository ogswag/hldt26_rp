package httpserver

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/catalog"
	"moscow_hackathon_2026/api/internal/catalogstore"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/engine"
	"moscow_hackathon_2026/api/internal/importers"
	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/objects"
)

const catalogPageDefault = 50

func (s *Server) listSolutions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	q := strings.TrimSpace(query.Get("q"))
	kind := strings.TrimSpace(query.Get("kind"))
	subtype := strings.TrimSpace(query.Get("subtype"))
	vendor := strings.TrimSpace(query.Get("vendor"))
	objectType := strings.TrimSpace(query.Get("object_type"))
	if objectType != "" && !objects.Valid(objectType) {
		writeError(w, r, http.StatusBadRequest, "Неизвестный тип объекта. Выберите склад, аэропорт или больницу.")
		return
	}
	families := codeList(query.Get("family"))
	for _, f := range families {
		if !catalog.ValidFamily(f) {
			writeError(w, r, http.StatusBadRequest, "Неизвестный тип решения в фильтре. Сбросьте фильтр и выберите тип из списка.")
			return
		}
	}
	statuses := codeList(query.Get("status"))
	specsComplete := false
	switch query.Get("specs_complete") {
	case "", "false", "0":
	case "true", "1":
		specsComplete = true
	default:
		writeError(w, r, http.StatusBadRequest, "specs_complete должен быть true или false.")
		return
	}
	archived := query.Get("archived")
	if archived != "" && archived != "include" {
		writeError(w, r, http.StatusBadRequest, "archived принимает только include.")
		return
	}
	if archived == "include" && !auth.FromRequest(r).IsAdmin() {
		writeError(w, r, http.StatusForbidden, "Архив каталога видят только администраторы.")
		return
	}
	minPayload := strings.TrimSpace(query.Get("min_payload_kg"))
	maxWidth := strings.TrimSpace(query.Get("max_width_mm"))
	sortBy := strings.TrimSpace(query.Get("sort"))
	if sortBy == "" {
		sortBy = "name"
	} else if sortBy != "name" && sortBy != "price_rub" && sortBy != "payload_kg" {
		writeError(w, r, http.StatusBadRequest, "sort должен быть name, price_rub или payload_kg.")
		return
	}
	if !validCatalogNumber(w, r, minPayload, "min_payload_kg") {
		return
	}
	if !validCatalogNumber(w, r, maxWidth, "max_width_mm") {
		return
	}
	limit, ok := queryInt(w, r, "limit", catalogPageDefault, 1, solutionsCap)
	if !ok {
		return
	}
	offset, ok := queryInt(w, r, "offset", 0, 0, 1_000_000)
	if !ok {
		return
	}
	if s.q == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []solutionJSON{}, "total": 0, "limit": limit, "offset": offset})
		return
	}
	rows, err := s.q.ListSolutions(r.Context(), solutionsCap)
	if err != nil {
		s.internal(w, r, "catalog.list", err, "Не удалось загрузить каталог. Повторите запрос.")
		return
	}
	filtered := make([]solutionJSON, 0, len(rows))
	minP, _ := strconv.ParseFloat(minPayload, 64)
	maxW, _ := strconv.ParseFloat(maxWidth, 64)
	ql := strings.ToLower(q)
	for _, row := range rows {
		if row.ArchivedAt.Valid && archived != "include" {
			continue
		}
		item := solutionJSONFromRow(row)
		if kind != "" && deref(item.Kind) != kind {
			continue
		}
		if subtype != "" && deref(item.Subtype) != subtype {
			continue
		}
		if vendor != "" && deref(item.Vendor) != vendor {
			continue
		}
		if objectType != "" && !slices.Contains(item.ObjectTypes, objectType) {
			continue
		}
		if len(families) > 0 && !slices.Contains(families, familyOrOther(item.Family)) {
			continue
		}
		if len(statuses) > 0 && !slices.Contains(statuses, deref(item.Status)) {
			continue
		}
		if specsComplete && item.DataQuality.Status != matching.StatusOK {
			continue
		}
		if minPayload != "" && (item.Specs.PayloadKg == nil || *item.Specs.PayloadKg < minP) {
			continue
		}
		if maxWidth != "" && (item.Specs.WidthMm == nil || *item.Specs.WidthMm > maxW) {
			continue
		}
		if ql != "" {
			blob := strings.ToLower(item.Name + " " + deref(item.Vendor) + " " + deref(item.Kind) + " " + deref(item.Subtype) + " " + deref(item.Modification))
			if !strings.Contains(blob, ql) {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	if sortBy == "price_rub" {
		sort.SliceStable(filtered, func(i, j int) bool {
			return numOrMax(filtered[i].PriceRub) < numOrMax(filtered[j].PriceRub)
		})
	} else if sortBy == "payload_kg" {
		sort.SliceStable(filtered, func(i, j int) bool {
			return numOrMax(filtered[i].Specs.PayloadKg) < numOrMax(filtered[j].Specs.PayloadKg)
		})
	}
	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  filtered[offset:end],
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// codeList splits a comma list of codes from a query parameter.
func codeList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// familyOrOther treats a robot with no group as «Другое», the way the catalog shows it.
func familyOrOther(f *string) string {
	if f == nil || *f == "" {
		return catalog.FamilyOther
	}
	return *f
}

func (s *Server) getSolution(w http.ResponseWriter, r *http.Request) {
	if s.q == nil {
		writeError(w, r, http.StatusNotFound, "Решение не найдено.")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "solutionId"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "Некорректный идентификатор решения.")
		return
	}
	item, err := s.solutionByID(r, id)
	if err != nil {
		if errors.Is(err, errSolutionNotFound) {
			writeError(w, r, http.StatusNotFound, "Решение не найдено.")
			return
		}
		s.internal(w, r, "catalog.get", err, "Не удалось загрузить решение. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// catalogBundle serves the live catalog as the engine takes it, for the browser's offline engine. The ETag is its
// format and content hash, so a browser that has this catalog gets 304.
func (s *Server) catalogBundle(w http.ResponseWriter, r *http.Request) {
	if s.q == nil {
		writeError(w, r, http.StatusNotFound, "Каталог не загружен.")
		return
	}
	cat, err := catalogstore.Load(r.Context(), s.q)
	if err != nil {
		s.internal(w, r, "catalog.bundle", err, "Не удалось собрать каталог. Повторите запрос.")
		return
	}
	etag := `"` + engine.CatalogFormat + "-" + cat.ContentSHA256 + "-" + cat.NormsSHA256 + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(cat)
}

func (s *Server) catalogDictionaries(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"tasks":        catalog.Tasks(),
		"capabilities": catalog.Capabilities(),
		"rules":        catalog.TaskRules(),
		"fields":       importers.Fields(),
	})
}

func solutionJSONFromRow(row db.ListSolutionsRow) solutionJSON {
	specs := specsJSON{
		PayloadKg:      numericPtr(row.PayloadKg),
		MassKg:         numericPtr(row.MassKg),
		LengthMm:       numericPtr(row.LengthMm),
		WidthMm:        numericPtr(row.WidthMm),
		HeightMm:       numericPtr(row.HeightMm),
		SpeedMps:       numericPtr(row.SpeedMps),
		EnduranceH:     numericPtr(row.EnduranceH),
		ChargeMin:      numericPtr(row.ChargeMin),
		NavType:        row.NavType,
		PosAccuracyMm:  numericPtr(row.PosAccuracyMm),
		MinAisleMm:     numericPtr(row.MinAisleMm),
		TurnRadiusMm:   numericPtr(row.TurnRadiusMm),
		TempMinC:       numericPtr(row.TempMinC),
		TempMaxC:       numericPtr(row.TempMaxC),
		LifetimeYears:  numericPtr(row.LifetimeYears),
		ServicePctYear: numericPtr(row.ServicePctYear),
		Confidence:     row.Confidence,
		SourcedAt:      timestamptzPtr(row.SourcedAt),
	}
	vals := importers.RowValues(row)
	text := func(code string) *string {
		if v, ok := vals[code].(string); ok {
			return &v
		}
		return nil
	}
	number := func(code string) *float64 {
		if v, ok := vals[code].(float64); ok {
			return &v
		}
		return nil
	}
	objectTypes, _ := vals["object_types"].([]string)
	if objectTypes == nil {
		objectTypes = []string{}
	}
	return solutionJSON{
		ID:           row.ID.String(),
		Name:         row.Name,
		Vendor:       row.Vendor,
		Kind:         row.Kind,
		Subtype:      row.Subtype,
		Status:       row.Status,
		Industry:     row.Industry,
		Scenario:     row.Scenario,
		PriceRub:     numericPtr(row.PriceRub),
		SourceURL:    row.SourceUrl,
		Family:       row.Family,
		Description:  text("description"),
		ObjectTypes:  objectTypes,
		FieldSources: catalog.ParseFieldSources(row.FieldSources),
		Modification: modification(row),
		Region:       text("region"),
		UGT:          number("ugt"),
		Market:       number("market"),
		ArchivedAt:   timestamptzPtr(row.ArchivedAt),
		ImageSHA:     row.ImageSha,
		Specs:        specs,
		DataQuality: matching.Classify(matching.Specs{
			PayloadKg:  specs.PayloadKg,
			WidthMm:    specs.WidthMm,
			MinAisleMm: specs.MinAisleMm,
			TempMinC:   specs.TempMinC,
			TempMaxC:   specs.TempMaxC,
		}),
	}
}

func modification(row db.ListSolutionsRow) *string {
	if catalog.ParseRaw(row.Raw).SourceID == "" || row.Industry == nil || strings.TrimSpace(*row.Industry) == "" {
		return nil
	}
	return row.Industry
}

func queryInt(w http.ResponseWriter, r *http.Request, key string, def, min, max int) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, key+" должен быть целым числом.")
		return 0, false
	}
	if n < min || n > max {
		writeError(w, r, http.StatusBadRequest, key+" должен быть от "+strconv.Itoa(min)+" до "+strconv.Itoa(max)+".")
		return 0, false
	}
	return n, true
}

func validCatalogNumber(w http.ResponseWriter, r *http.Request, raw, key string) bool {
	if raw == "" {
		return true
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		writeError(w, r, http.StatusBadRequest, key+" должен быть конечным неотрицательным числом.")
		return false
	}
	return true
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func numOrMax(v *float64) float64 {
	if v == nil {
		return 1e18
	}
	return *v
}
