import { describe, expect, it } from 'vitest'

import type { SimStat, SimulationSummary } from '../api/client'
import { csvText } from '../ui/download'

import { exportFileName, frameCaption, runCsvRows } from './exportRun'

const stat = (median: number, min = median, max = median): SimStat => ({ median, min, max, p10: min, p90: max })

function summary(): SimulationSummary {
  const kpi = Object.fromEntries(
    [
      'arrived',
      'completed',
      'violated',
      'violation_rate',
      'throughput_per_h',
      'wait_mean_s',
      'wait_p95_s',
      'cycle_mean_s',
      'cycle_p95_s',
      'queue_mean',
      'queue_max',
      'fleet_utilization',
      'charging_share',
      'idle_share',
      'distance_km',
      'battery_depleted',
      'dispatch_wait_s',
    ].map((k) => [k, stat(0)]),
  )
  kpi.completed = stat(96, 90, 99)
  kpi.throughput_per_h = stat(12.5, 11.25, 13.75)
  kpi.violation_rate = stat(0.045, 0.02, 0.07)
  return {
    variant_name: 'Вариант 2',
    result: {
      config: { mode: 'stochastic', policy: 'nearest' },
      replications: 5,
      horizon_s: 28800,
      sla_target_pct: 5,
      verdict_text: 'Флот справляется.',
      kpi,
      processes: [
        { code: 'p1', name: 'Приёмка; палеты', covered: true, expected_jobs: 100, arrived: stat(100), completed: stat(96), violation_rate: stat(0.04), wait_p95_s: stat(30), cycle_p95_s: stat(120), queue_max: stat(3) },
        { code: 'p2', name: 'Отгрузка', covered: false },
      ],
      bottlenecks: [{ name: 'Узкий проход', text: 'Роботы ждут в проходе.' }],
    },
    econ_check: { expected_jobs: 100, completed: 96, coverage: 0.96, flag: false, text: 'Флот выполнил 96% заданий.' },
  } as unknown as SimulationSummary
}

describe('runCsvRows', () => {
  const rows = runCsvRows(summary())
  const find = (section: string, name: string) => rows.find((r) => r[0] === section && r[1] === name)

  it('writes numbers with a decimal comma and the unit apart', () => {
    expect(find('Показатели', 'Производительность')).toEqual(['Показатели', 'Производительность', '12,5', '11,3', '13,8', 'заданий в час'])
  })

  it('turns shares into percent', () => {
    expect(find('Показатели', 'Доля нарушений SLA')).toEqual(['Показатели', 'Доля нарушений SLA', '4,5', '2', '7', '%'])
  })

  it('marks a process the run could not model', () => {
    expect(find('Процесс: Отгрузка', 'Состояние')?.[2]).toContain('не моделируется')
  })

  it('carries the check against the economics', () => {
    expect(find('Сверка с экономикой', 'Доля выполненных')).toEqual(['Сверка с экономикой', 'Доля выполненных', '96', '', '', '%'])
  })

  it('quotes a cell that holds the separator', () => {
    const text = csvText(rows)
    expect(text.startsWith('﻿')).toBe(true)
    expect(text).toContain('"Процесс: Приёмка; палеты"')
  })
})

describe('frameCaption', () => {
  it('names the variant, the moment and the counters', () => {
    expect(frameCaption('Вариант 2', 5430, { horizon: 28800 }, { queue: 3, completed: 41, violated: 1 })).toEqual([
      'Вариант 2',
      '01:30:30 из 08:00:00',
      'Заданий в очереди 3, выполнено 41, с нарушением SLA 1',
    ])
  })
})

describe('exportFileName', () => {
  it('drops characters a file system rejects', () => {
    expect(exportFileName('Вариант 2: buy/raas', '01-30-30', 'png')).toBe('Вариант-2-buy-raas-01-30-30.png')
  })

  it('falls back when nothing is left', () => {
    expect(exportFileName('', '', 'csv')).toBe('simulation.csv')
  })
})
