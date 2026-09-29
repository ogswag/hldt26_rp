// Keeps the confirmed snapshot, the tab's queue and its refused list across reloads (IndexedDB "robots-store").
// A queue and a refused list belong to one person in one tab: the key is project, user and client, so signing in
// as someone else never picks up edits that are not theirs, and two tabs never overwrite each other's lists.

import type { State } from './apply'
import type { Pending } from './state'
import type { Rejected } from './store'
import { track } from './writeTracker'

// demoVersion names the demo the state was seeded from; a demo whose seed has changed starts over.
export type SavedSnapshot = { seq: number; schemaVersion: number; confirmed: State; demoVersion?: string }

export type Saved = { snapshot: SavedSnapshot | null; pending: Pending[]; rejected: Rejected[] }

// SavedQueue is one tab's queue and refused list as they lie in storage. A tab whose lock is free has closed for good.
export type SavedQueue = { clientId: string; pending: Pending[]; rejected: Rejected[] }

export type Persistence = {
  load(projectId: string, owner: string, clientId: string): Promise<Saved>
  saveSnapshot(projectId: string, owner: string, snap: SavedSnapshot): Promise<void>
  saveQueue(projectId: string, owner: string, clientId: string, pending: readonly Pending[]): Promise<void>
  saveRejected(projectId: string, owner: string, clientId: string, rejected: readonly Rejected[]): Promise<void>
  // listQueues lists this person's queues and refused lists in this project, including this tab's own.
  listQueues(projectId: string, owner: string): Promise<SavedQueue[]>
  dropQueue(projectId: string, owner: string, clientId: string): Promise<void>
}

const DB = 'robots-store'
const SNAPSHOTS = 'snapshots'
const QUEUES = 'queues'
const REJECTED = 'rejected'

// GuestOwner names the queue of a browser nobody has signed in to.
export const GuestOwner = 'guest'

function queueKey(projectId: string, owner: string, clientId: string): string {
  return `${projectId}:${owner}:${clientId}`
}

function ownerKey(projectId: string, owner: string): string {
  return `${projectId}:${owner}`
}

function queuePrefix(projectId: string, owner: string): string {
  return `${projectId}:${owner}:`
}

function req<T>(r: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    r.onsuccess = () => resolve(r.result)
    r.onerror = () => reject(r.error)
  })
}

function open(): Promise<IDBDatabase> {
  const r = indexedDB.open(DB, 2)
  r.onupgradeneeded = (e) => {
    for (const name of [SNAPSHOTS, QUEUES, REJECTED]) {
      if (!r.result.objectStoreNames.contains(name)) {
        r.result.createObjectStore(name)
      }
    }
    // Version 1 kept one refused list per person and project, shared by all tabs. It has no owner tab to go back to.
    if (e.oldVersion === 1) {
      r.transaction?.objectStore(REJECTED).clear()
    }
  }
  return req(r)
}

// indexedDBPersistence returns null where IndexedDB is missing; the store then lives in memory only.
export function indexedDBPersistence(): Persistence | null {
  if (typeof indexedDB === 'undefined') {
    return null
  }
  let db: Promise<IDBDatabase> | null = null
  const transaction = async (name: string | string[], mode: IDBTransactionMode) => {
    db ??= open()
    return (await db).transaction(name, mode)
  }
  const finished = (tx: IDBTransaction) =>
    new Promise<void>((resolve, reject) => {
      tx.oncomplete = () => resolve()
      tx.onabort = () => reject(tx.error ?? new Error('IndexedDB transaction aborted'))
      tx.onerror = () => reject(tx.error ?? new Error('IndexedDB transaction failed'))
    })
  const put = (name: string, key: string, value: unknown) =>
    track(
      (async () => {
        const tx = await transaction(name, 'readwrite')
        tx.objectStore(name).put(value, key)
        await finished(tx)
      })(),
    )
  const get = async <T>(name: string, key: string): Promise<T | undefined> =>
    req((await transaction(name, 'readonly')).objectStore(name).get(key)) as Promise<T | undefined>
  return {
    async load(projectId, owner, clientId) {
      const [snapshot, pending, rejected] = await Promise.all([
        get<SavedSnapshot>(SNAPSHOTS, ownerKey(projectId, owner)),
        get<Pending[]>(QUEUES, queueKey(projectId, owner, clientId)),
        get<Rejected[]>(REJECTED, queueKey(projectId, owner, clientId)),
      ])
      return { snapshot: snapshot ?? null, pending: pending ?? [], rejected: rejected ?? [] }
    },
    saveSnapshot: (projectId, owner, snap) => put(SNAPSHOTS, ownerKey(projectId, owner), snap),
    saveQueue: (projectId, owner, clientId, pending) => put(QUEUES, queueKey(projectId, owner, clientId), pending),
    saveRejected: (projectId, owner, clientId, rejected) => put(REJECTED, queueKey(projectId, owner, clientId), rejected),
    async listQueues(projectId, owner) {
      const prefix = queuePrefix(projectId, owner)
      // "￿" ends the range: IndexedDB orders strings by code unit, so nothing of this owner sorts past it.
      const range = IDBKeyRange.bound(prefix, `${prefix}￿`)
      const tx = await transaction([QUEUES, REJECTED], 'readonly')
      const [queues, refused] = await Promise.all([QUEUES, REJECTED].map(async (name) => {
        const s = tx.objectStore(name)
        const [keys, values] = await Promise.all([req(s.getAllKeys(range)), req(s.getAll(range))])
        return new Map(keys.map((k, i) => [String(k).slice(prefix.length), values[i] as unknown[]]))
      }))
      return [...new Set([...queues.keys(), ...refused.keys()])].map((clientId) => ({
        clientId,
        pending: (queues.get(clientId) as Pending[] | undefined) ?? [],
        rejected: (refused.get(clientId) as Rejected[] | undefined) ?? [],
      }))
    },
    dropQueue: (projectId, owner, clientId) =>
      track(
        (async () => {
          const tx = await transaction([QUEUES, REJECTED], 'readwrite')
          tx.objectStore(QUEUES).delete(queueKey(projectId, owner, clientId))
          tx.objectStore(REJECTED).delete(queueKey(projectId, owner, clientId))
          await finished(tx)
        })(),
      ),
  }
}

// memoryPersistence stands in for IndexedDB in tests.
export function memoryPersistence(): Persistence & { data: Map<string, unknown> } {
  const data = new Map<string, unknown>()
  const clone = <T>(v: T): T => structuredClone(v)
  const qk = (projectId: string, owner: string, clientId: string) => `q:${queueKey(projectId, owner, clientId)}`
  return {
    data,
    async load(projectId, owner, clientId) {
      return {
        snapshot: clone((data.get(`s:${ownerKey(projectId, owner)}`) as SavedSnapshot | undefined) ?? null),
        pending: clone((data.get(qk(projectId, owner, clientId)) as Pending[] | undefined) ?? []),
        rejected: clone((data.get(`r:${queueKey(projectId, owner, clientId)}`) as Rejected[] | undefined) ?? []),
      }
    },
    async saveSnapshot(projectId, owner, snap) {
      data.set(`s:${ownerKey(projectId, owner)}`, clone(snap))
    },
    async saveQueue(projectId, owner, clientId, pending) {
      data.set(qk(projectId, owner, clientId), clone(pending))
    },
    async saveRejected(projectId, owner, clientId, rejected) {
      data.set(`r:${queueKey(projectId, owner, clientId)}`, clone(rejected))
    },
    async listQueues(projectId, owner) {
      const prefix = queuePrefix(projectId, owner)
      const found = new Map<string, SavedQueue>()
      for (const [key, value] of data) {
        const kind = key.slice(0, 2)
        if ((kind !== 'q:' && kind !== 'r:') || !key.slice(2).startsWith(prefix)) {
          continue
        }
        const clientId = key.slice(2 + prefix.length)
        const q = found.get(clientId) ?? { clientId, pending: [], rejected: [] }
        if (kind === 'q:') {
          q.pending = clone(value as Pending[])
        } else {
          q.rejected = clone(value as Rejected[])
        }
        found.set(clientId, q)
      }
      return [...found.values()]
    },
    async dropQueue(projectId, owner, clientId) {
      data.delete(qk(projectId, owner, clientId))
      data.delete(`r:${queueKey(projectId, owner, clientId)}`)
    },
  }
}
