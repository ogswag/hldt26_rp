import { describe, expect, it } from 'vitest'

import { calcSubtabs, parseAdminPath, parseProjectPath, shownTabs, tabHref } from './nav'

describe('shownTabs', () => {
  it('puts Админ first for an admin', () => {
    expect(shownTabs(true).map((t) => t.label)).toEqual(['Админ', 'Объект', 'Роботы', 'Расчёт'])
  })

  it('leaves Админ out for everyone else', () => {
    expect(shownTabs(false).map((t) => t.id)).toEqual(['object', 'robots', 'calc'])
  })
})

describe('calcSubtabs', () => {
  it('puts Симуляция after Детализация', () => {
    expect(calcSubtabs.map((t) => t.id)).toEqual([
      'summary',
      'items',
      'sim',
      'assumptions',
      'method',
      'export',
      'history',
    ])
  })
})

describe('parseProjectPath', () => {
  const cases: { path: string; want: ReturnType<typeof parseProjectPath> }[] = [
    { path: '/p/42/admin/catalog', want: { projectId: '42', demo: null, tab: 'admin', sub: 'catalog' } },
    { path: '/p/42/object/site', want: { projectId: '42', demo: null, tab: 'object', sub: 'site' } },
    { path: '/p/42/calc/sim', want: { projectId: '42', demo: null, tab: 'calc', sub: 'sim' } },
    { path: '/demo/warehouse/robots', want: { projectId: null, demo: 'warehouse', tab: 'robots', sub: null } },
    { path: '/admin/catalog', want: { projectId: null, demo: null, tab: null, sub: null } },
  ]
  for (const c of cases) {
    it(c.path, () => {
      expect(parseProjectPath(c.path)).toEqual(c.want)
    })
  }
})

describe('parseAdminPath', () => {
  const cases: [string, { sub: string | null } | null][] = [
    ['/admin', { sub: null }],
    ['/admin/', { sub: null }],
    ['/admin/audit', { sub: 'audit' }],
    ['/', null],
    ['/p/42/admin/catalog', null],
    ['/administration', null],
  ]
  for (const [path, want] of cases) {
    it(path, () => {
      expect(parseAdminPath(path)).toEqual(want)
    })
  }
})

describe('tabHref', () => {
  it('reaches Админ in a project and outside one', () => {
    expect(tabHref('/p/42', 'admin', 'invitations')).toBe('/p/42/admin/invitations')
    expect(tabHref('', 'admin', 'catalog')).toBe('/admin/catalog')
  })
})
