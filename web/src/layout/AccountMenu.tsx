import { useCallback, useState } from 'react'
import { Link } from 'react-router-dom'

import { demoMode } from '../api/demo'
import { useAuth } from '../auth/useAuth'
import { useCatalogUndo } from '../catalog/undo'
import { plural } from '../fleet/format'
import { unsentEdits, useProjectStore, useStoreSelector, useSyncStatus, useUndoHistory } from '../store/useProjectStore'
import { redoKeys, undoKeys, undoStep } from '../store/useUndoShortcuts'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { UserCircleIcon, WarningIcon } from '../ui/icons'
import { Popover } from '../ui/Popover'
import { Reflow } from '../ui/Reflow'
import { Select } from '../ui/Select'
import { useThemeChoice, type ThemeChoice } from '../ui/theme'
import { RejectedList } from './RejectedList'
import { useRejected } from './useRejected'
import { syncProblem } from './syncProblem'

const themeOptions = [
  { value: 'auto', label: 'Авто' },
  { value: 'light', label: 'Светлая' },
  { value: 'dark', label: 'Тёмная' },
] as const satisfies readonly { value: ThemeChoice; label: string }[]

// initials takes up to two letters from the name part of an email: ivan.petrov@x.ru gives IP.
function initials(email: string): string {
  const name = email.split('@')[0] ?? ''
  const parts = name.split(/[._-]+/).filter(Boolean)
  const letters = parts.length > 1 ? parts[0][0] + parts[1][0] : name.slice(0, 1)
  return letters.toUpperCase()
}

type Props = {
  // storeKey is the open project or demo; empty outside one.
  storeKey: string
  readOnly: boolean
  // catalog is true on Админ, where undo belongs to this tab's catalog edits, not to the project around them.
  catalog: boolean
}

export function AccountMenu({ storeKey, readOnly, catalog }: Props) {
  const auth = useAuth()
  const [theme, setTheme] = useThemeChoice()
  const [open, setOpen] = useState(false)
  const [unsentOnExit, setUnsentOnExit] = useState(0)
  const store = useProjectStore(storeKey)
  const status = useSyncStatus(storeKey)
  const unsent = useStoreSelector(store, () => store.unsent().length)
  const [history, projectUndo] = useUndoHistory(storeKey)
  const catalogUndo = useCatalogUndo()
  const undo = catalog ? catalogUndo : projectUndo
  const show = useCallback(() => setOpen(true), [])
  const rejected = useRejected(storeKey, show)
  const problem = storeKey ? syncProblem(status, unsent) : null
  const alert = problem !== null || rejected.length > 0
  const user = auth.user

  // Signing out with edits still in the queue keeps them: they are stored under this account and go out after
  // the next sign-in. The dialog says so, because leaving mid-save otherwise looks like losing it.
  function askSignOut() {
    setOpen(false)
    const n = unsentEdits()
    if (n === 0) {
      void auth.logout()
      return
    }
    setUnsentOnExit(n)
  }

  const step = (redo: boolean) => (catalog ? catalogUndo.step(redo) : undoStep(history, redo))

  const label = alert ? 'Аккаунт и настройки. Есть проблема с сохранением' : 'Аккаунт и настройки'

  return (
    <>
      <Popover
        open={open}
        onOpenChange={setOpen}
        label="Аккаунт и настройки"
        align="end"
        className="account"
        trigger={(t) => (
          <button type="button" className="account-button" aria-label={label} title={label} data-sync={status.phase} {...t}>
            {user ? <span className="account-initials">{initials(user.email)}</span> : <UserCircleIcon size={20} />}
            {alert ? <span className="account-dot" aria-hidden="true" /> : null}
          </button>
        )}
      >
        {problem ? (
          <div className="menu-problem" role="status">
            <WarningIcon size={16} />
            <p>
              {problem.text}
              {problem.gone ? (
                <>
                  {' '}
                  <Link to="/trash" onClick={() => setOpen(false)}>
                    Корзина
                  </Link>
                </>
              ) : null}
            </p>
          </div>
        ) : null}
        {rejected.length > 0 ? (
          <div className="menu-section">
            <p className="menu-heading">
              Не применено: {rejected.length} {plural(rejected.length, 'правка', 'правки', 'правок')}
            </p>
            <RejectedList storeKey={storeKey} rejected={rejected} />
          </div>
        ) : null}
        {user ? <p className="menu-email" title={user.email}>{user.email}</p> : null}
        {readOnly ? (
          <p className="menu-note">
            <Reflow>Только просмотр: менять этот проект может владелец или редактор.</Reflow>
          </p>
        ) : null}
        {(storeKey && !readOnly) || catalog ? (
          <div className="menu-section">
            <button type="button" className="menu-item" disabled={!undo.undo} onClick={() => step(false)}>
              <span>Отменить{undo.undo ? `: ${undo.undo}` : ''}</span>
              <kbd>{undoKeys}</kbd>
            </button>
            <button type="button" className="menu-item" disabled={!undo.redo} onClick={() => step(true)}>
              <span>Вернуть{undo.redo ? `: ${undo.redo}` : ''}</span>
              <kbd>{redoKeys}</kbd>
            </button>
          </div>
        ) : null}
        <div className="menu-section menu-theme">
          <span aria-hidden="true">Тема</span>
          <Select value={theme} options={themeOptions} onChange={setTheme} aria-label="Тема" />
        </div>
        {demoMode ? null : (
          <div className="menu-section">
            {user ? (
              <>
                <Link className="menu-item" to="/trash" onClick={() => setOpen(false)}>
                  Корзина
                </Link>
                <button type="button" className="menu-item" onClick={askSignOut}>
                  Выйти
                </button>
              </>
            ) : (
              <>
                <Link className="menu-item" to="/login" onClick={() => setOpen(false)}>
                  Войти
                </Link>
                <Link className="menu-item" to="/register" onClick={() => setOpen(false)}>
                  Регистрация
                </Link>
              </>
            )}
          </div>
        )}
      </Popover>
      <ConfirmDialog
        open={unsentOnExit > 0}
        title="Выйти с неотправленными правками?"
        confirmLabel="Выйти"
        cancelLabel="Остаться"
        onCancel={() => setUnsentOnExit(0)}
        onConfirm={() => {
          setUnsentOnExit(0)
          void auth.logout()
        }}
      >
        <p>
          <Reflow>
            {unsentOnExit} {plural(unsentOnExit, 'правка', 'правки', 'правок')} ещё не{' '}
            {plural(unsentOnExit, 'ушла', 'ушли', 'ушли')} на сервер. Они останутся в этом браузере под вашей учётной
            записью и уйдут, когда вы войдёте снова.
          </Reflow>
        </p>
      </ConfirmDialog>
    </>
  )
}
