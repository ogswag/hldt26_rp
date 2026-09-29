import { describe, expect, it } from 'vitest'

import { familyCodes, familyColors, familyOf, isFamilyCode } from './families'

describe('familyOf', () => {
  const cases: [string | null | undefined, string][] = [
    ['uav', 'Беспилотник'],
    ['manipulator', 'Манипулятор'],
    ['software', 'Программа'],
    [null, 'Другое'],
    [undefined, 'Другое'],
    ['', 'Другое'],
    ['БАС', 'Другое'],
  ]
  for (const [code, want] of cases) {
    it(`${JSON.stringify(code)} is «${want}»`, () => {
      expect(familyOf(code).label).toBe(want)
    })
  }
})

describe('families', () => {
  it('gives the eight groups their own color slot and leaves «Другое» gray', () => {
    const slots = familyCodes.map((c) => familyOf(c).series)
    expect(slots).toEqual([1, 2, 3, 4, 5, 6, 7, 8, null])
    expect(familyColors(familyOf('other'))).toEqual({ color: 'var(--icon)', background: 'var(--bg-muted)' })
    expect(familyColors(familyOf('marine'))).toEqual({ color: 'var(--series-4)', background: 'var(--series-4-bg)' })
  })

  it('knows only its own codes', () => {
    expect(isFamilyCode('humanoid')).toBe(true)
    expect(isFamilyCode('robot')).toBe(false)
    expect(isFamilyCode(3)).toBe(false)
  })
})
