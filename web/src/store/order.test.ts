import { describe, expect, it } from 'vitest'

import { keyBetween, keysAfter, validOrderKey } from './order'

// The same vectors as api/internal/ops/order_test.go.
const cases: [string | null, string | null, string | null][] = [
  [null, null, 'a0'],
  [null, 'a0', 'Zz'],
  [null, 'Zz', 'Zy'],
  ['a0', null, 'a1'],
  ['a1', null, 'a2'],
  ['a0', 'a1', 'a0V'],
  ['a1', 'a2', 'a1V'],
  ['a0V', 'a1', 'a0l'],
  ['Zz', 'a0', 'ZzV'],
  ['Zz', 'a1', 'a0'],
  [null, 'Y00', 'Xzzz'],
  ['bzz', null, 'c000'],
  ['a0', 'a0V', 'a0G'],
  ['a0', 'a0G', 'a08'],
  ['b125', 'b129', 'b127'],
  ['a0', 'a1V', 'a1'],
  ['Zz', 'a01', 'a0'],
  [null, 'a0V', 'a0'],
  [null, 'b999', 'b99'],
  ['az', null, 'b00'],
  [null, 'A00000000000000000000000000', null],
  [null, 'A000000000000000000000000001', 'A000000000000000000000000000V'],
  ['zzzzzzzzzzzzzzzzzzzzzzzzzzy', null, 'zzzzzzzzzzzzzzzzzzzzzzzzzzz'],
  ['zzzzzzzzzzzzzzzzzzzzzzzzzzz', null, 'zzzzzzzzzzzzzzzzzzzzzzzzzzzV'],
  ['a00', null, null],
  ['a00', 'a1', null],
  ['0', '1', null],
  ['a1', 'a0', null],
]

describe('order keys', () => {
  for (const [a, b, want] of cases) {
    it(`between ${a} and ${b}`, () => {
      if (want === null) {
        expect(() => keyBetween(a, b)).toThrow()
      } else {
        expect(keyBetween(a, b)).toBe(want)
      }
    })
  }

  it('increase', () => {
    const keys = keysAfter(null, 200)
    expect(keys[0]).toBe('a0')
    expect(keys[61]).toBe('az')
    expect(keys[62]).toBe('b00')
    for (let i = 1; i < keys.length; i++) {
      expect(keys[i - 1] < keys[i]).toBe(true)
      expect(validOrderKey(keys[i])).toBe(true)
    }
  })

  it('validates', () => {
    for (const [key, want] of [
      ['a0', true],
      ['a0V', true],
      ['Zz', true],
      ['b00', true],
      ['', false],
      ['a', false],
      ['a00', false],
      ['a0-', false],
      ['0', false],
      ['A00000000000000000000000000', false],
    ] as const) {
      expect(validOrderKey(key)).toBe(want)
    }
  })
})
