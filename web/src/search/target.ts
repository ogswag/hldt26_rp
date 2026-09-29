import { useEffect, useLayoutEffect, useRef } from 'react'

import type { SearchEntry } from './entries'

// SearchTarget is where the search just took the user. preview is an arrow step, the user is still in the search
// field; confirmed is Enter, a click or leaving the field. Pages whose target needs more than a scroll (a process
// field in its dialog, a map object, a robot behind a filter) act on it.
export type SearchTarget = { entry: SearchEntry; phase: 'preview' | 'confirmed'; at: number }

// NOTE: a jump to another page sets the target before that page mounts; the page takes a target this fresh.
const freshMs = 3000

let current: SearchTarget | null = null
const listeners = new Set<(t: SearchTarget) => void>()

export function setSearchTarget(entry: SearchEntry, phase: SearchTarget['phase']): void {
  const t = { entry, phase, at: Date.now() }
  current = t
  for (const l of listeners) {
    l(t)
  }
}

// useOnSearchTarget calls fn for every target the search sets while the page is open, and for the one that
// brought the user to it.
export function useOnSearchTarget(fn: (t: SearchTarget) => void): void {
  const ref = useRef(fn)
  useLayoutEffect(() => {
    ref.current = fn
  })
  useEffect(() => {
    const on = (t: SearchTarget) => ref.current(t)
    if (current && Date.now() - current.at < freshMs) {
      on(current)
    }
    listeners.add(on)
    return () => {
      listeners.delete(on)
    }
  }, [])
}
