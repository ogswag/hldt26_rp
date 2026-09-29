// Reference data kept for a browser without network: the last catalog it fetched and the object schemas
// (IndexedDB "robots-offline").

import type { EngineCatalog, ObjectSchema, ObjectType, ObjectTypeInfo } from '../api/client'

const DB = 'robots-offline'
const CATALOG = 'catalog'
const SCHEMAS = 'schemas'
const TYPES = 'types'

function req<T>(r: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    r.onsuccess = () => resolve(r.result)
    r.onerror = () => reject(r.error)
  })
}

function open(): Promise<IDBDatabase> {
  const r = indexedDB.open(DB, 1)
  r.onupgradeneeded = () => {
    for (const name of [CATALOG, SCHEMAS, TYPES]) {
      if (!r.result.objectStoreNames.contains(name)) {
        r.result.createObjectStore(name)
      }
    }
  }
  return req(r)
}

let db: Promise<IDBDatabase> | null = null

async function store(name: string, mode: IDBTransactionMode): Promise<IDBObjectStore> {
  db ??= open()
  return (await db).transaction(name, mode).objectStore(name)
}

function usable(): boolean {
  return typeof indexedDB !== 'undefined'
}

async function get<T>(name: string, key: string): Promise<T | null> {
  if (!usable()) {
    return null
  }
  try {
    return ((await req((await store(name, 'readonly')).get(key))) as T | undefined) ?? null
  } catch {
    return null
  }
}

async function put(name: string, key: string, value: unknown): Promise<void> {
  if (!usable()) {
    return
  }
  try {
    await req((await store(name, 'readwrite')).put(value, key))
  } catch {
    // A full or blocked store only means the next visit fetches again.
  }
}

// The catalog is one row; "live" is its only key.
export const cachedCatalog = () => get<EngineCatalog>(CATALOG, 'live')

export const putCatalog = (cat: EngineCatalog) => put(CATALOG, 'live', cat)

export const cachedSchema = (type: ObjectType) => get<ObjectSchema>(SCHEMAS, type)

export const putSchema = (type: ObjectType, schema: ObjectSchema) => put(SCHEMAS, type, schema)

// The list of object types is one row; "all" is its only key.
export const cachedTypes = () => get<ObjectTypeInfo[]>(TYPES, 'all')

export const putTypes = (items: ObjectTypeInfo[]) => put(TYPES, 'all', items)
