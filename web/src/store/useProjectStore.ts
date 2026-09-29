// One store and syncer per open project, shared by every component of the tab.

import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useSyncExternalStore } from 'react'

import { getSession, subscribeSession } from '../auth/session'
import type { ObjectType } from '../api/client'
import { demoSeed } from '../guest/seed'
import { plural } from '../fleet/format'
import { showToast } from '../ui/toast'
import type { State } from './apply'
import { clientId, pageId } from './clientId'
import { subscribeEvents } from './events'
import { GuestSync } from './guestSync'
import { GuestOwner, indexedDBPersistence } from './persist'
import { Tracker, type Person, type Selection } from './presence'
import { ProjectStore } from './store'
import { Syncer, type SyncStatus } from './sync'
import { httpTransport } from './transport'
import { UndoHistory, type UndoState } from './undo'

// ProjectSync is what a page needs of a syncer, whether the project is on the server or in this browser only.
type ProjectSync = {
  start(): Promise<void>
  stop(): void
  resume(): void
  onEvent(type: string, data: unknown): void
  getStatus: () => SyncStatus
  subscribeStatus: (fn: () => void) => () => void
}

type Entry = {
  owner: string
  store: ProjectStore
  sync: ProjectSync
  history: UndoHistory
  presence: Tracker
  refs: number
  started: boolean
  stop: () => void
  idle?: ReturnType<typeof setTimeout>
}

// Moving between pages of one project unmounts and remounts its components; the stream survives the gap.
const IDLE_CLOSE = 15_000

const entries = new Map<string, Entry>()
const persistence = indexedDBPersistence()

// A demo is a project of this browser only: the same store and the same pages, with no project on the server
// behind it. There is one per object type.
const demoPrefix = 'demo-'

export function demoStoreId(type: ObjectType): string {
  return `${demoPrefix}${type}`
}

function demoTypeOf(storeKey: string): ObjectType | null {
  const t = storeKey.startsWith(demoPrefix) ? storeKey.slice(demoPrefix.length) : ''
  return t === 'warehouse' || t === 'airport' || t === 'hospital' ? t : null
}

// storeId names the store a page reads: the project of the URL, or its demo. A page with neither reads an
// empty store that never starts.
export function storeId(projectId?: string | null, demo?: ObjectType | null): string {
  if (projectId) {
    return projectId
  }
  return demo ? demoStoreId(demo) : ''
}

// owner is whose queues this tab may load and send. Signing in or out changes it, and every open store is
// closed and built again, so one person's unsent edits never leave under another person's session. The guest
// project belongs to the browser, not to an account, so it stays the guest's through a sign-in.
function owner(projectId: string): string {
  if (demoTypeOf(projectId)) {
    return GuestOwner
  }
  return getSession()?.user.id ?? GuestOwner
}

let generation = 0
const generationListeners = new Set<() => void>()

const getGeneration = () => generation
const subscribeGeneration = (fn: () => void): (() => void) => {
  generationListeners.add(fn)
  return () => {
    generationListeners.delete(fn)
  }
}

subscribeSession(() => {
  for (const [key, e] of entries) {
    if (e.owner !== owner(e.store.projectId)) {
      e.stop()
      entries.delete(key)
    }
  }
  generation++
  for (const fn of generationListeners) {
    fn()
  }
})

function entryFor(projectId: string, qc: QueryClient): Entry {
  const who = owner(projectId)
  const key = `${who}:${projectId}`
  let e = entries.get(key)
  if (!e) {
    const store = new ProjectStore(projectId)
    const demo = demoTypeOf(projectId)
    const sync: ProjectSync = demo ? new GuestSync(store, {
      persistence,
      seed: () => demoSeed(demo),
      channel: (name) => (typeof BroadcastChannel === 'undefined' ? null : new BroadcastChannel(name)),
    }) : new Syncer(store, httpTransport, {
      clientId: clientId(),
      owner: who,
      persistence,
      online: () => navigator.onLine,
      // Pages still on React Query read the same project; they refresh when the confirmed state moves.
      onConfirmed: () => void qc.invalidateQueries({ queryKey: ['project', projectId] }),
      onAdopted: (count) =>
        showToast({
          message: `Отправляем ${count} ${plural(count, 'правку', 'правки', 'правок')} из закрытой вкладки.`,
        }),
    })
    e = {
      owner: who,
      store,
      sync,
      history: new UndoHistory(store),
      presence: new Tracker(projectId, { clientId: pageId(), userId: who }),
      refs: 0,
      started: false,
      stop: () => {},
    }
    entries.set(key, e)
  }
  return e
}

function start(e: Entry, qc: QueryClient): void {
  // A page with no project at all (an object type not chosen yet) has nothing to open.
  if (e.started || e.store.projectId === '') {
    return
  }
  e.started = true
  const pid = e.store.projectId
  void e.sync.start()
  // Nobody else is in a demo, and no stream carries it.
  if (demoTypeOf(pid)) {
    e.stop = () => e.sync.stop()
    return
  }
  const offEvents = subscribeEvents(pid, (type, data) => {
    e.sync.onEvent(type, data)
    if (type === 'presence') {
      e.presence.onEvent(data)
    }
    if (type === 'runs') {
      void qc.invalidateQueries({ queryKey: ['runs', pid] })
      void qc.invalidateQueries({ queryKey: ['simulations', pid] })
      void qc.invalidateQueries({ queryKey: ['sim-checks', pid] })
    }
  })
  const online = () => e.sync.resume()
  // A closed page says goodbye at once; pagehide is the event mobile browsers do fire. A page that comes back from
  // the back-forward cache or from hiding reports again, since timers do not run while it is away.
  const bye = () => e.presence.leave()
  const back = () => {
    if (document.visibilityState === 'visible') {
      e.presence.refresh()
    }
  }
  window.addEventListener('online', online)
  window.addEventListener('pagehide', bye)
  window.addEventListener('pageshow', back)
  document.addEventListener('visibilitychange', back)
  e.stop = () => {
    offEvents()
    window.removeEventListener('online', online)
    window.removeEventListener('pagehide', bye)
    window.removeEventListener('pageshow', back)
    document.removeEventListener('visibilitychange', back)
    e.presence.stop()
    e.sync.stop()
  }
}

// useProjectStore returns the project's store; its syncer runs while any component uses it.
export function useProjectStore(projectId: string): ProjectStore {
  const qc = useQueryClient()
  const generation = useSyncExternalStore(subscribeGeneration, getGeneration)
  const e = entryFor(projectId, qc)
  useEffect(() => {
    const cur = entryFor(projectId, qc)
    const key = `${cur.owner}:${projectId}`
    clearTimeout(cur.idle)
    // A store kept from an earlier visit may think the project is gone while it was restored meanwhile.
    if (cur.refs === 0 && cur.started) {
      cur.sync.resume()
    }
    cur.refs++
    start(cur, qc)
    return () => {
      cur.refs--
      if (cur.refs === 0) {
        cur.idle = setTimeout(() => {
          if (cur.refs === 0) {
            cur.stop()
            if (entries.get(key) === cur) {
              entries.delete(key)
            }
          }
        }, IDLE_CLOSE)
      }
    }
    // generation changes when the signed-in person changes: the old store is closed and this one takes over.
  }, [projectId, qc, generation])
  return e.store
}

// useStoreSelector re-renders only when the selected value changes. Selectors that build new arrays or
// objects pass an isEqual that compares their contents.
export function useStoreSelector<T>(store: ProjectStore, selector: (st: State) => T, isEqual: (a: T, b: T) => boolean = Object.is): T {
  const last = useRef<{ st: State; fn: (st: State) => T; sel: T } | null>(null)
  // getSnapshot is made on every render, so it always calls the current selector; a new selector (a new id,
  // say) is re-run even when the state has not moved.
  const getSnapshot = () => {
    const st = store.getState()
    const prev = last.current
    if (prev && prev.st === st && prev.fn === selector) {
      return prev.sel
    }
    const sel = selector(st)
    const keep = prev !== null && isEqual(prev.sel, sel)
    last.current = { st, fn: selector, sel: keep ? prev.sel : sel }
    return last.current.sel
  }
  return useSyncExternalStore(store.subscribe, getSnapshot)
}

// useSyncStatus follows the project's sending state (saved, sending, offline, error).
export function useSyncStatus(projectId: string): SyncStatus {
  const qc = useQueryClient()
  const { sync } = entryFor(projectId, qc)
  return useSyncExternalStore(sync.subscribeStatus, sync.getStatus)
}

// useUndoHistory returns the tab's undo history of the project and the labels of its next undo and redo.
export function useUndoHistory(projectId: string): [UndoHistory, UndoState] {
  const qc = useQueryClient()
  const { history } = entryFor(projectId, qc)
  return [history, useSyncExternalStore(history.subscribe, history.getState)]
}

// usePresence lists the other people in the project, with what each of them has selected.
export function usePresence(projectId: string): Person[] {
  const qc = useQueryClient()
  const { presence } = entryFor(projectId, qc)
  return useSyncExternalStore(presence.subscribe, presence.getState)
}

// useReportRoute tells the others which page of the project this tab is on. A demo has nobody else in it.
export function useReportRoute(projectId: string, route: string): void {
  const qc = useQueryClient()
  const { presence } = entryFor(projectId, qc)
  useEffect(() => {
    if (projectId && !demoTypeOf(projectId)) {
      presence.at(route)
    }
  }, [projectId, presence, route])
}

// useReportSelection tells the others what this tab has selected. Leaving the page clears it.
export function useReportSelection(projectId: string, selection: Selection): void {
  const qc = useQueryClient()
  const { presence } = entryFor(projectId, qc)
  const key = JSON.stringify(selection)
  const reports = Boolean(projectId) && !demoTypeOf(projectId)
  useEffect(() => {
    if (reports) {
      presence.select(key === 'null' ? null : (JSON.parse(key) as Selection))
    }
  }, [reports, presence, key])
  useEffect(
    () => () => {
      if (reports) {
        presence.select(null)
      }
    },
    [reports, presence],
  )
}

// storeFor gives non-React code (keyboard handlers, tests) the store of an open project.
export function storeFor(projectId: string): ProjectStore | undefined {
  return entries.get(`${owner(projectId)}:${projectId}`)?.store
}

// unsentEdits counts the transactions the open projects have not sent yet, for the warning before signing out.
export function unsentEdits(): number {
  let n = 0
  for (const e of entries.values()) {
    n += e.store.unsent().length
  }
  return n
}

// pendingEdits counts this person's edits of a project that still wait in this browser, open or not.
export async function pendingEdits(projectId: string): Promise<number> {
  if (!persistence) {
    return 0
  }
  const queues = await persistence.listQueues(projectId, owner(projectId)).catch(() => [])
  return queues.reduce((n, q) => n + q.pending.length, 0)
}

// editedDemo returns a demo as this browser keeps it, or null when it was never changed from its seed.
export async function editedDemo(type: ObjectType): Promise<{ seq: number; state: State } | null> {
  if (!persistence) {
    return null
  }
  const id = demoStoreId(type)
  const open = entries.get(`${GuestOwner}:${id}`)
  if (open?.store.ready) {
    const seq = open.store.getSeq()
    return seq > 0 ? { seq, state: open.store.getConfirmed() } : null
  }
  const kept = await persistence.load(id, GuestOwner, GuestOwner).catch(() => null)
  const snap = kept?.snapshot
  return snap && snap.seq > 0 ? { seq: snap.seq, state: snap.confirmed } : null
}
