// Map records in the project store. mapFromState builds the document the editor draws, as toMap in
// api/internal/ops/draft.go does; commandOps turns an editor command into operations.

import type { MapDocument, MapEdge, MapFlow, MapPoint, MapPolygon, MapResource, MapSegment, MapSourceKind, MapXY } from '../api/client'
import { equal, type RawOp, type Rec, type State } from '../store/apply'
import { uuid } from '../store/clientId'
import { keyBetween } from '../store/order'
import type {
  MapEdgesRecord,
  MapFlowsRecord,
  MapObstaclesRecord,
  MapPointsRecord,
  MapRecord,
  MapResourcesRecord,
  MapZonesRecord,
  ProcessesRecord,
} from '../store/schema.gen'

import type { MapCommand, PolygonLayer } from './commands'
import { metersPerPx } from './document'

export type MapPage = { width_px: number; height_px: number; source_kind: MapSourceKind }

const collections = ['map', 'map_points', 'map_edges', 'map_zones', 'map_obstacles', 'map_resources', 'map_flows'] as const

// Deleting in this order removes each record once: flows and resources first, then edges, so deleting a point
// cascades into nothing.
const featureCollections = ['map_flows', 'map_resources', 'map_edges', 'map_points', 'map_zones', 'map_obstacles'] as const

const polygonColl: Record<PolygonLayer, 'map_zones' | 'map_obstacles'> = { zones: 'map_zones', obstacles: 'map_obstacles' }

const defaultMetersPerPx = 0.1

function recs<T>(st: State, coll: string): Record<string, T> {
  return (st[coll] ?? {}) as Record<string, T>
}

function byId<T extends { id: string }>(list: T[]): T[] {
  return list.sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
}

function byOrder<T extends { id: string; order: string }>(list: T[]): T[] {
  return byId(list).sort((a, b) => (a.order < b.order ? -1 : a.order > b.order ? 1 : 0))
}

function nonEmpty(list: string[]): string[] | undefined {
  return list.length > 0 ? list : undefined
}

function polygons(st: State, coll: string): MapPolygon[] {
  return byOrder(Object.values(recs<MapZonesRecord | MapObstaclesRecord>(st, coll))).map((r) => ({
    id: r.id,
    kind: r.kind,
    ...(r.name ? { name: r.name } : {}),
    ring: r.ring,
  }))
}

// mapFromState returns the project's map, or null when it has none (map.page is null).
export function mapFromState(st: State): MapDocument | null {
  const m = recs<MapRecord>(st, 'map').map
  if (!m || m.page === null || m.page === undefined) {
    return null
  }
  const points: MapPoint[] = byId(Object.values(recs<MapPointsRecord>(st, 'map_points'))).map((r) => ({
    id: r.id,
    kind: r.kind,
    ...(r.name ? { name: r.name } : {}),
    x: r.pos.x,
    y: r.pos.y,
    ...(r.process_code ? { process_code: r.process_code } : {}),
  }))
  const edges: MapEdge[] = byId(Object.values(recs<MapEdgesRecord>(st, 'map_edges'))).map((r) => ({
    id: r.id,
    from: r.from,
    to: r.to,
    ...(r.bidirectional !== null ? { bidirectional: r.bidirectional } : {}),
    ...(r.width_m !== null ? { width_m: r.width_m } : {}),
  }))
  const resources: MapResource[] = byId(Object.values(recs<MapResourcesRecord>(st, 'map_resources'))).map((r) => {
    const out: MapResource = { id: r.id, kind: r.kind, capacity: r.capacity }
    if (r.name) {
      out.name = r.name
    }
    if (r.point_id) {
      out.point_id = r.point_id
    }
    const pointIds = nonEmpty(r.point_ids)
    if (pointIds) {
      out.point_ids = pointIds
    }
    const edgeIds = nonEmpty(r.edge_ids)
    if (edgeIds) {
      out.edge_ids = edgeIds
    }
    return out
  })
  const flows: MapFlow[] = byId(Object.values(recs<MapFlowsRecord>(st, 'map_flows')))
    .sort((a, b) => (a.process_code < b.process_code ? -1 : a.process_code > b.process_code ? 1 : 0))
    .map((r) => ({ id: r.id, process_code: r.process_code, pickup_point_ids: r.pickup_point_ids, drop_point_ids: r.drop_point_ids }))
  const segment = m.segment as MapSegment | null
  const check = m.check as MapSegment | null
  return {
    schema_version: 'map-v1',
    profile: m.profile,
    units: 'm',
    page: m.page as MapPage,
    calibration: {
      meters_per_px: m.meters_per_px,
      ...(segment ? { segment } : {}),
      ...(check ? { check } : {}),
    },
    layers: { zones: polygons(st, 'map_zones'), obstacles: polygons(st, 'map_obstacles'), points, resources, edges, flows },
  }
}

// mapSelector returns a store selector that rebuilds the document only when a map collection changed, so the
// editor keeps the same object while other parts of the project move.
export function mapSelector(): (st: State) => MapDocument | null {
  let refs: unknown[] = []
  let doc: MapDocument | null = null
  return (st) => {
    const next = collections.map((c) => st[c])
    if (next.length !== refs.length || next.some((r, i) => r !== refs[i])) {
      refs = next
      doc = mapFromState(st)
    }
    return doc
  }
}

// newFeatureId gives a new map feature its id. Features from maps saved before operations keep ids like "p1".
export function newFeatureId(): string {
  return uuid()
}

const round = (v: number) => Math.round(v * 100) / 100

function xy(p: MapXY): MapXY {
  return { x: round(p.x), y: round(p.y) }
}

function ring(r: MapXY[]): MapXY[] {
  return r.map(xy)
}

function segment(s: MapSegment): MapSegment {
  return { x1: round(s.x1), y1: round(s.y1), x2: round(s.x2), y2: round(s.y2), length_m: s.length_m }
}

function page(p: MapPage): MapPage {
  return { width_px: p.width_px, height_px: p.height_px, source_kind: p.source_kind }
}

// setOps sets the fields of rec that differ from next.
function setOps(coll: string, rec: Rec, next: Rec): RawOp[] {
  const out: RawOp[] = []
  for (const [path, value] of Object.entries(next)) {
    if (!equal(rec[path], value)) {
      out.push({ op: 'set', coll, id: rec.id as string, path, value })
    }
  }
  return out
}

function mapSet(st: State, next: Partial<Record<keyof Omit<MapRecord, 'id'>, unknown>>): RawOp[] {
  const m = recs<MapRecord>(st, 'map').map as unknown as Rec | undefined
  return m ? setOps('map', m, next) : []
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

function pointValue(p: MapPoint, codes: Set<string>): Rec {
  return {
    kind: p.kind,
    name: p.name ?? '',
    pos: xy(p),
    process_code: p.process_code && codes.has(p.process_code) ? p.process_code : null,
  }
}

function edgeValue(e: MapEdge): Rec {
  return {
    from: e.from,
    to: e.to,
    bidirectional: e.bidirectional ?? null,
    width_m: e.width_m !== undefined && e.width_m >= 0.01 && e.width_m <= 100 ? e.width_m : null,
  }
}

function resourceValue(r: MapResource): Rec {
  return {
    kind: r.kind,
    name: r.name ?? '',
    capacity: r.capacity,
    point_id: r.point_id ?? null,
    point_ids: r.point_ids ?? [],
    edge_ids: r.edge_ids ?? [],
  }
}

function processCodes(st: State): Set<string> {
  return new Set(Object.values(recs<ProcessesRecord>(st, 'processes')).map((p) => p.code))
}

function sameEdge(st: State, from: string, to: string): boolean {
  return Object.values(recs<MapEdgesRecord>(st, 'map_edges')).some(
    (e) => (e.from === from && e.to === to) || (e.from === to && e.to === from),
  )
}

// clearOps deletes every map feature.
function clearOps(st: State): RawOp[] {
  const out: RawOp[] = []
  for (const coll of featureCollections) {
    for (const id of Object.keys(recs(st, coll)).sort()) {
      out.push({ op: 'delete', coll, id })
    }
  }
  return out
}

// replaceOps turns the map into doc in one transaction. Features keep the document's ids; flows and point
// bindings to processes the project does not have are left out, as the schema refuses them.
function replaceOps(st: State, doc: MapDocument): RawOp[] {
  const codes = processCodes(st)
  const out = clearOps(st)
  out.push(
    ...mapSet(st, {
      page: page(doc.page ?? { width_px: 1000, height_px: 700, source_kind: 'none' }),
      profile: doc.profile,
      meters_per_px: doc.calibration.meters_per_px,
      segment: doc.calibration.segment ? segment(doc.calibration.segment) : null,
      check: doc.calibration.check ? segment(doc.calibration.check) : null,
    }),
  )
  const l = doc.layers
  const points = new Set<string>()
  for (const p of l.points) {
    if (!points.has(p.id)) {
      points.add(p.id)
      out.push({ op: 'insert', coll: 'map_points', id: p.id, value: pointValue(p, codes) })
    }
  }
  const edges = new Set<string>()
  const pairs = new Set<string>()
  for (const e of l.edges) {
    const pair = [e.from, e.to].sort().join(' ')
    if (e.from === e.to || !points.has(e.from) || !points.has(e.to) || edges.has(e.id) || pairs.has(pair)) {
      continue
    }
    edges.add(e.id)
    pairs.add(pair)
    out.push({ op: 'insert', coll: 'map_edges', id: e.id, value: edgeValue(e) })
  }
  for (const layer of ['zones', 'obstacles'] as const) {
    let prev: string | null = null
    for (const p of l[layer]) {
      prev = keyBetween(prev, null)
      out.push({
        op: 'insert',
        coll: polygonColl[layer],
        id: p.id,
        value: { order: prev, kind: p.kind, name: p.name ?? '', ring: ring(p.ring) },
      })
    }
  }
  for (const r of l.resources) {
    const v = resourceValue(r)
    v.point_id = r.point_id && points.has(r.point_id) ? r.point_id : null
    v.point_ids = (r.point_ids ?? []).filter((id) => points.has(id))
    v.edge_ids = (r.edge_ids ?? []).filter((id) => edges.has(id))
    out.push({ op: 'insert', coll: 'map_resources', id: r.id, value: v })
  }
  const flowCodes = new Set<string>()
  for (const f of l.flows ?? []) {
    if (!codes.has(f.process_code) || flowCodes.has(f.process_code)) {
      continue
    }
    flowCodes.add(f.process_code)
    out.push({
      op: 'insert',
      coll: 'map_flows',
      id: uuid(),
      value: {
        process_code: f.process_code,
        pickup_point_ids: f.pickup_point_ids.filter((id) => points.has(id)),
        drop_point_ids: f.drop_point_ids.filter((id) => points.has(id)),
      },
    })
  }
  return out
}

// deleteMapOps removes the map: the page goes back to null and every feature is deleted.
export function deleteMapOps(st: State): RawOp[] {
  return [
    ...clearOps(st),
    ...mapSet(st, { page: null, profile: 'indoor', meters_per_px: defaultMetersPerPx, segment: null, check: null }),
  ]
}

function commandBody(st: State, cmd: MapCommand): RawOp[] {
  switch (cmd.type) {
    case 'replace':
      return replaceOps(st, cmd.doc)
    case 'setPage':
      return mapSet(st, { page: page(cmd.page) })
    case 'calibrate': {
      const seg = segment(cmd.segment)
      const mpp = metersPerPx(seg)
      return mpp === null ? [] : mapSet(st, { segment: seg, meters_per_px: mpp })
    }
    case 'setScale':
      return cmd.metersPerPx > 0 ? mapSet(st, { meters_per_px: cmd.metersPerPx, segment: null }) : []
    case 'setCheck':
      return mapSet(st, { check: cmd.segment ? segment(cmd.segment) : null })
    case 'addPoint':
      if (recs(st, 'map_points')[cmd.point.id]) {
        return []
      }
      return [{ op: 'insert', coll: 'map_points', id: cmd.point.id, value: pointValue(cmd.point, processCodes(st)) }]
    case 'updatePoint': {
      const r = recs<MapPointsRecord>(st, 'map_points')[cmd.id]
      if (!r) {
        return []
      }
      const p = cmd.patch
      const next: Rec = {}
      if ('x' in p || 'y' in p) {
        next.pos = xy({ x: p.x ?? r.pos.x, y: p.y ?? r.pos.y })
      }
      if ('kind' in p && p.kind) {
        next.kind = p.kind
      }
      if ('name' in p) {
        next.name = p.name ?? ''
      }
      if ('process_code' in p) {
        next.process_code = p.process_code ?? null
      }
      return setOps('map_points', r as unknown as Rec, next)
    }
    case 'deletePoint':
      return recs(st, 'map_points')[cmd.id] ? [{ op: 'delete', coll: 'map_points', id: cmd.id }] : []
    case 'addEdge': {
      const { from, to } = cmd.edge
      const points = recs(st, 'map_points')
      if (from === to || !points[from] || !points[to] || recs(st, 'map_edges')[cmd.edge.id] || sameEdge(st, from, to)) {
        return []
      }
      return [{ op: 'insert', coll: 'map_edges', id: cmd.edge.id, value: edgeValue(cmd.edge) }]
    }
    case 'updateEdge': {
      const r = recs<MapEdgesRecord>(st, 'map_edges')[cmd.id]
      if (!r) {
        return []
      }
      const next: Rec = {}
      if ('width_m' in cmd.patch) {
        next.width_m = cmd.patch.width_m ?? null
      }
      if ('bidirectional' in cmd.patch) {
        next.bidirectional = cmd.patch.bidirectional ?? null
      }
      return setOps('map_edges', r as unknown as Rec, next)
    }
    case 'deleteEdge':
      return recs(st, 'map_edges')[cmd.id] ? [{ op: 'delete', coll: 'map_edges', id: cmd.id }] : []
    case 'addPolygon': {
      const coll = polygonColl[cmd.layer]
      if (cmd.polygon.ring.length < 3 || recs(st, coll)[cmd.polygon.id]) {
        return []
      }
      const p = cmd.polygon
      return [
        {
          op: 'insert',
          coll,
          id: p.id,
          value: { order: keyBetween(lastOrder(st, coll), null), kind: p.kind, name: p.name ?? '', ring: ring(p.ring) },
        },
      ]
    }
    case 'updatePolygon': {
      const coll = polygonColl[cmd.layer]
      const r = recs<MapZonesRecord>(st, coll)[cmd.id]
      if (!r) {
        return []
      }
      const next: Rec = {}
      if (cmd.patch.kind) {
        next.kind = cmd.patch.kind
      }
      if ('name' in cmd.patch) {
        next.name = cmd.patch.name ?? ''
      }
      if (cmd.patch.ring && cmd.patch.ring.length >= 3) {
        next.ring = ring(cmd.patch.ring)
      }
      return setOps(coll, r as unknown as Rec, next)
    }
    case 'movePolygon': {
      const coll = polygonColl[cmd.layer]
      const r = recs<MapZonesRecord>(st, coll)[cmd.id]
      if (!r || (cmd.dx === 0 && cmd.dy === 0)) {
        return []
      }
      return setOps(coll, r as unknown as Rec, { ring: ring(r.ring.map((v) => ({ x: v.x + cmd.dx, y: v.y + cmd.dy }))) })
    }
    case 'deletePolygon': {
      const coll = polygonColl[cmd.layer]
      return recs(st, coll)[cmd.id] ? [{ op: 'delete', coll, id: cmd.id }] : []
    }
    case 'upsertResource': {
      const r = recs<MapResourcesRecord>(st, 'map_resources')[cmd.resource.id]
      const v = resourceValue(cmd.resource)
      if (!r) {
        return [{ op: 'insert', coll: 'map_resources', id: cmd.resource.id, value: v }]
      }
      return setOps('map_resources', r as unknown as Rec, v)
    }
    case 'deleteResource':
      return recs(st, 'map_resources')[cmd.id] ? [{ op: 'delete', coll: 'map_resources', id: cmd.id }] : []
    case 'setFlow': {
      const f = cmd.flow
      const r = Object.values(recs<MapFlowsRecord>(st, 'map_flows')).find((x) => x.process_code === f.process_code)
      const lists = { pickup_point_ids: f.pickup_point_ids, drop_point_ids: f.drop_point_ids }
      if (!r) {
        return [{ op: 'insert', coll: 'map_flows', id: uuid(), value: { process_code: f.process_code, ...lists } }]
      }
      return setOps('map_flows', r as unknown as Rec, lists)
    }
    case 'deleteFlow': {
      const r = Object.values(recs<MapFlowsRecord>(st, 'map_flows')).find((x) => x.process_code === cmd.processCode)
      return r ? [{ op: 'delete', coll: 'map_flows', id: r.id }] : []
    }
  }
}

// commandOps returns the operations for cmd, or none when it changes nothing. A project without a map gets one
// with fallbackPage in the same transaction.
export function commandOps(st: State, cmd: MapCommand, fallbackPage: MapPage): RawOp[] {
  const body = commandBody(st, cmd)
  if (body.length === 0) {
    return []
  }
  const m = recs<MapRecord>(st, 'map').map
  if (cmd.type === 'replace' || cmd.type === 'setPage' || !m || m.page !== null) {
    return body
  }
  return [...mapSet(st, { page: page(fallbackPage) }), ...body]
}

const labels: Record<MapCommand['type'], string> = {
  replace: 'Заменить карту шаблоном',
  setPage: 'Изменить размер плана',
  calibrate: 'Калибровка',
  setScale: 'Изменить масштаб',
  setCheck: 'Контрольное измерение',
  addPoint: 'Добавить точку',
  updatePoint: 'Изменить точку',
  deletePoint: 'Удалить точку',
  addEdge: 'Добавить ребро',
  updateEdge: 'Изменить ребро',
  deleteEdge: 'Удалить ребро',
  addPolygon: 'Добавить контур',
  updatePolygon: 'Изменить контур',
  movePolygon: 'Переместить контур',
  deletePolygon: 'Удалить контур',
  upsertResource: 'Изменить ресурс',
  deleteResource: 'Удалить ресурс',
  setFlow: 'Изменить поток',
  deleteFlow: 'Удалить поток',
}

// commandLabel names the action in the list of changes the server did not accept.
export function commandLabel(cmd: MapCommand): string {
  if (cmd.type === 'updatePoint' && ('x' in cmd.patch || 'y' in cmd.patch) && Object.keys(cmd.patch).every((k) => k === 'x' || k === 'y')) {
    return 'Переместить точку'
  }
  if (cmd.type === 'addPolygon') {
    return cmd.layer === 'zones' ? 'Добавить зону' : 'Добавить препятствие'
  }
  return labels[cmd.type]
}
