import { describe, expect, it } from 'vitest'

import { apply, type Change, type State } from './apply'
import { deleter, restoreOps, title, values } from './rejected'
import { permissive } from './state'
import { projectSchema, ProjectStore, type Rejected } from './store'

function state(): State {
  return { map_points: { p1: { id: 'p1', kind: 'task', name: 'Приёмка 2', pos: { x: 1, y: 1 }, process_code: null } }, processes: {} }
}

function entry(intended: Change[], label = 'Переместить точку'): Rejected {
  return { txId: 't', label, reason: 'target_missing', at: 0, intended }
}

describe('deleter', () => {
  it('names the other person by email and the own tab without a name', () => {
    const r = { ...entry([]), actor: 'u2', actorEmail: 'maks@demo.local' }
    expect(deleter(r, 'u1')).toBe('удалил maks@demo.local')
    expect(deleter({ ...r, actor: 'u1', actorEmail: 'anna@demo.local' }, 'u1')).toBe('удалено в другой вкладке')
    expect(deleter({ ...r, actorEmail: undefined }, 'u1')).toBe('удалил другой участник')
    expect(deleter(entry([]), 'u1')).toBe('')
  })
})

describe('rejected', () => {
  it('names the action by its label and the record name', () => {
    expect(title(entry([{ coll: 'map_points', id: 'p1', kind: 'update', before: state().map_points.p1, after: { ...state().map_points.p1, pos: { x: 5, y: 1 } } }]))).toBe(
      'Переместить точку «Приёмка 2»',
    )
    expect(title(entry([], 'Удалить точку'))).toBe('Удалить точку')
  })

  it('lists the values the change carried', () => {
    const before = state().map_points.p1
    expect(values(entry([{ coll: 'map_points', id: 'p1', kind: 'update', before, after: { ...before, name: 'Приёмка 3' } }]))).toBe('name: Приёмка 3')
  })

  it('puts back a record someone deleted', () => {
    const before = state().map_points.p1
    const ops = restoreOps(projectSchema(), {}, [{ coll: 'map_points', id: 'p1', kind: 'update', before, after: { ...before, pos: { x: 5, y: 1 } } }])
    expect(ops).toEqual([{ op: 'insert', coll: 'map_points', id: 'p1', value: { kind: 'task', name: 'Приёмка 2', pos: { x: 5, y: 1 }, process_code: null } }])
    const st = apply(projectSchema(), { map_points: {}, processes: {} }, { tx_id: 'r', ops }, permissive)
    expect(st.outcome).toEqual({ status: 'applied' })
    expect(st.state.map_points.p1.pos).toEqual({ x: 5, y: 1 })
  })

  it('writes the fields again when the record is still there', () => {
    const before = state().map_points.p1
    expect(restoreOps(projectSchema(), state(), [{ coll: 'map_points', id: 'p1', kind: 'update', before, after: { ...before, name: 'Приёмка 3' } }])).toEqual([
      { op: 'set', coll: 'map_points', id: 'p1', path: 'name', value: 'Приёмка 3' },
    ])
  })

  it('leaves the list when the entry is hidden', () => {
    const store = new ProjectStore('p')
    store.loadSnapshot(0, state())
    store.refuse('Переместить точку', 'target_missing', undefined, [])
    expect(store.getRejected()).toHaveLength(1)
    store.dismiss(store.getRejected()[0].txId)
    expect(store.getRejected()).toEqual([])
  })
})
