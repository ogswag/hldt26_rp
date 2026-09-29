// Variants, fleet and financing in the project store. variantsFromState builds what the page draws; the ops
// functions turn one edit into one transaction.

import type { Financing, FleetItem, SolutionVariant } from '../api/client'
import { equal, type RawOp, type Rec, type State } from '../store/apply'
import { uuid } from '../store/clientId'
import { keyBetween, keysAfter } from '../store/order'
import type { FinancingRecord, FleetItemsRecord, VariantsRecord } from '../store/schema.gen'

export type Edit = { ops: RawOp[]; label: string }

const collections = ['variants', 'fleet_items', 'financing'] as const

function recs<T>(st: State, coll: string): Record<string, T> {
  return (st[coll] ?? {}) as Record<string, T>
}

function byOrder<T extends { id: string; order: string }>(list: T[]): T[] {
  return [...list].sort((a, b) => (a.order === b.order ? (a.id < b.id ? -1 : 1) : a.order < b.order ? -1 : 1))
}

// financing rows have no order; buy comes first, then the tariffs in a fixed order, as the server writes them.
const tariffRank = { fixed: 1, variable: 2, mixed: 3, null: 0 } as Record<string, number>

function financingOf(st: State, variantId: string): Financing[] {
  return Object.values(recs<FinancingRecord>(st, 'financing'))
    .filter((f) => f.variant_id === variantId)
    .sort((a, b) => (a.kind === b.kind ? tariffRank[String(a.tariff)] - tariffRank[String(b.tariff)] : a.kind === 'buy' ? -1 : 1))
    .map((f) => ({ id: f.id, kind: f.kind, tariff: f.tariff, assumptions: (f.assumptions ?? {}) as Record<string, unknown> }))
}

function fleetOf(st: State, variantId: string): FleetItem[] {
  return byOrder(Object.values(recs<FleetItemsRecord>(st, 'fleet_items')).filter((f) => f.variant_id === variantId)).map((f, i) => ({
    id: f.id,
    solution_id: f.solution_id,
    quantity: f.quantity,
    task_codes: f.task_codes,
    price_override_rub: f.price_override_rub,
    ...(f.price_override_reason ? { price_override_reason: f.price_override_reason } : {}),
    sort_order: i,
  }))
}

// variantsFromState returns the project's variants in the shape the pages use.
export function variantsFromState(st: State): SolutionVariant[] {
  return byOrder(Object.values(recs<VariantsRecord>(st, 'variants'))).map((v, i) => ({
    id: v.id,
    name: v.name,
    status: v.status,
    ...(v.notes ? { notes: v.notes } : {}),
    sort_order: i,
    fleet: fleetOf(st, v.id),
    financing: financingOf(st, v.id),
  }))
}

// variantsSelector rebuilds the list only when one of its collections changed.
export function variantsSelector(): (st: State) => SolutionVariant[] {
  let refs: unknown[] = []
  let out: SolutionVariant[] = []
  return (st) => {
    const next = collections.map((c) => st[c])
    if (next.length !== refs.length || next.some((r, i) => r !== refs[i])) {
      refs = next
      out = variantsFromState(st)
    }
    return out
  }
}

function fleetValue(item: FleetItem, variantId: string): Rec {
  return {
    variant_id: variantId,
    solution_id: item.solution_id,
    quantity: item.quantity,
    task_codes: item.task_codes ?? [],
    price_override_rub: item.price_override_rub,
    price_override_reason: item.price_override_reason ?? '',
  }
}

// orderKeys returns a key for every item of the wanted sequence, reusing the keys of records that stay in
// order. It regenerates the whole sequence when the wanted order cannot be written with the keys at hand.
function orderKeys(current: (string | null)[]): string[] {
  const out: string[] = []
  let prev: string | null = null
  for (let i = 0; i < current.length; i++) {
    const key = current[i]
    if (key !== null && (prev === null || key > prev)) {
      out.push(key)
      prev = key
      continue
    }
    // The next key that is already in place bounds the gap this item goes into.
    let next: string | null = null
    for (let j = i + 1; j < current.length; j++) {
      const k = current[j]
      if (k !== null && (prev === null || k > prev)) {
        next = k
        break
      }
    }
    const fresh = keyBetween(prev, next)
    if (next !== null && !(fresh < next)) {
      return keysAfter(null, current.length)
    }
    out.push(fresh)
    prev = fresh
  }
  return out
}

// fleetEdit turns the fleet of one variant into operations: new robots are inserted, changed ones are set,
// removed ones are deleted, and a robot that moved gets a new order key.
export function fleetEdit(st: State, variantId: string, next: FleetItem[]): Edit {
  const before = recs<FleetItemsRecord>(st, 'fleet_items')
  const kept = new Set<string>()
  const ops: RawOp[] = []
  const keys = orderKeys(next.map((item) => (item.id && before[item.id] ? before[item.id].order : null)))
  let inserted = 0
  let changed = 0
  next.forEach((item, i) => {
    const rec = item.id ? before[item.id] : undefined
    const value = fleetValue(item, variantId)
    if (!rec) {
      ops.push({ op: 'insert', coll: 'fleet_items', id: item.id ?? uuid(), value: { order: keys[i], ...value } })
      inserted++
      return
    }
    kept.add(rec.id)
    for (const [path, v] of Object.entries(value)) {
      if (!equal((rec as unknown as Rec)[path], v)) {
        ops.push({ op: 'set', coll: 'fleet_items', id: rec.id, path, value: v })
        changed++
      }
    }
    if (rec.order !== keys[i]) {
      ops.push({ op: 'move', coll: 'fleet_items', id: rec.id, order: keys[i] })
    }
  })
  let deleted = 0
  for (const rec of Object.values(before)) {
    if (rec.variant_id === variantId && !kept.has(rec.id)) {
      ops.push({ op: 'delete', coll: 'fleet_items', id: rec.id })
      deleted++
    }
  }
  let label = 'Изменить флот'
  if (inserted > 0 && changed === 0 && deleted === 0) {
    label = 'Добавить робота'
  } else if (deleted > 0 && inserted === 0 && changed === 0) {
    label = 'Убрать робота'
  } else if (changed > 0 && inserted === 0 && deleted === 0) {
    label = 'Изменить робота'
  }
  return { ops, label }
}


// pickEdit adds a robot to a variant with the count and the processes the calculation estimated for it.
export function pickEdit(st: State, variantId: string, solutionId: string, quantity: number, processCodes: string[] = []): Edit {
  const fleet = variantsFromState(st).find((v) => v.id === variantId)?.fleet ?? []
  const item = { solution_id: solutionId, quantity: Math.max(1, Math.round(quantity)), task_codes: processCodes, price_override_rub: null }
  const next = [...fleet, { ...item, sort_order: fleet.length }]
  return { ops: fleetEdit(st, variantId, next).ops, label: 'Выбрать робота' }
}

// renameEdit gives a variant a new name.
export function renameEdit(variantId: string, name: string): Edit {
  return { ops: [{ op: 'set', coll: 'variants', id: variantId, path: 'name', value: name }], label: 'Переименовать вариант' }
}
