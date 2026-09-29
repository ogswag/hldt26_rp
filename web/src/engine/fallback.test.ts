import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError, type CalculateResult, type EngineCatalog } from '../api/client'

const catalog: EngineCatalog = {
  content_sha256: 'sha',
  candidates: [],
  robots: [],
}

const serverResult = { model_version: 'econ-v3', seed: 1 } as unknown as CalculateResult
const localResult = { model_version: 'econ-v3', seed: 2 } as unknown as CalculateResult

const api = vi.hoisted(() => ({
  fetchCatalogBundle: vi.fn(),
  guestCalculate: vi.fn(),
  guestMatch: vi.fn(),
  guestSim: vi.fn(),
}))

const engine = vi.hoisted(() => ({
  available: vi.fn(() => true),
  calculate: vi.fn(),
  match: vi.fn(),
  simulate: vi.fn(),
}))

const cache = vi.hoisted(() => ({
  cachedCatalog: vi.fn(),
  putCatalog: vi.fn(),
}))

vi.mock('../api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/client')>()),
  ...api,
}))
vi.mock('./client', () => engine)
vi.mock('../offline/cache', () => cache)

const { calculateOrLocal, engineCatalog, forgetCatalog, NoCatalog, warmCatalog } = await import('./fallback')

const request = { object_type: 'warehouse', params: { area_m2: 5000 } } as const

function online(state: boolean) {
  vi.stubGlobal('navigator', { onLine: state })
}

beforeEach(() => {
  vi.clearAllMocks()
  forgetCatalog()
  engine.available.mockReturnValue(true)
  engine.calculate.mockResolvedValue(localResult)
  api.guestCalculate.mockResolvedValue(serverResult)
  api.fetchCatalogBundle.mockResolvedValue(catalog)
  cache.cachedCatalog.mockResolvedValue(null)
  cache.putCatalog.mockResolvedValue(undefined)
  online(true)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('calculateOrLocal', () => {
  it('takes the server answer and does not mark it preliminary', async () => {
    const out = await calculateOrLocal({ ...request })
    expect(out.seed).toBe(1)
    expect(out.preliminary).toBeUndefined()
    expect(engine.calculate).not.toHaveBeenCalled()
  })

  it('goes straight to the browser when it already knows there is no network', async () => {
    online(false)
    cache.cachedCatalog.mockResolvedValue(catalog)
    const out = await calculateOrLocal({ ...request })
    expect(out.seed).toBe(2)
    expect(out.preliminary).toBe(true)
    expect(api.guestCalculate).not.toHaveBeenCalled()
    expect(engine.calculate).toHaveBeenCalledWith(expect.objectContaining({ catalog, object_type: 'warehouse' }))
  })

  it('falls back when the request never reached the server', async () => {
    api.guestCalculate.mockRejectedValue(new TypeError('Failed to fetch'))
    const out = await calculateOrLocal({ ...request })
    expect(out.preliminary).toBe(true)
  })

  it('keeps a refusal the server sent, because the engine would refuse it too', async () => {
    api.guestCalculate.mockRejectedValue(new ApiError('Площадь слишком мала.', 400, undefined, []))
    await expect(calculateOrLocal({ ...request })).rejects.toThrow('Площадь слишком мала.')
    expect(engine.calculate).not.toHaveBeenCalled()
  })

  it('asks the server when the browser cannot run the engine at all', async () => {
    engine.available.mockReturnValue(false)
    online(false)
    const out = await calculateOrLocal({ ...request })
    expect(out.seed).toBe(1)
    expect(engine.calculate).not.toHaveBeenCalled()
  })

  it('says so plainly when there is neither network nor a kept catalog', async () => {
    online(false)
    api.fetchCatalogBundle.mockRejectedValue(new TypeError('Failed to fetch'))
    await expect(calculateOrLocal({ ...request })).rejects.toThrow(NoCatalog)
  })
})

describe('engineCatalog', () => {
  it('asks the server even with a kept copy, since the catalog changes', async () => {
    cache.cachedCatalog.mockResolvedValue({ ...catalog, content_sha256: 'old' })
    expect(await engineCatalog()).toEqual(catalog)
    expect(api.fetchCatalogBundle).toHaveBeenCalledTimes(1)
  })

  it('takes the kept copy when the server does not answer', async () => {
    api.fetchCatalogBundle.mockRejectedValue(new TypeError('Failed to fetch'))
    cache.cachedCatalog.mockResolvedValue(catalog)
    expect(await engineCatalog()).toEqual(catalog)
  })

  it('takes the kept copy offline without asking the server', async () => {
    online(false)
    cache.cachedCatalog.mockResolvedValue(catalog)
    expect(await engineCatalog()).toEqual(catalog)
    expect(api.fetchCatalogBundle).not.toHaveBeenCalled()
  })

  it('fetches once, keeps it and answers the next caller from memory', async () => {
    const [a, b] = await Promise.all([engineCatalog(), engineCatalog()])
    expect(a).toEqual(catalog)
    expect(b).toEqual(catalog)
    expect(api.fetchCatalogBundle).toHaveBeenCalledTimes(1)
    expect(cache.putCatalog).toHaveBeenCalledWith(catalog)
  })

  it('tries again after a failed fetch instead of remembering the failure', async () => {
    api.fetchCatalogBundle.mockRejectedValueOnce(new TypeError('Failed to fetch'))
    expect(await engineCatalog()).toBeNull()
    expect(await engineCatalog()).toEqual(catalog)
  })
})

describe('warmCatalog', () => {
  it('fetches while the network is there', async () => {
    warmCatalog()
    await vi.waitFor(() => expect(api.fetchCatalogBundle).toHaveBeenCalled())
  })

  it('does nothing offline, where the fetch would only fail', () => {
    online(false)
    warmCatalog()
    expect(api.fetchCatalogBundle).not.toHaveBeenCalled()
  })
})
