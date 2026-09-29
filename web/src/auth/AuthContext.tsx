import { useEffect, useMemo, useState, useSyncExternalStore, type ReactNode } from 'react'

import { login as apiLogin, logout as apiLogout, refreshSession, register as apiRegister } from '../api/client'
import { demoMode } from '../api/demo'
import { getSession, setSession, subscribeSession } from './session'
import { AuthContext, type AuthState } from './state'

export function AuthProvider({ children }: { children: ReactNode }) {
  const session = useSyncExternalStore(subscribeSession, getSession)
  // A demo build has no server to ask: everyone there is a guest, and the app renders at once.
  const [loading, setLoading] = useState(!demoMode)

  useEffect(() => {
    if (demoMode) {
      return
    }
    let cancelled = false
    void refreshSession().finally(() => {
      if (!cancelled) {
        setLoading(false)
      }
    })
    return () => {
      cancelled = true
    }
  }, [])

  const value = useMemo<AuthState>(
    () => ({
      user: session?.user ?? null,
      loading,
      login: async (email: string, password: string) => {
        const out = await apiLogin({ email, password })
        setSession({ user: out.user, csrfToken: out.csrf_token })
      },
      register: async (email: string, password: string, inviteToken?: string) => {
        const out = await apiRegister({ email, password, ...(inviteToken ? { invite_token: inviteToken } : {}) })
        setSession({ user: out.user, csrfToken: out.csrf_token })
      },
      logout: async () => {
        try {
          await apiLogout()
        } finally {
          setSession(null)
        }
      },
    }),
    [session, loading],
  )

  // Until /api/auth/me answers, the app would render as a guest and then flip; one round trip is not worth the flash.
  if (loading) {
    return null
  }
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
