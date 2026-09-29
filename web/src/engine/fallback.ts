// The server is the source of truth; these helpers only stand in when it cannot be reached. What the browser
// computes is marked preliminary, so a page never passes it off as a saved run.

import {
  fetchCatalogBundle,
  guestCalculate,
  guestMatch,
  type CalculateResult,
  type EconOverrides,
  type EngineCatalog,
  type MatchOutput,
  type MatchView,
  type ObjectType,
  type ParamsMap,
} from '../api/client'
import { demoMode } from '../api/demo'
import { cachedCatalog, putCatalog } from '../offline/cache'
import type { State } from '../store/apply'
import { schemaVersion } from '../store/schema.gen'
import { available, calculate, match, type EngineRequest } from './client'

// Preliminary marks a result the browser computed instead of the server.
export type Preliminary<T> = T & { preliminary?: boolean }

export type GuestRequest = {
  object_type: ObjectType
  params: ParamsMap
  seed?: number
  include_ids?: string[]
  task_codes?: string[]
  overrides?: EconOverrides
  // collections is a demo's records: the calculation then takes its processes, variants, fleets and map from them.
  collections?: State
}

export const NoCatalog = 'Нет сети и нет сохранённого каталога, поэтому расчёт недоступен.'

// offline reports whether the browser already knows it has no network.
function offline(): boolean {
  return typeof navigator !== 'undefined' && navigator.onLine === false
}

let catalog: Promise<EngineCatalog | null> | null = null

// engineCatalog fetches the catalog and keeps a copy; without network it takes the kept copy. The server
// answers 304 to a browser that already holds the current catalog.
export function engineCatalog(): Promise<EngineCatalog | null> {
  catalog ??= (async () => {
    if (!offline()) {
      try {
        const fresh = await fetchCatalogBundle()
        await putCatalog(fresh)
        return fresh
      } catch {
        // No answer from the server: the kept copy stands in.
      }
    }
    const saved = await cachedCatalog()
    if (!saved) {
      catalog = null
    }
    return saved
  })()
  return catalog
}

// forgetCatalog drops the in-memory copy. Tests use it; the page never needs to.
export function forgetCatalog(): void {
  catalog = null
}

// warmCatalog fetches the catalog while the network is there, so a later offline visit can still calculate.
export function warmCatalog(): void {
  if (available() && (demoMode || !offline())) {
    void engineCatalog()
  }
}

function engineRequest(req: GuestRequest, cat: EngineCatalog): EngineRequest {
  return {
    catalog: cat,
    object_type: req.object_type,
    params: req.params,
    seed: req.seed,
    include_ids: req.include_ids,
    task_codes: req.task_codes,
    overrides: req.overrides,
    collections: req.collections,
  }
}

async function local<T extends object>(req: GuestRequest, run: (r: EngineRequest) => Promise<T>): Promise<Preliminary<T>> {
  const cat = await engineCatalog()
  if (!cat) {
    throw new Error(NoCatalog)
  }
  return { ...(await run(engineRequest(req, cat))), preliminary: true }
}

// lost tells a network failure from an answer the server gave. Only the first is worth retrying in the
// browser: a 400 means the request itself is wrong, and the engine would refuse it too.
function lost(err: unknown): boolean {
  return err instanceof TypeError || offline()
}

// withFallback asks the server first, unless the browser already knows it is offline.
async function withFallback<T extends object>(
  req: GuestRequest,
  server: () => Promise<T>,
  run: (r: EngineRequest) => Promise<T>,
): Promise<Preliminary<T>> {
  // A server answer carries no mark: only what this browser computed is preliminary.
  const asked = (out: T): Preliminary<T> => out as Preliminary<T>
  if (!available()) {
    return asked(await server())
  }
  // A demo build has no server behind it, so there is nothing to ask and nothing to fall back from.
  if (demoMode || offline()) {
    return local(req, run)
  }
  try {
    return asked(await server())
  } catch (err) {
    if (!lost(err)) {
      throw err
    }
    return local(req, run)
  }
}

export function calculateOrLocal(req: GuestRequest): Promise<Preliminary<CalculateResult>> {
  return withFallback(
    req,
    () =>
      guestCalculate({
        object_type: req.object_type,
        params: req.params,
        seed: req.seed,
        include_ids: req.include_ids,
        overrides: req.overrides,
        ...(req.collections ? { schema_version: schemaVersion, collections: req.collections } : {}),
      }),
    calculate,
  )
}

export function matchOrLocal(req: GuestRequest, view: MatchView = 'full'): Promise<Preliminary<MatchOutput>> {
  return withFallback(
    req,
    () => guestMatch({ object_type: req.object_type, params: req.params, include_ids: req.include_ids, task_codes: req.task_codes }, view),
    match,
  )
}
