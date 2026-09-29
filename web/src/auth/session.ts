import type { AuthUser } from '../api/client'

// The session itself is an HttpOnly cookie. The page keeps only the user and the CSRF token, in memory.
export type Session = { user: AuthUser; csrfToken: string }

let current: Session | null = null
const listeners = new Set<() => void>()

// Bearer-token logins stored these; the cookie replaced them.
try {
  localStorage.removeItem('auth-token')
  localStorage.removeItem('auth-user')
} catch {
  // Storage can be blocked; nothing to clean then.
}

export function getSession(): Session | null {
  return current
}

export function setSession(next: Session | null): void {
  if (current === next) {
    return
  }
  current = next
  for (const fn of listeners) {
    fn()
  }
}

export function subscribeSession(fn: () => void): () => void {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}
