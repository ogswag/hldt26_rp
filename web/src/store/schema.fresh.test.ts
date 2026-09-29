import { execFileSync } from 'node:child_process'
import { join } from 'node:path'

import { expect, it } from 'vitest'

it('generated schema copy and types match contracts/ops/project.schema.json', () => {
  const script = join(__dirname, '../../../scripts/gen-ops-schema.mjs')
  expect(() => execFileSync('node', [script, '--check'], { stdio: 'pipe' })).not.toThrow()
})
