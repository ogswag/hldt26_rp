// Where the runs of the simulation page come from. A project's runs are jobs on the server; a demo's runs are made
// in this browser by the engine and kept next to the demo. The page reads both through the same few calls.

import {
  ApiError,
  cancelSimulationJob,
  createSimulation,
  fetchSimulationEvents,
  fetchSimulationRun,
  fetchSimulations,
  pinSimulationRun,
  retrySimulationJob,
  type EngineCatalog,
  type MapIssue,
  type SimulationConfig,
  type SimulationListItem,
  type SimulationLog,
  type SimulationRunResponse,
} from '../api/client'
import { draftHash, simulateMap, stopSimulation, variantHashes } from '../engine/client'
import { NoCatalog } from '../engine/fallback'
import { holdReload } from '../offline/serviceWorker'
import type { State } from '../store/apply'
import { uuid } from '../store/clientId'

import type { DemoRun, DemoRuns } from './demoRuns'

export type SimSource = {
  // key changes when the runs on show have to be read again.
  key: readonly unknown[]
  // local is a source whose runs are made in this browser: one at a time, cancelled by stopping the engine.
  local: boolean
  // poll is true while the source has to be asked again for jobs that are still running.
  poll: boolean
  list(): Promise<SimulationListItem[]>
  // create runs or queues a simulation and returns the warnings to show before its result.
  create(variantId: string | undefined, config: SimulationConfig, extra?: { line_key?: string }): Promise<string[]>
  cancel(jobId: string): Promise<void>
  retry?(jobId: string): Promise<void>
  run(runId: string): Promise<SimulationRunResponse>
  events(runId: string): Promise<SimulationLog>
  pin?(runId: string, pinned: boolean): Promise<void>
}

// SimRefused is a run the engine would not start, with the map issues behind it when the map is the cause.
export class SimRefused extends Error {
  readonly issues: MapIssue[]

  constructor(message: string, issues: MapIssue[] = []) {
    super(message)
    this.issues = issues
  }
}

// issuesOf reads the map issues out of a refused start, from either source.
export function issuesOf(err: unknown): MapIssue[] {
  if (err instanceof ApiError || err instanceof SimRefused) {
    return err.issues
  }
  return []
}

export function serverSource(projectId: string): SimSource {
  return {
    key: ['simulations', projectId],
    local: false,
    poll: true,
    list: async () => (await fetchSimulations(projectId)).items,
    create: async (variantId, config, extra) =>
      (await createSimulation(projectId, { ...config, variant_id: variantId, line_key: extra?.line_key })).preview.warnings,
    cancel: async (jobId) => void (await cancelSimulationJob(jobId)),
    retry: async (jobId) => void (await retrySimulationJob(jobId)),
    run: (runId) => fetchSimulationRun(runId),
    events: (runId) => fetchSimulationEvents(runId),
    pin: async (runId, pinned) => void (await pinSimulationRun(runId, pinned)),
  }
}

export type DemoSourceOptions = {
  // demo is the key the demo's store and runs go by.
  demo: string
  state: () => State
  // fingerprint moves with the demo's records, so the list is read again and stale runs are marked.
  fingerprint: string
  runs: DemoRuns
  catalog: () => Promise<EngineCatalog | null>
}

// runStale tells whether a kept run was made for other inputs than its variant has now. A run that recorded the hash
// of its variant is judged by it; a run on the suggestion, or an older one, by the whole demo.
export function runStale(run: DemoRun, hash: string, variants: Record<string, string>): boolean {
  const { variant_id: id, variant_hash: made } = run.summary
  if (id && id !== 'suggestion' && made) {
    return variants[id] !== made
  }
  return run.hash !== hash
}

function itemOf(demo: string, run: DemoRun, stale: boolean): SimulationListItem {
  const r = run.summary.result
  return {
    id: run.id,
    project_id: demo,
    run_id: run.id,
    status: 'succeeded',
    progress_pct: 100,
    attempt: 1,
    max_attempts: 1,
    error_text: null,
    replications_total: r.replications,
    replications_done: r.replications,
    config: { variant_id: run.summary.variant_id, sim: run.config },
    created_at: run.createdAt,
    updated_at: run.createdAt,
    started_at: run.createdAt,
    finished_at: run.createdAt,
    canceled_at: null,
    input_hash: run.hash,
    seed: r.config.seed ?? 0,
    sim_version: run.summary.snapshot.sim_version,
    project_version_id: '',
    stale_vs_draft: stale,
    brief: {
      verdict: r.verdict,
      verdict_text: r.verdict_text,
      kpi: r.kpi,
      fleet: r.fleet,
      variant_name: run.summary.variant_name,
      map_source: run.summary.map_source,
    },
  }
}

function responseOf(demo: string, run: DemoRun, stale: boolean, log: SimulationLog | null): SimulationRunResponse {
  return {
    run: {
      id: run.id,
      project_id: demo,
      project_version_id: '',
      input_hash: run.hash,
      match_version: run.summary.snapshot.match_version,
      econ_version: run.summary.snapshot.econ_version,
      sim_version: run.summary.snapshot.sim_version,
      seed: run.summary.result.config.seed ?? 0,
      status: 'succeeded',
      created_at: run.createdAt,
      version_no: 0,
      confidence_level: run.summary.confidence_level as SimulationRunResponse['run']['confidence_level'],
    },
    job: null,
    stale_vs_draft: stale,
    summary: run.summary,
    replications: [],
    artifact: log
      ? { kind: 'event_log', encoding: 'json', size_bytes: 0, raw_bytes: 0, event_count: log.events.length, pinned: true, expires_at: null, created_at: run.createdAt }
      : null,
  }
}

export function demoSource(o: DemoSourceOptions): SimSource {
  const find = async (runId: string) => {
    const run = (await o.runs.list(o.demo)).find((r) => r.id === runId)
    if (!run) {
      throw new Error('Запуск не найден. Он мог быть вытеснен новыми: демо хранит пять последних.')
    }
    return run
  }
  return {
    key: ['demo-simulations', o.demo, o.fingerprint],
    local: true,
    poll: false,
    async list() {
      const [runs, hash, variants] = await Promise.all([o.runs.list(o.demo), draftHash(o.state()), variantHashes(o.state())])
      return runs.map((r) => itemOf(o.demo, r, runStale(r, hash, variants)))
    },
    async create(variantId, config, extra) {
      const catalog = await o.catalog()
      if (!catalog) {
        throw new Error(NoCatalog)
      }
      const release = holdReload()
      try {
        const out = await simulateMap({
          catalog,
          collections: o.state(),
          variant_id: variantId,
          sim: config,
          with_log: true,
          mode: extra?.line_key ? 'fleet_search' : undefined,
          line_key: extra?.line_key,
        })
        if (out.status === 'refused') {
          throw new SimRefused(out.message, out.issues)
        }
        const run: DemoRun = {
          id: uuid(),
          createdAt: new Date().toISOString(),
          hash: out.summary.input_hash,
          config: out.summary.result.config,
          summary: out.summary,
        }
        await o.runs.put(o.demo, run, out.log ?? null)
        return []
      } finally {
        release()
      }
    },
    async cancel() {
      stopSimulation()
    },
    async run(runId) {
      const [run, hash, variants, log] = await Promise.all([find(runId), draftHash(o.state()), variantHashes(o.state()), o.runs.log(o.demo, runId)])
      return responseOf(o.demo, run, runStale(run, hash, variants), log)
    },
    async events(runId) {
      const log = await o.runs.log(o.demo, runId)
      if (!log) {
        throw new Error('Журнал этого запуска не сохранён. Запустите симуляцию заново.')
      }
      return log
    },
  }
}
