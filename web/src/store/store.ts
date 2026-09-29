// ProjectStore holds one project's records for the pages. Pages read the view and change it only through
// dispatch; sync.ts moves transactions to the server and the server's journal back.

import { apply, compileSchema, type Change, type Details, type Hooks, type Outcome, type RawOp, type Reason, type Schema, type SchemaSpec, type State } from './apply'
import { uuid } from './clientId'
import spec from './project.schema.json'
import { permissive, view, type Pending } from './state'

export type Rejected = {
  txId: string
  label: string
  reason: Reason
  details?: Details
  at: number
  intended: Change[]
  // actor and actorEmail name who deleted the record a refused change was about.
  actor?: string
  actorEmail?: string
}

export type JournalEntry = { seq: number; tx_id: string; actor?: string | null; actor_email?: string | null; ops: RawOp[] }

export type TxResult =
  | { tx_id: string; status: 'applied'; seq: number }
  | { tx_id: string; status: 'rejected'; reason: Reason; details?: Details }

export type Dispatched = { txId: string; outcome: Outcome; changes: Change[] }

let compiled: Schema | null = null

export function projectSchema(): Schema {
  compiled ??= compileSchema(spec as SchemaSpec)
  return compiled
}

export class ProjectStore {
  readonly projectId: string
  private readonly schema: Schema
  private readonly hooks: Hooks
  private confirmed: State = {}
  private seq = -1
  private pending: Pending[] = []
  private rejected: Rejected[] = []
  private current: State = {}
  private readOnly = false
  private readonly deletedBy = new Map<string, { id: string; email: string | null }>()
  private readonly listeners = new Set<() => void>()
  // onQueue runs after a local transaction joins the queue, so sync can send it.
  onQueue: () => void = () => {}

  constructor(projectId: string, hooks: Hooks = permissive, schema: Schema = projectSchema()) {
    this.projectId = projectId
    this.schema = schema
    this.hooks = hooks
  }

  // ready is false until the first snapshot arrives.
  get ready(): boolean {
    return this.seq >= 0
  }

  getState = (): State => this.current

  getSeq(): number {
    return this.seq
  }

  getConfirmed(): State {
    return this.confirmed
  }

  getPending(): readonly Pending[] {
    return this.pending
  }

  getRejected(): readonly Rejected[] {
    return this.rejected
  }

  setReadOnly(value: boolean): void {
    if (this.readOnly === value) {
      return
    }
    this.readOnly = value
    if (value) {
      this.refuseAll('forbidden')
    }
  }

  subscribe = (fn: () => void): (() => void) => {
    this.listeners.add(fn)
    return () => {
      this.listeners.delete(fn)
    }
  }

  private emit(): void {
    for (const fn of this.listeners) {
      fn()
    }
  }

  private recompute(): void {
    this.current = view(this.schema, this.confirmed, this.pending, this.hooks)
    this.emit()
  }

  // dispatch applies one user action at once. A transaction the schema refuses here is not queued: the caller
  // shows the outcome right away.
  dispatch(label: string, ops: RawOp[]): Dispatched {
    if (this.readOnly) {
      const outcome: Outcome = { status: 'rejected', reason: 'forbidden', details: { op: 0 } }
      return { txId: uuid(), outcome, changes: [] }
    }
    const tx = { tx_id: uuid(), label, ops }
    const r = apply(this.schema, this.current, tx, this.hooks)
    if (r.outcome.status === 'applied') {
      this.pending = [...this.pending, { tx, intended: r.changes, createdAt: Date.now() }]
      this.current = r.state
      this.emit()
      this.onQueue()
    }
    return { txId: tx.tx_id, outcome: r.outcome, changes: r.changes }
  }

  // refuse lists an action that could not apply here, such as an undo whose target someone deleted. intended
  // holds the records it meant to write.
  refuse(label: string, reason: Reason, details: Details | undefined, intended: Change[]): void {
    this.rejected = [...this.rejected, { txId: uuid(), label, reason, details, at: Date.now(), intended }]
    this.emit()
  }

  // restore puts back the queue and refusals saved before a reload.
  restore(pending: Pending[], rejected: Rejected[]): void {
    this.pending = pending
    this.rejected = rejected
    this.recompute()
  }

  // adopt takes over the queue and the refused list of a tab of the same person that closed before it finished
  // sending. Duplicates are dropped by tx_id; the server would refuse them anyway, but the queue should not carry
  // them. It returns how many transactions it took.
  adopt(pending: readonly Pending[], rejected: readonly Rejected[] = []): number {
    const known = new Set(this.pending.map((p) => p.tx.tx_id))
    const taken = pending.filter((p) => !known.has(p.tx.tx_id))
    const seen = new Set(this.rejected.map((r) => r.txId))
    const refused = rejected.filter((r) => !seen.has(r.txId))
    if (taken.length === 0 && refused.length === 0) {
      return 0
    }
    // Another tab's transactions were answered against its own queue, so what it recorded as accepted is not
    // this tab's seq. They go back unanswered: tx_id makes sending one twice safe.
    this.pending = [...this.pending, ...taken.map((p) => ({ ...p, ackSeq: undefined }))]
    this.rejected = [...this.rejected, ...refused]
    this.recompute()
    return taken.length
  }

  // loadSnapshot replaces what the server confirmed. Pending transactions the snapshot already contains leave.
  loadSnapshot(seq: number, collections: State): void {
    this.confirmed = collections
    this.seq = seq
    this.pending = this.pending.filter((p) => p.ackSeq === undefined || p.ackSeq > seq)
    this.recompute()
  }

  // applyJournal appends accepted transactions in seq order. It returns false when an entry does not follow the
  // current seq or does not apply: the caller takes a snapshot instead.
  applyJournal(entries: readonly JournalEntry[]): boolean {
    let st = this.confirmed
    let seq = this.seq
    const done = new Set<string>()
    for (const e of entries) {
      if (e.seq <= seq) {
        continue
      }
      if (e.seq !== seq + 1) {
        return false
      }
      const r = apply(this.schema, st, { tx_id: e.tx_id, ops: e.ops }, permissive)
      if (r.outcome.status !== 'applied') {
        return false
      }
      st = r.state
      seq = e.seq
      done.add(e.tx_id)
      if (e.actor) {
        for (const op of e.ops) {
          if (op.op === 'delete' && op.coll && op.id) {
            this.deletedBy.set(`${op.coll}:${op.id}`, { id: e.actor, email: e.actor_email ?? null })
          }
        }
      }
    }
    if (seq === this.seq) {
      return true
    }
    this.confirmed = st
    this.seq = seq
    this.pending = this.pending.filter((p) => !done.has(p.tx.tx_id) && (p.ackSeq === undefined || p.ackSeq > seq))
    this.recompute()
    return true
  }

  // settle records the server's answers to a batch.
  settle(results: readonly TxResult[]): void {
    let changed = false
    for (const res of results) {
      const i = this.pending.findIndex((p) => p.tx.tx_id === res.tx_id)
      if (i < 0) {
        continue
      }
      const p = this.pending[i]
      if (res.status === 'applied') {
        if (res.seq <= this.seq) {
          this.pending = this.pending.filter((_, j) => j !== i)
          changed = true
        } else if (p.ackSeq !== res.seq) {
          this.pending = this.pending.map((q, j) => (j === i ? { ...q, ackSeq: res.seq } : q))
          changed = true
        }
        continue
      }
      this.pending = this.pending.filter((_, j) => j !== i)
      const by = res.details?.coll && res.details.id ? this.deletedBy.get(`${res.details.coll}:${res.details.id}`) : undefined
      this.rejected = [
        ...this.rejected,
        {
          txId: res.tx_id,
          label: p.tx.label ?? '',
          reason: res.reason,
          details: res.details,
          at: Date.now(),
          intended: p.intended,
          ...(by ? { actor: by.id, ...(by.email ? { actorEmail: by.email } : {}) } : {}),
        },
      ]
      changed = true
    }
    if (changed) {
      this.recompute()
    }
  }

  // unsent lists the transactions the server has not answered yet, oldest first.
  unsent(): Pending[] {
    return this.pending.filter((p) => p.ackSeq === undefined)
  }

  // whenSent resolves once the server has answered every queued transaction, or after timeoutMs: offline, the
  // caller goes on with what the server already has.
  whenSent(timeoutMs: number): Promise<void> {
    if (this.unsent().length === 0) {
      return Promise.resolve()
    }
    return new Promise((resolve) => {
      const done = () => {
        clearTimeout(timer)
        off()
        resolve()
      }
      const timer = setTimeout(done, timeoutMs)
      const off = this.subscribe(() => {
        if (this.unsent().length === 0) {
          done()
        }
      })
    })
  }

  // refuseAll moves every waiting transaction to the refused list, when the project is gone or read-only.
  refuseAll(reason: Reason): void {
    const now = Date.now()
    const moved = this.pending.filter((p) => p.ackSeq === undefined)
    if (moved.length === 0) {
      return
    }
    this.rejected = [
      ...this.rejected,
      ...moved.map((p) => ({ txId: p.tx.tx_id, label: p.tx.label ?? '', reason, at: now, intended: p.intended })),
    ]
    this.pending = this.pending.filter((p) => p.ackSeq !== undefined)
    this.recompute()
  }

  dismiss(txId: string): void {
    const next = this.rejected.filter((r) => r.txId !== txId)
    if (next.length !== this.rejected.length) {
      this.rejected = next
      this.emit()
    }
  }
}
