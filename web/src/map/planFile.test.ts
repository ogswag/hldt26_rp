import { describe, expect, it } from 'vitest'

import { numberText } from '../ui/numberText'

import { pagesNote } from './planFile'

describe('pagesNote', () => {
  it('stays silent for a one-page PDF', () => {
    expect(pagesNote(1)).toBe('')
  })

  it.each([
    [2, '2 страницы'],
    [5, '5 страниц'],
    [11, '11 страниц'],
    [21, '21 страница'],
    [91, '91 страница'],
    [1234, `${numberText(1234)} страницы`],
  ])('agrees the word with %i pages', (pages, count) => {
    expect(pagesNote(pages)).toBe(`В PDF ${count}. Используется только первая, многостраничные планы пока не поддерживаются.`)
  })
})
