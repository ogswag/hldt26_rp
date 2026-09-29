import { describe, expect, it } from 'vitest'

import type { SchemaField } from '../api/client'
import { helpText } from './help'

function field(partial: Partial<SchemaField>): SchemaField {
  return {
    id: 'area',
    label: 'Общая площадь',
    unit: 'м²',
    type: 'number',
    required: true,
    default: 100,
    ...partial,
  }
}

describe('helpText', () => {
  it('uses the complete server text without adding a label or range', () => {
    expect(helpText(field({ short: 'Площадь', min: 10, max: 200, help: 'Готовая подсказка.' }))).toBe('Готовая подсказка.')
  })

  it('hides help when the server sends an empty override', () => {
    expect(helpText(field({ note: 'Старое пояснение.', help: '' }))).toBe('')
  })

  it('writes generated ranges as from and to', () => {
    expect(helpText(field({ min: 10, max: 200 }))).toBe('Допустимое значение: от 10 до 200 м².')
  })

  it('does not show a range for a fixed value', () => {
    expect(helpText(field({ min: 1.302, max: 1.302 }))).toBe('')
  })

  it('keeps the unknown-value instruction in generated help', () => {
    expect(helpText(field({ allow_unknown: true }))).toBe('Если значение неизвестно, оставьте поле пустым.')
  })
})
