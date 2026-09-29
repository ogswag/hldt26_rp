import { useState } from 'react'

import { equal } from '../store/apply'

// useStable returns the value it returned last time while the new one is deeply equal to it, so a value rebuilt
// on every render can be a dependency without rerunning what depends on it.
export function useStable<T>(value: T): T {
  const [kept, setKept] = useState(value)
  if (kept !== value && !equal(kept, value)) {
    setKept(value)
    return value
  }
  return kept
}
