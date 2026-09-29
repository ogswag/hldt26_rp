package simbuild

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/engine"
	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/sim"
)

const (
	LocalDone    = "done"
	LocalRefused = "refused"
)

// LocalRequest is one simulation of a project that lives in the browser: its records and the catalog it read.
type LocalRequest struct {
	Catalog     engine.Catalog
	Collections ops.State
	VariantID   string
	Sim         sim.Config
	Seed        int
	WithLog     bool
	Mode        string
	LineKey     string
}

// LocalRun is the answer. A refusal carries what the user can fix; Issues is set when the map is the cause.
type LocalRun struct {
	Status  string       `json:"status"`
	Summary *Summary     `json:"summary,omitempty"`
	Log     *sim.Log     `json:"log,omitempty"`
	Message string       `json:"message,omitempty"`
	Issues  []maps.Issue `json:"issues,omitempty"`
}

// DraftOf rebuilds the draft of a project from its records, with the params the schema fills in.
func DraftOf(state ops.State) (projects.Draft, error) {
	d, _, err := ops.ToDraft(state)
	if err != nil {
		return projects.Draft{}, fmt.Errorf("simbuild.draft: %w", err)
	}
	return d, nil
}

// DraftHash is the hash a run of the draft is checked against, so a page can tell that a result is stale.
func DraftHash(state ops.State) (string, error) {
	d, err := DraftOf(state)
	if err != nil {
		return "", err
	}
	return projects.InputHash(d)
}

// RunLocal builds and runs a simulation the way the job worker does, without a database. A project without a fleet
// runs on the robot a calculation suggests. Anything the user can fix comes back as a refusal, not an error.
func RunLocal(ctx context.Context, req LocalRequest, now func() time.Time) (LocalRun, error) {
	d, err := DraftOf(req.Collections)
	if err != nil {
		return LocalRun{}, err
	}
	hash, err := projects.InputHash(d)
	if err != nil {
		return LocalRun{}, fmt.Errorf("simbuild.local.hash: %w", err)
	}
	params, err := objects.FillDefaults(d.ObjectType, d.Params)
	if err == nil {
		err = objects.Validate(d.ObjectType, params)
	}
	if err != nil {
		var ve *objects.ValidationError
		if errors.As(err, &ve) {
			return LocalRun{Status: LocalRefused, Message: "Параметры объекта не прошли проверку. Откройте вкладку Объект и исправьте отмеченные поля."}, nil
		}
		return LocalRun{}, fmt.Errorf("simbuild.local.params: %w", err)
	}
	d.Params = params

	cfg := JobConfig{VariantID: req.VariantID, Sim: req.Sim, Mode: req.Mode, LineKey: req.LineKey}
	if req.VariantID == "" && !HasFleet(d) && d.ObjectType == "warehouse" {
		ov, err := engine.OverridesOf(nil, d)
		if err != nil {
			return LocalRun{}, err
		}
		res, _, err := engine.Calculate(req.Catalog, engine.Request{
			ObjectType: d.ObjectType, Params: d.Params, IncludeIDs: d.MatchSelectedIDs, Seed: req.Seed, Overrides: ov, Draft: d, HasDraft: true,
		})
		if err != nil {
			return refusal(err)
		}
		if cfg.Suggestion, err = SuggestionOf(res); err != nil {
			return refusal(err)
		}
	}
	start := now()
	var built *Built
	var res sim.Result
	var journal *sim.Log
	var search FleetSearch
	if req.Mode == ModeFleetSearch {
		deadline := start.Add(7 * time.Minute)
		if t, ok := ctx.Deadline(); ok {
			left := t.Sub(start)
			deadline = start.Add(time.Duration(float64(left) * 0.7))
		}
		built, res, _, journal, search, err = SearchFleet(ctx, RobotsOf(req.Catalog), d.ObjectType, d, cfg, nil, deadline)
	} else {
		built, err = Build(RobotsOf(req.Catalog), d.ObjectType, d, cfg)
		if err == nil {
			res, _, journal, err = built.Model.RunAll(ctx, nil)
		}
	}
	if err != nil {
		if errors.Is(err, sim.ErrTooManyEvents) {
			return LocalRun{Status: LocalRefused, Message: "Модель слишком большая для одного прогона. Сократите горизонт или спрос."}, nil
		}
		return refusal(err)
	}
	summary := NewSummary(built, res, SummaryMeta{
		InputHash:            hash,
		CatalogContentSHA256: req.Catalog.ContentSHA256,
		Snapshot:             SnapshotInfo{MatchVersion: projects.MatchVersion, EconVersion: econ.ModelVersion, SimVersion: sim.Version},
	}, start, now())
	if req.Mode == ModeFleetSearch {
		summary.FleetSearch = &search
	}
	out := LocalRun{Status: LocalDone, Summary: &summary}
	if req.WithLog {
		out.Log = journal
	}
	return out, nil
}

func refusal(err error) (LocalRun, error) {
	var me *MapError
	if errors.As(err, &me) {
		return LocalRun{Status: LocalRefused, Message: me.Error(), Issues: me.Issues}, nil
	}
	if msg, ok := UserMessage(err); ok {
		return LocalRun{Status: LocalRefused, Message: msg}, nil
	}
	var ie *econ.InputError
	if errors.As(err, &ie) {
		return LocalRun{Status: LocalRefused, Message: ie.Msg}, nil
	}
	return LocalRun{}, err
}

// LocalMapCheck checks a map document against the fleet of the project's variants, for a project in the browser.
func LocalMapCheck(cat engine.Catalog, state ops.State, doc json.RawMessage) (MapCheck, error) {
	d, err := DraftOf(state)
	if err != nil {
		return MapCheck{}, err
	}
	var document maps.Document
	if err := json.Unmarshal(doc, &document); err != nil {
		return MapCheck{}, fmt.Errorf("simbuild.local.map: %w", err)
	}
	return CheckMap(document, MapContext(RobotsOf(cat), d)), nil
}
