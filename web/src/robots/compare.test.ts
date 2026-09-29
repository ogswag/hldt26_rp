import { describe, expect, it } from 'vitest'

import type { CatalogField, MatchItem, Solution } from '../api/client'
import { compareGroups } from './compare'

const fields: CatalogField[] = [
  { code: 'payload_kg', label: 'Грузоподъёмность', unit: 'кг', kind: 'number', group: 'specs', editable: true },
  { code: 'mass_kg', label: 'Масса', unit: 'кг', kind: 'number', group: 'specs', editable: true },
  { code: 'turn_radius_mm', label: 'Радиус разворота', unit: 'мм', kind: 'number', group: 'specs', editable: true },
  { code: 'price_rub', label: 'Цена', unit: '₽', kind: 'number', group: 'offer', editable: true },
  {
    code: 'confidence',
    label: 'Источник ТТХ',
    kind: 'choice',
    group: 'specs',
    editable: true,
    choices: [{ code: 'vendor', label: 'заявлено производителем' }],
  },
]

function robot(id: string, over: Partial<Solution>, specs: Record<string, unknown> = {}): Solution {
  return {
    id,
    name: id,
    vendor: null,
    kind: 'brs',
    subtype: null,
    status: null,
    industry: null,
    scenario: null,
    modification: null,
    price_rub: null,
    source_url: null,
    field_sources: {},
    specs: { confidence: null, sourced_at: null, ...specs },
    data_quality: { status: 'ok', missing: [], reasons: [] },
    ...over,
  } as unknown as Solution
}

function verdict(id: string, status: string, reasons: string[], fleet: number, capex: number, payback: number | null): MatchItem {
  return {
    solution_id: id,
    name: id,
    status,
    reasons,
    estimate: { fleet_size: fleet, capex_rub: capex, payback_years: payback },
  } as unknown as MatchItem
}

// norm folds the no-break and narrow spaces of digit groups and units into one plain space.
const norm = (t: string) => t.replace(/\s/g, ' ')
const rowsOf = (groups: ReturnType<typeof compareGroups>, title: string) => groups.find((g) => g.title === title)?.rows ?? []
const texts = (groups: ReturnType<typeof compareGroups>, title: string, label: string) =>
  rowsOf(groups, title)
    .find((r) => r.label === label)
    ?.cells.map((c) => norm(c.text))

describe('compareGroups', () => {
  const a = robot('а', { vendor: 'Эвокарго', price_rub: 2700000, source_url: 'https://www.evocargo.example/h1500', field_sources: { mass_kg: { source_url: 'x' } } }, { payload_kg: 1500, mass_kg: 1500, confidence: 'vendor' })
  const b = robot('б', { modification: 'Промышленность', price_rub: 3100000 }, { payload_kg: 1000 })

  it('lays out one cell per robot in each row', () => {
    const g = compareGroups([a, b], null, fields)
    expect(texts(g, 'Идентификация', 'Компания')).toEqual(['Эвокарго', 'нет данных'].map(norm))
    expect(texts(g, 'Идентификация', 'Отрасль')).toEqual(['нет данных', 'Промышленность'].map(norm))
    expect(texts(g, 'Технические характеристики', 'Грузоподъёмность')).toEqual(['1 500 кг', '1 000 кг'].map(norm))
    expect(texts(g, 'Экономика', 'Цена')).toEqual(['2 700 000 ₽', '3 100 000 ₽'].map(norm))
  })

  it('leaves out a row no robot has a value for', () => {
    const g = compareGroups([a, b], null, fields)
    expect(rowsOf(g, 'Технические характеристики').map((r) => r.label)).toEqual(['Грузоподъёмность', 'Масса'].map(norm))
    expect(rowsOf(g, 'Инфраструктура')).toEqual([])
    expect(g.map((x) => x.title)).not.toContain('Инфраструктура')
  })

  it('has no fit group without a calculation', () => {
    expect(compareGroups([a, b], null, fields).map((x) => x.title)).not.toContain('Применимость')
  })

  it('adds the calculation cases and verdicts when it has them', () => {
    const items = new Map([
      ['а', verdict('а', 'recommended', [], 13, 55212300, 2.4)],
      ['б', verdict('б', 'excluded', ['Ширина больше проёма.'], 0, 0, null)],
    ])
    const g = compareGroups([a, b], items, fields)
    expect(texts(g, 'Экономика', 'Флот в расчёте')?.[0]).toBe(norm('13 шт'))
    expect(texts(g, 'Экономика', 'Окупаемость')).toEqual(['2,4 года', 'не окупается'].map(norm))
    expect(rowsOf(g, 'Применимость').find((r) => r.label === 'Оценка')?.cells).toEqual([
      { text: 'подходит', tag: 'tag-good' },
      { text: 'не подходит', tag: 'tag-danger' },
    ])
    expect(texts(g, 'Применимость', 'Замечания')).toEqual(['нет данных', 'Ширина больше проёма.'].map(norm))
  })

  it('counts the values that have a source of their own and links the card source', () => {
    const g = compareGroups([a, b], null, fields)
    expect(texts(g, 'Качество данных', 'Значений со своим источником')).toEqual(['1 из 4', '0 из 2'].map(norm))
    const source = rowsOf(g, 'Качество данных').find((r) => r.label === 'Источник')
    expect(source?.cells[0]).toEqual({ text: 'evocargo.example', href: 'https://www.evocargo.example/h1500' })
    expect(texts(g, 'Качество данных', 'Достоверность')).toEqual(['заявлено производителем', 'нет данных'].map(norm))
  })
})
