import { getSession } from '../auth/session'
import { deleter, lowerFirst, restoreOps, title, values } from '../store/rejected'
import { reasonText } from '../store/reasons'
import { projectSchema, type Rejected } from '../store/store'
import { useProjectStore, useUndoHistory } from '../store/useProjectStore'
import { showToast } from '../ui/toast'

const time = new Intl.DateTimeFormat('ru-RU', { hour: '2-digit', minute: '2-digit' })

// RejectedList shows the refused edits with a way to write each again, copy its values or hide it.
export function RejectedList({ storeKey, rejected }: { storeKey: string; rejected: readonly Rejected[] }) {
  const store = useProjectStore(storeKey)
  const [history] = useUndoHistory(storeKey)
  const self = getSession()?.user.id ?? null

  function restore(r: Rejected) {
    const ops = restoreOps(projectSchema(), store.getState(), r.intended)
    if (ops.length === 0) {
      store.dismiss(r.txId)
      return
    }
    const out = history.run(`Восстановить: ${lowerFirst(title(r))}`, ops)
    if (out.outcome.status === 'rejected') {
      showToast({ message: `Не удалось восстановить. ${reasonText(out.outcome.reason)}` })
      return
    }
    store.dismiss(r.txId)
  }

  function copy(r: Rejected) {
    void navigator.clipboard
      .writeText(values(r))
      .then(() => showToast({ message: 'Значение скопировано.' }))
      .catch(() => showToast({ message: 'Браузер не дал скопировать значение.' }))
  }

  return (
    <ul className="rejected-list">
      {[...rejected].reverse().map((r) => (
        <li key={r.txId}>
          <p className="rejected-title">{title(r)}</p>
          <p className="rejected-reason">
            {reasonText(r.reason)}
            {r.actor ? ` · ${deleter(r, self)}` : ''} · {time.format(r.at)}
          </p>
          <p className="rejected-acts">
            {r.intended.length > 0 ? (
              <button type="button" className="linkish" onClick={() => restore(r)}>
                Восстановить
              </button>
            ) : null}
            {values(r) ? (
              <button type="button" className="linkish" onClick={() => copy(r)}>
                Скопировать значение
              </button>
            ) : null}
            <button type="button" className="linkish" onClick={() => store.dismiss(r.txId)}>
              Скрыть
            </button>
          </p>
        </li>
      ))}
    </ul>
  )
}
