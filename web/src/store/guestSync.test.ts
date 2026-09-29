import { describe, expect, it } from 'vitest'

import type { State } from './apply'
import type { DemoSeed } from '../guest/seed'
import { GuestSync, type Relay } from './guestSync'
import { memoryPersistence } from './persist'
import { schemaVersion } from './schema.gen'
import { ProjectStore } from './store'

const V1 = '00000000-0000-4000-8000-000000000301'

// demo is the seed of a demo at version v: what the page carries and how it builds the first state.
function demo(v = 'v1'): () => Promise<DemoSeed> {
  return async () => ({ version: v, state: async () => seed() })
}

// settled waits for the guest syncer to finish writing, the way the status line does on screen.
async function settled(sync: GuestSync): Promise<void> {
  for (let i = 0; i < 50 && sync.getStatus().phase !== 'saved'; i++) {
    await new Promise((r) => setTimeout(r, 0))
  }
}

function seed(): State {
  return {
    project: {
      project: {
        id: 'project',
        name: 'Демо-расчёт',
        object_type: 'warehouse',
        params: { area_m2: 5000 },
        match_selected_ids: [],
        econ_overrides: null,
        active_assumption_set_id: null,
      },
    },
    processes: {},
    variants: { [V1]: { id: V1, order: 'a0', name: 'AMR', status: 'draft', notes: '' } },
    fleet_items: {},
    financing: {},
    shared_costs: {},
    assumption_sets: {},
    map: { map: { id: 'map', page: null, profile: 'indoor', meters_per_px: 0.1, segment: null, check: null } },
    map_points: {},
    map_edges: {},
    map_zones: {},
    map_obstacles: {},
    map_resources: {},
    map_flows: {},
  }
}

// relayPair is two BroadcastChannel objects of one name: a message reaches the other one, never its sender.
function relayPair(): [Relay, Relay] {
  const make = (): Relay & { peer?: Relay } => ({
    onmessage: null,
    postMessage(data: unknown) {
      const peer = (this as { peer?: Relay }).peer
      queueMicrotask(() => peer?.onmessage?.({ data } as MessageEvent))
    },
    close() {},
  })
  const a = make()
  const b = make()
  a.peer = b
  b.peer = a
  return [a, b]
}

// The guest project has no server: it must behave as if every accepted transaction came straight back.
describe('GuestSync', () => {
  it('seeds the store, confirms what the page does and keeps it', async () => {
    const p = memoryPersistence()
    const store = new ProjectStore('guest')
    const sync = new GuestSync(store, { persistence: p, seed: demo() })
    await sync.start()

    expect(store.ready).toBe(true)
    // The status follows the write: it says "saved" once the browser really has the state.
    expect(sync.getStatus().phase).toBe('saved')
    await settled(sync)
    expect(sync.getStatus().phase).toBe('saved')

    store.dispatch('Имя варианта', [{ op: 'set', coll: 'variants', id: V1, path: 'name', value: 'Штабелёр' }])
    expect(store.getPending()).toHaveLength(0)
    expect(store.getSeq()).toBe(1)
    expect((store.getConfirmed().variants[V1] as { name: string }).name).toBe('Штабелёр')

    // The next visit opens on what was kept, not on the defaults.
    await settled(sync)
    const again = new ProjectStore('guest')
    const back = new GuestSync(again, { persistence: p, seed: demo() })
    await back.start()
    expect(again.getSeq()).toBe(1)
    expect((again.getState().variants[V1] as { name: string }).name).toBe('Штабелёр')
    expect(p.data.get('s:guest:guest')).toMatchObject({ schemaVersion })
  })

  it('starts over when the kept state belongs to another schema', async () => {
    const p = memoryPersistence()
    await p.saveSnapshot('guest', 'guest', { seq: 9, schemaVersion: schemaVersion + 1, confirmed: seed(), demoVersion: 'v1' })
    const store = new ProjectStore('guest')
    await new GuestSync(store, { persistence: p, seed: demo() }).start()
    expect(store.getSeq()).toBe(0)
  })

  it('starts over when the kept state was seeded from another version of the demo', async () => {
    const p = memoryPersistence()
    const store = new ProjectStore('guest')
    const old = new GuestSync(store, { persistence: p, seed: demo('v1') })
    await old.start()
    store.dispatch('Имя варианта', [{ op: 'set', coll: 'variants', id: V1, path: 'name', value: 'Штабелёр' }])
    await settled(old)
    old.stop()
    expect(p.data.get('s:guest:guest')).toMatchObject({ demoVersion: 'v1', seq: 1 })

    const next = new ProjectStore('guest')
    const fresh = new GuestSync(next, { persistence: p, seed: demo('v2') })
    await fresh.start()
    expect(next.getSeq()).toBe(0)
    expect((next.getState().variants[V1] as { name: string }).name).toBe('AMR')
    await settled(fresh)
    expect(p.data.get('s:guest:guest')).toMatchObject({ demoVersion: 'v2', seq: 0 })

    // The same version keeps what the guest did.
    const same = new ProjectStore('guest')
    await new GuestSync(same, { persistence: p, seed: demo('v2') }).start()
    expect(same.getSeq()).toBe(0)
  })

  it('a state kept before demo versions were stored is not used', async () => {
    const p = memoryPersistence()
    await p.saveSnapshot('guest', 'guest', { seq: 4, schemaVersion, confirmed: seed() })
    const store = new ProjectStore('guest')
    await new GuestSync(store, { persistence: p, seed: demo('v1') }).start()
    expect(store.getSeq()).toBe(0)
  })

  it('gives the demo\'s other tab each change as its next one', async () => {
    const p = memoryPersistence()
    const [ra, rb] = relayPair()
    const a = new ProjectStore('demo-warehouse')
    const b = new ProjectStore('demo-warehouse')
    const syncA = new GuestSync(a, { persistence: p, seed: demo(), channel: () => ra })
    const syncB = new GuestSync(b, { persistence: p, seed: demo(), channel: () => rb })
    await syncA.start()
    await syncB.start()

    a.dispatch('Имя варианта', [{ op: 'set', coll: 'variants', id: V1, path: 'name', value: 'Из вкладки A' }])
    b.dispatch('Заметка', [{ op: 'set', coll: 'variants', id: V1, path: 'notes', value: 'Из вкладки B' }])
    await new Promise((r) => setTimeout(r, 0))

    for (const s of [a, b]) {
      expect(s.getState().variants[V1]).toMatchObject({ name: 'Из вкладки A', notes: 'Из вкладки B' })
      expect(s.getSeq()).toBe(2)
      expect(s.getPending()).toHaveLength(0)
    }
    syncA.stop()
    syncB.stop()
  })

  it('leaves out a change from the other tab that no longer applies here', async () => {
    const [ra, rb] = relayPair()
    const a = new ProjectStore('demo-warehouse')
    const b = new ProjectStore('demo-warehouse')
    const syncA = new GuestSync(a, { persistence: memoryPersistence(), seed: demo(), channel: () => ra })
    const syncB = new GuestSync(b, { persistence: memoryPersistence(), seed: demo(), channel: () => rb })
    await syncA.start()
    await syncB.start()

    b.dispatch('Удалить вариант', [{ op: 'delete', coll: 'variants', id: V1 }])
    await new Promise((r) => setTimeout(r, 0))
    a.dispatch('Имя варианта', [{ op: 'set', coll: 'variants', id: V1, path: 'name', value: 'Поздно' }])
    await new Promise((r) => setTimeout(r, 0))

    expect(b.getState().variants[V1]).toBeUndefined()
    expect(b.getRejected()).toHaveLength(0)
    syncA.stop()
    syncB.stop()
  })
})

describe('memoryPersistence', () => {
  it('keeps snapshots and rejected edits separate for each account', async () => {
    const p = memoryPersistence()
    const one = { seq: 1, schemaVersion, confirmed: seed() }
    const two = { seq: 2, schemaVersion, confirmed: { ...seed(), variants: {} } }
    await p.saveSnapshot('p1', 'user-1', one)
    await p.saveSnapshot('p1', 'user-2', two)
    await p.saveRejected('p1', 'user-1', 'tab', [{ txId: 't1', label: 'x', reason: 'forbidden', at: 1, intended: [] }])

    expect((await p.load('p1', 'user-1', 'tab')).snapshot?.seq).toBe(1)
    expect((await p.load('p1', 'user-1', 'tab')).rejected).toHaveLength(1)
    expect((await p.load('p1', 'user-1', 'other-tab')).rejected).toHaveLength(0)
    expect((await p.load('p1', 'user-2', 'tab')).snapshot?.seq).toBe(2)
    expect((await p.load('p1', 'user-2', 'tab')).rejected).toHaveLength(0)
  })

  it('lists a tab that has only a refused list, and drops both together', async () => {
    const p = memoryPersistence()
    await p.saveRejected('p1', 'user-1', 'tab', [{ txId: 't1', label: 'x', reason: 'forbidden', at: 1, intended: [] }])
    await p.saveQueue('p1', 'user-1', 'busy', [])
    expect((await p.listQueues('p1', 'user-1')).map((q) => [q.clientId, q.pending.length, q.rejected.length]).sort()).toEqual([
      ['busy', 0, 0],
      ['tab', 0, 1],
    ])
    await p.dropQueue('p1', 'user-1', 'tab')
    expect((await p.listQueues('p1', 'user-1')).map((q) => q.clientId)).toEqual(['busy'])
  })
})
