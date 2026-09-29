import type {
  MapDocument,
  MapEdge,
  MapPoint,
  MapPointKind,
  MapPolygon,
  MapResource,
  MapResourceKind,
  MapSegment,
  MapSourceKind,
  MapXY,
} from '../api/client'

export type Option<T extends string> = { value: T; label: string }

export const pointKinds: Option<MapPointKind>[] = [
  { value: 'task', label: 'Точка задания' },
  { value: 'dock', label: 'Док' },
  { value: 'charger', label: 'Зарядка' },
  { value: 'gate', label: 'Проход' },
  { value: 'other', label: 'Узел графа' },
]

// pointPrefix starts the code of a new point of each kind ("D2" for a dock).
export const pointPrefix: Record<MapPointKind, string> = {
  task: 'T',
  dock: 'D',
  charger: 'C',
  gate: 'G',
  other: 'N',
}

export const zoneKinds: Option<string>[] = [
  { value: 'workspace', label: 'Рабочая область (стены)' },
  { value: 'storage', label: 'Хранение' },
  { value: 'dock_area', label: 'Доки' },
  { value: 'pick', label: 'Отбор' },
  { value: 'charge', label: 'Зарядка' },
  { value: 'other', label: 'Другое' },
]

export const obstacleKinds: Option<string>[] = [
  { value: 'rack', label: 'Стеллаж' },
  { value: 'wall', label: 'Стена' },
  { value: 'column', label: 'Колонна' },
  { value: 'other', label: 'Другое' },
]

export const resourceKinds: Option<MapResourceKind>[] = [
  { value: 'dock', label: 'Док' },
  { value: 'narrow_aisle', label: 'Узкий участок' },
  { value: 'charger', label: 'Зарядная станция' },
]

export function labelOf<T extends string>(options: Option<T>[], value: string): string {
  return options.find((o) => o.value === value)?.label ?? value
}

export function emptyMap(page?: { width_px: number; height_px: number; source_kind: MapSourceKind }): MapDocument {
  return {
    schema_version: 'map-v1',
    profile: 'indoor',
    units: 'm',
    page: page ?? { width_px: 1000, height_px: 700, source_kind: 'none' },
    calibration: { meters_per_px: 0.1 },
    layers: { zones: [], obstacles: [], points: [], resources: [], edges: [], flows: [] },
  }
}

export function segmentPx(s: Pick<MapSegment, 'x1' | 'y1' | 'x2' | 'y2'>): number {
  return Math.hypot(s.x2 - s.x1, s.y2 - s.y1)
}

// metersPerPx derives the scale from a reference segment, or null when the segment is unusable.
export function metersPerPx(s: MapSegment): number | null {
  const px = segmentPx(s)
  if (px < 1 || !(s.length_m > 0)) {
    return null
  }
  return s.length_m / px
}

export function measuredLengthM(s: MapSegment, mpp: number): number {
  return segmentPx(s) * mpp
}

// checkDeviation returns the relative error of the control measurement in percent.
export function checkDeviation(s: MapSegment | undefined, mpp: number): number | null {
  if (!s || !(s.length_m > 0) || segmentPx(s) < 1) {
    return null
  }
  return (Math.abs(measuredLengthM(s, mpp) - s.length_m) / s.length_m) * 100
}

export function toMeters(p: MapXY, mpp: number): MapXY {
  return { x: p.x * mpp, y: p.y * mpp }
}

// A feature code is what the canvas prints next to a point (D1, B2). Other ids, UUIDs and the slugs of older
// maps such as "zone-work", are internal: a feature without a code goes by its name.
const featureCode = /^[A-Z0-9][A-Z0-9-]*$/

export function featureLabel(f: { id: string; name?: string }): string {
  return featureCode.test(f.id) ? f.id : f.name || 'без имени'
}

// pointOption labels a point in lists: the code, and the name when it differs.
export function pointOption(p: MapPoint): string {
  const label = featureLabel(p)
  return p.name && p.name !== label ? `${label} ${p.name}` : label
}

// edgeLabel names an edge by its ends.
export function edgeLabel(e: MapEdge, points: Map<string, MapPoint>): string {
  const end = (id: string) => {
    const p = points.get(id)
    return p ? featureLabel(p) : 'без имени'
  }
  return `${end(e.from)} - ${end(e.to)}`
}

// nextName returns the first free code with the prefix ("T4"), numbered as feature ids were before operations.
export function nextName(doc: MapDocument, prefix: string): string {
  const taken = new Set<string>()
  const l = doc.layers
  for (const list of [l.zones, l.obstacles, l.points, l.resources, l.edges]) {
    for (const f of list) {
      taken.add(f.id)
      if ('name' in f && f.name) {
        taken.add(f.name)
      }
    }
  }
  let n = 1
  while (taken.has(`${prefix}${n}`)) {
    n++
  }
  return `${prefix}${n}`
}

// byLabel sorts features for tables: new ids are random, so the order follows the codes ("T2" before "T10").
export function byLabel<T extends { id: string; name?: string }>(list: T[], label: (f: T) => string = featureLabel): T[] {
  return [...list].sort((a, b) => label(a).localeCompare(label(b), 'ru', { numeric: true }))
}

// featureCenter is the middle of a point, zone, obstacle or flow on the plan, in plan pixels.
export function featureCenter(doc: MapDocument, ref: string): MapXY | null {
  const l = doc.layers
  const flow = ref.startsWith('flow:') ? (l.flows ?? []).find((f) => `flow:${f.process_code}` === ref) : undefined
  const ids = new Set(flow ? [...flow.pickup_point_ids, ...flow.drop_point_ids] : [])
  const pts: MapXY[] =
    [...l.zones, ...l.obstacles].find((z) => z.id === ref)?.ring ??
    l.points.filter((p) => p.id === ref || ids.has(p.id)).map((p) => ({ x: p.x, y: p.y }))
  if (pts.length === 0) {
    return null
  }
  const xs = pts.map((p) => p.x)
  const ys = pts.map((p) => p.y)
  return { x: (Math.min(...xs) + Math.max(...xs)) / 2, y: (Math.min(...ys) + Math.max(...ys)) / 2 }
}

export function pointIndex(doc: MapDocument): Map<string, MapPoint> {
  return new Map(doc.layers.points.map((p) => [p.id, p]))
}

export function edgeLengthM(doc: MapDocument, e: MapEdge, points = pointIndex(doc)): number {
  const a = points.get(e.from)
  const b = points.get(e.to)
  if (!a || !b) {
    return 0
  }
  return Math.hypot(b.x - a.x, b.y - a.y) * doc.calibration.meters_per_px
}

export function resourcePoints(r: MapResource): string[] {
  const out = r.point_id ? [r.point_id] : []
  for (const id of r.point_ids ?? []) {
    if (id && !out.includes(id)) {
      out.push(id)
    }
  }
  return out
}

export function polygonCenter(p: MapPolygon): MapXY {
  let x = 0
  let y = 0
  for (const v of p.ring) {
    x += v.x
    y += v.y
  }
  const n = Math.max(1, p.ring.length)
  return { x: x / n, y: y / n }
}

export function rectRing(x1: number, y1: number, x2: number, y2: number): MapXY[] {
  const lx = Math.min(x1, x2)
  const hx = Math.max(x1, x2)
  const ly = Math.min(y1, y2)
  const hy = Math.max(y1, y2)
  return [
    { x: lx, y: ly },
    { x: hx, y: ly },
    { x: hx, y: hy },
    { x: lx, y: hy },
  ]
}

export function distToSegment(p: MapXY, a: MapXY, b: MapXY): number {
  const dx = b.x - a.x
  const dy = b.y - a.y
  const len2 = dx * dx + dy * dy
  if (len2 === 0) {
    return Math.hypot(p.x - a.x, p.y - a.y)
  }
  const u = Math.max(0, Math.min(1, ((p.x - a.x) * dx + (p.y - a.y) * dy) / len2))
  return Math.hypot(p.x - (a.x + u * dx), p.y - (a.y + u * dy))
}

const round = (v: number) => Math.round(v * 100) / 100

// forSave drops server-written fields and rounds coordinates so equal maps hash equally.
export function forSave(doc: MapDocument): MapDocument {
  const ring = (r: MapXY[]) => r.map((v) => ({ x: round(v.x), y: round(v.y) }))
  const seg = (s?: MapSegment) =>
    s ? { x1: round(s.x1), y1: round(s.y1), x2: round(s.x2), y2: round(s.y2), length_m: s.length_m } : undefined
  const out: MapDocument = {
    schema_version: 'map-v1',
    profile: doc.profile,
    units: 'm',
    page: doc.page,
    calibration: {
      meters_per_px: doc.calibration.meters_per_px,
      ...(doc.calibration.segment ? { segment: seg(doc.calibration.segment) } : {}),
      ...(doc.calibration.check ? { check: seg(doc.calibration.check) } : {}),
    },
    layers: {
      zones: doc.layers.zones.map((z) => ({ ...z, ring: ring(z.ring) })),
      obstacles: doc.layers.obstacles.map((o) => ({ ...o, ring: ring(o.ring) })),
      points: doc.layers.points.map((p) => ({ ...p, x: round(p.x), y: round(p.y) })),
      resources: doc.layers.resources,
      edges: doc.layers.edges,
      flows: doc.layers.flows ?? [],
    },
  }
  return out
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

// asMapDocument accepts a loaded JSON value when it has the map-v1 shape.
export function asMapDocument(v: unknown): MapDocument | null {
  if (!isObject(v) || v.schema_version !== 'map-v1' || !isObject(v.layers) || !isObject(v.calibration)) {
    return null
  }
  const l = v.layers
  const lists = ['zones', 'obstacles', 'points', 'resources', 'edges'] as const
  if (!lists.every((k) => Array.isArray(l[k]))) {
    return null
  }
  if (typeof v.calibration.meters_per_px !== 'number') {
    return null
  }
  const doc = v as unknown as MapDocument
  return { ...doc, layers: { ...doc.layers, flows: doc.layers.flows ?? [] } }
}

export type MapCounts = {
  points: number
  edges: number
  zones: number
  obstacles: number
  resources: number
  flows: number
}

export function counts(doc: MapDocument): MapCounts {
  const l = doc.layers
  return {
    points: l.points.length,
    edges: l.edges.length,
    zones: l.zones.length,
    obstacles: l.obstacles.length,
    resources: l.resources.length,
    flows: (l.flows ?? []).length,
  }
}

// hasFlow says whether the process has a flow with at least one pickup and one drop point.
export function hasFlow(doc: MapDocument, processCode: string): boolean {
  const f = (doc.layers.flows ?? []).find((x) => x.process_code === processCode)
  return Boolean(f && f.pickup_point_ids.length > 0 && f.drop_point_ids.length > 0)
}
