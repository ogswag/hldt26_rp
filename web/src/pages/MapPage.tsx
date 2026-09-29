import { useQuery } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import {
  fetchMapTemplate,
  fetchProject,
  validateProjectMap,
  type MapCheck,
  type MapDocument,
  type MapIssue,
  type MapPointKind,
  type MapSegment,
  type ParamsMap,
  type Process,
} from '../api/client'
import { checkMap, mapTemplate } from '../engine/client'
import { engineCatalog, NoCatalog } from '../engine/fallback'
import { plural } from '../fleet/format'
import { useProjectRoute } from '../layout/route'
import { preview, type MapCommand } from '../map/commands'
import { asMapDocument, counts, emptyMap, featureCenter, forSave, hasFlow, obstacleKinds, pointKinds, zoneKinds } from '../map/document'
import { MapCanvas, type Selection, type ShapeMode, type Tool } from '../map/MapCanvas'
import {
  CalibrationPanel,
  EdgeLegend,
  FeatureTables,
  FlowsPanel,
  IssuesPanel,
  ResourcesPanel,
  SelectionPanel,
} from '../map/MapPanels'
import { forgetLocalPlan, loadLocalPlan, loadPlan, PlanError, savePlanLocally, type Plan } from '../map/planFile'
import { commandLabel, commandOps, deleteMapOps, mapSelector, type MapPage as MapPageSize } from '../map/records'
import { paramsOf } from '../projects/econRecords'
import { processesSelector } from '../projects/processRecords'
import { useOnSearchTarget } from '../search/target'
import type { State } from '../store/apply'
import { reasonText } from '../store/reasons'
import { usePresence, useProjectStore, useReportSelection, useStoreSelector, useUndoHistory } from '../store/useProjectStore'
import { undoKeys } from '../store/useUndoShortcuts'
import { ActionBar } from '../ui/ActionBar'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { isTyping } from '../ui/keys'
import { Reflow } from '../ui/Reflow'
import { Select, type SelectOption } from '../ui/Select'
import { showToast } from '../ui/toast'

// mapStatus is the line in the action bar: what the check found, in words, and the processes the simulation
// leaves out because they have no flow.
function mapStatus(checking: boolean, issues: readonly MapIssue[], noFlow: number): string {
  if (checking) {
    return 'Проверяем карту...'
  }
  const errors = issues.filter((i) => i.level === 'error').length
  const warnings = issues.length - errors
  const flows =
    noFlow > 0
      ? `${noFlow} ${plural(noFlow, 'процесс', 'процесса', 'процессов')} без потока: симуляция ${noFlow === 1 ? 'его' : 'их'} не моделирует`
      : ''
  if (errors === 0 && warnings === 0) {
    return flows || 'Карта без ошибок'
  }
  const parts = []
  if (errors > 0) {
    parts.push(`${errors} ${plural(errors, 'ошибка', 'ошибки', 'ошибок')}`)
  }
  if (warnings > 0) {
    parts.push(`${warnings} ${plural(warnings, 'предупреждение', 'предупреждения', 'предупреждений')}`)
  }
  return [`На карте ${parts.join(' и ')}`, flows].filter(Boolean).join('; ')
}

const tools: { value: Tool; label: string; key: string }[] = [
  { value: 'select', label: 'Выбор', key: 'v' },
  { value: 'pan', label: 'Сдвиг', key: 'h' },
  { value: 'point', label: 'Точка', key: 'p' },
  { value: 'edge', label: 'Ребро', key: 'e' },
  { value: 'zone', label: 'Зона', key: 'z' },
  { value: 'obstacle', label: 'Препятствие', key: 'o' },
  { value: 'calibrate', label: 'Калибровка', key: 'k' },
  { value: 'check', label: 'Контроль', key: 'm' },
]

const shapeOptions: SelectOption<ShapeMode>[] = [
  { value: 'rect', label: 'Прямоугольник (2 щелчка)' },
  { value: 'poly', label: 'Многоугольник' },
]

// MapBackend is what the map page asks of the outside: the check of a map and the starter map. A project asks the
// server; a demo has no server behind it and asks the engine in the browser.
type MapBackend = {
  check(doc: MapDocument, signal: AbortSignal): Promise<MapCheck>
  template(): Promise<MapDocument>
}

function serverBackend(projectId: string): MapBackend {
  return {
    check: (doc, signal) => validateProjectMap(projectId, doc, signal),
    template: () => fetchMapTemplate(projectId),
  }
}

function demoBackend(state: () => State): MapBackend {
  return {
    async check(doc, signal) {
      const catalog = await engineCatalog()
      if (!catalog) {
        throw new Error(NoCatalog)
      }
      const out = await checkMap(catalog, state(), doc)
      if (signal.aborted) {
        throw new DOMException('Aborted', 'AbortError')
      }
      return out
    },
    template: () => mapTemplate(state()),
  }
}

const warehouseOnly = (
  <section>
    <h1 className="sr-only">Карта объекта</h1>
    <p>
      <Reflow>Редактор карты и событийная симуляция пока доступны только для склада.</Reflow>
    </p>
  </section>
)

export function MapPage() {
  const { projectId, demo, storeKey } = useProjectRoute()
  if (demo) {
    return demo === 'warehouse' ? <DemoMap storeKey={storeKey} /> : warehouseOnly
  }
  return projectId ? <MapEditor projectId={projectId} /> : null
}

function DemoMap({ storeKey }: { storeKey: string }) {
  const store = useProjectStore(storeKey)
  const selectProcesses = useMemo(() => processesSelector(), [])
  const processes = useStoreSelector(store, selectProcesses)
  const params = useStoreSelector(store, paramsOf)
  const backend = useMemo(() => demoBackend(() => store.getState()), [store])
  return <MapWorkspace storeKey={storeKey} processes={processes} params={params} backend={backend} />
}

function MapEditor({ projectId }: { projectId: string }) {
  const projectQ = useQuery({ queryKey: ['project', projectId], queryFn: () => fetchProject(projectId) })
  const backend = useMemo(() => serverBackend(projectId), [projectId])
  if (projectQ.isPending) {
    return (
      <section>
        <h1 className="sr-only">Карта склада</h1>
        <p>Загрузка...</p>
      </section>
    )
  }
  if (projectQ.isError) {
    return (
      <section>
        <h1 className="sr-only">Карта склада</h1>
        <p className="error">
          <Reflow>{projectQ.error instanceof Error ? projectQ.error.message : 'Не удалось загрузить проект.'}</Reflow>
        </p>
      </section>
    )
  }
  if (projectQ.data.object_type !== 'warehouse') {
    return warehouseOnly
  }
  return (
    <MapWorkspace
      storeKey={projectId}
      processes={projectQ.data.processes ?? []}
      params={projectQ.data.params as ParamsMap}
      backend={backend}
    />
  )
}

type WorkspaceProps = { storeKey: string; processes: Process[]; params: ParamsMap; backend: MapBackend }

function MapWorkspace({ storeKey, processes, params, backend }: WorkspaceProps) {
  const store = useProjectStore(storeKey)
  const [history, undoState] = useUndoHistory(storeKey)
  const ready = useStoreSelector(store, () => store.ready)
  const selectMap = useMemo(() => mapSelector(), [])
  const stored = useStoreSelector(store, selectMap)
  const [plan, setPlan] = useState<Plan | null>(null)
  const [planNote, setPlanNote] = useState('')
  const [tool, setTool] = useState<Tool>('select')
  const [pointKind, setPointKind] = useState<MapPointKind>('task')
  const [zoneKind, setZoneKind] = useState('storage')
  const [obstacleKind, setObstacleKind] = useState('rack')
  const [shape, setShape] = useState<ShapeMode>('rect')
  const [selection, setSelection] = useState<Selection | null>(null)
  const [pendingSegment, setPendingSegment] = useState<{ kind: 'calibrate' | 'check'; seg: Omit<MapSegment, 'length_m'> } | null>(null)
  const [flowFocus, setFlowFocus] = useState<string | null>(null)
  const [check, setCheck] = useState<MapCheck | null>(null)
  const [checking, setChecking] = useState(false)
  const [note, setNote] = useState('')
  const [error, setError] = useState('')
  const [confirmRemove, setConfirmRemove] = useState(false)
  // drag is the move in progress: drawn at once, sent as one transaction on release.
  const [drag, setDrag] = useState<MapCommand | null>(null)
  const dragRef = useRef<MapCommand | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const fallbackPage = useMemo<MapPageSize>(
    () => (plan ? { width_px: plan.width, height_px: plan.height, source_kind: plan.kind } : { width_px: 1000, height_px: 700, source_kind: 'none' }),
    [plan],
  )
  const base = useMemo(() => stored ?? emptyMap(fallbackPage), [stored, fallbackPage])
  const doc = drag ? preview(base, drag) : base

  useEffect(() => {
    let alive = true
    void loadLocalPlan(storeKey).then((p) => {
      if (alive && p) {
        setPlan((cur) => cur ?? p)
      }
    })
    return () => {
      alive = false
    }
  }, [storeKey])

  useEffect(() => {
    const ctrl = new AbortController()
    const timer = window.setTimeout(() => {
      setChecking(true)
      backend.check(forSave(base), ctrl.signal)
        .then((c) => setCheck(c))
        .catch((err: unknown) => {
          if (!(err instanceof DOMException && err.name === 'AbortError')) {
            setError(err instanceof Error ? err.message : 'Не удалось проверить карту.')
          }
        })
        .finally(() => setChecking(false))
    }, 600)
    return () => {
      window.clearTimeout(timer)
      ctrl.abort()
    }
  }, [base, backend])

  const commit = useCallback(
    (cmd: MapCommand, mergeKey: string | null = null) => {
      setNote('')
      const ops = commandOps(store.getState(), cmd, fallbackPage)
      if (ops.length === 0) {
        return
      }
      const r = history.run(commandLabel(cmd), ops, mergeKey)
      setError(r.outcome.status === 'rejected' ? `Изменение не применено. ${reasonText(r.outcome.reason)}` : '')
    },
    [store, history, fallbackPage],
  )

  const dispatch = useCallback(
    (cmd: MapCommand, mergeKey?: string) => {
      if (mergeKey?.startsWith('drag:') || mergeKey?.startsWith('vertex:')) {
        dragRef.current = cmd
        setDrag(cmd)
        return
      }
      commit(cmd)
    },
    [commit],
  )

  const endDrag = useCallback(() => {
    const cmd = dragRef.current
    dragRef.current = null
    setDrag(null)
    if (cmd) {
      commit(cmd)
    }
  }, [commit])

  const step = useCallback(
    (redo: boolean) => {
      const out = redo ? history.redo() : history.undo()
      if (out?.status === 'rejected') {
        showToast({ message: `${redo ? 'Повтор не применён' : 'Отмена не применена'}. ${reasonText(out.reason)}` })
      }
    },
    [history],
  )

  const deleteSelection = useCallback(() => {
    if (!selection) {
      return
    }
    if (selection.type === 'point') {
      dispatch({ type: 'deletePoint', id: selection.id })
    } else if (selection.type === 'edge') {
      dispatch({ type: 'deleteEdge', id: selection.id })
    } else {
      dispatch({ type: 'deletePolygon', layer: selection.type, id: selection.id })
    }
    setSelection(null)
  }, [selection, dispatch])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (isTyping(e.target)) {
        return
      }
      // Undo and redo work on every page of the project (store/useUndoShortcuts).
      if (e.ctrlKey || e.metaKey) {
        return
      }
      if (e.key === 'Delete' || e.key === 'Backspace') {
        e.preventDefault()
        deleteSelection()
        return
      }
      if (e.key === 'Escape') {
        setSelection(null)
        setPendingSegment(null)
        return
      }
      if (selection?.type === 'point' && e.key.startsWith('Arrow')) {
        e.preventDefault()
        const shift = e.shiftKey ? 10 : 1
        const dx = e.key === 'ArrowLeft' ? -shift : e.key === 'ArrowRight' ? shift : 0
        const dy = e.key === 'ArrowUp' ? -shift : e.key === 'ArrowDown' ? shift : 0
        const pos = store.getState().map_points?.[selection.id]?.pos as { x: number; y: number } | undefined
        if (pos) {
          commit({ type: 'updatePoint', id: selection.id, patch: { x: pos.x + dx, y: pos.y + dy } }, `nudge:${selection.id}`)
        }
        return
      }
      const t = tools.find((x) => x.key === e.key.toLowerCase())
      if (t) {
        setTool(t.value)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [selection, deleteSelection, step, store, commit])

  function removeMap() {
    const r = history.run('Удалить карту', deleteMapOps(store.getState()))
    if (r.outcome.status === 'applied') {
      setConfirmRemove(false)
      setSelection(null)
      setNote(`Карта удалена. Симуляция будет использовать шаблон склада. ${undoKeys} вернёт её.`)
    }
  }

  async function applyTemplate() {
    setError('')
    try {
      const t = await backend.template()
      const next = asMapDocument(t)
      if (next) {
        dispatch({ type: 'replace', doc: next })
        setPlan(null)
        setSelection(null)
        setNote('Загружен шаблон склада 84 x 52 м. Замените его разметкой своего плана.')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Не удалось загрузить шаблон.')
    }
  }

  async function onFile(file: File | undefined) {
    if (!file) {
      return
    }
    setError('')
    setPlanNote('Читаем план...')
    try {
      const p = await loadPlan(file)
      setPlan(p)
      void savePlanLocally(storeKey, p)
      const hasFeatures = counts(doc).points + counts(doc).zones + counts(doc).obstacles > 0
      const page = stored?.page
      if (!hasFeatures || !page) {
        dispatch({ type: 'setPage', page: { width_px: p.width, height_px: p.height, source_kind: p.kind } })
        setPlanNote(`${p.note} Проведите отрезок калибровки по известному размеру.`.trim())
        setTool('calibrate')
        return
      }
      const ratio = p.width / p.height
      const pageRatio = page.width_px / page.height_px
      const mismatch = Math.abs(ratio - pageRatio) / pageRatio > 0.01
      dispatch({ type: 'setPage', page: { ...page, source_kind: p.kind } })
      setPlanNote(
        `${p.note} План растянут под размер разметки ${page.width_px} x ${page.height_px}.${mismatch ? ' Пропорции плана отличаются от разметки: проверьте совмещение и масштаб.' : ''}`.trim(),
      )
    } catch (err) {
      setPlanNote('')
      setError(err instanceof PlanError ? err.message : 'Не удалось прочитать план. Попробуйте другой файл.')
    }
  }

  // Presence: others see what this tab has selected, and the canvas marks what they have.
  const selectionColl: Record<Selection['type'], string> = {
    point: 'map_points',
    edge: 'map_edges',
    zones: 'map_zones',
    obstacles: 'map_obstacles',
  }
  useReportSelection(storeKey, selection ? { coll: selectionColl[selection.type], id: selection.id } : null)
  const others = usePresence(storeKey)
  const watchers = useMemo(() => {
    const out = new Map<string, { initials: string; color: number }>()
    for (const person of others) {
      for (const sel of person.selections) {
        if (sel && sel.coll.startsWith('map')) {
          out.set(sel.id, { initials: person.initials, color: person.color })
        }
      }
    }
    return out
  }, [others])

  const edgeChecks = useMemo(() => new Map((check?.edges ?? []).map((e) => [e.id, e])), [check])
  const issueRefs = useMemo(() => new Set((check?.issues ?? []).filter((i) => i.level === 'error').map((i) => i.ref ?? '')), [check])
  const defaultWidthM = useMemo(() => {
    const v = params.aisle_main_m
    return typeof v === 'number' && v > 0 ? v : 3
  }, [params])

  const focusFlow = flowFocus ? (doc.layers.flows ?? []).find((f) => f.process_code === flowFocus) : undefined

  // A map object found by the search is selected and brought to the middle of the canvas once the map is ready.
  const [sought, setSought] = useState<{ ref: string; n: number } | null>(null)
  const [soughtDone, setSoughtDone] = useState(0)
  useOnSearchTarget((t) => {
    if (t.entry.kind === 'map') {
      setSought((prev) => ({ ref: t.entry.id.replace(/^map:/, ''), n: (prev?.n ?? 0) + 1 }))
    }
  })
  if (sought && ready && soughtDone !== sought.n) {
    setSoughtDone(sought.n)
    pickRef(sought.ref.replace(/^flow:/, ''))
  }
  const soughtAt = sought && ready ? featureCenter(doc, sought.ref) : null
  const focus = soughtAt && sought ? { ...soughtAt, n: sought.n } : null

  function pickRef(ref: string) {
    if (doc.layers.points.some((p) => p.id === ref)) {
      setSelection({ type: 'point', id: ref })
    } else if (doc.layers.edges.some((e) => e.id === ref)) {
      setSelection({ type: 'edge', id: ref })
    } else if (doc.layers.obstacles.some((o) => o.id === ref)) {
      setSelection({ type: 'obstacles', id: ref })
    } else if (doc.layers.zones.some((z) => z.id === ref)) {
      setSelection({ type: 'zones', id: ref })
    } else if (processes.some((p) => p.code === ref)) {
      setFlowFocus(ref)
    }
    setTool('select')
  }

  if (!ready) {
    return (
      <section>
        <h1 className="sr-only">Карта склада</h1>
        <p>Загрузка...</p>
      </section>
    )
  }

  return (
    <section className="map-page">
      <h1 className="sr-only">Карта склада</h1>
      {note ? (
        <p className="note">
          <Reflow>{note}</Reflow>
        </p>
      ) : null}
      {error ? (
        <p className="error">
          <Reflow>{error}</Reflow>
        </p>
      ) : null}
      {planNote ? (
        <p className="field-hint">
          <Reflow>{planNote}</Reflow>
        </p>
      ) : null}

      <div className="actions">
        <input
          ref={fileRef}
          className="file-input"
          type="file"
          accept=".png,.jpg,.jpeg,.pdf,image/png,image/jpeg,application/pdf"
          onChange={(e) => {
            void onFile(e.target.files?.[0])
            e.target.value = ''
          }}
        />
        <button type="button" onClick={() => fileRef.current?.click()}>
          Загрузить план (PNG, JPEG, PDF)
        </button>
        {plan ? (
          <button
            type="button"
            onClick={() => {
              setPlan(null)
              void forgetLocalPlan(storeKey)
            }}
          >
            Скрыть план
          </button>
        ) : null}
        <button type="button" onClick={() => void applyTemplate()}>
          Шаблон склада
        </button>
        <button type="button" onClick={() => step(false)} disabled={!undoState.undo} title={undoState.undo ? `Отменить: ${undoState.undo}` : undefined}>
          Отменить
        </button>
        <button type="button" onClick={() => step(true)} disabled={!undoState.redo} title={undoState.redo ? `Повторить: ${undoState.redo}` : undefined}>
          Повторить
        </button>
      </div>

      {stored ? (
        <ActionBar label="Карта" status={mapStatus(checking, check?.issues ?? [], processes.filter((p) => !hasFlow(doc, p.code)).length)}>
          <button type="button" className="btn btn-danger" onClick={() => setConfirmRemove(true)}>
            Удалить карту
          </button>
        </ActionBar>
      ) : null}
      <ConfirmDialog
        open={confirmRemove}
        title="Удалить карту проекта?"
        confirmLabel="Удалить карту"
        cancelLabel="Отмена"
        onConfirm={removeMap}
        onCancel={() => setConfirmRemove(false)}
      >
        <p>
          <Reflow>
            Разметка будет удалена. Симуляция станет использовать шаблон склада. Действие можно отменить через {undoKeys}.
          </Reflow>
        </p>
      </ConfirmDialog>

      <div className="map-toolbar" role="toolbar" aria-label="Инструменты карты">
        {tools.map((t) => (
          <button
            key={t.value}
            type="button"
            aria-pressed={tool === t.value}
            className={tool === t.value ? 'tool active' : 'tool'}
            onClick={() => setTool(t.value)}
            title={`Клавиша ${t.key.toUpperCase()}`}
          >
            {t.label}
          </button>
        ))}
        {tool === 'point' ? (
          <Select aria-label="Тип точки" value={pointKind} options={pointKinds} onChange={setPointKind} />
        ) : null}
        {tool === 'zone' || tool === 'obstacle' ? (
          <>
            <Select
              aria-label="Тип контура"
              value={tool === 'zone' ? zoneKind : obstacleKind}
              options={tool === 'zone' ? zoneKinds : obstacleKinds}
              onChange={tool === 'zone' ? setZoneKind : setObstacleKind}
            />
            <Select aria-label="Форма" value={shape} options={shapeOptions} onChange={setShape} />
          </>
        ) : null}
      </div>

      <div className="map-layout" data-points={doc.layers.points.length}>
        <MapCanvas
          doc={doc}
          plan={plan}
          tool={tool}
          pointKind={pointKind}
          zoneKind={zoneKind}
          obstacleKind={obstacleKind}
          shape={shape}
          defaultWidthM={defaultWidthM}
          selection={selection}
          edgeChecks={edgeChecks}
          issueRefs={issueRefs}
          flowFocus={focusFlow ? { pickups: focusFlow.pickup_point_ids, drops: focusFlow.drop_point_ids } : null}
          watchers={watchers}
          focus={focus}
          height={640}
          onSelect={setSelection}
          onCommand={dispatch}
          onEndDrag={endDrag}
          onSegment={(kind, seg) => {
            setPendingSegment({ kind, seg })
            setTool('select')
          }}
        />
        <aside className="map-side">
          <CalibrationPanel doc={doc} pending={pendingSegment} onCommand={dispatch} onCancel={() => setPendingSegment(null)} />
          <SelectionPanel doc={doc} selection={selection} edgeChecks={edgeChecks} onCommand={dispatch} onSelect={setSelection} />
          <IssuesPanel issues={check?.issues ?? []} classes={check?.classes ?? []} checking={checking} onPick={pickRef} />
        </aside>
      </div>

      <EdgeLegend />

      <div className="map-bottom">
        <FlowsPanel doc={doc} processes={processes} focus={flowFocus} onFocus={setFlowFocus} onCommand={dispatch} />
        <ResourcesPanel doc={doc} onCommand={dispatch} />
      </div>
      <FeatureTables
        doc={doc}
        selection={selection}
        onSelect={(s) => {
          setSelection(s)
          setTool('select')
        }}
        onCommand={dispatch}
        defaultWidthM={defaultWidthM}
      />
    </section>
  )
}
