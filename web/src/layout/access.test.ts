import { describe, expect, it } from 'vitest'

import { adminShown, subtabAccess, tabAccess, type AccessInput } from './access'

const tabs = [
  { id: 'site', label: 'Площадка', key: true },
  { id: 'staff', label: 'Персонал', key: true },
  { id: 'infra', label: 'Инфраструктура', key: false },
]

const fresh: AccessInput = {
  demo: false,
  objectType: 'warehouse',
  loaded: true,
  hasResult: false,
  schemaTabs: tabs,
  reviewed: [],
  errorTabs: [],
  hasFleet: false,
  suggestion: 'none',
}

describe('tabAccess', () => {
  const cases: { name: string; input: Partial<AccessInput>; calc: boolean; sim: boolean; reason?: RegExp }[] = [
    { name: 'new project waits for key tabs', input: {}, calc: false, sim: false, reason: /Площадка, Персонал/ },
    { name: 'optional tab is not needed', input: { reviewed: ['site', 'staff'] }, calc: true, sim: false },
    { name: 'one key tab left', input: { reviewed: ['site'] }, calc: false, sim: false, reason: /: Персонал\./ },
    { name: 'errors close the calculation', input: { reviewed: ['site', 'staff'], errorTabs: ['infra'] }, calc: false, sim: false, reason: /Инфраструктура/ },
    { name: 'a calculated project is open', input: { hasResult: true }, calc: true, sim: false },
    { name: 'fleet opens the simulation', input: { hasResult: true, hasFleet: true }, calc: true, sim: true },
    { name: 'a suggested robot opens the simulation', input: { hasResult: true, suggestion: 'ready' }, calc: true, sim: true },
    { name: 'a suggestion without a count does not', input: { hasResult: true, suggestion: 'no_count' }, calc: true, sim: false },
    { name: 'demo is open', input: { demo: true }, calc: true, sim: true },
    { name: 'no simulation outside a warehouse', input: { demo: true, objectType: 'airport' }, calc: true, sim: false },
    { name: 'nothing opens while loading', input: { loaded: false, hasResult: true, hasFleet: true }, calc: false, sim: false },
  ]
  for (const c of cases) {
    it(c.name, () => {
      const p = { ...fresh, ...c.input }
      const out = tabAccess(p)
      expect(out.object.open).toBe(true)
      expect(out.robots.open).toBe(true)
      expect(out.calc.open).toBe(c.calc)
      expect(subtabAccess(p, 'calc', 'sim').open).toBe(c.sim)
      if (c.reason && !out.calc.open) {
        expect(out.calc.reason).toMatch(c.reason)
      }
    })
  }

  it('keeps История of a calculated project with errors and closes the rest of Расчёт', () => {
    const p = { ...fresh, hasResult: true, hasFleet: true, errorTabs: ['infra'] }
    expect(tabAccess(p).calc.open).toBe(true)
    expect(subtabAccess(p, 'calc', 'history').open).toBe(true)
    const summary = subtabAccess(p, 'calc', 'summary')
    expect(summary.open).toBe(false)
    expect(summary.open ? null : summary.reason).toMatch(/Инфраструктура.*Прошлые запуски открыты в Истории/)
    expect(subtabAccess(p, 'calc', 'export').open).toBe(false)
    expect(subtabAccess(p, 'calc', 'sim').open).toBe(false)
    expect(subtabAccess({ ...p, errorTabs: [] }, 'calc', 'summary').open).toBe(true)
  })

  it('says what the simulation waits for and links to it', () => {
    const cases: [Partial<AccessInput>, RegExp, string][] = [
      [{}, /Его предложит расчёт/, 'calc'],
      [{ hasResult: true, suggestion: 'no_count' }, /нет цены в каталоге/, 'robots'],
      [{ hasResult: true }, /Ни один робот из каталога не подходит/, 'robots'],
    ]
    for (const [input, reason, tab] of cases) {
      const sim = subtabAccess({ ...fresh, ...input }, 'calc', 'sim')
      expect(sim.open ? null : sim.reason).toMatch(reason)
      expect(sim.open ? null : sim.link?.tab).toBe(tab)
    }
  })

  it('links to the first tab left to review', () => {
    const out = tabAccess({ ...fresh, reviewed: ['site'] })
    expect(out.calc.open ? null : out.calc.link).toEqual({ label: 'Открыть «Персонал»', tab: 'object', sub: 'staff' })
  })
})

describe('adminShown', () => {
  const cases: { role: string | undefined; demo: boolean; want: boolean }[] = [
    { role: 'admin', demo: false, want: true },
    { role: 'admin', demo: true, want: false },
    { role: 'user', demo: false, want: false },
    { role: undefined, demo: false, want: false },
  ]
  for (const c of cases) {
    it(`${c.role ?? 'guest'}${c.demo ? ' in a demo' : ''}`, () => {
      expect(adminShown(c.role, c.demo)).toBe(c.want)
    })
  }

  it('keeps Админ open, since the admin pages check the role themselves', () => {
    expect(tabAccess(fresh).admin.open).toBe(true)
  })
})
