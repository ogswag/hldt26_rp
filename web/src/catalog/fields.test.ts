import { describe, expect, it } from 'vitest'

import type { CatalogField } from '../api/client'
import { fieldLabel, valueText } from './fields'

const fields: CatalogField[] = [
  { code: 'price_rub', label: 'Цена', unit: '₽', kind: 'number', group: 'offer', editable: true },
  { code: 'service_pct_year', label: 'Сервис в год', unit: '%', kind: 'number', group: 'specs', percent: true, editable: true },
  { code: 'payload_kg', label: 'Грузоподъёмность', unit: 'кг', kind: 'number', group: 'specs', editable: true },
  { code: 'lifetime_years', label: 'Срок службы', unit: 'лет', kind: 'number', group: 'specs', editable: true },
  {
    code: 'object_types',
    label: 'Объекты',
    kind: 'choices',
    group: 'offer',
    editable: true,
    choices: [
      { code: 'warehouse', label: 'Склад' },
      { code: 'airport', label: 'Аэропорт' },
    ],
  },
  { code: 'sourced_at', label: 'Дата источника', kind: 'date', group: 'offer', editable: true },
]

const f = (code: string) => fields.find((x) => x.code === code)

describe('valueText', () => {
  const cases: [string, unknown, string][] = [
    ['price_rub', 1200000, '1\u00a0200\u00a0000\u00a0₽'],
    ['service_pct_year', 0.05, '5%'],
    ['service_pct_year', 0.125, '12,5%'],
    ['payload_kg', 1500, '1\u00a0500 кг'],
    ['lifetime_years', 2, '2 года'],
    ['lifetime_years', 5, '5 лет'],
    ['object_types', ['warehouse', 'airport'], 'Склад, Аэропорт'],
    ['sourced_at', '2026-09-28', '28.09.2026'],
    ['payload_kg', null, 'пусто'],
  ]
  for (const [code, v, want] of cases) {
    it(`${code} ${JSON.stringify(v)}`, () => {
      expect(valueText(f(code), v)).toBe(want)
    })
  }

  it('never shows a field code', () => {
    expect(fieldLabel(fields, 'uses')).toBe('Применение')
    expect(fieldLabel(fields, 'unknown_code')).toBe('Поле')
    expect(fieldLabel(fields, 'price_rub')).toBe('Цена')
  })
})
