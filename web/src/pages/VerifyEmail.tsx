import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'

import { resendVerification, verifyEmail } from '../api/client'
import { tokenFromHash } from '../auth/linkToken'
import { useAuth } from '../auth/useAuth'
import { Reflow } from '../ui/Reflow'

type State = 'checking' | 'done' | 'failed' | 'no-token'

// VerifyEmail confirms the address from the letter's link, and asks for a new letter when the link is stale.
export function VerifyEmail() {
  const auth = useAuth()
  const [state, setState] = useState<State>(() => (tokenFromHash() ? 'checking' : 'no-token'))
  const [error, setError] = useState('')
  const [sent, setSent] = useState(false)
  const request = useRef<Promise<unknown> | null>(null)

  useEffect(() => {
    const token = tokenFromHash()
    if (!token) {
      return
    }
    let cancelled = false
    // NOTE: the token works once, so an effect that runs again (StrictMode, remount) waits on the first request.
    request.current ??= verifyEmail(token)
    request.current
      .then(() => {
        if (!cancelled) {
          setState('done')
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Не удалось подтвердить email.')
          setState('failed')
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <section>
      <h1>Подтверждение email</h1>
      {state === 'checking' ? <p>Проверяем ссылку...</p> : null}
      {state === 'done' ? (
        <p>
          <Reflow>
            Адрес подтверждён. <Link to="/">К проектам</Link>
          </Reflow>
        </p>
      ) : null}
      {state === 'no-token' ? (
        <p>
          <Reflow>Откройте ссылку из письма целиком: в ней есть код после знака #.</Reflow>
        </p>
      ) : null}
      {state === 'failed' ? (
        <p className="error">
          <Reflow>{error}</Reflow>
        </p>
      ) : null}
      {state !== 'done' && auth.user ? (
        <div className="actions">
          <button
            type="button"
            className="btn btn-primary"
            disabled={sent}
            onClick={() => {
              void resendVerification().then(() => setSent(true))
            }}
          >
            {sent ? 'Письмо отправлено' : 'Прислать письмо снова'}
          </button>
        </div>
      ) : null}
      {state !== 'done' && !auth.user ? (
        <p>
          <Reflow>
            Войдите, чтобы запросить письмо снова. <Link to="/login">Вход</Link>
          </Reflow>
        </p>
      ) : null}
    </section>
  )
}
