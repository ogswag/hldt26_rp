import type { SimStat, SimulationSummary } from '../api/client'
import { csvNumber } from '../ui/download'

import { policyLabels } from './format'
import { formatClock, type Replay } from './replay'

type Row = (string | number)[]

const header: Row = ['Раздел', 'Показатель', 'Значение', 'Минимум', 'Максимум', 'Единица']

function stat(section: string, name: string, s: SimStat, unit: string, digits: number, scale = 1): Row {
  return [section, name, csvNumber(s.median * scale, digits), csvNumber(s.min * scale, digits), csvNumber(s.max * scale, digits), unit]
}

// runCsvRows lists what a simulation run reports as one table: totals, per process, bottlenecks and the check
// against the economics. Numbers carry a decimal comma and their unit sits in its own column.
export function runCsvRows(summary: SimulationSummary): Row[] {
  const r = summary.result
  const k = r.kpi
  const rows: Row[] = [header]

  rows.push(['Итог', 'Вариант', summary.variant_name, '', '', ''])
  rows.push(['Итог', 'Вывод', r.verdict_text, '', '', ''])
  rows.push(['Итог', 'Режим', r.config.mode === 'deterministic' ? 'Детерминированный прогон' : `Повторов: ${r.replications}`, '', '', ''])
  rows.push(['Итог', 'Политика назначения', policyLabels[r.config.policy ?? 'fifo'], '', '', ''])
  rows.push(['Итог', 'Горизонт', csvNumber(r.horizon_s / 3600, 2), '', '', 'ч'])
  rows.push(['Итог', 'Допуск нарушений SLA', csvNumber(r.sla_target_pct, 1), '', '', '%'])

  rows.push(stat('Показатели', 'Заданий поступило', k.arrived, 'заданий', 0))
  rows.push(stat('Показатели', 'Заданий выполнено', k.completed, 'заданий', 0))
  rows.push(stat('Показатели', 'Производительность', k.throughput_per_h, 'заданий в час', 1))
  rows.push(stat('Показатели', 'Доля нарушений SLA', k.violation_rate, '%', 1, 100))
  rows.push(['Показатели', 'Доля нарушений SLA, 90-й процентиль', csvNumber(k.violation_rate.p90 * 100, 1), '', '', '%'])
  rows.push(stat('Показатели', 'Ожидание назначения, среднее', k.wait_mean_s, 'с', 0))
  rows.push(stat('Показатели', 'Ожидание назначения, 95-й процентиль', k.wait_p95_s, 'с', 0))
  rows.push(stat('Показатели', 'Цикл задания, среднее', k.cycle_mean_s, 'с', 0))
  rows.push(stat('Показатели', 'Цикл задания, 95-й процентиль', k.cycle_p95_s, 'с', 0))
  rows.push(stat('Показатели', 'Очередь заданий, средняя', k.queue_mean, 'заданий', 1))
  rows.push(stat('Показатели', 'Очередь заданий, максимум', k.queue_max, 'заданий', 0))
  rows.push(stat('Показатели', 'Загрузка флота', k.fleet_utilization, '%', 1, 100))
  rows.push(stat('Показатели', 'Доля времени на зарядке', k.charging_share, '%', 1, 100))
  rows.push(stat('Показатели', 'Доля времени в простое', k.idle_share, '%', 1, 100))
  rows.push(stat('Показатели', 'Пробег флота', k.distance_km, 'км', 1))

  for (const p of r.processes) {
    const section = `Процесс: ${p.name}`
    if (!p.covered) {
      rows.push([section, 'Состояние', 'не моделируется: нет подходящих роботов или потока на карте', '', '', ''])
      continue
    }
    rows.push([section, 'Ожидалось заданий', csvNumber(p.expected_jobs, 0), '', '', 'заданий'])
    rows.push(stat(section, 'Поступило', p.arrived, 'заданий', 0))
    rows.push(stat(section, 'Выполнено', p.completed, 'заданий', 0))
    rows.push(stat(section, 'Доля нарушений SLA', p.violation_rate, '%', 1, 100))
    rows.push(stat(section, 'Ожидание назначения, 95-й процентиль', p.wait_p95_s, 'с', 0))
    rows.push(stat(section, 'Цикл задания, 95-й процентиль', p.cycle_p95_s, 'с', 0))
    rows.push(stat(section, 'Очередь, максимум', p.queue_max, 'заданий', 0))
  }

  for (const b of r.bottlenecks) {
    rows.push(['Узкие места', b.name, b.text, '', '', ''])
  }

  if (summary.econ_check) {
    const c = summary.econ_check
    rows.push(['Сверка с экономикой', 'Ожидалось заданий', csvNumber(c.expected_jobs, 0), '', '', 'заданий'])
    rows.push(['Сверка с экономикой', 'Выполнено заданий', csvNumber(c.completed, 0), '', '', 'заданий'])
    rows.push(['Сверка с экономикой', 'Доля выполненных', csvNumber(c.coverage * 100, 1), '', '', '%'])
    rows.push(['Сверка с экономикой', 'Вывод', c.text, '', '', ''])
  }
  return rows
}

// counters are the values the replay shows next to its clock at one moment.
export type FrameCounters = { queue: number; completed: number; violated: number }

// frameCaption words the strip under a saved frame: the variant, the moment and the running counters.
export function frameCaption(variant: string, t: number, replay: Pick<Replay, 'horizon'>, c: FrameCounters): string[] {
  return [
    variant,
    `${formatClock(t)} из ${formatClock(replay.horizon)}`,
    `Заданий в очереди ${c.queue}, выполнено ${c.completed}, с нарушением SLA ${c.violated}`,
  ]
}

const fileNameUnsafe = /[\\/:*?"<>|\s]+/g

// exportFileName joins the variant name and a suffix into a file name without characters a file system rejects.
export function exportFileName(variant: string, part: string, ext: string): string {
  const base = `${variant}-${part}`.replace(fileNameUnsafe, '-').replace(/-+/g, '-').replace(/^-|-$/g, '')
  return `${base || 'simulation'}.${ext}`
}
