import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'

import { useAuth } from '../auth/useAuth'
import { Reflow } from '../ui/Reflow'

export function Login() {
  const auth = useAuth()
  const nav = useNavigate()
  const [params] = useSearchParams()
  // next is where an error page sent the user from; only an address inside the app is followed.
  const raw = params.get('next') ?? ''
  const next = raw.startsWith('/') && !raw.startsWith('//') ? raw : '/'
  const [email, setEmail] = useState('user@demo.local')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  return (
    <section>
      <h1>Вход</h1>
      <p>Демо: user@demo.local / demo-user и admin@demo.local / demo-admin.</p>
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
            .login(email, password)
            .then(() => {
              nav(next, { replace: true })
            })
            .catch((err: unknown) => {
              setError(err instanceof Error ? err.message : 'Не удалось войти.')
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
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <div className="field">
          <label htmlFor="password">Пароль</label>
          <input
            id="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        <div className="actions">
          <button type="submit" className="btn btn-primary" disabled={busy}>
            Войти
          </button>
        </div>
      </form>
      <p>
        <Reflow>
          Нет аккаунта? <Link to="/register">Регистрация</Link>
        </Reflow>
      </p>
      <p>
        <Reflow>
          Забыли пароль? <Link to="/reset-password">Восстановить</Link>
        </Reflow>
      </p>
    </section>
  )
}
