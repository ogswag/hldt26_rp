// Ids made in the browser: record and transaction ids, and the tab's client id.

const hex = (b: number) => b.toString(16).padStart(2, '0')

// uuid returns a lowercase v4 UUID. crypto.randomUUID needs a secure context, which a LAN address over HTTP is not.
export function uuid(): string {
  if (typeof crypto.randomUUID === 'function' && globalThis.isSecureContext !== false) {
    return crypto.randomUUID()
  }
  const b = crypto.getRandomValues(new Uint8Array(16))
  b[6] = (b[6] & 0x0f) | 0x40
  b[8] = (b[8] & 0x3f) | 0x80
  const s = Array.from(b, hex).join('')
  return `${s.slice(0, 8)}-${s.slice(8, 12)}-${s.slice(12, 16)}-${s.slice(16, 20)}-${s.slice(20)}`
}

const KEY = 'robots-client-id'

type Kept = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>
type Events = Pick<EventTarget, 'addEventListener'>

// tabIds makes the function that names a tab to the server. sessionStorage holds the id only while the page is
// away (reloading, closing, frozen): a reload finds it and keeps its queue, while "Duplicate tab" copies the
// storage of a live page, which holds nothing, so the copy gets an id of its own instead of sharing a queue.
export function tabIds(kept: Kept | null, win: Events | null, doc: Events | null): () => string {
  let memo: string | null = null
  const away = () => {
    try {
      if (memo) {
        kept?.setItem(KEY, memo)
      }
    } catch {
      // A blocked storage costs the tab its queue on reload, which adoption then finds.
    }
  }
  const back = () => {
    try {
      kept?.removeItem(KEY)
    } catch {
      // Same as above.
    }
  }
  return () => {
    if (memo) {
      return memo
    }
    try {
      memo = kept?.getItem(KEY) ?? null
      kept?.removeItem(KEY)
    } catch {
      memo = null
    }
    memo ||= uuid()
    win?.addEventListener('pagehide', away)
    win?.addEventListener('pageshow', back)
    doc?.addEventListener('freeze', away)
    doc?.addEventListener('resume', back)
    return memo
  }
}

function keptStorage(): Kept | null {
  try {
    return typeof sessionStorage === 'undefined' ? null : sessionStorage
  } catch {
    return null
  }
}

// clientId names this tab to the server. It survives a reload, so the tab finds its own queue again.
export const clientId = tabIds(
  keptStorage(),
  typeof window === 'undefined' ? null : window,
  typeof document === 'undefined' ? null : document,
)

let page: string | null = null

// pageId names this page load in the presence list. It is new on every load, so an old page's goodbye can never
// remove the entry its reloaded successor just made.
export function pageId(): string {
  page ??= uuid()
  return page
}
