import { afterEach, describe, expect, it, vi } from 'vitest'

import { pageId } from './clientId'
import { colorOf, COLORS, initials, people, Tracker, type Presence } from './presence'

const list: Presence[] = [
  { client_id: 'me', user_id: 'u1', email: 'anna.petrova@demo.local', route: '/map/p', selection: { coll: 'map_points', id: 'S1' } },
  { client_id: 'my-other-tab', user_id: 'u1', email: 'anna.petrova@demo.local', route: '/calc/p', selection: { coll: 'processes', id: 'P1' } },
  { client_id: 'other', user_id: 'u2', email: 'maks@demo.local', route: '/map/p', selection: { coll: 'map_points', id: 'S2' } },
  { client_id: 'third', user_id: 'u2', email: 'maks@demo.local', route: '/variants/p', selection: null },
]

function tracker(sent: unknown[] = []): Tracker {
  return new Tracker('p', { clientId: 'me', userId: 'u1', report: async (body, keepalive) => void sent.push({ ...body, keepalive }) })
}

afterEach(() => {
  vi.useRealTimers()
})

describe('presence', () => {
  it('makes initials out of an email', () => {
    expect(initials('anna.petrova@demo.local')).toBe('AP')
    expect(initials('maks@demo.local')).toBe('MA')
    expect(initials('x@demo.local')).toBe('X')
  })

  it('keeps one colour per person', () => {
    expect(colorOf('u1')).toBe(colorOf('u1'))
    expect(colorOf('u1')).toBeLessThan(COLORS)
  })

  it('groups the pages of one person and leaves out every page of this user', () => {
    const got = people(list, 'u1')
    expect(got).toHaveLength(1)
    expect(got[0]).toMatchObject({ email: 'maks@demo.local', initials: 'MA' })
    expect(got[0].selections).toEqual([{ coll: 'map_points', id: 'S2' }])
  })

  it('shows nobody when the only other page is this user in another tab', () => {
    expect(people(list.slice(0, 2), 'u1')).toEqual([])
  })

  it('names each page load once, and differently from the queue id', () => {
    expect(pageId()).toBe(pageId())
    expect(pageId()).toMatch(/^[0-9a-f-]{36}$/)
  })

  it('reports once per place and says goodbye', () => {
    const sent: unknown[] = []
    const t = tracker(sent)
    t.at('/p/1/object/map')
    t.at('/p/1/object/map')
    t.select({ coll: 'map_points', id: 'S1' })
    t.stop()
    expect(sent).toEqual([
      { client_id: 'me', route: '/p/1/object/map', selection: null, keepalive: false },
      { client_id: 'me', route: '/p/1/object/map', selection: { coll: 'map_points', id: 'S1' }, keepalive: false },
      { client_id: 'me', route: '/p/1/object/map', selection: { coll: 'map_points', id: 'S1' }, leave: true, keepalive: true },
    ])
  })

  it('holds a selection until the page is known, then sends both', () => {
    const sent: unknown[] = []
    const t = tracker(sent)
    t.select({ coll: 'processes', id: 'P1' })
    expect(sent).toHaveLength(0)
    t.at('/p/1/object/processes')
    expect(sent).toEqual([{ client_id: 'me', route: '/p/1/object/processes', selection: { coll: 'processes', id: 'P1' }, keepalive: false }])
    t.stop()
  })

  it('clears the selection when the page is left, and moves on with the new page', () => {
    const sent: unknown[] = []
    const t = tracker(sent)
    t.at('/p/1/object/processes')
    t.select({ coll: 'processes', id: 'P1' })
    t.select(null)
    t.at('/p/1/robots')
    expect(sent.map((s) => (s as { route: string; selection: unknown }).selection)).toEqual([null, { coll: 'processes', id: 'P1' }, null, null])
    expect((sent.at(-1) as { route: string }).route).toBe('/p/1/robots')
    t.stop()
  })

  it('reports again when the page comes back, and never before it has a place', () => {
    const sent: unknown[] = []
    const t = tracker(sent)
    t.refresh()
    expect(sent).toHaveLength(0)
    t.at('/p/1/calc')
    t.refresh()
    expect(sent).toHaveLength(2)
    t.stop()
  })

  it('repeats the report so the record does not expire', () => {
    vi.useFakeTimers()
    const sent: unknown[] = []
    const t = tracker(sent)
    t.at('/p/1/object/map')
    vi.advanceTimersByTime(31_000)
    expect(sent).toHaveLength(3)
    t.stop()
  })

  it('tells its listeners when someone else arrives', () => {
    const t = tracker()
    let calls = 0
    t.subscribe(() => calls++)
    t.onEvent({ list })
    expect(calls).toBe(1)
    t.onEvent({ list })
    expect(calls).toBe(1)
    expect(t.getState()[0].email).toBe('maks@demo.local')
  })

  it('takes the list the stream sends and forgets everyone when it is empty', () => {
    const t = tracker()
    t.onEvent({ list })
    expect(t.getState()).toHaveLength(1)
    t.onEvent({ list: [] })
    expect(t.getState()).toHaveLength(0)
    t.onEvent(list)
    expect(t.getState()).toHaveLength(1)
  })
})
