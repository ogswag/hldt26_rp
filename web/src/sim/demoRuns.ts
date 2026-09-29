// The last runs of a demo, kept in the browser. A demo has no server, so its runs live next to its project: five
// per demo, the newest first. The journal of a run is large, so it is stored apart from the summary and read only
// when the run is opened.

import type { SimulationConfig, SimulationLog, SimulationSummary } from '../api/client'
import { track } from '../store/writeTracker'

export const KeptRuns = 5

export type DemoRun = {
  id: string
  createdAt: string
  // hash names the inputs the run was made from; a run whose hash differs from the demo's now is stale.
  hash: string
  config: SimulationConfig
  summary: SimulationSummary
}

export type DemoRuns = {
  list(demo: string): Promise<DemoRun[]>
  log(demo: string, id: string): Promise<SimulationLog | null>
  // put keeps the run and its journal, and drops the oldest runs beyond KeptRuns.
  put(demo: string, run: DemoRun, log: SimulationLog | null): Promise<void>
}

const DB = 'robots-demo-runs'
const RUNS = 'runs'
const LOGS = 'logs'

function key(demo: string, run: Pick<DemoRun, 'createdAt' | 'id'>): string {
  return `${demo}:${run.createdAt}:${run.id}`
}

function prefix(demo: string): string {
  return `${demo}:`
}

function req<T>(r: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    r.onsuccess = () => resolve(r.result)
    r.onerror = () => reject(r.error)
  })
}

function open(): Promise<IDBDatabase> {
  const r = indexedDB.open(DB, 1)
  r.onupgradeneeded = () => {
    r.result.createObjectStore(RUNS)
    r.result.createObjectStore(LOGS)
  }
  return req(r)
}

// indexedDBRuns returns null where IndexedDB is missing; the runs then live in memory until the page closes.
export function indexedDBRuns(): DemoRuns | null {
  if (typeof indexedDB === 'undefined') {
    return null
  }
  let db: Promise<IDBDatabase> | null = null
  const transaction = async (names: string[], mode: IDBTransactionMode) => {
    db ??= open()
    return (await db).transaction(names, mode)
  }
  const finished = (tx: IDBTransaction) =>
    new Promise<void>((resolve, reject) => {
      tx.oncomplete = () => resolve()
      tx.onabort = () => reject(tx.error ?? new Error('IndexedDB transaction aborted'))
      tx.onerror = () => reject(tx.error ?? new Error('IndexedDB transaction failed'))
    })
  // "￿" ends the range: IndexedDB orders strings by code unit, so nothing of this demo sorts past it.
  const range = (demo: string) => IDBKeyRange.bound(prefix(demo), `${prefix(demo)}￿`)
  return {
    async list(demo) {
      const tx = await transaction([RUNS], 'readonly')
      const found = (await req(tx.objectStore(RUNS).getAll(range(demo)))) as DemoRun[]
      return found.sort((a, b) => (a.createdAt < b.createdAt ? 1 : -1))
    },
    async log(demo, id) {
      const tx = await transaction([LOGS], 'readonly')
      const s = tx.objectStore(LOGS)
      const keys = await req(s.getAllKeys(range(demo)))
      const at = keys.find((k) => String(k).endsWith(`:${id}`))
      return at === undefined ? null : ((await req(s.get(at))) as SimulationLog)
    },
    put: (demo, run, log) =>
      track(
        (async () => {
          const tx = await transaction([RUNS, LOGS], 'readwrite')
          const runs = tx.objectStore(RUNS)
          const logs = tx.objectStore(LOGS)
          runs.put(run, key(demo, run))
          if (log) {
            logs.put(log, key(demo, run))
          }
          const keys = (await req(runs.getAllKeys(range(demo)))).map(String).sort().reverse()
          for (const old of keys.slice(KeptRuns)) {
            runs.delete(old)
            logs.delete(old)
          }
          await finished(tx)
        })(),
      ),
  }
}

// memoryRuns stands in for IndexedDB in tests and where it is missing.
export function memoryRuns(): DemoRuns {
  const runs = new Map<string, DemoRun>()
  const logs = new Map<string, SimulationLog>()
  const keys = (demo: string) => [...runs.keys()].filter((k) => k.startsWith(prefix(demo))).sort().reverse()
  return {
    async list(demo) {
      return keys(demo).map((k) => structuredClone(runs.get(k) as DemoRun))
    },
    async log(demo, id) {
      const at = keys(demo).find((k) => k.endsWith(`:${id}`))
      const found = at === undefined ? undefined : logs.get(at)
      return found ? structuredClone(found) : null
    },
    async put(demo, run, log) {
      runs.set(key(demo, run), structuredClone(run))
      if (log) {
        logs.set(key(demo, run), structuredClone(log))
      }
      for (const old of keys(demo).slice(KeptRuns)) {
        runs.delete(old)
        logs.delete(old)
      }
    },
  }
}
