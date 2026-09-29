import { describe, expect, it } from 'vitest'

import type { ObjectSchema } from '../api/client'
import type { Access } from '../layout/access'
import type { State } from '../store/apply'
import { commandEntries, navEntries, projectEntries, schemaEntries, type CommandInput } from './sources'

const open: Access = { open: true }
const closedCalc: Access = { open: false, reason: 'Проверьте «Площадку».' }

const schema: ObjectSchema = {
  type: 'warehouse',
  label: 'Склад',
  tabs: [{ id: 'site', label: 'Площадка', key: true }],
  groups: [
    {
      id: 'general',
      label: 'Общие параметры',
      tab: 'site',
      fields: [
        { id: 'area_total_m2', label: 'Общая площадь склада', short: 'Площадь склада', unit: 'м²', type: 'number', required: true, default: 1000 },
        { id: 'floor', label: 'Покрытие пола', unit: '', type: 'enum', required: false, default: 'concrete', options: [{ value: 'concrete', label: 'Бетон' }] },
      ],
    },
  ],
}

describe('navEntries', () => {
  const entries = navEntries('/p/1', schema, false, false, (tab) => (tab === 'calc' ? closedCalc : open), false)
  const cases: { id: string; to: string; closed?: string }[] = [
    { id: 'page:object', to: '/p/1/object' },
    { id: 'page:object:site', to: '/p/1/object/site' },
    { id: 'page:object:processes', to: '/p/1/object/processes' },
    { id: 'page:calc', to: '/p/1/calc', closed: 'Проверьте «Площадку».' },
    { id: 'page:calc:sim', to: '/p/1/calc/sim', closed: 'Проверьте «Площадку».' },
    { id: 'page:calc:history', to: '/p/1/calc/history', closed: 'Проверьте «Площадку».' },
  ]
  for (const c of cases) {
    it(c.id, () => {
      expect(entries.find((e) => e.id === c.id)).toMatchObject({ to: c.to, closed: c.closed })
    })
  }
  it('has no map subtab without a map', () => {
    expect(entries.some((e) => e.id === 'page:object:map')).toBe(false)
  })

  it('hides history in a demo', () => {
    const demoEntries = navEntries('/demo/warehouse', schema, false, true, () => open, false)
    expect(demoEntries.some((e) => e.id === 'page:calc:history')).toBe(false)
  })

  it('lists Админ for an admin only', () => {
    expect(entries.some((e) => e.id.startsWith('page:admin'))).toBe(false)
    const inProject = navEntries('/p/1', schema, false, false, () => open, true)
    expect(inProject.find((e) => e.id === 'page:admin:catalog')).toMatchObject({ to: '/p/1/admin/catalog', path: ['Админ'] })
    const home = navEntries('', null, false, false, () => open, true)
    expect(home.map((e) => e.to)).toEqual(['/admin', '/admin/catalog', '/admin/invitations', '/admin/audit'])
  })
})

describe('schemaEntries', () => {
  const state: State = { project: { project: { id: 'project', params: { area_total_m2: 20000 } } } }
  const entries = schemaEntries('/demo/warehouse', schema, state)
  const cases: { id: string; want: Record<string, unknown> }[] = [
    { id: 'section:general', want: { label: 'Общие параметры', path: ['Объект', 'Площадка'] } },
    {
      id: 'field:area_total_m2',
      want: { label: 'Площадь склада', fullLabel: 'Общая площадь склада', value: '20\u00a0000 м²', section: 'group:general', to: '/demo/warehouse/object/site' },
    },
    { id: 'field:floor', want: { label: 'Покрытие пола', fullLabel: undefined, value: 'Бетон' } },
  ]
  for (const c of cases) {
    it(c.id, () => {
      expect(entries.find((e) => e.id === c.id)).toMatchObject(c.want)
    })
  }
})

describe('commandEntries', () => {
  const act = { undo: () => {}, redo: () => {}, theme: () => {}, copy: () => {} }
  const base: CommandInput = { base: '/p/1', projectId: '1', signedIn: true, readOnly: false, calcOpen: true, undo: 'Изменить точку', redo: null, act }
  const ids = (c: Partial<CommandInput>) => commandEntries({ ...base, ...c }).map((e) => e.id)
  const cases: { name: string; input: Partial<CommandInput>; has: string[]; lacks: string[] }[] = [
    { name: 'owner of a calculated project', input: {}, has: ['cmd:undo', 'cmd:recalc', 'cmd:add-set', 'cmd:copy', 'cmd:trash'], lacks: ['cmd:redo', 'cmd:login'] },
    { name: 'viewer', input: { readOnly: true }, has: ['cmd:pdf'], lacks: ['cmd:recalc', 'cmd:add-process', 'cmd:add-set', 'cmd:import'] },
    { name: 'calculation closed', input: { calcOpen: false }, has: ['cmd:add-process'], lacks: ['cmd:recalc', 'cmd:pdf', 'cmd:add-cost'] },
    { name: 'guest demo', input: { base: '/demo/warehouse', projectId: null, signedIn: false }, has: ['cmd:login', 'cmd:recalc'], lacks: ['cmd:add-set', 'cmd:copy', 'cmd:new-project'] },
    { name: 'start page', input: { base: '', projectId: null, undo: null }, has: ['cmd:new-project', 'cmd:theme-dark'], lacks: ['cmd:projects', 'cmd:undo', 'cmd:add-process'] },
  ]
  for (const c of cases) {
    it(c.name, () => {
      const got = ids(c.input)
      for (const id of c.has) {
        expect(got).toContain(id)
      }
      for (const id of c.lacks) {
        expect(got).not.toContain(id)
      }
    })
  }
})

describe('projectEntries', () => {
  it('lists the other projects of a signed-in user', () => {
    const got = projectEntries(true, [{ id: 'a', name: 'Склад А', object_type: 'warehouse' }, { id: 'b', name: 'Склад Б', object_type: 'warehouse' }], 'a', null)
    expect(got.map((e) => [e.id, e.to])).toEqual([['project:b', '/p/b/object']])
  })

  it('lists the other demos for a guest', () => {
    expect(projectEntries(false, [], null, 'airport').map((e) => e.id)).toEqual(['demo:warehouse', 'demo:hospital'])
  })
})
