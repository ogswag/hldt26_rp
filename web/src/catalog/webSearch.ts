import type { Solution } from '../api/client'

// searchQuery is the robot's name followed by its company, when the catalog has one.
export function searchQuery(s: Pick<Solution, 'name' | 'vendor'>): string {
  return [s.name, s.vendor]
    .map((v) => v?.trim())
    .filter(Boolean)
    .join(' ')
}

export type SearchLink = { label: string; href: string }

// searchLinks are the web searches an admin opens in a new tab to check a robot.
export function searchLinks(s: Pick<Solution, 'name' | 'vendor'>): SearchLink[] {
  const q = encodeURIComponent(searchQuery(s))
  return [
    { label: 'Найти в Google', href: `https://www.google.com/search?q=${q}` },
    { label: 'Найти в Яндексе', href: `https://yandex.ru/search/?text=${q}` },
  ]
}
