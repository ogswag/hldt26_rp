import type { FleetItem } from '../api/client'

const pluralRules = new Intl.PluralRules('ru-RU')

export function plural(n: number, one: string, few: string, many: string): string {
  const form = pluralRules.select(n)
  if (form === 'one') {
    return one
  }
  return form === 'few' ? few : many
}

const rubCompact = new Intl.NumberFormat('ru-RU', {
  style: 'currency',
  currency: 'RUB',
  notation: 'compact',
  maximumFractionDigits: 1,
})

const rubWhole = new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB', maximumFractionDigits: 0, useGrouping: true })

export function rubShort(v: number): string {
  return rubCompact.format(v)
}

export function rubExact(v: number): string {
  return rubWhole.format(v)
}

// fleetSummary counts robots and distinct models, or returns '' for an empty fleet.
export function fleetSummary(fleet: FleetItem[]): string {
  const withModel = fleet.filter((f) => f.solution_id)
  if (withModel.length === 0) {
    return ''
  }
  const robots = withModel.reduce((n, f) => n + f.quantity, 0)
  const models = new Set(withModel.map((f) => f.solution_id)).size
  return `${robots} ${plural(robots, 'робот', 'робота', 'роботов')}, ${models} ${plural(models, 'модель', 'модели', 'моделей')}`
}
