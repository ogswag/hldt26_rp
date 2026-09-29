import type { State } from './apply'

const known = new WeakMap<State, string>()

function canonical(value: unknown): string {
  if (Array.isArray(value)) {
    return `[${value.map(canonical).join(',')}]`
  }
  if (value !== null && typeof value === 'object') {
    const obj = value as Record<string, unknown>
    return `{${Object.keys(obj)
      .sort()
      .map((k) => `${JSON.stringify(k)}:${canonical(obj[k])}`)
      .join(',')}}`
  }
  return JSON.stringify(value) ?? 'null'
}

// stateFingerprint names the content of a project's records: two states with the same records have the same
// fingerprint whatever order their keys were written in. It keys what is computed from a demo in this browser.
export function stateFingerprint(state: State): string {
  let out = known.get(state)
  if (out === undefined) {
    const text = canonical(state)
    let h1 = 0xdeadbeef
    let h2 = 0x41c6ce57
    for (let i = 0; i < text.length; i++) {
      const c = text.charCodeAt(i)
      h1 = Math.imul(h1 ^ c, 2654435761)
      h2 = Math.imul(h2 ^ c, 1597334677)
    }
    h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909)
    h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909)
    out = (4294967296 * (2097151 & h2) + (h1 >>> 0)).toString(36)
    known.set(state, out)
  }
  return out
}
