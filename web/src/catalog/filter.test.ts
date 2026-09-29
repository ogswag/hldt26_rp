import { describe, expect, it } from 'vitest'

import type { Solution } from '../api/client'
import {
  activeCount,
  emptyFilter,
  facetCounts,
  filterSolutions,
  matcher,
  pageOf,
  readCompare,
  readFilter,
  sortSolutions,
  vendorsByRobots,
  writeFilter,
  type CatalogFilter,
} from './filter'

type Fixture = {
  id: string
  name: string
  vendor?: string
  family?: string | null
  status?: string | null
  objects?: string[]
  payload?: number | null
  width?: number | null
  price?: number | null
  complete?: boolean
  archived?: boolean
}

function robot(f: Fixture): Solution {
  return {
    id: f.id,
    name: f.name,
    vendor: f.vendor ?? null,
    kind: 'brs',
    subtype: null,
    status: f.status ?? null,
    industry: null,
    scenario: null,
    price_rub: f.price ?? null,
    source_url: null,
    family: f.family ?? null,
    description: null,
    object_types: f.objects ?? null,
    region: null,
    ugt: null,
    market: null,
    archived_at: f.archived ? '2026-09-01T10:00:00Z' : null,
    image_sha: null,
    specs: { payload_kg: f.payload ?? null, width_mm: f.width ?? null } as Solution['specs'],
    data_quality: { status: f.complete ? 'ok' : 'gaps', missing: [], reasons: [] },
  }
}

const catalog = [
  robot({ id: 'a', name: 'Складской робот Щука', vendor: 'Ёж Роботикс', family: 'mobile', status: 'operation', objects: ['warehouse'], payload: 600, width: 800, price: 3_000_000, complete: true }),
  robot({ id: 'b', name: 'Дрон инвентаризации', family: 'uav', status: 'piloting', objects: ['warehouse', 'airport'], payload: 2, width: 400, price: 900_000 }),
  robot({ id: 'c', name: 'Тягач багажа', family: 'ground', status: 'operation', objects: ['airport'], payload: 3000, width: 1500, complete: true }),
  robot({ id: 'd', name: 'Робот доставки лекарств', family: 'mobile', objects: ['hospital'], payload: 50, width: 550, price: 1_500_000 }),
  robot({ id: 'e', name: 'Старый погрузчик', family: 'ground', status: 'operation', objects: ['warehouse'], payload: 1000, width: 1100, price: 2_000_000, archived: true }),
  robot({ id: 'f', name: 'Система без группы', family: null, status: 'rnd' }),
]

const ids = (list: Solution[]) => list.map((s) => s.id).join(' ')

describe('readFilter and writeFilter', () => {
  it('reads an empty address as the empty filter', () => {
    expect(readFilter(new URLSearchParams())).toEqual(emptyFilter)
  })

  it('reads back what it wrote, keeps other keys and drops the page', () => {
    const want: CatalogFilter = {
      q: 'дрон',
      families: ['uav', 'mobile'],
      statuses: ['piloting', 'none'],
      vendors: ['ООО «Ронави, Роботикс»', 'МФТИ'],
      objectType: 'airport',
      minPayloadKg: 1.5,
      maxWidthMm: 900,
      specs: 'gaps',
      archive: 'all',
    }
    const written = writeFilter(new URLSearchParams('page=3&robot=x'), want)
    expect(readFilter(written)).toEqual(want)
    expect(written.get('robot')).toBe('x')
    expect(written.has('page')).toBe(false)
  })

  it('leaves defaults out of the address', () => {
    const written = writeFilter(new URLSearchParams('type=uav&archive=all&q=x'), { families: [], archive: 'active', q: '  ' })
    expect(written.toString()).toBe('')
  })

  const bad: [string, Partial<CatalogFilter>][] = [
    ['payload=-5', { minPayloadKg: undefined }],
    ['payload=abc', { minPayloadKg: undefined }],
    ['width=', { maxWidthMm: undefined }],
    ['specs=all', { specs: '' }],
    ['archive=deleted', { archive: 'active' }],
    ['type=,uav,,', { families: ['uav'] }],
    ['vendor=%D0%9C%D0%A4%D0%A2%D0%98&vendor=%20&vendor=%D0%9C%D0%A4%D0%A2%D0%98', { vendors: ['МФТИ'] }],
  ]
  for (const [query, want] of bad) {
    it(`reads ${query} as ${JSON.stringify(want)}`, () => {
      expect(readFilter(new URLSearchParams(query))).toMatchObject(want)
    })
  }
})

describe('activeCount', () => {
  const cases: [Partial<CatalogFilter>, number][] = [
    [{}, 0],
    [{ q: 'робот' }, 0],
    [{ families: ['uav', 'mobile'] }, 1],
    [{ families: ['uav'], statuses: ['rnd'], objectType: 'airport' }, 3],
    [{ minPayloadKg: 0, maxWidthMm: 800, specs: 'complete' }, 3],
    [{ archive: 'archived' }, 1],
    [{ vendors: ['МФТИ', 'АО «Эйдос»'] }, 1],
  ]
  for (const [patch, want] of cases) {
    it(`${JSON.stringify(patch)} counts ${want}`, () => {
      expect(activeCount({ ...emptyFilter, ...patch })).toBe(want)
    })
  }
})

describe('filterSolutions', () => {
  const match = matcher(catalog)
  const cases: [string, Partial<CatalogFilter>, string][] = [
    ['hides the archive by default', {}, 'a b c d f'],
    ['lists the archive alone', { archive: 'archived' }, 'e'],
    ['lists everything', { archive: 'all' }, 'a b c d e f'],
    ['by group, an empty group as «Другое»', { families: ['mobile', 'other'] }, 'a d f'],
    ['by status, an unknown status as none', { statuses: ['none', 'rnd'] }, 'd f'],
    ['by object type', { objectType: 'airport' }, 'b c'],
    ['by payload, a missing payload never passes', { minPayloadKg: 50 }, 'a c d'],
    ['by width, a missing width never passes', { maxWidthMm: 800 }, 'a b d'],
    ['with complete specs', { specs: 'complete' }, 'a c'],
    ['with gaps in the specs', { specs: 'gaps' }, 'b d f'],
    ['by search and group together', { q: 'с', families: ['mobile', 'other'] }, 'a f'],
  ]
  for (const [name, patch, want] of cases) {
    it(name, () => {
      expect(ids(filterSolutions(catalog, { ...emptyFilter, ...patch }, match))).toBe(want)
    })
  }
})

describe('matcher', () => {
  const match = matcher(catalog)
  const a = catalog[0]
  const cases: [string, boolean][] = [
    ['', true],
    ['скл', true],
    ['щука складской', true],
    ['еж', true],
    ['склдской', true],
    ['мобильный', true],
    ['дрон', false],
    ['щука дрон', false],
  ]
  for (const [query, want] of cases) {
    it(`«${query}» ${want ? 'finds' : 'misses'} the robot`, () => {
      expect(match(a, query)).toBe(want)
    })
  }

  it('misses a robot it was not built with', () => {
    expect(match(robot({ id: 'z', name: 'Щука' }), 'щука')).toBe(false)
  })
})

describe('facetCounts', () => {
  const match = matcher(catalog)

  it('counts groups against the rest of the filter, not the group filter itself', () => {
    const f = { ...emptyFilter, families: ['uav'], objectType: 'warehouse' }
    expect(Object.fromEntries(facetCounts(catalog, f, 'families', match))).toEqual({ mobile: 1, uav: 1 })
  })

  it('counts statuses with the search applied', () => {
    const f = { ...emptyFilter, statuses: ['piloting'], q: 'с' }
    expect(Object.fromEntries(facetCounts(catalog, f, 'statuses', match))).toEqual({ operation: 1, rnd: 1 })
  })
})

describe('company filter', () => {
  const list = [
    robot({ id: 'a', name: 'Альфа', vendor: 'ООО «Ронави»' }),
    robot({ id: 'b', name: 'Бета', vendor: 'ООО «Ронави»', family: 'uav' }),
    robot({ id: 'c', name: 'Гамма', vendor: 'МФТИ' }),
    robot({ id: 'd', name: 'Дельта', vendor: 'АО «Эйдос»' }),
    robot({ id: 'e', name: 'Эпсилон' }),
    robot({ id: 'f', name: 'Архивный', vendor: 'МФТИ', archived: true }),
  ]
  const match = matcher(list)

  it('lists companies by their robots, then by name, and skips a robot without one', () => {
    expect(vendorsByRobots(list)).toEqual(['МФТИ', 'ООО «Ронави»', 'АО «Эйдос»'])
  })

  const cases: [string, Partial<CatalogFilter>, string][] = [
    ['one company', { vendors: ['ООО «Ронави»'] }, 'a b'],
    ['several companies', { vendors: ['МФТИ', 'АО «Эйдос»'] }, 'c d'],
    ['a company with the archive', { vendors: ['МФТИ'], archive: 'all' }, 'c f'],
    ['a company and a group together', { vendors: ['ООО «Ронави»'], families: ['uav'] }, 'b'],
    ['a company nobody has', { vendors: ['Нет такой'] }, ''],
  ]
  for (const [name, patch, want] of cases) {
    it(`filters by ${name}`, () => {
      expect(ids(filterSolutions(list, { ...emptyFilter, ...patch }, match))).toBe(want)
    })
  }

  it('counts companies against the rest of the filter, not the company filter itself', () => {
    const f = { ...emptyFilter, vendors: ['МФТИ'], families: ['uav'] }
    expect(Object.fromEntries(facetCounts(list, f, 'vendors', match))).toEqual({ 'ООО «Ронави»': 1 })
  })
})

describe('sortSolutions', () => {
  const list = catalog.filter((s) => !s.archived_at)
  const cases: [Parameters<typeof sortSolutions>[1], string][] = [
    ['name', 'b d f a c'],
    ['price_rub', 'b d a f c'],
    ['payload_kg', 'b d a c f'],
  ]
  for (const [sort, want] of cases) {
    it(`by ${sort}, missing values last`, () => {
      expect(ids(sortSolutions(list, sort))).toBe(want)
    })
  }

  it('breaks ties by name', () => {
    const same = [robot({ id: 'y', name: 'Бета', price: 5 }), robot({ id: 'x', name: 'Альфа', price: 5 })]
    expect(ids(sortSolutions(same, 'price_rub'))).toBe('x y')
  })
})

describe('pageOf', () => {
  const cases: [string, number, number][] = [
    ['', 120, 1],
    ['page=2', 120, 2],
    ['page=9', 120, 3],
    ['page=0', 120, 1],
    ['page=-4', 120, 1],
    ['page=abc', 120, 1],
    ['page=2.7', 120, 2],
    ['page=5', 0, 1],
  ]
  for (const [query, total, want] of cases) {
    it(`«${query}» of ${total} is page ${want}`, () => {
      expect(pageOf(new URLSearchParams(query), total)).toBe(want)
    })
  }
})

describe('readCompare', () => {
  it('keeps the order of the picks and drops repeats and blanks', () => {
    expect(readCompare('b, a,b,,c')).toEqual(['b', 'a', 'c'])
  })
  it('holds at most six robots', () => {
    expect(readCompare('1,2,3,4,5,6,7,8')).toEqual(['1', '2', '3', '4', '5', '6'])
  })
  it('reads an absent key as no robots', () => {
    expect(readCompare(null)).toEqual([])
  })
  it('is not touched by a change of the filter', () => {
    const p = new URLSearchParams('cmp=a,b&q=x')
    expect(writeFilter(p, { q: 'y' }).get('cmp')).toBe('a,b')
  })
})
