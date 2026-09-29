//go:build js && wasm

// Command engine-wasm exposes the calculation engine to the browser. Every method takes one JSON string and
// returns one JSON string, so the worker on the other side needs no type bridge.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"syscall/js"
	"time"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/engine"
	"moscow_hackathon_2026/api/internal/objects"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/sim"
	"moscow_hackathon_2026/api/internal/simbuild"
)

// Version is what the page checks against the server, so a stale build is never mistaken for a fresh one.
const Version = "engine-wasm-8"

type request struct {
	Catalog    engine.Catalog  `json:"catalog"`
	ObjectType string          `json:"object_type"`
	Params     json.RawMessage `json:"params"`
	IncludeIDs []string        `json:"include_ids"`
	TaskCodes  []string        `json:"task_codes"`
	Seed       int             `json:"seed"`
	Overrides  *econ.Overrides `json:"overrides"`
	Draft      json.RawMessage `json:"draft"`
	Document   json.RawMessage `json:"document"`

	// Collections is the project as its records. It replaces object_type, params, draft and the what-if.
	Collections ops.State  `json:"collections"`
	VariantID   string     `json:"variant_id"`
	Sim         sim.Config `json:"sim"`
	WithLog     bool       `json:"with_log"`
	Mode        string     `json:"mode"`
	LineKey     string     `json:"line_key"`
	// Runs are the finished simulations kept in the browser, newest first, for simChecks.
	Runs []simbuild.RunBrief `json:"runs"`
}

func main() {
	js.Global().Set("robotsEngine", js.ValueOf(map[string]any{
		"version":        Version,
		"modelVersion":   econ.ModelVersion,
		"simVersion":     sim.Version,
		"validateParams": method(validateParams),
		"match":          method(match),
		"calculate":      method(calculate),
		"simulateMap":    method(simulateMap),
		"checkMap":       method(checkMap),
		"mapTemplate":    method(mapTemplate),
		"draftHash":      method(draftHash),
		"simChecks":      method(simChecks),
		"variantHashes":  method(variantHashes),
	}))
	select {}
}

// method turns a Go function into the JS one: one JSON string in, one JSON string out. A panic inside the
// engine comes back as an error instead of killing the instance.
func method(fn func(request) (any, error)) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) (out any) {
		defer func() {
			if r := recover(); r != nil {
				out = fail(fmt.Errorf("engine: %v", r))
			}
		}()
		if len(args) == 0 || args[0].Type() != js.TypeString {
			return fail(fmt.Errorf("engine: expected one JSON string"))
		}
		var req request
		if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
			return fail(err)
		}
		res, err := fn(req)
		if err != nil {
			return fail(err)
		}
		body, err := json.Marshal(map[string]any{"ok": true, "result": res})
		if err != nil {
			return fail(err)
		}
		return string(body)
	})
}

func fail(err error) string {
	body, mErr := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
	if mErr != nil {
		return `{"ok":false,"error":"engine: cannot encode the error"}`
	}
	return string(body)
}

// toEngineRequest fills the schema defaults and validates, exactly as the API does before it calculates, so
// a half-filled form gives the browser the same numbers as the server.
func toEngineRequest(req request) (engine.Request, error) {
	params, err := filled(req.ObjectType, req.Params)
	if err != nil {
		return engine.Request{}, err
	}
	out := engine.Request{
		ObjectType: req.ObjectType,
		Params:     params,
		IncludeIDs: req.IncludeIDs,
		TaskCodes:  req.TaskCodes,
		Seed:       req.Seed,
	}
	if req.Overrides != nil {
		out.Overrides = *req.Overrides
	}
	return out, nil
}

func filled(objectType string, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	out, err := objects.FillDefaults(objectType, raw)
	if err != nil {
		return nil, err
	}
	if err := objects.Validate(objectType, out); err != nil {
		return nil, err
	}
	return out, nil
}

func validateParams(req request) (any, error) {
	if err := objects.Validate(req.ObjectType, req.Params); err != nil {
		var ve *objects.ValidationError
		if errors.As(err, &ve) {
			return map[string]any{"ok": false, "details": ve.Details}, nil
		}
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func match(req request) (any, error) {
	params, err := filled(req.ObjectType, req.Params)
	if err != nil {
		return nil, err
	}
	taskCodes := req.TaskCodes
	if len(taskCodes) == 0 {
		taskCodes = engine.TaskCodes(req.ObjectType, nil, nil)
	}
	return engine.Match(req.Catalog, req.ObjectType, params, req.IncludeIDs, taskCodes)
}

func calculate(req request) (any, error) {
	if len(req.Collections) > 0 {
		return calculateProject(req)
	}
	in, err := toEngineRequest(req)
	if err != nil {
		return nil, err
	}
	if len(req.Draft) > 0 && string(req.Draft) != "null" {
		if err := json.Unmarshal(req.Draft, &in.Draft); err != nil {
			return nil, err
		}
		in.HasDraft = true
	}
	res, _, err := engine.Calculate(req.Catalog, in)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// calculateProject calculates a project given as records, as the guest endpoint does for a browser project.
func calculateProject(req request) (any, error) {
	d, err := simbuild.DraftOf(req.Collections)
	if err != nil {
		return nil, err
	}
	params, err := filled(d.ObjectType, d.Params)
	if err != nil {
		return nil, err
	}
	d.Params = params
	ov, err := engine.OverridesOf(req.Overrides, d)
	if err != nil {
		return nil, err
	}
	include := req.IncludeIDs
	if include == nil {
		include = d.MatchSelectedIDs
	}
	res, _, err := engine.Calculate(req.Catalog, engine.Request{
		ObjectType: d.ObjectType, Params: params, IncludeIDs: include, Seed: req.Seed, Overrides: ov, Draft: d, HasDraft: true,
	})
	return res, err
}

func simulateMap(req request) (any, error) {
	return simbuild.RunLocal(context.Background(), simbuild.LocalRequest{
		Catalog:     req.Catalog,
		Collections: req.Collections,
		VariantID:   req.VariantID,
		Sim:         req.Sim,
		Seed:        req.Seed,
		WithLog:     req.WithLog,
		Mode:        req.Mode,
		LineKey:     req.LineKey,
	}, time.Now)
}

func checkMap(req request) (any, error) {
	return simbuild.LocalMapCheck(req.Catalog, req.Collections, req.Document)
}

func mapTemplate(req request) (any, error) {
	d, err := simbuild.DraftOf(req.Collections)
	if err != nil {
		return nil, err
	}
	return simbuild.Template(d), nil
}

func draftHash(req request) (any, error) {
	return simbuild.DraftHash(req.Collections)
}

func simChecks(req request) (any, error) {
	return simbuild.LocalChecks(req.Collections, req.Runs)
}

func variantHashes(req request) (any, error) {
	return simbuild.LocalVariantHashes(req.Collections)
}
