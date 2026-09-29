import type { CalculateResult, MetricOrigin, ScenarioResult } from '../api/client'
import { plural } from '../fleet/format'
import { numberText } from '../ui/numberText'

export function scenarioLabel(kind: string): string {
  if (kind === 'baseline') {
    return 'База'
  }
  if (kind === 'buy') {
    return 'Покупка'
  }
  if (kind === 'raas') {
    return 'RaaS'
  }
  return kind
}

export function tariffLabel(tariff: string | undefined): string {
  if (tariff === 'variable') {
    return 'переменный'
  }
  if (tariff === 'mixed') {
    return 'смешанный'
  }
  if (tariff === 'fixed') {
    return 'фиксированный'
  }
  return tariff ?? ''
}

export function originFor(origins: MetricOrigin[] | undefined, metric: string): MetricOrigin | undefined {
  if (!origins) {
    return undefined
  }
  return origins.find((o) => o.metric === metric)
}

export function priceSourceLabel(source: string | undefined): string {
  if (source === 'project_override') {
    return 'проектная цена'
  }
  if (source === 'catalog') {
    return 'каталог'
  }
  return source ?? ''
}

export function bandLabel(band: string | undefined): string {
  if (band === 'up_to_3') {
    return 'до 3 лет'
  }
  if (band === 'from_3_to_5') {
    return 'от 3 до 5 лет'
  }
  if (band === 'over_5') {
    return 'более 5 лет'
  }
  if (band === 'none') {
    return 'нет срока'
  }
  return ''
}

const rubFormat = new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB', maximumFractionDigits: 0, useGrouping: true })

// formatRub prints whole rubles with the sign, e.g. 1 200 000 ₽.
export function formatRub(v: number | null | undefined): string {
  if (v === null || v === undefined) {
    return 'нет данных'
  }
  return rubFormat.format(v)
}

// yearsWord agrees «год» with a number; a fraction takes «года», as in 2,5 года.
export function yearsWord(v: number): string {
  if (!Number.isInteger(v)) {
    return 'года'
  }
  return plural(v, 'год', 'года', 'лет')
}

export function formatYears(v: number, digits = 1): string {
  const rounded = Number(v.toFixed(digits))
  return `${formatNum(rounded, digits)} ${yearsWord(rounded)}`
}

export function formatNum(v: number | null | undefined, digits = 4): string {
  if (v === null || v === undefined) {
    return 'нет данных'
  }
  return numberText(v, digits)
}

export function formatPayback(sc: ScenarioResult | undefined): string {
  if (!sc || sc.payback_years === null || sc.payback_years === undefined) {
    if (!sc || sc.kind === 'baseline') {
      return 'нет данных'
    }
    return 'нет срока'
  }
  return formatYears(sc.payback_years, 1)
}

// formatPct prints a percent with one decimal, or the text for a metric the scenario has no value of.
export function formatPct(v: number | null | undefined, none: string): string {
  if (v === null || v === undefined) {
    return none
  }
  return `${formatNum(v, 1)}%`
}

export function formatYearsOrNone(v: number | null | undefined): string {
  if (v === null || v === undefined) {
    return 'нет срока'
  }
  return formatYears(v, 1)
}

export function objectTypeLabel(t: string): string {
  if (t === 'warehouse') {
    return 'Склад'
  }
  if (t === 'airport') {
    return 'Аэропорт'
  }
  if (t === 'hospital') {
    return 'Медучреждение'
  }
  return t
}

export function sensitivityParamLabel(param: string): string {
  if (param === 'equipment_price') {
    return 'Цена оборудования'
  }
  if (param === 'volume') {
    return 'Объём операций'
  }
  if (param === 'labor') {
    return 'ФОТ'
  }
  return param
}

export function bottleneckLabel(id: string): string {
  if (id === 'op_points') {
    return 'точки операций'
  }
  if (id === 'chargers') {
    return 'зарядка'
  }
  if (id === 'none') {
    return 'нет'
  }
  return id
}

export function workKindLabel(kind: string): string {
  if (kind === 'pallet') {
    return 'паллетная перевозка'
  }
  if (kind === 'piece') {
    return 'штучный отбор'
  }
  if (kind === 'cleaner') {
    return 'уборка склада'
  }
  if (kind === 'airport_ramp') {
    return 'перрон'
  }
  if (kind === 'airport_trolley') {
    return 'тележки терминала'
  }
  if (kind === 'airport_cleaner') {
    return 'уборка терминала'
  }
  if (kind === 'hospital_cart') {
    return 'внутрибольничные рейсы'
  }
  if (kind === 'hospital_cleaner') {
    return 'уборка медучреждения'
  }
  return kind
}

export function overrideFieldLabel(field: string): string {
  if (field === 'price_rub') {
    return 'Цена изделия'
  }
  if (field === 'volume_factor') {
    return 'Объём операций'
  }
  if (field === 'labor_factor') {
    return 'ФОТ'
  }
  if (field === 'fleet_size') {
    return 'Флот'
  }
  if (field === 'capex_rub') {
    return 'CAPEX'
  }
  if (field === 'opex_year_rub') {
    return 'OPEX/год'
  }
  return field
}

export function pickedPrice(result: CalculateResult): number | null {
  const sid = result.scenarios.find((s) => s.kind === 'buy')?.solution_id
  if (!sid) {
    return null
  }
  const item = result.match.items.find((i) => i.solution_id === sid)
  return item?.price_rub ?? null
}

// Short names of the specs matching needs, in the order the API checks them. Both temperature bounds read as one.
const missingLabels: Record<string, string> = {
  payload_kg: 'грузоподъёмность',
  width_mm: 'ширина',
  min_aisle_mm: 'мин. проезд',
  temp_min_c: 'температура',
  temp_max_c: 'температура',
}

// missingSpecs names what the catalog lacks for a robot: «мин. проезд, температура». Empty when nothing is missing.
export function missingSpecs(missing: readonly string[]): string {
  return [...new Set(missing.map((m) => missingLabels[m] ?? m))].join(', ')
}

// lineScenarioLabel names the scenario of a cost line: the variant, then the scenario kind and the RaaS tariff.
export function lineScenarioLabel(result: CalculateResult, scenario: string): string {
  const [variantId, kind, tariff] = scenario.split(':')
  const variant = (result.variants ?? []).find((v) => v.variant_id === variantId)?.name
  if (!variant) {
    return scenarioLabel(scenario)
  }
  if (!kind) {
    return variant
  }
  const suffix = kind === 'raas' && tariff ? `${scenarioLabel(kind)}, ${tariffLabel(tariff)}` : scenarioLabel(kind)
  return `${variant} · ${suffix}`
}
