import { describe, expect, it } from 'vitest'

import type { SchemaField } from '../api/client'
import { apply, compileSchema, type State } from '../store/apply'
import { permissive } from '../store/state'
import { projectSchema } from '../store/store'
import { demoContent, demoSeed, guestState } from './seed'
import schema from '../../../contracts/ops/project.schema.json' with { type: 'json' }

// The guest project must be a project the schema recognises: the same pages write to it, and its snapshot is
// what POST /api/projects/import sends to the server.
describe('guestState', () => {
  const content = {
    objectType: 'warehouse' as const,
    params: { area_m2: 5000 },
    processes: [
      {
        code: 'inbound',
        name: 'Приёмка',
        task_type: 'pallet_inbound',
        is_baseline: true,
        demand: { units_per_day: 1000, unit: 'поддон' },
        sla: { max_wait_min: 30, max_cycle_min: 45 },
        durations: { load_s: 90, unload_s: 60, travel_s: 180 },
        baseline_staff: { headcount: 8, role: 'приёмка' },
        sort_order: 0,
      },
    ],
    variants: [{ name: 'AMR', status: 'draft', sort_order: 0, fleet: [], financing: [{ kind: 'buy' as const, tariff: null, assumptions: {} }] }],
    selected: [],
    overrides: {},
  }

  it('fills every collection the schema declares', () => {
    const st = guestState(content)
    for (const name of Object.keys((schema as { collections: Record<string, unknown> }).collections)) {
      expect(st[name], name).toBeDefined()
    }
    expect(Object.keys(st.processes)).toHaveLength(1)
    expect(Object.keys(st.variants)).toHaveLength(1)
    expect(Object.keys(st.financing)).toHaveLength(1)
    // One assumption set is active, as a new project on the server has.
    const active = (st.project.project as { active_assumption_set_id: string }).active_assumption_set_id
    expect(st.assumption_sets[active]).toBeDefined()
  })

  it('takes the operations the pages send', () => {
    const st: State = guestState(content)
    const id = Object.keys(st.variants)[0]
    const out = apply(projectSchema(), st, { tx_id: 'tx-1', ops: [{ op: 'set', coll: 'variants', id, path: 'name', value: 'Штабелёр' }] }, permissive)
    expect(out.outcome.status).toBe('applied')
    expect((out.state.variants[id] as { name: string }).name).toBe('Штабелёр')
  })

  it('has no map until one is drawn', () => {
    expect((guestState(content).map.map as { page: unknown }).page).toBeNull()
  })

  it('is the shape compileSchema expects', () => {
    expect(() => compileSchema(schema as never)).not.toThrow()
  })
})

describe('demoContent', () => {
  const fields = [
    { id: 'area_total_m2', label: 'Площадь', type: 'number', unit: 'м2', required: true, min: 100, max: 100000, default: 20000 },
  ] as unknown as SchemaField[]

  it('starts at the schema defaults with no variants', () => {
    const out = demoContent('airport', fields)
    expect(out.params).toEqual({ area_total_m2: 20000 })
    expect(out.variants).toEqual([])
    expect((guestState(out).project.project as { name: string }).name).toBe('Демо Аэропорт')
  })
})

type Recs = Record<string, Record<string, unknown>>

describe('Демо Склад', () => {
  it('is a prepared project: the map, three variants with robots, and the demand the map can carry', async () => {
    const seed = await demoSeed('warehouse')
    expect(seed.version).toMatch(/^[0-9a-f]{12}$/)
    const st = await seed.state()
    expect((st.project.project as { name: string; object_type: string }).name).toBe('Демо Склад')
    expect((st.project.project as { object_type: string }).object_type).toBe('warehouse')

    const variants = (Object.values(st.variants) as { id: string; name: string; order: string }[]).sort((a, b) => (a.order < b.order ? -1 : 1))
    expect(variants.map((v) => v.name)).toEqual(['Смешанный флот', 'AMR комплектация', 'Паллетный штабелёр'])
    const fleet = Object.values(st.fleet_items) as { variant_id: string; solution_id: string; quantity: number; task_codes: string[] }[]
    for (const v of variants) {
      const own = fleet.filter((f) => f.variant_id === v.id)
      expect(own.length, v.name).toBeGreaterThan(0)
      for (const f of own) {
        expect(f.solution_id).toMatch(/^[0-9a-f-]{36}$/)
        expect(f.quantity).toBeGreaterThan(0)
        expect(f.task_codes.length).toBeGreaterThan(0)
      }
    }
    const demand = Object.fromEntries((Object.values(st.processes) as { code: string; demand: { units_per_day: number } }[]).map((p) => [p.code, p.demand.units_per_day]))
    expect(demand).toEqual({ inbound: 500, putaway: 500, piece_pick: 50000, outbound: 500 })
    expect(Object.values(st.financing).every((f) => (st.variants[(f as { variant_id: string }).variant_id] ?? null) !== null)).toBe(true)
  })

  it('has a map whose flows serve every process and point at points that exist', async () => {
    const st = await (await demoSeed('warehouse')).state()
    const map = st.map.map as { page: { width_px: number } | null; meters_per_px: number }
    expect(map.page).not.toBeNull()
    const width = map.page!.width_px * map.meters_per_px
    expect(width).toBeCloseTo(84, 5)

    const codes = new Set((Object.values(st.processes) as { code: string }[]).map((p) => p.code))
    const flows = Object.values(st.map_flows) as { process_code: string; pickup_point_ids: string[]; drop_point_ids: string[] }[]
    expect(new Set(flows.map((f) => f.process_code))).toEqual(codes)
    const points = st.map_points as Recs
    for (const f of flows) {
      for (const id of [...f.pickup_point_ids, ...f.drop_point_ids]) {
        expect(points[id], `${f.process_code}: ${id}`).toBeDefined()
      }
    }
    expect(Object.keys(st.map_edges).length).toBeGreaterThan(0)
    expect(Object.keys(st.map_obstacles).length).toBeGreaterThan(0)
  })

  it('hands out a copy each time, so an edit in one tab of the page never changes the seed', async () => {
    const seed = await demoSeed('warehouse')
    const a = await seed.state()
    ;(a.project.project as { name: string }).name = 'Другое имя'
    expect(((await seed.state()).project.project as { name: string }).name).toBe('Демо Склад')
  })

  it('takes the operations the pages send', async () => {
    const st = await (await demoSeed('warehouse')).state()
    const [id] = Object.keys(st.fleet_items)
    const out = apply(projectSchema(), st, { tx_id: 'tx-1', ops: [{ op: 'set', coll: 'fleet_items', id, path: 'quantity', value: 7 }] }, permissive)
    expect(out.outcome.status).toBe('applied')
    expect((out.state.fleet_items[id] as { quantity: number }).quantity).toBe(7)
  })

  it('leaves the other demos at the schema defaults with a version of their own', async () => {
    const seed = await demoSeed('airport')
    expect(seed.version).not.toBe((await demoSeed('warehouse')).version)
    expect(Object.keys((await seed.state()).variants)).toHaveLength(0)
  })
})
