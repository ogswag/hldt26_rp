import { describe, expect, it } from 'vitest'

import type { State } from './apply'
import { stateFingerprint } from './fingerprint'

describe('stateFingerprint', () => {
  it('does not depend on the order of keys', () => {
    const a: State = { processes: { p1: { id: 'p1', name: 'Приёмка' }, p2: { id: 'p2', name: 'Отбор' } } }
    const b: State = { processes: { p2: { name: 'Отбор', id: 'p2' }, p1: { name: 'Приёмка', id: 'p1' } } }
    expect(stateFingerprint(a)).toBe(stateFingerprint(b))
  })

  it('changes with a value', () => {
    const a: State = { fleet_items: { f1: { quantity: 3 } } }
    const b: State = { fleet_items: { f1: { quantity: 4 } } }
    expect(stateFingerprint(a)).not.toBe(stateFingerprint(b))
  })

  it('tells a missing value from a null one', () => {
    const a: State = { map: { m: { id: 'm' } } }
    const b: State = { map: { m: { id: 'm', page: null } } }
    expect(stateFingerprint(a)).not.toBe(stateFingerprint(b))
  })
})
