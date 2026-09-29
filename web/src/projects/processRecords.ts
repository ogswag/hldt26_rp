// The project's base processes in the store. processesFromState builds what the page draws; the edit builders
// turn one change into one transaction. A process code is immutable in the schema, because the fleet and the
// map flows refer to it by code.

import type { Process, ProcessDemand, ProcessDurations, ProcessSLA, ProcessStaff } from '../api/client'
import { equal, type RawOp, type Rec, type State } from '../store/apply'
import { uuid } from '../store/clientId'
import { keyBetween } from '../store/order'
import type { ProcessesRecord } from '../store/schema.gen'

export type Edit = { ops: RawOp[]; label: string }

function recs(st: State): Record<string, ProcessesRecord> {
  return (st.processes ?? {}) as Record<string, ProcessesRecord>
}

function byOrder(list: ProcessesRecord[]): ProcessesRecord[] {
  return [...list].sort((a, b) => (a.order === b.order ? (a.id < b.id ? -1 : 1) : a.order < b.order ? -1 : 1))
}

// processesFromState returns the processes in the shape the page and the calculation use.
export function processesFromState(st: State): Process[] {
  return byOrder(Object.values(recs(st))).map((p, i) => ({
    id: p.id,
    code: p.code,
    name: p.name,
    task_type: p.task_type,
    is_baseline: p.is_baseline,
    demand: (p.demand ?? {}) as ProcessDemand,
    sla: (p.sla ?? {}) as ProcessSLA,
    point_ids: p.point_ids ?? [],
    durations: (p.durations ?? {}) as ProcessDurations,
    baseline_staff: (p.baseline_staff ?? {}) as ProcessStaff,
    sort_order: i,
  }))
}

// processesSelector rebuilds the list only when the collection changed.
export function processesSelector(): (st: State) => Process[] {
  let ref: unknown = null
  let out: Process[] = []
  return (st) => {
    if (st.processes !== ref) {
      ref = st.processes
      out = processesFromState(st)
    }
    return out
  }
}

function lastOrder(st: State): string | null {
  let last: string | null = null
  for (const r of Object.values(recs(st))) {
    if (last === null || r.order > last) {
      last = r.order
    }
  }
  return last
}

const editable = ['name', 'task_type', 'is_baseline', 'demand', 'sla', 'durations', 'baseline_staff'] as const

export type ProcessPatch = Partial<Pick<Process, (typeof editable)[number]>>

// processFieldEdit writes the fields of one process. The code is not among them: the schema refuses to change
// it, and the fleet and the map flows point at it.
export function processFieldEdit(st: State, id: string, patch: ProcessPatch): Edit {
  const rec = recs(st)[id]
  if (!rec) {
    return { ops: [], label: 'Изменить процесс' }
  }
  const ops: RawOp[] = []
  for (const [path, value] of Object.entries(patch)) {
    if (!equal((rec as unknown as Rec)[path], value)) {
      ops.push({ op: 'set', coll: 'processes', id, path, value: value as Rec[string] })
    }
  }
  return { ops, label: 'Изменить процесс' }
}

// addProcessEdit puts a new process at the end of the list.
export function addProcessEdit(st: State, p: Process): Edit {
  const value: Rec = {
    order: keyBetween(lastOrder(st), null),
    code: p.code,
    name: p.name,
    task_type: p.task_type,
    is_baseline: p.is_baseline,
    demand: p.demand,
    sla: p.sla,
    point_ids: p.point_ids ?? [],
    durations: p.durations,
    baseline_staff: p.baseline_staff,
  }
  return { ops: [{ op: 'insert', coll: 'processes', id: p.id ?? uuid(), value }], label: 'Добавить процесс' }
}

// deleteProcessEdit removes a process. The schema takes care of what pointed at it.
export function deleteProcessEdit(id: string): Edit {
  return { ops: [{ op: 'delete', coll: 'processes', id }], label: 'Удалить процесс' }
}
