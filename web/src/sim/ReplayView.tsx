import type Konva from 'konva'
import { useEffect, useMemo, useRef, useState, type RefObject } from 'react'
import { Circle, Group, Layer, Line, Rect, Stage, Text } from 'react-konva'

import type { SimulationLog } from '../api/client'
import { featureLabel } from '../map/document'
import { triggerDownload } from '../ui/download'
import { Reflow } from '../ui/Reflow'
import { Select } from '../ui/Select'
import { useCanvasPalette, type CanvasPalette } from '../ui/theme'

import { canvasBlob, composeFrame } from './frame'
import { exportFileName, frameCaption } from './exportRun'
import {
  buildReplay,
  counterAt,
  formatClock,
  jobsDoneAt,
  nextViolation,
  poseAt,
  robotForJob,
  robotStateLabels,
  trailOf,
  type Pose,
  type Replay,
  type RobotState,
  type Violation,
} from './replay'

const speedOptions = [1, 10, 30, 60, 120, 300, 600].map((s) => ({ value: String(s), label: `${s}x` }))
const trailWindowS = 180

const robotStates: RobotState[] = ['idle', 'moving', 'loaded', 'handling', 'waiting', 'charging']

function stateColor(pal: CanvasPalette, state: RobotState): string {
  return {
    idle: pal.idle,
    moving: pal.path,
    loaded: pal.charger,
    handling: pal.dock,
    waiting: pal.danger,
    charging: pal.gate,
  }[state]
}

function zoneFill(pal: CanvasPalette, kind: string): string {
  const fills: Record<string, string> = {
    storage: pal.zoneStorage,
    dock_area: pal.zoneDock,
    pick: pal.zonePick,
    charge: pal.zoneCharge,
  }
  return fills[kind] ?? 'transparent'
}

type Props = {
  log: SimulationLog
  processNames: Map<string, string>
  variantName: string
}

const frameRatio = 2

export function ReplayView({ log, processNames, variantName }: Props) {
  const replay = useMemo(() => buildReplay(log), [log])
  const pal = useCanvasPalette()
  const stageRef = useRef<Konva.Stage>(null)
  const [frameError, setFrameError] = useState(false)
  const [t, setT] = useState(0)
  const [playing, setPlaying] = useState(false)
  const [speed, setSpeed] = useState(60)
  const [selected, setSelected] = useState<string | null>(null)
  const [showTrail, setShowTrail] = useState(true)
  const [violation, setViolation] = useState<{ v: Violation; later: string | null } | null>(null)
  const tRef = useRef(0)
  const lastRef = useRef<number | null>(null)
  const playingRef = useRef(false)
  const speedRef = useRef(speed)

  useEffect(() => {
    playingRef.current = playing
    lastRef.current = null
  }, [playing])
  useEffect(() => {
    speedRef.current = speed
  }, [speed])

  useEffect(() => {
    let id = 0
    const tick = (now: number) => {
      if (playingRef.current) {
        const last = lastRef.current ?? now
        lastRef.current = now
        let next = tRef.current + ((now - last) / 1000) * speedRef.current
        if (next >= replay.horizon) {
          next = replay.horizon
          playingRef.current = false
          setPlaying(false)
        }
        tRef.current = next
        setT(next)
      }
      id = requestAnimationFrame(tick)
    }
    id = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(id)
  }, [replay.horizon])

  function seek(v: number) {
    const clamped = Math.max(0, Math.min(replay.horizon, v))
    tRef.current = clamped
    lastRef.current = null
    setT(clamped)
  }

  function jumpViolation(dir: 1 | -1) {
    const v = nextViolation(replay, tRef.current, dir)
    if (!v) {
      return
    }
    setPlaying(false)
    seek(v.t)
    const later = v.robotId ? null : robotForJob(replay, v.jobId)
    setViolation({ v, later })
    const robot = v.robotId ?? later
    if (robot) {
      setSelected(robot)
    }
  }

  async function saveFrame() {
    const stage = stageRef.current
    if (!stage) {
      return
    }
    setFrameError(false)
    try {
      const counters = { queue: counterAt(replay.jobQueue, t), completed: counterAt(replay.completed, t), violated: counterAt(replay.violated, t) }
      const canvas = composeFrame(stage.toCanvas({ pixelRatio: frameRatio }), frameCaption(variantName, t, replay, counters), pal, frameRatio)
      triggerDownload(exportFileName(variantName, formatClock(t).replace(/:/g, '-'), 'png'), await canvasBlob(canvas))
    } catch {
      setFrameError(true)
    }
  }

  const poses = poseAt(replay, t)
  const sel = selected ? poses.find((p) => p.id === selected) : undefined
  const selTrack = selected ? replay.trackById.get(selected) : undefined

  return (
    <div className="replay">
      <div className="actions replay-controls" role="group" aria-label="Управление воспроизведением">
        <button
          type="button"
          onClick={() => {
            if (!playing && tRef.current >= replay.horizon) {
              seek(0)
            }
            setPlaying((p) => !p)
          }}
          aria-pressed={playing}
        >
          {playing ? 'Пауза' : t >= replay.horizon ? 'Сначала' : 'Пуск'}
        </button>
        <button
          type="button"
          onClick={() => {
            setPlaying(false)
            seek(0)
          }}
        >
          В начало
        </button>
        <label>
          Скорость
          <Select value={String(speed)} options={speedOptions} onChange={(v) => setSpeed(Number(v))} />
        </label>
        <button type="button" onClick={() => jumpViolation(-1)} disabled={!nextViolation(replay, t, -1)}>
          Предыдущее нарушение
        </button>
        <button type="button" onClick={() => jumpViolation(1)} disabled={!nextViolation(replay, t, 1)}>
          Следующее нарушение SLA
        </button>
        <label>
          Робот
          <Select
            value={selected ?? ''}
            options={[{ value: '', label: 'не выбран' }, ...replay.tracks.map((tr) => ({ value: tr.id, label: `${tr.id} ${tr.name}` }))]}
            onChange={(v) => setSelected(v || null)}
          />
        </label>
        <label className="check-row">
          <input type="checkbox" checked={showTrail} onChange={(e) => setShowTrail(e.target.checked)} />
          След
        </label>
        <button type="button" className="btn btn-text" onClick={() => void saveFrame()}>
          Сохранить кадр
        </button>
      </div>
      {frameError ? (
        <p className="error">
          <Reflow>Не удалось сохранить кадр. Обновите страницу и повторите.</Reflow>
        </p>
      ) : null}
      <Timeline replay={replay} t={t} onSeek={(v) => seek(v)} />
      <p className="replay-status">
        <Reflow>
          {formatClock(t)} из {formatClock(replay.horizon)}. Заданий в очереди {counterAt(replay.jobQueue, t)}, выполнено{' '}
          {counterAt(replay.completed, t)}, с нарушением SLA {counterAt(replay.violated, t)}.
          {log.truncated ? ' Журнал обрезан по лимиту событий, конец смены не показан.' : ''}
        </Reflow>
      </p>
      {violation ? (
        <p className="error">
          <Reflow>
            {formatClock(violation.v.t)}: задание {violation.v.jobId} ({processNames.get(violation.v.processCode) ?? violation.v.processCode}){' '}
            {violation.v.kind === 'wait' ? 'не назначено роботу за время SLA' : 'не выполнено за время SLA'}.
            {violation.v.robotId
              ? ` Его выполняет ${violation.v.robotId}.`
              : violation.later
                ? ` Свободного подходящего робота не было, позже задание взял ${violation.later}.`
                : ' Свободного подходящего робота не было до конца горизонта.'}
          </Reflow>
        </p>
      ) : null}
      <div className="replay-layout">
        <ReplayCanvas stageRef={stageRef} replay={replay} t={t} poses={poses} selected={selected} showTrail={showTrail} onSelect={setSelected} />
        <aside className="map-side">
          <ResourceBoard replay={replay} t={t} />
          {sel && selTrack ? (
            <div className="map-props">
              <h3>
                {sel.id}: {selTrack.name}
              </h3>
              <p>
                {selTrack.profile === 'pallet' ? 'Паллетный робот' : 'AMR'}, {robotStateLabels[sel.state]}
                {sel.resource ? ` (${sel.resource})` : ''}.
              </p>
              <p>
                <Reflow>
                  Задание {sel.jobId ?? 'нет'}. Груз {sel.loaded ? 'есть' : 'нет'}. Заряд {Math.round(sel.battery * 100)}%.
                  Выполнено заданий {jobsDoneAt(selTrack, t)}.
                </Reflow>
              </p>
            </div>
          ) : (
            <p className="field-hint"><Reflow>Щёлкните робота, чтобы увидеть его состояние и след.</Reflow></p>
          )}
          <Legend />
        </aside>
      </div>
    </div>
  )
}

function Legend() {
  const pal = useCanvasPalette()
  return (
    <ul className="legend">
      {robotStates.map((k) => (
        <li key={k}>
          <span className="swatch" style={{ background: stateColor(pal, k) }} />
          {robotStateLabels[k]}
        </li>
      ))}
      <li><Reflow>Прямоугольник с вилами: паллетный робот. Квадрат: AMR. Светлая вставка: груз.</Reflow></li>
    </ul>
  )
}

function ResourceBoard({ replay, t }: { replay: Replay; t: number }) {
  const auto = replay.log.resources.filter((r) => r.auto)
  let autoUsed = 0
  let autoQueue = 0
  for (const r of auto) {
    autoUsed += counterAt(replay.resourceUse.get(r.id), t)
    autoQueue += counterAt(replay.resourceQueue.get(r.id), t)
  }
  return (
    <div className="map-props">
      <h3>Ресурсы</h3>
      <table className="num-table">
        <thead>
          <tr>
            <th>Ресурс</th>
            <th>Занято</th>
            <th>Очередь</th>
          </tr>
        </thead>
        <tbody>
          {replay.log.resources.filter((r) => !r.auto).map((r) => {
            const used = counterAt(replay.resourceUse.get(r.id), t)
            const queue = counterAt(replay.resourceQueue.get(r.id), t)
            return (
              <tr key={r.id} className={queue > 0 ? 'row-alert' : undefined}>
                <td>{r.name}</td>
                <td>
                  {used} из {r.capacity}
                </td>
                <td>{queue}</td>
              </tr>
            )
          })}
          {auto.length > 0 ? (
            <tr className={autoQueue > 0 ? 'row-alert' : undefined}>
              <td>Однополосные участки ({auto.length})</td>
              <td>
                {autoUsed} из {auto.length}
              </td>
              <td>{autoQueue}</td>
            </tr>
          ) : null}
        </tbody>
      </table>
    </div>
  )
}

function Timeline({ replay, t, onSeek }: { replay: Replay; t: number; onSeek: (v: number) => void }) {
  const cps = replay.log.checkpoints
  const maxQueue = Math.max(1, ...cps.map((c) => c.queue))
  const h = replay.horizon || 1
  const pal = useCanvasPalette()
  const path = cps.map((c, i) => `${i === 0 ? 'M' : 'L'}${((c.t_s / h) * 1000).toFixed(1)},${(40 - (c.queue / maxQueue) * 36).toFixed(1)}`).join(' ')
  return (
    <div className="timeline">
      <svg viewBox="0 0 1000 44" preserveAspectRatio="none" aria-hidden="true" className="timeline-chart">
        <path d={path} fill="none" stroke={pal.path} strokeWidth="1.5" />
        {replay.violations.map((v, i) => (
          <line key={i} x1={(v.t / h) * 1000} x2={(v.t / h) * 1000} y1={0} y2={44} stroke={pal.danger} strokeWidth="1" opacity="0.5" />
        ))}
        <line x1={(t / h) * 1000} x2={(t / h) * 1000} y1={0} y2={44} stroke={pal.cursor} strokeWidth="2" />
      </svg>
      <input
        type="range"
        min={0}
        max={replay.horizon}
        step={1}
        value={Math.round(t)}
        aria-label="Время модели"
        aria-valuetext={formatClock(t)}
        onChange={(e) => onSeek(Number(e.target.value))}
      />
      <p className="field-hint">
        <Reflow>Синяя линия: очередь заданий (до {maxQueue}). Красные отметки: нарушения SLA.</Reflow>
      </p>
    </div>
  )
}

function RobotShape({ pal, pose, profile, lengthM, widthM, scale, selected }: {
  pal: CanvasPalette
  pose: Pose
  profile: 'amr' | 'pallet'
  lengthM: number
  widthM: number
  scale: number
  selected: boolean
}) {
  const minPx = 7 / scale
  const len = Math.max(lengthM, minPx)
  const wid = Math.max(widthM, minPx * 0.8)
  const color = stateColor(pal, pose.state)
  const rot = (pose.heading * 180) / Math.PI
  return (
    <Group x={pose.x} y={pose.y} rotation={rot}>
      {selected ? <Circle radius={len * 1.2} stroke={pal.select} strokeWidth={2 / scale} /> : null}
      {profile === 'pallet' ? (
        <>
          <Rect x={-len / 2} y={-wid / 2} width={len * 0.6} height={wid} fill={color} cornerRadius={wid * 0.1} />
          <Line points={[len * 0.1, -wid * 0.3, len / 2, -wid * 0.3]} stroke={color} strokeWidth={wid * 0.12} />
          <Line points={[len * 0.1, wid * 0.3, len / 2, wid * 0.3]} stroke={color} strokeWidth={wid * 0.12} />
          {pose.loaded ? <Rect x={len * 0.12} y={-wid * 0.42} width={len * 0.38} height={wid * 0.84} fill={pal.cargo} stroke={pal.dock} strokeWidth={1 / scale} /> : null}
        </>
      ) : (
        <>
          <Rect x={-len / 2} y={-wid / 2} width={len} height={wid} fill={color} cornerRadius={wid * 0.25} />
          <Line points={[len * 0.2, 0, len / 2, 0]} stroke={pal.halo} strokeWidth={wid * 0.12} />
          {pose.loaded ? <Rect x={-len * 0.3} y={-wid * 0.3} width={len * 0.5} height={wid * 0.6} fill={pal.cargo} /> : null}
        </>
      )}
      {pose.battery < 0.25 ? <Circle x={-len / 2} y={-wid / 2} radius={minPx * 0.35} fill={pal.select} /> : null}
    </Group>
  )
}

function ReplayCanvas({
  stageRef,
  replay,
  t,
  poses,
  selected,
  showTrail,
  onSelect,
}: {
  stageRef: RefObject<Konva.Stage | null>
  replay: Replay
  t: number
  poses: Pose[]
  selected: string | null
  showTrail: boolean
  onSelect: (id: string | null) => void
}) {
  const wrapRef = useRef<HTMLDivElement>(null)
  const pal = useCanvasPalette()
  const [width, setWidth] = useState(900)
  const height = 560
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
  const scene = replay.log.scene
  const pad = 12
  const scale = Math.min((width - 2 * pad) / Math.max(1, scene.width_m), (height - 2 * pad) / Math.max(1, scene.height_m))
  const offX = pad + (width - 2 * pad - scene.width_m * scale) / 2
  const offY = pad + (height - 2 * pad - scene.height_m * scale) / 2
  const nodes = replay.nodes
  const narrow = useMemo(() => {
    const s = new Set<string>()
    for (const r of replay.log.resources) {
      if (r.kind === 'narrow_aisle') {
        for (const e of r.edge_ids ?? []) {
          s.add(e)
        }
      }
    }
    return s
  }, [replay])
  const queueAt = useMemo(() => {
    const m = new Map<string, { id: string; kind: string }[]>()
    for (const r of replay.log.resources) {
      for (const n of r.node_ids ?? []) {
        m.set(n, [...(m.get(n) ?? []), { id: r.id, kind: r.kind }])
      }
    }
    return m
  }, [replay])
  const trail = selected && showTrail ? trailOf(replay, selected, t, trailWindowS) : []
  const flat = (pts: { x: number; y: number }[]) => pts.flatMap((p) => [p.x, p.y])

  return (
    <div ref={wrapRef} className="map-canvas">
      <Stage ref={stageRef} width={width} height={height} onClick={(e) => e.target === e.target.getStage() && onSelect(null)}>
        <Layer x={offX} y={offY} scaleX={scale} scaleY={scale} listening={false}>
          <Rect width={scene.width_m} height={scene.height_m} fill={pal.paper} />
          {scene.zones.map((z) => {
            const topLeft = z.ring.reduce(
              (position, point) => ({ x: Math.min(position.x, point.x), y: Math.min(position.y, point.y) }),
              { x: Number.POSITIVE_INFINITY, y: Number.POSITIVE_INFINITY },
            )
            return (
              <Group key={z.id}>
                <Line
                  points={flat(z.ring)}
                  closed
                  fill={zoneFill(pal, z.kind)}
                  stroke={z.kind === 'workspace' ? pal.ink : pal.lineFaint}
                  strokeWidth={(z.kind === 'workspace' ? 2 : 1) / scale}
                />
                {z.kind !== 'workspace' ? (
                  <Text
                    fontFamily={pal.font}
                    x={topLeft.x + 4 / scale}
                    y={topLeft.y + 4 / scale}
                    text={z.name || featureLabel(z)}
                    fontSize={10 / scale}
                    fill={pal.inkMuted}
                    listening={false}
                  />
                ) : null}
              </Group>
            )
          })}
          {scene.obstacles.map((o) => (
            <Line key={o.id} points={flat(o.ring)} closed fill={pal.obstacle} stroke={pal.obstacleEdge} strokeWidth={1 / scale} />
          ))}
          {replay.log.edges.map((e) => {
            const a = nodes.get(e.from)
            const b = nodes.get(e.to)
            if (!a || !b) {
              return null
            }
            const n = narrow.has(e.id)
            return (
              <Line
                key={e.id}
                points={[a.x, a.y, b.x, b.y]}
                stroke={n ? pal.warning : pal.pathFaint}
                strokeWidth={(n ? 3 : 1.5) / scale}
                dash={n ? [6 / scale, 4 / scale] : undefined}
              />
            )
          })}
          {replay.log.nodes
            .filter((n) => n.kind !== 'other')
            .map((n) => (
              <Group key={n.id}>
                <Rect
                  x={n.x - 4 / scale}
                  y={n.y - 4 / scale}
                  width={8 / scale}
                  height={8 / scale}
                  fill={n.kind === 'dock' ? pal.dock : n.kind === 'charger' ? pal.gate : pal.task}
                />
                <Text
                  fontFamily={pal.font}
                  x={n.x - 4 / scale}
                  y={n.y - 18 / scale}
                  text={featureLabel(n)}
                  fontSize={10 / scale}
                  fill={pal.ink}
                />
              </Group>
            ))}
          {[...queueAt.entries()].map(([nodeId, list]) => {
            const n = nodes.get(nodeId)
            if (!n) {
              return null
            }
            return list.map((r) => {
              const q = counterAt(replay.resourceQueue.get(r.id), t)
              return q > 0 ? (
                <Text fontFamily={pal.font} key={`${r.id}:${nodeId}`} x={n.x - 10 / scale} y={n.y + 8 / scale} text={`очередь ${q}`} fontSize={10 / scale} fill={pal.danger} />
              ) : null
            })
          })}
          {trail.length > 1 ? <Line points={flat(trail)} stroke={pal.select} strokeWidth={2 / scale} opacity={0.6} /> : null}
        </Layer>
        <Layer x={offX} y={offY} scaleX={scale} scaleY={scale}>
          {poses.map((p) => {
            const tr = replay.trackById.get(p.id)
            if (!tr) {
              return null
            }
            return (
              <Group key={p.id} onClick={() => onSelect(p.id)} onTap={() => onSelect(p.id)}>
                <RobotShape pal={pal} pose={p} profile={tr.profile} lengthM={tr.lengthM} widthM={tr.widthM} scale={scale} selected={p.id === selected} />
              </Group>
            )
          })}
        </Layer>
      </Stage>
    </div>
  )
}
