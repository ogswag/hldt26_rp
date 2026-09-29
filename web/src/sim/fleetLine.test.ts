import { describe, expect, it } from 'vitest'

import type { SolutionVariant } from '../api/client'

import { findLine, lineKeyOf, withLineQuantity } from './fleetLine'

const line = (id: string | undefined, quantity: number) => ({ id, solution_id: 'robot', quantity }) as SolutionVariant['fleet'][number]

const variant = (id: string, fleet: SolutionVariant['fleet']) => ({ id, name: id, fleet }) as SolutionVariant

describe('fleet line of a search', () => {
  const first = variant('v1', [line(undefined, 2)])
  const second = variant('v2', [line(undefined, 5), line('f2', 7)])

  it('names a line without an id by its place, like the server', () => {
    expect(lineKeyOf(first.fleet[0], 0)).toBe('fleet-1')
    expect(lineKeyOf(second.fleet[1], 1)).toBe('f2')
  })

  it('finds the variant the run was made for, not the one the form shows', () => {
    expect(findLine([first, second], 'v2', 'fleet-1')).toBe(second)
    expect(findLine([first, second], 'v1', 'fleet-1')).toBe(first)
  })

  it('gives nothing when the variant or the line is gone', () => {
    expect(findLine([first, second], 'v3', 'fleet-1')).toBeNull()
    expect(findLine([first], 'v1', 'fleet-2')).toBeNull()
  })

  it('changes only the named line and keeps at least one robot', () => {
    const next = withLineQuantity(second.fleet, 'f2', 11)
    expect(next.map((f) => f.quantity)).toEqual([5, 11])
    expect(withLineQuantity(second.fleet, 'fleet-1', 0).map((f) => f.quantity)).toEqual([1, 7])
    expect(second.fleet.map((f) => f.quantity)).toEqual([5, 7])
  })
})
