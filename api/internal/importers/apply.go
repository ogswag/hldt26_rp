package importers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"moscow_hackathon_2026/api/internal/db"
)

// ErrStale means the catalog changed after the preview; the admin has to check the upload again.
var ErrStale = errors.New("importers.apply: catalog changed since the preview")

// Pick values for a conflicting field.
const (
	PickFile    = "file"
	PickCatalog = "catalog"
)

// Selection is what the admin chose in the preview.
type Selection struct {
	// Rows maps the key of each row to apply to the Rev the admin saw ("" for a new robot).
	Rows map[string]string `json:"rows"`
	// Fields are the field codes to take from the file; organizer uses go with «cases».
	Fields []string `json:"fields"`
	// Picks choose a side for conflicting fields, by row key and field code; the catalog side is the default.
	Picks map[string]map[string]string `json:"picks"`
	// Archive lists missing robots to move to the archive.
	Archive []string `json:"archive"`
}

// RobotChange is one robot the upload created or updated.
type RobotChange struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Changes []Change `json:"changes"`
}

// PhotoTask is a photo link to fetch for a robot once the upload is saved.
type PhotoTask struct {
	SolutionID string `json:"solution_id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
}

// Applied reports what an upload changed.
type Applied struct {
	Created  []RobotChange `json:"created"`
	Updated  []RobotChange `json:"updated"`
	Archived []RobotRef    `json:"archived"`
	Photos   []PhotoTask   `json:"photos"`
}

// Apply writes the selected rows of a plan. Call it in one transaction with a plan built inside it.
func Apply(ctx context.Context, q *db.Queries, plan Plan, sel Selection) (Applied, error) {
	out := Applied{Created: []RobotChange{}, Updated: []RobotChange{}, Archived: []RobotRef{}, Photos: []PhotoTask{}}
	take := func(code string) bool {
		if code == UsesKey {
			code = "cases"
		}
		return slices.Contains(sel.Fields, code)
	}
	for _, rp := range plan.Rows {
		rev, ok := sel.Rows[rp.Key]
		if !ok || rp.Class == ClassError || rp.Class == ClassUnchanged && rp.Photo == "" {
			continue
		}
		if rev != rp.Rev {
			return Applied{}, ErrStale
		}
		patch := map[string]any{}
		for _, d := range rp.Fields {
			if !take(d.Code) && !(rp.Class == ClassNew && d.Code == "name") {
				continue
			}
			if d.Conflict && sel.Picks[rp.Key][d.Code] != PickFile {
				continue
			}
			patch[d.Code] = d.File
		}
		var (
			change RobotChange
			err    error
		)
		if rp.Class == ClassNew {
			change, err = createRobot(ctx, q, rp, patch)
			if err != nil {
				return Applied{}, err
			}
			out.Created = append(out.Created, change)
		} else {
			change, err = updateRobot(ctx, q, rp, patch)
			if err != nil {
				return Applied{}, err
			}
			if len(change.Changes) > 0 {
				out.Updated = append(out.Updated, change)
			}
		}
		if rp.Photo != "" && take("photo") {
			out.Photos = append(out.Photos, PhotoTask{SolutionID: change.ID, Name: change.Name, URL: rp.Photo})
		}
	}
	missing := map[string]string{}
	for _, m := range plan.Missing {
		missing[m.ID] = m.Name
	}
	for _, id := range sel.Archive {
		name, ok := missing[id]
		if !ok {
			return Applied{}, ErrStale
		}
		uid, err := uuid.Parse(id)
		if err != nil {
			return Applied{}, ErrStale
		}
		n, err := q.ArchiveSolution(ctx, uid)
		if err != nil {
			return Applied{}, fmt.Errorf("importers.apply.archive %s: %w", id, err)
		}
		if n > 0 {
			out.Archived = append(out.Archived, RobotRef{ID: id, Name: name})
		}
	}
	return out, nil
}

func createRobot(ctx context.Context, q *db.Queries, rp RowPlan, patch map[string]any) (RobotChange, error) {
	id := uuid.New()
	if rp.ID != "" {
		parsed, err := uuid.Parse(rp.ID)
		if err != nil {
			return RobotChange{}, ErrStale
		}
		id = parsed
	}
	name, _ := patch["name"].(string)
	if err := q.CreateSolution(ctx, db.CreateSolutionParams{ID: id, Name: name, Raw: json.RawMessage(`{}`)}); err != nil {
		if isUniqueViolation(err) {
			return RobotChange{}, ErrStale
		}
		return RobotChange{}, fmt.Errorf("importers.apply.create: %w", err)
	}
	if err := q.InsertSolutionSpecs(ctx, id); err != nil {
		return RobotChange{}, fmt.Errorf("importers.apply.create: %w", err)
	}
	return saveRobot(ctx, q, id, patch)
}

func updateRobot(ctx context.Context, q *db.Queries, rp RowPlan, patch map[string]any) (RobotChange, error) {
	id, err := uuid.Parse(rp.ID)
	if err != nil {
		return RobotChange{}, ErrStale
	}
	if _, err := q.LockSolution(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RobotChange{}, ErrStale
		}
		return RobotChange{}, fmt.Errorf("importers.apply.lock: %w", err)
	}
	return saveRobot(ctx, q, id, patch)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func saveRobot(ctx context.Context, q *db.Queries, id uuid.UUID, patch map[string]any) (RobotChange, error) {
	got, err := q.GetSolution(ctx, id)
	if err != nil {
		return RobotChange{}, fmt.Errorf("importers.apply.read: %w", err)
	}
	row := db.ListSolutionsRow(got)
	before := RowValues(row)
	after := cloneValues(before)
	for code, v := range patch {
		setValue(after, code, v)
	}
	if fe := Check(after); fe != nil {
		return RobotChange{}, fe
	}
	change := RobotChange{ID: id.String(), Name: str(after["name"]), Changes: Diff(before, after)}
	if len(change.Changes) == 0 {
		return change, nil
	}
	if err := Save(ctx, q, row, after); err != nil {
		return RobotChange{}, err
	}
	return change, nil
}
