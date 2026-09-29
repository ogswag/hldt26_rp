import type { CatalogField, Solution, SolutionSpecs } from '../api/client'
import { formatRub, missingSpecs } from '../econ/view'
import { noData } from '../ui/noData'
import { numberText } from '../ui/numberText'
import { familyOf } from './families'
import { choiceLabel } from './fields'
import { statusLabels } from './labels'

// solutionValue reads one catalog field of a robot in stored units.
export function solutionValue(s: Solution, code: string): unknown {
  if (code in s.specs) {
    return s.specs[code as keyof SolutionSpecs]
  }
  return (s as unknown as Record<string, unknown>)[code] ?? null
}

export function choiceOptions(f: CatalogField): { value: string; label: string }[] {
  return (f.choices ?? []).map((c) => ({ value: c.code, label: choiceLabel(f, c.code) }))
}

export function statusText(s: Solution): string | null {
  return s.status ? (statusLabels[s.status] ?? null) : null
}

// sourceHost is the site a source link leads to, the way the robot overlay names it.
export function sourceHost(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return url
  }
}

function withUnit(v: number | null, unit: string): string {
  return v === null ? noData : `${numberText(v)} ${unit}`
}

// sizeText writes width, length and height in that order; a missing one stays a «?» in its place.
export function sizeText(s: Solution['specs']): string {
  const parts = [s.width_mm, s.length_mm, s.height_mm]
  if (parts.every((p) => p === null)) {
    return noData
  }
  return `${parts.map((p) => (p === null ? '?' : numberText(p))).join(' × ')} мм`
}

// robotFacts are the rows of the hover card, in the order of the design decision (H3).
export function robotFacts(s: Solution): [string, string][] {
  return [
    ['Тип', familyOf(s.family).label],
    ['Подтип', s.subtype ?? noData],
    ...(s.modification ? ([['Отрасль', s.modification]] as [string, string][]) : []),
    ['Груз', withUnit(s.specs.payload_kg, 'кг')],
    ['Ш × Д × В', sizeText(s.specs)],
    ['Скорость', withUnit(s.specs.speed_mps, 'м/с')],
    ['Работа', withUnit(s.specs.endurance_h, 'ч')],
    ['Нет в ТТХ', missingSpecs(s.data_quality.missing) || 'всё указано'],
    ['Цена', formatRub(s.price_rub)],
  ]
}
