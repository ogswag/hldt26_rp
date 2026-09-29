// The browser engine: the same Go code the server runs, for a guest, for a page without network and for a
// what-if. Everything it returns is preliminary; a saved run always comes from the server.

import type {
  CalculateResult,
  EconOverrides,
  EconSimCheck,
  EngineCatalog,
  MapCheck,
  MapDocument,
  MapIssue,
  MatchOutput,
  ObjectType,
  ParamsMap,
  SimulationConfig,
  SimulationLog,
  SimulationSummary,
} from '../api/client'
import { demoMode } from '../api/demo'
import type { State } from '../store/apply'
import type { EngineCall, EngineMethod, EngineReply } from './protocol'

// The API serves the engine next to itself; a demo build carries it as static files of its own.
const base = demoMode ? `${import.meta.env.BASE_URL}engine` : '/api/engine'

// A run of the simulation can take seconds and is cancelled by dropping its worker, so it gets a worker of its own:
// stopping it never kills a calculation, and a calculation never waits behind a run.
export type Lane = 'calc' | 'sim'

type Waiting = Map<number, { reply: (r: EngineReply) => void; reject: (err: Error) => void }>
type LaneState = { worker: Worker | null; waiting: Waiting }

const lanes: Record<Lane, LaneState> = { calc: { worker: null, waiting: new Map() }, sim: { worker: null, waiting: new Map() } }
let nextID = 1

// available reports whether this browser can run the engine at all.
export function available(): boolean {
  return typeof Worker !== 'undefined' && typeof WebAssembly !== 'undefined'
}

function start(lane: Lane): Worker {
  const st = lanes[lane]
  if (st.worker) {
    return st.worker
  }
  const worker = new Worker(new URL('./worker.ts', import.meta.url), { type: 'module' })
  st.worker = worker
  worker.onmessage = (e: MessageEvent<EngineReply>) => {
    st.waiting.get(e.data.id)?.reply(e.data)
    st.waiting.delete(e.data.id)
  }
  const failed = (message: string) => {
    const err = new Error(message)
    for (const call of st.waiting.values()) {
      call.reject(err)
    }
    st.waiting.clear()
    worker.terminate()
    if (st.worker === worker) {
      st.worker = null
    }
  }
  worker.onerror = () => failed('Движок остановился с ошибкой. Повторите расчёт.')
  worker.onmessageerror = () => failed('Движок вернул повреждённый ответ. Повторите расчёт.')
  return worker
}

function drop(lane: Lane, message: string): void {
  const st = lanes[lane]
  const err = new Error(message)
  for (const call of st.waiting.values()) {
    call.reject(err)
  }
  st.worker?.terminate()
  st.worker = null
  st.waiting.clear()
}

// stop drops the workers and the engines they loaded.
export function stop(): void {
  drop('calc', 'Расчёт остановлен.')
  drop('sim', 'Расчёт остановлен.')
}

// StoppedRun is what a run of the simulation rejects with when it is cancelled.
export const StoppedRun = 'Симуляция остановлена.'

// stopSimulation cancels the run in progress; the next call starts a new worker.
export function stopSimulation(): void {
  drop('sim', StoppedRun)
}

function call<T>(method: EngineMethod, request: unknown, lane: Lane = 'calc'): Promise<T> {
  if (!available()) {
    return Promise.reject(new Error('Этот браузер не умеет считать без сервера.'))
  }
  const id = nextID++
  const w = start(lane)
  const waiting = lanes[lane].waiting
  return new Promise<T>((resolve, reject) => {
    waiting.set(id, {
      reject,
      reply: (r) => {
        if (r.ok) {
          resolve(r.result as T)
        } else {
          reject(new Error(r.error))
        }
      },
    })
    try {
      w.postMessage({ id, base, method, request } satisfies EngineCall)
    } catch (err) {
      waiting.delete(id)
      reject(err instanceof Error ? err : new Error('Не удалось передать данные движку.'))
    }
  })
}

export type EngineRequest = {
  catalog: EngineCatalog
  object_type: ObjectType
  params: ParamsMap
  seed?: number
  include_ids?: string[]
  task_codes?: string[]
  overrides?: EconOverrides
  // collections is the project as its records. It replaces object_type, params, include_ids and overrides.
  collections?: State
}

export function calculate(req: EngineRequest): Promise<CalculateResult> {
  return call<CalculateResult>('calculate', req)
}

export function match(req: EngineRequest): Promise<MatchOutput> {
  return call<MatchOutput>('match', req)
}

export function validateParams(objectType: ObjectType, params: ParamsMap): Promise<{ ok: boolean; details?: unknown }> {
  return call('validateParams', { object_type: objectType, params })
}

export type LocalRun =
  | { status: 'done'; summary: SimulationSummary; log?: SimulationLog }
  | { status: 'refused'; message: string; issues?: MapIssue[] }

export type LocalRunRequest = {
  catalog: EngineCatalog
  collections: State
  variant_id?: string
  sim?: SimulationConfig
  seed?: number
  with_log?: boolean
  mode?: string
  line_key?: string
}

// simulateMap runs the event simulation on the project's own map. Cancel it with stopSimulation.
export function simulateMap(req: LocalRunRequest): Promise<LocalRun> {
  return call<LocalRun>('simulateMap', req, 'sim')
}

// checkMap checks a map document against the fleets of the project's variants.
export function checkMap(catalog: EngineCatalog, collections: State, document: MapDocument): Promise<MapCheck> {
  return call<MapCheck>('checkMap', { catalog, collections, document })
}

// mapTemplate is the starter map for the processes of the project.
export function mapTemplate(collections: State): Promise<MapDocument> {
  return call<MapDocument>('mapTemplate', { collections })
}

// draftHash names the inputs of a run, so a page can tell that a result no longer matches the project.
export function draftHash(collections: State): Promise<string> {
  return call<string>('draftHash', { collections })
}

// SimRunBrief is what the check of a variant needs of a kept run.
export type SimRunBrief = {
  run_id: string
  variant_id: string
  variant_hash: string
  econ_check?: NonNullable<SimulationSummary['econ_check']>
}

// simChecks compares the latest kept run of each variant with what the economics counts on. Runs go newest first.
export function simChecks(collections: State, runs: SimRunBrief[]): Promise<EconSimCheck[]> {
  return call<EconSimCheck[]>('simChecks', { collections, runs })
}

// variantHashes names the inputs of every variant, so a page can tell which kept runs are stale.
export function variantHashes(collections: State): Promise<Record<string, string>> {
  return call<Record<string, string>>('variantHashes', { collections })
}
