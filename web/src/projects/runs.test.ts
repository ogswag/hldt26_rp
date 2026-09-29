import { describe, expect, it } from 'vitest'

import type { RunHistoryItem } from '../api/client'

import { canExportRun, canOpenRun, confidenceLabel, confidenceLevels, runBriefText, runOpenHref, runReference, runStatusLabel } from './runs'

function item(over: Partial<RunHistoryItem>): RunHistoryItem {
  return {
    id: 'run-1',
    kind: 'calculation',
    status: 'succeeded',
    project_id: 'p1',
    project_version_id: 'v1',
    version_no: 1,
    input_hash: 'a'.repeat(64),
    match_version: 'match-v2',
    econ_version: 'econ-v3',
    sim_version: 'sim-v1',
    seed: 0,
    confidence_level: 'preliminary',
    created_at: null,
    is_current: false,
    job_id: null,
    stale_vs_draft: false,
    brief: {},
    ...over,
  }
}

describe('confidence labels', () => {
  it('names every level and falls back to preliminary', () => {
    const names = new Set(confidenceLevels.map((l) => confidenceLabel(l).name))
    expect(names.size).toBe(4)
    expect(confidenceLabel('unknown').name).toBe('Предварительный')
    expect(confidenceLabel(undefined).meaning).toContain('Каталожные данные')
  })
})

describe('run brief', () => {
  it('summarizes a calculation run', () => {
    const text = runBriefText(item({ brief: { variant_names: ['AMR', 'Смешанный'], best_payback_years: 2.34, verification_flag: true } }))
    expect(text).toContain('Варианты: AMR, Смешанный')
    expect(text).toContain('лучшая окупаемость 2,3 года')
    expect(text).toContain('расходятся больше 15%')
  })

  it('says when no variant pays back', () => {
    expect(runBriefText(item({ brief: { variant_names: [], solution_name: 'H1500', best_payback_years: null } }))).toBe(
      'Решение: H1500, окупаемости нет.',
    )
  })

  it('summarizes a finished simulation', () => {
    const text = runBriefText(
      item({ kind: 'simulation', brief: { variant_name: 'AMR', verdict: 'fail', throughput_per_h: 42.26, violation_rate: 0.125, map_source: 'template' } }),
    )
    expect(text).toBe('Вариант AMR, SLA нарушается, 42,3 заданий в час, нарушений 12,5%, шаблон карты.')
  })

  it('does not invent results for an unfinished simulation', () => {
    expect(runBriefText(item({ kind: 'simulation', status: 'queued', brief: { variant_name: 'AMR' } }))).toBe('Вариант AMR.')
  })
})

describe('run navigation', () => {
  it('uses a project-local reference instead of the internal id', () => {
    expect(runReference(item({ version_no: 12 }))).toBe('Расчёт №12')
    expect(runReference(item({ kind: 'simulation', version_no: 13 }))).toBe('Симуляция №13')
  })

  it('opens calculations in history and simulations on the sim page', () => {
    expect(runOpenHref('p1', { id: 'r1', kind: 'calculation' })).toBe('/p/p1/calc/history/r1')
    expect(runOpenHref('p1', { id: 'r2', kind: 'simulation' })).toBe('/p/p1/calc/sim?run=r2')
  })

  it('exports only finished runs', () => {
    expect(canExportRun({ status: 'succeeded' })).toBe(true)
    expect(canExportRun({ status: 'canceled' })).toBe(false)
    expect(canOpenRun({ kind: 'simulation', status: 'failed' })).toBe(false)
    expect(canOpenRun({ kind: 'calculation', status: 'succeeded' })).toBe(true)
    expect(runStatusLabel('canceled')).toBe('отменено')
  })
})
