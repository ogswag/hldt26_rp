// What the pages show for a change the server did not apply: its name, the values it carried, and the
// operations that put it back.

import type { Change, RawOp, Rec, Schema, State } from './apply'
import { equal } from './apply'
import type { Rejected } from './store'

// lowerFirst starts a phrase inside a sentence without touching the names inside it.
export function lowerFirst(text: string): string {
  return text.charAt(0).toLowerCase() + text.slice(1)
}

// deleter says who deleted the record a refused change was about. The person's own delete came from another tab.
export function deleter(r: Rejected, selfUserId: string | null): string {
  if (!r.actor) {
    return ''
  }
  if (r.actor === selfUserId) {
    return 'удалено в другой вкладке'
  }
  return r.actorEmail ? `удалил ${r.actorEmail}` : 'удалил другой участник'
}

// title names the refused action and, when the records say so, what it was about.
export function title(r: Rejected): string {
  const label = r.label || 'Изменение'
  for (const c of r.intended) {
    const name = (c.after ?? c.before ?? {}).name
    if (typeof name === 'string' && name.trim() !== '') {
      return `${label} «${name}»`
    }
  }
  return label
}

function fieldValues(c: Change): [string, unknown][] {
  const after = c.after ?? {}
  const before = c.before ?? {}
  return Object.entries(after).filter(([f, v]) => f !== 'id' && f !== 'order' && !equal(before[f], v))
}

function show(v: unknown): string {
  if (typeof v === 'string') {
    return v
  }
  if (v === null || v === undefined) {
    return ''
  }
  return JSON.stringify(v)
}

// values is the text of what the change carried, for copying it by hand.
export function values(r: Rejected): string {
  const out: string[] = []
  for (const c of r.intended) {
    for (const [field, v] of fieldValues(c)) {
      const text = show(v)
      if (text !== '') {
        out.push(`${field}: ${text}`)
      }
    }
  }
  return out.join('\n')
}

// restoreOps writes the refused change again against the state as it is now: a record that is gone comes back
// with the values it had, one that is still there gets the fields the change meant to set.
export function restoreOps(schema: Schema, st: State, intended: readonly Change[]): RawOp[] {
  const ops: RawOp[] = []
  for (const c of intended) {
    const coll = schema.spec.collections[c.coll]
    if (!coll) {
      continue
    }
    const exists = Object.prototype.hasOwnProperty.call(st[c.coll] ?? {}, c.id)
    if (c.kind === 'delete') {
      if (exists) {
        ops.push({ op: 'delete', coll: c.coll, id: c.id })
      }
      continue
    }
    const after = c.after ?? {}
    if (!exists) {
      if (coll.kind === 'singleton') {
        continue
      }
      const value: Rec = {}
      for (const [f, v] of Object.entries(after)) {
        if (f !== 'id') {
          value[f] = v
        }
      }
      ops.push({ op: 'insert', coll: c.coll, id: c.id, value })
      continue
    }
    const cur = (st[c.coll]?.[c.id] ?? {}) as Rec
    for (const [f, v] of Object.entries(after)) {
      if (f === 'id' || equal(cur[f], v)) {
        continue
      }
      if (f === 'order') {
        ops.push({ op: 'move', coll: c.coll, id: c.id, order: v as string })
      } else {
        ops.push({ op: 'set', coll: c.coll, id: c.id, path: f, value: v })
      }
    }
  }
  return ops
}
