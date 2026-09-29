import { describe, expect, it } from 'vitest'

import type { SimEvent, SimulationLog } from '../api/client'

import {
  buildReplay,
  checkpointAt,
  counterAt,
  formatClock,
  jobsDoneAt,
  lastIndexAtOrBefore,
  nextViolation,
  poseAt,
  robotForJob,
  trailOf,
} from './replay'

let seq = 0
function ev(t: number, type: SimEvent['type'], extra: Partial<SimEvent> = {}): SimEvent {
  return { seq: seq++, t_s: t, type, ...extra }
}

function log(): SimulationLog {
  seq = 0
  const events: SimEvent[] = [
    ev(0, 'job_arrival', { job_id: 'J1', process_code: 'inbound', from_id: 'A', to_id: 'C' }),
    ev(0, 'dispatch', { job_id: 'J1', robot_id: 'R1', from_id: 'S', to_id: 'A' }),
    ev(0, 'route_start', { robot_id: 'R1', job_id: 'J1', from_id: 'S', to_id: 'A', payload: { path: ['S', 'A'], dur_s: 10, battery: 1 } }),
    ev(10, 'route_end', { robot_id: 'R1', job_id: 'J1', payload: { battery: 0.9 } }),
    ev(10, 'job_arrival', { job_id: 'J2', process_code: 'inbound' }),
    ev(15, 'resource_acquire', { robot_id: 'R1', job_id: 'J1', resource_id: 'dock', payload: { wait_s: 5 } }),
    ev(15, 'load', { robot_id: 'R1', job_id: 'J1', payload: { dur_s: 5 } }),
    ev(20, 'resource_release', { robot_id: 'R1', job_id: 'J1', resource_id: 'dock' }),
    ev(20, 'route_start', { robot_id: 'R1', job_id: 'J1', from_id: 'A', to_id: 'C', payload: { path: ['A', 'B', 'C'], dur_s: 20, loaded: true, battery: 0.88 } }),
    ev(25, 'sla_violated', { job_id: 'J2', process_code: 'inbound', payload: { kind: 'wait' } }),
    ev(40, 'route_end', { robot_id: 'R1', job_id: 'J1', payload: { battery: 0.8 } }),
    ev(40, 'unload', { robot_id: 'R1', job_id: 'J1', payload: { dur_s: 10 } }),
    ev(50, 'sla_reached', { robot_id: 'R1', job_id: 'J1' }),
    ev(50, 'dispatch', { job_id: 'J2', robot_id: 'R1' }),
    ev(55, 'sla_violated', { job_id: 'J2', robot_id: 'R1', process_code: 'inbound', payload: { kind: 'cycle' } }),
  ]
  return {
    schema_version: 'sim-log-v1',
    event_schema: 'sim-event-v1',
    sim_version: 'sim-v2',
    replication: 0,
    seed: 0,
    horizon_s: 60,
    scene: { width_m: 30, height_m: 20, zones: [], obstacles: [] },
    nodes: [
      { id: 'S', kind: 'charger', x: 0, y: 10 },
      { id: 'A', kind: 'dock', x: 10, y: 10 },
      { id: 'B', kind: 'other', x: 20, y: 10 },
      { id: 'C', kind: 'task', x: 20, y: 0 },
    ],
    edges: [],
    resources: [{ id: 'dock', kind: 'dock', name: 'Док', capacity: 1 }],
    robots: [{ id: 'R1', name: 'AMR', profile: 'amr', width_m: 0.7, length_m: 1, fleet_key: 'f', start_id: 'S', battery: 1 }],
    processes: [{ code: 'inbound', name: 'Приёмка', covered: true, priority: 0 }],
    events,
    checkpoints: [
      { t_s: 0, queue: 0, busy: 0, idle: 1, charging: 0, completed: 0, violated: 0, processes: [0], resources: [] },
      { t_s: 60, queue: 0, busy: 1, idle: 0, charging: 0, completed: 1, violated: 1, processes: [0], resources: [] },
    ],
    truncated: false,
  }
}

describe('replay', () => {
  const r = buildReplay(log())

  it('interpolates along multi-segment paths', () => {
    const [p5] = poseAt(r, 5)
    expect(p5.x).toBeCloseTo(5)
    expect(p5.state).toBe('moving')
    const [p30] = poseAt(r, 30)
    expect(p30.x).toBeCloseTo(20)
    expect(p30.y).toBeCloseTo(10)
    expect(p30.state).toBe('loaded')
    expect(p30.loaded).toBe(true)
    const [p35] = poseAt(r, 35)
    expect(p35.y).toBeCloseTo(5)
    expect(p35.heading).toBeCloseTo(-Math.PI / 2)
  })

  it('derives waiting, handling and idle states', () => {
    expect(poseAt(r, 12)[0].state).toBe('waiting')
    expect(poseAt(r, 12)[0].resource).toBe('dock')
    expect(poseAt(r, 16)[0].state).toBe('handling')
    expect(poseAt(r, 16)[0].loaded).toBe(false)
    expect(poseAt(r, 45)[0].state).toBe('handling')
    expect(poseAt(r, 45)[0].loaded).toBe(true)
    expect(poseAt(r, 50)[0].jobId).toBe('J2')
    expect(poseAt(r, 50)[0].loaded).toBe(false)
    expect(poseAt(r, 5)[0].battery).toBeCloseTo(0.95)
  })

  it('tracks queues, completions and violations', () => {
    expect(counterAt(r.jobQueue, 12)).toBe(1)
    expect(counterAt(r.jobQueue, 51)).toBe(0)
    expect(counterAt(r.resourceQueue.get('dock'), 12)).toBe(1)
    expect(counterAt(r.resourceQueue.get('dock'), 15)).toBe(0)
    expect(counterAt(r.resourceUse.get('dock'), 17)).toBe(1)
    expect(counterAt(r.resourceUse.get('dock'), 21)).toBe(0)
    expect(counterAt(r.completed, 49)).toBe(0)
    expect(counterAt(r.completed, 50)).toBe(1)
    expect(counterAt(r.violated, 60)).toBe(1)
    expect(jobsDoneAt(r.tracks[0], 50)).toBe(1)
    expect(nextViolation(r, 0, 1)?.t).toBe(25)
    expect(nextViolation(r, 25, 1)?.t).toBe(55)
    expect(nextViolation(r, 55, 1)).toBeNull()
    expect(nextViolation(r, 55, -1)?.t).toBe(25)
    expect(robotForJob(r, 'J2')).toBe('R1')
    expect(checkpointAt(r, 59)?.t_s).toBe(0)
  })

  it('samples a trail and formats time', () => {
    const trail = trailOf(r, 'R1', 30, 30)
    expect(trail[0]).toEqual({ x: 0, y: 10 })
    expect(trail[trail.length - 1].x).toBeCloseTo(20)
    expect(trailOf(r, 'nope', 10, 10)).toEqual([])
    expect(formatClock(3725.9)).toBe('01:02:05')
    expect(lastIndexAtOrBefore([1, 2, 2, 5], 2)).toBe(2)
    expect(lastIndexAtOrBefore([1, 2], 0)).toBe(-1)
  })
})
