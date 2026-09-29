import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'

import { confirmPasswordReset, requestPasswordReset } from '../api/client'
import { tokenFromHash } from '../auth/linkToken'
import { Reflow } from '../ui/Reflow'

// PasswordReset asks for the letter, and sets the new password when the page is opened from its link.
export function PasswordReset() {
  const nav = useNavigate()
  const token = tokenFromHash()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [sent, setSent] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  if (token) {
    return (
      <section>
        <h1>Новый пароль</h1>
        <p><Reflow>Задайте пароль. Другие устройства выйдут из аккаунта.</Reflow></p>
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
            confirmPasswordReset(token, password)
              .then(() => {
                nav('/login')
              })
              .catch((err: unknown) => {
                setError(err instanceof Error ? err.message : 'Не удалось задать пароль.')
              })
              .finally(() => {
                setBusy(false)
              })
          }}
        >
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
              Сохранить пароль
            </button>
          </div>
        </form>
      </section>
    )
  }

  return (
    <section>
      <h1>Восстановление пароля</h1>
      <p><Reflow>Укажите email аккаунта. Мы пришлём ссылку, она работает один час.</Reflow></p>
      {error ? (
        <p className="error">
          <Reflow>{error}</Reflow>
        </p>
      ) : null}
      {sent ? (
        <p className="note">
          <Reflow>{sent}</Reflow>
        </p>
      ) : null}
      <form
        className="auth-form"
        onSubmit={(e) => {
          e.preventDefault()
          setBusy(true)
          setError('')
          requestPasswordReset(email)
            .then((out) => {
              setSent(out.message || 'Если такой аккаунт есть, письмо со ссылкой уже отправлено.')
            })
            .catch((err: unknown) => {
              setError(err instanceof Error ? err.message : 'Не удалось отправить письмо.')
            })
            .finally(() => {
              setBusy(false)
            })
        }}
      >
        <div className="field">
          <label htmlFor="email">Email</label>
          <input id="email" type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} />
        </div>
        <div className="actions">
          <button type="submit" className="btn btn-primary" disabled={busy}>
            Прислать ссылку
          </button>
        </div>
      </form>
      <p>
        <Reflow>
          Вспомнили пароль? <Link to="/login">Вход</Link>
        </Reflow>
      </p>
    </section>
  )
}
