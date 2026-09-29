import type { MapDocument, MapXY } from '../api/client'
import { Reflow } from '../ui/Reflow'
import { useCanvasPalette } from '../ui/theme'
import type { SearchEntry } from './entries'

const around = 2

// Neighbours are the rows the page shows next to the entry, from the same section.
function neighbours(entry: SearchEntry, index: readonly SearchEntry[]): SearchEntry[] {
  if (!entry.section) {
    return [entry]
  }
  const rows = index.filter((e) => e.section === entry.section)
  const i = rows.findIndex((e) => e.id === entry.id)
  return i < 0 ? [entry] : rows.slice(Math.max(0, i - around), i + around + 1)
}

type Box = { x0: number; y0: number; x1: number; y1: number }

function boxOf(pts: readonly MapXY[]): Box {
  const xs = pts.map((p) => p.x)
  const ys = pts.map((p) => p.y)
  return { x0: Math.min(...xs), y0: Math.min(...ys), x1: Math.max(...xs), y1: Math.max(...ys) }
}

// MapCrop draws the part of the plan around a map object: zones, obstacles and points, the object outlined.
function MapCrop({ doc, id }: { doc: MapDocument; id: string }) {
  const pal = useCanvasPalette()
  const ref = id.replace(/^map:/, '')
  const flow = ref.startsWith('flow:') ? (doc.layers.flows ?? []).find((f) => `flow:${f.process_code}` === ref) : undefined
  const flowPoints = new Set(flow ? [...flow.pickup_point_ids, ...flow.drop_point_ids] : [])
  const poly = [...doc.layers.zones, ...doc.layers.obstacles].find((z) => z.id === ref)
  const point = doc.layers.points.find((p) => p.id === ref)
  const own: MapXY[] = poly ? poly.ring : point ? [point] : doc.layers.points.filter((p) => flowPoints.has(p.id))
  if (own.length === 0) {
    return null
  }
  const b = boxOf(own)
  const pad = Math.max(40, (b.x1 - b.x0) * 0.6, (b.y1 - b.y0) * 0.6)
  const view = { x: b.x0 - pad, y: b.y0 - pad, w: b.x1 - b.x0 + 2 * pad, h: b.y1 - b.y0 + 2 * pad }
  const r = Math.max(view.w, view.h) / 60
  const ring = (pts: MapXY[]) => pts.map((p) => `${p.x},${p.y}`).join(' ')
  return (
    <svg className="search-map" viewBox={`${view.x} ${view.y} ${view.w} ${view.h}`} preserveAspectRatio="xMidYMid meet" aria-hidden="true">
      <rect x={view.x} y={view.y} width={view.w} height={view.h} fill={pal.paper} />
      {doc.layers.zones.map((z) => (
        <polygon key={z.id} points={ring(z.ring)} fill={pal.zone} stroke={z.id === ref ? pal.select : pal.line} strokeWidth={z.id === ref ? r / 2 : r / 6} />
      ))}
      {doc.layers.obstacles.map((o) => (
        <polygon key={o.id} points={ring(o.ring)} fill={pal.obstacle} stroke={o.id === ref ? pal.select : pal.obstacleEdge} strokeWidth={o.id === ref ? r / 2 : r / 6} />
      ))}
      {doc.layers.points.map((p) => {
        const hit = p.id === ref || flowPoints.has(p.id)
        return <circle key={p.id} cx={p.x} cy={p.y} r={hit ? r * 1.4 : r} fill={hit ? pal.select : pal.inkMuted} />
      })}
    </svg>
  )
}

// Preview shows where an entry sits before the user goes there: its path, its section heading, and the rows
// around it with their values, the entry itself marked.
export function Preview({ entry, index, mapDoc }: { entry: SearchEntry; index: readonly SearchEntry[]; mapDoc: MapDocument | null }) {
  const heading = entry.path.at(-1)
  const trail = entry.path.slice(0, -1).join(' / ')
  if (entry.closed || entry.hint || entry.kind === 'tab' || entry.kind === 'section') {
    return (
      <>
        {entry.path.length > 0 ? <p className="search-preview-path">{entry.path.join(' / ')}</p> : null}
        <p className="search-preview-title">{entry.label}</p>
        {entry.closed ? (
          <p>
            <Reflow>{`Вкладка закрыта. ${entry.closed}`}</Reflow>
          </p>
        ) : entry.hint ? (
          <p>
            <Reflow>{entry.hint}</Reflow>
          </p>
        ) : null}
      </>
    )
  }
  return (
    <>
      {trail ? <p className="search-preview-path">{trail}</p> : null}
      {heading ? <p className="search-preview-heading">{heading}</p> : null}
      {entry.kind === 'map' && mapDoc ? <MapCrop doc={mapDoc} id={entry.id} /> : null}
      <dl className="search-preview-rows">
        {neighbours(entry, index).map((e) => (
          <div key={e.id} className={e.id === entry.id ? 'search-preview-row is-target' : 'search-preview-row'}>
            <dt>{e.label}</dt>
            <dd>{e.value ?? e.note ?? ''}</dd>
          </div>
        ))}
      </dl>
    </>
  )
}
