import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError, type ObjectSchema, type ObjectTypeInfo } from '../api/client'

const schema = { object_type: 'warehouse', groups: [] } as unknown as ObjectSchema
const kept = { object_type: 'warehouse', groups: [{ id: 'old', title: 'Старая копия', fields: [] }] } as unknown as ObjectSchema
const types: ObjectTypeInfo[] = [{ type: 'warehouse', name: 'Склад', description: '' } as ObjectTypeInfo]

const api = vi.hoisted(() => ({ fetchObjectSchema: vi.fn(), fetchObjectTypes: vi.fn() }))
const cache = vi.hoisted(() => ({
  cachedSchema: vi.fn(),
  putSchema: vi.fn(),
  cachedTypes: vi.fn(),
  putTypes: vi.fn(),
}))

vi.mock('../api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/client')>()),
  ...api,
}))
vi.mock('./cache', () => cache)

const { objectSchema, objectTypes } = await import('./reference')

beforeEach(() => {
  vi.clearAllMocks()
  api.fetchObjectSchema.mockResolvedValue(schema)
  api.fetchObjectTypes.mockResolvedValue({ items: types })
  cache.cachedSchema.mockResolvedValue(null)
  cache.cachedTypes.mockResolvedValue(null)
})

describe('objectSchema', () => {
  it('takes the fresh schema and keeps it for a visit without network', async () => {
    expect(await objectSchema('warehouse')).toEqual(schema)
    expect(cache.putSchema).toHaveBeenCalledWith('warehouse', schema)
  })

  it('answers from the kept copy when the request never reached the server', async () => {
    api.fetchObjectSchema.mockRejectedValue(new TypeError('Failed to fetch'))
    cache.cachedSchema.mockResolvedValue(kept)
    expect(await objectSchema('warehouse')).toEqual(kept)
  })

  it('reports a server failure instead of hiding it behind stale data', async () => {
    api.fetchObjectSchema.mockRejectedValue(new ApiError('Внутренняя ошибка.', 500, undefined, []))
    cache.cachedSchema.mockResolvedValue(kept)
    await expect(objectSchema('warehouse')).rejects.toThrow('Внутренняя ошибка.')
  })

  it('keeps a refusal the server sent instead of papering over it', async () => {
    api.fetchObjectSchema.mockRejectedValue(new ApiError('Неизвестный тип объекта.', 404, undefined, []))
    cache.cachedSchema.mockResolvedValue(kept)
    await expect(objectSchema('warehouse')).rejects.toThrow('Неизвестный тип объекта.')
  })

  it('passes the failure on when nothing was kept', async () => {
    api.fetchObjectSchema.mockRejectedValue(new TypeError('Failed to fetch'))
    await expect(objectSchema('warehouse')).rejects.toThrow('Failed to fetch')
  })
})

describe('objectTypes', () => {
  it('keeps the list it fetched', async () => {
    expect(await objectTypes()).toEqual({ items: types })
    expect(cache.putTypes).toHaveBeenCalledWith(types)
  })

  it('answers from the kept list without network', async () => {
    api.fetchObjectTypes.mockRejectedValue(new TypeError('Failed to fetch'))
    cache.cachedTypes.mockResolvedValue(types)
    expect(await objectTypes()).toEqual({ items: types })
  })
})
