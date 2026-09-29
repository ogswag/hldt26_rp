// The decisions behind a silent update, free of the browser so they can be tested: when a page runs an old
// build, and when reloading it is safe.

export type PageState = {
  // typing is true while a text field or an editable region has focus: a draft may only exist there.
  typing: boolean
  dialog: boolean
  // holds counts work a reload would cut off, such as a running simulation or a download.
  holds: number
}

export function safeToReload(s: PageState): boolean {
  return !s.typing && !s.dialog && s.holds === 0
}

const noText = new Set(['button', 'checkbox', 'color', 'file', 'image', 'radio', 'range', 'reset', 'submit'])

// isTextEntry says whether a focused element takes typed text.
export function isTextEntry(tag: string, type: string | undefined, editable: boolean): boolean {
  if (editable) {
    return true
  }
  const t = tag.toLowerCase()
  if (t === 'textarea') {
    return true
  }
  return t === 'input' && !noText.has((type ?? 'text').toLowerCase())
}

// isBehind tells that the build the worker now carries no longer has this page's entry code: the page is old.
export function isBehind(entry: string, shell: readonly string[]): boolean {
  return !shell.includes(entry)
}

const UPDATE_CHECK_EVERY = 60_000

// dueForUpdateCheck limits how often a tab asks the server for a newer worker.
export function dueForUpdateCheck(last: number | null, now: number): boolean {
  return last === null || now - last >= UPDATE_CHECK_EVERY
}

const CHUNK_RELOAD_WINDOW = 30_000

// mayReloadForChunk allows one reload per window for a page whose code chunk is gone. A second failure inside
// the window means the reload did not help, so the page shows the error instead of looping.
export function mayReloadForChunk(last: number | null, now: number): boolean {
  return last === null || now - last >= CHUNK_RELOAD_WINDOW || now < last
}
