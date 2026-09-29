package ops

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const fixturesDir = "../../../contracts/ops/fixtures"

var update = flag.Bool("update", false, "rewrite contracts/ops/fixtures from the scenario table")

type fixture struct {
	Before   State            `json:"before"`
	Txs      []Tx             `json:"txs"`
	After    State            `json:"after"`
	Outcomes []fixtureOutcome `json:"outcomes"`
}

type fixtureOutcome struct {
	TxID    string   `json:"tx_id"`
	Status  string   `json:"status"`
	Reason  string   `json:"reason,omitempty"`
	Details *Details `json:"details,omitempty"`
	Changes []Change `json:"changes,omitempty"`
}

// id builds a readable UUID: id(0x701) is map point 1. Ranges: 1 processes, 2 variants, 3 fleet,
// 4 financing, 5 shared costs, 6 assumption sets, 7 points, 8 edges, 9 zones, a obstacles, b resources,
// c flows, e catalog, f new records.
func id(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", n)
}

func obj(kv ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

func list(items ...any) []any {
	if items == nil {
		return []any{}
	}
	return items
}

func pt(x, y float64) map[string]any {
	return obj("x", x, "y", y)
}

func raw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func ins(coll, rid string, value map[string]any) Op {
	return Op{Op: OpInsert, Coll: coll, ID: rid, Value: raw(value)}
}

func set(coll, rid, path string, value any) Op {
	return Op{Op: OpSet, Coll: coll, ID: rid, Path: path, Value: raw(value)}
}

func del(coll, rid string) Op {
	return Op{Op: OpDelete, Coll: coll, ID: rid}
}

func mv(coll, rid, order string) Op {
	return Op{Op: OpMove, Coll: coll, ID: rid, Order: order}
}

func tx(n int, ops ...Op) Tx {
	return Tx{TxID: fmt.Sprintf("tx-%d", n), Label: fmt.Sprintf("action %d", n), Ops: ops}
}

func put(st State, coll string, rec Record) {
	st[coll][rec.String(FieldID)] = rec
}

// base is a small project: two processes, two variants with fleet and financing, one shared cost,
// two assumption sets and a map with three points, two edges, zones, an obstacle, a resource and a flow.
func base() State {
	st := NewState(ProjectSchema())
	put(st, CollProject, Record{
		FieldID: CollProject, "name": "Склад", "object_type": "warehouse",
		"params":             obj("area_m2", 5000.0, "shifts", 2.0, "power_kw", nil),
		"match_selected_ids": list(), "econ_overrides": nil, "active_assumption_set_id": id(0x601),
		"reviewed_tabs": list(),
	})
	put(st, CollProcesses, Record{
		FieldID: id(0x101), FieldOrder: "a0", "code": "inbound", "name": "Приёмка", "task_type": "pallet_inbound",
		"is_baseline": true, "demand": obj("units_per_day", 1000.0, "unit", "поддон"), "sla": obj("max_wait_min", 30.0),
		"point_ids": list(id(0x701)), "durations": obj("load_s", 90.0), "baseline_staff": obj("headcount", 8.0),
	})
	put(st, CollProcesses, Record{
		FieldID: id(0x102), FieldOrder: "a1", "code": "pick", "name": "Отбор", "task_type": "piece_pick",
		"is_baseline": true, "demand": obj(), "sla": obj(), "point_ids": list(), "durations": obj(), "baseline_staff": obj(),
	})
	put(st, CollVariants, Record{FieldID: id(0x201), FieldOrder: "a0", "name": "AMR", "status": "draft", "notes": ""})
	put(st, CollVariants, Record{FieldID: id(0x202), FieldOrder: "a1", "name": "Смешанный", "status": "ready", "notes": "две модели"})
	put(st, CollFleetItems, Record{
		FieldID: id(0x301), FieldOrder: "a0", "variant_id": id(0x201), "solution_id": id(0xe001), "quantity": 2.0,
		"price_override_rub": nil, "price_override_reason": "", "task_codes": list("inbound"),
	})
	put(st, CollFleetItems, Record{
		FieldID: id(0x302), FieldOrder: "a1", "variant_id": id(0x201), "solution_id": id(0xe002), "quantity": 1.0,
		"price_override_rub": 100.0, "price_override_reason": "КП", "task_codes": list("inbound", "pick"),
	})
	put(st, CollFleetItems, Record{
		FieldID: id(0x303), FieldOrder: "a0", "variant_id": id(0x202), "solution_id": nil, "quantity": 3.0,
		"price_override_rub": nil, "price_override_reason": "", "task_codes": list(),
	})
	put(st, CollFinancing, Record{FieldID: id(0x401), "variant_id": id(0x201), "kind": "buy", "tariff": nil, "assumptions": obj()})
	put(st, CollFinancing, Record{FieldID: id(0x402), "variant_id": id(0x201), "kind": "raas", "tariff": "fixed", "assumptions": obj()})
	put(st, CollFinancing, Record{FieldID: id(0x403), "variant_id": id(0x202), "kind": "buy", "tariff": nil, "assumptions": obj()})
	put(st, CollSharedCosts, Record{FieldID: id(0x501), FieldOrder: "a0", "code": "infra", "label": "Инфраструктура", "bucket": "capex", "rub": 500000.0})
	put(st, CollAssumptionSets, Record{
		FieldID: id(0x601), FieldOrder: "a0", "name": "Базовый", "vat_rate": 0.22, "prices_include_vat": true,
		"vat_recoverable": false, "labor_cash_share": 1.0, "discount_rate": 0.15,
	})
	put(st, CollAssumptionSets, Record{
		FieldID: id(0x602), FieldOrder: "a1", "name": "Пессимистичный", "vat_rate": 0.22, "prices_include_vat": true,
		"vat_recoverable": false, "labor_cash_share": 0.5, "discount_rate": 0.2,
	})
	put(st, CollMap, Record{
		FieldID: CollMap, "page": obj("width_px", 1000.0, "height_px", 700.0, "source_kind", "none"),
		"profile": "indoor", "meters_per_px": 0.1, "segment": nil, "check": nil,
	})
	put(st, CollMapPoints, Record{FieldID: id(0x701), "kind": "dock", "name": "Док 1", "pos": pt(10, 10), "process_code": "inbound"})
	put(st, CollMapPoints, Record{FieldID: id(0x702), "kind": "task", "name": "", "pos": pt(100, 10), "process_code": nil})
	put(st, CollMapPoints, Record{FieldID: id(0x703), "kind": "charger", "name": "", "pos": pt(100, 80), "process_code": nil})
	put(st, CollMapEdges, Record{FieldID: id(0x801), "from": id(0x701), "to": id(0x702), "bidirectional": nil, "width_m": nil})
	put(st, CollMapEdges, Record{FieldID: id(0x802), "from": id(0x702), "to": id(0x703), "bidirectional": false, "width_m": 2.5})
	put(st, CollMapZones, Record{FieldID: id(0x901), FieldOrder: "a0", "kind": "workspace", "name": "", "ring": list(pt(0, 0), pt(200, 0), pt(200, 100))})
	put(st, CollMapZones, Record{FieldID: id(0x902), FieldOrder: "a1", "kind": "storage", "name": "Хранение", "ring": list(pt(10, 10), pt(50, 10), pt(50, 50))})
	put(st, CollMapObstacles, Record{FieldID: id(0xa01), FieldOrder: "a0", "kind": "rack", "name": "", "ring": list(pt(60, 60), pt(70, 60), pt(70, 70))})
	put(st, CollMapResources, Record{
		FieldID: id(0xb01), "kind": "dock", "name": "Доки", "capacity": 2.0, "point_id": id(0x701),
		"point_ids": list(id(0x701)), "edge_ids": list(id(0x801)),
	})
	put(st, CollMapFlows, Record{FieldID: id(0xc01), "process_code": "inbound", "pickup_point_ids": list(id(0x701)), "drop_point_ids": list(id(0x702))})
	return st
}

func newVariant(n int, order string) Op {
	return ins(CollVariants, id(0xf00+n), obj("order", order, "name", fmt.Sprintf("Вариант %d", n), "status", "draft"))
}

func newFleet(n int, variant string, extra map[string]any) Op {
	v := obj("order", "a5", "variant_id", variant, "quantity", 1.0)
	for k, x := range extra {
		v[k] = x
	}
	return ins(CollFleetItems, id(0xf00+n), v)
}

// sub keeps only the named collections of base, so each fixture carries what it needs.
func sub(colls ...string) func() State {
	return func() State {
		full := base()
		st := State{}
		for _, c := range colls {
			st[c] = full[c]
		}
		return st
	}
}

type scenario struct {
	name   string
	before func() State
	txs    []Tx
}

func scenarios() []scenario {
	vf := sub(CollProcesses, CollVariants, CollFleetItems, CollFinancing)
	pj := sub(CollProject, CollAssumptionSets)
	mp := sub(CollProcesses, CollMap, CollMapPoints, CollMapEdges, CollMapZones, CollMapObstacles, CollMapResources, CollMapFlows)
	sc := sub(CollSharedCosts)
	pr := sub(CollProcesses)
	return []scenario{
		{"insert_variant", vf, []Tx{tx(1, newVariant(1, "a2"))}},
		{"insert_zero_values", mp, []Tx{tx(1, ins(CollMapPoints, id(0xf01), obj("kind", "task", "pos", pt(5, 5))))}},
		{"insert_nullable_defaults_null", mp, []Tx{tx(1, ins(CollMapEdges, id(0xf01), obj("from", id(0x701), "to", id(0x703))))}},
		{"insert_existing_id", vf, []Tx{tx(1, ins(CollVariants, id(0x201), obj("order", "a5", "name", "Дубль", "status", "draft")))}},
		{"insert_bad_uuid", vf, []Tx{tx(1, ins(CollVariants, "v-1", obj("order", "a5", "name", "x", "status", "draft")))}},
		{"insert_legacy_map_id", mp, []Tx{tx(1, ins(CollMapPoints, "T7", obj("kind", "task", "pos", pt(5, 5))))}},
		{"insert_legacy_map_id_bad_chars", mp, []Tx{tx(1, ins(CollMapPoints, "T 7", obj("kind", "task", "pos", pt(5, 5))))}},
		{"insert_legacy_map_id_too_long", mp, []Tx{tx(1, ins(CollMapPoints, strings.Repeat("p", 65), obj("kind", "task", "pos", pt(5, 5))))}},
		{"insert_uppercase_uuid", vf, []Tx{tx(1, ins(CollVariants, strings.ToUpper("0000000a-0000-4000-8000-00000000f001"), obj("order", "a5", "name", "x", "status", "draft")))}},
		{"insert_unknown_field", vf, []Tx{tx(1, ins(CollVariants, id(0xf01), obj("order", "a5", "name", "x", "status", "draft", "color", "red")))}},
		{"insert_id_in_value", vf, []Tx{tx(1, ins(CollVariants, id(0xf01), obj("id", id(0xf01), "order", "a5", "name", "x", "status", "draft")))}},
		{"insert_missing_required", vf, []Tx{tx(1, ins(CollVariants, id(0xf01), obj("order", "a5", "status", "draft")))}},
		{"insert_not_object", vf, []Tx{tx(1, Op{Op: OpInsert, Coll: CollVariants, ID: id(0xf01), Value: raw(list(1.0))})}},
		{"insert_singleton", mp, []Tx{tx(1, ins(CollMap, CollMap, obj("profile", "indoor")))}},
		{"insert_ordered_without_order", vf, []Tx{tx(1, ins(CollVariants, id(0xf01), obj("name", "x", "status", "draft")))}},
		{"insert_bad_order_key", vf, []Tx{tx(1, ins(CollVariants, id(0xf01), obj("order", "a00", "name", "x", "status", "draft")))}},
		{"insert_unordered_with_order", mp, []Tx{tx(1, ins(CollMapPoints, id(0xf01), obj("order", "a0", "kind", "task", "pos", pt(1, 1))))}},
		{"insert_enum_bad", vf, []Tx{tx(1, ins(CollVariants, id(0xf01), obj("order", "a5", "name", "x", "status", "archived")))}},
		{"insert_ref_missing", vf, []Tx{tx(1, newFleet(1, id(0x2ff), nil))}},
		{"insert_ref_later_in_tx", vf, []Tx{tx(1, newFleet(2, id(0xf01), nil), newVariant(1, "a2"))}},
		{"insert_ref_list_by_key", vf, []Tx{tx(1, newFleet(1, id(0x202), obj("task_codes", list("pick"))))}},
		{"insert_ref_list_key_missing", vf, []Tx{tx(1, newFleet(1, id(0x202), obj("task_codes", list("sorting"))))}},
		{"insert_ref_list_duplicate", vf, []Tx{tx(1, newFleet(1, id(0x202), obj("task_codes", list("pick", "pick"))))}},
		{"insert_external_ref", vf, []Tx{tx(1, newFleet(1, id(0x202), obj("solution_id", id(0xe002))))}},
		{"insert_external_ref_missing", vf, []Tx{tx(1, newFleet(1, id(0x202), obj("solution_id", id(0xe0ff))))}},
		{"insert_max_items_reached", vf, []Tx{tx(1, newVariant(1, "a2"), newVariant(2, "a3"), newVariant(3, "a4"), newVariant(4, "a5"))}},
		{"insert_max_items_exceeded", vf, []Tx{tx(1, newVariant(1, "a2"), newVariant(2, "a3"), newVariant(3, "a4"), newVariant(4, "a5"), newVariant(5, "a6"))}},
		{"insert_unique_code", pr, []Tx{tx(1, ins(CollProcesses, id(0xf01), obj("order", "a5", "code", "pick", "name", "x", "task_type", "piece_pick",
			"demand", obj(), "sla", obj(), "durations", obj(), "baseline_staff", obj())))}},
		{"insert_unique_pair", mp, []Tx{tx(1, ins(CollMapEdges, id(0xf01), obj("from", id(0x701), "to", id(0x702))))}},
		{"insert_unique_pair_reversed", mp, []Tx{tx(1, ins(CollMapEdges, id(0xf01), obj("from", id(0x702), "to", id(0x701))))}},
		{"insert_unique_with_null", vf, []Tx{tx(1, ins(CollFinancing, id(0xf01), obj("variant_id", id(0x202), "kind", "buy", "assumptions", obj())))}},
		{"insert_requires_missing", vf, []Tx{tx(1, newFleet(1, id(0x202), obj("price_override_rub", 5.0)))}},
		{"insert_requires_blank", vf, []Tx{tx(1, newFleet(1, id(0x202), obj("price_override_rub", 5.0, "price_override_reason", "  ")))}},
		{"insert_requires_met", vf, []Tx{tx(1, newFleet(1, id(0x202), obj("price_override_rub", 5.0, "price_override_reason", "КП 09")))}},
		{"insert_points_too_short", mp, []Tx{tx(1, ins(CollMapZones, id(0xf01), obj("order", "a5", "kind", "pick", "ring", list(pt(0, 0), pt(1, 1)))))}},
		{"insert_object_validator", pr, []Tx{tx(1, ins(CollProcesses, id(0xf01), obj("order", "a5", "code", "move", "name", "Перемещение",
			"task_type", "pallet_move", "demand", obj("units_per_day", -1.0), "sla", obj(), "durations", obj(), "baseline_staff", obj())))}},
		{"insert_then_update", vf, []Tx{tx(1, newVariant(1, "a2"), set(CollVariants, id(0xf01), "name", "Новый"), mv(CollVariants, id(0xf01), "Zz"))}},
		{"insert_then_delete", vf, []Tx{tx(1, newVariant(1, "a2"), newFleet(2, id(0xf01), nil), del(CollVariants, id(0xf01)))}},

		{"set_string", vf, []Tx{tx(1, set(CollVariants, id(0x201), "name", "AMR ночь"))}},
		{"set_string_too_long", mp, []Tx{tx(1, set(CollMapPoints, id(0x702), "name", strings.Repeat("я", 121)))}},
		{"set_string_max_length_runes", mp, []Tx{tx(1, set(CollMapPoints, id(0x702), "name", strings.Repeat("я", 120)))}},
		{"set_string_empty", vf, []Tx{tx(1, set(CollVariants, id(0x201), "name", ""))}},
		{"set_int", vf, []Tx{tx(1, set(CollFleetItems, id(0x301), "quantity", 5.0))}},
		{"set_int_fraction", vf, []Tx{tx(1, set(CollFleetItems, id(0x301), "quantity", 1.5))}},
		{"set_int_range", vf, []Tx{tx(1, set(CollFleetItems, id(0x301), "quantity", 0.0))}},
		{"set_number_range", pj, []Tx{tx(1, set(CollAssumptionSets, id(0x601), "vat_rate", 1.5))}},
		{"set_number_type", pj, []Tx{tx(1, set(CollAssumptionSets, id(0x601), "vat_rate", "0.2"))}},
		{"set_discount_rate", pj, []Tx{tx(1, set(CollAssumptionSets, id(0x601), "discount_rate", 0.18))}},
		{"set_discount_rate_range", pj, []Tx{tx(1, set(CollAssumptionSets, id(0x601), "discount_rate", 0.001))}},
		{"set_assumption_norm", pj, []Tx{tx(1, set(CollAssumptionSets, id(0x601), "utilization", 0.7), set(CollAssumptionSets, id(0x601), "comm_rub_per_robot_year", 9000.0))}},
		{"set_assumption_norm_range", pj, []Tx{tx(1, set(CollAssumptionSets, id(0x601), "utilization", 1.5))}},
		{"set_assumption_norm_zero_range", pj, []Tx{tx(1, set(CollAssumptionSets, id(0x601), "availability", 0.0))}},
		{"set_assumption_norm_zero_allowed", pj, []Tx{tx(1, set(CollAssumptionSets, id(0x601), "reserve", 0.0))}},
		{"set_assumption_norm_clear", pj, []Tx{tx(1, set(CollAssumptionSets, id(0x601), "technician_wage_month_rub", 120000.0), set(CollAssumptionSets, id(0x601), "technician_wage_month_rub", nil))}},
		{"insert_assumption_set_norms", pj, []Tx{tx(1, ins(CollAssumptionSets, id(0x6f0), obj("order", "a2", "name", "Свой", "vat_rate", 0.22, "labor_cash_share", 1.0, "delivery_share", 0.05)))}},
		{"set_bool", pj, []Tx{tx(1, set(CollAssumptionSets, id(0x601), "vat_recoverable", true))}},
		{"set_bool_type", pj, []Tx{tx(1, set(CollAssumptionSets, id(0x601), "vat_recoverable", 1.0))}},
		{"set_enum", vf, []Tx{tx(1, set(CollVariants, id(0x201), "status", "ready"))}},
		{"set_enum_bad", vf, []Tx{tx(1, set(CollVariants, id(0x201), "status", "done"))}},
		{"set_point", mp, []Tx{tx(1, set(CollMapPoints, id(0x702), "pos", pt(120.5, 15)))}},
		{"set_point_extra_key", mp, []Tx{tx(1, set(CollMapPoints, id(0x702), "pos", obj("x", 1.0, "y", 2.0, "z", 3.0)))}},
		{"set_points", mp, []Tx{tx(1, set(CollMapObstacles, id(0xa01), "ring", list(pt(60, 60), pt(80, 60), pt(80, 80), pt(60, 80))))}},
		{"set_points_bad_item", mp, []Tx{tx(1, set(CollMapObstacles, id(0xa01), "ring", list(pt(60, 60), pt(80, 60), obj("x", 1.0))))}},
		{"set_object", pr, []Tx{tx(1, set(CollProcesses, id(0x102), "demand", obj("units_per_day", 20000.0, "unit", "строка")))}},
		{"set_object_invalid", pr, []Tx{tx(1, set(CollProcesses, id(0x102), "demand", obj("speed", 3.0)))}},
		{"set_object_key", pr, []Tx{tx(1, set(CollProcesses, id(0x101), "demand.units_per_day", 1200.0))}},
		{"set_object_key_invalid", pr, []Tx{tx(1, set(CollProcesses, id(0x101), "demand.units_per_day", -5.0))}},
		{"set_object_nullable", mp, []Tx{tx(1, set(CollMap, CollMap, "segment", obj("x1", 0.0, "y1", 0.0, "x2", 100.0, "y2", 0.0, "length_m", 10.0)), set(CollMap, CollMap, "check", nil))}},
		{"set_object_null_not_nullable", pr, []Tx{tx(1, set(CollProcesses, id(0x101), "sla", nil))}},
		{"set_reviewed_tabs", pj, []Tx{tx(1, set(CollProject, CollProject, "reviewed_tabs", list("site", "staff")))}},
		{"set_reviewed_tabs_invalid", pj, []Tx{tx(1, set(CollProject, CollProject, "reviewed_tabs", obj("site", true)))}},
		{"set_params_key", pj, []Tx{tx(1, set(CollProject, CollProject, "params.shifts", 3.0))}},
		{"set_params_key_null_allowed", pj, []Tx{tx(1, set(CollProject, CollProject, "params.power_kw", nil))}},
		{"set_params_key_out_of_range", pj, []Tx{tx(1, set(CollProject, CollProject, "params.shifts", 4.0))}},
		{"set_params_unknown_key", pj, []Tx{tx(1, set(CollProject, CollProject, "params.lanes", 4.0))}},
		{"set_params_whole", pj, []Tx{tx(1, set(CollProject, CollProject, "params", obj("area_m2", 100.0, "cold_store", true)))}},
		{"set_params_whole_bad", pj, []Tx{tx(1, set(CollProject, CollProject, "params", obj("area_m2", 100.0, "cold_store", "да")))}},
		{"set_params_after_type_change", pj, []Tx{tx(1, set(CollProject, CollProject, "object_type", "airport"), set(CollProject, CollProject, "params", obj("terminal_m2", 30000.0)))}},
		{"set_type_keeps_old_params", pj, []Tx{tx(1, set(CollProject, CollProject, "object_type", "airport"))}},
		{"set_type_with_params_key", pj, []Tx{tx(1, set(CollProject, CollProject, "object_type", "airport"), set(CollProject, CollProject, "params.terminal_m2", 30000.0))}},
		{"set_params_not_object", pj, []Tx{tx(1, set(CollProject, CollProject, "params", list()))}},
		{"set_nullable_null", vf, []Tx{tx(1, set(CollFleetItems, id(0x302), "price_override_rub", nil))}},
		{"set_not_nullable_null", vf, []Tx{tx(1, set(CollVariants, id(0x201), "name", nil))}},
		{"set_immutable", pr, []Tx{tx(1, set(CollProcesses, id(0x101), "code", "inbound2"))}},
		{"set_immutable_ref", vf, []Tx{tx(1, set(CollFleetItems, id(0x301), "variant_id", id(0x202)))}},
		{"set_same_value", vf, []Tx{tx(1, set(CollVariants, id(0x201), "name", "AMR"))}},
		{"set_missing_target", vf, []Tx{tx(1, set(CollVariants, id(0x2ff), "name", "x"))}},
		{"set_singleton_without_id", pj, []Tx{tx(1, Op{Op: OpSet, Coll: CollProject, Path: "name", Value: raw("Склад 2")})}},
		{"set_singleton_wrong_id", pj, []Tx{tx(1, set(CollProject, "main", "name", "Склад 2"))}},
		{"set_unknown_path", vf, []Tx{tx(1, set(CollVariants, id(0x201), "title", "x"))}},
		{"set_key_on_plain_field", vf, []Tx{tx(1, set(CollVariants, id(0x201), "name.first", "x"))}},
		{"set_deep_key", pr, []Tx{tx(1, set(CollProcesses, id(0x101), "demand.a.b", 1.0))}},
		{"set_order_by_set", vf, []Tx{tx(1, set(CollVariants, id(0x201), "order", "Zz"))}},
		{"set_id", vf, []Tx{tx(1, set(CollVariants, id(0x201), "id", id(0xf01)))}},
		{"set_without_value", vf, []Tx{tx(1, Op{Op: OpSet, Coll: CollVariants, ID: id(0x201), Path: "name"})}},
		{"set_ref", pj, []Tx{tx(1, set(CollProject, CollProject, "active_assumption_set_id", id(0x602)))}},
		{"set_ref_missing", pj, []Tx{tx(1, set(CollProject, CollProject, "active_assumption_set_id", id(0x6ff)))}},
		{"set_ref_empty_string", pj, []Tx{tx(1, set(CollProject, CollProject, "active_assumption_set_id", ""))}},
		{"set_ref_by_key", mp, []Tx{tx(1, set(CollMapPoints, id(0x702), "process_code", "pick"))}},
		{"set_ref_list_duplicate", mp, []Tx{tx(1, set(CollMapResources, id(0xb01), "point_ids", list(id(0x701), id(0x701))))}},
		{"set_ref_list_missing", mp, []Tx{tx(1, set(CollMapResources, id(0xb01), "point_ids", list(id(0x701), id(0x7ff))))}},
		{"set_requires_violated", vf, []Tx{tx(1, set(CollFleetItems, id(0x301), "price_override_rub", 10.0))}},
		{"set_requires_in_order", vf, []Tx{tx(1, set(CollFleetItems, id(0x301), "price_override_rub", 10.0), set(CollFleetItems, id(0x301), "price_override_reason", "Скидка"))}},
		{"set_requires_clear_reason", vf, []Tx{tx(1, set(CollFleetItems, id(0x302), "price_override_reason", ""))}},
		{"set_unique_conflict", sc, []Tx{
			tx(1, ins(CollSharedCosts, id(0xf01), obj("order", "a1", "code", "it", "label", "ИТ", "bucket", "opex", "rub", 1.0))),
			tx(2, set(CollSharedCosts, id(0xf01), "code", "infra")),
		}},
		{"set_unique_swap", sc, []Tx{
			tx(1, ins(CollSharedCosts, id(0xf01), obj("order", "a1", "code", "it", "label", "ИТ", "bucket", "opex", "rub", 1.0))),
			tx(2, set(CollSharedCosts, id(0xf01), "code", "tmp"), set(CollSharedCosts, id(0x501), "code", "it"), set(CollSharedCosts, id(0xf01), "code", "infra")),
		}},

		{"delete_cascade", vf, []Tx{tx(1, del(CollVariants, id(0x201)))}},
		{"delete_set_null", pj, []Tx{tx(1, del(CollAssumptionSets, id(0x601)))}},
		{"delete_point_everywhere", sub(CollProcesses, CollMapPoints, CollMapEdges, CollMapResources, CollMapFlows), []Tx{tx(1, del(CollMapPoints, id(0x701)))}},
		{"delete_edge_removes_from_lists", sub(CollMapPoints, CollMapEdges, CollMapResources), []Tx{tx(1, del(CollMapEdges, id(0x801)))}},
		{"delete_by_key", sub(CollProcesses, CollVariants, CollFleetItems, CollMapPoints, CollMapFlows), []Tx{tx(1, del(CollProcesses, id(0x101)))}},
		{"delete_missing", vf, []Tx{tx(1, del(CollVariants, id(0x2ff)))}},
		{"delete_singleton", pj, []Tx{tx(1, del(CollProject, CollProject))}},
		{"delete_twice", mp, []Tx{tx(1, del(CollMapPoints, id(0x703))), tx(2, del(CollMapPoints, id(0x703)))}},
		{"delete_then_reinsert", vf, []Tx{
			tx(1, del(CollVariants, id(0x202))),
			tx(2,
				ins(CollFleetItems, id(0x303), obj("order", "a0", "variant_id", id(0x202), "quantity", 3.0)),
				ins(CollFinancing, id(0x403), obj("variant_id", id(0x202), "kind", "buy", "assumptions", obj())),
				ins(CollVariants, id(0x202), obj("order", "a1", "name", "Смешанный", "status", "ready", "notes", "две модели")),
			),
		}},

		{"move", vf, []Tx{tx(1, mv(CollVariants, id(0x202), "Zz"))}},
		{"move_same_key", vf, []Tx{tx(1, mv(CollVariants, id(0x202), "a1"))}},
		{"move_unordered", mp, []Tx{tx(1, mv(CollMapPoints, id(0x701), "a0"))}},
		{"move_bad_key", vf, []Tx{tx(1, mv(CollVariants, id(0x202), "a"))}},
		{"move_missing", vf, []Tx{tx(1, mv(CollVariants, id(0x2ff), "a3"))}},

		{"atomic_second_op_fails", vf, []Tx{tx(1, set(CollVariants, id(0x201), "name", "Новое"), set(CollVariants, id(0x201), "status", "gone"))}},
		{"atomic_finish_check_fails", sub(CollProject, CollAssumptionSets, CollVariants), []Tx{tx(1, set(CollVariants, id(0x201), "name", "Новое"), set(CollProject, CollProject, "active_assumption_set_id", id(0x6ff)))}},
		{"sequence_mixed", vf, []Tx{
			tx(1, set(CollVariants, id(0x201), "name", "Первый")),
			tx(2, set(CollVariants, id(0x201), "status", "gone")),
			tx(3, set(CollVariants, id(0x202), "name", "Третий")),
		}},
		{"op_on_deleted_record", mp, []Tx{tx(1, del(CollMapPoints, id(0x703))), tx(2, set(CollMapPoints, id(0x703), "pos", pt(1, 1)))}},
		{"last_write_wins", mp, []Tx{tx(1, set(CollMapPoints, id(0x702), "pos", pt(1, 1))), tx(2, set(CollMapPoints, id(0x702), "pos", pt(2, 2)))}},
		{"unknown_op", vf, []Tx{tx(1, Op{Op: "upsert", Coll: CollVariants, ID: id(0x201)})}},
		{"reset_from_client", vf, []Tx{tx(1, Op{Op: OpReset})}},
		{"unknown_collection", vf, []Tx{tx(1, set("robots", id(0x201), "name", "x"))}},
		{"empty_tx", vf, []Tx{tx(1)}},
	}
}

func runFixture(s *Schema, h Hooks, f fixture) fixture {
	st := f.Before
	f.Outcomes = nil
	for _, t := range f.Txs {
		next, changes, out := Apply(s, st, t, h)
		f.Outcomes = append(f.Outcomes, fixtureOutcome{TxID: t.TxID, Status: out.Status, Reason: out.Reason, Details: out.Details, Changes: changes})
		st = next
	}
	f.After = st
	return f
}

// encodeFixture writes one record, operation or change per line and skips empty collections.
func encodeFixture(f fixture) []byte {
	var b bytes.Buffer
	line := func(v any) string {
		out, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		return string(out)
	}
	state := func(st State) {
		b.WriteString("{")
		first := true
		for _, coll := range sortedColls(st) {
			if len(st[coll]) == 0 {
				continue
			}
			if !first {
				b.WriteString(",")
			}
			first = false
			fmt.Fprintf(&b, "\n    %q: {", coll)
			for i, rid := range st.IDs(coll) {
				if i > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, "\n      %q: %s", rid, line(st[coll][rid]))
			}
			b.WriteString("\n    }")
		}
		if !first {
			b.WriteString("\n  ")
		}
		b.WriteString("}")
	}
	b.WriteString("{\n  \"before\": ")
	state(f.Before)
	b.WriteString(",\n  \"txs\": [")
	for i, t := range f.Txs {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "\n    {\"tx_id\": %s, \"label\": %s, \"ops\": [", line(t.TxID), line(t.Label))
		for j, op := range t.Ops {
			if j > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, "\n      %s", line(op))
		}
		if len(t.Ops) > 0 {
			b.WriteString("\n    ")
		}
		b.WriteString("]}")
	}
	b.WriteString("\n  ],\n  \"after\": ")
	state(f.After)
	b.WriteString(",\n  \"outcomes\": [")
	for i, o := range f.Outcomes {
		if i > 0 {
			b.WriteString(",")
		}
		head := fixtureOutcome{TxID: o.TxID, Status: o.Status, Reason: o.Reason, Details: o.Details}
		h := line(head)
		if len(o.Changes) == 0 {
			fmt.Fprintf(&b, "\n    %s", h)
			continue
		}
		fmt.Fprintf(&b, "\n    %s, \"changes\": [", h[:len(h)-1])
		for j, c := range o.Changes {
			if j > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, "\n      %s", line(c))
		}
		b.WriteString("\n    ]}")
	}
	b.WriteString("\n  ]\n}\n")
	var check fixture
	if err := json.Unmarshal(b.Bytes(), &check); err != nil {
		panic(fmt.Sprintf("encodeFixture wrote invalid JSON: %v", err))
	}
	return b.Bytes()
}

func sortedColls(st State) []string {
	out := make([]string, 0, len(st))
	for k := range st {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestFixtures(t *testing.T) {
	s := ProjectSchema()
	h := loadStub(t)
	want := map[string]bool{"hooks.json": true}
	for _, sc := range scenarios() {
		file := sc.name + ".json"
		if want[file] {
			t.Fatalf("duplicate scenario %s", sc.name)
		}
		want[file] = true
		got := encodeFixture(runFixture(s, h, fixture{Before: sc.before(), Txs: sc.txs}))
		path := filepath.Join(fixturesDir, file)
		if *update {
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		stored, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v (run go test ./internal/ops -run TestFixtures -update)", sc.name, err)
			continue
		}
		if !bytes.Equal(stored, got) {
			t.Errorf("%s: fixture is stale (run go test ./internal/ops -run TestFixtures -update)", sc.name)
		}
	}
	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		t.Fatal(err)
	}
	var extra []string
	for _, e := range entries {
		if !want[e.Name()] {
			extra = append(extra, e.Name())
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		if *update {
			for _, name := range extra {
				_ = os.Remove(filepath.Join(fixturesDir, name))
			}
			return
		}
		t.Errorf("fixtures without a scenario: %v", extra)
	}
}

// TestFixtureFilesReplay applies every stored file as the TS test does.
func TestFixtureFilesReplay(t *testing.T) {
	s := ProjectSchema()
	h := loadStub(t)
	files, err := filepath.Glob(filepath.Join(fixturesDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if filepath.Base(path) == "hooks.json" {
			continue
		}
		stored, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var f fixture
		if err := json.Unmarshal(stored, &f); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if got := encodeFixture(runFixture(s, h, f)); !bytes.Equal(got, stored) {
			t.Errorf("%s: replay differs", filepath.Base(path))
		}
	}
}
