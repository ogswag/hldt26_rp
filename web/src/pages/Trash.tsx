import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'

import { listTrash, purgeProject, restoreProject, type TrashItem } from '../api/client'
import { useAuth } from '../auth/useAuth'
import { objectTypeLabel } from '../econ/view'
import { keptText } from '../projects/trash'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { Reflow } from '../ui/Reflow'
import { showToast } from '../ui/toast'

export function Trash() {
  const auth = useAuth()
  const nav = useNavigate()
  const qc = useQueryClient()
  const [purging, setPurging] = useState<TrashItem | null>(null)
  const q = useQuery({ queryKey: ['trash'], queryFn: listTrash, enabled: Boolean(auth.user) })

  const restore = useMutation({
    mutationFn: restoreProject,
    onSuccess: (p) => {
      void qc.invalidateQueries({ queryKey: ['trash'] })
      void qc.invalidateQueries({ queryKey: ['projects'] })
      showToast({ message: `Проект «${p.name}» восстановлен.`, action: { label: 'Открыть', run: () => nav(`/p/${p.id}/object`) } })
    },
  })
  const purge = useMutation({
    mutationFn: purgeProject,
    onSuccess: () => {
      setPurging(null)
      void qc.invalidateQueries({ queryKey: ['trash'] })
    },
  })

  if (!auth.user) {
    return (
      <section>
        <h1>Корзина</h1>
        <p>
          <Reflow>
            Войдите, чтобы увидеть удалённые проекты. <Link to="/login">Вход</Link>
          </Reflow>
        </p>
      </section>
    )
  }

  if (q.isPending) {
    return (
      <section>
        <h1>Корзина</h1>
        <p>Загрузка списка...</p>
      </section>
    )
  }

  if (q.isError) {
    return (
      <section>
        <h1>Корзина</h1>
        <p className="error">
          <Reflow>{q.error instanceof Error ? q.error.message : 'Неизвестная ошибка'}</Reflow>
        </p>
      </section>
    )
  }

  const items = q.data.items

  return (
    <section>
      <h1>Корзина</h1>
      <p><Reflow>Удалённые проекты хранятся 30 дней. Потом они стираются насовсем.</Reflow></p>
      {restore.isError ? (
        <p className="error">
          <Reflow>{restore.error instanceof Error ? restore.error.message : 'Не удалось восстановить.'}</Reflow>
        </p>
      ) : null}
      {items.length === 0 ? (
        <p>
          <Reflow>
            Корзина пуста. <Link to="/">К проектам</Link>
          </Reflow>
        </p>
      ) : (
        <ul className="project-cards">
          {items.map((p) => (
            <li key={p.id} className="task-card project-card">
              <div className="task-card-head">
                <span className="task-card-title">{p.name}</span>
                <span className="task-card-summary">
                  {objectTypeLabel(p.object_type)} · {keptText(p)}
                </span>
              </div>
              <div className="project-card-actions">
                <button type="button" disabled={restore.isPending} onClick={() => restore.mutate(p.id)}>
                  Восстановить
                </button>
                <button type="button" className="btn btn-danger" onClick={() => setPurging(p)}>
                  Удалить навсегда
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
      <ConfirmDialog
        open={purging !== null}
        title="Удалить проект навсегда?"
        confirmLabel="Удалить навсегда"
        cancelLabel="Отмена"
        busy={purge.isPending}
        error={purge.isError ? 'Не удалось удалить. Повторите запрос.' : undefined}
        onConfirm={() => {
          if (purging) {
            purge.mutate(purging.id)
          }
        }}
        onCancel={() => setPurging(null)}
      >
        <p>
          <Reflow>
            Проект «{purging?.name}» и все его расчёты, симуляции и карта будут стёрты. Восстановить их будет нельзя.
          </Reflow>
        </p>
      </ConfirmDialog>
    </section>
  )
}
