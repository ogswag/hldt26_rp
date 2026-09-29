import { useState } from 'react'

import type {
  MapClassCheck,
  MapDocument,
  MapEdgeCheck,
  MapIssue,
  MapPoint,
  MapPointKind,
  MapResource,
  MapResourceKind,
  MapSegment,
  Process,
} from '../api/client'
import { CheckSelect } from '../ui/CheckSelect'
import { HelpButton } from '../ui/HelpButton'
import { NumberField } from '../ui/NumberField'
import { numberText, parseNumberText } from '../ui/numberText'
import { Reflow } from '../ui/Reflow'
import { Select } from '../ui/Select'
import { TextField } from '../ui/TextField'
import { useCanvasPalette } from '../ui/theme'
import { UnitField } from '../ui/UnitField'

import type { MapCommand, PolygonLayer } from './commands'
import type { Selection } from './MapCanvas'
import {
  byLabel,
  checkDeviation,
  edgeLabel,
  edgeLengthM,
  featureLabel,
  hasFlow,
  labelOf,
  measuredLengthM,
  nextName,
  obstacleKinds,
  pointIndex,
  pointKinds,
  pointOption,
  pointPrefix,
  resourceKinds,
  resourcePoints,
  segmentPx,
  zoneKinds,
} from './document'
import { newFeatureId } from './records'

type Dispatch = (cmd: MapCommand, mergeKey?: string) => void

function num(v: string): number | null {
  const n = parseNumberText(v)
  return n !== undefined && Number.isFinite(n) ? n : null
}

function NumberInput({
  value,
  onCommit,
  min,
  max,
  integer,
  label,
  unit,
}: {
  value: number
  onCommit: (v: number) => void
  step?: number
  min?: number
  max?: number
  integer?: boolean
  label: string
  unit?: string
}) {
  const input = (
    <NumberField
      aria-label={label}
      value={Math.round(value * 1000) / 1000}
      valid={(n) => (min === undefined || n >= min) && (max === undefined || n <= max) && (!integer || Number.isInteger(n))}
      onCommit={onCommit}
    />
  )
  return unit ? <UnitField unit={unit}>{input}</UnitField> : input
}

export function SelectionPanel({
  doc,
  selection,
  edgeChecks,
  onCommand,
  onSelect,
}: {
  doc: MapDocument
  selection: Selection | null
  edgeChecks: Map<string, MapEdgeCheck>
  onCommand: Dispatch
  onSelect: (s: Selection | null) => void
}) {
  const mpp = doc.calibration.meters_per_px
  if (!selection) {
    return null
  }
  if (selection.type === 'point') {
    const pt = doc.layers.points.find((p) => p.id === selection.id)
    if (!pt) {
      return null
    }
    const edges = doc.layers.edges.filter((e) => e.from === pt.id || e.to === pt.id)
    return (
      <div className="map-props is-quiet" data-search-id="map:selection">
        <h3>Точка {featureLabel(pt)}</h3>
        <label>
          Тип
          <Select
            value={pt.kind}
            options={pointKinds}
            onChange={(kind) => onCommand({ type: 'updatePoint', id: pt.id, patch: { kind } })}
          />
        </label>
        <label>
          Название
          <TextField maxLength={120} value={pt.name ?? ''} onCommit={(name) => onCommand({ type: 'updatePoint', id: pt.id, patch: { name: name || undefined } })} />
        </label>
        <label>
          X
          <NumberInput label="X, м" unit="м" value={pt.x * mpp} onCommit={(v) => onCommand({ type: 'updatePoint', id: pt.id, patch: { x: v / mpp } })} />
        </label>
        <label>
          Y
          <NumberInput label="Y, м" unit="м" value={pt.y * mpp} onCommit={(v) => onCommand({ type: 'updatePoint', id: pt.id, patch: { y: v / mpp } })} />
        </label>
        <p className="field-hint">
          <Reflow>Рёбер: {edges.length}. Стрелки сдвигают точку на 1 пиксель, с Shift на 10.</Reflow>
        </p>
        <button
          type="button"
          onClick={() => {
            onCommand({ type: 'deletePoint', id: pt.id })
            onSelect(null)
          }}
        >
          Удалить точку и её рёбра
        </button>
      </div>
    )
  }
  if (selection.type === 'edge') {
    const e = doc.layers.edges.find((x) => x.id === selection.id)
    if (!e) {
      return null
    }
    const check = edgeChecks.get(e.id)
    return (
      <div className="map-props is-quiet" data-search-id="map:selection">
        <h3>Ребро {edgeLabel(e, pointIndex(doc))}</h3>
        <p>
          <Reflow>Длина {numberText(edgeLengthM(doc, e), 2)} м.</Reflow>
        </p>
        <label>
          Заявленная ширина
          <NumberInput
            label="Ширина, м"
            unit="м"
            min={0.1}
            value={e.width_m ?? 0}
            onCommit={(v) => onCommand({ type: 'updateEdge', id: e.id, patch: { width_m: v > 0 ? v : undefined } })}
          />
        </label>
        <label className="check-row">
          <input
            type="checkbox"
            checked={e.bidirectional !== false}
            onChange={(ev) => onCommand({ type: 'updateEdge', id: e.id, patch: { bidirectional: ev.target.checked ? undefined : false } })}
          />
          <Reflow>Движение в обе стороны</Reflow>
        </label>
        {check ? (
          <p className={check.blocked_for?.length ? 'error' : undefined}>
            <Reflow>
              Ширина для маршрутов {numberText(check.width_m, 2)} м
              {check.measured_width_m !== undefined ? `, свободно по плану ${numberText(check.measured_width_m, 2)} м` : ''}.
              {check.single_lane ? ' Однополосный участок.' : ''}
              {check.blocked_for?.length ? ` Непроходимо: ${check.blocked_for.join(', ')}.` : ''}
            </Reflow>
          </p>
        ) : null}
        <button
          type="button"
          onClick={() => {
            onCommand({ type: 'deleteEdge', id: e.id })
            onSelect(null)
          }}
        >
          Удалить ребро
        </button>
      </div>
    )
  }
  const layer: PolygonLayer = selection.type
  const poly = doc.layers[layer].find((x) => x.id === selection.id)
  if (!poly) {
    return null
  }
  const kinds = layer === 'zones' ? zoneKinds : obstacleKinds
  return (
    <div className="map-props is-quiet" data-search-id="map:selection">
      <h3>
        {layer === 'zones' ? 'Зона' : 'Препятствие'} {featureLabel(poly)}
      </h3>
      <label>
        Тип
        <Select value={poly.kind} options={kinds} onChange={(kind) => onCommand({ type: 'updatePolygon', layer, id: poly.id, patch: { kind } })} />
      </label>
      <label>
        Название
        <TextField maxLength={120} value={poly.name ?? ''} onCommit={(name) => onCommand({ type: 'updatePolygon', layer, id: poly.id, patch: { name: name || undefined } })} />
      </label>
      <label>
        Вершины, м (x y через запятую)
        <RingInput
          ring={poly.ring.map((v) => ({ x: v.x * mpp, y: v.y * mpp }))}
          onCommit={(ring) =>
            onCommand({ type: 'updatePolygon', layer, id: poly.id, patch: { ring: ring.map((v) => ({ x: v.x / mpp, y: v.y / mpp })) } })
          }
        />
      </label>
      <button
        type="button"
        onClick={() => {
          onCommand({ type: 'deletePolygon', layer, id: poly.id })
          onSelect(null)
        }}
      >
        Удалить
      </button>
    </div>
  )
}

function RingInput({ ring, onCommit }: { ring: { x: number; y: number }[]; onCommit: (r: { x: number; y: number }[]) => void }) {
  const [text, setText] = useState<string | null>(null)
  // NOTE: a vertex is "x y" with a decimal comma and no grouping, so a space always splits x from y.
  const coord = (n: number) => String(Math.round(n * 100) / 100).replace('.', ',')
  const shown = text ?? ring.map((v) => `${coord(v.x)} ${coord(v.y)}`).join('; ')
  const [error, setError] = useState('')
  const commit = () => {
    if (text === null) {
      return
    }
    const parts = text.split(';').map((s) => s.trim().split(/\s+/).map((n) => parseNumberText(n) ?? NaN))
    if (parts.length < 3 || parts.some((p) => p.length !== 2 || p.some((n) => !Number.isFinite(n)))) {
      setError('Нужно не меньше трёх вершин вида «x y», через точку с запятой.')
      return
    }
    setError('')
    onCommit(parts.map(([x, y]) => ({ x, y })))
    setText(null)
  }
  return (
    <>
      <textarea rows={3} value={shown} onChange={(e) => setText(e.target.value)} onBlur={commit} />
      {error ? (
        <span className="error">
          <Reflow>{error}</Reflow>
        </span>
      ) : null}
    </>
  )
}

export function CalibrationPanel({
  doc,
  pending,
  onCommand,
  onCancel,
}: {
  doc: MapDocument
  pending: { kind: 'calibrate' | 'check'; seg: Omit<MapSegment, 'length_m'> } | null
  onCommand: Dispatch
  onCancel: () => void
}) {
  const [length, setLength] = useState('')
  const cal = doc.calibration
  const dev = checkDeviation(cal.check, cal.meters_per_px)
  const page = doc.page
  return (
    <div className="map-props is-quiet">
      <h3>Масштаб</h3>
      <p>
        <Reflow>
          {numberText(cal.meters_per_px, 5)} м в пикселе
          {page ? `, план ${numberText(page.width_px * cal.meters_per_px, 1)} x ${numberText(page.height_px * cal.meters_per_px, 1)} м` : ''}.
        </Reflow>
      </p>
      {cal.segment ? (
        <p className="field-hint">
          <Reflow>
            Отрезок калибровки {numberText(segmentPx(cal.segment), 0)} пикс = {numberText(cal.segment.length_m)} м.
          </Reflow>
        </p>
      ) : (
        <p className="error">
          <Reflow>Отрезок калибровки не задан. Инструмент Калибровка: два щелчка по концам известного размера.</Reflow>
        </p>
      )}
      {cal.check && dev !== null ? (
        <p className={dev > 5 ? 'error' : undefined}>
          <Reflow>
            Контроль: по плану {numberText(measuredLengthM(cal.check, cal.meters_per_px), 2)} м, указано {numberText(cal.check.length_m)} м,
            расхождение {numberText(dev, 1)}%.{dev > 5 ? ' Масштаб неверный, откалибруйте заново.' : dev > 2 ? ' Точность снижена.' : ''}
          </Reflow>
        </p>
      ) : (
        <p className="field-hint">
          <Reflow>Контрольное измерение не выполнено. Инструмент Контроль: отрезок другого известного размера.</Reflow>
        </p>
      )}
      {pending ? (
        <form
          className="inline-form"
          onSubmit={(e) => {
            e.preventDefault()
            const v = num(length)
            if (v === null || v <= 0) {
              return
            }
            const seg = { ...pending.seg, length_m: v }
            if (pending.kind === 'calibrate') {
              onCommand({ type: 'calibrate', segment: seg })
            } else {
              onCommand({ type: 'setCheck', segment: seg })
            }
            setLength('')
            onCancel()
          }}
        >
          <label>
            {pending.kind === 'calibrate' ? 'Длина отрезка калибровки' : 'Длина контрольного отрезка'}, м (
            {numberText(segmentPx(pending.seg), 0)} пикс)
            <input autoFocus type="text" inputMode="decimal" value={length} onChange={(e) => setLength(e.target.value)} />
          </label>
          <button type="submit" disabled={!(num(length) && Number(num(length)) > 0)}>
            Применить
          </button>
          <button type="button" onClick={onCancel}>
            Отмена
          </button>
        </form>
      ) : null}
      {cal.check ? (
        <button type="button" className="linkish" onClick={() => onCommand({ type: 'setCheck', segment: null })}>
          Убрать контрольное измерение
        </button>
      ) : null}
    </div>
  )
}

export function ResourcesPanel({ doc, onCommand }: { doc: MapDocument; onCommand: Dispatch }) {
  const [kind, setKind] = useState<MapResourceKind>('dock')
  // added is the resource this panel just created; its name field takes the focus.
  const [added, setAdded] = useState<string | null>(null)
  const points = doc.layers.points
  const edges = doc.layers.edges
  const index = pointIndex(doc)
  function update(r: MapResource, patch: Partial<MapResource>) {
    onCommand({ type: 'upsertResource', resource: { ...r, ...patch } })
  }
  return (
    <div className="map-props is-quiet">
      <div className="map-props-head">
        <h3>Общие ресурсы</h3>
        <HelpButton
          label="Общие ресурсы"
          text="Док и зарядка объединяют точки, узкий участок объединяет рёбра. Вместимость: сколько роботов одновременно. Однополосные рёбра без ресурса сервер объединяет сам."
        />
      </div>
      {doc.layers.resources.length > 0 ? (
        <div className="table-wrap">
          <table className="edit-rows">
            <thead>
              <tr>
                <th>Тип</th>
                <th>Название</th>
                <th>Вместимость</th>
                <th>Точки или рёбра</th>
                <th>
                  <span className="sr-only">Действия</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {byLabel(doc.layers.resources).map((r) => {
                const memberIds = r.kind === 'narrow_aisle' ? (r.edge_ids ?? []) : resourcePoints(r)
                const options =
                  r.kind === 'narrow_aisle'
                    ? byLabel(edges, (e) => edgeLabel(e, index)).map((e) => ({ value: e.id, label: edgeLabel(e, index) }))
                    : byLabel(points.filter((p) => p.kind === (r.kind === 'dock' ? 'dock' : 'charger'))).map((p) => ({
                        value: p.id,
                        label: pointOption(p),
                      }))
                const name = r.name || labelOf(resourceKinds, r.kind)
                return (
                  <tr key={r.id} data-resource={r.id}>
                    <td>{labelOf(resourceKinds, r.kind)}</td>
                    <td>
                      <TextField
                        aria-label={`Название: ${name}`}
                        maxLength={120}
                        value={r.name ?? ''}
                        autoFocus={r.id === added}
                        onCommit={(v) => update(r, { name: v || undefined })}
                      />
                    </td>
                    <td>
                      <NumberInput
                        label={`Вместимость: ${name}`}
                        min={1}
                        max={100}
                        integer
                        value={r.capacity}
                        onCommit={(capacity) => update(r, { capacity })}
                      />
                    </td>
                    <td>
                      <CheckSelect
                        label={r.kind === 'narrow_aisle' ? 'Рёбра' : 'Точки'}
                        rowLabel={name}
                        options={options}
                        value={memberIds}
                        empty={
                          r.kind === 'narrow_aisle'
                            ? 'Сначала нарисуйте рёбра.'
                            : `Сначала поставьте точки типа ${r.kind === 'dock' ? 'Док' : 'Зарядка'}.`
                        }
                        onChange={(ids) => {
                          if (r.kind === 'narrow_aisle') {
                            update(r, { edge_ids: ids })
                          } else {
                            update(r, { point_id: undefined, point_ids: ids })
                          }
                        }}
                      />
                    </td>
                    <td>
                      <button type="button" className="btn btn-danger" onClick={() => onCommand({ type: 'deleteResource', id: r.id })}>
                        Удалить
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      ) : null}
      <div className="inline-form">
        <Select value={kind} options={resourceKinds} onChange={setKind} aria-label="Тип ресурса" />
        <button
          type="button"
          onClick={() => {
            const id = newFeatureId()
            setAdded(id)
            onCommand({
              type: 'upsertResource',
              resource: {
                id,
                kind,
                name: nextName(doc, 'res-'),
                capacity: 1,
                ...(kind === 'narrow_aisle' ? { edge_ids: [] } : { point_ids: [] }),
              },
            })
          }}
        >
          Добавить ресурс
        </button>
      </div>
    </div>
  )
}

export function FlowsPanel({
  doc,
  processes,
  focus,
  onFocus,
  onCommand,
}: {
  doc: MapDocument
  processes: Process[]
  focus: string | null
  onFocus: (code: string | null) => void
  onCommand: Dispatch
}) {
  const flows = doc.layers.flows ?? []
  const options = byLabel(doc.layers.points.filter((p) => p.kind !== 'charger' && p.kind !== 'other')).map((p) => ({
    value: p.id,
    label: pointOption(p),
  }))
  const noPoints = 'Сначала поставьте точки задания, доки или проходы.'
  const orphan = flows.filter((f) => !processes.some((p) => p.code === f.process_code))
  return (
    <div className="map-props is-quiet">
      <h3>Потоки процессов</h3>
      {processes.length > 0 ? (
        <div className="table-wrap">
          <table className="edit-rows">
            <thead>
              <tr>
                <th>Процесс</th>
                <th>Забор</th>
                <th>Доставка</th>
                <th>
                  <span className="sr-only">Действия</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {processes.map((p) => {
                const f = flows.find((x) => x.process_code === p.code) ?? { process_code: p.code, pickup_point_ids: [], drop_point_ids: [] }
                const set = (patch: Partial<typeof f>) => onCommand({ type: 'setFlow', flow: { ...f, ...patch } })
                return (
                  <tr key={p.code} className={focus === p.code ? 'row-selected' : undefined} data-flow={p.code}>
                    <td>
                      <button
                        type="button"
                        className="linkish"
                        aria-pressed={focus === p.code}
                        title="Показать поток на карте"
                        onClick={() => onFocus(focus === p.code ? null : p.code)}
                      >
                        {p.name}
                      </button>
                      {hasFlow(doc, p.code) ? null : (
                        <>
                          {' '}
                          <span className="tag tag-warning">нет потока</span>
                        </>
                      )}
                    </td>
                    <td>
                      <CheckSelect
                        label="Забор"
                        rowLabel={p.name}
                        options={options}
                        value={f.pickup_point_ids}
                        empty={noPoints}
                        onChange={(ids) => set({ pickup_point_ids: ids })}
                      />
                    </td>
                    <td>
                      <CheckSelect
                        label="Доставка"
                        rowLabel={p.name}
                        options={options}
                        value={f.drop_point_ids}
                        empty={noPoints}
                        onChange={(ids) => set({ drop_point_ids: ids })}
                      />
                    </td>
                    <td>
                      {flows.some((x) => x.process_code === p.code) ? (
                        <button type="button" className="btn btn-danger" onClick={() => onCommand({ type: 'deleteFlow', processCode: p.code })}>
                          Убрать
                        </button>
                      ) : null}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      ) : null}
      {orphan.map((f) => (
        <p key={f.process_code} className="error">
          <Reflow>
            Поток {f.process_code} ссылается на процесс, которого нет в проекте.{' '}
            <button type="button" className="linkish" onClick={() => onCommand({ type: 'deleteFlow', processCode: f.process_code })}>
              Удалить
            </button>
          </Reflow>
        </p>
      ))}
    </div>
  )
}

export function IssuesPanel({
  issues,
  classes,
  checking,
  onPick,
}: {
  issues: MapIssue[]
  classes: MapClassCheck[]
  checking: boolean
  onPick: (ref: string) => void
}) {
  const errors = issues.filter((i) => i.level === 'error')
  const warnings = issues.filter((i) => i.level === 'warning')
  return (
    <div className="map-props" aria-live="polite">
      <h3>
        <Reflow>
          Проверка карты{checking ? ' (идёт...)' : ''}: ошибок {errors.length}, предупреждений {warnings.length}
        </Reflow>
      </h3>
      <ul className="issue-list">
        {[...errors, ...warnings].map((is, i) => (
          <li key={`${is.code}:${is.ref ?? ''}:${i}`} className={is.level === 'error' ? 'error' : 'risk-warning'}>
            {is.ref ? (
              <button type="button" className="linkish" onClick={() => onPick(is.ref ?? '')}>
                <Reflow>{is.message}</Reflow>
              </button>
            ) : (
              <Reflow>{is.message}</Reflow>
            )}
          </li>
        ))}
      </ul>
      {classes.length > 0 ? (
        <p className="field-hint">
          <Reflow>
            Проверено для{' '}
            {classes
              .map((cl) => `${cl.label}: проезд от ${numberText(cl.required_width_m, 2)} м, разъезд от ${numberText(cl.two_lane_width_m, 2)} м`)
              .join('; ')}
            .{classes.some((cl) => cl.default) ? ' Класс по умолчанию: в проекте нет флота.' : ''}
          </Reflow>
        </p>
      ) : null}
    </div>
  )
}

// EdgeLegend is the key to the edge colours on the map canvas (MapCanvas: blocked, single lane, two lanes).
export function EdgeLegend() {
  const pal = useCanvasPalette()
  const items: [string, string][] = [
    [pal.path, 'двухполосное'],
    [pal.warning, 'однополосное, встречные ждут'],
    [pal.danger, 'непроходимо'],
  ]
  return (
    <ul className="legend is-row" aria-label="Цвет ребра">
      {items.map(([color, label]) => (
        <li key={label}>
          <span className="swatch" style={{ background: color }} />
          {label}
        </li>
      ))}
    </ul>
  )
}


export function FeatureTables({
  doc,
  selection,
  onSelect,
  onCommand,
  defaultWidthM,
}: {
  doc: MapDocument
  selection: Selection | null
  onSelect: (s: Selection) => void
  onCommand: Dispatch
  defaultWidthM: number
}) {
  const mpp = doc.calibration.meters_per_px
  const [newPoint, setNewPoint] = useState({ kind: 'task' as MapPointKind, x: '', y: '' })
  const [newEdge, setNewEdge] = useState({ from: '', to: '' })
  const index = pointIndex(doc)
  const pts = byLabel(doc.layers.points)
  const edges = byLabel(doc.layers.edges, (e) => edgeLabel(e, index))
  const pointIds = pts.map((p) => ({ value: p.id, label: featureLabel(p) }))
  return (
    <details className="map-props is-quiet">
      <summary>Точки, рёбра и зоны</summary>
      <h4>Точки</h4>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Код</th>
              <th>Тип</th>
              <th>X, м</th>
              <th>Y, м</th>
            </tr>
          </thead>
          <tbody>
            {pts.map((p) => (
              <tr key={p.id} className={selection?.type === 'point' && selection.id === p.id ? 'row-selected' : undefined}>
                <td>
                  <button type="button" className="linkish" onClick={() => onSelect({ type: 'point', id: p.id })}>
                    {featureLabel(p)}
                  </button>
                </td>
                <td>{labelOf(pointKinds, p.kind)}</td>
                <td>
                  <NumberInput label={`${featureLabel(p)} X`} value={p.x * mpp} onCommit={(v) => onCommand({ type: 'updatePoint', id: p.id, patch: { x: v / mpp } })} />
                </td>
                <td>
                  <NumberInput label={`${featureLabel(p)} Y`} value={p.y * mpp} onCommit={(v) => onCommand({ type: 'updatePoint', id: p.id, patch: { y: v / mpp } })} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <form
        className="inline-form"
        onSubmit={(e) => {
          e.preventDefault()
          const x = num(newPoint.x)
          const y = num(newPoint.y)
          if (x === null || y === null) {
            return
          }
          const id = newFeatureId()
          onCommand({ type: 'addPoint', point: { id, kind: newPoint.kind, name: nextName(doc, pointPrefix[newPoint.kind]), x: x / mpp, y: y / mpp } })
          onSelect({ type: 'point', id })
          setNewPoint({ ...newPoint, x: '', y: '' })
        }}
      >
        <Select aria-label="Тип новой точки" value={newPoint.kind} options={pointKinds} onChange={(kind) => setNewPoint({ ...newPoint, kind })} />
        <UnitField unit="м">
          <input aria-label="X новой точки, м" placeholder="X" value={newPoint.x} onChange={(e) => setNewPoint({ ...newPoint, x: e.target.value })} />
        </UnitField>
        <UnitField unit="м">
          <input aria-label="Y новой точки, м" placeholder="Y" value={newPoint.y} onChange={(e) => setNewPoint({ ...newPoint, y: e.target.value })} />
        </UnitField>
        <button type="submit">Добавить точку</button>
      </form>
      <h4>Рёбра</h4>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Ребро</th>
              <th>Откуда</th>
              <th>Куда</th>
              <th>Длина, м</th>
              <th>Ширина, м</th>
            </tr>
          </thead>
          <tbody>
            {edges.map((e) => (
              <tr key={e.id} className={selection?.type === 'edge' && selection.id === e.id ? 'row-selected' : undefined}>
                <td>
                  <button type="button" className="linkish" onClick={() => onSelect({ type: 'edge', id: e.id })}>
                    {edgeLabel(e, index)}
                  </button>
                </td>
                <td>{index.get(e.from) ? featureLabel(index.get(e.from) as MapPoint) : e.from}</td>
                <td>{index.get(e.to) ? featureLabel(index.get(e.to) as MapPoint) : e.to}</td>
                <td>{numberText(edgeLengthM(doc, e), 1)}</td>
                <td>
                  <NumberInput
                    label={`${edgeLabel(e, index)} ширина`}
                    min={0.1}
                    value={e.width_m ?? 0}
                    onCommit={(v) => onCommand({ type: 'updateEdge', id: e.id, patch: { width_m: v > 0 ? v : undefined } })}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <form
        className="inline-form"
        onSubmit={(e) => {
          e.preventDefault()
          if (!newEdge.from || !newEdge.to) {
            return
          }
          onCommand({ type: 'addEdge', edge: { id: newFeatureId(), from: newEdge.from, to: newEdge.to, width_m: defaultWidthM } })
          setNewEdge({ from: '', to: '' })
        }}
      >
        <Select
          aria-label="Начало ребра"
          value={newEdge.from}
          options={[{ value: '', label: 'Откуда' }, ...pointIds]}
          onChange={(from) => setNewEdge({ ...newEdge, from })}
        />
        <Select
          aria-label="Конец ребра"
          value={newEdge.to}
          options={[{ value: '', label: 'Куда' }, ...pointIds]}
          onChange={(to) => setNewEdge({ ...newEdge, to })}
        />
        <button type="submit" disabled={!newEdge.from || !newEdge.to || newEdge.from === newEdge.to}>
          Добавить ребро
        </button>
      </form>
      <h4>Зоны и препятствия</h4>
      <ul>
        {(['zones', 'obstacles'] as const).flatMap((layer) =>
          doc.layers[layer].map((poly) => (
            <li key={poly.id}>
              <button type="button" className="linkish" onClick={() => onSelect({ type: layer, id: poly.id })}>
                {featureLabel(poly)}
              </button>{' '}
              {labelOf(layer === 'zones' ? zoneKinds : obstacleKinds, poly.kind)} {poly.name && poly.name !== featureLabel(poly) ? poly.name : ''}
            </li>
          )),
        )}
      </ul>
    </details>
  )
}
