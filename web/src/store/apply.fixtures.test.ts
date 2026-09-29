import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

import { apply, compileSchema, type Change, type Hooks, type Rec, type SchemaSpec, type State, type Tx } from './apply'
import schemaJSON from './project.schema.json'

const dir = join(__dirname, '../../../contracts/ops/fixtures')

type StubParam = { type: string; min?: number; max?: number; nullable?: boolean }
type StubSpec = {
  params: Record<string, Record<string, StubParam>>
  objects: Record<string, string | Record<string, string>>
  external: Record<string, string[]>
}

type FixtureOutcome = {
  tx_id: string
  status: string
  reason?: string
  details?: Record<string, unknown>
  changes?: Change[]
}

type Fixture = { before: State; txs: Tx[]; after: State; outcomes: FixtureOutcome[] }

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

// stubHooks mirrors stubHooks in api/internal/ops/stub_test.go.
function stubHooks(spec: StubSpec): Hooks {
  const paramOK = (p: StubParam, v: unknown): boolean => {
    if (v === null || v === undefined) {
      return Boolean(p.nullable)
    }
    if (p.type === 'bool') {
      return typeof v === 'boolean'
    }
    if (p.type === 'number' || p.type === 'int') {
      if (typeof v !== 'number' || (p.type === 'int' && !Number.isInteger(v))) {
        return false
      }
      return (p.min === undefined || v >= p.min) && (p.max === undefined || v <= p.max)
    }
    return false
  }
  return {
    params(rec: Rec, field: string, keys: string[] | null) {
      const params = isObject(rec[field]) ? (rec[field] as Record<string, unknown>) : {}
      const types = spec.params[rec.object_type as string] ?? {}
      for (const k of keys ?? Object.keys(params).sort()) {
        const p = types[k]
        if (!p || !paramOK(p, params[k])) {
          return k
        }
      }
      return null
    },
    object(validator: string, value: unknown) {
      const s = spec.objects[validator]
      if (s === undefined) {
        return false
      }
      if (typeof s === 'string') {
        return Array.isArray(value) && value.every((x) => typeof x === 'string')
      }
      if (!isObject(value)) {
        return false
      }
      for (const k of Object.keys(value).sort()) {
        const kind = s[k] ?? s['*']
        if (kind === undefined) {
          return false
        }
        if (kind === 'number' && !(typeof value[k] === 'number' && value[k] >= 0)) {
          return false
        }
        if (kind === 'string' && typeof value[k] !== 'string') {
          return false
        }
      }
      return true
    },
    external(to: string, id: string) {
      return (spec.external[to] ?? []).includes(id)
    },
  }
}

function nonEmpty(st: State): State {
  const out: State = {}
  for (const [k, v] of Object.entries(st)) {
    if (Object.keys(v).length > 0) {
      out[k] = v
    }
  }
  return out
}

const schema = compileSchema(schemaJSON as SchemaSpec)
const hooks = stubHooks(JSON.parse(readFileSync(join(dir, 'hooks.json'), 'utf8')) as StubSpec)
const files = readdirSync(dir)
  .filter((f) => f.endsWith('.json') && f !== 'hooks.json')
  .sort()

describe('operation fixtures', () => {
  it('finds the fixtures', () => {
    expect(files.length).toBeGreaterThan(100)
  })

  for (const file of files) {
    it(file, () => {
      const f = JSON.parse(readFileSync(join(dir, file), 'utf8')) as Fixture
      const before = JSON.stringify(f.before)
      let st = f.before
      const outcomes: FixtureOutcome[] = []
      for (const tx of f.txs) {
        const res = apply(schema, st, tx, hooks)
        const o: FixtureOutcome = { tx_id: tx.tx_id, status: res.outcome.status }
        if (res.outcome.status === 'rejected') {
          o.reason = res.outcome.reason
          o.details = res.outcome.details
        }
        if (res.changes.length > 0) {
          o.changes = res.changes
        }
        outcomes.push(o)
        st = res.state
      }
      expect(outcomes).toEqual(f.outcomes)
      expect(nonEmpty(st)).toEqual(f.after)
      expect(JSON.stringify(f.before)).toBe(before)
    })
  }
})
