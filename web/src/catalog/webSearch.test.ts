import { describe, expect, it } from 'vitest'

import { searchLinks, searchQuery } from './webSearch'

describe('searchQuery', () => {
  const cases: [string, { name: string; vendor: string | null }, string][] = [
    ['name and company', { name: 'Keenon T8', vendor: 'Keenon Robotics' }, 'Keenon T8 Keenon Robotics'],
    ['no company', { name: 'Keenon T8', vendor: null }, 'Keenon T8'],
    ['blank company', { name: 'Keenon T8', vendor: '  ' }, 'Keenon T8'],
    ['spaces around both', { name: ' Keenon T8 ', vendor: ' Keenon Robotics ' }, 'Keenon T8 Keenon Robotics'],
  ]
  for (const [title, s, want] of cases) {
    it(title, () => {
      expect(searchQuery(s)).toBe(want)
    })
  }
})

describe('searchLinks', () => {
  it('builds one link per engine with the query encoded', () => {
    const links = searchLinks({ name: 'Т8 & Ко', vendor: 'ООО «Проба»' })
    const q = encodeURIComponent('Т8 & Ко ООО «Проба»')
    expect(links).toEqual([
      { label: 'Найти в Google', href: `https://www.google.com/search?q=${q}` },
      { label: 'Найти в Яндексе', href: `https://yandex.ru/search/?text=${q}` },
    ])
    expect(new URL(links[0].href).searchParams.get('q')).toBe('Т8 & Ко ООО «Проба»')
    expect(new URL(links[1].href).searchParams.get('text')).toBe('Т8 & Ко ООО «Проба»')
  })
})
