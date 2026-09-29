import { describe, expect, it } from 'vitest'

import { apply, type RawOp, type State } from '../store/apply'
import { permissive } from '../store/state'
import { projectSchema } from '../store/store'

import { fleetEdit, pickEdit, renameEdit, variantsFromState } from './records'

const v1 = '00000000-0000-4000-8000-000000000201'
const v2 = '00000000-0000-4000-8000-000000000202'
const f1 = '00000000-0000-4000-8000-000000000301'
const f2 = '00000000-0000-4000-8000-000000000302'
const sol = '00000000-0000-4000-8000-0000000000e1'

function base(): State {
  return {
    processes: {
      p1: { id: 'p1', order: 'a0', code: 'inbound', name: 'Приёмка', task_type: 'pallet_inbound', is_baseline: true, point_ids: [], demand: {}, sla: {}, durations: {}, baseline_staff: {} },
    },
    variants: {
      [v1]: { id: v1, order: 'a0', name: 'Смешанный флот', status: 'draft', notes: '' },
      [v2]: { id: v2, order: 'a1', name: 'AMR', status: 'ready', notes: 'проверить' },
    },
    fleet_items: {
      [f1]: { id: f1, order: 'a0', variant_id: v1, solution_id: sol, quantity: 2, task_codes: ['inbound'], price_override_rub: null, price_override_reason: '' },
      [f2]: { id: f2, order: 'a1', variant_id: v1, solution_id: null, quantity: 1, task_codes: [], price_override_rub: 100, price_override_reason: 'КП' },
    },
    financing: {
      b1: { id: 'b1', variant_id: v1, kind: 'buy', tariff: null, assumptions: {} },
      r1: { id: 'r1', variant_id: v1, kind: 'raas', tariff: 'fixed', assumptions: {} },
    },
  }
}

function run(st: State, ops: RawOp[]): State {
  const r = apply(projectSchema(), st, { tx_id: 't', ops }, permissive)
  expect(r.outcome).toEqual({ status: 'applied' })
  return r.state
}

describe('variantsFromState', () => {
  it('lists variants in order with their fleet and financing', () => {
    const list = variantsFromState(base())
    expect(list.map((v) => [v.name, v.sort_order])).toEqual([
      ['Смешанный флот', 0],
      ['AMR', 1],
    ])
    expect(list[0].fleet.map((f) => [f.id, f.quantity, f.sort_order])).toEqual([
      [f1, 2, 0],
      [f2, 1, 1],
    ])
    expect(list[0].fleet[1].price_override_reason).toBe('КП')
    expect(list[0].financing.map((f) => `${f.kind}:${f.tariff ?? ''}`)).toEqual(['buy:', 'raas:fixed'])
    expect(list[1].notes).toBe('проверить')
    expect(list[1].fleet).toEqual([])
  })
})

describe('fleetEdit', () => {
  const fleet = () => variantsFromState(base())[0].fleet

  it('adds a robot at the end', () => {
    const next = [...fleet(), { solution_id: sol, quantity: 3, task_codes: [], price_override_rub: null, sort_order: 2 }]
    const edit = fleetEdit(base(), v1, next)
    expect(edit.label).toBe('Добавить робота')
    expect(edit.ops).toHaveLength(1)
    const got = variantsFromState(run(base(), edit.ops))[0].fleet
    expect(got.map((f) => f.quantity)).toEqual([2, 1, 3])
  })

  it('changes only the fields that changed', () => {
    const next = fleet().map((f, i) => (i === 0 ? { ...f, quantity: 5 } : f))
    const edit = fleetEdit(base(), v1, next)
    expect(edit).toEqual({ label: 'Изменить робота', ops: [{ op: 'set', coll: 'fleet_items', id: f1, path: 'quantity', value: 5 }] })
  })

  it('removes a robot and puts it back where it was', () => {
    const removed = fleetEdit(base(), v1, [fleet()[1]])
    expect(removed).toEqual({ label: 'Убрать робота', ops: [{ op: 'delete', coll: 'fleet_items', id: f1 }] })
    const st = run(base(), removed.ops)
    const back = fleetEdit(st, v1, [{ ...fleet()[0], id: undefined }, ...variantsFromState(st)[0].fleet])
    const got = variantsFromState(run(st, back.ops))[0].fleet
    expect(got.map((f) => f.quantity)).toEqual([2, 1])
  })

  it('reorders robots with move operations', () => {
    const [a, b] = fleet()
    const edit = fleetEdit(base(), v1, [b, a])
    expect(edit.ops.every((o) => o.op === 'move')).toBe(true)
    expect(variantsFromState(run(base(), edit.ops))[0].fleet.map((f) => f.id)).toEqual([f2, f1])
  })

  it('adds a picked robot at the end of a variant and renames a variant, each as one step', () => {
    const picked = pickEdit(base(), v2, sol, 12.6, ['inbound'])
    expect(picked.label).toBe('Выбрать робота')
    expect(picked.ops).toHaveLength(1)
    const after = run(base(), picked.ops)
    expect(variantsFromState(after)[1].fleet).toMatchObject([{ solution_id: sol, quantity: 13, task_codes: ['inbound'] }])
    expect(variantsFromState(run(after, pickEdit(after, v2, sol, 0).ops))[1].fleet.map((f) => f.quantity)).toEqual([13, 1])
    const renamed = run(base(), renameEdit(v1, 'Вариант А').ops)
    expect(variantsFromState(renamed)[0].name).toBe('Вариант А')
  })
})
