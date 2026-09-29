import { describe, expect, it } from 'vitest'

import type { MatchItem, MatchStep } from '../api/client'
import { partRows, ruleLabel, stepsOf, stepValue, stepVerdict } from './explain'

const step = (over: Partial<MatchStep>): MatchStep => ({
  rule_id: 'aisle_width',
  kind: 'hard',
  outcome: 'pass',
  object_value: null,
  solution_value: null,
  text: '',
  ...over,
})

const item = (over: Partial<MatchItem>): MatchItem => ({
  solution_id: 'a',
  name: 'A',
  vendor: null,
  kind: null,
  subtype: null,
  price_rub: null,
  status: 'recommended',
  reasons: [],
  score_parts: { fit: 0.8, process_match: 1, data_quality: 0.6, price_band: 0.5 },
  score: 0.79,
  forced: false,
  ...over,
})

describe('stepVerdict', () => {
  it('reads the outcome and the kind of a check', () => {
    expect(stepVerdict(step({}))).toEqual({ label: 'выполнено', tag: 'tag-good' })
    expect(stepVerdict(step({ outcome: 'fail' }))).toEqual({ label: 'не выполнено', tag: 'tag-danger' })
    expect(stepVerdict(step({ outcome: 'fail', kind: 'soft' }))).toEqual({ label: 'проверить', tag: 'tag-warning' })
    expect(stepVerdict(step({ outcome: 'unknown', kind: 'missing_evidence' }))).toEqual({ label: 'нет данных', tag: 'tag-warning' })
  })
})

describe('stepValue', () => {
  it('prints a value with its unit in Russian', () => {
    expect(stepValue(1500, 'kg')).toBe('1\u00a0500 кг')
    expect(stepValue(2563.07, 'kg/m2')).toBe('2\u00a0563,07 кг/м²')
    expect(stepValue(2800, 'mm')).toBe('2\u00a0800 мм')
    expect(stepValue(2700000, 'rub')).toBe('2\u00a0700\u00a0000\u00a0₽')
  })
  it('prints nothing for a check with no value, and no code for an unknown unit', () => {
    expect(stepValue(null, 'mm')).toBe('')
    expect(stepValue(3, 'furlong')).toBe('3')
  })
})

describe('ruleLabel', () => {
  it('names every rule the API runs and never shows its code', () => {
    expect(ruleLabel('turning_envelope')).toBe('Место для разворота')
    expect(ruleLabel('something_new')).toBe('Проверка')
  })
})

describe('stepsOf', () => {
  it('keeps the order of the run and falls back to the three lists', () => {
    const a = step({ rule_id: 'aisle_width' })
    const b = step({ rule_id: 'payload_kg', kind: 'soft' })
    const c = step({ rule_id: 'data_quality', kind: 'missing_evidence', outcome: 'unknown' })
    expect(stepsOf(item({ explanation: [b, a, c] }))).toEqual([b, a, c])
    expect(stepsOf(item({ hard: [a], soft: [b], missing_evidence: [c] }))).toEqual([a, b, c])
    expect(stepsOf(item({}))).toEqual([])
  })
})

describe('partRows', () => {
  it('lists the score parts with their weights, then the total', () => {
    const rows = partRows(item({}), { fit: 0.4, process_match: 0.25, data_quality: 0.2, price_band: 0.15 })
    expect(rows).toEqual([
      ['Соответствие объекту', '0,8, вес 0,4'],
      ['Тип объекта и задачи', '1, вес 0,25'],
      ['Полнота данных', '0,6, вес 0,2'],
      ['Цена и бюджет', '0,5, вес 0,15'],
      ['Итоговая оценка', '0,79'],
    ])
  })
})
