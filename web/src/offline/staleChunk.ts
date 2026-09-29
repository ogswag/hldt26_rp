// A deployment replaces the hashed code chunks, so a tab that is still on the old build fails to load its next
// lazy page. One reload gets it the new build; a second failure right after means the reload did not help.

import { mayReloadForChunk } from './updateRules'

const KEY = 'robots-chunk-reload'

// reloadForStaleChunk reloads the page once per window and says whether it did.
export function reloadForStaleChunk(): boolean {
  const now = Date.now()
  let last: number | null = null
  try {
    const kept = sessionStorage.getItem(KEY)
    last = kept === null ? null : Number(kept)
    if (!mayReloadForChunk(Number.isFinite(last) ? last : null, now)) {
      return false
    }
    sessionStorage.setItem(KEY, String(now))
  } catch {
    // Without storage the guard cannot be kept, and a loop is worse than an error page.
    return false
  }
  window.location.reload()
  return true
}

// guardPreloadErrors reloads for a chunk Vite could not preload, before the failure reaches a page.
export function guardPreloadErrors(): void {
  window.addEventListener('vite:preloadError', (e) => {
    if (reloadForStaleChunk()) {
      e.preventDefault()
    }
  })
}
