// The project record itself, its shared cost lines and its assumption sets, in the store. Each edit builder
// turns one change into one transaction, so two people working on different lines never overwrite each other.

import type { AssumptionSet, EconOverrides, ObjectType, ParamsMap, ParamValue, SharedCost } from '../api/client'
import { equal, type RawOp, type Rec, type State } from '../store/apply'
import { uuid } from '../store/clientId'
import { keyBetween } from '../store/order'
import type { AssumptionSetsRecord, ProjectRecord, SharedCostsRecord } from '../store/schema.gen'

export type Edit = { ops: RawOp[]; label: string }

// The discount rate a set gets when nothing is stored, as in projects.DefaultDiscountRate.
export const defaultDiscountRate = 0.15

const none: Edit = { ops: [], label: '' }

function recs<T>(st: State, coll: string): Record<string, T> {
  return (st[coll] ?? {}) as Record<string, T>
}

function byOrder<T extends { id: string; order: string }>(list: T[]): T[] {
  return [...list].sort((a, b) => (a.order === b.order ? (a.id < b.id ? -1 : 1) : a.order < b.order ? -1 : 1))
}

function lastOrder(st: State, coll: string): string | null {
  let last: string | null = null
  for (const r of Object.values(recs<{ order: string }>(st, coll))) {
    if (last === null || r.order > last) {
      last = r.order
    }
  }
  return last
}

// projectRecord is the project's singleton. It is absent until the first snapshot arrives.
export function projectRecord(st: State): ProjectRecord | undefined {
  return recs<ProjectRecord>(st, 'project').project
}

// paramsOf returns the object parameters as the form reads them.
export function paramsOf(st: State): ParamsMap {
  return (projectRecord(st)?.params ?? {}) as ParamsMap
}

// objectTypeOf returns the project's object type, or null before the first snapshot arrives.
export function objectTypeOf(st: State): ObjectType | null {
  const type = projectRecord(st)?.object_type
  return type === 'warehouse' || type === 'airport' || type === 'hospital' ? type : null
}

export function selectedSolutions(st: State): string[] {
  const ids = projectRecord(st)?.match_selected_ids
  return Array.isArray(ids) ? (ids as string[]) : []
}

// reviewedTabsOf lists the object tabs someone has looked at. The calculation opens once the key ones are here.
export function reviewedTabsOf(st: State): string[] {
  const ids = projectRecord(st)?.reviewed_tabs
  return Array.isArray(ids) ? ids.filter((x): x is string => typeof x === 'string') : []
}

// reviewTabEdit adds a tab to the reviewed list. It steers the UI only, so it stays out of undo.
export function reviewTabEdit(st: State, tab: string): Edit {
  const before = reviewedTabsOf(st)
  if (before.includes(tab)) {
    return { ...none, label: 'Просмотреть параметры' }
  }
  return {
    ops: [{ op: 'set', coll: 'project', id: 'project', path: 'reviewed_tabs', value: [...before, tab] }],
    label: 'Просмотреть параметры',
  }
}

// hasFleet says whether any variant of the project has a robot in it.
export function hasFleet(st: State): boolean {
  return Object.values((st.fleet_items ?? {}) as Record<string, { solution_id?: unknown; quantity?: unknown }>).some(
    (f) => typeof f.solution_id === 'string' && f.solution_id !== '' && typeof f.quantity === 'number' && f.quantity > 0,
  )
}

export function overridesOf(st: State): EconOverrides {
  return (projectRecord(st)?.econ_overrides ?? {}) as EconOverrides
}

// paramsEdit writes the changed parameters, one operation per field, so a parameter someone else is editing
// keeps their value.
export function paramsEdit(st: State, next: ParamsMap): Edit {
  const before = paramsOf(st)
  const ops: RawOp[] = []
  for (const [key, value] of Object.entries(next)) {
    if (!equal(before[key], value)) {
      ops.push({ op: 'set', coll: 'project', id: 'project', path: `params.${key}`, value: value as ParamValue })
    }
  }
  for (const key of Object.keys(before)) {
    if (!(key in next)) {
      ops.push({ op: 'set', coll: 'project', id: 'project', path: `params.${key}`, value: null })
    }
  }
  return { ops, label: 'Изменить параметры объекта' }
}

// paramFieldEdit writes one parameter of the object. Forms save field by field, so a parameter someone else is
// editing keeps their value, and a field still being typed in is not sent at all.
export function paramFieldEdit(st: State, field: string, value: ParamValue | undefined): Edit {
  const label = 'Изменить параметр объекта'
  const before = paramsOf(st)
  // A field cleared to nothing and a field marked unknown are both "no value" in the stored parameters.
  const next = value === undefined ? null : value
  if (equal(before[field] ?? null, next)) {
    return { ...none, label }
  }
  return { ops: [{ op: 'set', coll: 'project', id: 'project', path: `params.${field}`, value: next }], label }
}

export function overridesEdit(st: State, ov: EconOverrides): Edit {
  const value = Object.keys(ov).length === 0 ? null : (ov as Rec)
  if (equal(projectRecord(st)?.econ_overrides ?? null, value)) {
    return { ...none, label: 'Изменить what-if' }
  }
  return { ops: [{ op: 'set', coll: 'project', id: 'project', path: 'econ_overrides', value }], label: 'Изменить what-if' }
}

export function projectNameEdit(st: State, name: string): Edit {
  if (projectRecord(st)?.name === name) {
    return { ...none, label: 'Переименовать проект' }
  }
  return { ops: [{ op: 'set', coll: 'project', id: 'project', path: 'name', value: name }], label: 'Переименовать проект' }
}

// sharedCostsFromState returns the shared cost lines in their order.
export function sharedCostsFromState(st: State): SharedCost[] {
  return byOrder(Object.values(recs<SharedCostsRecord>(st, 'shared_costs'))).map((c, i) => ({
    id: c.id,
    code: c.code,
    label: c.label,
    bucket: c.bucket,
    rub: c.rub,
    sort_order: i,
  }))
}

export function sharedCostsSelector(): (st: State) => SharedCost[] {
  let ref: unknown = null
  let out: SharedCost[] = []
  return (st) => {
    if (st.shared_costs !== ref) {
      ref = st.shared_costs
      out = sharedCostsFromState(st)
    }
    return out
  }
}

export type SharedCostPatch = Partial<Pick<SharedCost, 'code' | 'label' | 'bucket' | 'rub'>>

export function sharedCostEdit(st: State, id: string, patch: SharedCostPatch): Edit {
  const rec = recs<SharedCostsRecord>(st, 'shared_costs')[id]
  if (!rec) {
    return { ...none, label: 'Изменить статью затрат' }
  }
  const ops: RawOp[] = []
  for (const [path, value] of Object.entries(patch)) {
    if (!equal((rec as unknown as Rec)[path], value)) {
      ops.push({ op: 'set', coll: 'shared_costs', id, path, value: value as Rec[string] })
    }
  }
  return { ops, label: 'Изменить статью затрат' }
}

export function addSharedCostEdit(st: State, cost: SharedCost): Edit {
  const value: Rec = {
    order: keyBetween(lastOrder(st, 'shared_costs'), null),
    code: cost.code,
    label: cost.label,
    bucket: cost.bucket,
    rub: cost.rub,
  }
  return { ops: [{ op: 'insert', coll: 'shared_costs', id: cost.id ?? uuid(), value }], label: 'Добавить статью затрат' }
}

export function deleteSharedCostEdit(id: string): Edit {
  return { ops: [{ op: 'delete', coll: 'shared_costs', id }], label: 'Удалить статью затрат' }
}

// assumptionSetsFromState returns the sets with the active one marked from the project record.
export function assumptionSetsFromState(st: State): AssumptionSet[] {
  const active = projectRecord(st)?.active_assumption_set_id ?? null
  return byOrder(Object.values(recs<AssumptionSetsRecord>(st, 'assumption_sets'))).map((a, i) => ({
    id: a.id,
    name: a.name,
    is_active: a.id === active,
    vat_rate: a.vat_rate,
    prices_include_vat: a.prices_include_vat,
    vat_recoverable: a.vat_recoverable,
    labor_cash_share: a.labor_cash_share,
    discount_rate: a.discount_rate ?? defaultDiscountRate,
    utilization: a.utilization,
    availability: a.availability,
    reserve: a.reserve,
    service_share: a.service_share,
    delivery_share: a.delivery_share,
    comm_rub_per_robot_year: a.comm_rub_per_robot_year,
    technician_wage_month_rub: a.technician_wage_month_rub,
    sort_order: i,
  }))
}

export function assumptionSetsSelector(): (st: State) => AssumptionSet[] {
  let refs: unknown[] = []
  let out: AssumptionSet[] = []
  return (st) => {
    const next = [st.assumption_sets, st.project]
    if (next.some((r, i) => r !== refs[i])) {
      refs = next
      out = assumptionSetsFromState(st)
    }
    return out
  }
}

export type AssumptionPatch = Partial<{
  name: string
  vat_rate: number
  prices_include_vat: boolean
  vat_recoverable: boolean
  labor_cash_share: number
  discount_rate: number
  utilization: number | null
  availability: number | null
  reserve: number | null
  service_share: number | null
  delivery_share: number | null
  comm_rub_per_robot_year: number | null
  technician_wage_month_rub: number | null
}>

export function assumptionSetEdit(st: State, id: string, patch: AssumptionPatch): Edit {
  const rec = recs<AssumptionSetsRecord>(st, 'assumption_sets')[id]
  if (!rec) {
    return { ...none, label: 'Изменить набор допущений' }
  }
  const ops: RawOp[] = []
  for (const [path, value] of Object.entries(patch)) {
    if (!equal((rec as unknown as Rec)[path], value)) {
      ops.push({ op: 'set', coll: 'assumption_sets', id, path, value: value as Rec[string] })
    }
  }
  return { ops, label: 'Изменить набор допущений' }
}

// activeAssumptionEdit points the project at one set. Which set is active belongs to the project, not to the
// sets, so choosing one is a single write nobody can half-apply.
export function activeAssumptionEdit(st: State, id: string): Edit {
  if ((projectRecord(st)?.active_assumption_set_id ?? null) === id) {
    return { ...none, label: 'Выбрать набор допущений' }
  }
  return {
    ops: [{ op: 'set', coll: 'project', id: 'project', path: 'active_assumption_set_id', value: id }],
    label: 'Выбрать набор допущений',
  }
}

export function addAssumptionSetEdit(st: State, set: AssumptionSet): Edit {
  const id = set.id ?? uuid()
  const value: Rec = {
    order: keyBetween(lastOrder(st, 'assumption_sets'), null),
    name: set.name,
    vat_rate: set.vat_rate,
    prices_include_vat: set.prices_include_vat,
    vat_recoverable: set.vat_recoverable,
    labor_cash_share: set.labor_cash_share,
    discount_rate: set.discount_rate,
  }
  const ops: RawOp[] = [{ op: 'insert', coll: 'assumption_sets', id, value }]
  if (set.is_active) {
    ops.push({ op: 'set', coll: 'project', id: 'project', path: 'active_assumption_set_id', value: id })
  }
  return { ops, label: 'Добавить набор допущений' }
}

// deleteAssumptionSetEdit removes a set. The project's pointer is nulled by the schema (on_delete: set_null),
// so the next active set is chosen here to leave the project with one.
export function deleteAssumptionSetEdit(st: State, id: string): Edit {
  const ops: RawOp[] = [{ op: 'delete', coll: 'assumption_sets', id }]
  if ((projectRecord(st)?.active_assumption_set_id ?? null) === id) {
    const next = byOrder(Object.values(recs<AssumptionSetsRecord>(st, 'assumption_sets'))).find((a) => a.id !== id)
    if (next) {
      ops.push({ op: 'set', coll: 'project', id: 'project', path: 'active_assumption_set_id', value: next.id })
    }
  }
  return { ops, label: 'Удалить набор допущений' }
}
