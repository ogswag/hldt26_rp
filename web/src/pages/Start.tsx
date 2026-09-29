import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'

import {
  copyProject,
  deleteProject,
  importProject,
  listProjects,
  restoreProject,
  type ObjectType,
  type ProjectListItem,
} from '../api/client'
import { demoMode } from '../api/demo'
import { useAuth } from '../auth/useAuth'
import { objectTypeLabel } from '../econ/view'
import { plural } from '../fleet/format'
import { demoName } from '../guest/seed'
import type { State } from '../store/apply'
import { schemaVersion } from '../store/schema.gen'
import { editedDemo, pendingEdits } from '../store/useProjectStore'
import { ActionBar } from '../ui/ActionBar'
import { Reflow } from '../ui/Reflow'
import { showToast } from '../ui/toast'
import { NewProjectDialog } from './NewProjectDialog'

const types: ObjectType[] = ['warehouse', 'airport', 'hospital']

// A hidden offer remembers the demo version it was hidden at; a later edit of the demo brings it back.
const offerKey = (t: ObjectType) => `demo-offer-hidden:${t}`

function hiddenAt(t: ObjectType): string | null {
  try {
    return localStorage.getItem(offerKey(t))
  } catch {
    return null
  }
}

function hideOffer(t: ObjectType, seq: number): void {
  try {
    localStorage.setItem(offerKey(t), String(seq))
  } catch {
    return
  }
}

export function Start() {
  const auth = useAuth()
  if (demoMode || !auth.user) {
    return <Demos />
  }
  return <Projects />
}

function Demos() {
  return (
    <section>
      <h1 className="sr-only">Демо</h1>
      <ul className="project-cards">
        {types.map((t) => (
          <li key={t} className="task-card project-card">
            <div className="task-card-head">
              <Link className="task-card-title" to={`/demo/${t}/object`}>
                {demoName(t)}
              </Link>
            </div>
          </li>
        ))}
      </ul>
    </section>
  )
}

type Offer = { type: ObjectType; seq: number; state: State }

// useDemoOffers finds the demos this browser changed before signing in, so they can become projects.
function useDemoOffers(): [Offer[], (t: ObjectType) => void] {
  const [offers, setOffers] = useState<Offer[]>([])
  useEffect(() => {
    let live = true
    void Promise.all(types.map(async (type) => ({ type, kept: await editedDemo(type) }))).then((list) => {
      if (!live) {
        return
      }
      const out: Offer[] = []
      for (const { type, kept } of list) {
        if (kept && hiddenAt(type) !== String(kept.seq)) {
          out.push({ type, seq: kept.seq, state: kept.state })
        }
      }
      setOffers(out)
    })
    return () => {
      live = false
    }
  }, [])
  const drop = (t: ObjectType) => setOffers((prev) => prev.filter((o) => o.type !== t))
  return [offers, drop]
}

// usePending counts, per project, the edits still waiting in this browser.
function usePending(items: ProjectListItem[] | undefined): Record<string, number> {
  const [counts, setCounts] = useState<Record<string, number>>({})
  const ids = (items ?? []).map((p) => p.id).join(',')
  useEffect(() => {
    let live = true
    const list = ids ? ids.split(',') : []
    void Promise.all(list.map(async (id) => [id, await pendingEdits(id)] as const)).then((pairs) => {
      if (live) {
        setCounts(Object.fromEntries(pairs))
      }
    })
    return () => {
      live = false
    }
  }, [ids])
  return counts
}

function Projects() {
  const nav = useNavigate()
  const qc = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [offers, dropOffer] = useDemoOffers()
  const q = useQuery({ queryKey: ['projects'], queryFn: listProjects })
  const pending = usePending(q.data?.items)

  const restore = useMutation({
    mutationFn: restoreProject,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['projects'] })
      void qc.invalidateQueries({ queryKey: ['trash'] })
    },
  })
  // A delete goes to the trash, so it is taken back from the toast instead of asked about first.
  const del = useMutation({
    mutationFn: (p: { id: string; name: string }) => deleteProject(p.id).then(() => p),
    onSuccess: (p) => {
      void qc.invalidateQueries({ queryKey: ['projects'] })
      void qc.invalidateQueries({ queryKey: ['trash'] })
      showToast({
        message: `Проект «${p.name}» в корзине. Он хранится 30 дней.`,
        action: { label: 'Вернуть', run: () => restore.mutate(p.id) },
      })
    },
  })
  const copy = useMutation({
    mutationFn: (id: string) => copyProject(id),
    onSuccess: (p) => {
      void qc.invalidateQueries({ queryKey: ['projects'] })
      nav(`/p/${p.id}/object`)
    },
  })
  const saveDemo = useMutation({
    mutationFn: (o: Offer) =>
      importProject({ name: demoName(o.type), schema_version: schemaVersion, collections: o.state }),
    onSuccess: (p, o) => {
      hideOffer(o.type, o.seq)
      void qc.invalidateQueries({ queryKey: ['projects'] })
      nav(`/p/${p.id}/object`)
    },
  })

  return (
    <section>
      <h1 className="sr-only">Проекты</h1>

      {offers.map((o) => (
        <div key={o.type} className="note-muted">
          <p>
            <Reflow>
              Вы меняли «{demoName(o.type)}» в этом браузере до входа. Его можно сохранить как проект в аккаунте.
            </Reflow>
          </p>
          <div className="actions">
            <button type="button" disabled={saveDemo.isPending} onClick={() => saveDemo.mutate(o)}>
              Сохранить как проект
            </button>
            <button
              type="button"
              className="btn btn-text"
              onClick={() => {
                hideOffer(o.type, o.seq)
                dropOffer(o.type)
              }}
            >
              Скрыть
            </button>
          </div>
          {saveDemo.isError && saveDemo.variables?.type === o.type ? (
            <p className="error">
              <Reflow>
                {saveDemo.error instanceof Error ? saveDemo.error.message : 'Не удалось сохранить проект.'} Повторите
                попытку.
              </Reflow>
            </p>
          ) : null}
        </div>
      ))}

      <ActionBar label="Проекты">
        <button type="button" className="btn btn-primary" data-search-id="action:new-project" onClick={() => setCreating(true)}>
          Новый проект
        </button>
      </ActionBar>
      <NewProjectDialog open={creating} onClose={() => setCreating(false)} />

      {q.isPending ? <p>Загрузка списка...</p> : null}
      {q.isError ? (
        <p className="error">
          <Reflow>
            {q.error instanceof Error ? q.error.message : 'Не удалось загрузить проекты.'} Обновите страницу.
          </Reflow>
        </p>
      ) : null}
      {del.isError ? (
        <p className="error">
          <Reflow>{del.error instanceof Error ? del.error.message : 'Не удалось удалить.'}</Reflow>
        </p>
      ) : null}
      {copy.isError ? (
        <p className="error">
          <Reflow>{copy.error instanceof Error ? copy.error.message : 'Не удалось скопировать.'}</Reflow>
        </p>
      ) : null}
      {q.data && q.data.items.length > 0 ? (
        <ul className="project-cards">
          {q.data.items.map((p) => {
            const waiting = pending[p.id] ?? 0
            return (
              <li key={p.id} className="task-card project-card">
                <div className="task-card-head">
                  <Link className="task-card-title" to={`/p/${p.id}/object`}>
                    {p.name}
                  </Link>
                  <span className={p.has_results ? (p.stale ? 'tag tag-warning' : 'tag tag-good') : 'tag'}>
                    {p.has_results ? (p.stale ? 'расчёт устарел' : 'рассчитан') : 'без расчёта'}
                  </span>
                  <span className="task-card-summary">
                    <Reflow>
                      {objectTypeLabel(p.object_type)}
                      {waiting > 0
                        ? ` · ${waiting} ${plural(waiting, 'правка не отправлена', 'правки не отправлены', 'правок не отправлены')}`
                        : ''}
                    </Reflow>
                  </span>
                </div>
                <div className="project-card-actions">
                  {p.has_results ? (
                    <Link className="button-link" to={`/p/${p.id}/calc/export`}>
                      Экспорт
                    </Link>
                  ) : null}
                  <button type="button" disabled={copy.isPending} onClick={() => copy.mutate(p.id)}>
                    Копия
                  </button>
                  <button
                    type="button"
                    className="btn btn-danger"
                    disabled={del.isPending}
                    onClick={() => del.mutate({ id: p.id, name: p.name })}
                  >
                    Удалить
                  </button>
                </div>
              </li>
            )
          })}
        </ul>
      ) : null}
    </section>
  )
}
