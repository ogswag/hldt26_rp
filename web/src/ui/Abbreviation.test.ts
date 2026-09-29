import { describe, expect, it } from 'vitest'

import { abbreviationTitle } from './Abbreviation'

describe('abbreviationTitle', () => {
  it('lists each abbreviation in a compound label', () => {
    expect(abbreviationTitle('CAPEX и OPEX/год')).toBe('Капитальные затраты на внедрение; Текущие операционные затраты')
  })

  it('leaves ordinary text alone', () => {
    expect(abbreviationTitle('Сумма за год')).toBeUndefined()
  })
})
