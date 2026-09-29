import type { CatalogField, MatchItem, Solution } from '../api/client'
import { formatRub, missingSpecs } from '../econ/view'
import { dateText, fieldByCode, choiceLabel, valueText } from '../catalog/fields'
import { familyOf } from '../catalog/families'
import { solutionValue, sourceHost, statusText } from '../catalog/values'
import { noData } from '../ui/noData'
import { numberText } from '../ui/numberText'
import { estimatePayback, verdicts } from './format'

// CompareCell is one robot's value in a row; a link leads to the source, a tag marks the calculation's verdict.
export type CompareCell = { text: string; href?: string; tag?: string }
// A clamped row holds long text, so its cells stop after two lines and show whole on hover.
export type CompareRow = { label: string; cells: CompareCell[]; clamp?: boolean }
export type CompareGroup = { title: string; rows: CompareRow[] }

// Codes of the specs, split the way the organizers group them: the robot itself, then what it needs from the site.
const techCodes = ['payload_kg', 'mass_kg', 'length_mm', 'width_mm', 'height_mm', 'speed_mps', 'endurance_h', 'nav_type', 'pos_accuracy_mm', 'temp_min_c', 'temp_max_c']
const siteCodes = ['charge_min', 'min_aisle_mm', 'turn_radius_mm']
const lifeCodes = ['lifetime_years', 'service_pct_year']

const plain = (text: string): CompareCell => ({ text })

// compareGroups lays robots side by side: identity, specs, infrastructure, economy, fit for the project and how
// well the data is sourced. A row that no robot has a value for is left out; «Применимость» needs a calculation.
export function compareGroups(
  robots: readonly Solution[],
  items: ReadonlyMap<string, MatchItem> | null,
  fields: readonly CatalogField[],
): CompareGroup[] {
  const value = (s: Solution, code: string): CompareCell => plain(valueText(fieldByCode(fields, code), solutionValue(s, code), noData))
  const codes = (list: string[]): CompareRow[] =>
    list.flatMap((code) => {
      const f = fieldByCode(fields, code)
      return f ? [{ label: f.label, cells: robots.map((s) => value(s, code)) }] : []
    })
  const known = (s: Solution, code: string) => {
    const v = solutionValue(s, code)
    return v !== null && v !== undefined && v !== ''
  }
  const trust = fieldByCode(fields, 'confidence')
  const sourced = (s: Solution): string => {
    const codes = [...fields.filter((f) => f.group === 'specs').map((f) => f.code), 'price_rub'].filter((c) => known(s, c))
    const own = codes.filter((c) => s.field_sources?.[c] !== undefined).length
    return `${numberText(own)} из ${numberText(codes.length)}`
  }
  const estimates = robots.map((s) => items?.get(s.id) ?? null)

  const groups: CompareGroup[] = [
    {
      title: 'Идентификация',
      rows: [
        { label: 'Компания', cells: robots.map((s) => plain(s.vendor ?? noData)) },
        { label: 'Тип', cells: robots.map((s) => plain(familyOf(s.family).label)) },
        { label: 'Подтип', cells: robots.map((s) => plain(s.subtype ?? noData)) },
        { label: 'Отрасль', cells: robots.map((s) => plain(s.modification ?? noData)) },
        { label: 'Статус', cells: robots.map((s) => plain(statusText(s) ?? noData)) },
        ...codes(['object_types']),
      ],
    },
    { title: 'Технические характеристики', rows: codes(techCodes) },
    { title: 'Инфраструктура', rows: codes(siteCodes) },
    {
      title: 'Экономика',
      rows: [
        { label: 'Цена', cells: robots.map((s) => plain(formatRub(s.price_rub))) },
        ...codes(lifeCodes),
        ...(items
          ? [
              { label: 'Флот в расчёте', cells: estimates.map((i) => plain(i?.estimate ? `${numberText(i.estimate.fleet_size)} шт` : noData)) },
              { label: 'CAPEX', cells: estimates.map((i) => plain(i?.estimate ? formatRub(i.estimate.capex_rub) : noData)) },
              { label: 'Окупаемость', cells: estimates.map((i) => plain(estimatePayback(i) || noData)) },
            ]
          : []),
      ],
    },
    ...(items
      ? [
          {
            title: 'Применимость',
            rows: [
              {
                label: 'Оценка',
                cells: estimates.map((i): CompareCell => {
                  const v = i ? verdicts[i.status] : undefined
                  return v ? { text: v.label, tag: v.tag } : plain(noData)
                }),
              },
              { label: 'Замечания', cells: estimates.map((i) => plain(i && i.reasons.length > 0 ? i.reasons.join(' ') : noData)), clamp: true },
            ],
          },
        ]
      : []),
    {
      title: 'Качество данных',
      rows: [
        { label: 'Нет в ТТХ', cells: robots.map((s) => plain(missingSpecs(s.data_quality.missing) || 'всё указано')) },
        {
          label: 'Источник',
          cells: robots.map((s): CompareCell => (s.source_url ? { text: sourceHost(s.source_url), href: s.source_url } : plain(noData))),
        },
        {
          label: 'Достоверность',
          cells: robots.map((s) => plain(trust && s.specs.confidence ? choiceLabel(trust, s.specs.confidence) : noData)),
        },
        { label: 'Дата источника', cells: robots.map((s) => plain(s.specs.sourced_at ? dateText(s.specs.sourced_at) : noData)) },
        { label: 'Значений со своим источником', cells: robots.map((s) => plain(sourced(s))) },
      ],
    },
  ]
  return groups
    .map((g) => ({ ...g, rows: g.rows.filter((r) => r.cells.some((c) => c.text !== noData)) }))
    .filter((g) => g.rows.length > 0)
}
