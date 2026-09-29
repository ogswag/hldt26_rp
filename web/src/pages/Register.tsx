import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'

import { inspectInvitation, type InvitationCheck } from '../api/client'
import { tokenFromHash } from '../auth/linkToken'
import { useAuth } from '../auth/useAuth'
import { Reflow } from '../ui/Reflow'

export function Register() {
  const auth = useAuth()
  const nav = useNavigate()
  const token = tokenFromHash()
  const [invite, setInvite] = useState<InvitationCheck | null>(null)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!token) {
      return
    }
    let cancelled = false
    void inspectInvitation(token)
      .then((out) => {
        if (cancelled) {
          return
        }
        setInvite(out)
        if (out.valid && out.email) {
          setEmail(out.email)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setInvite({ valid: false, reason: 'Не удалось проверить приглашение. Обновите страницу.' })
        }
      })
    return () => {
      cancelled = true
    }
  }, [token])

  const invited = invite?.valid === true

  return (
    <section>
      <h1>Регистрация</h1>
      {invited ? (
        <p>
          <Reflow>
            {invite?.project_name
              ? `Приглашение в проект «${invite.project_name}» на роль ${invite.project_role === 'viewer' ? 'просмотра' : 'редактора'}.`
              : 'Приглашение принято. Создайте аккаунт.'}
          </Reflow>
        </p>
      ) : null}
      {invite && !invite.valid && invite.reason ? (
        <p className="error">
          <Reflow>{invite.reason}</Reflow>
        </p>
      ) : null}
      {error ? (
        <p className="error">
          <Reflow>{error}</Reflow>
        </p>
      ) : null}
      <form
        className="auth-form"
        onSubmit={(e) => {
          e.preventDefault()
          setBusy(true)
          setError('')
          auth
            .register(email, password, invited ? token : undefined)
            .then(() => {
              nav('/')
            })
            .catch((err: unknown) => {
              setError(err instanceof Error ? err.message : 'Не удалось зарегистрироваться.')
            })
            .finally(() => {
              setBusy(false)
            })
        }}
      >
        <div className="field">
          <label htmlFor="email">Email</label>
          <input
            id="email"
            type="email"
            autoComplete="username"
            readOnly={invited}
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
          {invited ? <p className="field-hint"><Reflow>Приглашение выписано на этот адрес.</Reflow></p> : null}
        </div>
        <div className="field">
          <label htmlFor="password">Пароль (от 8 символов)</label>
          <input
            id="password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        <div className="actions">
          <button type="submit" className="btn btn-primary" disabled={busy}>
            Создать аккаунт
          </button>
        </div>
      </form>
      <p>
        <Reflow>
          Уже есть аккаунт? <Link to="/login">Вход</Link>
        </Reflow>
      </p>
    </section>
  )
}
