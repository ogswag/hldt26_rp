import { describe, expect, it } from 'vitest'

import fixture from '../../../contracts/ops/map_document.json'

import type { Rec, State } from './apply'
import { ProjectStore } from './store'
import { merge, UndoHistory } from './undo'

const newId = 'a0000000-0000-4000-8000-000000000001'

function processes(): State['processes'] {
  const out: Record<string, Rec> = {}
  ;['inbound', 'putaway', 'piece_pick', 'outbound'].forEach((code, i) => {
    const id = `00000000-0000-4000-8000-00000000010${i}`
    out[id] = { id, order: `a${i}`, code, name: code, task_type: 'piece_pick', is_baseline: true, point_ids: [], demand: {}, sla: {}, durations: {}, baseline_staff: {} }
  })
  return out
}

function setup() {
  const store = new ProjectStore('p')
  store.loadSnapshot(0, { ...(structuredClone(fixture.records) as unknown as State), processes: processes() })
  return { store, history: new UndoHistory(store) }
}

// remote applies another person's transaction as the server journal would.
function remote(store: ProjectStore, ops: Parameters<ProjectStore['dispatch']>[1]) {
  expect(store.applyJournal([{ seq: store.getSeq() + 1, tx_id: `r${store.getSeq()}`, ops }])).toBe(true)
}

const edges = (st: State) => Object.values(st.map_edges)
const firstEdge = fixture.document.layers.edges[0]

describe('UndoHistory', () => {
  it('undoes and redoes an insert', () => {
    const { store, history } = setup()
    history.run('Добавить точку', [{ op: 'insert', coll: 'map_points', id: newId, value: { kind: 'task', pos: { x: 1, y: 1 } } }])
    expect(history.getState()).toEqual({ undo: 'Добавить точку', redo: null })
    history.undo()
    expect(store.getState().map_points[newId]).toBeUndefined()
    expect(history.getState()).toEqual({ undo: null, redo: 'Добавить точку' })
    history.redo()
    expect(store.getState().map_points[newId]).toMatchObject({ kind: 'task', pos: { x: 1, y: 1 } })
  })

  it('brings back a deleted point with its old id and everything the delete cascaded into', () => {
    const { store, history } = setup()
    const before = store.getState()
    const id = firstEdge.from
    history.run('Удалить точку', [{ op: 'delete', coll: 'map_points', id }])
    expect(edges(store.getState()).some((e) => e.from === id || e.to === id)).toBe(false)
    expect(history.undo()).toEqual({ status: 'applied' })
    const after = store.getState()
    for (const coll of ['map_points', 'map_edges', 'map_resources', 'map_flows']) {
      expect(after[coll]).toEqual(before[coll])
    }
  })

  it('keeps what someone else changed in another field', () => {
    const { store, history } = setup()
    const id = firstEdge.from
    history.run('Переместить точку', [{ op: 'set', coll: 'map_points', id, path: 'pos', value: { x: 1, y: 2 } }])
    remote(store, [{ op: 'set', coll: 'map_points', id, path: 'name', value: 'Док Б' }])
    history.undo()
    const p = store.getState().map_points[id]
    expect(p.name).toBe('Док Б')
    expect(p.pos).toEqual((fixture.records.map_points as Record<string, Rec>)[id].pos)
  })

  it('undo puts back this tab\'s old value over a newer edit of the same field, and redo brings the newer one back', () => {
    const { store, history } = setup()
    const id = firstEdge.from
    const was = (store.getState().map_points[id] as Rec).name
    const ops = [{ op: 'set', coll: 'map_points', id, path: 'name', value: 'Моё имя' }]
    const mine = history.run('Имя точки', ops)
    // The server confirms this tab's edit, then someone else changes the same field.
    expect(store.applyJournal([{ seq: store.getSeq() + 1, tx_id: mine.txId, ops }])).toBe(true)
    remote(store, [{ op: 'set', coll: 'map_points', id, path: 'name', value: 'Чужое имя' }])
    history.undo()
    expect(store.getState().map_points[id].name).toBe(was)
    history.redo()
    expect(store.getState().map_points[id].name).toBe('Чужое имя')
  })

  it('refuses an undo whose target someone deleted', () => {
    const { store, history } = setup()
    const id = firstEdge.from
    history.run('Переместить точку', [{ op: 'set', coll: 'map_points', id, path: 'pos', value: { x: 1, y: 2 } }])
    remote(store, [{ op: 'delete', coll: 'map_points', id }])
    expect(history.undo()).toMatchObject({ status: 'rejected', reason: 'target_missing' })
    expect(store.getRejected()).toHaveLength(1)
    expect(store.getRejected()[0]).toMatchObject({ label: 'Отменить: переместить точку', reason: 'target_missing' })
    expect(history.getState()).toEqual({ undo: null, redo: null })
  })

  it('leaves alone a record the step inserted and someone deleted', () => {
    const { store, history } = setup()
    const ops = [{ op: 'insert', coll: 'map_points', id: newId, value: { kind: 'task', pos: { x: 1, y: 1 } } }]
    const { txId } = history.run('Добавить точку', ops)
    expect(store.applyJournal([{ seq: 1, tx_id: txId, ops }])).toBe(true)
    remote(store, [{ op: 'delete', coll: 'map_points', id: newId }])
    expect(history.undo()).toEqual({ status: 'applied' })
    expect(store.getRejected()).toHaveLength(0)
  })

  it('makes one step of nudges with the same merge key', () => {
    const { store, history } = setup()
    const id = firstEdge.from
    const pos = store.getState().map_points[id].pos as { x: number; y: number }
    for (let i = 1; i <= 3; i++) {
      history.run('Переместить точку', [{ op: 'set', coll: 'map_points', id, path: 'pos', value: { x: pos.x + i, y: pos.y } }], `nudge:${id}`)
    }
    history.undo()
    expect(store.getState().map_points[id].pos).toEqual(pos)
    expect(history.getState().undo).toBeNull()
  })
})

describe('merge', () => {
  it('drops a record inserted and deleted in the same step', () => {
    const ins = { coll: 'c', id: '1', kind: 'insert' as const, after: { id: '1' } }
    const del = { coll: 'c', id: '1', kind: 'delete' as const, before: { id: '1' } }
    expect(merge([ins], [del])).toEqual([])
  })
})
