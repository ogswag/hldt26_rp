import type Konva from 'konva'
import type { KonvaEventObject } from 'konva/lib/Node'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Arrow, Circle, Group, Image as KImage, Layer, Line, Rect, Stage, Text } from 'react-konva'

import type { MapDocument, MapEdgeCheck, MapPointKind, MapSegment, MapXY } from '../api/client'
import { numberText } from '../ui/numberText'
import { presenceColor, useCanvasPalette, type CanvasPalette } from '../ui/theme'

import type { MapCommand, PolygonLayer } from './commands'
import { featureLabel, nextName, pointIndex, pointPrefix, rectRing, resourcePoints } from './document'
import type { Plan } from './planFile'
import { newFeatureId } from './records'

export type Tool = 'select' | 'pan' | 'point' | 'edge' | 'zone' | 'obstacle' | 'calibrate' | 'check'

export type Selection =
  | { type: 'point'; id: string }
  | { type: 'edge'; id: string }
  | { type: 'zones'; id: string }
  | { type: 'obstacles'; id: string }

export type ShapeMode = 'rect' | 'poly'

type Props = {
  doc: MapDocument
  plan: Plan | null
  tool: Tool
  pointKind: MapPointKind
  zoneKind: string
  obstacleKind: string
  shape: ShapeMode
  defaultWidthM: number
  selection: Selection | null
  edgeChecks: Map<string, MapEdgeCheck>
  issueRefs: Set<string>
  // watchers marks the objects other people have selected right now.
  watchers: Map<string, { initials: string; color: number }>
  flowFocus: { pickups: string[]; drops: string[] } | null
  // focus centres the view on a plan point; n changes with every request, so the same point can be asked twice.
  focus?: { x: number; y: number; n: number } | null
  height: number
  onSelect: (s: Selection | null) => void
  onCommand: (cmd: MapCommand, mergeKey?: string) => void
  onEndDrag: () => void
  onSegment: (kind: 'calibrate' | 'check', seg: Omit<MapSegment, 'length_m'>) => void
}

function pointColor(pal: CanvasPalette, kind: MapPointKind): string {
  return { task: pal.task, dock: pal.dock, charger: pal.charger, gate: pal.gate, other: pal.other }[kind]
}

function zoneFill(pal: CanvasPalette, kind: string): string {
  const fills: Record<string, string> = {
    workspace: 'transparent',
    storage: pal.zoneStorage,
    dock_area: pal.zoneDock,
    pick: pal.zonePick,
    charge: pal.zoneCharge,
  }
  return fills[kind] ?? pal.zone
}

const flat = (ring: MapXY[]) => ring.flatMap((p) => [p.x, p.y])

function polygonTopLeft(ring: MapXY[]): MapXY {
  return ring.reduce(
    (topLeft, point) => ({ x: Math.min(topLeft.x, point.x), y: Math.min(topLeft.y, point.y) }),
    { x: Number.POSITIVE_INFINITY, y: Number.POSITIVE_INFINITY },
  )
}

export function MapCanvas(p: Props) {
  const wrapRef = useRef<HTMLDivElement>(null)
  const stageRef = useRef<Konva.Stage>(null)
  const pal = useCanvasPalette()
  const [width, setWidth] = useState(0)
  const [view, setView] = useState({ scale: 1, x: 0, y: 0 })
  const [cursor, setCursor] = useState<MapXY | null>(null)
  const [draftState, setDraftState] = useState<{ tool: Tool; pts: MapXY[] }>({ tool: p.tool, pts: [] })
  const [edgeState, setEdgeState] = useState<{ tool: Tool; id: string | null }>({ tool: p.tool, id: null })
  const draft = draftState.tool === p.tool ? draftState.pts : []
  const edgeFrom = edgeState.tool === p.tool ? edgeState.id : null
  const setDraft = (pts: MapXY[]) => setDraftState({ tool: p.tool, pts })
  const setEdgeFrom = (id: string | null) => setEdgeState({ tool: p.tool, id })
  const [fitted, setFitted] = useState(false)

  const page = p.doc.page ?? { width_px: 1000, height_px: 700, source_kind: 'none' as const }
  const mpp = p.doc.calibration.meters_per_px
  const points = useMemo(() => pointIndex(p.doc), [p.doc])

  useEffect(() => {
    const el = wrapRef.current
    if (!el) {
      return
    }
    const apply = () => setWidth(Math.max(320, el.clientWidth))
    apply()
    const ro = new ResizeObserver(apply)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const fitView = (w: number) => {
    const scale = Math.min(w / page.width_px, p.height / page.height_px) * 0.95
    return { scale, x: (w - page.width_px * scale) / 2, y: (p.height - page.height_px * scale) / 2 }
  }
  if (!fitted && width > 0) {
    setFitted(true)
    setView(fitView(width))
  }
  const [focusSeen, setFocusSeen] = useState(0)
  if (p.focus && width > 0 && p.focus.n !== focusSeen) {
    const at = p.focus
    setFocusSeen(at.n)
    setView((v) => ({ scale: v.scale, x: width / 2 - at.x * v.scale, y: p.height / 2 - at.y * v.scale }))
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setDraftState((d) => ({ ...d, pts: [] }))
        setEdgeState((d) => ({ ...d, id: null }))
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const px = 1 / view.scale
  const watchColor = (id: string) => {
    const w = p.watchers.get(id)
    return w ? presenceColor(pal, w.color) : null
  }

  function worldPointer(): MapXY | null {
    const pos = stageRef.current?.getRelativePointerPosition()
    return pos ? { x: pos.x, y: pos.y } : null
  }

  function onWheel(e: KonvaEventObject<WheelEvent>) {
    e.evt.preventDefault()
    const stage = stageRef.current
    const pointer = stage?.getPointerPosition()
    if (!stage || !pointer) {
      return
    }
    const factor = e.evt.deltaY > 0 ? 1 / 1.15 : 1.15
    const scale = Math.min(40, Math.max(0.02, view.scale * factor))
    const wx = (pointer.x - view.x) / view.scale
    const wy = (pointer.y - view.y) / view.scale
    setView({ scale, x: pointer.x - wx * scale, y: pointer.y - wy * scale })
  }

  function addPointAt(at: MapXY, kind: MapPointKind): string {
    const id = newFeatureId()
    p.onCommand({ type: 'addPoint', point: { id, kind, name: nextName(p.doc, pointPrefix[kind]), x: at.x, y: at.y } })
    return id
  }

  function connect(to: string) {
    if (edgeFrom && edgeFrom !== to) {
      p.onCommand({
        type: 'addEdge',
        edge: { id: newFeatureId(), from: edgeFrom, to, width_m: p.defaultWidthM },
      })
    }
    setEdgeFrom(to)
  }

  function finishPolygon(ring: MapXY[]) {
    const layer: PolygonLayer = p.tool === 'zone' ? 'zones' : 'obstacles'
    const kind = p.tool === 'zone' ? p.zoneKind : p.obstacleKind
    if (ring.length >= 3) {
      const id = newFeatureId()
      p.onCommand({ type: 'addPolygon', layer, polygon: { id, kind, name: nextName(p.doc, layer === 'zones' ? 'z' : 'o'), ring } })
      p.onSelect({ type: layer, id })
    }
    setDraft([])
  }

  function onStageClick(e: KonvaEventObject<MouseEvent | TouchEvent>) {
    const onEmpty = e.target === e.target.getStage() || e.target.name() === 'background'
    const at = worldPointer()
    if (!at) {
      return
    }
    switch (p.tool) {
      case 'select':
        if (onEmpty) {
          p.onSelect(null)
        }
        return
      case 'point':
        if (onEmpty) {
          p.onSelect({ type: 'point', id: addPointAt(at, p.pointKind) })
        }
        return
      case 'edge':
        if (onEmpty) {
          connect(addPointAt(at, 'other'))
        }
        return
      case 'zone':
      case 'obstacle':
        if (p.shape === 'rect') {
          if (draft.length === 0) {
            setDraft([at])
          } else {
            finishPolygon(rectRing(draft[0].x, draft[0].y, at.x, at.y))
          }
          return
        }
        if (draft.length >= 3 && Math.hypot(at.x - draft[0].x, at.y - draft[0].y) < 8 * px) {
          finishPolygon(draft)
          return
        }
        setDraft([...draft, at])
        return
      case 'calibrate':
      case 'check':
        if (draft.length === 0) {
          setDraft([at])
        } else {
          p.onSegment(p.tool, { x1: draft[0].x, y1: draft[0].y, x2: at.x, y2: at.y })
          setDraft([])
        }
        return
      default:
        return
    }
  }

  function onPointClick(id: string, e: KonvaEventObject<MouseEvent | TouchEvent>) {
    e.cancelBubble = true
    if (p.tool === 'edge') {
      connect(id)
      return
    }
    if (p.tool === 'select' || p.tool === 'point') {
      p.onSelect({ type: 'point', id })
    }
  }

  const sel = p.selection
  const pickups = new Set(p.flowFocus?.pickups ?? [])
  const drops = new Set(p.flowFocus?.drops ?? [])
  const narrowEdges = new Map<string, string>()
  for (const r of p.doc.layers.resources) {
    if (r.kind === 'narrow_aisle') {
      for (const e of r.edge_ids ?? []) {
        narrowEdges.set(e, r.name || r.id)
      }
    }
  }
  const edgeFromPoint = edgeFrom ? points.get(edgeFrom) : undefined

  return (
    <div ref={wrapRef} className="map-canvas" data-tool={p.tool} data-search-id="map:canvas">
      {width > 0 ? (
      <Stage
        ref={stageRef}
        width={width}
        height={p.height}
        scaleX={view.scale}
        scaleY={view.scale}
        x={view.x}
        y={view.y}
        draggable={p.tool === 'pan'}
        onDragEnd={(e) => {
          if (e.target === stageRef.current) {
            setView((v) => ({ ...v, x: e.target.x(), y: e.target.y() }))
          }
        }}
        onWheel={onWheel}
        onClick={onStageClick}
        onTap={onStageClick}
        onDblClick={() => {
          if ((p.tool === 'zone' || p.tool === 'obstacle') && p.shape === 'poly' && draft.length >= 3) {
            finishPolygon(draft)
          }
        }}
        onMouseMove={() => setCursor(worldPointer())}
      >
        <Layer listening={true}>
          {p.plan ? (
            <KImage
              name="background"
              image={p.plan.canvas}
              width={page.width_px}
              height={page.height_px}
              opacity={0.9}
            />
          ) : (
            <Rect name="background" width={page.width_px} height={page.height_px} fill={pal.paper} stroke={pal.edge} strokeWidth={px} />
          )}
        </Layer>
        <Layer>
          {p.doc.layers.zones.map((z) => {
            const selected = sel?.type === 'zones' && sel.id === z.id
            const topLeft = polygonTopLeft(z.ring)
            return (
              <Group key={z.id}>
                <Line
                  points={flat(z.ring)}
                  closed
                  fill={zoneFill(pal, z.kind)}
                  stroke={watchColor(z.id) ?? (selected ? pal.select : z.kind === 'workspace' ? pal.ink : pal.line)}
                  strokeWidth={(watchColor(z.id) ? 3 : z.kind === 'workspace' ? 3 : 1.5) * px}
                  dash={z.kind === 'workspace' ? undefined : [6 * px, 4 * px]}
                  listening={p.tool === 'select'}
                  onClick={(e) => {
                    e.cancelBubble = true
                    p.onSelect({ type: 'zones', id: z.id })
                  }}
                />
                {z.kind !== 'workspace' ? (
                  <Text
                    fontFamily={pal.font}
                    x={topLeft.x + 4 * px}
                    y={topLeft.y + 4 * px}
                    text={z.name || featureLabel(z)}
                    fontSize={12 * px}
                    fill={pal.inkMuted}
                    listening={false}
                  />
                ) : null}
              </Group>
            )
          })}
          {p.doc.layers.obstacles.map((o) => {
            const selected = sel?.type === 'obstacles' && sel.id === o.id
            return (
              <Line
                key={o.id}
                points={flat(o.ring)}
                closed
                fill={p.issueRefs.has(o.id) ? pal.issue : pal.obstacle}
                stroke={watchColor(o.id) ?? (selected ? pal.select : pal.obstacleEdge)}
                strokeWidth={(selected || watchColor(o.id) ? 3 : 1) * px}
                draggable={p.tool === 'select' && selected}
                listening={p.tool === 'select'}
                onClick={(e) => {
                  e.cancelBubble = true
                  p.onSelect({ type: 'obstacles', id: o.id })
                }}
                onDragEnd={(e) => {
                  const dx = e.target.x()
                  const dy = e.target.y()
                  e.target.position({ x: 0, y: 0 })
                  p.onCommand({ type: 'movePolygon', layer: 'obstacles', id: o.id, dx, dy })
                }}
              />
            )
          })}
          {sel && (sel.type === 'zones' || sel.type === 'obstacles')
            ? p.doc.layers[sel.type]
                .find((poly) => poly.id === sel.id)
                ?.ring.map((v, i) => (
                  <Rect
                    key={`v${i}`}
                    x={v.x - 4 * px}
                    y={v.y - 4 * px}
                    width={8 * px}
                    height={8 * px}
                    fill={pal.halo}
                    stroke={pal.select}
                    strokeWidth={px}
                    draggable={p.tool === 'select'}
                    onDragMove={(e) => {
                      const poly = p.doc.layers[sel.type].find((x) => x.id === sel.id)
                      if (!poly) {
                        return
                      }
                      const ring = poly.ring.map((pt, k) =>
                        k === i ? { x: e.target.x() + 4 * px, y: e.target.y() + 4 * px } : pt,
                      )
                      p.onCommand({ type: 'updatePolygon', layer: sel.type, id: sel.id, patch: { ring } }, `vertex:${sel.id}:${i}`)
                    }}
                    onDragEnd={p.onEndDrag}
                  />
                ))
            : null}
        </Layer>
        <Layer>
          {p.doc.layers.edges.map((e) => {
            const a = points.get(e.from)
            const b = points.get(e.to)
            if (!a || !b) {
              return null
            }
            const check = p.edgeChecks.get(e.id)
            const w = check?.width_m ?? e.width_m ?? p.defaultWidthM
            const blocked = Boolean(check?.blocked_for?.length)
            const color = blocked ? pal.danger : check?.single_lane ? pal.warning : pal.path
            const selected = sel?.type === 'edge' && sel.id === e.id
            const pts = [a.x, a.y, b.x, b.y]
            return (
              <Group
                key={e.id}
                onClick={(ev) => {
                  ev.cancelBubble = true
                  if (p.tool === 'select') {
                    p.onSelect({ type: 'edge', id: e.id })
                  }
                }}
              >
                <Line points={pts} stroke={color} opacity={0.18} strokeWidth={mpp > 0 ? w / mpp : 4 * px} lineCap="round" />
                {e.bidirectional === false ? (
                  <Arrow
                    points={pts}
                    stroke={watchColor(e.id) ?? (selected ? pal.select : color)}
                    fill={watchColor(e.id) ?? (selected ? pal.select : color)}
                    strokeWidth={(selected || watchColor(e.id) ? 4 : 2) * px}
                    pointerLength={10 * px}
                    pointerWidth={8 * px}
                    hitStrokeWidth={12 * px}
                  />
                ) : (
                  <Line
                    points={pts}
                    stroke={watchColor(e.id) ?? (selected ? pal.select : color)}
                    strokeWidth={(selected || watchColor(e.id) ? 4 : 2) * px}
                    dash={narrowEdges.has(e.id) ? [8 * px, 5 * px] : undefined}
                    hitStrokeWidth={12 * px}
                  />
                )}
                {narrowEdges.has(e.id) ? (
                  <Text
                    fontFamily={pal.font}
                    x={(a.x + b.x) / 2}
                    y={(a.y + b.y) / 2 + 4 * px}
                    text={narrowEdges.get(e.id)}
                    fontSize={11 * px}
                    fill={pal.dock}
                    listening={false}
                  />
                ) : null}
              </Group>
            )
          })}
          {p.doc.layers.resources
            .filter((r) => r.kind !== 'narrow_aisle')
            .flatMap((r) =>
              resourcePoints(r).map((id) => {
                const pt = points.get(id)
                if (!pt) {
                  return null
                }
                return (
                  <Circle
                    key={`${r.id}:${id}`}
                    x={pt.x}
                    y={pt.y}
                    radius={12 * px}
                    stroke={r.kind === 'dock' ? pal.dock : pal.charger}
                    strokeWidth={1.5 * px}
                    dash={[3 * px, 3 * px]}
                    listening={false}
                  />
                )
              }),
            )}
          {p.doc.layers.points.map((pt) => {
            const selected = sel?.type === 'point' && sel.id === pt.id
            const focus = pickups.has(pt.id) ? pal.charger : drops.has(pt.id) ? pal.drop : null
            return (
              <Group key={pt.id}>
                {focus ? <Circle x={pt.x} y={pt.y} radius={16 * px} fill={focus} opacity={0.25} listening={false} /> : null}
                {watchColor(pt.id) ? (
                  <>
                    <Circle
                      x={pt.x}
                      y={pt.y}
                      radius={13 * px}
                      stroke={watchColor(pt.id) as string}
                      strokeWidth={2 * px}
                      listening={false}
                    />
                    <Text
                      fontFamily={pal.font}
                      x={pt.x + 12 * px}
                      y={pt.y + 6 * px}
                      text={p.watchers.get(pt.id)?.initials ?? ''}
                      fontSize={10 * px}
                      fontStyle="bold"
                      fill={watchColor(pt.id) as string}
                      listening={false}
                    />
                  </>
                ) : null}
                <Circle
                  x={pt.x}
                  y={pt.y}
                  radius={(pt.kind === 'other' ? 4 : 7) * px}
                  fill={pointColor(pal, pt.kind)}
                  stroke={selected ? pal.select : p.issueRefs.has(pt.id) ? pal.danger : pal.halo}
                  strokeWidth={(selected || p.issueRefs.has(pt.id) ? 3 : 1.5) * px}
                  hitStrokeWidth={10 * px}
                  draggable={p.tool === 'select'}
                  onClick={(e) => onPointClick(pt.id, e)}
                  onTap={(e) => onPointClick(pt.id, e)}
                  onDragStart={() => p.onSelect({ type: 'point', id: pt.id })}
                  onDragMove={(e) =>
                    p.onCommand({ type: 'updatePoint', id: pt.id, patch: { x: e.target.x(), y: e.target.y() } }, `drag:${pt.id}`)
                  }
                  onDragEnd={p.onEndDrag}
                />
                {pt.kind !== 'other' || selected ? (
                  <Text
                    fontFamily={pal.font}
                    x={pt.x - (pt.kind === 'other' ? 4 : 7) * px}
                    y={pt.y - 20 * px}
                    text={featureLabel(pt)}
                    fontSize={11 * px}
                    fill={pal.ink}
                    listening={false}
                  />
                ) : null}
              </Group>
            )
          })}
        </Layer>
        <Layer listening={false}>
          {(['segment', 'check'] as const).map((k) => {
            const s = p.doc.calibration[k]
            if (!s) {
              return null
            }
            const color = k === 'segment' ? pal.segment : pal.drop
            return (
              <Group key={k}>
                <Line points={[s.x1, s.y1, s.x2, s.y2]} stroke={color} strokeWidth={2 * px} dash={[4 * px, 3 * px]} />
                <Circle x={s.x1} y={s.y1} radius={4 * px} fill={color} />
                <Circle x={s.x2} y={s.y2} radius={4 * px} fill={color} />
                <Text
                  fontFamily={pal.font}
                  x={(s.x1 + s.x2) / 2}
                  y={(s.y1 + s.y2) / 2 - 16 * px}
                  text={`${k === 'segment' ? 'масштаб' : 'контроль'} ${s.length_m} м`}
                  fontSize={12 * px}
                  fill={color}
                />
              </Group>
            )
          })}
          {edgeFromPoint && cursor ? (
            <Line points={[edgeFromPoint.x, edgeFromPoint.y, cursor.x, cursor.y]} stroke={pal.path} strokeWidth={1.5 * px} dash={[4 * px, 4 * px]} />
          ) : null}
          {draft.length > 0 && cursor ? (
            p.tool === 'calibrate' || p.tool === 'check' ? (
              <Line points={[draft[0].x, draft[0].y, cursor.x, cursor.y]} stroke={pal.segment} strokeWidth={2 * px} />
            ) : p.shape === 'rect' ? (
              <Line points={flat(rectRing(draft[0].x, draft[0].y, cursor.x, cursor.y))} closed stroke={pal.select} strokeWidth={1.5 * px} />
            ) : (
              <Line points={flat([...draft, cursor])} stroke={pal.select} strokeWidth={1.5 * px} />
            )
          ) : null}
        </Layer>
      </Stage>
      ) : null}
      <div className="map-canvas-status">
        <span>Масштаб просмотра {Math.round(view.scale * 100)}%</span>
        {cursor ? (
          <span>
            x {numberText(cursor.x * mpp, 2)} м, y {numberText(cursor.y * mpp, 2)} м
          </span>
        ) : null}
        {edgeFrom ? <span>Ребро от {edgeFrom}. Esc: закончить.</span> : null}
        {draft.length > 0 && (p.tool === 'zone' || p.tool === 'obstacle') && p.shape === 'poly' ? (
          <span>Вершин {draft.length}. Двойной щелчок или щелчок по первой вершине замыкает контур.</span>
        ) : null}
        <button
          type="button"
          className="linkish"
          onClick={() => setView(fitView(width))}
        >
          Показать весь план
        </button>
      </div>
    </div>
  )
}
