package httpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/ops"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/simbuild"
)

// opsState is the project as operation records, loaded under the project lock.
type opsState struct {
	state ops.State
	draft projects.Draft
	seq   int64
}

// loadOpsState assembles the draft from the tables and turns it into records. Call inside inProjectTx.
func (s *Server) loadOpsState(ctx context.Context, row db.GetProjectRow) (opsState, error) {
	d, _, err := s.seedProjectGraph(ctx, row, false)
	if err != nil {
		return opsState{}, err
	}
	fresh, err := s.q.GetProject(ctx, row.ID)
	if err != nil {
		return opsState{}, fmt.Errorf("ops.load.project: %w", err)
	}
	st, err := ops.FromDraft(fresh.Name, d)
	if err != nil {
		return opsState{}, err
	}
	return opsState{state: st, draft: d, seq: s.lockedSeq}, nil
}

func touchedAny(touched map[string]bool, colls []string) bool {
	for _, c := range colls {
		if touched[c] {
			return true
		}
	}
	return false
}

// writeOpsState rewrites the tables of the touched collections with the records' own ids and order keys,
// then recomputes the draft document and input hash the same way the legacy endpoints do.
func (s *Server) writeOpsState(ctx context.Context, row db.GetProjectRow, loaded opsState, st ops.State, touched map[string]bool) error {
	d, name, err := ops.ToDraft(st)
	if err != nil {
		return err
	}
	pid := row.ID
	if touched[ops.CollProject] {
		// A run of the old object type answers a different question, so a type change detaches it: the run stays
		// in history, but the project stops presenting it as its current result.
		if d.ObjectType != row.ObjectType {
			if _, err := s.q.ChangeProjectObjectType(ctx, db.ChangeProjectObjectTypeParams{
				ID: pid, Name: name, ObjectType: d.ObjectType, Params: d.Params,
			}); err != nil {
				return fmt.Errorf("ops.write.type: %w", err)
			}
		} else if err := s.q.SetProjectContent(ctx, db.SetProjectContentParams{ID: pid, Name: name, ObjectType: d.ObjectType, Params: d.Params}); err != nil {
			return fmt.Errorf("ops.write.project: %w", err)
		}
	}
	if touched[ops.CollProcesses] {
		if err := s.writeProcesses(ctx, pid, d.Processes); err != nil {
			return err
		}
	}
	switch {
	case touched[ops.CollVariants]:
		if err := s.q.DeleteSolutionVariants(ctx, pid); err != nil {
			return fmt.Errorf("ops.write.variants: %w", err)
		}
		for _, v := range d.Variants {
			if err := s.insertVariantRow(ctx, pid, v); err != nil {
				return err
			}
		}
		if err := s.writeFleet(ctx, d.Variants); err != nil {
			return err
		}
		if err := s.writeFinancing(ctx, d.Variants); err != nil {
			return err
		}
	default:
		if touched[ops.CollFleetItems] {
			if err := s.q.DeleteProjectFleetItems(ctx, pid); err != nil {
				return fmt.Errorf("ops.write.fleet: %w", err)
			}
			if err := s.writeFleet(ctx, d.Variants); err != nil {
				return err
			}
		}
		if touched[ops.CollFinancing] {
			if err := s.q.DeleteProjectFinancing(ctx, pid); err != nil {
				return fmt.Errorf("ops.write.financing: %w", err)
			}
			if err := s.writeFinancing(ctx, d.Variants); err != nil {
				return err
			}
		}
	}
	if touched[ops.CollSharedCosts] {
		if err := s.writeSharedCosts(ctx, pid, d.SharedCosts); err != nil {
			return err
		}
	}
	// The active set lives on the project record, so a project change rewrites the sets' flags too.
	if touched[ops.CollAssumptionSets] || touched[ops.CollProject] {
		if err := s.writeAssumptionSets(ctx, pid, d.AssumptionSets); err != nil {
			return err
		}
	}

	fresh, err := s.q.GetProject(ctx, pid)
	if err != nil {
		return fmt.Errorf("ops.write.reload: %w", err)
	}
	next, _, err := s.assembleDraft(ctx, fresh, d.MatchSelectedIDs)
	if err != nil {
		return err
	}
	if touched[ops.CollProject] {
		next.EconOverrides = d.EconOverrides
		next.ReviewedTabs = d.ReviewedTabs
	}
	if touchedAny(touched, ops.MapCollections) {
		next.Map = nil
		if len(d.Map) > 0 {
			doc, err := maps.Decode(d.Map)
			if err != nil {
				return fmt.Errorf("ops.write.map: %w", err)
			}
			mc, err := s.mapContext(ctx, next)
			if err != nil {
				return fmt.Errorf("ops.write.map: %w", err)
			}
			check := simbuild.CheckMap(doc, mc)
			doc.Errors, doc.Warnings = maps.Messages(check.Issues)
			if next.Map, err = json.Marshal(doc); err != nil {
				return fmt.Errorf("ops.write.map: %w", err)
			}
		}
	}
	hash, err := projects.InputHash(next)
	if err != nil {
		return err
	}
	return s.persistDraft(ctx, pid, next, hash)
}

func parseRecordID(id, what string) (*uuid.UUID, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("ops.write.%s: id %q: %w", what, id, err)
	}
	return &u, nil
}

func (s *Server) writeProcesses(ctx context.Context, pid uuid.UUID, items []projects.Process) error {
	if err := s.q.DeleteProjectProcesses(ctx, pid); err != nil {
		return fmt.Errorf("ops.write.processes: %w", err)
	}
	for _, p := range items {
		id, err := parseRecordID(p.ID, "process")
		if err != nil {
			return err
		}
		points := p.PointIDs
		if points == nil {
			points = []string{}
		}
		if _, err := s.q.InsertProjectProcess(ctx, db.InsertProjectProcessParams{
			ID: id, ProjectID: pid, Code: p.Code, Name: p.Name, TaskType: p.TaskType, IsBaseline: p.IsBaseline,
			Demand: mustJSON(p.Demand), Sla: mustJSON(p.SLA), PointIds: mustJSON(points), Durations: mustJSON(p.Durations),
			BaselineStaff: mustJSON(p.BaselineStaff), SortOrder: int32(p.SortOrder), OrderKey: p.Order,
		}); err != nil {
			return fmt.Errorf("ops.write.process: %w", err)
		}
	}
	return nil
}

func (s *Server) insertVariantRow(ctx context.Context, pid uuid.UUID, v projects.Variant) error {
	id, err := parseRecordID(v.ID, "variant")
	if err != nil {
		return err
	}
	if _, err := s.q.InsertSolutionVariant(ctx, db.InsertSolutionVariantParams{
		ID: id, ProjectID: pid, Name: v.Name, Status: v.Status, Notes: v.Notes, SortOrder: int32(v.SortOrder), OrderKey: v.Order,
	}); err != nil {
		return fmt.Errorf("ops.write.variant: %w", err)
	}
	return nil
}

func (s *Server) writeFleet(ctx context.Context, variants []projects.Variant) error {
	for _, v := range variants {
		vid, err := uuid.Parse(v.ID)
		if err != nil {
			return fmt.Errorf("ops.write.fleet: variant %q: %w", v.ID, err)
		}
		for _, item := range v.Fleet {
			id, err := parseRecordID(item.ID, "fleet")
			if err != nil {
				return err
			}
			var sid *uuid.UUID
			if item.SolutionID != nil {
				if sid, err = parseRecordID(*item.SolutionID, "fleet.solution"); err != nil {
					return err
				}
			}
			codes := item.TaskCodes
			if codes == nil {
				codes = []string{}
			}
			if _, err := s.q.InsertVariantFleetItem(ctx, db.InsertVariantFleetItemParams{
				ID: id, VariantID: vid, SolutionID: sid, Quantity: int32(item.Quantity), TaskCodes: mustJSON(codes),
				PriceOverrideRub: numericFromFloatPtr(item.PriceOverrideRub), PriceOverrideReason: item.PriceOverrideReason,
				SortOrder: int32(item.SortOrder), OrderKey: item.Order,
			}); err != nil {
				return fmt.Errorf("ops.write.fleet: %w", err)
			}
		}
	}
	return nil
}

func (s *Server) writeFinancing(ctx context.Context, variants []projects.Variant) error {
	for _, v := range variants {
		vid, err := uuid.Parse(v.ID)
		if err != nil {
			return fmt.Errorf("ops.write.financing: variant %q: %w", v.ID, err)
		}
		for _, f := range v.Financing {
			id, err := parseRecordID(f.ID, "financing")
			if err != nil {
				return err
			}
			if _, err := s.q.InsertFinancingScenario(ctx, db.InsertFinancingScenarioParams{
				ID: id, VariantID: vid, Kind: f.Kind, Tariff: f.Tariff, Assumptions: f.Assumptions,
			}); err != nil {
				return fmt.Errorf("ops.write.financing: %w", err)
			}
		}
	}
	return nil
}

func (s *Server) writeSharedCosts(ctx context.Context, pid uuid.UUID, items []projects.SharedCost) error {
	if err := s.q.DeleteSharedCostItems(ctx, pid); err != nil {
		return fmt.Errorf("ops.write.shared_costs: %w", err)
	}
	for _, c := range items {
		id, err := parseRecordID(c.ID, "shared_cost")
		if err != nil {
			return err
		}
		if _, err := s.q.InsertSharedCostItem(ctx, db.InsertSharedCostItemParams{
			ID: id, ProjectID: pid, Code: c.Code, Label: c.Label, Bucket: c.Bucket, Rub: numericFromFloat(c.Rub),
			SortOrder: int32(c.SortOrder), OrderKey: c.Order,
		}); err != nil {
			return fmt.Errorf("ops.write.shared_cost: %w", err)
		}
	}
	return nil
}

func (s *Server) writeAssumptionSets(ctx context.Context, pid uuid.UUID, items []projects.AssumptionSet) error {
	if err := s.q.DeleteAssumptionSets(ctx, pid); err != nil {
		return fmt.Errorf("ops.write.assumption_sets: %w", err)
	}
	for _, a := range items {
		id, err := parseRecordID(a.ID, "assumption_set")
		if err != nil {
			return err
		}
		if _, err := s.q.InsertAssumptionSet(ctx, db.InsertAssumptionSetParams{
			ID: id, ProjectID: pid, Name: a.Name, IsActive: a.IsActive, VatRate: numericFromFloat(a.VATRate),
			PricesIncludeVat: a.PricesIncludeVAT, VatRecoverable: a.VATRecoverable, LaborCashShare: numericFromFloat(a.LaborCashShare),
			DiscountRate: numericFromFloat(projects.DiscountOrDefault(a.DiscountRate)), SortOrder: int32(a.SortOrder), OrderKey: a.Order,
			Utilization: numericFromFloatPtr(a.Utilization), Availability: numericFromFloatPtr(a.Availability),
			Reserve: numericFromFloatPtr(a.Reserve), ServiceShare: numericFromFloatPtr(a.ServiceShare),
			DeliveryShare: numericFromFloatPtr(a.DeliveryShare), CommRubPerRobotYear: numericFromFloatPtr(a.CommRubPerRobotYear),
			TechnicianWageMonthRub: numericFromFloatPtr(a.TechnicianWageMonthRub),
		}); err != nil {
			return fmt.Errorf("ops.write.assumption_set: %w", err)
		}
	}
	return nil
}
