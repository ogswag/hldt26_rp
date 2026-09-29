import type { ParamsMap, SchemaField } from '../api/client'
import { sanitizeParams } from '../guest/store'
import { equal } from '../store/apply'

// followStore brings the fields on screen up to the stored parameters after an undo, a redo or a co-author's
// edit. A busy field (focused, or holding a draft that is not saved) keeps what is in it. It returns shown
// itself when nothing moves, so the form does not re-render for nothing.
export function followStore(fields: SchemaField[], shown: ParamsMap, stored: ParamsMap, busy: ReadonlySet<string>): ParamsMap {
  const saved = sanitizeParams(fields, stored)
  let next: ParamsMap | null = null
  for (const f of fields) {
    if (busy.has(f.id) || equal(shown[f.id], saved[f.id])) {
      continue
    }
    next ??= { ...shown }
    if (saved[f.id] === undefined) {
      delete next[f.id]
    } else {
      next[f.id] = saved[f.id]
    }
  }
  return next ?? shown
}
