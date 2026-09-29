import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { EngineCatalog, SimulationConfig, SimulationLog, SimulationSummary } from '../api/client'
import type { State } from '../store/apply'

const engine = vi.hoisted(() => ({
  simulateMap: vi.fn(),
  draftHash: vi.fn(async () => 'hash-now'),
  variantHashes: vi.fn(async () => ({ v1: 'variant-now' }) as Record<string, string>),
  stopSimulation: vi.fn(),
}))
vi.mock('../engine/client', () => engine)
vi.mock('../offline/serviceWorker', () => ({ holdReload: () => () => {} }))

import { memoryRuns } from './demoRuns'
import { SimRefused, demoSource, runStale } from './source'

const catalog = { content_sha256: 'c', candidates: [], robots: [] } as unknown as EngineCatalog
const state: State = { project: { project: { id: 'project' } } }
const config = { seed: 1 } as SimulationConfig

function summary(hash: string): SimulationSummary {
  return {
    input_hash: hash,
    variant_id: 'v1',
    variant_name: 'AMR комплектация',
    map_source: 'project',
    result: { config, replications: 1, verdict: 'pass', verdict_text: '', kpi: {}, fleet: [] },
    snapshot: { match_version: 'm', econ_version: 'e', sim_version: 's' },
    confidence_level: 'medium',
  } as unknown as SimulationSummary
}

const kept = (id: string, made: string | undefined, hash = 'hash-then') =>
  ({ id: 'r', createdAt: '', hash, config, summary: { ...summary(hash), variant_id: id, variant_hash: made } }) as unknown as import('./demoRuns').DemoRun

function source(runs = memoryRuns()) {
  return { runs, src: demoSource({ demo: 'demo:warehouse', state: () => state, fingerprint: 'f', runs, catalog: async () => catalog }) }
}

describe('runStale', () => {
  it('judges a run by the hash of its variant, so another edit leaves it current', () => {
    expect(runStale(kept('v1', 'variant-now'), 'hash-now', { v1: 'variant-now' })).toBe(false)
    expect(runStale(kept('v1', 'variant-before'), 'hash-then', { v1: 'variant-now' })).toBe(true)
  })

  it('takes a variant that is gone as stale', () => {
    expect(runStale(kept('v9', 'variant-now'), 'hash-now', { v1: 'variant-now' })).toBe(true)
  })

  it('judges a run on the suggestion, and one without a variant hash, by the whole demo', () => {
    expect(runStale(kept('suggestion', 'x', 'hash-now'), 'hash-now', {})).toBe(false)
    expect(runStale(kept('suggestion', 'x', 'hash-then'), 'hash-now', {})).toBe(true)
    expect(runStale(kept('v1', undefined, 'hash-now'), 'hash-now', { v1: 'variant-now' })).toBe(false)
    expect(runStale(kept('v1', undefined, 'hash-then'), 'hash-now', { v1: 'variant-now' })).toBe(true)
  })
})

describe('demo simulation source', () => {
  beforeEach(() => {
    engine.simulateMap.mockReset()
    engine.stopSimulation.mockReset()
  })

  it('keeps a finished run and its journal, and lists it', async () => {
    const log = { seed: 1, events: [] } as unknown as SimulationLog
    engine.simulateMap.mockResolvedValue({ status: 'done', summary: summary('hash-then'), log })
    const { src } = source()
    expect(await src.create('v1', config)).toEqual([])
    const [item] = await src.list()
    expect(item.status).toBe('succeeded')
    expect(item.stale_vs_draft).toBe(true)
    expect(await src.events(item.run_id as string)).toEqual(log)
    const shown = await src.run(item.run_id as string)
    expect(shown.stale_vs_draft).toBe(true)
    expect(shown.summary?.variant_name).toBe('AMR комплектация')
  })

  it('marks a run made from the current records as fresh', async () => {
    engine.simulateMap.mockResolvedValue({ status: 'done', summary: summary('hash-now'), log: { seed: 1, events: [] } })
    const { src } = source()
    await src.create(undefined, config)
    expect((await src.list())[0].stale_vs_draft).toBe(false)
  })

  it('turns a refusal into an error that carries the map issues', async () => {
    const issues = [{ level: 'error', code: 'no_route', message: 'Нет маршрута' }]
    engine.simulateMap.mockResolvedValue({ status: 'refused', message: 'Карта не прошла проверку.', issues })
    const { src, runs } = source()
    const err = await src.create('v1', config).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(SimRefused)
    expect((err as SimRefused).issues).toEqual(issues)
    expect(await runs.list('demo:warehouse')).toHaveLength(0)
  })

  it('says so when the run was pushed out by newer ones', async () => {
    const { src } = source()
    await expect(src.run('gone')).rejects.toThrow(/пять последних/)
  })

  it('cancels by stopping the engine', async () => {
    const { src } = source()
    await src.cancel('any')
    expect(engine.stopSimulation).toHaveBeenCalledTimes(1)
  })
})
