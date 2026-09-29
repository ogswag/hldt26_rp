import { describe, expect, it } from 'vitest'

import fixture from '../../../contracts/ops/map_document.json'
import type { MapDocument } from '../api/client'
import { apply, type RawOp, type State } from '../store/apply'
import { permissive } from '../store/state'
import { projectSchema } from '../store/store'

import { forSave } from './document'
import { commandOps, deleteMapOps, mapFromState, mapSelector } from './records'

const records = fixture.records as unknown as State
const document = fixture.document as unknown as MapDocument
const page = { width_px: 1000, height_px: 700, source_kind: 'none' as const }

function run(st: State, ops: RawOp[]): State {
  const r = apply(projectSchema(), st, { tx_id: 't', ops }, permissive)
  expect(r.outcome).toEqual({ status: 'applied' })
  return r.state
}

function process(code: string, order: string) {
  const id = `00000000-0000-4000-8000-0000000001${order.slice(1).padStart(2, '0')}`
  return [id, { id, order, code, name: code, task_type: 'piece_pick', is_baseline: true, point_ids: [], demand: {}, sla: {}, durations: {}, baseline_staff: {} }] as const
}

// empty is a project without a map and with the template's processes.
function empty(): State {
  return {
    map: { map: { id: 'map', page: null, profile: 'indoor', meters_per_px: 0.1, segment: null, check: null } },
    map_points: {},
    map_edges: {},
    map_zones: {},
    map_obstacles: {},
    map_resources: {},
    map_flows: {},
    processes: Object.fromEntries(['inbound', 'putaway', 'piece_pick', 'outbound'].map((c, i) => process(c, `a${i}`))),
  }
}

function withoutFlowIds(doc: MapDocument | null): MapDocument | null {
  return doc && { ...doc, layers: { ...doc.layers, flows: (doc.layers.flows ?? []).map((f) => ({ ...f, id: undefined })) } }
}

describe('mapFromState', () => {
  it('builds the document toMap builds from the same records', () => {
    expect(mapFromState(records)).toEqual(document)
  })

  it('returns null while map.page is null', () => {
    expect(mapFromState(empty())).toBeNull()
  })

  it('keeps the document while no map collection changes', () => {
    const select = mapSelector()
    const a = select(records)
    expect(select({ ...records, processes: {} })).toBe(a)
    expect(select({ ...records, map_points: { ...records.map_points } })).not.toBe(a)
  })
})

describe('commandOps', () => {
  it('replaces the map with a document in one transaction', () => {
    const want = withoutFlowIds(forSave(document))
    const st = run(empty(), commandOps(empty(), { type: 'replace', doc: document }, page))
    expect(withoutFlowIds(mapFromState(st))).toEqual(want)
    const again = run(st, commandOps(st, { type: 'replace', doc: document }, page))
    expect(withoutFlowIds(mapFromState(again))).toEqual(want)
  })

  it('leaves out flows of processes the project does not have', () => {
    const st = empty()
    delete (st.processes as Record<string, unknown>)[process('outbound', 'a3')[0]]
    const got = mapFromState(run(st, commandOps(st, { type: 'replace', doc: document }, page)))
    expect(got?.layers.flows?.map((f) => f.process_code)).not.toContain('outbound')
  })

  it('creates the map with the first feature', () => {
    const ops = commandOps(empty(), { type: 'addPoint', point: { id: 'b3a5e0f2-7d7c-4c3e-9d55-0d1f2e3a4b5c', kind: 'task', x: 1.004, y: 2 } }, page)
    expect(ops[0]).toEqual({ op: 'set', coll: 'map', id: 'map', path: 'page', value: page })
    const doc = mapFromState(run(empty(), ops))
    expect(doc?.layers.points).toEqual([{ id: 'b3a5e0f2-7d7c-4c3e-9d55-0d1f2e3a4b5c', kind: 'task', x: 1, y: 2 }])
  })

  it('deletes a point with one operation and lets the schema cascade', () => {
    const edge = document.layers.edges[0]
    const ops = commandOps(records, { type: 'deletePoint', id: edge.from }, page)
    expect(ops).toEqual([{ op: 'delete', coll: 'map_points', id: edge.from }])
    const doc = mapFromState(run(records, ops))
    expect(doc?.layers.edges.some((e) => e.from === edge.from || e.to === edge.from)).toBe(false)
    expect(doc?.layers.flows?.some((f) => [...f.pickup_point_ids, ...f.drop_point_ids].includes(edge.from))).toBe(false)
  })

  it('refuses an edge that repeats another in reverse', () => {
    const e = document.layers.edges[0]
    expect(commandOps(records, { type: 'addEdge', edge: { id: 'a0000000-0000-4000-8000-000000000001', from: e.to, to: e.from } }, page)).toEqual([])
  })

  it('refuses a loop, a missing end and a repeated id', () => {
    const e = document.layers.edges[0]
    const add = (edge: { id: string; from: string; to: string }) => commandOps(records, { type: 'addEdge', edge }, page)
    expect(add({ id: 'a0000000-0000-4000-8000-000000000002', from: e.from, to: e.from })).toEqual([])
    expect(add({ id: 'a0000000-0000-4000-8000-000000000003', from: e.from, to: 'nope' })).toEqual([])
    expect(add({ id: e.id, from: e.from, to: document.layers.points.at(-1)?.id ?? '' })).toEqual([])
  })

  it('removes a deleted edge from the narrow aisle that holds it', () => {
    const st = run(records, commandOps(records, { type: 'deleteEdge', id: 'e-extra' }, page))
    expect(mapFromState(st)?.layers.resources.find((r) => r.id === 'r-extra')?.edge_ids).toBeUndefined()
  })

  it('calibrates from a rounded segment and ignores a zero one', () => {
    const ops = commandOps(records, { type: 'calibrate', segment: { x1: 0, y1: 0, x2: 200.004, y2: 0, length_m: 10 } }, page)
    expect(ops).toEqual([
      { op: 'set', coll: 'map', id: 'map', path: 'segment', value: { x1: 0, y1: 0, x2: 200, y2: 0, length_m: 10 } },
      { op: 'set', coll: 'map', id: 'map', path: 'meters_per_px', value: 0.05 },
    ])
    expect(commandOps(records, { type: 'calibrate', segment: { x1: 0, y1: 0, x2: 0, y2: 0, length_m: 10 } }, page)).toEqual([])
    const scaled = mapFromState(run(records, commandOps(records, { type: 'setScale', metersPerPx: 0.2 }, page)))
    expect(scaled?.calibration).toEqual({ meters_per_px: 0.2, check: document.calibration.check })
  })

  it('sets only the fields that change and rounds coordinates', () => {
    const p = document.layers.points[0]
    expect(commandOps(records, { type: 'updatePoint', id: p.id, patch: { x: p.x, y: p.y } }, page)).toEqual([])
    expect(commandOps(records, { type: 'updatePoint', id: p.id, patch: { x: 10.126 } }, page)).toEqual([
      { op: 'set', coll: 'map_points', id: p.id, path: 'pos', value: { x: 10.13, y: p.y } },
    ])
  })

  it('removes the map', () => {
    const st = run(records, deleteMapOps(records))
    expect(mapFromState(st)).toBeNull()
    expect(Object.keys(st.map_points)).toEqual([])
    expect(Object.keys(st.map_zones)).toEqual([])
  })
})
