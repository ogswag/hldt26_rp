// Checks that the browser build of the engine answers exactly as the native one.
//
//   go test ./internal/engine -update     # in api/, refreshes contracts/engine/reference.json
//   node scripts/ci/wasm-parity.mjs       # builds engine.wasm and compares
//
// Strings, booleans and integers must match exactly. Floats may differ by 1e-9 relative: the browser build
// rounds the last bit differently. The server is built with GOAMD64=v1 so its compiler does not fuse a
// multiply and an add into one instruction, which would widen that gap.

import { execFileSync } from 'node:child_process'
import { mkdtempSync, readFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const api = join(root, 'api')
const tolerance = 1e-9

function goEnv(name) {
  return execFileSync('go', ['env', name], { cwd: api, encoding: 'utf8' }).trim()
}

function buildWasm() {
  const dir = mkdtempSync(join(tmpdir(), 'engine-wasm-'))
  const out = join(dir, 'engine.wasm')
  execFileSync('go', ['build', '-o', out, './cmd/engine-wasm'], {
    cwd: api,
    env: { ...process.env, GOOS: 'js', GOARCH: 'wasm' },
    stdio: 'inherit',
  })
  return out
}

// diff walks both answers and reports every place they disagree.
function diff(want, got, path = '', out = []) {
  if (typeof want === 'number' && typeof got === 'number') {
    if (Number.isInteger(want) && Number.isInteger(got)) {
      if (want !== got) out.push(`${path}: ${want} vs ${got}`)
      return out
    }
    const scale = Math.max(Math.abs(want), Math.abs(got), 1)
    if (Math.abs(want - got) / scale > tolerance) out.push(`${path}: ${want} vs ${got}`)
    return out
  }
  if (Array.isArray(want) || Array.isArray(got)) {
    if (!Array.isArray(want) || !Array.isArray(got) || want.length !== got.length) {
      out.push(`${path}: length ${want?.length} vs ${got?.length}`)
      return out
    }
    want.forEach((v, i) => diff(v, got[i], `${path}[${i}]`, out))
    return out
  }
  if (want && got && typeof want === 'object' && typeof got === 'object') {
    const keys = new Set([...Object.keys(want), ...Object.keys(got)])
    for (const k of keys) diff(want[k], got[k], path ? `${path}.${k}` : k, out)
    return out
  }
  if (want !== got) out.push(`${path}: ${JSON.stringify(want)} vs ${JSON.stringify(got)}`)
  return out
}

const wasmPath = process.env.ENGINE_WASM ?? buildWasm()
const execPath = join(goEnv('GOROOT'), 'lib', 'wasm', 'wasm_exec.js')
globalThis.performance ??= (await import('node:perf_hooks')).performance
globalThis.crypto ??= (await import('node:crypto')).webcrypto
await import(execPath)

const go = new globalThis.Go()
const wasm = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject)
void go.run(wasm.instance)

const engine = globalThis.robotsEngine
if (!engine) {
  console.error('the wasm build did not register robotsEngine')
  process.exit(1)
}

// forget drops a dotted path from an answer, for values the browser fills from its own clock.
function forget(value, path) {
  const keys = path.split('.')
  let at = value
  for (const k of keys.slice(0, -1)) {
    if (at === null || typeof at !== 'object') return
    at = at[k]
  }
  if (at !== null && typeof at === 'object') delete at[keys.at(-1)]
}

const cases = JSON.parse(readFileSync(join(root, 'contracts', 'engine', 'reference.json'), 'utf8'))
let failed = 0
for (const c of cases) {
  const answer = JSON.parse(engine[c.method](JSON.stringify(c.request)))
  if (!answer.ok) {
    console.error(`${c.name}: engine failed: ${answer.error}`)
    failed += 1
    continue
  }
  for (const path of c.ignore ?? []) {
    forget(c.result, path)
    forget(answer.result, path)
  }
  const problems = diff(c.result, answer.result)
  if (problems.length > 0) {
    console.error(`${c.name}: ${problems.length} differences`)
    for (const p of problems.slice(0, 10)) console.error(`  ${p}`)
    failed += 1
  }
}

console.log(`${cases.length - failed}/${cases.length} cases match (tolerance ${tolerance})`)
process.exit(failed > 0 ? 1 : 0)
