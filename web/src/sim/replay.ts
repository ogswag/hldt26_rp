import type { SimCheckpoint, SimEvent, SimulationLog } from '../api/client'

export type XY = { x: number; y: number }

export type RobotState = 'idle' | 'moving' | 'loaded' | 'handling' | 'waiting' | 'charging'

export const robotStateLabels: Record<RobotState, string> = {
  idle: 'свободен',
  moving: 'едет порожним',
  loaded: 'едет с грузом',
  handling: 'погрузка или выгрузка',
  waiting: 'ждёт ресурс',
  charging: 'заряжается',
}

type Leg = {
  t0: number
  t1: number
  pts: XY[]
  cum: number[]
  loaded: boolean
  jobId: string | null
}

type Mark = {
  t: number
  order: number
  state: RobotState
  jobId: string | null
  resource: string | null
}

type Sample = { t: number; v: number }

export type RobotTrack = {
  id: string
  name: string
  profile: 'amr' | 'pallet'
  widthM: number
  lengthM: number
  start: XY
  legs: Leg[]
  marks: Mark[]
  battery: Sample[]
  jobsDone: number[]
  legStarts: number[]
  markTimes: number[]
  batteryTimes: number[]
}

export type Violation = {
  t: number
  jobId: string
  robotId: string | null
  processCode: string
  kind: string
}

type Counter = { times: number[]; values: number[] }

export type Replay = {
  log: SimulationLog
  horizon: number
  nodes: Map<string, XY>
  tracks: RobotTrack[]
  trackById: Map<string, RobotTrack>
  violations: Violation[]
  checkpointTimes: number[]
  resourceUse: Map<string, Counter>
  resourceQueue: Map<string, Counter>
  jobQueue: Counter
  completed: Counter
  violated: Counter
}

export type Pose = {
  id: string
  x: number
  y: number
  heading: number
  state: RobotState
  loaded: boolean
  battery: number
  jobId: string | null
  resource: string | null
}

function counter(deltas: { t: number; d: number; order: number }[]): Counter {
  deltas.sort((a, b) => a.t - b.t || a.order - b.order)
  const times: number[] = []
  const values: number[] = []
  let v = 0
  for (const x of deltas) {
    v += x.d
    if (times.length > 0 && times[times.length - 1] === x.t) {
      values[values.length - 1] = v
    } else {
      times.push(x.t)
      values.push(v)
    }
  }
  return { times, values }
}

// lastIndexAtOrBefore returns the index of the last element <= t, or -1.
export function lastIndexAtOrBefore(times: number[], t: number): number {
  let lo = 0
  let hi = times.length - 1
  let ans = -1
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    if (times[mid] <= t) {
      ans = mid
      lo = mid + 1
    } else {
      hi = mid - 1
    }
  }
  return ans
}

export function counterAt(c: Counter | undefined, t: number): number {
  if (!c) {
    return 0
  }
  const i = lastIndexAtOrBefore(c.times, t)
  return i < 0 ? 0 : c.values[i]
}

function legOf(e: SimEvent, nodes: Map<string, XY>): Leg | null {
  const path = e.payload?.path ?? []
  const pts: XY[] = []
  for (const id of path) {
    const p = nodes.get(id)
    if (p) {
      pts.push(p)
    }
  }
  if (pts.length < 2) {
    return null
  }
  const cum = [0]
  for (let i = 1; i < pts.length; i++) {
    cum.push(cum[i - 1] + Math.hypot(pts[i].x - pts[i - 1].x, pts[i].y - pts[i - 1].y))
  }
  const dur = e.payload?.dur_s ?? 0
  return { t0: e.t_s, t1: e.t_s + dur, pts, cum, loaded: Boolean(e.payload?.loaded), jobId: e.job_id ?? null }
}

export function buildReplay(log: SimulationLog): Replay {
  const nodes = new Map(log.nodes.map((n) => [n.id, { x: n.x, y: n.y }]))
  const tracks: RobotTrack[] = log.robots.map((r) => ({
    id: r.id,
    name: r.name,
    profile: r.profile,
    widthM: r.width_m,
    lengthM: r.length_m,
    start: nodes.get(r.start_id) ?? { x: 0, y: 0 },
    legs: [],
    marks: [{ t: 0, order: -1, state: 'idle', jobId: null, resource: null }],
    battery: [{ t: 0, v: r.battery }],
    jobsDone: [],
    legStarts: [],
    markTimes: [],
    batteryTimes: [],
  }))
  const byId = new Map(tracks.map((t) => [t.id, t]))
  const violations: Violation[] = []
  const violatedJobs = new Set<string>()
  const use = new Map<string, { t: number; d: number; order: number }[]>()
  const queue = new Map<string, { t: number; d: number; order: number }[]>()
  const jobs: { t: number; d: number; order: number }[] = []
  const done: { t: number; d: number; order: number }[] = []
  const viol: { t: number; d: number; order: number }[] = []
  const push = (m: Map<string, { t: number; d: number; order: number }[]>, id: string, t: number, d: number, order: number) => {
    const list = m.get(id)
    if (list) {
      list.push({ t, d, order })
    } else {
      m.set(id, [{ t, d, order }])
    }
  }
  for (const e of log.events) {
    const tr = e.robot_id ? byId.get(e.robot_id) : undefined
    const job = e.job_id ?? null
    const mark = (t: number, state: RobotState, jobId: string | null, resource: string | null = null, order = e.seq) => {
      tr?.marks.push({ t, order, state, jobId, resource })
    }
    const battery = e.payload?.battery
    if (tr && typeof battery === 'number') {
      tr.battery.push({ t: e.t_s, v: battery })
    }
    switch (e.type) {
      case 'job_arrival':
        jobs.push({ t: e.t_s, d: 1, order: e.seq })
        break
      case 'dispatch':
        jobs.push({ t: e.t_s, d: -1, order: e.seq })
        mark(e.t_s, 'moving', job)
        break
      case 'route_start': {
        const leg = legOf(e, nodes)
        if (tr && leg) {
          tr.legs.push(leg)
          mark(e.t_s, leg.loaded ? 'loaded' : 'moving', job)
        }
        break
      }
      case 'route_end':
        mark(e.t_s, 'waiting', job)
        break
      case 'resource_acquire': {
        const wait = e.payload?.wait_s ?? 0
        if (e.resource_id) {
          push(use, e.resource_id, e.t_s, 1, e.seq)
          if (wait > 0) {
            push(queue, e.resource_id, e.t_s - wait, 1, e.seq - 0.5)
            push(queue, e.resource_id, e.t_s, -1, e.seq)
            mark(e.t_s - wait, 'waiting', job, e.resource_id, e.seq - 0.5)
          }
        }
        break
      }
      case 'resource_release':
        if (e.resource_id) {
          push(use, e.resource_id, e.t_s, -1, e.seq)
        }
        break
      case 'load':
        mark(e.t_s, 'handling', job)
        break
      case 'unload': {
        mark(e.t_s, 'handling', job)
        const end = e.t_s + (e.payload?.dur_s ?? 0)
        mark(end, 'idle', null, null, e.seq + 0.25)
        tr?.jobsDone.push(end)
        done.push({ t: end, d: 1, order: e.seq })
        break
      }
      case 'charge_start':
        mark(e.t_s, 'charging', null, e.resource_id ?? null)
        break
      case 'charge_end':
        mark(e.t_s, 'idle', null)
        break
      case 'sla_violated':
        violations.push({ t: e.t_s, jobId: job ?? '', robotId: e.robot_id ?? null, processCode: e.process_code ?? '', kind: e.payload?.kind ?? '' })
        if (job && !violatedJobs.has(job)) {
          violatedJobs.add(job)
          viol.push({ t: e.t_s, d: 1, order: e.seq })
        }
        break
      default:
        break
    }
  }
  for (const tr of tracks) {
    tr.marks.sort((a, b) => a.t - b.t || a.order - b.order)
    tr.battery.sort((a, b) => a.t - b.t)
    tr.legs.sort((a, b) => a.t0 - b.t0)
    tr.jobsDone.sort((a, b) => a - b)
    tr.legStarts = tr.legs.map((l) => l.t0)
    tr.markTimes = tr.marks.map((m) => m.t)
    tr.batteryTimes = tr.battery.map((b) => b.t)
  }
  const resourceUse = new Map<string, Counter>()
  const resourceQueue = new Map<string, Counter>()
  for (const [id, list] of use) {
    resourceUse.set(id, counter(list))
  }
  for (const [id, list] of queue) {
    resourceQueue.set(id, counter(list))
  }
  return {
    log,
    horizon: log.horizon_s,
    nodes,
    tracks,
    trackById: byId,
    violations,
    checkpointTimes: log.checkpoints.map((c) => c.t_s),
    resourceUse,
    resourceQueue,
    jobQueue: counter(jobs),
    completed: counter(done),
    violated: counter(viol),
  }
}

function pointOnLeg(leg: Leg, t: number): { p: XY; heading: number } {
  const total = leg.cum[leg.cum.length - 1]
  const span = leg.t1 - leg.t0
  const u = span > 0 ? Math.min(1, Math.max(0, (t - leg.t0) / span)) : 1
  const d = u * total
  let i = 1
  while (i < leg.cum.length - 1 && leg.cum[i] < d) {
    i++
  }
  const a = leg.pts[i - 1]
  const b = leg.pts[i]
  const segLen = leg.cum[i] - leg.cum[i - 1]
  const k = segLen > 0 ? (d - leg.cum[i - 1]) / segLen : 1
  return {
    p: { x: a.x + (b.x - a.x) * k, y: a.y + (b.y - a.y) * k },
    heading: Math.atan2(b.y - a.y, b.x - a.x),
  }
}

function batteryAt(samples: Sample[], times: number[], t: number): number {
  const i = lastIndexAtOrBefore(times, t)
  if (i < 0) {
    return samples[0]?.v ?? 1
  }
  const a = samples[i]
  const b = samples[i + 1]
  if (!b || b.t === a.t) {
    return a.v
  }
  return a.v + ((b.v - a.v) * (t - a.t)) / (b.t - a.t)
}

export function poseOf(tr: RobotTrack, t: number): Pose {
  const legIdx = lastIndexAtOrBefore(tr.legStarts, t)
  let p = tr.start
  let heading = 0
  let moving: Leg | null = null
  if (legIdx >= 0) {
    const leg = tr.legs[legIdx]
    const at = pointOnLeg(leg, t)
    p = at.p
    heading = at.heading
    if (t < leg.t1) {
      moving = leg
    }
  }
  const markIdx = lastIndexAtOrBefore(tr.markTimes, t)
  const mark = tr.marks[Math.max(0, markIdx)]
  let state = mark.state
  if (moving) {
    state = moving.loaded ? 'loaded' : 'moving'
  } else if (state === 'moving' || state === 'loaded') {
    state = 'waiting'
  }
  const last = legIdx >= 0 ? tr.legs[legIdx] : null
  const carrying = moving
    ? moving.loaded
    : Boolean(last?.loaded) && last?.jobId === mark.jobId && (state === 'waiting' || state === 'handling')
  return {
    id: tr.id,
    x: p.x,
    y: p.y,
    heading,
    state,
    loaded: carrying,
    battery: Math.max(0, Math.min(1, batteryAt(tr.battery, tr.batteryTimes, t))),
    jobId: mark.jobId,
    resource: mark.resource,
  }
}

export function poseAt(r: Replay, t: number): Pose[] {
  return r.tracks.map((tr) => poseOf(tr, t))
}

// trailOf samples the robot path over the last windowS seconds.
export function trailOf(r: Replay, robotId: string, t: number, windowS: number, stepS = 2): XY[] {
  const tr = r.trackById.get(robotId)
  if (!tr) {
    return []
  }
  const out: XY[] = []
  const from = Math.max(0, t - windowS)
  for (let s = from; s < t; s += stepS) {
    const p = poseOf(tr, s)
    const last = out[out.length - 1]
    if (!last || last.x !== p.x || last.y !== p.y) {
      out.push({ x: p.x, y: p.y })
    }
  }
  const now = poseOf(tr, t)
  out.push({ x: now.x, y: now.y })
  return out
}

export function jobsDoneAt(tr: RobotTrack, t: number): number {
  return lastIndexAtOrBefore(tr.jobsDone, t) + 1
}

export function checkpointAt(r: Replay, t: number): SimCheckpoint | null {
  const i = lastIndexAtOrBefore(r.checkpointTimes, t)
  return i < 0 ? null : r.log.checkpoints[i]
}

// nextViolation finds the first violation strictly after t (dir 1) or before t (dir -1).
export function nextViolation(r: Replay, t: number, dir: 1 | -1): Violation | null {
  const eps = 0.05
  if (dir > 0) {
    return r.violations.find((v) => v.t > t + eps) ?? null
  }
  for (let i = r.violations.length - 1; i >= 0; i--) {
    if (r.violations[i].t < t - eps) {
      return r.violations[i]
    }
  }
  return null
}

// robotForJob returns the robot serving a job, looked up from dispatch events.
export function robotForJob(r: Replay, jobId: string): string | null {
  const e = r.log.events.find((x) => x.type === 'dispatch' && x.job_id === jobId)
  return e?.robot_id ?? null
}

export function formatClock(s: number): string {
  const total = Math.max(0, Math.floor(s))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const sec = total % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(h)}:${pad(m)}:${pad(sec)}`
}
