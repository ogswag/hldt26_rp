import type { Solution } from '../api/client'
import type { SearchEntry } from '../search/entries'
import { prepare, score, words } from '../search/match'
import type { SelectOption } from '../ui/Select'
import { familyOf } from './families'
import { kindLabels } from './labels'

export type SpecsFilter = '' | 'complete' | 'gaps'
export type ArchiveFilter = 'active' | 'archived' | 'all'
export type CatalogSort = 'name' | 'price_rub' | 'payload_kg'

// CatalogFilter is what the catalog table and the robot overlay show. It lives in the address, so both read the
// same list and a link keeps it.
export type CatalogFilter = {
  q: string
  families: string[]
  statuses: string[]
  // vendors are company names as the catalog spells them; a robot without a company never passes a filter on them.
  vendors: string[]
  objectType: string
  minPayloadKg: number | undefined
  maxWidthMm: number | undefined
  specs: SpecsFilter
  // archive is for admins: the list shows active robots unless they ask for the archive.
  archive: ArchiveFilter
}

// noStatus stands for a robot whose catalog status is not known, in the status filter.
export const noStatus = 'none'

export const emptyFilter: CatalogFilter = {
  q: '',
  families: [],
  statuses: [],
  vendors: [],
  objectType: '',
  minPayloadKg: undefined,
  maxWidthMm: undefined,
  specs: '',
  archive: 'active',
}

// The address keys of the filter, the sort, the table page and the open robot.
export const keys = {
  q: 'q',
  families: 'type',
  statuses: 'status',
  vendors: 'vendor',
  objectType: 'object',
  minPayloadKg: 'payload',
  maxWidthMm: 'width',
  specs: 'specs',
  archive: 'archive',
  sort: 'sort',
  page: 'page',
  robot: 'robot',
  compare: 'cmp',
} as const

// compareCap is how many robots the comparison holds, as many as the comparison of scenarios.
export const compareCap = 6

// readCompare reads the ids picked for the comparison from the address, in the order they were picked.
export function readCompare(v: string | null): string[] {
  return [...new Set(list(v))].slice(0, compareCap)
}

function list(v: string | null): string[] {
  return (v ?? '')
    .split(',')
    .map((x) => x.trim())
    .filter(Boolean)
}

// vendorList reads a repeated key, since a company name may hold a comma.
function vendorList(p: URLSearchParams): string[] {
  return [...new Set(p.getAll(keys.vendors).map((v) => v.trim()).filter(Boolean))]
}

function positive(v: string | null): number | undefined {
  if (v === null || v.trim() === '') {
    return undefined
  }
  const n = Number(v)
  return Number.isFinite(n) && n >= 0 ? n : undefined
}

export function readFilter(p: URLSearchParams): CatalogFilter {
  const specs = p.get(keys.specs)
  const archive = p.get(keys.archive)
  return {
    q: p.get(keys.q) ?? '',
    families: list(p.get(keys.families)),
    statuses: list(p.get(keys.statuses)),
    vendors: vendorList(p),
    objectType: p.get(keys.objectType) ?? '',
    minPayloadKg: positive(p.get(keys.minPayloadKg)),
    maxWidthMm: positive(p.get(keys.maxWidthMm)),
    specs: specs === 'complete' || specs === 'gaps' ? specs : '',
    archive: archive === 'archived' || archive === 'all' ? archive : 'active',
  }
}

// writeFilter puts a filter change into the address. A default value leaves its key out, and the table goes back
// to its first page.
export function writeFilter(p: URLSearchParams, patch: Partial<CatalogFilter>): URLSearchParams {
  const next = new URLSearchParams(p)
  const f = { ...readFilter(p), ...patch }
  const put = (key: string, v: string) => (v ? next.set(key, v) : next.delete(key))
  put(keys.q, f.q.trim())
  put(keys.families, f.families.join(','))
  put(keys.statuses, f.statuses.join(','))
  next.delete(keys.vendors)
  for (const v of f.vendors) {
    next.append(keys.vendors, v)
  }
  put(keys.objectType, f.objectType)
  put(keys.minPayloadKg, f.minPayloadKg === undefined ? '' : String(f.minPayloadKg))
  put(keys.maxWidthMm, f.maxWidthMm === undefined ? '' : String(f.maxWidthMm))
  put(keys.specs, f.specs)
  put(keys.archive, f.archive === 'active' ? '' : f.archive)
  next.delete(keys.page)
  return next
}

// activeCount is the number on «Фильтры: N»: the panel's filters that narrow the list. The search field is not in
// the panel, so it does not count.
export function activeCount(f: CatalogFilter): number {
  return (
    (f.families.length > 0 ? 1 : 0) +
    (f.statuses.length > 0 ? 1 : 0) +
    (f.vendors.length > 0 ? 1 : 0) +
    (f.objectType ? 1 : 0) +
    (f.minPayloadKg !== undefined ? 1 : 0) +
    (f.maxWidthMm !== undefined ? 1 : 0) +
    (f.specs ? 1 : 0) +
    (f.archive !== 'active' ? 1 : 0)
  )
}

type Facet = 'families' | 'statuses' | 'vendors'

function statusOf(s: Solution): string {
  return s.status || noStatus
}

// passes applies every filter but the search and, when given, one facet: the panel counts a facet's options
// against the rest of the filter.
function passes(s: Solution, f: CatalogFilter, skip?: Facet): boolean {
  const archived = s.archived_at !== null && s.archived_at !== undefined
  if ((f.archive === 'active' && archived) || (f.archive === 'archived' && !archived)) {
    return false
  }
  if (skip !== 'families' && f.families.length > 0 && !f.families.includes(familyOf(s.family).code)) {
    return false
  }
  if (skip !== 'statuses' && f.statuses.length > 0 && !f.statuses.includes(statusOf(s))) {
    return false
  }
  if (skip !== 'vendors' && f.vendors.length > 0 && !f.vendors.includes(s.vendor ?? '')) {
    return false
  }
  if (f.objectType && !(s.object_types ?? []).includes(f.objectType)) {
    return false
  }
  const payload = s.specs.payload_kg
  if (f.minPayloadKg !== undefined && (payload === null || payload < f.minPayloadKg)) {
    return false
  }
  const width = s.specs.width_mm
  if (f.maxWidthMm !== undefined && (width === null || width > f.maxWidthMm)) {
    return false
  }
  const complete = s.data_quality.status === 'ok'
  if ((f.specs === 'complete' && !complete) || (f.specs === 'gaps' && complete)) {
    return false
  }
  return true
}

function searchEntry(s: Solution, order: number): SearchEntry {
  const family = familyOf(s.family)
  return {
    id: s.id,
    kind: 'robot',
    label: s.name,
    path: [s.vendor ?? ''],
    note: [s.subtype, family.label, s.kind ? kindLabels[s.kind] : '', s.modification].filter(Boolean).join(' '),
    order,
  }
}

// Matcher finds robots by name, vendor, subtype and group the way the project search does: word prefixes, ё as е
// and one typo in words of five letters and more.
export type Matcher = (s: Solution, query: string) => boolean

export function matcher(items: readonly Solution[]): Matcher {
  const prepared = new Map(prepare(items.map(searchEntry)).map((p) => [p.entry.id, p]))
  return (s, query) => {
    const q = words(query)
    if (q.length === 0) {
      return true
    }
    const p = prepared.get(s.id)
    return p ? score(p, q) > 0 : false
  }
}

export function filterSolutions(items: readonly Solution[], f: CatalogFilter, match: Matcher): Solution[] {
  return items.filter((s) => passes(s, f) && match(s, f.q))
}

function facetKey(s: Solution, facet: Facet): string | null {
  switch (facet) {
    case 'families':
      return familyOf(s.family).code
    case 'statuses':
      return statusOf(s)
    default:
      return s.vendor
  }
}

// facetCounts counts the robots of each group, status or company that the rest of the filter and the search let
// through.
export function facetCounts(items: readonly Solution[], f: CatalogFilter, facet: Facet, match: Matcher): Map<string, number> {
  const out = new Map<string, number>()
  for (const s of items) {
    const key = facetKey(s, facet)
    if (key === null || !passes(s, f, facet) || !match(s, f.q)) {
      continue
    }
    out.set(key, (out.get(key) ?? 0) + 1)
  }
  return out
}

// vendorsByRobots lists the companies of the loaded catalog, the one with the most robots first, ties by name. The
// order does not follow the filter, so the list holds still while other filters change its counts.
export function vendorsByRobots(items: readonly Solution[]): string[] {
  const n = new Map<string, number>()
  for (const s of items) {
    if (s.vendor) {
      n.set(s.vendor, (n.get(s.vendor) ?? 0) + 1)
    }
  }
  return [...n.keys()].sort((a, b) => (n.get(b) ?? 0) - (n.get(a) ?? 0) || a.localeCompare(b, 'ru'))
}

function ascending(a: number | null, b: number | null): number {
  if (a === null || b === null) {
    return a === b ? 0 : a === null ? 1 : -1
  }
  return a - b
}

const byName = (a: Solution, b: Solution) => a.name.localeCompare(b.name, 'ru')

// sortSolutions orders the list; a missing price or payload goes last, ties go by name.
export function sortSolutions(items: readonly Solution[], sort: CatalogSort): Solution[] {
  const out = [...items]
  switch (sort) {
    case 'price_rub':
      return out.sort((a, b) => ascending(a.price_rub, b.price_rub) || byName(a, b))
    case 'payload_kg':
      return out.sort((a, b) => ascending(a.specs.payload_kg, b.specs.payload_kg) || byName(a, b))
    default:
      return out.sort(byName)
  }
}

// sortFrom reads the order from the address; an unknown one is the list's first.
export function sortFrom<T extends string>(v: string | null, options: readonly SelectOption<T>[]): T {
  return options.find((o) => o.value === v)?.value ?? options[0].value
}

export const pageSize = 50

export function pageOf(p: URLSearchParams, total: number): number {
  const pages = Math.max(1, Math.ceil(total / pageSize))
  const n = Math.floor(Number(p.get(keys.page) ?? '1'))
  return Number.isFinite(n) ? Math.min(Math.max(n, 1), pages) : 1
}
