// Syncer moves a store's queue to the server and the server's journal into the store. The transport is the
// HTTP API in the app and a fake server in tests.

import type { State, Tx } from './apply'
import { GuestOwner, type Persistence } from './persist'
import * as queueLock from './queueLock'
import { schemaVersion } from './schema.gen'
import type { JournalEntry, ProjectStore, TxResult } from './store'

export type SnapshotResponse = { schema_version: number; seq: number; collections: State }
export type OperationsPage = { items: JournalEntry[]; seq: number; reset: boolean }
export type BatchResponse = { results: TxResult[]; seq: number }
export type BatchBody = { client_id: string; schema_version: number; txs: Tx[] }

export type Transport = {
  snapshot(projectId: string): Promise<SnapshotResponse>
  operations(projectId: string, after: number): Promise<OperationsPage>
  transactions(projectId: string, body: BatchBody): Promise<BatchResponse>
}

// network, server and auth failures are retried; the others stop sending.
export type SyncErrorKind = 'network' | 'server' | 'auth' | 'too_large' | 'schema' | 'forbidden' | 'gone'

export class SyncError extends Error {
  readonly kind: SyncErrorKind
  constructor(kind: SyncErrorKind, message: string) {
    super(message)
    this.kind = kind
  }
}

export type SyncPhase = 'loading' | 'saved' | 'sending' | 'offline' | 'error'
export type SyncStatus = { phase: SyncPhase; error?: SyncErrorKind }

export type Clock = {
  setTimeout(fn: () => void, ms: number): unknown
  clearTimeout(handle: unknown): void
}

const realClock: Clock = {
  setTimeout: (fn, ms) => setTimeout(fn, ms),
  clearTimeout: (h) => clearTimeout(h as ReturnType<typeof setTimeout>),
}

export type SyncOptions = {
  clientId: string
  // owner is the signed-in user's id, or GuestOwner. A queue never crosses from one person to another.
  owner?: string
  persistence?: Persistence | null
  clock?: Clock
  online?: () => boolean
  // onConfirmed runs after the confirmed state moved, for pages still on React Query.
  onConfirmed?: () => void
  // onAdopted reports how many transactions came from a tab that closed before it finished sending.
  onAdopted?: (count: number) => void
}

const PAGE = 500
const MAX_BATCH = 200
const MAX_BYTES = 900_000
const MIN_RETRY = 1000
const MAX_RETRY = 60_000
const SNAPSHOT_SAVE_DELAY = 1000

export class Syncer {
  private readonly store: ProjectStore
  private readonly transport: Transport
  private readonly opts: SyncOptions
  private readonly clock: Clock
  private status: SyncStatus = { phase: 'loading' }
  private readonly listeners = new Set<() => void>()
  private sending: Promise<void> | null = null
  private catching: Promise<void> | null = null
  private catchAgain = false
  private retryTimer: unknown = null
  private retryDelay = MIN_RETRY
  private snapshotTimer: unknown = null
  private batchLimit = MAX_BATCH
  private stopped = false
  private blocked: SyncErrorKind | null = null
  private blocks = 0
  private unsubscribe: () => void = () => {}

  constructor(store: ProjectStore, transport: Transport, opts: SyncOptions) {
    this.store = store
    this.transport = transport
    this.opts = opts
    this.clock = opts.clock ?? realClock
  }

  private owner(): string {
    return this.opts.owner ?? GuestOwner
  }

  getStatus = (): SyncStatus => this.status

  subscribeStatus = (fn: () => void): (() => void) => {
    this.listeners.add(fn)
    return () => {
      this.listeners.delete(fn)
    }
  }

  private setStatus(next: SyncStatus): void {
    if (next.phase === this.status.phase && next.error === this.status.error) {
      return
    }
    this.status = next
    for (const fn of this.listeners) {
      fn()
    }
  }

  private online(): boolean {
    return this.opts.online ? this.opts.online() : true
  }

  // start restores what the last visit saved, catches up with the server and sends the queue.
  async start(): Promise<void> {
    const p = this.opts.persistence
    if (p) {
      const saved = await p.load(this.store.projectId, this.owner(), this.opts.clientId).catch(() => null)
      if (saved?.snapshot && saved.snapshot.schemaVersion > 0 && saved.snapshot.schemaVersion <= schemaVersion) {
        this.store.loadSnapshot(saved.snapshot.seq, saved.snapshot.confirmed)
      }
      if (saved) {
        this.store.restore(saved.pending, saved.rejected)
      }
    }
    queueLock.hold(this.store.projectId, this.owner(), this.opts.clientId)
    this.watchStore()
    this.store.onQueue = () => void this.flush()
    await this.adoptOrphans()
    await this.catchUp()
    await this.flush()
  }

  // adoptOrphans takes over the queues and refused lists of this person's tabs that closed before they finished sending. A queue
  // whose Web Lock is still held belongs to a tab that is open, and is left alone.
  private async adoptOrphans(): Promise<void> {
    const p = this.opts.persistence
    if (!p || !queueLock.available()) {
      return
    }
    const owner = this.owner()
    const pid = this.store.projectId
    const queues = await p.listQueues(pid, owner).catch(() => [])
    let taken = 0
    for (const q of queues) {
      if (q.clientId === this.opts.clientId || (q.pending.length === 0 && q.rejected.length === 0)) {
        continue
      }
      await queueLock
        .claim(pid, owner, q.clientId, async () => {
          taken += this.store.adopt(q.pending, q.rejected)
          await p.saveQueue(pid, owner, this.opts.clientId, this.store.getPending())
          await p.saveRejected(pid, owner, this.opts.clientId, this.store.getRejected())
          await p.dropQueue(pid, owner, q.clientId)
        })
        .catch(() => false)
    }
    if (taken > 0) {
      this.opts.onAdopted?.(taken)
    }
  }

  stop(): void {
    this.stopped = true
    this.unsubscribe()
    this.store.onQueue = () => {}
    this.clock.clearTimeout(this.retryTimer)
    this.clock.clearTimeout(this.snapshotTimer)
  }

  // watchStore saves the queue and refusals on every change and the snapshot shortly after it moves.
  private watchStore(): void {
    const p = this.opts.persistence
    let pending = this.store.getPending()
    let rejected = this.store.getRejected()
    let seq = this.store.getSeq()
    this.unsubscribe = this.store.subscribe(() => {
      const pid = this.store.projectId
      if (p && this.store.getPending() !== pending) {
        pending = this.store.getPending()
        void p.saveQueue(pid, this.owner(), this.opts.clientId, pending).catch(() => {})
      }
      if (p && this.store.getRejected() !== rejected) {
        rejected = this.store.getRejected()
        void p.saveRejected(pid, this.owner(), this.opts.clientId, rejected).catch(() => {})
      }
      if (this.store.getSeq() !== seq) {
        seq = this.store.getSeq()
        this.opts.onConfirmed?.()
        if (p) {
          this.clock.clearTimeout(this.snapshotTimer)
          this.snapshotTimer = this.clock.setTimeout(() => {
            void p
              .saveSnapshot(pid, this.owner(), { seq: this.store.getSeq(), schemaVersion, confirmed: this.store.getConfirmed() })
              .catch(() => {})
          }, SNAPSHOT_SAVE_DELAY)
        }
      }
      if (this.status.phase === 'saved' && this.store.unsent().length > 0) {
        this.setStatus({ phase: this.online() ? 'sending' : 'offline' })
      }
    })
  }

  // onEvent handles one project stream event.
  onEvent(type: string, data: unknown): void {
    switch (type) {
      case 'ops': {
        // The stream opens only for a project the server serves, so an ops event while gone means it is back.
        if (this.blocked === 'gone') {
          void this.revive()
          return
        }
        const seq = (data as { seq?: number } | null)?.seq ?? Infinity
        if (seq > this.store.getSeq()) {
          void this.catchUp()
        }
        return
      }
      case 'reset':
        void this.catchUp()
        return
      case 'deleted':
        this.block('gone')
        return
      case 'access': {
        const role = (data as { role?: string | null } | null)?.role ?? null
        if (role === null || role === 'viewer') {
          this.block(role === null ? 'gone' : 'forbidden')
        } else {
          this.blocked = null
          this.store.setReadOnly(false)
          void this.flush()
        }
      }
    }
  }

  // resume reads and sends again after the browser reconnects or a page opens the project again.
  resume(): void {
    if (this.blocked === 'gone') {
      void this.revive()
      return
    }
    this.retryDelay = MIN_RETRY
    this.clock.clearTimeout(this.retryTimer)
    this.retryTimer = null
    void this.catchUp()
    void this.flush()
  }

  private block(kind: SyncErrorKind): void {
    this.blocks++
    this.blocked = kind
    this.store.setReadOnly(true)
    this.store.refuseAll(kind === 'gone' ? 'project_deleted' : 'forbidden')
    this.setStatus({ phase: 'error', error: kind })
  }

  // revive leaves 'gone' once the server serves the project again: restored from the trash or shared again.
  // A read that fails keeps it gone; one that started before a newer block proves nothing.
  private async revive(): Promise<void> {
    const blocks = this.blocks
    try {
      await this.readJournal()
    } catch {
      return
    }
    if (this.stopped || this.blocked !== 'gone' || this.blocks !== blocks) {
      return
    }
    this.blocked = null
    this.store.setReadOnly(false)
    this.setStatus({ phase: 'saved' })
    void this.flush()
  }

  // catchUp reads the journal after the store's seq, or a snapshot when the journal cannot bridge the gap.
  // Concurrent calls share one run; a call during a run schedules one more.
  catchUp(): Promise<void> {
    if (this.catching) {
      this.catchAgain = true
      return this.catching
    }
    const run = async () => {
      try {
        do {
          this.catchAgain = false
          await this.readJournal()
        } while (this.catchAgain && !this.stopped)
      } catch (e) {
        this.fail(e)
      }
    }
    // Cleared in a later microtask: a run that finishes without awaiting must not clear it before it is set.
    this.catching = run().finally(() => {
      this.catching = null
    })
    return this.catching
  }

  private async readJournal(): Promise<void> {
    const pid = this.store.projectId
    if (!this.store.ready) {
      await this.loadSnapshot()
    }
    for (;;) {
      const page = await this.transport.operations(pid, this.store.getSeq())
      if (page.reset || !this.store.applyJournal(page.items)) {
        await this.loadSnapshot()
        return
      }
      if (page.items.length < PAGE) {
        return
      }
    }
  }

  private async loadSnapshot(): Promise<void> {
    const s = await this.transport.snapshot(this.store.projectId)
    if (s.schema_version < 1 || s.schema_version > schemaVersion) {
      throw new SyncError('schema', `schema ${s.schema_version}`)
    }
    this.store.loadSnapshot(s.seq, s.collections)
  }

  // flush sends the unanswered transactions in batches until the queue is empty or sending fails.
  flush(): Promise<void> {
    if (this.sending) {
      return this.sending
    }
    const run = async () => {
      while (!this.stopped && this.blocked === null && this.retryTimer === null) {
        const batch = this.nextBatch()
        if (batch.length === 0) {
          this.setStatus({ phase: 'saved' })
          return
        }
        if (!this.online()) {
          this.setStatus({ phase: 'offline' })
          return
        }
        this.setStatus({ phase: 'sending' })
        try {
          const resp = await this.transport.transactions(this.store.projectId, {
            client_id: this.opts.clientId,
            schema_version: schemaVersion,
            txs: batch,
          })
          this.retryDelay = MIN_RETRY
          this.store.settle(resp.results)
          if (resp.seq > this.store.getSeq()) {
            void this.catchUp()
          }
        } catch (e) {
          if (e instanceof SyncError && e.kind === 'too_large' && this.batchLimit > 1) {
            this.batchLimit = Math.max(1, Math.floor(this.batchLimit / 2))
            continue
          }
          this.fail(e)
          return
        }
      }
    }
    this.sending = run().finally(() => {
      this.sending = null
    })
    return this.sending
  }

  private nextBatch(): Tx[] {
    const out: Tx[] = []
    let bytes = 0
    for (const p of this.store.unsent()) {
      const size = JSON.stringify(p.tx).length
      if (out.length > 0 && (out.length >= this.batchLimit || bytes + size > MAX_BYTES)) {
        break
      }
      out.push(p.tx)
      bytes += size
    }
    return out
  }

  // fail stops on errors retrying cannot fix and otherwise tries reading and sending again later, backing off
  // to a minute.
  private fail(e: unknown): void {
    const kind = e instanceof SyncError ? e.kind : 'network'
    if (kind === 'forbidden' || kind === 'gone') {
      this.block(kind)
      return
    }
    if (kind === 'schema') {
      this.blocked = kind
      this.setStatus({ phase: 'error', error: kind })
      return
    }
    this.setStatus(kind === 'network' ? { phase: 'offline' } : { phase: 'error', error: kind })
    if (this.stopped || this.retryTimer !== null) {
      return
    }
    const delay = this.retryDelay
    this.retryDelay = Math.min(MAX_RETRY, this.retryDelay * 2)
    this.retryTimer = this.clock.setTimeout(() => {
      this.retryTimer = null
      void this.catchUp()
      void this.flush()
    }, delay)
  }
}
