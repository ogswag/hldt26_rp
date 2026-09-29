package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/exporters"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
)

// serverClientID is the client_id of journal rows written by the server.
const serverClientID = "server"

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

func emptyJSONArray() json.RawMessage {
	return json.RawMessage(`[]`)
}

func emptyJSONObject() json.RawMessage {
	return json.RawMessage(`{}`)
}

func (s *Server) loadProcesses(ctx context.Context, projectID uuid.UUID) ([]projects.Process, error) {
	rows, err := s.q.ListProjectProcesses(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("projects.processes: %w", err)
	}
	out := make([]projects.Process, 0, len(rows))
	for _, row := range rows {
		out = append(out, processFromRow(row))
	}
	return out, nil
}

func processFromRow(row db.ProjectProcess) projects.Process {
	p := projects.Process{
		ID:         row.ID.String(),
		Code:       row.Code,
		Name:       row.Name,
		TaskType:   row.TaskType,
		IsBaseline: row.IsBaseline,
		SortOrder:  int(row.SortOrder),
		Order:      row.OrderKey,
	}
	_ = json.Unmarshal(row.Demand, &p.Demand)
	_ = json.Unmarshal(row.Sla, &p.SLA)
	_ = json.Unmarshal(row.PointIds, &p.PointIDs)
	_ = json.Unmarshal(row.Durations, &p.Durations)
	_ = json.Unmarshal(row.BaselineStaff, &p.BaselineStaff)
	if p.PointIDs == nil {
		p.PointIDs = []string{}
	}
	return p
}

func (s *Server) insertProcess(ctx context.Context, projectID uuid.UUID, p projects.Process) (projects.Process, error) {
	points := p.PointIDs
	if points == nil {
		points = []string{}
	}
	row, err := s.q.InsertProjectProcess(ctx, db.InsertProjectProcessParams{
		ProjectID:     projectID,
		Code:          p.Code,
		Name:          p.Name,
		TaskType:      p.TaskType,
		IsBaseline:    p.IsBaseline,
		Demand:        mustJSON(p.Demand),
		Sla:           mustJSON(p.SLA),
		PointIds:      mustJSON(points),
		Durations:     mustJSON(p.Durations),
		BaselineStaff: mustJSON(p.BaselineStaff),
		SortOrder:     int32(p.SortOrder),
	})
	if err != nil {
		return projects.Process{}, err
	}
	return processFromRow(row), nil
}

func (s *Server) loadVariants(ctx context.Context, projectID uuid.UUID) ([]projects.Variant, error) {
	rows, err := s.q.ListSolutionVariants(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("projects.variants: %w", err)
	}
	out := make([]projects.Variant, 0, len(rows))
	for _, row := range rows {
		v, err := s.loadVariant(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Server) loadVariant(ctx context.Context, row db.SolutionVariant) (projects.Variant, error) {
	v := projects.Variant{
		ID:        row.ID.String(),
		Name:      row.Name,
		Status:    row.Status,
		Notes:     row.Notes,
		SortOrder: int(row.SortOrder),
		Order:     row.OrderKey,
	}
	fleet, err := s.q.ListVariantFleetItems(ctx, row.ID)
	if err != nil {
		return projects.Variant{}, fmt.Errorf("projects.fleet: %w", err)
	}
	v.Fleet = make([]projects.FleetItem, 0, len(fleet))
	for _, item := range fleet {
		fi := projects.FleetItem{
			ID:                  item.ID.String(),
			Quantity:            int(item.Quantity),
			PriceOverrideReason: item.PriceOverrideReason,
			SortOrder:           int(item.SortOrder),
			Order:               item.OrderKey,
			PriceOverrideRub:    numericPtr(item.PriceOverrideRub),
		}
		if item.SolutionID != nil {
			s := item.SolutionID.String()
			fi.SolutionID = &s
		}
		_ = json.Unmarshal(item.TaskCodes, &fi.TaskCodes)
		if fi.TaskCodes == nil {
			fi.TaskCodes = []string{}
		}
		v.Fleet = append(v.Fleet, fi)
	}
	fins, err := s.q.ListFinancingScenarios(ctx, row.ID)
	if err != nil {
		return projects.Variant{}, fmt.Errorf("projects.financing: %w", err)
	}
	v.Financing = make([]projects.Financing, 0, len(fins))
	for _, f := range fins {
		v.Financing = append(v.Financing, projects.Financing{
			ID:          f.ID.String(),
			Kind:        f.Kind,
			Tariff:      f.Tariff,
			Assumptions: f.Assumptions,
		})
	}
	if v.Fleet == nil {
		v.Fleet = []projects.FleetItem{}
	}
	if v.Financing == nil {
		v.Financing = []projects.Financing{}
	}
	return v, nil
}

func (s *Server) insertVariant(ctx context.Context, projectID uuid.UUID, v projects.Variant) (projects.Variant, error) {
	status := v.Status
	if status == "" {
		status = "draft"
	}
	row, err := s.q.InsertSolutionVariant(ctx, db.InsertSolutionVariantParams{
		ProjectID: projectID,
		Name:      v.Name,
		Status:    status,
		Notes:     v.Notes,
		SortOrder: int32(v.SortOrder),
	})
	if err != nil {
		return projects.Variant{}, err
	}
	if err := s.replaceVariantChildren(ctx, row.ID, v); err != nil {
		return projects.Variant{}, err
	}
	return s.loadVariant(ctx, row)
}

func (s *Server) replaceVariantChildren(ctx context.Context, variantID uuid.UUID, v projects.Variant) error {
	if err := s.q.DeleteVariantFleetItems(ctx, variantID); err != nil {
		return err
	}
	if err := s.q.DeleteFinancingScenarios(ctx, variantID); err != nil {
		return err
	}
	for i, item := range v.Fleet {
		var sid *uuid.UUID
		if item.SolutionID != nil && *item.SolutionID != "" {
			id, err := uuid.Parse(*item.SolutionID)
			if err != nil {
				return fmt.Errorf("projects.fleet.solution_id: %w", err)
			}
			sid = &id
		}
		codes := item.TaskCodes
		if codes == nil {
			codes = []string{}
		}
		order := item.SortOrder
		if order == 0 {
			order = i
		}
		if _, err := s.q.InsertVariantFleetItem(ctx, db.InsertVariantFleetItemParams{
			VariantID:           variantID,
			SolutionID:          sid,
			Quantity:            int32(item.Quantity),
			TaskCodes:           mustJSON(codes),
			PriceOverrideRub:    numericFromFloatPtr(item.PriceOverrideRub),
			PriceOverrideReason: item.PriceOverrideReason,
			SortOrder:           int32(order),
		}); err != nil {
			return err
		}
	}
	fins := v.Financing
	if len(fins) == 0 {
		t := "fixed"
		fins = []projects.Financing{
			{Kind: "buy", Assumptions: emptyJSONObject()},
			{Kind: "raas", Tariff: &t, Assumptions: emptyJSONObject()},
		}
	}
	for _, f := range fins {
		ass := f.Assumptions
		if len(ass) == 0 {
			ass = emptyJSONObject()
		}
		if _, err := s.q.InsertFinancingScenario(ctx, db.InsertFinancingScenarioParams{
			VariantID:   variantID,
			Kind:        f.Kind,
			Tariff:      f.Tariff,
			Assumptions: ass,
		}); err != nil {
			return err
		}
	}
	return nil
}

func numericFromFloatPtr(p *float64) pgtype.Numeric {
	var n pgtype.Numeric
	if p == nil {
		return n
	}
	_ = n.Scan(strconv.FormatFloat(*p, 'f', -1, 64))
	return n
}

func numericFromFloat(v float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(v, 'f', -1, 64))
	return n
}

func (s *Server) loadSharedCosts(ctx context.Context, projectID uuid.UUID) ([]projects.SharedCost, error) {
	rows, err := s.q.ListSharedCostItems(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("projects.shared_costs: %w", err)
	}
	out := make([]projects.SharedCost, 0, len(rows))
	for _, row := range rows {
		out = append(out, projects.SharedCost{
			ID:        row.ID.String(),
			Code:      row.Code,
			Label:     row.Label,
			Bucket:    row.Bucket,
			Rub:       derefNumeric(row.Rub),
			SortOrder: int(row.SortOrder),
			Order:     row.OrderKey,
		})
	}
	return out, nil
}

func (s *Server) replaceSharedCosts(ctx context.Context, projectID uuid.UUID, items []projects.SharedCost) ([]projects.SharedCost, error) {
	if err := s.q.DeleteSharedCostItems(ctx, projectID); err != nil {
		return nil, err
	}
	out := make([]projects.SharedCost, 0, len(items))
	for i, c := range items {
		order := c.SortOrder
		if order == 0 {
			order = i
		}
		row, err := s.q.InsertSharedCostItem(ctx, db.InsertSharedCostItemParams{
			ProjectID: projectID,
			Code:      c.Code,
			Label:     c.Label,
			Bucket:    c.Bucket,
			Rub:       numericFromFloat(c.Rub),
			SortOrder: int32(order),
		})
		if err != nil {
			return nil, err
		}
		out = append(out, projects.SharedCost{
			ID:        row.ID.String(),
			Code:      row.Code,
			Label:     row.Label,
			Bucket:    row.Bucket,
			Rub:       derefNumeric(row.Rub),
			SortOrder: int(row.SortOrder),
		})
	}
	return out, nil
}

func (s *Server) loadAssumptionSets(ctx context.Context, projectID uuid.UUID) ([]projects.AssumptionSet, error) {
	rows, err := s.q.ListAssumptionSets(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("projects.assumption_sets: %w", err)
	}
	out := make([]projects.AssumptionSet, 0, len(rows))
	for _, row := range rows {
		out = append(out, assumptionFromRow(row))
	}
	return out, nil
}

func assumptionFromRow(row db.AssumptionSet) projects.AssumptionSet {
	return projects.AssumptionSet{
		ID:               row.ID.String(),
		Name:             row.Name,
		IsActive:         row.IsActive,
		VATRate:          derefNumeric(row.VatRate),
		PricesIncludeVAT: row.PricesIncludeVat,
		VATRecoverable:   row.VatRecoverable,
		LaborCashShare:   derefNumeric(row.LaborCashShare),
		DiscountRate:     projects.DiscountOrDefault(derefNumeric(row.DiscountRate)),

		Utilization:            numericPtr(row.Utilization),
		Availability:           numericPtr(row.Availability),
		Reserve:                numericPtr(row.Reserve),
		ServiceShare:           numericPtr(row.ServiceShare),
		DeliveryShare:          numericPtr(row.DeliveryShare),
		CommRubPerRobotYear:    numericPtr(row.CommRubPerRobotYear),
		TechnicianWageMonthRub: numericPtr(row.TechnicianWageMonthRub),
		SortOrder:              int(row.SortOrder),
		Order:                  row.OrderKey,
	}
}

func (s *Server) replaceAssumptionSets(ctx context.Context, projectID uuid.UUID, items []projects.AssumptionSet) ([]projects.AssumptionSet, error) {
	if err := s.q.DeleteAssumptionSets(ctx, projectID); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		items = projects.DefaultAssumptionSets()
	}
	active := 0
	for i, a := range items {
		if a.IsActive {
			active = i
		}
	}
	out := make([]projects.AssumptionSet, 0, len(items))
	for i, a := range items {
		a.IsActive = i == active
		order := a.SortOrder
		if order == 0 {
			order = i
		}
		row, err := s.q.InsertAssumptionSet(ctx, db.InsertAssumptionSetParams{
			ProjectID:        projectID,
			Name:             a.Name,
			IsActive:         a.IsActive,
			VatRate:          numericFromFloat(a.VATRate),
			PricesIncludeVat: a.PricesIncludeVAT,
			VatRecoverable:   a.VATRecoverable,
			LaborCashShare:   numericFromFloat(a.LaborCashShare),
			DiscountRate:     numericFromFloat(projects.DiscountOrDefault(a.DiscountRate)),
			SortOrder:        int32(order),

			Utilization:            numericFromFloatPtr(a.Utilization),
			Availability:           numericFromFloatPtr(a.Availability),
			Reserve:                numericFromFloatPtr(a.Reserve),
			ServiceShare:           numericFromFloatPtr(a.ServiceShare),
			DeliveryShare:          numericFromFloatPtr(a.DeliveryShare),
			CommRubPerRobotYear:    numericFromFloatPtr(a.CommRubPerRobotYear),
			TechnicianWageMonthRub: numericFromFloatPtr(a.TechnicianWageMonthRub),
		})
		if err != nil {
			return nil, err
		}
		out = append(out, assumptionFromRow(row))
	}
	return out, nil
}

func derefNumeric(n pgtype.Numeric) float64 {
	f := numericPtr(n)
	if f == nil {
		return 0
	}
	return *f
}

func (s *Server) selectedIDs(ctx context.Context, row db.GetProjectRow) []string {
	draft, err := s.q.GetProjectDraft(ctx, row.ID)
	if err == nil {
		var d projects.Draft
		if json.Unmarshal(draft.Document, &d) == nil {
			return d.MatchSelectedIDs
		}
	}
	return selectedFromResults(row.Results)
}

func (s *Server) storedDraft(ctx context.Context, projectID uuid.UUID) projects.Draft {
	row, err := s.q.GetProjectDraft(ctx, projectID)
	if err != nil {
		return projects.Draft{}
	}
	var d projects.Draft
	if json.Unmarshal(row.Document, &d) != nil {
		return projects.Draft{}
	}
	return d
}

func (s *Server) assembleDraft(ctx context.Context, row db.GetProjectRow, selected []string) (projects.Draft, string, error) {
	stored := s.storedDraft(ctx, row.ID)
	procs, err := s.loadProcesses(ctx, row.ID)
	if err != nil {
		return projects.Draft{}, "", err
	}
	vars, err := s.loadVariants(ctx, row.ID)
	if err != nil {
		return projects.Draft{}, "", err
	}
	costs, err := s.loadSharedCosts(ctx, row.ID)
	if err != nil {
		return projects.Draft{}, "", err
	}
	sets, err := s.loadAssumptionSets(ctx, row.ID)
	if err != nil {
		return projects.Draft{}, "", err
	}
	d := projects.NewDraft(row.ObjectType, row.Params, procs, vars, selected)
	d.SharedCosts = costs
	d.AssumptionSets = sets
	d.Map = stored.Map
	d.EconOverrides = stored.EconOverrides
	d.ReviewedTabs = stored.ReviewedTabs
	for _, a := range sets {
		if a.IsActive {
			d.ActiveAssumptionSetID = a.ID
			break
		}
	}
	hash, err := projects.InputHash(d)
	if err != nil {
		return projects.Draft{}, "", err
	}
	return d, hash, nil
}

func (s *Server) persistDraft(ctx context.Context, projectID uuid.UUID, d projects.Draft, hash string) error {
	doc, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("projects.draft.marshal: %w", err)
	}
	if _, err := s.q.UpsertProjectDraft(ctx, db.UpsertProjectDraftParams{
		ProjectID: projectID,
		Document:  doc,
		InputHash: hash,
	}); err != nil {
		return fmt.Errorf("projects.draft: %w", err)
	}
	if err := s.q.SetProjectInputHash(ctx, db.SetProjectInputHashParams{ID: projectID, InputHash: hash}); err != nil {
		return fmt.Errorf("projects.hash: %w", err)
	}
	return s.rehashRuns(ctx, projectID)
}

// rehashRuns fills draft_hash for the project's runs that have none: the run's snapshot hashed the way drafts are
// hashed now, so a change in the hash alone never makes a run stale. A run of another model keeps its own hash.
func (s *Server) rehashRuns(ctx context.Context, projectID uuid.UUID) error {
	runs, err := s.q.ListRunsWithoutDraftHash(ctx, projectID)
	if err != nil {
		return fmt.Errorf("projects.rehash.list: %w", err)
	}
	for _, run := range runs {
		hash := run.InputHash
		if sameModel(run) {
			var snap projects.Snapshot
			err := json.Unmarshal(run.Snapshot, &snap)
			if err == nil {
				hash, err = projects.InputHash(snap.Draft)
			}
			if err != nil {
				s.log.Warn("projects.rehash", "op", "projects.rehash", "request_id", middleware.GetReqID(ctx),
					"project_id", projectID.String(), "run_id", run.ID.String(), "err", err)
				hash = run.InputHash
			}
		}
		if err := s.q.SetRunDraftHash(ctx, db.SetRunDraftHashParams{ID: run.ID, DraftHash: hash}); err != nil {
			return fmt.Errorf("projects.rehash.set: %w", err)
		}
	}
	return nil
}

// sameModel reports whether the run was computed by the econ, matching and verification versions of this build.
func sameModel(run db.ListRunsWithoutDraftHashRow) bool {
	if run.EconVersion != econ.ModelVersion || run.MatchVersion != projects.MatchVersion {
		return false
	}
	// NOTE: a simulation run records the simulator version, which simStale checks on its own.
	return run.Kind != exporters.KindCalculation || run.SimVersion == projects.SimVersion
}

// httpError is a user-facing failure raised inside a project transaction.
type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

// inProjectTx runs fn with queries bound to one transaction that holds the project row lock.
// The copy passed to fn knows the project's draft_seq as of the lock.
func (s *Server) inProjectTx(ctx context.Context, projectID uuid.UUID, fn func(tx *Server) error) error {
	tx, err := s.q.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("projects.tx.begin: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	cp := *s
	cp.q = s.q.WithTx(tx)
	seq, err := cp.q.LockProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("projects.tx.lock: %w", err)
	}
	cp.lockedSeq = seq
	if err := fn(&cp); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("projects.tx.commit: %w", err)
	}
	return nil
}

// logServerOps journals ops the server made itself as the next seq and wakes the project's streams on commit.
// Call inside inProjectTx.
func (s *Server) logServerOps(ctx context.Context, projectID uuid.UUID, label string, list []ops.Op) error {
	raw, err := json.Marshal(list)
	if err != nil {
		return fmt.Errorf("projects.ops.marshal: %w", err)
	}
	s.lockedSeq++
	seq := s.lockedSeq
	var actor *uuid.UUID
	if id := auth.FromContext(ctx); !id.IsGuest() {
		actor = &id.UserID
	}
	if err := s.q.InsertProjectOperation(ctx, db.InsertProjectOperationParams{
		ProjectID: projectID, Seq: &seq, TxID: uuid.New(), ClientID: serverClientID, ActorUserID: actor, Label: label, Ops: raw,
	}); err != nil {
		return fmt.Errorf("projects.ops.log: %w", err)
	}
	return s.finishSeq(ctx, projectID, seq)
}

// logReset marks a write the server made outside operations, such as filling a copied project; clients reload
// the snapshot when they reach it. label says which write it was.
func (s *Server) logReset(ctx context.Context, projectID uuid.UUID, label string) error {
	return s.logServerOps(ctx, projectID, label, []ops.Op{{Op: ops.OpReset}})
}

// finishSeq stores the project's new seq and notifies listeners once the transaction commits.
func (s *Server) finishSeq(ctx context.Context, projectID uuid.UUID, seq int64) error {
	if err := s.q.SetProjectDraftSeq(ctx, db.SetProjectDraftSeqParams{ID: projectID, DraftSeq: seq}); err != nil {
		return fmt.Errorf("projects.ops.seq: %w", err)
	}
	if err := s.q.NotifyProjectOps(ctx, fmt.Sprintf("%s:%d", projectID, seq)); err != nil {
		return fmt.Errorf("projects.ops.notify: %w", err)
	}
	return nil
}

// ensureProjectGraph seeds default processes, variants and assumption sets once and refreshes the draft.
func (s *Server) ensureProjectGraph(ctx context.Context, row db.GetProjectRow) (projects.Draft, string, error) {
	d, hash, _, err := s.currentDraft(ctx, row)
	return d, hash, err
}

// currentDraft is ensureProjectGraph that also returns the draft_seq the draft belongs to.
func (s *Server) currentDraft(ctx context.Context, row db.GetProjectRow) (projects.Draft, string, int64, error) {
	var (
		d    projects.Draft
		hash string
		seq  int64
	)
	err := s.inProjectTx(ctx, row.ID, func(tx *Server) error {
		var err error
		d, hash, err = tx.seedProjectGraph(ctx, row, false)
		seq = tx.lockedSeq
		return err
	})
	return d, hash, seq, err
}

// seedProjectGraph fills empty processes, variants and assumption sets with defaults, then refreshes the draft.
// Without force it seeds only a project nobody has edited yet: after the first operation an empty list is the
// users' choice. Call inside inProjectTx.
func (s *Server) seedProjectGraph(ctx context.Context, row db.GetProjectRow, force bool) (projects.Draft, string, error) {
	seed := force || s.lockedSeq == 0
	row, err := s.q.GetProject(ctx, row.ID)
	if err != nil {
		return projects.Draft{}, "", fmt.Errorf("projects.seed.reload: %w", err)
	}
	procs, err := s.loadProcesses(ctx, row.ID)
	if err != nil {
		return projects.Draft{}, "", err
	}
	if seed && len(procs) == 0 {
		for _, p := range projects.DefaultProcesses(row.ObjectType) {
			if _, err := s.insertProcess(ctx, row.ID, p); err != nil {
				return projects.Draft{}, "", fmt.Errorf("projects.seed.process: %w", err)
			}
		}
	}
	vars, err := s.loadVariants(ctx, row.ID)
	if err != nil {
		return projects.Draft{}, "", err
	}
	if seed && len(vars) == 0 {
		for _, v := range projects.DefaultVariants() {
			if _, err := s.insertVariant(ctx, row.ID, v); err != nil {
				return projects.Draft{}, "", fmt.Errorf("projects.seed.variant: %w", err)
			}
		}
	}
	sets, err := s.loadAssumptionSets(ctx, row.ID)
	if err != nil {
		return projects.Draft{}, "", err
	}
	if seed && len(sets) == 0 {
		if _, err := s.replaceAssumptionSets(ctx, row.ID, projects.DefaultAssumptionSets()); err != nil {
			return projects.Draft{}, "", fmt.Errorf("projects.seed.assumptions: %w", err)
		}
	}
	d, hash, err := s.assembleDraft(ctx, row, s.selectedIDs(ctx, row))
	if err != nil {
		return projects.Draft{}, "", err
	}
	if err := s.persistDraft(ctx, row.ID, d, hash); err != nil {
		return projects.Draft{}, "", err
	}
	return d, hash, nil
}

// insertVersion locks the project and stores an immutable snapshot of d. Call inside a transaction.
func insertVersion(ctx context.Context, q *db.Queries, row db.GetProjectRow, d projects.Draft, hash string, model projects.SnapshotModel) (db.ProjectVersion, projects.Snapshot, error) {
	if _, err := q.LockProjectForVersion(ctx, row.ID); err != nil {
		return db.ProjectVersion{}, projects.Snapshot{}, fmt.Errorf("projects.version.lock: %w", err)
	}
	verNo, err := q.NextProjectVersionNo(ctx, row.ID)
	if err != nil {
		return db.ProjectVersion{}, projects.Snapshot{}, fmt.Errorf("projects.version.next: %w", err)
	}
	snap := projects.NewSnapshot(row.ID.String(), row.Name, row.ObjectType, int(verNo), d, hash, model)
	rawSnap, err := json.Marshal(snap)
	if err != nil {
		return db.ProjectVersion{}, projects.Snapshot{}, fmt.Errorf("projects.snapshot: %w", err)
	}
	ver, err := q.InsertProjectVersion(ctx, db.InsertProjectVersionParams{
		ProjectID: row.ID,
		VersionNo: verNo,
		Snapshot:  rawSnap,
		InputHash: hash,
	})
	if err != nil {
		return db.ProjectVersion{}, projects.Snapshot{}, fmt.Errorf("projects.version: %w", err)
	}
	return ver, snap, nil
}

// persistRun stores a calculation of d, computed from the draft at seq. If the draft is still at seq, the run's
// selection and overrides become the draft's through a journaled set; otherwise the draft is left alone and the
// new run is stale at once.
func (s *Server) persistRun(ctx context.Context, row db.GetProjectRow, d projects.Draft, hash string, seq int64, seed int, result econ.Result) (*uuid.UUID, error) {
	var runID uuid.UUID
	err := s.inProjectTx(ctx, row.ID, func(tx *Server) error {
		cur, draftHash, err := tx.seedProjectGraph(ctx, row, false)
		if err != nil {
			return err
		}
		runSeq := seq
		if tx.lockedSeq == seq {
			if list := draftSets(cur, d); len(list) > 0 {
				if err := tx.persistDraft(ctx, row.ID, d, hash); err != nil {
					return err
				}
				if err := tx.logServerOps(ctx, row.ID, "calculate", list); err != nil {
					return err
				}
				draftHash = hash
			}
			runSeq = tx.lockedSeq
		}
		model := projects.CalculationModel()
		ver, _, err := insertVersion(ctx, tx.q, row, d, hash, model)
		if err != nil {
			return err
		}
		run, err := tx.q.InsertCalculationRun(ctx, db.InsertCalculationRunParams{
			ProjectID:        row.ID,
			ProjectVersionID: ver.ID,
			InputHash:        hash,
			MatchVersion:     projects.MatchVersion,
			EconVersion:      econ.ModelVersion,
			SimVersion:       model.SimVersion,
			Seed:             int32(seed),
			Status:           "succeeded",
			ConfidenceLevel:  model.ConfidenceLevel,
			DraftSeq:         &runSeq,
		})
		if err != nil {
			return fmt.Errorf("projects.run: %w", err)
		}
		summary, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("projects.result.marshal: %w", err)
		}
		if _, err := tx.q.InsertCalculationResult(ctx, db.InsertCalculationResultParams{RunID: run.ID, Summary: summary}); err != nil {
			return fmt.Errorf("projects.result: %w", err)
		}
		seed32 := int32(seed)
		if _, err := tx.q.SetProjectCalculation(ctx, db.SetProjectCalculationParams{
			ID:           row.ID,
			Results:      summary,
			ModelVersion: econ.ModelVersion,
			CalcSeed:     &seed32,
			CurrentRunID: &run.ID,
			InputHash:    draftHash,
		}); err != nil {
			return fmt.Errorf("projects.run.link: %w", err)
		}
		runID = run.ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &runID, nil
}

// draftSets lists the project sets that turn from's match selection and overrides into to's.
func draftSets(from, to projects.Draft) []ops.Op {
	var list []ops.Op
	if !slices.Equal(from.MatchSelectedIDs, to.MatchSelectedIDs) {
		ids := to.MatchSelectedIDs
		if ids == nil {
			ids = []string{}
		}
		list = append(list, ops.Op{Op: ops.OpSet, Coll: ops.CollProject, ID: ops.CollProject, Path: "match_selected_ids", Value: mustJSON(ids)})
	}
	if !bytes.Equal(canonical(from.EconOverrides), canonical(to.EconOverrides)) {
		v := json.RawMessage("null")
		if len(bytes.TrimSpace(to.EconOverrides)) > 0 {
			v = to.EconOverrides
		}
		list = append(list, ops.Op{Op: ops.OpSet, Coll: ops.CollProject, ID: ops.CollProject, Path: "econ_overrides", Value: v})
	}
	return list
}

func canonical(raw json.RawMessage) []byte {
	if len(bytes.TrimSpace(raw)) == 0 {
		return []byte("null")
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return raw
	}
	out, err := projects.CanonicalJSON(v)
	if err != nil {
		return raw
	}
	return out
}

func (s *Server) copyGraph(ctx context.Context, from, to uuid.UUID) error {
	stored := s.storedDraft(ctx, from)
	procs, err := s.loadProcesses(ctx, from)
	if err != nil {
		return err
	}
	for _, p := range procs {
		p.ID = ""
		if _, err := s.insertProcess(ctx, to, p); err != nil {
			return err
		}
	}
	vars, err := s.loadVariants(ctx, from)
	if err != nil {
		return err
	}
	for _, v := range vars {
		v.ID = ""
		if _, err := s.insertVariant(ctx, to, v); err != nil {
			return err
		}
	}
	costs, err := s.loadSharedCosts(ctx, from)
	if err != nil {
		return err
	}
	if _, err := s.replaceSharedCosts(ctx, to, costs); err != nil {
		return err
	}
	sets, err := s.loadAssumptionSets(ctx, from)
	if err != nil {
		return err
	}
	if _, err := s.replaceAssumptionSets(ctx, to, sets); err != nil {
		return err
	}
	target, err := s.q.GetProject(ctx, to)
	if err != nil {
		return err
	}
	d, _, err := s.assembleDraft(ctx, target, stored.MatchSelectedIDs)
	if err != nil {
		return err
	}
	d.Map = stored.Map
	d.EconOverrides = stored.EconOverrides
	hash, err := projects.InputHash(d)
	if err != nil {
		return err
	}
	if err := s.persistDraft(ctx, to, d, hash); err != nil {
		return err
	}
	return nil
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
