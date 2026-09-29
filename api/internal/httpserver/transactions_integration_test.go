package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
)

type fullProject struct {
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	Processes      []projects.Process       `json:"processes"`
	Variants       []projects.Variant       `json:"variants"`
	SharedCosts    []projects.SharedCost    `json:"shared_costs"`
	AssumptionSets []projects.AssumptionSet `json:"assumption_sets"`
	MatchSelected  []string                 `json:"match_selected_ids"`
	InputHash      string                   `json:"input_hash"`
}

func getFull(t *testing.T, c *itClient, pid string) fullProject {
	t.Helper()
	var p fullProject
	c.json(http.MethodGet, "/api/projects/"+pid, nil, http.StatusOK, &p)
	return p
}

type testTx struct {
	TxID  string   `json:"tx_id"`
	Label string   `json:"label"`
	Ops   []ops.Op `json:"ops"`
}

func newTx(list ...ops.Op) testTx {
	return testTx{TxID: uuid.NewString(), Label: "test", Ops: list}
}

func opSet(coll, id, path string, v any) ops.Op {
	return ops.Op{Op: ops.OpSet, Coll: coll, ID: id, Path: path, Value: mustJSON(v)}
}

func opInsert(coll, id string, v map[string]any) ops.Op {
	return ops.Op{Op: ops.OpInsert, Coll: coll, ID: id, Value: mustJSON(v)}
}

// mapOps turns a whole map document into the operations that write it: the map record field by field, its
// features as inserts, and whatever was there before as deletes. This is what the map page sends.
func mapOps(t *testing.T, before ops.State, doc maps.Document) []ops.Op {
	t.Helper()
	st, err := ops.FromDraft("", projects.Draft{Map: mustJSON(doc)})
	if err != nil {
		t.Fatal(err)
	}
	list := []ops.Op{}
	// Points come first in MapCollections and everything else refers to them, so deleting in that order would
	// cascade the edges and flows away before their own deletes are reached. Empty the map from the far end.
	for i := len(ops.MapCollections) - 1; i >= 0; i-- {
		coll := ops.MapCollections[i]
		if coll == ops.CollMap {
			continue
		}
		for _, id := range before.IDs(coll) {
			list = append(list, ops.Op{Op: ops.OpDelete, Coll: coll, ID: id})
		}
	}
	m := st.Get(ops.CollMap, ops.CollMap)
	for _, field := range []string{"page", "profile", "meters_per_px", "segment", "check"} {
		list = append(list, opSet(ops.CollMap, ops.CollMap, field, m[field]))
	}
	for _, coll := range ops.MapCollections {
		if coll == ops.CollMap {
			continue
		}
		for _, id := range st.IDs(coll) {
			value := map[string]any{}
			for k, v := range st[coll][id] {
				if k != ops.FieldID {
					value[k] = v
				}
			}
			list = append(list, opInsert(coll, id, value))
		}
	}
	return list
}

// putMap replaces the project's map in one transaction and returns the batch result.
func putMap(t *testing.T, c *itClient, pid string, doc maps.Document) txBatchJSON {
	t.Helper()
	var snap snapshotResp
	c.json(http.MethodGet, "/api/projects/"+pid+"/snapshot", nil, http.StatusOK, &snap)
	return postTxs(t, c, pid, newTx(mapOps(t, snap.state(t), doc)...))
}

// clearMap empties the project's map the way the editor's "remove map" does: every feature goes, and the page
// the map hangs on is unset, which is what makes the draft mapless again.
func clearMap(t *testing.T, c *itClient, pid string) txBatchJSON {
	t.Helper()
	var snap snapshotResp
	c.json(http.MethodGet, "/api/projects/"+pid+"/snapshot", nil, http.StatusOK, &snap)
	before := snap.state(t)
	list := []ops.Op{}
	for i := len(ops.MapCollections) - 1; i >= 0; i-- {
		coll := ops.MapCollections[i]
		if coll == ops.CollMap {
			continue
		}
		for _, id := range before.IDs(coll) {
			list = append(list, ops.Op{Op: ops.OpDelete, Coll: coll, ID: id})
		}
	}
	list = append(list, opSet(ops.CollMap, ops.CollMap, "page", nil))
	return postTxs(t, c, pid, newTx(list...))
}

func postTxs(t *testing.T, c *itClient, pid string, txs ...testTx) txBatchJSON {
	t.Helper()
	var out txBatchJSON
	c.json(http.MethodPost, "/api/projects/"+pid+"/transactions", map[string]any{"client_id": "test-client", "schema_version": 1, "txs": txs}, http.StatusOK, &out)
	if len(out.Results) != len(txs) {
		t.Fatalf("results %d for %d txs", len(out.Results), len(txs))
	}
	return out
}

func draftSeq(t *testing.T, env itEnv, pid string) int64 {
	t.Helper()
	var seq int64
	if err := env.db.QueryRow(context.Background(), `SELECT draft_seq FROM projects WHERE id = $1`, pid).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	return seq
}

// lastOrder is a key after every key FromDraft can assign to a list of up to 200 legacy rows.
func lastOrder(t *testing.T) string {
	t.Helper()
	keys, err := ops.KeysAfter("", 200)
	if err != nil {
		t.Fatal(err)
	}
	k, err := ops.KeyBetween(keys[len(keys)-1], "")
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestTransactionsApplyAndReplay(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "tx-owner@example.com")
	p, _ := warehouseProject(t, owner, "Операции")
	before := getFull(t, owner, p.ID)
	base := draftSeq(t, env, p.ID)
	if base == 0 {
		t.Fatal("legacy writes must advance draft_seq")
	}
	variant := before.Variants[0]
	proc := before.Processes[0]
	fleetID := uuid.NewString()
	price := 1234.567

	good1 := newTx(
		opSet(ops.CollProject, ops.CollProject, "name", "Склад после операций"),
		opSet(ops.CollVariants, variant.ID, "name", "Вариант А"),
		opInsert(ops.CollFleetItems, fleetID, map[string]any{
			"order": lastOrder(t), "variant_id": variant.ID, "solution_id": h1500ID, "quantity": 2,
			"price_override_rub": price, "price_override_reason": "КП поставщика", "task_codes": []string{proc.Code},
		}),
	)
	bad := newTx(opSet(ops.CollVariants, variant.ID, "name", ""))
	good2 := newTx(opSet(ops.CollProcesses, proc.ID, "demand.units_per_day", 1234))
	out := postTxs(t, owner, p.ID, good1, bad, good2)

	if r := out.Results[0]; r.Status != ops.StatusApplied || r.Seq == nil || *r.Seq != base+1 {
		t.Fatalf("tx1: %+v", r)
	}
	if r := out.Results[1]; r.Status != ops.StatusRejected || r.Reason != ops.ReasonInvalidValue || r.Details == nil || r.Details.Field != "name" {
		t.Fatalf("tx2: %+v", r)
	}
	if r := out.Results[2]; r.Status != ops.StatusApplied || *r.Seq != base+2 {
		t.Fatalf("tx3: %+v", r)
	}
	if out.Seq != base+2 || draftSeq(t, env, p.ID) != base+2 {
		t.Fatalf("seq %d", out.Seq)
	}

	after := getFull(t, owner, p.ID)
	if after.Name != "Склад после операций" || after.Variants[0].Name != "Вариант А" {
		t.Fatalf("names: %q %q", after.Name, after.Variants[0].Name)
	}
	var item *projects.FleetItem
	for i, f := range after.Variants[0].Fleet {
		if f.ID == fleetID {
			item = &after.Variants[0].Fleet[i]
		}
	}
	if item == nil || item.Quantity != 2 || item.PriceOverrideRub == nil || *item.PriceOverrideRub != price {
		t.Fatalf("fleet item: %+v", item)
	}
	if item.SortOrder != len(after.Variants[0].Fleet)-1 {
		t.Fatalf("inserted item sort_order %d of %d", item.SortOrder, len(after.Variants[0].Fleet))
	}
	for _, pr := range after.Processes {
		if pr.ID == proc.ID && pr.Demand.UnitsPerDay != 1234 {
			t.Fatalf("demand %v", pr.Demand)
		}
	}
	if after.InputHash == before.InputHash {
		t.Fatal("input_hash must change")
	}
	// Ids and order keys survive the write.
	var key string
	if err := env.db.QueryRow(context.Background(), `SELECT order_key FROM variant_fleet_items WHERE id = $1`, fleetID).Scan(&key); err != nil || key != lastOrder(t) {
		t.Fatalf("order key %q %v", key, err)
	}

	// The same batch again: stored outcomes, nothing written.
	again := postTxs(t, owner, p.ID, good1, bad, good2)
	for i := range out.Results {
		if a, b := string(mustJSON(out.Results[i])), string(mustJSON(again.Results[i])); a != b {
			t.Fatalf("replay %d: %s vs %s", i, a, b)
		}
	}
	if again.Seq != out.Seq || getFull(t, owner, p.ID).InputHash != after.InputHash {
		t.Fatal("replay changed the project")
	}
	var logged, rejected int
	if err := env.db.QueryRow(context.Background(),
		`SELECT count(*) FILTER (WHERE seq > $2), count(*) FILTER (WHERE seq IS NULL AND rejected_reason = 'invalid_value') FROM project_operations WHERE project_id = $1`,
		p.ID, base).Scan(&logged, &rejected); err != nil {
		t.Fatal(err)
	}
	if logged != 2 || rejected != 1 {
		t.Fatalf("log: %d applied, %d rejected", logged, rejected)
	}

	// A rejected-only batch writes nothing but the log line and keeps the seq.
	hash := getFull(t, owner, p.ID).InputHash
	out = postTxs(t, owner, p.ID, newTx(opSet(ops.CollFleetItems, fleetID, "price_override_reason", " ")))
	if out.Results[0].Status != ops.StatusRejected || out.Seq != base+2 || getFull(t, owner, p.ID).InputHash != hash {
		t.Fatalf("rejected batch: %+v", out)
	}
}

func TestTransactionsHooks(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "tx-hooks@example.com")
	p, _ := warehouseProject(t, owner, "Хуки")
	full := getFull(t, owner, p.ID)
	variant := full.Variants[0]
	unknownSolution := uuid.NewString()
	cases := []struct {
		name  string
		op    ops.Op
		field string
	}{
		{"solution outside the revision", opInsert(ops.CollFleetItems, uuid.NewString(), map[string]any{
			"order": lastOrder(t), "variant_id": variant.ID, "solution_id": unknownSolution, "quantity": 1,
		}), "solution_id"},
		{"params key", opSet(ops.CollProject, ops.CollProject, "params.shifts", "три"), "params.shifts"},
		{"unknown params key", opSet(ops.CollProject, ops.CollProject, "params.no_such_field", 1), "params.no_such_field"},
		{"object validator", opSet(ops.CollProcesses, full.Processes[0].ID, "sla.priority", 99), "sla.priority"},
		{"unknown object key", opSet(ops.CollProcesses, full.Processes[0].ID, "demand.pallets", 1), "demand.pallets"},
		{"type without params", opSet(ops.CollProject, ops.CollProject, "object_type", "airport"), ""},
	}
	for _, tc := range cases {
		out := postTxs(t, owner, p.ID, newTx(tc.op))
		r := out.Results[0]
		if r.Status != ops.StatusRejected {
			t.Errorf("%s: applied", tc.name)
			continue
		}
		if tc.field != "" && (r.Details == nil || r.Details.Field != tc.field) {
			t.Errorf("%s: %+v %+v", tc.name, r, r.Details)
		}
	}

	// A malformed operation rejects its transaction, not the batch.
	var out txBatchJSON
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/transactions", map[string]any{
		"client_id": "c", "schema_version": 1,
		"txs": []any{
			map[string]any{"tx_id": uuid.NewString(), "ops": []any{map[string]any{"op": "set", "coll": "project", "path": "name", "value": "x", "extra": 1}}},
			map[string]any{"tx_id": uuid.NewString(), "ops": "not a list"},
			map[string]any{"tx_id": uuid.NewString(), "ops": []any{map[string]any{"op": "reset"}}},
			newTx(opSet(ops.CollProject, ops.CollProject, "name", "Хуки 2")),
		},
	}, http.StatusOK, &out)
	for i, want := range []string{ops.StatusRejected, ops.StatusRejected, ops.StatusRejected, ops.StatusApplied} {
		if out.Results[i].Status != want {
			t.Fatalf("entry %d: %+v", i, out.Results[i])
		}
	}

	// Envelope errors reject the request.
	for name, tc := range map[string]struct {
		body map[string]any
		want int
	}{
		"schema version": {map[string]any{"client_id": "c", "schema_version": 2, "txs": []any{}}, http.StatusConflict},
		"no client":      {map[string]any{"schema_version": 1, "txs": []any{}}, http.StatusBadRequest},
		"upper tx_id":    {map[string]any{"client_id": "c", "schema_version": 1, "txs": []any{map[string]any{"tx_id": "A0000000-0000-4000-8000-000000000001", "ops": []any{}}}}, http.StatusBadRequest},
		"unknown field":  {map[string]any{"client_id": "c", "schema_version": 1, "txs": []any{}, "force": true}, http.StatusBadRequest},
		"too many": {map[string]any{"client_id": "c", "schema_version": 1, "txs": func() []testTx {
			list := make([]testTx, txBatchMax+1)
			for i := range list {
				list[i] = newTx()
			}
			return list
		}()}, http.StatusBadRequest},
	} {
		rec := owner.do(http.MethodPost, "/api/projects/"+p.ID+"/transactions", tc.body, false)
		if rec.Code != tc.want {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if rec := owner.do(http.MethodPost, "/api/projects/"+p.ID+"/transactions", map[string]any{"client_id": "c", "schema_version": 2, "txs": []any{}}, false); errorCode(t, rec) != ops.ReasonSchemaVersion {
		t.Fatalf("schema version code: %s", rec.Body.String())
	}

	// A viewer cannot send transactions.
	viewer := register(t, env.h, "tx-viewer@example.com")
	addMember(t, owner, p.ID, "tx-viewer@example.com", RoleViewer)
	rec := viewer.do(http.MethodPost, "/api/projects/"+p.ID+"/transactions", map[string]any{"client_id": "c", "schema_version": 1, "txs": []testTx{newTx(opSet(ops.CollProject, ops.CollProject, "name", "Взлом"))}}, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer: %d", rec.Code)
	}
}

// The input hash follows the content, not the way it was written: the same edits in one transaction or in
// three, in either order, leave twin projects identical, and one different value parts them.
func TestTransactionsHashFollowsContent(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "tx-hash@example.com")
	a, _ := warehouseProject(t, owner, "Одна транзакция")
	b, _ := warehouseProject(t, owner, "Три транзакции")
	fa, fb := getFull(t, owner, a.ID), getFull(t, owner, b.ID)
	if fa.InputHash != fb.InputHash {
		t.Fatal("twin projects differ before the edit")
	}

	va, vb := fa.Variants[1], fb.Variants[1]
	postTxs(t, owner, a.ID, newTx(
		opSet(ops.CollAssumptionSets, fa.AssumptionSets[0].ID, "vat_rate", 0.1),
		opSet(ops.CollProcesses, fa.Processes[0].ID, "demand.units_per_day", 777),
		opSet(ops.CollFleetItems, va.Fleet[0].ID, "quantity", 3),
		opSet(ops.CollVariants, va.ID, "name", "Смешанный флот"),
	))
	postTxs(t, owner, b.ID,
		newTx(opSet(ops.CollVariants, vb.ID, "name", "Смешанный флот"), opSet(ops.CollFleetItems, vb.Fleet[0].ID, "quantity", 3)),
		newTx(opSet(ops.CollProcesses, fb.Processes[0].ID, "demand.units_per_day", 777)),
		newTx(opSet(ops.CollAssumptionSets, fb.AssumptionSets[0].ID, "vat_rate", 0.1)),
	)
	ha, hb := getFull(t, owner, a.ID).InputHash, getFull(t, owner, b.ID).InputHash
	if ha != hb {
		t.Fatalf("one transaction %s, three transactions %s", ha, hb)
	}
	if ha == fa.InputHash {
		t.Fatal("the hash did not move although the content did")
	}

	postTxs(t, owner, b.ID, newTx(opSet(ops.CollFleetItems, vb.Fleet[0].ID, "quantity", 4)))
	if getFull(t, owner, b.ID).InputHash == ha {
		t.Fatal("a different fleet must give a different hash")
	}
}

func TestTransactionsSerialize(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "tx-race@example.com")
	editor := register(t, env.h, "tx-race-editor@example.com")
	p, _ := warehouseProject(t, owner, "Гонка")
	addMember(t, owner, p.ID, "tx-race-editor@example.com", RoleEditor)
	full := getFull(t, owner, p.ID)
	base := draftSeq(t, env, p.ID)

	const perClient = 15
	var (
		mu   sync.Mutex
		seqs []int64
		wg   sync.WaitGroup
	)
	for ci, c := range []*itClient{owner, editor} {
		wg.Add(1)
		go func(ci int, c *itClient, variant string) {
			defer wg.Done()
			for i := 0; i < perClient; i++ {
				out := postTxs(t, c, p.ID, newTx(opSet(ops.CollVariants, variant, "notes", fmt.Sprintf("%d-%d", ci, i))))
				mu.Lock()
				seqs = append(seqs, *out.Results[0].Seq)
				mu.Unlock()
			}
		}(ci, c, full.Variants[ci].ID)
	}
	wg.Wait()
	seen := map[int64]bool{}
	for _, s := range seqs {
		seen[s] = true
	}
	for s := base + 1; s <= base+2*perClient; s++ {
		if !seen[s] {
			t.Fatalf("seq %d missing; got %v", s, seqs)
		}
	}
	if draftSeq(t, env, p.ID) != base+2*perClient {
		t.Fatalf("draft_seq %d", draftSeq(t, env, p.ID))
	}
	after := getFull(t, owner, p.ID)
	if after.Variants[0].Notes != fmt.Sprintf("0-%d", perClient-1) || after.Variants[1].Notes != fmt.Sprintf("1-%d", perClient-1) {
		t.Fatalf("notes %q %q", after.Variants[0].Notes, after.Variants[1].Notes)
	}
}

func TestServerWritesAndCalculationsJournal(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	owner := register(t, env.h, "tx-legacy@example.com")
	p, _ := warehouseProject(t, owner, "Журнал")

	// Copying a project fills the new one outside operations, so its journal opens with a reset.
	var copied projectResp
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/copy", map[string]any{"name": "Журнал копия"}, http.StatusCreated, &copied)
	var opsRaw []byte
	var label string
	if err := env.db.QueryRow(ctx, `SELECT ops, label FROM project_operations WHERE project_id = $1 ORDER BY seq DESC LIMIT 1`, copied.ID).Scan(&opsRaw, &label); err != nil {
		t.Fatal(err)
	}
	var list []ops.Op
	if err := json.Unmarshal(opsRaw, &list); err != nil || len(list) != 1 || list[0].Op != ops.OpReset || label != "copy" {
		t.Fatalf("copy entry %s %q", opsRaw, label)
	}
	seq := draftSeq(t, env, p.ID)

	// Deleting every variant through operations is kept: the project is not reseeded.
	full := getFull(t, owner, p.ID)
	var dels []ops.Op
	for _, v := range full.Variants {
		dels = append(dels, ops.Op{Op: ops.OpDelete, Coll: ops.CollVariants, ID: v.ID})
	}
	postTxs(t, owner, p.ID, newTx(dels...))
	if n := len(getFull(t, owner, p.ID).Variants); n != 0 {
		t.Fatalf("variants reseeded: %d", n)
	}

	// A calculation records the seq it used and journals a changed selection.
	seq = draftSeq(t, env, p.ID)
	var calc calculateResponse
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/calculations", map[string]any{"include_ids": []string{h1500ID}}, http.StatusOK, &calc)
	var runSeq *int64
	if err := env.db.QueryRow(ctx, `SELECT draft_seq FROM calculation_runs WHERE id = $1`, calc.RunID).Scan(&runSeq); err != nil {
		t.Fatal(err)
	}
	if runSeq == nil || *runSeq != seq+1 || draftSeq(t, env, p.ID) != seq+1 {
		t.Fatalf("run seq %v, draft seq %d, before %d", runSeq, draftSeq(t, env, p.ID), seq)
	}
	if err := env.db.QueryRow(ctx, `SELECT label FROM project_operations WHERE project_id = $1 AND seq = $2`, p.ID, seq+1).Scan(&label); err != nil || label != "calculate" {
		t.Fatalf("calculate entry %q %v", label, err)
	}
	if got := getFull(t, owner, p.ID).MatchSelected; len(got) != 1 || got[0] != h1500ID {
		t.Fatalf("selection %v", got)
	}
	// The same calculation again changes nothing in the draft.
	owner.json(http.MethodPost, "/api/projects/"+p.ID+"/calculations", map[string]any{"include_ids": []string{h1500ID}}, http.StatusOK, &calc)
	if draftSeq(t, env, p.ID) != seq+1 {
		t.Fatal("unchanged selection must not be journaled")
	}
}

func TestOperationsNameTheirActor(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "tx-actor@example.com")
	p, _ := warehouseProject(t, owner, "Автор правки")
	before := draftSeq(t, env, p.ID)
	postTxs(t, owner, p.ID, newTx(opSet(ops.CollProject, ops.CollProject, "name", "Автор правки 2")))

	var page opsPageJSON
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/operations?after="+strconv.FormatInt(before, 10), nil, http.StatusOK, &page)
	if len(page.Items) != 1 || page.Items[0].ActorEmail == nil || *page.Items[0].ActorEmail != "tx-actor@example.com" {
		t.Fatalf("journal %+v", page.Items)
	}
}

type snapshotResp struct {
	SchemaVersion int                                   `json:"schema_version"`
	Seq           int64                                 `json:"seq"`
	Collections   map[string]map[string]json.RawMessage `json:"collections"`
}

func (s snapshotResp) state(t *testing.T) ops.State {
	t.Helper()
	st := ops.State{}
	for coll, recs := range s.Collections {
		st[coll] = map[string]ops.Record{}
		for id, raw := range recs {
			var r ops.Record
			if err := json.Unmarshal(raw, &r); err != nil {
				t.Fatal(err)
			}
			st[coll][id] = r
		}
	}
	return st
}

type allowHooks struct{}

func (allowHooks) Params(ops.Record, string, []string) (string, bool) { return "", true }
func (allowHooks) Object(string, any) bool                            { return true }
func (allowHooks) External(string, string) bool                       { return true }

// A client that applies the journal to its snapshot ends with the server's next snapshot.
func TestSnapshotAndOperationsFeed(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "feed-owner@example.com")
	viewer := register(t, env.h, "feed-viewer@example.com")
	p, _ := warehouseProject(t, owner, "Лента")
	addMember(t, owner, p.ID, "feed-viewer@example.com", RoleViewer)

	var snap snapshotResp
	viewer.json(http.MethodGet, "/api/projects/"+p.ID+"/snapshot", nil, http.StatusOK, &snap)
	if snap.SchemaVersion != 1 || snap.Seq != draftSeq(t, env, p.ID) || len(snap.Collections[ops.CollProcesses]) == 0 {
		t.Fatalf("snapshot seq %d, %d processes", snap.Seq, len(snap.Collections[ops.CollProcesses]))
	}
	st := snap.state(t)
	full := getFull(t, owner, p.ID)
	variant := full.Variants[1]
	newVariant, newFleet, newCost := uuid.NewString(), uuid.NewString(), uuid.NewString()
	postTxs(t, owner, p.ID,
		newTx(
			ops.Op{Op: ops.OpInsert, Coll: ops.CollVariants, ID: newVariant, Value: mustJSON(map[string]any{"order": lastOrder(t), "name": "Третий", "status": "draft"})},
			opInsert(ops.CollFleetItems, newFleet, map[string]any{"order": lastOrder(t), "variant_id": newVariant, "solution_id": stackerID, "quantity": 4, "price_override_rub": 1.005, "price_override_reason": "тест"}),
			opInsert(ops.CollFinancing, uuid.NewString(), map[string]any{"variant_id": newVariant, "kind": "buy", "assumptions": map[string]any{}}),
		),
		newTx(ops.Op{Op: ops.OpDelete, Coll: ops.CollFleetItems, ID: variant.Fleet[0].ID}),
		newTx(opInsert(ops.CollSharedCosts, newCost, map[string]any{"order": lastOrder(t), "code": "it", "label": "ИТ-интеграция", "bucket": "capex", "rub": 250000.5})),
		newTx(opSet(ops.CollProcesses, full.Processes[1].ID, "sla", map[string]any{"max_wait_min": 5, "priority": 2})),
		newTx(opSet(ops.CollProject, ops.CollProject, "active_assumption_set_id", nil)),
		newTx(ops.Op{Op: ops.OpMove, Coll: ops.CollVariants, ID: full.Variants[0].ID, Order: lastOrder(t) + "V"}),
	)

	var page opsPageJSON
	viewer.json(http.MethodGet, fmt.Sprintf("/api/projects/%s/operations?after=%d", p.ID, snap.Seq), nil, http.StatusOK, &page)
	if page.Reset || len(page.Items) != 6 || page.Seq != snap.Seq+6 {
		t.Fatalf("feed: reset %v, %d items, seq %d", page.Reset, len(page.Items), page.Seq)
	}
	schema := ops.ProjectSchema()
	for _, e := range page.Items {
		var list []ops.Op
		if err := json.Unmarshal(e.Ops, &list); err != nil {
			t.Fatal(err)
		}
		next, _, out := ops.Apply(schema, st, ops.Tx{TxID: e.TxID, Ops: list}, allowHooks{})
		if out.Status != ops.StatusApplied {
			t.Fatalf("seq %d does not apply to the snapshot: %+v", e.Seq, out)
		}
		st = next
		if e.Actor == nil || e.ClientID != "test-client" {
			t.Fatalf("entry %+v", e)
		}
	}
	var later snapshotResp
	viewer.json(http.MethodGet, "/api/projects/"+p.ID+"/snapshot", nil, http.StatusOK, &later)
	if a, b := string(mustJSON(st)), string(mustJSON(later.state(t))); a != b {
		t.Fatalf("feed on snapshot differs from the new snapshot:\n%s\n%s", a, b)
	}
	if later.Seq != page.Seq {
		t.Fatalf("snapshot seq %d, feed seq %d", later.Seq, page.Seq)
	}

	// Paging, an up-to-date client, a client from the future.
	viewer.json(http.MethodGet, fmt.Sprintf("/api/projects/%s/operations?after=%d&limit=2", p.ID, snap.Seq), nil, http.StatusOK, &page)
	if len(page.Items) != 2 || page.Items[1].Seq != snap.Seq+2 {
		t.Fatalf("page of 2: %+v", page.Items)
	}
	viewer.json(http.MethodGet, fmt.Sprintf("/api/projects/%s/operations?after=%d", p.ID, later.Seq), nil, http.StatusOK, &page)
	if page.Reset || len(page.Items) != 0 {
		t.Fatalf("up to date: %+v", page)
	}
	viewer.json(http.MethodGet, fmt.Sprintf("/api/projects/%s/operations?after=%d", p.ID, later.Seq+5), nil, http.StatusOK, &page)
	if !page.Reset {
		t.Fatal("a seq past the project must reset")
	}

	// A reset in the range and a purged range both reset. The server writes a reset when it fills a project
	// outside operations, such as a copy; here one is put in the journal directly.
	if _, err := env.db.Exec(context.Background(),
		`INSERT INTO project_operations (project_id, seq, tx_id, client_id, label, ops)
		 VALUES ($1, $2, gen_random_uuid(), 'server', 'copy', '[{"op":"reset"}]'::jsonb)`,
		p.ID, later.Seq+1); err != nil {
		t.Fatal(err)
	}
	viewer.json(http.MethodGet, fmt.Sprintf("/api/projects/%s/operations?after=%d", p.ID, later.Seq), nil, http.StatusOK, &page)
	if !page.Reset || len(page.Items) != 0 {
		t.Fatalf("reset in range: %+v", page)
	}
	if _, err := env.db.Exec(context.Background(), `DELETE FROM project_operations WHERE project_id = $1 AND seq = $2`, p.ID, snap.Seq+1); err != nil {
		t.Fatal(err)
	}
	viewer.json(http.MethodGet, fmt.Sprintf("/api/projects/%s/operations?after=%d", p.ID, snap.Seq), nil, http.StatusOK, &page)
	if !page.Reset {
		t.Fatal("a purged range must reset")
	}
	stranger := register(t, env.h, "feed-stranger@example.com")
	stranger.json(http.MethodGet, "/api/projects/"+p.ID+"/snapshot", nil, http.StatusNotFound, nil)
	viewer.json(http.MethodGet, "/api/projects/"+p.ID+"/operations?after=-1", nil, http.StatusBadRequest, nil)
}
