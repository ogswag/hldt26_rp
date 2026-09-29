import { describe, expect, it } from 'vitest'

import type { SimulationConfig, SimulationLog, SimulationSummary } from '../api/client'

import { KeptRuns, memoryRuns, type DemoRun } from './demoRuns'

function run(n: number): DemoRun {
  return {
    id: `run-${n}`,
    createdAt: `2026-09-29T10:00:${String(n).padStart(2, '0')}.000Z`,
    hash: 'h',
    config: {} as SimulationConfig,
    summary: { run_id: `run-${n}` } as SimulationSummary,
  }
}

const log = (n: number) => ({ seed: n }) as SimulationLog

describe('demo runs', () => {
  it('lists the newest run first', async () => {
    const runs = memoryRuns()
    await runs.put('demo:warehouse', run(1), null)
    await runs.put('demo:warehouse', run(2), null)
    expect((await runs.list('demo:warehouse')).map((r) => r.id)).toEqual(['run-2', 'run-1'])
  })

  it('keeps five runs and drops the oldest with its journal', async () => {
    const runs = memoryRuns()
    for (let n = 1; n <= KeptRuns + 2; n++) {
      await runs.put('demo:warehouse', run(n), log(n))
    }
    const kept = (await runs.list('demo:warehouse')).map((r) => r.id)
    expect(kept).toHaveLength(KeptRuns)
    expect(kept[0]).toBe(`run-${KeptRuns + 2}`)
    expect(await runs.log('demo:warehouse', 'run-1')).toBeNull()
    expect(await runs.log('demo:warehouse', 'run-2')).toBeNull()
    expect(await runs.log('demo:warehouse', 'run-3')).toEqual(log(3))
  })

  it('counts each demo on its own', async () => {
    const runs = memoryRuns()
    for (let n = 1; n <= KeptRuns; n++) {
      await runs.put('demo:warehouse', run(n), null)
    }
    await runs.put('demo:airport', run(9), null)
    expect(await runs.list('demo:warehouse')).toHaveLength(KeptRuns)
    expect((await runs.list('demo:airport')).map((r) => r.id)).toEqual(['run-9'])
  })

  it('returns copies, so a page cannot change what is kept', async () => {
    const runs = memoryRuns()
    await runs.put('demo:warehouse', run(1), log(1))
    const [first] = await runs.list('demo:warehouse')
    first.hash = 'changed'
    expect((await runs.list('demo:warehouse'))[0].hash).toBe('h')
  })
})
