// Project events over SSE. One EventSource per project per browser: the tab holding the Web Lock opens it and
// relays events to the project's other tabs through a BroadcastChannel. HTTP/1.1 allows six connections per
// origin, so a stream in every tab would starve the page's own requests.

export type EventHandler = (type: string, data: unknown) => void

const TYPES = ['ops', 'presence', 'runs', 'access', 'deleted', 'reset']
const MAX_RETRY = 60_000

function parse(raw: string): unknown {
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

// openSource keeps one EventSource open. The browser retries a dropped connection by itself; after an HTTP
// error it gives up, so the source is reopened with backoff.
function openSource(url: string, onEvent: EventHandler): () => void {
  let es: EventSource | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  let delay = 1000
  const connect = () => {
    es = new EventSource(url)
    for (const type of TYPES) {
      es.addEventListener(type, (e) => {
        delay = 1000
        onEvent(type, parse((e as MessageEvent<string>).data))
      })
    }
    es.onerror = () => {
      if (es?.readyState === EventSource.CLOSED) {
        es.close()
        timer = setTimeout(connect, delay)
        delay = Math.min(MAX_RETRY, delay * 2)
      }
    }
  }
  connect()
  return () => {
    clearTimeout(timer)
    es?.close()
  }
}

export function subscribeEvents(projectId: string, onEvent: EventHandler): () => void {
  const url = `/api/projects/${projectId}/events`
  const locks = typeof navigator !== 'undefined' ? navigator.locks : undefined
  if (!locks || typeof BroadcastChannel === 'undefined') {
    return openSource(url, onEvent)
  }
  const channel = new BroadcastChannel(`sse:${projectId}`)
  channel.onmessage = (e: MessageEvent<{ type: string; data: unknown }>) => onEvent(e.data.type, e.data.data)
  const abort = new AbortController()
  let release = () => {}
  const held = new Promise<void>((resolve) => {
    release = resolve
  })
  locks
    .request(`sse:${projectId}`, { signal: abort.signal }, async () => {
      // Nobody held the stream for a moment: events may be missing.
      onEvent('reset', {})
      const stop = openSource(url, (type, data) => {
        onEvent(type, data)
        channel.postMessage({ type, data })
      })
      await held
      stop()
    })
    .catch(() => {})
  return () => {
    abort.abort()
    release()
    channel.close()
  }
}
