import { describe, expect, it } from 'vitest'

import type { MatchItem, Solution } from '../api/client'

import { rankRows, type RankSort } from './ranking'

function solution(id: string, price: number | null, payload: number | null): Solution {
  return {
    id,
    name: id,
    vendor: null,
    kind: 'brs',
    subtype: null,
    status: null,
    industry: null,
    scenario: null,
    price_rub: price,
    source_url: null,
    specs: { payload_kg: payload } as Solution['specs'],
    data_quality: { status: 'ok', missing: [], reasons: [] } as Solution['data_quality'],
  }
}

function item(id: string, status: string, payback: number | null, capex: number): MatchItem {
  return {
    solution_id: id,
    name: id,
    vendor: null,
    kind: 'brs',
    subtype: null,
    price_rub: null,
    status,
    reasons: [],
    score_parts: { fit: 0, process_match: 0, data_quality: 0, price_band: 0 },
    score: 0,
    forced: false,
    estimate: { fleet_size: 1, capex_rub: capex, payback_years: payback },
  }
}

describe('rankRows', () => {
  const solutions = [solution('в', 300, null), solution('а', null, 800), solution('б', 100, 1500), solution('г', 200, 500)]
  // The calculation's order: б fits and pays back fastest, в does not pay back, а needs a check, г is not in it.
  const items = [item('б', 'recommended', 1.5, 900), item('в', 'recommended', null, 400), item('а', 'needs_review', 1.2, 700)]
  const cases: [RankSort, string][] = [
    ['fit', 'б в а г'],
    ['payback', 'а б в г'],
    ['capex', 'в а б г'],
    ['price_rub', 'б г в а'],
    ['payload_kg', 'г а б в'],
    ['name', 'а б в г'],
  ]
  for (const [sort, want] of cases) {
    it(`orders by ${sort}, missing values last`, () => {
      expect(rankRows(solutions, items, sort).map((r) => r.solution.id).join(' ')).toBe(want)
    })
  }

  it('joins each robot with its verdict', () => {
    const rows = rankRows(solutions, items, 'fit')
    expect(rows.map((r) => r.item?.status ?? null)).toEqual(['recommended', 'recommended', 'needs_review', null])
  })
})
