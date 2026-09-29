import type { SimPolicy, SimStat } from '../api/client'
import { formatNum } from '../econ/view'

export const policyLabels: Record<SimPolicy, string> = {
  fifo: 'FIFO: раньше пришло, раньше назначено',
  nearest: 'Ближайший свободный робот',
  sla_priority: 'Приоритет SLA: срочные и близкие к сроку',
}

export function pct(v: number): string {
  return `${formatNum(v * 100, 1)}%`
}

export function dur(s: number): string {
  if (s < 90) {
    return `${formatNum(s, 0)} с`
  }
  if (s < 5400) {
    return `${formatNum(s / 60, 1)} мин`
  }
  return `${formatNum(s / 3600, 1)} ч`
}

export function range(s: SimStat, f: (v: number) => string): string {
  if (s.min === s.max) {
    return f(s.median)
  }
  return `${f(s.median)} (${f(s.min)} - ${f(s.max)})`
}
