// A demo project as operation records. A guest has no account and no project on the server, but the pages
// are the same pages, so each demo is the same store as a project: one per object type, confirmed here.

import type { EconOverrides, ObjectType, ParamsMap, Process, SchemaField, SolutionVariant } from '../api/client'
import { objectSchema } from '../offline/reference'
import { defaultProcesses } from '../projects/defaults'
import type { Rec, State } from '../store/apply'
import { uuid } from '../store/clientId'
import { keyBetween } from '../store/order'
import { defaultsFromSchema, fieldList } from './store'

const demoNames: Record<ObjectType, string> = {
  warehouse: 'Демо Склад',
  airport: 'Демо Аэропорт',
  hospital: 'Демо Больница',
}

export function demoName(type: ObjectType): string {
  return demoNames[type]
}

// The single assumption set a new project comes with, mirroring projects.DefaultAssumptionSets.
function assumptionSet(): Rec {
  return {
    id: uuid(),
    order: keyBetween(null, null),
    name: 'Базовый',
    vat_rate: 0.22,
    prices_include_vat: true,
    vat_recoverable: false,
    labor_cash_share: 1,
    discount_rate: 0.15,
  }
}

function processRecords(items: Process[]): Record<string, Rec> {
  const out: Record<string, Rec> = {}
  let order: string | null = null
  for (const p of items) {
    order = keyBetween(order, null)
    const id = p.id ?? uuid()
    out[id] = {
      id,
      order,
      code: p.code,
      name: p.name,
      task_type: p.task_type,
      is_baseline: p.is_baseline,
      demand: p.demand as unknown as Rec,
      sla: p.sla as unknown as Rec,
      point_ids: p.point_ids ?? [],
      durations: p.durations as unknown as Rec,
      baseline_staff: p.baseline_staff as unknown as Rec,
    }
  }
  return out
}

type VariantRecords = { variants: Record<string, Rec>; fleet: Record<string, Rec>; financing: Record<string, Rec> }

function variantRecords(items: SolutionVariant[]): VariantRecords {
  const out: VariantRecords = { variants: {}, fleet: {}, financing: {} }
  let order: string | null = null
  for (const v of items) {
    order = keyBetween(order, null)
    const id = v.id ?? uuid()
    out.variants[id] = { id, order, name: v.name, status: v.status, notes: v.notes ?? '' }
    let fleetOrder: string | null = null
    for (const item of v.fleet ?? []) {
      fleetOrder = keyBetween(fleetOrder, null)
      const fid = item.id ?? uuid()
      out.fleet[fid] = {
        id: fid,
        order: fleetOrder,
        variant_id: id,
        solution_id: item.solution_id ?? null,
        quantity: item.quantity,
        price_override_rub: item.price_override_rub ?? null,
        price_override_reason: item.price_override_reason ?? '',
        task_codes: item.task_codes ?? [],
      }
    }
    for (const f of v.financing ?? []) {
      const fid = f.id ?? uuid()
      out.financing[fid] = {
        id: fid,
        variant_id: id,
        kind: f.kind,
        tariff: f.tariff ?? null,
        assumptions: (f.assumptions ?? {}) as unknown as Rec,
      }
    }
  }
  return out
}

export type GuestContent = {
  objectType: ObjectType
  params: ParamsMap
  processes: Process[]
  variants: SolutionVariant[]
  selected: string[]
  overrides: EconOverrides
}

// guestState lays the content out as the collections of a project, exactly as a snapshot of one would arrive.
export function guestState(content: GuestContent): State {
  const set = assumptionSet()
  const { variants, fleet, financing } = variantRecords(content.variants)
  return {
    project: {
      project: {
        id: 'project',
        name: demoName(content.objectType),
        object_type: content.objectType,
        params: content.params as unknown as Rec,
        match_selected_ids: content.selected,
        econ_overrides: content.overrides as unknown as Rec,
        active_assumption_set_id: set.id,
        reviewed_tabs: [],
      },
    },
    processes: processRecords(content.processes),
    variants,
    fleet_items: fleet,
    financing,
    shared_costs: {},
    assumption_sets: { [set.id as string]: set },
    map: { map: { id: 'map', page: null, profile: 'indoor', meters_per_px: 0.1, segment: null, check: null } },
    map_points: {},
    map_edges: {},
    map_zones: {},
    map_obstacles: {},
    map_resources: {},
    map_flows: {},
  }
}

// demoContent is a demo at its starting point: the schema defaults and the default processes of its type.
export function demoContent(type: ObjectType, fields: SchemaField[]): GuestContent {
  return {
    objectType: type,
    params: defaultsFromSchema(fields),
    processes: defaultProcesses(type),
    variants: [],
    selected: [],
    overrides: {},
  }
}

// DemoSeed is a demo at its starting point and the version of it this build carries.
export type DemoSeed = { version: string; state: () => Promise<State> }

// demoSeed describes the first state of a demo. Демо Склад is a prepared project (web/src/guest/demo-warehouse.json,
// written by the Go tests); the others start at the schema defaults, and a schema this browser cannot reach leaves
// their parameters empty, which the object form fills as they are entered.
export async function demoSeed(type: ObjectType): Promise<DemoSeed> {
  if (type === 'warehouse') {
    const { default: file } = (await import('./demo-warehouse.json')) as { default: { version: string; collections: unknown } }
    return { version: file.version, state: async () => structuredClone(file.collections) as State }
  }
  return {
    version: 'defaults-1',
    state: async () => {
      let fields: SchemaField[] = []
      try {
        fields = fieldList(await objectSchema(type))
      } catch {
        fields = []
      }
      return guestState(demoContent(type, fields))
    },
  }
}
