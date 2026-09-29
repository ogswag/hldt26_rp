import { useEffect, useState, type ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'

import { Reflow } from '../ui/Reflow'
import type { ErrorKind } from './errorKind'

type Props = {
  kind: ErrorKind
  status?: number
  requestId?: string
  message?: string
  // title and primary replace the kind's own heading and main action, for a page that knows better
  // («Запуск не найден», «К истории запусков»).
  title?: string
  primary?: { label: string; to: string }
  onRetry?: () => void
  // standalone draws the page without the shell, for when the shell itself failed.
  standalone?: boolean
}

const unsentKept = 'Правки, которые ещё не отправлены, хранятся в браузере и не пропадут.'

const texts: Record<ErrorKind, { title: string; text: string }> = {
  notFound: {
    title: 'Страница не найдена',
    text: 'По этому адресу ничего нет. Проверьте ссылку или откройте список проектов.',
  },
  projectNotFound: {
    title: 'Проект не найден',
    text: 'Проекта нет, или у вас нет к нему доступа. Проверьте ссылку или попросите владельца добавить вас в проект.',
  },
  trashed: {
    title: 'Проект в корзине',
    text: 'Проект удалён в корзину. Корзина хранит проекты 30 дней: восстановите его там, чтобы продолжить.',
  },
  signIn: {
    title: 'Нужно войти',
    text: 'Эта страница открывается только после входа. Войдите, и она откроется снова.',
  },
  noAccess: {
    title: 'Нет доступа',
    text: 'Эта страница только для администраторов.',
  },
  serverDown: {
    title: 'Сервер не отвечает',
    text: `Не удалось связаться с сервером. ${unsentKept} Повторите через минуту; когда сеть вернётся, страница повторит сама.`,
  },
  crash: {
    title: 'Сбой приложения',
    text: `Страница не открылась из-за ошибки в приложении. ${unsentKept} Перезагрузите страницу; если сбой повторится, отправьте разработчикам текст ошибки.`,
  },
  updated: {
    title: 'Вышла новая версия',
    text: `Приложение обновилось, и часть страницы не загрузилась. Перезагрузите страницу. ${unsentKept}`,
  },
}

function CopyCode({ label, code }: { label: string; code: string }) {
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) {
      return
    }
    const t = setTimeout(() => setCopied(false), 2000)
    return () => clearTimeout(t)
  }, [copied])
  return (
    <p className="error-code">
      <span>{label}: </span>
      <code>{code}</code>{' '}
      <button
        type="button"
        className="btn btn-text"
        onClick={() => {
          void navigator.clipboard?.writeText(code).then(() => setCopied(true))
        }}
      >
        {copied ? 'Скопировано' : 'Скопировать'}
      </button>
    </p>
  )
}

// Explains why a page did not open and offers one way on. The HTTP status and
// the request id are the one place a user sees codes: they are what a report to the developers needs.
export function ErrorPage({ kind, status, requestId, message, title, primary, onRetry, standalone }: Props) {
  const loc = useLocation()
  const navigate = useNavigate()
  const t = texts[kind]
  const text = kind === 'noAccess' && message ? message : t.text
  const reload = () => window.location.reload()
  const retry = onRetry ?? reload

  useEffect(() => {
    if (kind !== 'serverDown') {
      return
    }
    window.addEventListener('online', retry)
    return () => window.removeEventListener('online', retry)
  }, [kind, retry])

  const toProjects = (
    <Link className="btn" to="/">
      К проектам
    </Link>
  )
  let actions: ReactNode
  if (primary) {
    actions = (
      <Link className="btn btn-primary" to={primary.to}>
        {primary.label}
      </Link>
    )
  } else {
    switch (kind) {
      case 'notFound':
      case 'projectNotFound':
      case 'noAccess':
        actions = (
          <>
            <Link className="btn btn-primary" to="/">
              К проектам
            </Link>
            {window.history.length > 1 ? (
              <button type="button" className="btn" onClick={() => navigate(-1)}>
                Назад
              </button>
            ) : null}
          </>
        )
        break
      case 'trashed':
        actions = (
          <>
            <Link className="btn btn-primary" to="/trash">
              Открыть корзину
            </Link>
            {toProjects}
          </>
        )
        break
      case 'signIn':
        actions = (
          <Link className="btn btn-primary" to={`/login?next=${encodeURIComponent(loc.pathname + loc.search)}`}>
            Войти
          </Link>
        )
        break
      case 'serverDown':
        actions = (
          <button type="button" className="btn btn-primary" onClick={retry}>
            Повторить
          </button>
        )
        break
      case 'crash':
        actions = (
          <>
            <button type="button" className="btn btn-primary" onClick={reload}>
              Перезагрузить
            </button>
            {standalone ? (
              <a className="btn" href="/">
                К проектам
              </a>
            ) : (
              toProjects
            )}
          </>
        )
        break
      case 'updated':
        actions = (
          <button type="button" className="btn btn-primary" onClick={reload}>
            Перезагрузить
          </button>
        )
        break
    }
  }

  const body = (
    <section className="error-page">
      {status ? <p className="error-status">Ошибка {status}</p> : null}
      <h1>{title ?? t.title}</h1>
      <p>
        <Reflow>{text}</Reflow>
      </p>
      <div className="error-actions">{actions}</div>
      {requestId && (kind === 'serverDown' || kind === 'crash') ? <CopyCode label="Код запроса" code={requestId} /> : null}
      {kind === 'crash' && !requestId && message ? <CopyCode label="Текст ошибки" code={message} /> : null}
    </section>
  )
  if (standalone) {
    return <main className="page">{body}</main>
  }
  return body
}
