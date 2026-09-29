// Applies operations to project state. The contract, check order and results match api/internal/ops/apply.go;
// contracts/ops/fixtures holds the shared cases.

import { validOrderKey } from './order'

export type Rec = Record<string, unknown>
export type State = Record<string, Record<string, Rec>>

export type FieldSpec = {
  type: string
  min?: number
  max?: number
  min_length?: number
  max_length?: number
  nullable?: boolean
  required?: boolean
  immutable?: boolean
  values?: string[]
  to?: string
  key?: string
  on_delete?: string
  validator?: string
}

export type CollectionSpec = {
  kind: 'singleton' | 'list'
  ordered?: boolean
  // legacy_ids lets insert take ids like "p1" besides UUIDs: map features had them before operations, and
  // undoing a delete brings a record back under its id.
  legacy_ids?: boolean
  max_items?: number
  unique?: string[][]
  fields: Record<string, FieldSpec>
  requires?: { if_set: string; then_nonempty: string }[]
}

export type SchemaSpec = { version: number; collections: Record<string, CollectionSpec> }

export type RawOp = { op: string; coll?: string; id?: string; path?: string; value?: unknown; order?: string }
export type Tx = { tx_id: string; label?: string; ops: RawOp[] }

export type Reason =
  | 'target_missing'
  | 'ref_missing'
  | 'unique'
  | 'limit'
  | 'invalid_value'
  | 'immutable'
  | 'forbidden'
  | 'schema_version'
  | 'project_deleted'

export type Details = { op: number; coll?: string; id?: string; field?: string }
export type Outcome = { status: 'applied' } | { status: 'rejected'; reason: Reason; details: Details }

export type Change = { coll: string; id: string; kind: 'insert' | 'update' | 'delete'; before?: Rec; after?: Rec }

export type Hooks = {
  // params returns the first bad key of rec[field]; keys null means every key. Return null when valid.
  params(rec: Rec, field: string, keys: string[] | null): string | null
  object(validator: string, value: unknown): boolean
  external(to: string, id: string): boolean
}

export type Applied = { state: State; changes: Change[]; outcome: Outcome }

type BackRef = { coll: string; field: string }

export type Schema = {
  spec: SchemaSpec
  names: string[]
  fieldNames: Record<string, string[]>
  refs: Record<string, BackRef[]>
}

export function compileSchema(spec: SchemaSpec): Schema {
  const names = Object.keys(spec.collections).sort()
  const fieldNames: Record<string, string[]> = {}
  const refs: Record<string, BackRef[]> = {}
  for (const name of names) {
    fieldNames[name] = Object.keys(spec.collections[name].fields).sort()
  }
  for (const name of names) {
    for (const fn of fieldNames[name]) {
      const f = spec.collections[name].fields[fn]
      if ((f.type === 'ref' || f.type === 'ref_list') && f.to) {
        ;(refs[f.to] ??= []).push({ coll: name, field: fn })
      }
    }
  }
  return { spec, names, fieldNames, refs }
}

type Touch = {
  before: Rec | undefined
  fields: Set<string>
  allParams: Set<string>
  params: Map<string, Set<string>>
  lastOp: number
}

type Violation = { reason: Reason; details: Details }

function reject(reason: Reason, coll: string | undefined, id: string | undefined, field: string): Violation {
  const details: Details = { op: 0 }
  if (coll) {
    details.coll = coll
  }
  if (id) {
    details.id = id
  }
  if (field) {
    details.field = field
  }
  return { reason, details }
}

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
const legacyIdPattern = /^[A-Za-z0-9._-]{1,64}$/

// blank matches the characters Go treats as space in ops.nonEmpty.
const blank = new RegExp('^[\\t\\n\\v\\f\\r \\u0085\\u00a0\\u1680\\u2000-\\u200a\\u2028\\u2029\\u202f\\u205f\\u3000]*$')

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

export function equal(a: unknown, b: unknown): boolean {
  if (a === b) {
    return true
  }
  if (Array.isArray(a)) {
    return Array.isArray(b) && a.length === b.length && a.every((x, i) => equal(x, b[i]))
  }
  if (isObject(a) && isObject(b)) {
    const ka = Object.keys(a)
    if (ka.length !== Object.keys(b).length) {
      return false
    }
    return ka.every((k) => Object.prototype.hasOwnProperty.call(b, k) && equal(a[k], b[k]))
  }
  return false
}

function codePoints(s: string): number {
  let n = 0
  for (const _ of s) {
    n++
  }
  return n
}

function lengthOK(f: FieldSpec, n: number): boolean {
  return (f.min_length === undefined || n >= f.min_length) && (f.max_length === undefined || n <= f.max_length)
}

function numberOK(f: FieldSpec, n: number): boolean {
  return Number.isFinite(n) && (f.min === undefined || n >= f.min) && (f.max === undefined || n <= f.max)
}

function isPoint(v: unknown): boolean {
  if (!isObject(v) || Object.keys(v).length !== 2) {
    return false
  }
  return typeof v.x === 'number' && typeof v.y === 'number' && Number.isFinite(v.x) && Number.isFinite(v.y)
}

export function checkValue(f: FieldSpec, v: unknown): boolean {
  if (v === null || v === undefined) {
    return Boolean(f.nullable)
  }
  switch (f.type) {
    case 'string':
      return typeof v === 'string' && lengthOK(f, codePoints(v))
    case 'int':
      return typeof v === 'number' && Number.isInteger(v) && numberOK(f, v)
    case 'number':
      return typeof v === 'number' && numberOK(f, v)
    case 'bool':
      return typeof v === 'boolean'
    case 'enum':
      return typeof v === 'string' && (f.values ?? []).includes(v)
    case 'point':
      return isPoint(v)
    case 'points':
      return Array.isArray(v) && lengthOK(f, v.length) && v.every(isPoint)
    case 'object':
      return true
    case 'params':
      return isObject(v)
    case 'ref':
    case 'external_ref':
      return typeof v === 'string' && v !== ''
    case 'ref_list': {
      if (!Array.isArray(v) || !lengthOK(f, v.length)) {
        return false
      }
      const seen = new Set<string>()
      for (const x of v) {
        if (typeof x !== 'string' || x === '' || seen.has(x)) {
          return false
        }
        seen.add(x)
      }
      return true
    }
  }
  return false
}

export function zeroValue(f: FieldSpec): unknown {
  if (f.nullable) {
    return null
  }
  switch (f.type) {
    case 'string':
      return ''
    case 'int':
    case 'number':
      return 0
    case 'bool':
      return false
    case 'point':
      return { x: 0, y: 0 }
    case 'points':
    case 'ref_list':
      return []
    case 'params':
      return {}
  }
  return null
}

function nonEmpty(v: unknown): boolean {
  if (v === null || v === undefined) {
    return false
  }
  if (typeof v === 'string') {
    return !blank.test(v)
  }
  if (Array.isArray(v)) {
    return v.length > 0
  }
  return true
}

function refKey(f: FieldSpec): string {
  return f.key || 'id'
}

class Applier {
  readonly s: Schema
  readonly h: Hooks
  st: State
  owned = new Set<string>()
  order: [string, string][] = []
  touched = new Map<string, Touch>()
  checked = new Map<string, number>()
  inserts = new Map<string, string>()
  op = 0

  constructor(s: Schema, h: Hooks, st: State) {
    this.s = s
    this.h = h
    this.st = { ...st }
  }

  apply(op: RawOp): Violation | null {
    const c = op.coll !== undefined ? this.s.spec.collections[op.coll] : undefined
    if (!['insert', 'set', 'delete', 'move'].includes(op.op) || !c || !Object.prototype.hasOwnProperty.call(this.s.spec.collections, op.coll ?? '')) {
      return reject('invalid_value', op.coll, op.id, '')
    }
    const name = op.coll as string
    switch (op.op) {
      case 'insert':
        return this.insert(name, c, op)
      case 'set':
        return this.set(name, c, op)
      case 'delete':
        return this.delete(name, c, op)
    }
    return this.move(name, c, op)
  }

  get(coll: string, id: string): Rec | undefined {
    const recs = this.st[coll]
    return recs && Object.prototype.hasOwnProperty.call(recs, id) ? recs[id] : undefined
  }

  write(coll: string, id: string, rec: Rec | null): void {
    if (!this.owned.has(coll)) {
      this.st[coll] = { ...(this.st[coll] ?? {}) }
      this.owned.add(coll)
    }
    const k = `${coll}\u0000${id}`
    let t = this.touched.get(k)
    if (!t) {
      t = { before: this.get(coll, id), fields: new Set(), allParams: new Set(), params: new Map(), lastOp: this.op }
      this.touched.set(k, t)
      this.order.push([coll, id])
    }
    t.lastOp = this.op
    if (rec === null) {
      delete this.st[coll][id]
      return
    }
    this.st[coll][id] = rec
  }

  mark(coll: string, id: string, field: string, key: string, keyed: boolean): void {
    const t = this.touched.get(`${coll}\u0000${id}`) as Touch
    t.fields.add(field)
    if (keyed) {
      let set = t.params.get(field)
      if (!set) {
        set = new Set()
        t.params.set(field, set)
      }
      set.add(key)
    } else {
      t.allParams.add(field)
    }
    this.checked.set(coll, this.op)
  }

  objectOK(f: FieldSpec, v: unknown): boolean {
    return f.type !== 'object' || v === null || this.h.object(f.validator ?? '', v)
  }

  insert(name: string, c: CollectionSpec, op: RawOp): Violation | null {
    const id = op.id ?? ''
    if (c.kind === 'singleton') {
      return reject('immutable', name, id, '')
    }
    if (!uuidPattern.test(id) && !(c.legacy_ids && legacyIdPattern.test(id))) {
      return reject('invalid_value', name, id, 'id')
    }
    if (this.get(name, id)) {
      return reject('unique', name, id, 'id')
    }
    if (!isObject(op.value)) {
      return reject('invalid_value', name, id, '')
    }
    const obj = op.value
    for (const k of Object.keys(obj).sort()) {
      if (k === 'order' && c.ordered) {
        continue
      }
      if (!Object.prototype.hasOwnProperty.call(c.fields, k)) {
        return reject('invalid_value', name, id, k)
      }
    }
    const rec: Rec = { id }
    if (c.ordered) {
      const o = obj.order
      if (typeof o !== 'string' || !validOrderKey(o)) {
        return reject('invalid_value', name, id, 'order')
      }
      rec.order = o
    }
    for (const fn of this.s.fieldNames[name]) {
      const f = c.fields[fn]
      let v: unknown
      if (Object.prototype.hasOwnProperty.call(obj, fn)) {
        v = obj[fn]
      } else {
        if (f.required) {
          return reject('invalid_value', name, id, fn)
        }
        v = zeroValue(f)
      }
      if (!checkValue(f, v) || !this.objectOK(f, v)) {
        return reject('invalid_value', name, id, fn)
      }
      rec[fn] = v
    }
    this.write(name, id, rec)
    for (const fn of this.s.fieldNames[name]) {
      this.mark(name, id, fn, '', false)
    }
    this.inserts.set(name, id)
    return null
  }

  set(name: string, c: CollectionSpec, op: RawOp): Violation | null {
    let id = op.id ?? ''
    if (c.kind === 'singleton' && id === '') {
      id = name
    }
    const rec = this.get(name, id)
    if (!rec) {
      return reject('target_missing', name, id, '')
    }
    const path = op.path ?? ''
    const dot = path.indexOf('.')
    const keyed = dot >= 0
    const field = keyed ? path.slice(0, dot) : path
    const key = keyed ? path.slice(dot + 1) : ''
    const f = Object.prototype.hasOwnProperty.call(c.fields, field) ? c.fields[field] : undefined
    if (!f || (keyed && ((f.type !== 'object' && f.type !== 'params') || key === '' || key.includes('.')))) {
      return reject('invalid_value', name, id, path)
    }
    if (!('value' in op) || op.value === undefined) {
      return reject('invalid_value', name, id, path)
    }
    if (f.immutable) {
      return reject('immutable', name, id, field)
    }
    let next: unknown = op.value
    if (keyed) {
      const cur = rec[field]
      if (cur !== null && cur !== undefined && !isObject(cur)) {
        return reject('invalid_value', name, id, path)
      }
      next = { ...(isObject(cur) ? cur : {}), [key]: op.value }
    }
    if (!checkValue(f, next) || !this.objectOK(f, next)) {
      return reject('invalid_value', name, id, path)
    }
    if (equal(rec[field], next)) {
      return null
    }
    this.write(name, id, { ...rec, [field]: next })
    this.mark(name, id, field, key, keyed)
    return null
  }

  delete(name: string, c: CollectionSpec, op: RawOp): Violation | null {
    const id = op.id ?? ''
    if (c.kind === 'singleton') {
      return reject('immutable', name, id, '')
    }
    if (!this.get(name, id)) {
      return reject('target_missing', name, id, '')
    }
    this.remove(name, id)
    return null
  }

  remove(coll: string, id: string): void {
    const rec = this.get(coll, id) as Rec
    this.write(coll, id, null)
    for (const br of this.s.refs[coll] ?? []) {
      const f = this.s.spec.collections[br.coll].fields[br.field]
      const target = rec[refKey(f)]
      if (typeof target !== 'string' || target === '') {
        continue
      }
      const recs = this.st[br.coll] ?? {}
      const hits = Object.keys(recs)
        .filter((rid) => refers(f, recs[rid][br.field], target))
        .sort()
      for (const rid of hits) {
        const r = this.get(br.coll, rid)
        if (!r) {
          continue
        }
        if (f.type === 'ref' && f.on_delete === 'cascade') {
          this.remove(br.coll, rid)
        } else if (f.type === 'ref') {
          this.write(br.coll, rid, { ...r, [br.field]: null })
        } else {
          const list = Array.isArray(r[br.field]) ? (r[br.field] as unknown[]) : []
          this.write(br.coll, rid, { ...r, [br.field]: list.filter((x) => x !== target) })
        }
      }
    }
  }

  move(name: string, c: CollectionSpec, op: RawOp): Violation | null {
    const id = op.id ?? ''
    if (!c.ordered) {
      return reject('invalid_value', name, id, 'order')
    }
    const rec = this.get(name, id)
    if (!rec) {
      return reject('target_missing', name, id, '')
    }
    const key = op.order ?? ''
    if (!validOrderKey(key)) {
      return reject('invalid_value', name, id, 'order')
    }
    if (rec.order === key) {
      return null
    }
    this.write(name, id, { ...rec, order: key })
    return null
  }

  exists(f: FieldSpec, v: string): boolean {
    if (v === '') {
      return false
    }
    const key = refKey(f)
    const to = f.to ?? ''
    if (key === 'id') {
      return this.get(to, v) !== undefined
    }
    return Object.values(this.st[to] ?? {}).some((r) => r[key] === v)
  }

  finish(): Violation | null {
    for (const [coll, id] of this.order) {
      const rec = this.get(coll, id)
      const t = this.touched.get(`${coll}\u0000${id}`) as Touch
      if (!rec || t.fields.size === 0) {
        continue
      }
      const v = this.checkRecord(coll, this.s.spec.collections[coll], rec, t)
      if (v) {
        v.details.op = t.lastOp
        return v
      }
    }
    for (const name of [...this.checked.keys()].sort()) {
      const v = this.checkCollection(name, this.s.spec.collections[name])
      if (v) {
        return v
      }
    }
    return null
  }

  checkRecord(name: string, c: CollectionSpec, rec: Rec, t: Touch): Violation | null {
    const id = rec.id as string
    for (const fn of this.s.fieldNames[name]) {
      const f = c.fields[fn]
      // Params are valid only for an object type, so a new type re-checks them in full.
      const retyped = f.type === 'params' && t.fields.has('object_type')
      if (!t.fields.has(fn) && !retyped) {
        continue
      }
      const v = rec[fn]
      switch (f.type) {
        case 'ref':
          if (typeof v === 'string' && !this.exists(f, v)) {
            return reject('ref_missing', name, id, fn)
          }
          break
        case 'ref_list':
          for (const x of Array.isArray(v) ? v : []) {
            if (!this.exists(f, typeof x === 'string' ? x : '')) {
              return reject('ref_missing', name, id, fn)
            }
          }
          break
        case 'external_ref':
          if (typeof v === 'string' && !this.h.external(f.to ?? '', v)) {
            return reject('ref_missing', name, id, fn)
          }
          break
        case 'params': {
          const keys = t.allParams.has(fn) || retyped ? null : [...(t.params.get(fn) ?? [])].sort()
          const bad = this.h.params(rec, fn, keys)
          if (bad !== null) {
            return reject('invalid_value', name, id, bad ? `${fn}.${bad}` : fn)
          }
          break
        }
      }
    }
    for (const r of c.requires ?? []) {
      if (!t.fields.has(r.if_set) && !t.fields.has(r.then_nonempty)) {
        continue
      }
      if (rec[r.if_set] !== null && rec[r.if_set] !== undefined && !nonEmpty(rec[r.then_nonempty])) {
        return reject('invalid_value', name, id, r.then_nonempty)
      }
    }
    return null
  }

  checkCollection(name: string, c: CollectionSpec): Violation | null {
    const recs = this.st[name] ?? {}
    const last = this.inserts.get(name)
    if (last !== undefined && c.max_items && Object.keys(recs).length > c.max_items) {
      const v = reject('limit', name, last, '')
      v.details.op = (this.touched.get(`${name}\u0000${last}`) as Touch).lastOp
      return v
    }
    for (const fields of c.unique ?? []) {
      const count = new Map<string, number>()
      for (const r of Object.values(recs)) {
        const k = tupleKey(r, fields)
        count.set(k, (count.get(k) ?? 0) + 1)
      }
      for (const [coll, id] of this.order) {
        const rec = recs[id]
        const t = this.touched.get(`${coll}\u0000${id}`) as Touch
        if (coll !== name || !rec || !fields.some((f) => t.fields.has(f))) {
          continue
        }
        if ((count.get(tupleKey(rec, fields)) ?? 0) > 1) {
          const v = reject('unique', name, id, fields[0])
          v.details.op = t.lastOp
          return v
        }
      }
    }
    return null
  }

  changes(): Change[] {
    const out: Change[] = []
    for (const [coll, id] of this.order) {
      const before = (this.touched.get(`${coll}\u0000${id}`) as Touch).before
      const after = this.get(coll, id)
      if (!before && !after) {
        continue
      }
      if (!before) {
        out.push({ coll, id, kind: 'insert', after })
      } else if (!after) {
        out.push({ coll, id, kind: 'delete', before })
      } else if (!equal(before, after)) {
        out.push({ coll, id, kind: 'update', before, after })
      }
    }
    return out
  }
}

function refers(f: FieldSpec, v: unknown, target: string): boolean {
  if (f.type === 'ref') {
    return v === target
  }
  return Array.isArray(v) && v.includes(target)
}

function tupleKey(rec: Rec, fields: string[]): string {
  return JSON.stringify(fields.map((f) => rec[f] ?? null))
}

// apply runs tx against st. On rejection it returns st unchanged and no changes.
export function apply(s: Schema, st: State, tx: Tx, h: Hooks): Applied {
  const a = new Applier(s, h, st)
  for (let i = 0; i < tx.ops.length; i++) {
    a.op = i
    const v = a.apply(tx.ops[i])
    if (v) {
      v.details.op = i
      return { state: st, changes: [], outcome: { status: 'rejected', reason: v.reason, details: v.details } }
    }
  }
  const v = a.finish()
  if (v) {
    return { state: st, changes: [], outcome: { status: 'rejected', reason: v.reason, details: v.details } }
  }
  return { state: a.st, changes: a.changes(), outcome: { status: 'applied' } }
}
