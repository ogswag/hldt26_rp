// Package catalogstore reads the live catalog into the form the engine takes.
package catalogstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/catalog"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/engine"
	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/simbuild"
)

// SolutionsCap is how many rows the catalog may hold for matching to stay quick.
const SolutionsCap = 500

// contentRow is one solution as the checksum covers it.
type contentRow struct {
	ID              string                         `json:"id"`
	Name            string                         `json:"name"`
	Vendor          *string                        `json:"vendor"`
	Kind            *string                        `json:"kind"`
	Subtype         *string                        `json:"subtype"`
	Status          *string                        `json:"status"`
	Industry        *string                        `json:"industry"`
	Scenario        *string                        `json:"scenario"`
	PriceRub        *float64                       `json:"price_rub"`
	SourceURL       *string                        `json:"source_url"`
	PayloadKg       *float64                       `json:"payload_kg"`
	MassKg          *float64                       `json:"mass_kg"`
	LengthMm        *float64                       `json:"length_mm"`
	WidthMm         *float64                       `json:"width_mm"`
	HeightMm        *float64                       `json:"height_mm"`
	SpeedMps        *float64                       `json:"speed_mps"`
	EnduranceH      *float64                       `json:"endurance_h"`
	ChargeMin       *float64                       `json:"charge_min"`
	NavType         *string                        `json:"nav_type"`
	PosAccuracyMm   *float64                       `json:"pos_accuracy_mm"`
	MinAisleMm      *float64                       `json:"min_aisle_mm"`
	TurnRadiusMm    *float64                       `json:"turn_radius_mm"`
	TempMinC        *float64                       `json:"temp_min_c"`
	TempMaxC        *float64                       `json:"temp_max_c"`
	LifetimeYears   *float64                       `json:"lifetime_years"`
	ServicePctYear  *float64                       `json:"service_pct_year"`
	Confidence      *string                        `json:"confidence"`
	SourcedAt       *time.Time                     `json:"sourced_at"`
	Family          string                         `json:"family"`
	Description     string                         `json:"description"`
	ObjectTypes     []string                       `json:"object_types"`
	Uses            []matching.Use                 `json:"uses"`
	CapabilityCodes []string                       `json:"capability_codes"`
	FieldSources    map[string]catalog.FieldSource `json:"field_sources"`
	Raw             json.RawMessage                `json:"raw"`
	Archived        bool                           `json:"archived,omitempty"`
}

// Load reads the live catalog, archived robots included, since fleets and runs may name them. ContentSHA256 names
// what calculations read, so a run records which catalog it priced with; photos and groups are not part of it.
func Load(ctx context.Context, q *db.Queries) (engine.Catalog, error) {
	rows, err := q.ListSolutions(ctx, SolutionsCap+1)
	if err != nil {
		return engine.Catalog{}, fmt.Errorf("catalog.load: %w", err)
	}
	if len(rows) > SolutionsCap {
		return engine.Catalog{}, fmt.Errorf("catalog.load: more than %d rows, the matching limit", SolutionsCap)
	}
	cat := engine.Catalog{
		Candidates: make([]matching.Candidate, 0, len(rows)),
		Robots:     make([]econ.Robot, 0, len(rows)),
	}
	content := make([]contentRow, 0, len(rows))
	for _, row := range rows {
		c := toContent(row)
		content = append(content, c)
		cat.Candidates = append(cat.Candidates, candidate(c))
		cat.Robots = append(cat.Robots, robot(c))
	}
	sort.Slice(content, func(i, j int) bool { return content[i].ID < content[j].ID })
	canon, err := json.Marshal(content)
	if err != nil {
		return engine.Catalog{}, fmt.Errorf("catalog.load.hash: %w", err)
	}
	sum := sha256.Sum256(canon)
	cat.ContentSHA256 = hex.EncodeToString(sum[:])
	n, sha, err := LoadNorms(ctx, q)
	if err != nil {
		return engine.Catalog{}, err
	}
	cat.Norms = n
	cat.NormsSHA256 = sha
	return cat, nil
}

// LoadNorms reads the admin defaults. An empty row is DefaultNorms.
func LoadNorms(ctx context.Context, q *db.Queries) (econ.Norms, string, error) {
	n := econ.DefaultNorms()
	row, err := q.GetCalcNorms(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return n, HashNorms(n), nil
		}
		return econ.Norms{}, "", fmt.Errorf("catalog.norms: %w", err)
	}
	if len(row.Values) > 0 && string(row.Values) != "null" && string(row.Values) != "{}" {
		if err := json.Unmarshal(row.Values, &n); err != nil {
			return econ.Norms{}, "", fmt.Errorf("catalog.norms.parse: %w", err)
		}
	}
	return n, HashNorms(n), nil
}

// HashNorms names the numbers a calculation used, the same way ContentSHA256 names the catalog.
func HashNorms(n econ.Norms) string {
	b, err := json.Marshal(n)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// SaveNorms writes the admin defaults.
func SaveNorms(ctx context.Context, q *db.Queries, n econ.Norms, by *uuid.UUID) error {
	raw, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("catalog.norms.save: %w", err)
	}
	_, err = q.UpsertCalcNorms(ctx, db.UpsertCalcNormsParams{Values: raw, UpdatedBy: by})
	if err != nil {
		return fmt.Errorf("catalog.norms.save: %w", err)
	}
	return nil
}

// RobotsFor reads only the robots the ids name, for a run or a map check that needs its own fleet and not the
// whole catalog. An id that is not there is left out, and the run says so.
func RobotsFor(ctx context.Context, q *db.Queries, ids []string) (simbuild.Robots, error) {
	out := make(simbuild.Robots, len(ids))
	for _, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		row, err := q.GetSolution(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("catalog.robots: %w", err)
		}
		c := toContent(db.ListSolutionsRow(row))
		out[c.ID] = simbuild.NewRobot(candidate(c), robot(c))
	}
	return out, nil
}

func toContent(row db.ListSolutionsRow) contentRow {
	meta := catalog.ParseRaw(row.Raw)
	c := contentRow{
		ID:             row.ID.String(),
		Name:           row.Name,
		Vendor:         row.Vendor,
		Kind:           row.Kind,
		Subtype:        row.Subtype,
		Status:         row.Status,
		Industry:       row.Industry,
		Scenario:       row.Scenario,
		PriceRub:       numericPtr(row.PriceRub),
		SourceURL:      row.SourceUrl,
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
		Family:         meta.Family,
		Description:    meta.Description,
		ObjectTypes:    meta.ObjectTypes,
		Uses:           make([]matching.Use, 0, len(meta.Uses)),
		FieldSources:   catalog.ParseFieldSources(row.FieldSources),
		Raw:            row.Raw,
		Archived:       row.ArchivedAt.Valid,
	}
	if c.ObjectTypes == nil {
		c.ObjectTypes = []string{}
	}
	for _, u := range meta.Uses {
		c.Uses = append(c.Uses, matching.Use{Industry: u.Industry, Scenario: u.Scenario, Cases: u.Cases})
	}
	if len(c.Raw) == 0 {
		c.Raw = json.RawMessage("{}")
	}
	c.CapabilityCodes = catalog.DeriveCapabilities(catalog.SpecInput{
		Name:        row.Name,
		Family:      meta.Family,
		Subtype:     deref(row.Subtype),
		Scenario:    deref(row.Scenario),
		ObjectTypes: meta.ObjectTypes,
		PayloadKg:   c.PayloadKg,
		WidthMm:     c.WidthMm,
		MinAisleMm:  c.MinAisleMm,
		TempMinC:    c.TempMinC,
		TempMaxC:    c.TempMaxC,
	})
	return c
}

func candidate(c contentRow) matching.Candidate {
	var uses []matching.Use
	if len(c.Uses) > 0 {
		uses = c.Uses
	}
	return matching.Candidate{
		ID:              c.ID,
		Name:            c.Name,
		Vendor:          c.Vendor,
		Kind:            c.Kind,
		Subtype:         c.Subtype,
		Scenario:        c.Scenario,
		Family:          c.Family,
		Description:     c.Description,
		PriceRub:        c.PriceRub,
		Confidence:      c.Confidence,
		SourceURL:       c.SourceURL,
		ObjectTypes:     c.ObjectTypes,
		Uses:            uses,
		PayloadKg:       c.PayloadKg,
		MassKg:          c.MassKg,
		LengthMm:        c.LengthMm,
		WidthMm:         c.WidthMm,
		HeightMm:        c.HeightMm,
		MinAisleMm:      c.MinAisleMm,
		TurnRadiusMm:    c.TurnRadiusMm,
		TempMinC:        c.TempMinC,
		TempMaxC:        c.TempMaxC,
		CapabilityCodes: c.CapabilityCodes,
		FieldSources:    c.FieldSources,
		Archived:        c.Archived,
	}
}

func robot(c contentRow) econ.Robot {
	return econ.Robot{
		ID:             c.ID,
		Name:           c.Name,
		Kind:           c.Kind,
		Subtype:        c.Subtype,
		Family:         c.Family,
		Scenario:       c.Scenario,
		Description:    c.Description,
		PriceRub:       c.PriceRub,
		SpeedMps:       c.SpeedMps,
		WidthMm:        c.WidthMm,
		LengthMm:       c.LengthMm,
		PayloadKg:      c.PayloadKg,
		LifetimeYears:  c.LifetimeYears,
		ServicePctYear: c.ServicePctYear,
		EnduranceH:     c.EnduranceH,
		ChargeMin:      c.ChargeMin,
	}
}

func numericPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

func timestamptzPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
