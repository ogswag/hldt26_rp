package importers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/catalog"
	"moscow_hackathon_2026/api/internal/db"
)

// Values are a robot's editable fields by field code. A missing key is an empty field. Numbers are float64 in
// stored units, lists are []string of codes, dates are YYYY-MM-DD.
type Values map[string]any

// UsesKey holds the organizer's uses ([]CatalogUse) in Values read from a file in the organizer's format.
const UsesKey = "uses"

// RowValues reads a stored robot.
func RowValues(row db.ListSolutionsRow) Values {
	v := Values{}
	set := func(code string, x any) {
		switch t := x.(type) {
		case *string:
			if t != nil && strings.TrimSpace(*t) != "" {
				v[code] = *t
			}
		case string:
			if strings.TrimSpace(t) != "" {
				v[code] = t
			}
		case pgtype.Numeric:
			if f, err := t.Float64Value(); err == nil && f.Valid {
				v[code] = f.Float64
			}
		}
	}
	set("name", row.Name)
	set("vendor", row.Vendor)
	set("family", row.Family)
	set("kind", row.Kind)
	set("subtype", row.Subtype)
	set("status", row.Status)
	set("industry", row.Industry)
	set("scenario", row.Scenario)
	set("price_rub", row.PriceRub)
	set("source_url", row.SourceUrl)
	set("payload_kg", row.PayloadKg)
	set("mass_kg", row.MassKg)
	set("length_mm", row.LengthMm)
	set("width_mm", row.WidthMm)
	set("height_mm", row.HeightMm)
	set("speed_mps", row.SpeedMps)
	set("endurance_h", row.EnduranceH)
	set("charge_min", row.ChargeMin)
	set("nav_type", row.NavType)
	set("pos_accuracy_mm", row.PosAccuracyMm)
	set("min_aisle_mm", row.MinAisleMm)
	set("turn_radius_mm", row.TurnRadiusMm)
	set("temp_min_c", row.TempMinC)
	set("temp_max_c", row.TempMaxC)
	set("lifetime_years", row.LifetimeYears)
	set("service_pct_year", row.ServicePctYear)
	set("confidence", row.Confidence)
	if row.SourcedAt.Valid {
		v["sourced_at"] = row.SourcedAt.Time.UTC().Format(time.DateOnly)
	}
	raw := rawMap(row.Raw)
	for _, f := range fields {
		if f.store != storeRaw {
			continue
		}
		if x := rawValue(f, raw[f.rawKey]); x != nil {
			v[f.Code] = x
		}
	}
	if uses := rawUses(raw[UsesKey]); len(uses) > 0 {
		v[UsesKey] = uses
	}
	return v
}

func rawMap(raw json.RawMessage) map[string]any {
	m := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}

// rawValue reads a field kept in the raw payload, where the organizer file stored every value as text.
func rawValue(f Field, x any) any {
	switch f.Kind {
	case FieldNumber:
		switch t := x.(type) {
		case float64:
			return t
		case string:
			n, err := strconv.ParseFloat(strings.Replace(strings.TrimSpace(t), ",", ".", 1), 64)
			if err == nil {
				return n
			}
		}
	case FieldChoices:
		list, ok := x.([]any)
		if !ok {
			return nil
		}
		codes := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok && f.hasChoice(s) {
				codes = append(codes, s)
			}
		}
		return f.orderChoices(codes)
	default:
		if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return nil
}

func rawUses(x any) []CatalogUse {
	if x == nil {
		return nil
	}
	b, err := json.Marshal(x)
	if err != nil {
		return nil
	}
	var uses []CatalogUse
	if json.Unmarshal(b, &uses) != nil {
		return nil
	}
	return uses
}

// Equal compares two values of a field the way a person would: empty equals empty, numbers within rounding.
func Equal(code string, a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	switch x := a.(type) {
	case float64:
		y, ok := b.(float64)
		return ok && math.Abs(x-y) <= 1e-9*math.Max(1, math.Max(math.Abs(x), math.Abs(y)))
	case string:
		y, ok := b.(string)
		return ok && strings.TrimSpace(x) == strings.TrimSpace(y)
	case []string:
		y, ok := b.([]string)
		return ok && slices.Equal(x, y)
	}
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(ja) == string(jb)
}

// Check tests rules that tie fields together. It returns nil when the values can be saved.
func Check(v Values) *FieldError {
	if name, _ := v["name"].(string); strings.TrimSpace(name) == "" {
		f, _ := FieldByCode("name")
		return f.fail("укажите название решения.")
	}
	lo, okLo := v["temp_min_c"].(float64)
	hi, okHi := v["temp_max_c"].(float64)
	if okLo && okHi && lo > hi {
		f, _ := FieldByCode("temp_min_c")
		return f.fail("нижняя граница выше верхней. Проверьте обе температуры.")
	}
	return nil
}

// Change is one field's value before and after a save.
type Change struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// Diff lists the editable fields whose values differ, in field order; organizer uses come last.
func Diff(before, after Values) []Change {
	var out []Change
	for _, f := range fields {
		if !f.Editable {
			continue
		}
		if !Equal(f.Code, before[f.Code], after[f.Code]) {
			out = append(out, Change{Field: f.Code, Before: before[f.Code], After: after[f.Code]})
		}
	}
	if _, ok := after[UsesKey]; ok && !Equal(UsesKey, before[UsesKey], after[UsesKey]) {
		out = append(out, Change{Field: UsesKey, Before: before[UsesKey], After: after[UsesKey]})
	}
	return out
}

// Save writes after over the stored robot row. The raw payload keeps keys no field owns; the organizer's «Тип»
// label changes only when the group does, since matching reads it.
func Save(ctx context.Context, q *db.Queries, row db.ListSolutionsRow, after Values) error {
	before := RowValues(row)
	raw := rawMap(row.Raw)
	for _, f := range fields {
		if f.store != storeRaw {
			continue
		}
		if x, ok := after[f.Code]; ok && x != nil {
			raw[f.rawKey] = x
		} else {
			delete(raw, f.rawKey)
		}
	}
	if uses, ok := after[UsesKey]; ok {
		raw[UsesKey] = uses
	}
	if fam, _ := after["family"].(string); !Equal("family", before["family"], after["family"]) {
		if fam == "" {
			delete(raw, "Тип")
		} else {
			raw["Тип"] = catalog.FamilyLabel(fam)
		}
	}
	rawJSON, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("importers.save.raw: %w", err)
	}
	price, err := numericValue(after["price_rub"])
	if err != nil {
		return fmt.Errorf("importers.save.price: %w", err)
	}
	name, _ := after["name"].(string)
	if err := q.SaveSolution(ctx, db.SaveSolutionParams{
		ID:        row.ID,
		Name:      name,
		Vendor:    textValue(after["vendor"]),
		Kind:      textValue(after["kind"]),
		Subtype:   textValue(after["subtype"]),
		Status:    textValue(after["status"]),
		Industry:  textValue(after["industry"]),
		Scenario:  textValue(after["scenario"]),
		PriceRub:  price,
		SourceUrl: textValue(after["source_url"]),
		Raw:       rawJSON,
		Family:    textValue(after["family"]),
	}); err != nil {
		return fmt.Errorf("importers.save: %w", err)
	}
	specs, err := SpecsParams(row.ID, after)
	if err != nil {
		return err
	}
	if err := q.UpsertSolutionSpecs(ctx, specs); err != nil {
		return fmt.Errorf("importers.save.specs: %w", err)
	}
	// A value an admin changed no longer comes from the source recorded for the old one.
	var changed []string
	for _, code := range sourcedCodes() {
		if !Equal(code, before[code], after[code]) {
			changed = append(changed, code)
		}
	}
	if len(changed) > 0 {
		if err := q.DropFieldSources(ctx, db.DropFieldSourcesParams{ID: row.ID, Codes: changed}); err != nil {
			return fmt.Errorf("importers.save.sources: %w", err)
		}
	}
	return nil
}

// sourcedCodes are the fields that can carry a source of their own: the price and the specs.
func sourcedCodes() []string {
	out := []string{"price_rub"}
	for _, f := range fields {
		if f.store == storeSpec && f.Code != "confidence" && f.Code != "sourced_at" {
			out = append(out, f.Code)
		}
	}
	return out
}

// SpecsParams builds the specs row of a robot from its values.
func SpecsParams(id uuid.UUID, v Values) (db.UpsertSolutionSpecsParams, error) {
	p := db.UpsertSolutionSpecsParams{
		SolutionID: id,
		NavType:    textValue(v["nav_type"]),
		Confidence: textValue(v["confidence"]),
	}
	if s, ok := v["sourced_at"].(string); ok {
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			return p, fmt.Errorf("importers.save.sourced_at: %w", err)
		}
		p.SourcedAt = pgtype.Timestamptz{Time: t, Valid: true}
	}
	for _, n := range []struct {
		code string
		dst  *pgtype.Numeric
	}{
		{"payload_kg", &p.PayloadKg}, {"mass_kg", &p.MassKg}, {"length_mm", &p.LengthMm}, {"width_mm", &p.WidthMm},
		{"height_mm", &p.HeightMm}, {"speed_mps", &p.SpeedMps}, {"endurance_h", &p.EnduranceH},
		{"charge_min", &p.ChargeMin}, {"pos_accuracy_mm", &p.PosAccuracyMm}, {"min_aisle_mm", &p.MinAisleMm},
		{"turn_radius_mm", &p.TurnRadiusMm}, {"temp_min_c", &p.TempMinC}, {"temp_max_c", &p.TempMaxC}, {"lifetime_years", &p.LifetimeYears},
		{"service_pct_year", &p.ServicePctYear},
	} {
		x, err := numericValue(v[n.code])
		if err != nil {
			return p, fmt.Errorf("importers.save.%s: %w", n.code, err)
		}
		*n.dst = x
	}
	return p, nil
}

func textValue(x any) *string {
	s, ok := x.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func numericValue(x any) (pgtype.Numeric, error) {
	f, ok := x.(float64)
	if !ok {
		return pgtype.Numeric{}, nil
	}
	return numericFromFloat(&f)
}
