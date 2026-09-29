import { describe, expect, it } from 'vitest'

import {
  bandLabel,
  bottleneckLabel,
  formatPayback,
  formatRub,
  formatYears,
  missingSpecs,
  originFor,
  scenarioLabel,
  tariffLabel,
} from './view'

describe('display labels', () => {
  it('maps scenario kinds', () => {
    expect(scenarioLabel('baseline')).toBe('База')
    expect(scenarioLabel('buy')).toBe('Покупка')
    expect(scenarioLabel('raas')).toBe('RaaS')
  })

  it('maps bands without inventing thresholds', () => {
    expect(bandLabel('up_to_3')).toBe('до 3 лет')
    expect(bandLabel('from_3_to_5')).toBe('от 3 до 5 лет')
    expect(bandLabel('over_5')).toBe('более 5 лет')
    expect(bandLabel('none')).toBe('нет срока')
  })

  it('formats rubles with the sign', () => {
    const s = formatRub(55212300)
    expect(s.replace(/\s/g, '')).toBe('55212300₽')
    expect(formatRub(null)).toBe('нет данных')
  })

  it('agrees the word for years', () => {
    const cases: [number, string][] = [
      [1, '1 год'],
      [2, '2 года'],
      [5, '5 лет'],
      [11, '11 лет'],
      [21, '21 год'],
      [2.5, '2,5 года'],
      [0.5, '0,5 года'],
      [3.04, '3 года'],
    ]
    for (const [v, want] of cases) {
      expect(formatYears(v)).toBe(want)
    }
    expect(formatYears(7, 0)).toBe('7 лет')
  })

  it('maps sim bottleneck', () => {
    expect(bottleneckLabel('op_points')).toBe('точки операций')
    expect(bottleneckLabel('none')).toBe('нет')
  })

  it('maps raas tariffs', () => {
    expect(tariffLabel('fixed')).toBe('фиксированный')
    expect(tariffLabel('variable')).toBe('переменный')
    expect(tariffLabel('mixed')).toBe('смешанный')
  })

  it('finds metric origin', () => {
    expect(originFor([{ metric: 'payback_years', source: 'formula', note: 'денежная окупаемость' }], 'payback_years')?.note).toBe(
      'денежная окупаемость',
    )
  })

  it('formats missing payback', () => {
    expect(formatPayback({ kind: 'buy', solution_id: null, fleet_size: 1, capex_rub: 1, opex_year_rub: 1, annual_effect_rub: -1, payback_years: null, roi_pct: null, tco_rub: 1, payback_band: 'none' })).toBe(
      'нет срока',
    )
  })
})

describe('missingSpecs', () => {
  it('names what the catalog lacks, both temperature bounds as one', () => {
    expect(missingSpecs(['min_aisle_mm', 'temp_min_c', 'temp_max_c'])).toBe('мин. проезд, температура')
    expect(missingSpecs(['payload_kg', 'width_mm'])).toBe('грузоподъёмность, ширина')
    expect(missingSpecs([])).toBe('')
  })
})
