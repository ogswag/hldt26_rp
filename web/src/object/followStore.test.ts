import { describe, expect, it } from 'vitest'

import type { SchemaField } from '../api/client'
import { followStore } from './followStore'

const fields: SchemaField[] = [
  { id: 'area', label: 'Площадь', unit: 'м²', type: 'number', required: true, default: 20000, min: 100, max: 500000 },
  { id: 'ceiling', label: 'Высота', unit: 'м', type: 'number', required: true, default: 10, min: 2, max: 40 },
  { id: 'note', label: 'Комментарий', unit: '-', type: 'string', required: false, default: null },
]

const none = new Set<string>()

describe('followStore', () => {
  it('shows the stored value after an undo', () => {
    const out = followStore(fields, { area: 21000, ceiling: 10, note: null }, { area: 20000, ceiling: 10 }, none)
    expect(out).toEqual({ area: 20000, ceiling: 10, note: null })
  })

  it('keeps a busy field as typed and moves the rest', () => {
    const out = followStore(fields, { area: 2150, ceiling: 10, note: null }, { area: 20000, ceiling: 12 }, new Set(['area']))
    expect(out).toEqual({ area: 2150, ceiling: 12, note: null })
  })

  it('keeps a value that failed its check', () => {
    const out = followStore(fields, { area: 50, ceiling: 10 }, { area: 20000, ceiling: 10 }, new Set(['area']))
    expect(out.area).toBe(50)
  })

  it('falls back to the default when the stored value is gone or out of range', () => {
    expect(followStore(fields, { area: 21000, ceiling: 10 }, { ceiling: 10 }, none).area).toBe(20000)
    expect(followStore(fields, { area: 21000, ceiling: 10 }, { area: 50, ceiling: 10 }, none).area).toBe(20000)
  })

  it('empties a field whose stored value was removed and has no default', () => {
    const out = followStore(fields, { area: 20000, ceiling: 10, note: 'рампа' }, { area: 20000, ceiling: 10, note: null }, none)
    expect(out).toEqual({ area: 20000, ceiling: 10, note: null })
  })

  it('returns the same object when nothing moves', () => {
    const shown = { area: 20000, ceiling: 10, note: null }
    expect(followStore(fields, shown, { area: 20000, ceiling: 10 }, none)).toBe(shown)
  })
})
