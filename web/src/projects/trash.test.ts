import { describe, expect, it } from 'vitest'

import type { TrashItem } from '../api/client'
import { daysLeft, keptText } from './trash'

const now = Date.parse('2026-09-20T12:00:00Z')

function item(deleted: string | null, purge: string | null): TrashItem {
  return { id: 'p1', name: 'Склад', object_type: 'warehouse', deleted_at: deleted, purge_at: purge, created_at: null }
}

describe('trash', () => {
  it('counts whole days until the project is sterilized', () => {
    expect(daysLeft('2026-09-25T12:00:00Z', now)).toBe(5)
    expect(daysLeft('2026-09-20T11:00:00Z', now)).toBe(0)
    expect(daysLeft(null, now)).toBe(0)
  })

  it('agrees the word with the number', () => {
    expect(keptText(item('2026-09-19T12:00:00Z', '2026-09-21T12:00:00Z'), now)).toContain('через 1 день')
    expect(keptText(item('2026-09-19T12:00:00Z', '2026-09-23T12:00:00Z'), now)).toContain('через 3 дня')
    expect(keptText(item('2026-09-19T12:00:00Z', '2026-09-25T12:00:00Z'), now)).toContain('через 5 дней')
    expect(keptText(item('2026-09-19T12:00:00Z', '2026-10-01T12:00:00Z'), now)).toContain('через 11 дней')
  })

  it('names the day the project went to the trash', () => {
    expect(keptText(item('2026-09-19T12:00:00Z', '2026-10-19T12:00:00Z'), now)).toMatch(/^Удалён 19 сентября/)
  })

  it('says the purge is due when nothing is left', () => {
    expect(keptText(item('2026-08-19T12:00:00Z', '2026-09-18T12:00:00Z'), now)).toContain('со дня на день')
  })
})
