import type { MatchItem, Solution } from '../api/client'

export type RankSort = 'fit' | 'payback' | 'capex' | 'price_rub' | 'payload_kg' | 'name'

// RankedRow is a catalog robot with its verdict and buy case from the calculation, when the calculation has it.
export type RankedRow = { solution: Solution; item: MatchItem | null }

type Key = (r: RankedRow) => number | null | undefined

// Missing values go last in every order, as the catalog list does with a missing price.
const keys: Record<Exclude<RankSort, 'name'>, Key> = {
  fit: () => null,
  payback: (r) => r.item?.estimate?.payback_years,
  capex: (r) => r.item?.estimate?.capex_rub,
  price_rub: (r) => r.solution.price_rub,
  payload_kg: (r) => r.solution.specs.payload_kg,
}

function ascending(a: number | null | undefined, b: number | null | undefined): number {
  const ma = a === null || a === undefined
  const mb = b === null || b === undefined
  if (ma || mb) {
    return ma === mb ? 0 : ma ? 1 : -1
  }
  return a - b
}

// rankRows joins catalog robots with the calculation and orders them. Ties keep the calculation's order: verdict,
// then payback, then match score.
export function rankRows(solutions: Solution[], items: MatchItem[], sort: RankSort): RankedRow[] {
  const byId = new Map(items.map((it) => [it.solution_id, it]))
  const place = new Map(items.map((it, i) => [it.solution_id, i]))
  const fit = (r: RankedRow) => place.get(r.solution.id) ?? items.length
  const rows = solutions.map((s) => ({ solution: s, item: byId.get(s.id) ?? null }))
  return rows.sort((a, b) => {
    const by = sort === 'name' ? a.solution.name.localeCompare(b.solution.name, 'ru') : ascending(keys[sort](a), keys[sort](b))
    return by || fit(a) - fit(b) || a.solution.name.localeCompare(b.solution.name, 'ru')
  })
}
