// Registers the service worker that keeps the app openable without network, and updates a page that runs an old
// build without asking. A new build installs and takes over at once. A tab that then finds itself on the old
// build reloads when it is hidden or on its next move to another page, and never while a field has focus, a
// dialog is open or work is held.

import { persistenceIdle } from '../store/writeTracker'
import { dueForUpdateCheck, isBehind, isTextEntry, safeToReload, type PageState } from './updateRules'

let holds = 0
let behind = false
let reloading = false

// holdReload keeps this tab from reloading until the returned function is called, for work a reload would cut
// off: a running simulation, a download.
export function holdReload(): () => void {
  holds++
  let released = false
  return () => {
    if (!released) {
      released = true
      holds--
    }
  }
}

function pageState(): PageState {
  const el = document.activeElement
  const typing = el instanceof HTMLElement && isTextEntry(el.tagName, (el as HTMLInputElement).type, el.isContentEditable)
  return { typing, dialog: document.querySelector('dialog[open]') !== null, holds }
}

async function reloadIfSafe(): Promise<void> {
  if (!behind || reloading || !safeToReload(pageState())) {
    return
  }
  reloading = true
  await persistenceIdle()
  if (!safeToReload(pageState())) {
    reloading = false
    return
  }
  window.location.reload()
}

// pageMoved is called when the address changes to another page of the app; the URL is already the new one, so the
// reload opens it on the new build.
export function pageMoved(): void {
  void reloadIfSafe()
}

// shellOf asks a worker which files its build needs to open the app.
function shellOf(worker: ServiceWorker): Promise<string[] | null> {
  return new Promise((resolve) => {
    const finish = (shell: string[] | null) => {
      clearTimeout(timer)
      navigator.serviceWorker.removeEventListener('message', on)
      resolve(shell)
    }
    const on = (e: MessageEvent) => {
      const shell = (e.data as { shell?: unknown } | null)?.shell
      if (Array.isArray(shell)) {
        finish(shell as string[])
      }
    }
    const timer = setTimeout(() => finish(null), 3000)
    navigator.serviceWorker.addEventListener('message', on)
    worker.postMessage('version')
  })
}

// registerServiceWorker starts the worker. entry is the path of this page's entry chunk.
export function registerServiceWorker(entry: string): void {
  if (!import.meta.env.PROD || typeof navigator === 'undefined' || !('serviceWorker' in navigator)) {
    return
  }
  window.addEventListener('load', () => {
    let lastCheck: number | null = null
    void navigator.serviceWorker.register('/sw.js').then((reg) => {
      document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'hidden') {
          void reloadIfSafe()
        } else if (dueForUpdateCheck(lastCheck, Date.now())) {
          lastCheck = Date.now()
          void reg.update().catch(() => {})
        }
      })
    })
    // A page opened after the new build was installed loaded that build from the network, so only a worker that
    // takes over while the page is open can leave it behind.
    navigator.serviceWorker.addEventListener('controllerchange', () => {
      const worker = navigator.serviceWorker.controller
      if (!worker || behind) {
        return
      }
      void shellOf(worker).then((shell) => {
        if (shell && isBehind(entry, shell)) {
          behind = true
          if (document.visibilityState === 'hidden') {
            void reloadIfSafe()
          }
        }
      })
    })
  })
}
