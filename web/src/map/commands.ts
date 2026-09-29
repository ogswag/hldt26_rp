import type { MapDocument, MapEdge, MapFlow, MapPoint, MapPolygon, MapResource, MapSegment, MapSourceKind } from '../api/client'

export type PolygonLayer = 'zones' | 'obstacles'

// MapCommand is one editor action. records.ts turns it into operations on the project store.
export type MapCommand =
  | { type: 'replace'; doc: MapDocument }
  | { type: 'setPage'; page: { width_px: number; height_px: number; source_kind: MapSourceKind } }
  | { type: 'calibrate'; segment: MapSegment }
  | { type: 'setScale'; metersPerPx: number }
  | { type: 'setCheck'; segment: MapSegment | null }
  | { type: 'addPoint'; point: MapPoint }
  | { type: 'updatePoint'; id: string; patch: Partial<Omit<MapPoint, 'id'>> }
  | { type: 'deletePoint'; id: string }
  | { type: 'addEdge'; edge: MapEdge }
  | { type: 'updateEdge'; id: string; patch: Partial<Omit<MapEdge, 'id' | 'from' | 'to'>> }
  | { type: 'deleteEdge'; id: string }
  | { type: 'addPolygon'; layer: PolygonLayer; polygon: MapPolygon }
  | { type: 'updatePolygon'; layer: PolygonLayer; id: string; patch: Partial<Omit<MapPolygon, 'id'>> }
  | { type: 'movePolygon'; layer: PolygonLayer; id: string; dx: number; dy: number }
  | { type: 'deletePolygon'; layer: PolygonLayer; id: string }
  | { type: 'upsertResource'; resource: MapResource }
  | { type: 'deleteResource'; id: string }
  | { type: 'setFlow'; flow: MapFlow }
  | { type: 'deleteFlow'; processCode: string }

// preview draws a drag in progress: the dragged point or polygon moves locally, and the store gets one
// transaction when the mouse is released. Other commands leave the document as it is.
export function preview(doc: MapDocument, cmd: MapCommand): MapDocument {
  const l = doc.layers
  if (cmd.type === 'updatePoint') {
    return { ...doc, layers: { ...l, points: l.points.map((p) => (p.id === cmd.id ? { ...p, ...cmd.patch } : p)) } }
  }
  if (cmd.type === 'updatePolygon') {
    return { ...doc, layers: { ...l, [cmd.layer]: l[cmd.layer].map((p) => (p.id === cmd.id ? { ...p, ...cmd.patch } : p)) } }
  }
  return doc
}
