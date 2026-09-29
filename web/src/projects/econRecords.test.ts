import { describe, expect, it } from 'vitest'

import type { State } from '../store/apply'
import {
  activeAssumptionEdit,
  addAssumptionSetEdit,
  addSharedCostEdit,
  assumptionSetsFromState,
  deleteAssumptionSetEdit,
  overridesEdit,
  paramsEdit,
  sharedCostsFromState,
} from './econRecords'

const A1 = '00000000-0000-4000-8000-000000000401'
const A2 = '00000000-0000-4000-8000-000000000402'
const C1 = '00000000-0000-4000-8000-000000000501'

function state(active: string | null = A1): State {
  return {
    project: {
      project: {
        id: 'project',
        name: 'Склад',
        object_type: 'warehouse',
        params: { area_m2: 5000, ceiling_m: 8 },
        match_selected_ids: ['r1'],
        econ_overrides: null,
        active_assumption_set_id: active,
      },
    },
    assumption_sets: {
      [A2]: { id: A2, order: 'a1', name: 'Оптимистичный', vat_rate: 0.2, prices_include_vat: true, vat_recoverable: true, labor_cash_share: 1 },
      [A1]: { id: A1, order: 'a0', name: 'Базовый', vat_rate: 0.2, prices_include_vat: true, vat_recoverable: false, labor_cash_share: 1 },
    },
    shared_costs: {
      [C1]: { id: C1, order: 'a0', code: 'wifi', label: 'Wi-Fi', bucket: 'capex', rub: 500000 },
    },
  } as unknown as State
}

describe('paramsEdit', () => {
  it('writes one operation per changed parameter and leaves the rest alone', () => {
    const edit = paramsEdit(state(), { area_m2: 6000, ceiling_m: 8 })
    expect(edit.ops).toEqual([{ op: 'set', coll: 'project', id: 'project', path: 'params.area_m2', value: 6000 }])
  })

  it('clears a parameter the form no longer has', () => {
    const edit = paramsEdit(state(), { area_m2: 5000 })
    expect(edit.ops).toEqual([{ op: 'set', coll: 'project', id: 'project', path: 'params.ceiling_m', value: null }])
  })

  it('says nothing when nothing changed', () => {
    expect(paramsEdit(state(), { area_m2: 5000, ceiling_m: 8 }).ops).toEqual([])
  })
})

describe('overridesEdit', () => {
  it('writes null when what-if is emptied, so the project has no leftover override', () => {
    expect(overridesEdit(state(), {}).ops).toEqual([])
    const set = overridesEdit(state(), { price_rub: 1_000_000 })
    expect(set.ops).toEqual([
      { op: 'set', coll: 'project', id: 'project', path: 'econ_overrides', value: { price_rub: 1_000_000 } },
    ])
  })
})

describe('assumption sets', () => {
  it('marks the set the project points at as the active one', () => {
    const sets = assumptionSetsFromState(state())
    expect(sets.map((a) => a.name)).toEqual(['Базовый', 'Оптимистичный'])
    expect(sets.map((a) => a.is_active)).toEqual([true, false])
  })

  it('reads a set stored before the discount rate existed as 15%', () => {
    expect(assumptionSetsFromState(state()).map((a) => a.discount_rate)).toEqual([0.15, 0.15])
  })

  it('a stored discount rate is kept', () => {
    const st = state()
    ;(st.assumption_sets as Record<string, Record<string, unknown>>)[A1].discount_rate = 0.2
    expect(assumptionSetsFromState(st)[0].discount_rate).toBe(0.2)
  })

  it('choosing a set writes the project, not the sets', () => {
    expect(activeAssumptionEdit(state(), A2).ops).toEqual([
      { op: 'set', coll: 'project', id: 'project', path: 'active_assumption_set_id', value: A2 },
    ])
    expect(activeAssumptionEdit(state(), A1).ops).toEqual([])
  })

  it('deleting the active set hands the project the next one', () => {
    const ops = deleteAssumptionSetEdit(state(), A1).ops
    expect(ops).toHaveLength(2)
    expect(ops[1]).toMatchObject({ path: 'active_assumption_set_id', value: A2 })
  })

  it('deleting an inactive set leaves the project pointer alone', () => {
    expect(deleteAssumptionSetEdit(state(), A2).ops).toHaveLength(1)
  })

  it('a new set that is active is inserted and pointed at in one transaction', () => {
    const ops = addAssumptionSetEdit(state(), {
      name: 'Пессимистичный',
      is_active: true,
      vat_rate: 0.2,
      prices_include_vat: true,
      vat_recoverable: false,
      labor_cash_share: 1,
      discount_rate: 0.18,
      sort_order: 2,
    }).ops
    expect(ops).toHaveLength(2)
    expect(ops[0]).toMatchObject({ op: 'insert', coll: 'assumption_sets', value: { discount_rate: 0.18 } })
  })
})

describe('shared costs', () => {
  it('lists the lines in their order', () => {
    expect(sharedCostsFromState(state()).map((c) => c.code)).toEqual(['wifi'])
  })

  it('puts a new line after the last one', () => {
    const op = addSharedCostEdit(state(), { code: 'charge', label: 'Зарядные', bucket: 'capex', rub: 0, sort_order: 1 })
      .ops[0] as { value: { order: string } }
    expect(op.value.order > 'a0').toBe(true)
  })
})
