import { describe, expect, it } from 'vitest'

import { numberText, parseNumberText } from './numberText'

describe('number text', () => {
  it('groups digits and uses a decimal comma', () => {
    expect(numberText(20000).replace(/\s/g, ' ')).toBe('20 000')
    expect(numberText(3.5)).toBe('3,5')
    expect(numberText(1.302)).toBe('1,302')
  })

  it('reads grouped, comma and point input', () => {
    expect(parseNumberText('20 000')).toBe(20000)
    expect(parseNumberText('20\u00a0000')).toBe(20000)
    expect(parseNumberText('3,5')).toBe(3.5)
    expect(parseNumberText('3.5')).toBe(3.5)
    expect(parseNumberText('')).toBeUndefined()
    expect(parseNumberText('  ')).toBeUndefined()
    expect(parseNumberText('abc')).toBeNaN()
    expect(parseNumberText('1,2,3')).toBeNaN()
  })
})
