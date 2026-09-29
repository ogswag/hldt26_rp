package importers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/catalog"
	"moscow_hackathon_2026/api/internal/db"
)

type specsFile struct {
	SourcedAt string      `json:"sourced_at"`
	Robots    []robotSeed `json:"robots"`
}

type robotSeed struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	InCatalog   bool           `json:"in_catalog"`
	ObjectTypes []string       `json:"object_types"`
	SourceURL   string         `json:"source_url"`
	SourcedAt   string         `json:"sourced_at"`
	Confidence  string         `json:"confidence"`
	Subtype     string         `json:"subtype"`
	Family      string         `json:"family"`
	Description string         `json:"description"`
	Specs       seedSpecValues `json:"specs"`
	// FieldNotes explain a value that is not copied as the source states it, by field code.
	FieldNotes map[string]string `json:"field_notes"`
	Solution   *extraSolution    `json:"solution"`
}

type extraSolution struct {
	Name     string `json:"name"`
	Vendor   string `json:"vendor"`
	Kind     string `json:"kind"`
	Subtype  string `json:"subtype"`
	Status   string `json:"status"`
	Industry string `json:"industry"`
	Scenario string `json:"scenario"`
}

type seedSpecValues struct {
	PayloadKg      *float64 `json:"payload_kg"`
	MassKg         *float64 `json:"mass_kg"`
	LengthMm       *float64 `json:"length_mm"`
	WidthMm        *float64 `json:"width_mm"`
	HeightMm       *float64 `json:"height_mm"`
	SpeedMps       *float64 `json:"speed_mps"`
	EnduranceH     *float64 `json:"endurance_h"`
	ChargeMin      *float64 `json:"charge_min"`
	NavType        *string  `json:"nav_type"`
	PosAccuracyMm  *float64 `json:"pos_accuracy_mm"`
	MinAisleMm     *float64 `json:"min_aisle_mm"`
	TurnRadiusMm   *float64 `json:"turn_radius_mm"`
	TempMinC       *float64 `json:"temp_min_c"`
	TempMaxC       *float64 `json:"temp_max_c"`
	LifetimeYears  *float64 `json:"lifetime_years"`
	ServicePctYear *float64 `json:"service_pct_year"`
}

// codes lists the fields the seed gives a value, in the order of the fields table.
func (s seedSpecValues) codes() []string {
	var out []string
	for _, c := range []struct {
		code string
		set  bool
	}{
		{"payload_kg", s.PayloadKg != nil}, {"mass_kg", s.MassKg != nil}, {"length_mm", s.LengthMm != nil},
		{"width_mm", s.WidthMm != nil}, {"height_mm", s.HeightMm != nil}, {"speed_mps", s.SpeedMps != nil},
		{"endurance_h", s.EnduranceH != nil}, {"charge_min", s.ChargeMin != nil}, {"nav_type", s.NavType != nil},
		{"pos_accuracy_mm", s.PosAccuracyMm != nil}, {"min_aisle_mm", s.MinAisleMm != nil},
		{"turn_radius_mm", s.TurnRadiusMm != nil}, {"temp_min_c", s.TempMinC != nil}, {"temp_max_c", s.TempMaxC != nil},
		{"lifetime_years", s.LifetimeYears != nil}, {"service_pct_year", s.ServicePctYear != nil},
	} {
		if c.set {
			out = append(out, c.code)
		}
	}
	return out
}

// maxSeedDescription caps a seed description; the catalog shows it whole in the robot overlay.
const maxSeedDescription = 1000

func LoadSpecsFile(path string) (specsFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return specsFile{}, fmt.Errorf("importers.specs.open: %w", err)
	}
	defer f.Close()
	var doc specsFile
	if err := json.NewDecoder(f).Decode(&doc); err != nil {
		return specsFile{}, fmt.Errorf("importers.specs.decode: %w", err)
	}
	if err := validateSpecsFile(doc); err != nil {
		return specsFile{}, err
	}
	return doc, nil
}

// seedSpecs adds the seed robots missing from the organizer file and fills empty fields of the others. A robot
// whose id the file repeats has the same specs in every row of that id.
func seedSpecs(ctx context.Context, q *db.Queries, doc specsFile, edited map[uuid.UUID]bool, siblings map[uuid.UUID][]uuid.UUID) (int, error) {
	extra := 0
	for _, robot := range doc.Robots {
		id, err := uuid.Parse(robot.ID)
		if err != nil {
			return 0, fmt.Errorf("importers.specs.id %s: %w", robot.ID, err)
		}
		if !robot.InCatalog {
			n, err := insertExtra(ctx, q, id, robot)
			if err != nil {
				return 0, err
			}
			extra += n
		}
		for _, target := range append([]uuid.UUID{id}, siblings[id]...) {
			if edited[target] {
				continue
			}
			if err := fillRobot(ctx, q, target, robot, doc.SourcedAt); err != nil {
				return 0, err
			}
		}
	}
	return extra, nil
}

func fillRobot(ctx context.Context, q *db.Queries, id uuid.UUID, robot robotSeed, fileSourcedAt string) error {
	if _, err := q.LockSolution(ctx, id); err != nil {
		// NOTE: a robot deleted before archiving existed stays deleted.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("importers.specs.lock %s: %w", id, err)
	}
	sourced := robot.SourcedAt
	if sourced == "" {
		sourced = fileSourcedAt
	}
	at, err := parseSeedTime(sourced)
	if err != nil {
		return fmt.Errorf("importers.specs.sourced_at %s: %w", id, err)
	}
	var nerr error
	num := func(p *float64) pgtype.Numeric {
		v, err := numericFromFloat(p)
		if err != nil {
			nerr = err
		}
		return v
	}
	s := robot.Specs
	params := db.FillSolutionSpecsParams{
		SolutionID:     id,
		PayloadKg:      num(s.PayloadKg),
		MassKg:         num(s.MassKg),
		LengthMm:       num(s.LengthMm),
		WidthMm:        num(s.WidthMm),
		HeightMm:       num(s.HeightMm),
		SpeedMps:       num(s.SpeedMps),
		EnduranceH:     num(s.EnduranceH),
		ChargeMin:      num(s.ChargeMin),
		NavType:        s.NavType,
		PosAccuracyMm:  num(s.PosAccuracyMm),
		MinAisleMm:     num(s.MinAisleMm),
		TurnRadiusMm:   num(s.TurnRadiusMm),
		TempMinC:       num(s.TempMinC),
		TempMaxC:       num(s.TempMaxC),
		LifetimeYears:  num(s.LifetimeYears),
		ServicePctYear: num(s.ServicePctYear),
		Confidence:     optString(robot.Confidence),
		SourcedAt:      pgtype.Timestamptz{Time: at, Valid: true},
	}
	if nerr != nil {
		return fmt.Errorf("importers.specs.fill %s: %w", id, nerr)
	}
	if err := q.FillSolutionSpecs(ctx, params); err != nil {
		return fmt.Errorf("importers.specs.fill %s: %w", id, err)
	}
	sources := map[string]catalog.FieldSource{}
	for _, code := range s.codes() {
		sources[code] = catalog.FieldSource{
			SourceURL: robot.SourceURL, Confidence: robot.Confidence, Note: robot.FieldNotes[code], SourcedAt: at.Format(time.DateOnly),
		}
	}
	if err := fillFieldSources(ctx, q, id, sources); err != nil {
		return err
	}
	if url := optString(robot.SourceURL); url != nil {
		if err := q.FillSolutionSourceURL(ctx, db.FillSolutionSourceURLParams{ID: id, SourceUrl: url}); err != nil {
			return fmt.Errorf("importers.specs.source %s: %w", id, err)
		}
	}
	if len(robot.ObjectTypes) > 0 {
		raw, err := json.Marshal(robot.ObjectTypes)
		if err != nil {
			return fmt.Errorf("importers.specs.object_types %s: %w", id, err)
		}
		if err := q.FillSolutionObjectTypes(ctx, db.FillSolutionObjectTypesParams{ID: id, Replacement: raw}); err != nil {
			return fmt.Errorf("importers.specs.object_types %s: %w", id, err)
		}
	}
	text := db.FillSolutionTextParams{
		ID:          id,
		Subtype:     optString(Text(robot.Subtype)),
		Family:      optString(robot.Family),
		Description: optString(Text(robot.Description)),
	}
	if text.Subtype != nil || text.Family != nil || text.Description != nil {
		if err := q.FillSolutionText(ctx, text); err != nil {
			return fmt.Errorf("importers.specs.text %s: %w", id, err)
		}
	}
	return nil
}

// fillFieldSources adds the sources a robot has no entry for; an entry that is there stays.
func fillFieldSources(ctx context.Context, q *db.Queries, id uuid.UUID, sources map[string]catalog.FieldSource) error {
	if len(sources) == 0 {
		return nil
	}
	raw, err := json.Marshal(sources)
	if err != nil {
		return fmt.Errorf("importers.sources %s: %w", id, err)
	}
	if err := q.FillFieldSources(ctx, db.FillFieldSourcesParams{ID: id, Sources: raw}); err != nil {
		return fmt.Errorf("importers.sources %s: %w", id, err)
	}
	return nil
}

func insertExtra(ctx context.Context, q *db.Queries, id uuid.UUID, robot robotSeed) (int, error) {
	if robot.Solution == nil {
		return 0, fmt.Errorf("importers.specs.extra %s: missing solution", id)
	}
	sol := robot.Solution
	uses := []CatalogUse{{
		Industry: sol.Industry,
		Scenario: sol.Scenario,
		Cases:    "not in catalog_export_v4",
	}}
	raw, err := json.Marshal(map[string]any{
		"in_catalog_export_v4": false,
		"uses":                 uses,
		"object_types":         robot.ObjectTypes,
	})
	if err != nil {
		return 0, fmt.Errorf("importers.specs.extra %s: raw: %w", id, err)
	}
	name := strings.TrimSpace(sol.Name)
	if name == "" {
		name = strings.TrimSpace(robot.Name)
	}
	family := robot.Family
	if family == "" {
		family = catalog.FamilyCode("", sol.Kind)
	}
	n, err := q.InsertSolution(ctx, db.InsertSolutionParams{
		ID:       id,
		Name:     name,
		Vendor:   optString(sol.Vendor),
		Kind:     optString(sol.Kind),
		Subtype:  optString(sol.Subtype),
		Status:   optString(sol.Status),
		Industry: optString(sol.Industry),
		Scenario: optString(sol.Scenario),
		Raw:      raw,
		Family:   optString(family),
	})
	if err != nil {
		return 0, fmt.Errorf("importers.specs.extra %s: %w", id, err)
	}
	return int(n), nil
}

// parseSeedTime reads an RFC 3339 time or a plain date.
func parseSeedTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(time.DateOnly, s)
}

func validateSpecsFile(doc specsFile) error {
	if doc.SourcedAt == "" {
		return fmt.Errorf("importers.specs: sourced_at required")
	}
	if _, err := parseSeedTime(doc.SourcedAt); err != nil {
		return fmt.Errorf("importers.specs.sourced_at: %w", err)
	}
	if len(doc.Robots) == 0 {
		return fmt.Errorf("importers.specs: robots required")
	}
	seen := make(map[string]struct{}, len(doc.Robots))
	for i, robot := range doc.Robots {
		if robot.ID == "" {
			return fmt.Errorf("importers.specs: robot %d: id required", i)
		}
		if _, err := uuid.Parse(robot.ID); err != nil {
			return fmt.Errorf("importers.specs: robot %d: id: %w", i, err)
		}
		if _, dup := seen[robot.ID]; dup {
			return fmt.Errorf("importers.specs: duplicate id %s", robot.ID)
		}
		seen[robot.ID] = struct{}{}
		switch robot.Confidence {
		case "measured", "vendor", "assumed":
		default:
			return fmt.Errorf("importers.specs: %s: confidence must be measured, vendor, or assumed", robot.ID)
		}
		if !robot.InCatalog && robot.Solution == nil {
			return fmt.Errorf("importers.specs: %s: extra row needs solution", robot.ID)
		}
		if robot.SourcedAt != "" {
			if _, err := parseSeedTime(robot.SourcedAt); err != nil {
				return fmt.Errorf("importers.specs: %s: sourced_at: %w", robot.ID, err)
			}
		}
		if robot.Family != "" && !catalog.ValidFamily(robot.Family) {
			return fmt.Errorf("importers.specs: %s: unknown family %q", robot.ID, robot.Family)
		}
		for _, t := range robot.ObjectTypes {
			switch t {
			case "warehouse", "airport", "hospital":
			default:
				return fmt.Errorf("importers.specs: %s: unknown object type %q", robot.ID, t)
			}
		}
		specCodes := robot.Specs.codes()
		for code, note := range robot.FieldNotes {
			if !slices.Contains(specCodes, code) {
				return fmt.Errorf("importers.specs: %s: field_notes names %q, which has no value", robot.ID, code)
			}
			if strings.ContainsAny(note, "\u2014\u2013\u2026") {
				return fmt.Errorf("importers.specs: %s: field note %q: use a comma or a colon instead of a dash, and ... instead of an ellipsis", robot.ID, code)
			}
		}
		if utf8.RuneCountInString(robot.Description) > maxSeedDescription {
			return fmt.Errorf("importers.specs: %s: description longer than %d characters", robot.ID, maxSeedDescription)
		}
		if strings.ContainsAny(robot.Description+robot.Subtype, "\u2014\u2013\u2026") {
			return fmt.Errorf("importers.specs: %s: use a comma or a colon instead of a dash, and ... instead of an ellipsis", robot.ID)
		}
	}
	return nil
}
