import { afterEach, describe, expect, it, vi } from 'vitest'

import { apply, type RawOp, type Reason, type State, type Tx } from './apply'
import { memoryPersistence } from './persist'
import { permissive } from './state'
import { ProjectStore, projectSchema, type JournalEntry, type TxResult } from './store'
import { SyncError, Syncer, type BatchBody, type BatchResponse, type Clock, type OperationsPage, type SnapshotResponse, type Transport } from './sync'

const V1 = '00000000-0000-4000-8000-000000000201'
const V2 = '00000000-0000-4000-8000-000000000202'
const P1 = '00000000-0000-4000-8000-000000000701'

function initial(): State {
  return {
    project: { project: { id: 'project', name: 'Склад', object_type: 'warehouse', params: {}, match_selected_ids: [], econ_overrides: null, active_assumption_set_id: null } },
    processes: {},
    variants: {
      [V1]: { id: V1, order: 'a0', name: 'AMR', status: 'draft', notes: '' },
      [V2]: { id: V2, order: 'a1', name: 'Штабелёр', status: 'draft', notes: '' },
    },
    fleet_items: {},
    financing: {},
    shared_costs: {},
    assumption_sets: {},
    map: { map: { id: 'map', page: { width_px: 1000, height_px: 700, source_kind: 'none' }, profile: 'indoor', meters_per_px: 0.1, segment: null, check: null } },
    map_points: { [P1]: { id: P1, kind: 'task', name: 'Приёмка', pos: { x: 1, y: 2 }, process_code: null } },
    map_edges: {},
    map_zones: {},
    map_obstacles: {},
    map_resources: {},
    map_flows: {},
  }
}

const set = (coll: string, id: string, path: string, value: unknown): RawOp => ({ op: 'set', coll, id, path, value })

// FakeServer keeps a journal like the API: stored outcomes by tx_id, seq without gaps, reset entries.
class FakeServer implements Transport {
  state = initial()
  seq = 0
  journal: (JournalEntry & { client_id: string })[] = []
  outcomes = new Map<string, TxResult>()
  refuse: (tx: Tx) => Reason | null = () => null
  // lose drops the next response: 'request' before the server sees it, 'response' after it applied the batch.
  lose: 'request' | 'response' | null = null
  batches = 0
  offline = false
  gone = false

  async snapshot(): Promise<SnapshotResponse> {
    this.check()
    return { schema_version: 1, seq: this.seq, collections: structuredClone(this.state) }
  }

  async operations(_: string, after: number): Promise<OperationsPage> {
    this.check()
    const items = this.journal.filter((e) => e.seq > after)
    if (items.some((e) => e.ops.some((o) => o.op === 'reset')) || after > this.seq) {
      return { items: [], seq: this.seq, reset: true }
    }
    return { items: structuredClone(items), seq: this.seq, reset: false }
  }

  async transactions(_: string, body: BatchBody): Promise<BatchResponse> {
    this.check()
    if (this.lose === 'request') {
      this.lose = null
      throw new SyncError('network', 'lost request')
    }
    this.batches++
    const results = body.txs.map((tx) => this.commit(body.client_id, tx))
    if (this.lose === 'response') {
      this.lose = null
      throw new SyncError('network', 'lost response')
    }
    return { results, seq: this.seq }
  }

  private check(): void {
    if (this.offline) {
      throw new SyncError('network', 'offline')
    }
    if (this.gone) {
      throw new SyncError('gone', 'project in the trash')
    }
  }

  commit(clientId: string, tx: Tx): TxResult {
    const prior = this.outcomes.get(tx.tx_id)
    if (prior) {
      return prior
    }
    const refused = this.refuse(tx)
    const r = refused ? null : apply(projectSchema(), this.state, tx, permissive)
    let res: TxResult
    if (r && r.outcome.status === 'applied') {
      this.state = r.state
      this.seq++
      this.journal.push({ seq: this.seq, tx_id: tx.tx_id, ops: tx.ops, client_id: clientId })
      res = { tx_id: tx.tx_id, status: 'applied', seq: this.seq }
    } else {
      const reason = refused ?? (r?.outcome.status === 'rejected' ? r.outcome.reason : 'invalid_value')
      res = { tx_id: tx.tx_id, status: 'rejected', reason }
    }
    this.outcomes.set(tx.tx_id, res)
    return res
  }

  legacyReset(change: (st: State) => void): void {
    change(this.state)
    this.seq++
    this.journal.push({ seq: this.seq, tx_id: `legacy-${this.seq}`, ops: [{ op: 'reset' }], client_id: 'server' })
  }
}

// manualClock runs timers only when the test says so.
function manualClock(): Clock & { run(): void; pending(): number } {
  const timers = new Map<number, () => void>()
  let n = 0
  return {
    setTimeout(fn) {
      timers.set(++n, fn)
      return n
    },
    clearTimeout(h) {
      timers.delete(h as number)
    },
    run() {
      const all = [...timers.values()]
      timers.clear()
      all.forEach((fn) => fn())
    },
    pending: () => timers.size,
  }
}

async function settle(): Promise<void> {
  for (let i = 0; i < 20; i++) {
    await Promise.resolve()
  }
}

type ClientOpts = {
  clientId?: string
  owner?: string
  persistence?: ReturnType<typeof memoryPersistence>
  online?: () => boolean
  onAdopted?: (n: number) => void
}

async function client(server: FakeServer, opts: ClientOpts = {}) {
  const store = new ProjectStore('p1')
  const clock = manualClock()
  const sync = new Syncer(store, server, {
    clientId: opts.clientId ?? 'tab-a',
    owner: opts.owner,
    persistence: opts.persistence,
    clock,
    online: opts.online,
    onAdopted: opts.onAdopted,
  })
  await sync.start()
  return { store, sync, clock }
}

// fakeLocks is the Web Lock API as far as queueLock uses it. A name in `held` is a tab that is still open;
// the test closes a tab by deleting its name.
function fakeLocks() {
  const held = new Set<string>()
  const api = {
    async request(name: string, options: { ifAvailable?: boolean }, fn: (lock: unknown) => Promise<unknown>) {
      if (!options.ifAvailable) {
        held.add(name)
        void fn({ name })
        return new Promise(() => {})
      }
      if (held.has(name)) {
        return fn(null)
      }
      held.add(name)
      try {
        return await fn({ name })
      } finally {
        held.delete(name)
      }
    },
  }
  vi.stubGlobal('navigator', { locks: api, onLine: true })
  return held
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('view', () => {
  it('keeps pending edits over remote changes and shares untouched records', async () => {
    const server = new FakeServer()
    server.offline = true
    const store = new ProjectStore('p1')
    store.loadSnapshot(0, initial())
    const before = store.getState()
    store.dispatch('Переименовать', [set('variants', V1, 'name', 'AMR 2')])
    const after = store.getState()
    expect(after.variants[V1].name).toBe('AMR 2')
    expect(after.variants[V2]).toBe(before.variants[V2])
    expect(after.map_points).toBe(before.map_points)

    // A remote change to another record: the local edit stays on top.
    expect(store.applyJournal([{ seq: 1, tx_id: 'remote', ops: [set('variants', V2, 'notes', 'чужая правка')] }])).toBe(true)
    expect(store.getState().variants[V1].name).toBe('AMR 2')
    expect(store.getState().variants[V2].notes).toBe('чужая правка')
    // A remote change to the same field: the local edit still shows until the server answers.
    store.applyJournal([{ seq: 2, tx_id: 'remote-2', ops: [set('variants', V1, 'name', 'Чужое имя')] }])
    expect(store.getState().variants[V1].name).toBe('AMR 2')
    expect(store.getConfirmed().variants[V1].name).toBe('Чужое имя')
  })

  it('refuses a schema violation at once without queuing it', () => {
    const store = new ProjectStore('p1')
    store.loadSnapshot(0, initial())
    const d = store.dispatch('Пустое имя', [set('variants', V1, 'name', '')])
    expect(d.outcome).toMatchObject({ status: 'rejected', reason: 'invalid_value' })
    expect(store.getPending()).toHaveLength(0)
  })

  it('refuses a journal gap so the caller takes a snapshot', () => {
    const store = new ProjectStore('p1')
    store.loadSnapshot(3, initial())
    expect(store.applyJournal([{ seq: 5, tx_id: 't', ops: [] }])).toBe(false)
    expect(store.getSeq()).toBe(3)
  })

  it('notifies subscribers when an accepted transaction receives its sequence', () => {
    const store = new ProjectStore('p1')
    store.loadSnapshot(0, initial())
    const sent = store.dispatch('Переименовать', [set('variants', V1, 'name', 'AMR 2')])
    let changes = 0
    store.subscribe(() => changes++)
    store.settle([{ tx_id: sent.txId, status: 'applied', seq: 1 }])
    expect(changes).toBe(1)
    expect(store.getPending()[0].ackSeq).toBe(1)
  })

  it('attributes a target-missing refusal to the member who deleted the record', () => {
    const store = new ProjectStore('p1')
    store.loadSnapshot(0, initial())
    const sent = store.dispatch('Заметка', [set('variants', V1, 'notes', 'local')])
    expect(store.applyJournal([{ seq: 1, tx_id: 'remote-delete', actor: 'user-2', ops: [{ op: 'delete', coll: 'variants', id: V1 }] }])).toBe(true)
    store.settle([{ tx_id: sent.txId, status: 'rejected', reason: 'target_missing', details: { op: 0, coll: 'variants', id: V1 } }])
    expect(store.getRejected()[0].actor).toBe('user-2')
  })

  it('carries the deleter\'s email into the refused list', () => {
    const store = new ProjectStore('p1')
    store.loadSnapshot(0, initial())
    const sent = store.dispatch('Заметка', [set('variants', V1, 'notes', 'local')])
    store.applyJournal([{ seq: 1, tx_id: 'remote-delete', actor: 'user-2', actor_email: 'maks@demo.local', ops: [{ op: 'delete', coll: 'variants', id: V1 }] }])
    store.settle([{ tx_id: sent.txId, status: 'rejected', reason: 'target_missing', details: { op: 0, coll: 'variants', id: V1 } }])
    expect(store.getRejected()[0]).toMatchObject({ actor: 'user-2', actorEmail: 'maks@demo.local' })
  })
})

describe('sync', () => {
  it('sends, confirms through the journal and matches the server', async () => {
    const server = new FakeServer()
    const { store, sync } = await client(server)
    expect(store.ready).toBe(true)
    store.dispatch('Переименовать', [set('variants', V1, 'name', 'AMR 2')])
    await settle()
    expect(server.state.variants[V1].name).toBe('AMR 2')
    expect(store.getPending()).toHaveLength(0)
    expect(store.getSeq()).toBe(1)
    expect(store.getState()).toEqual(server.state)
    expect(sync.getStatus().phase).toBe('saved')
  })

  it('applies other clients through ops events', async () => {
    const server = new FakeServer()
    const a = await client(server, { clientId: 'a' })
    const b = await client(server, { clientId: 'b' })
    a.store.dispatch('Точка', [set('map_points', P1, 'pos', { x: 10, y: 20 })])
    await settle()
    b.sync.onEvent('ops', { seq: server.seq })
    await settle()
    expect(b.store.getState().map_points[P1].pos).toEqual({ x: 10, y: 20 })
    expect(b.store.getSeq()).toBe(server.seq)
  })

  it('moves a refused transaction to the refused list with the intended records', async () => {
    const server = new FakeServer()
    const a = await client(server, { clientId: 'a' })
    const b = await client(server, { clientId: 'b' })
    server.offline = true
    b.store.dispatch('Сдвинуть точку', [set('map_points', P1, 'pos', { x: 5, y: 5 })])
    await settle()
    server.offline = false
    a.store.dispatch('Удалить точку', [{ op: 'delete', coll: 'map_points', id: P1 }])
    await settle()

    // b learns about the delete first: its edit drops out of the view but waits for the server's answer.
    b.sync.onEvent('ops', { seq: server.seq })
    await settle()
    expect(b.store.getState().map_points[P1]).toBeUndefined()
    b.sync.resume()
    await settle()
    expect(b.store.getPending()).toHaveLength(0)
    const [r] = b.store.getRejected()
    expect(r).toMatchObject({ label: 'Сдвинуть точку', reason: 'target_missing' })
    expect(r.intended[0].after).toMatchObject({ id: P1, pos: { x: 5, y: 5 } })
    b.store.dismiss(r.txId)
    expect(b.store.getRejected()).toHaveLength(0)
  })

  it('refuses what only the server can check', async () => {
    const server = new FakeServer()
    server.refuse = (tx) => (tx.ops.some((o) => o.path === 'params.shifts') ? 'invalid_value' : null)
    const { store } = await client(server)
    store.dispatch('Смены', [set('project', 'project', 'params.shifts', 9)])
    expect(store.getState().project.project.params).toEqual({ shifts: 9 })
    await settle()
    expect(store.getRejected()[0]).toMatchObject({ reason: 'invalid_value' })
    expect(store.getState().project.project.params).toEqual({})
  })

  it('takes a snapshot after a legacy write and keeps the unsent queue', async () => {
    const server = new FakeServer()
    const { store, sync } = await client(server)
    server.offline = true
    store.dispatch('Заметка', [set('variants', V2, 'notes', 'моя')])
    await settle()
    server.offline = false
    server.legacyReset((st) => {
      st.variants[V1].name = 'Из старой формы'
    })
    sync.onEvent('ops', { seq: server.seq })
    await settle()
    expect(store.getConfirmed().variants[V1].name).toBe('Из старой формы')
    expect(store.getState().variants[V2].notes).toBe('моя')
    sync.resume()
    await settle()
    expect(server.state.variants[V2].notes).toBe('моя')
    expect(store.getState()).toEqual(server.state)
  })

  it('resends after a lost response without applying twice', async () => {
    const server = new FakeServer()
    const { store, sync, clock } = await client(server)
    server.lose = 'response'
    store.dispatch('Переименовать', [set('variants', V1, 'name', 'Один раз')])
    await settle()
    expect(server.seq).toBe(1)
    expect(sync.getStatus().phase).toBe('offline')
    expect(store.getPending()).toHaveLength(1)
    clock.run()
    await settle()
    expect(server.seq).toBe(1)
    expect(server.journal).toHaveLength(1)
    expect(store.getPending()).toHaveLength(0)
    expect(sync.getStatus().phase).toBe('saved')
  })

  it('waits for the server to answer the queue, or gives up after the timeout', async () => {
    const server = new FakeServer()
    const { store } = await client(server)
    let sent = false
    store.dispatch('Переименовать', [set('variants', V1, 'name', 'AMR 3')])
    void store.whenSent(5000).then(() => {
      sent = true
    })
    await settle()
    expect(sent).toBe(true)
    expect(server.state.variants[V1].name).toBe('AMR 3')

    vi.useFakeTimers()
    try {
      server.offline = true
      let gaveUp = false
      store.dispatch('Переименовать', [set('variants', V1, 'name', 'AMR 4')])
      void store.whenSent(5000).then(() => {
        gaveUp = true
      })
      await settle()
      expect(gaveUp).toBe(false)
      vi.advanceTimersByTime(5000)
      await settle()
      expect(gaveUp).toBe(true)
      expect(store.unsent()).toHaveLength(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('retries a lost request with backoff', async () => {
    const server = new FakeServer()
    const { store, clock } = await client(server)
    server.lose = 'request'
    store.dispatch('Переименовать', [set('variants', V1, 'name', 'Со второго раза')])
    await settle()
    expect(server.batches).toBe(0)
    expect(clock.pending()).toBe(1)
    clock.run()
    await settle()
    expect(server.state.variants[V1].name).toBe('Со второго раза')
  })

  it('sends one batch at a time', async () => {
    const server = new FakeServer()
    const { store, sync } = await client(server)
    store.dispatch('Раз', [set('variants', V1, 'notes', '1')])
    store.dispatch('Два', [set('variants', V1, 'notes', '2')])
    void sync.flush()
    void sync.flush()
    await settle()
    expect(server.journal.map((e) => e.seq)).toEqual([1, 2])
    expect(server.state.variants[V1].notes).toBe('2')
    expect(server.batches).toBeLessThanOrEqual(2)
  })

  it('keeps the queue offline across a reload and sends it when back online', async () => {
    const server = new FakeServer()
    const persistence = memoryPersistence()
    let online = false
    const first = await client(server, { persistence, online: () => online })
    first.store.dispatch('Без сети', [set('variants', V1, 'name', 'Офлайн')])
    await settle()
    expect(first.sync.getStatus().phase).toBe('offline')
    first.clock.run()
    first.sync.stop()

    server.offline = true
    const second = await client(server, { persistence, online: () => online })
    expect(second.store.getState().variants[V1].name).toBe('Офлайн')
    expect(second.store.getPending()).toHaveLength(1)
    server.offline = false
    online = true
    second.sync.resume()
    await settle()
    expect(server.state.variants[V1].name).toBe('Офлайн')
    expect(second.store.getPending()).toHaveLength(0)
  })

  it('keeps one person\'s queue away from another person in the same browser', async () => {
    const server = new FakeServer()
    const persistence = memoryPersistence()
    const first = await client(server, { persistence, owner: 'user-1' })
    server.offline = true
    first.store.dispatch('Правка первого', [set('variants', V1, 'name', 'Первый')])
    await settle()
    first.sync.stop()

    const second = await client(server, { persistence, owner: 'user-2' })
    expect(second.store.getPending()).toHaveLength(0)
    expect(await persistence.listQueues('p1', 'user-1')).toHaveLength(1)
    expect(await persistence.listQueues('p1', 'user-2')).toHaveLength(0)
  })

  it('takes over the queue of a tab that closed before it finished sending', async () => {
    const held = fakeLocks()
    const server = new FakeServer()
    const persistence = memoryPersistence()
    const first = await client(server, { persistence, owner: 'user-1', clientId: 'tab-a' })
    server.offline = true
    first.store.dispatch('Правка вкладки A', [set('variants', V1, 'name', 'Из вкладки A')])
    await settle()
    first.sync.stop()
    // The tab closes: its lock goes, its queue stays in storage.
    held.delete('queue:p1:user-1:tab-a')

    server.offline = false
    let adopted = 0
    const second = await client(server, {
      persistence,
      owner: 'user-1',
      clientId: 'tab-b',
      onAdopted: (n) => {
        adopted = n
      },
    })
    await settle()
    expect(adopted).toBe(1)
    expect(server.state.variants[V1].name).toBe('Из вкладки A')
    expect(second.store.getPending()).toHaveLength(0)
    expect(await persistence.listQueues('p1', 'user-1')).toEqual([{ clientId: 'tab-b', pending: [], rejected: [] }])
  })

  it('keeps each tab\'s refused list apart and takes over that of a tab that closed', async () => {
    const held = fakeLocks()
    const server = new FakeServer()
    const persistence = memoryPersistence()
    const a = await client(server, { persistence, owner: 'user-1', clientId: 'tab-e' })
    const b = await client(server, { persistence, owner: 'user-1', clientId: 'tab-f' })
    a.store.refuse('Отменить: имя', 'target_missing', undefined, [])
    b.store.refuse('Отменить: заметку', 'target_missing', undefined, [])
    await settle()

    expect(a.store.getRejected().map((r) => r.label)).toEqual(['Отменить: имя'])
    expect(b.store.getRejected().map((r) => r.label)).toEqual(['Отменить: заметку'])
    const saved = Object.fromEntries((await persistence.listQueues('p1', 'user-1')).map((q) => [q.clientId, q.rejected.map((r) => r.label)]))
    expect(saved).toEqual({ 'tab-e': ['Отменить: имя'], 'tab-f': ['Отменить: заметку'] })

    a.sync.stop()
    held.delete('queue:p1:user-1:tab-e')
    const c = await client(server, { persistence, owner: 'user-1', clientId: 'tab-g' })
    await settle()
    expect(c.store.getRejected().map((r) => r.label)).toEqual(['Отменить: имя'])
    expect(b.store.getRejected().map((r) => r.label)).toEqual(['Отменить: заметку'])
    expect((await persistence.listQueues('p1', 'user-1')).map((q) => q.clientId).sort()).toEqual(['tab-f', 'tab-g'])
  })

  it('leaves the queue of a tab that is still open alone', async () => {
    fakeLocks()
    const server = new FakeServer()
    const persistence = memoryPersistence()
    const first = await client(server, { persistence, owner: 'user-1', clientId: 'tab-c' })
    server.offline = true
    first.store.dispatch('Правка вкладки C', [set('variants', V1, 'name', 'Из вкладки C')])
    await settle()

    const second = await client(server, { persistence, owner: 'user-1', clientId: 'tab-d' })
    expect(second.store.getPending()).toHaveLength(0)
    // The open tab keeps its own queue: nothing was taken from it.
    expect(await persistence.listQueues('p1', 'user-1')).toMatchObject([{ clientId: 'tab-c' }])
    expect((await persistence.listQueues('p1', 'user-1'))[0].pending).toHaveLength(1)
  })

  it('refuses the queue when the project is deleted or the role drops to viewer', async () => {
    const server = new FakeServer()
    server.offline = true
    const { store, sync } = await client(server)
    store.loadSnapshot(0, initial())
    store.dispatch('Правка', [set('variants', V1, 'notes', 'x')])
    sync.onEvent('access', { role: 'viewer' })
    expect(store.getRejected()[0]).toMatchObject({ reason: 'forbidden' })
    expect(sync.getStatus()).toEqual({ phase: 'error', error: 'forbidden' })
    expect(store.dispatch('Ещё одна правка', [set('variants', V1, 'notes', 'y')]).outcome).toMatchObject({
      status: 'rejected',
      reason: 'forbidden',
    })
    sync.onEvent('deleted', {})
    expect(sync.getStatus()).toEqual({ phase: 'error', error: 'gone' })
  })

  it('stays gone while the project is in the trash and recovers once it is restored', async () => {
    const server = new FakeServer()
    const { store, sync } = await client(server)
    sync.onEvent('deleted', {})
    server.gone = true
    expect(sync.getStatus()).toEqual({ phase: 'error', error: 'gone' })

    sync.resume()
    await settle()
    expect(sync.getStatus()).toEqual({ phase: 'error', error: 'gone' })

    server.gone = false
    sync.onEvent('ops', { seq: server.seq })
    await settle()
    expect(sync.getStatus()).toEqual({ phase: 'saved' })
    store.dispatch('Правка', [set('variants', V1, 'notes', 'после восстановления')])
    await settle()
    expect(server.state.variants[V1]).toMatchObject({ notes: 'после восстановления' })
  })

  it('recovers when a page opens a project restored from the trash', async () => {
    const server = new FakeServer()
    server.gone = true
    const { sync } = await client(server)
    expect(sync.getStatus()).toEqual({ phase: 'error', error: 'gone' })

    server.gone = false
    sync.resume()
    await settle()
    expect(sync.getStatus()).toEqual({ phase: 'saved' })
  })
})
