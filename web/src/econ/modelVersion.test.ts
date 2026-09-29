import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import { currentModelVersion } from './modelVersion'

describe('currentModelVersion', () => {
  it('is the model version the engine reference was made with', () => {
    const cases = JSON.parse(readFileSync(resolve(__dirname, '../../../contracts/engine/reference.json'), 'utf8')) as {
      method: string
      result: { model_version?: string }
    }[]
    const versions = cases.filter((c) => c.method === 'calculate').map((c) => c.result.model_version)
    expect(versions.length).toBeGreaterThan(0)
    for (const v of versions) {
      expect(v).toBe(currentModelVersion)
    }
  })
})
