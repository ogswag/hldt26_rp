import { describe, expect, it } from 'vitest'

import type { Solution } from '../api/client'
import { sourceOf } from './sources'

const robot = (over: Partial<Solution>): Solution =>
  ({
    source_url: 'https://card.example/robot',
    field_sources: {},
    specs: { confidence: 'vendor', sourced_at: '2026-09-28' },
    ...over,
  }) as Solution

describe('sourceOf', () => {
  it('reads the entry of the value first', () => {
    const s = robot({ field_sources: { mass_kg: { source_url: 'https://vendor.example/mass', confidence: 'measured', note: 'Со стенда.', sourced_at: '2026-10-01' } } })
    expect(sourceOf(s, 'mass_kg')).toEqual({
      url: 'https://vendor.example/mass',
      confidence: 'measured',
      note: 'Со стенда.',
      date: '2026-10-01',
      own: true,
    })
  })
  it('falls back to the card for a value with no entry, and says so', () => {
    expect(sourceOf(robot({}), 'mass_kg')).toEqual({
      url: 'https://card.example/robot',
      confidence: 'vendor',
      note: null,
      date: '2026-09-28',
      own: false,
    })
  })
  it('fills only what an entry leaves out', () => {
    const s = robot({ field_sources: { price_rub: { note: 'Цена из каталога организаторов.' } } })
    expect(sourceOf(s, 'price_rub')).toMatchObject({ url: 'https://card.example/robot', note: 'Цена из каталога организаторов.', own: true })
  })
})
