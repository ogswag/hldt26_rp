// GuestSync keeps a guest project alive in the browser. There is no project on the server behind it, so every
// transaction the schema accepts here is final at once: it goes into the confirmed state and is written to
// IndexedDB. The page cannot tell the difference: it reads the same store and sees the same "Сохранено". Other tabs
// of the same demo get each transaction over a BroadcastChannel and apply it as their own next one.

import type { DemoSeed } from '../guest/seed'
import type { RawOp } from './apply'
import type { Persistence, SavedSnapshot } from './persist'
import { GuestOwner } from './persist'
import { schemaVersion } from './schema.gen'
import type { ProjectStore } from './store'
import type { SyncStatus } from './sync'

// Relay is the part of BroadcastChannel a demo uses to reach its other tabs.
export type Relay = Pick<BroadcastChannel, 'postMessage' | 'onmessage' | 'close'>

type Shared = { txs: { tx_id: string; ops: RawOp[] }[] }

export type GuestSyncOptions = {
  persistence?: Persistence | null
  // channel opens the relay named for this demo; without it the demo has one tab.
  channel?: (name: string) => Relay | null
  // seed says which version of the demo this build has and builds its first state. What an earlier visit left in
  // the browser is used only when it was seeded from the same version.
  seed: () => Promise<DemoSeed>
  onFirstSaved?: () => void
}

export class GuestSync {
  private readonly store: ProjectStore
  private readonly opts: GuestSyncOptions
  private status: SyncStatus = { phase: 'loading' }
  private readonly listeners = new Set<() => void>()
  private stopped = false
  private writing = 0
  private relay: Relay | null = null
  private version = ''
  private unsubscribe: () => void = () => {}

  constructor(store: ProjectStore, opts: GuestSyncOptions) {
    this.store = store
    this.opts = opts
  }

  getStatus = (): SyncStatus => this.status

  subscribeStatus = (fn: () => void): (() => void) => {
    this.listeners.add(fn)
    return () => {
      this.listeners.delete(fn)
    }
  }

  private setStatus(next: SyncStatus): void {
    if (next.phase === this.status.phase) {
      return
    }
    this.status = next
    for (const fn of this.listeners) {
      fn()
    }
  }

  async start(): Promise<void> {
    const p = this.opts.persistence
    const seed = await this.opts.seed()
    let saved: SavedSnapshot | null = null
    if (p) {
      const kept = await p.load(this.store.projectId, GuestOwner, GuestOwner).catch(() => null)
      // A schema this build does not know is not readable, and a demo seeded from another version is not the demo
      // of this build: the guest starts over rather than on half a project.
      if (kept?.snapshot && kept.snapshot.schemaVersion === schemaVersion && kept.snapshot.demoVersion === seed.version) {
        saved = kept.snapshot
        this.store.restore(kept.pending, kept.rejected)
      }
    }
    const first = saved ? saved.confirmed : await seed.state()
    if (this.stopped) {
      return
    }
    this.version = seed.version
    this.store.loadSnapshot(saved ? saved.seq : 0, first)
    this.watchStore()
    this.store.onQueue = () => this.confirm()
    this.listen()
    this.confirm()
    if (saved && this.writing === 0) {
      this.setStatus({ phase: 'saved' })
    }
    if (!saved) {
      this.save()
    }
  }

  stop(): void {
    this.stopped = true
    this.unsubscribe()
    this.store.onQueue = () => {}
    this.relay?.close()
    this.relay = null
  }

  // The stream and the network are the server's; a guest project has neither.
  onEvent(): void {}

  resume(): void {}

  // confirm folds what the page just did into the confirmed state, in the order it was done.
  private confirm(): void {
    const waiting = this.store.unsent()
    if (waiting.length === 0) {
      return
    }
    let seq = this.store.getSeq()
    const txs = waiting.map((p) => ({ tx_id: p.tx.tx_id, ops: p.tx.ops }))
    const ok = this.store.applyJournal(txs.map((t) => ({ seq: ++seq, ...t })))
    if (!ok) {
      // The queue cannot follow the confirmed state, which here can only be a bug. Keeping the queue would
      // repeat it forever, so it is refused and the person is told through the "Не применено" list.
      this.store.refuseAll('invalid_value')
      return
    }
    this.relay?.postMessage({ txs } satisfies Shared)
  }

  private listen(): void {
    this.relay = this.opts.channel?.(`robots-${this.store.projectId}`) ?? null
    if (!this.relay) {
      return
    }
    this.relay.onmessage = (e: MessageEvent) => {
      const txs = (e.data as Shared | null)?.txs
      if (this.stopped || !Array.isArray(txs)) {
        return
      }
      // Each transaction becomes this tab's next one. What no longer applies here, such as a change to a record
      // this tab deleted, is left out.
      for (const tx of txs) {
        this.store.applyJournal([{ seq: this.store.getSeq() + 1, tx_id: tx.tx_id, ops: tx.ops }])
      }
    }
  }

  private watchStore(): void {
    const p = this.opts.persistence
    let seq = this.store.getSeq()
    let rejected = this.store.getRejected()
    this.unsubscribe = this.store.subscribe(() => {
      if (!p) {
        return
      }
      if (this.store.getRejected() !== rejected) {
        rejected = this.store.getRejected()
        void p.saveRejected(this.store.projectId, GuestOwner, GuestOwner, rejected).catch(() => {})
      }
      if (this.store.getSeq() !== seq) {
        seq = this.store.getSeq()
        this.save()
      }
    })
  }

  // save writes the state as soon as it moves, and starts the write there and then. This browser holds the
  // only copy of a guest project: a reload a moment after an edit must find it. Waiting out a timer, or
  // holding a write back until the one before it finishes, is long enough to lose the last change; IndexedDB
  // runs the writes in the order they were asked for, so the newest state is the one that stays.
  //
  // The status follows the write, not the edit: a page that says "Сохранено" means the browser has it, and
  // closing the tab then loses nothing.
  private save(): void {
    const p = this.opts.persistence
    if (!p) {
      return
    }
    this.writing++
    this.setStatus({ phase: 'sending' })
    void p
      .saveSnapshot(this.store.projectId, GuestOwner, { seq: this.store.getSeq(), schemaVersion, confirmed: this.store.getConfirmed(), demoVersion: this.version })
      .then(() => {
        this.opts.onFirstSaved?.()
        this.writing--
        if (this.writing === 0 && !this.stopped) {
          this.setStatus({ phase: 'saved' })
        }
      })
      .catch(() => {
        this.writing--
        if (!this.stopped) {
          this.setStatus({ phase: 'error', error: 'server' })
        }
      })
  }
}
