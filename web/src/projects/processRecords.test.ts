import { describe, expect, it } from 'vitest'

import type { State } from '../store/apply'
import { addProcessEdit, deleteProcessEdit, processFieldEdit, processesFromState } from './processRecords'

const P1 = '00000000-0000-4000-8000-000000000301'
const P2 = '00000000-0000-4000-8000-000000000302'

function process(id: string, order: string, code: string, name: string) {
  return {
    id,
    order,
    code,
    name,
    task_type: 'pallet_move',
    is_baseline: true,
    demand: { units_per_day: 100, unit: 'палл.' },
    sla: { max_wait_min: 30 },
    durations: { load_s: 60, unload_s: 60 },
    baseline_staff: { headcount: 2 },
    point_ids: [],
  }
}

function state(): State {
  return {
    processes: {
      [P2]: process(P2, 'a1', 'pick', 'Отбор'),
      [P1]: process(P1, 'a0', 'move', 'Перемещение'),
    },
  } as unknown as State
}

describe('processesFromState', () => {
  it('lists the processes in their order, not in the order the records happen to sit', () => {
    expect(processesFromState(state()).map((p) => p.code)).toEqual(['move', 'pick'])
    expect(processesFromState(state()).map((p) => p.sort_order)).toEqual([0, 1])
  })
})

describe('processFieldEdit', () => {
  it('writes only the fields that actually changed', () => {
    const edit = processFieldEdit(state(), P1, { name: 'Перевозка', is_baseline: true })
    expect(edit.ops).toEqual([{ op: 'set', coll: 'processes', id: P1, path: 'name', value: 'Перевозка' }])
  })

  it('writes a nested object as one value, so two people editing two fields do not clash on the record', () => {
    const edit = processFieldEdit(state(), P1, { sla: { max_wait_min: 15 } })
    expect(edit.ops).toEqual([{ op: 'set', coll: 'processes', id: P1, path: 'sla', value: { max_wait_min: 15 } }])
  })

  it('does nothing for a process someone else already deleted', () => {
    expect(processFieldEdit(state(), 'missing', { name: 'x' }).ops).toEqual([])
  })
})

describe('addProcessEdit', () => {
  it('puts the new process after the last one', () => {
    const edit = addProcessEdit(state(), { ...process('new-id', '', 'wash', 'Мойка'), sort_order: 2 })
    expect(edit.ops).toHaveLength(1)
    const op = edit.ops[0] as { op: string; coll: string; id: string; value: Record<string, unknown> }
    expect(op.op).toBe('insert')
    expect(op.value.code).toBe('wash')
    expect(String(op.value.order) > 'a1').toBe(true)
  })
})

describe('deleteProcessEdit', () => {
  it('removes the record and leaves the rest to the schema', () => {
    expect(deleteProcessEdit(P1).ops).toEqual([{ op: 'delete', coll: 'processes', id: P1 }])
  })
})
