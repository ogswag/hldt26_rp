// Reference data a page cannot work without: the object types and their schemas. Both are versioned with the
// deployment, not with the project, so a copy from the last visit is good enough when the network is gone.

import { fetchObjectSchema, fetchObjectTypes, type ObjectSchema, type ObjectType, type ObjectTypeInfo } from '../api/client'
import { cachedSchema, cachedTypes, putSchema, putTypes } from './cache'

// unreachable tells a network failure from an answer the server gave. A 404 on an unknown type is an answer,
// and the kept copy must not paper over it.
function unreachable(err: unknown): boolean {
  return err instanceof TypeError || (typeof navigator !== 'undefined' && navigator.onLine === false)
}

// objectSchema asks the server first and keeps the answer; it falls back to the kept copy only when the
// request never reached the server.
export async function objectSchema(type: ObjectType): Promise<ObjectSchema> {
  try {
    const fresh = await fetchObjectSchema(type)
    void putSchema(type, fresh)
    return fresh
  } catch (err) {
    const saved = unreachable(err) ? await cachedSchema(type) : null
    if (saved) {
      return saved
    }
    throw err
  }
}

export async function objectTypes(): Promise<{ items: ObjectTypeInfo[] }> {
  try {
    const fresh = await fetchObjectTypes()
    void putTypes(fresh.items)
    return fresh
  } catch (err) {
    const saved = unreachable(err) ? await cachedTypes() : null
    if (saved) {
      return { items: saved }
    }
    throw err
  }
}
