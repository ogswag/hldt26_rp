export type NoticeAction = { label: string; run: () => void }

// Notice reports a job the page is waiting on (busy) or one that failed. It sits under the toast and has no
// countdown: a busy notice leaves when the job ends, a failed one when the user closes it or acts on it.
export type Notice = { id: string; message: string; busy: boolean; action?: NoticeAction }

let current: Notice | null = null
const listeners = new Set<() => void>()

function emit() {
  for (const l of listeners) {
    l()
  }
}

export function showNotice(n: Notice): void {
  current = n
  emit()
}

// hideNotice closes the notice only when it is still the one with this id.
export function hideNotice(id: string): void {
  if (current?.id === id) {
    current = null
    emit()
  }
}

export function subscribeNotice(onChange: () => void): () => void {
  listeners.add(onChange)
  return () => listeners.delete(onChange)
}

export function currentNotice(): Notice | null {
  return current
}
