import type { ConfidenceLevel, RunHistoryItem, RunKind, RunStatus } from '../api/client'
import { formatYears } from '../econ/view'

export const confidenceLevels: ConfidenceLevel[] = ['preliminary', 'configured', 'calibrated', 'validated']

const confidenceText: Record<ConfidenceLevel, { name: string; meaning: string }> = {
  preliminary: {
    name: 'Предварительный',
    meaning: 'Каталожные данные и нормативы. Фактические маршруты, спрос и длительности не подтверждены.',
  },
  configured: {
    name: 'Настроенный',
    meaning: 'Маршруты, спрос и длительности введены пользователем. Параметры не оценены по журналам или замерам.',
  },
  calibrated: {
    name: 'Калиброванный',
    meaning: 'Параметры оценены по журналам или замерам объекта.',
  },
  validated: {
    name: 'Подтверждённый',
    meaning: 'Прогноз сопоставлен с фактическим процессом и прошёл порог ошибки.',
  },
}

export function confidenceLabel(level: string | undefined): { name: string; meaning: string } {
  if (level && level in confidenceText) {
    return confidenceText[level as ConfidenceLevel]
  }
  return confidenceText.preliminary
}

export function runKindLabel(kind: RunKind): string {
  return kind === 'simulation' ? 'Симуляция' : 'Расчёт экономики'
}

export function runReference(run: Pick<RunHistoryItem, 'kind' | 'version_no'>): string {
  return `${run.kind === 'simulation' ? 'Симуляция' : 'Расчёт'} №${run.version_no}`
}

const statusText: Record<RunStatus, string> = {
  queued: 'в очереди',
  running: 'считается',
  succeeded: 'готово',
  failed: 'ошибка',
  canceled: 'отменено',
}

export function runStatusLabel(status: string): string {
  return statusText[status as RunStatus] ?? status
}

function num(v: number, digits: number): string {
  return v.toLocaleString('ru-RU', { maximumFractionDigits: digits, minimumFractionDigits: digits, useGrouping: true })
}

export function runBriefText(item: RunHistoryItem): string {
  const b = item.brief ?? {}
  if (item.kind === 'simulation') {
    if (item.status !== 'succeeded') {
      return b.variant_name ? `Вариант ${b.variant_name}.` : ''
    }
    const parts: string[] = []
    if (b.variant_name) {
      parts.push(`Вариант ${b.variant_name}`)
    }
    if (b.verdict) {
      parts.push(b.verdict === 'pass' ? 'SLA выполняется' : 'SLA нарушается')
    }
    if (typeof b.throughput_per_h === 'number') {
      parts.push(`${num(b.throughput_per_h, 1)} заданий в час`)
    }
    if (typeof b.violation_rate === 'number') {
      parts.push(`нарушений ${num(b.violation_rate * 100, 1)}%`)
    }
    if (b.map_source) {
      parts.push(b.map_source === 'project' ? 'карта проекта' : 'шаблон карты')
    }
    return parts.length > 0 ? `${parts.join(', ')}.` : ''
  }
  const parts: string[] = []
  const names = (b.variant_names ?? []).filter((n): n is string => typeof n === 'string' && n !== '')
  if (names.length > 0) {
    parts.push(`Варианты: ${names.join(', ')}`)
  } else if (b.solution_name) {
    parts.push(`Решение: ${b.solution_name}`)
  }
  if (typeof b.best_payback_years === 'number') {
    parts.push(`лучшая окупаемость ${formatYears(b.best_payback_years, 1)}`)
  } else {
    parts.push('окупаемости нет')
  }
  if (b.verification_flag) {
    parts.push('экономика и симуляция расходятся больше 15%')
  }
  return `${parts.join(', ')}.`
}

export function runOpenHref(projectId: string, item: Pick<RunHistoryItem, 'id' | 'kind'>): string {
  if (item.kind === 'simulation') {
    return `/p/${projectId}/calc/sim?run=${item.id}`
  }
  return `/p/${projectId}/calc/history/${item.id}`
}

export function canOpenRun(item: Pick<RunHistoryItem, 'kind' | 'status'>): boolean {
  return item.kind === 'calculation' || item.status === 'succeeded'
}

export function canExportRun(item: Pick<RunHistoryItem, 'status'>): boolean {
  return item.status === 'succeeded'
}

export function formatDateTime(value: string | null | undefined): string {
  if (!value) {
    return ''
  }
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) {
    return ''
  }
  return d.toLocaleString('ru-RU')
}
