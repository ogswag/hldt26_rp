import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { createInvitation, listInvitations, listProjects, revokeInvitation, type Invitation } from '../../api/client'
import { formatDateTime } from '../../projects/runs'
import { ActionBar } from '../../ui/ActionBar'
import { ConfirmDialog } from '../../ui/ConfirmDialog'
import { FieldRow } from '../../ui/FieldRow'
import { Reflow } from '../../ui/Reflow'
import { Select, type SelectOption } from '../../ui/Select'

type Role = 'editor' | 'viewer'

const roleOptions: SelectOption<Role>[] = [
  { value: 'editor', label: 'Редактор' },
  { value: 'viewer', label: 'Только просмотр' },
]

// inviteState is what became of an invitation, for its tag.
function inviteState(i: Invitation, now: number): { label: string; tag: string; open: boolean } {
  if (i.accepted_at) {
    return { label: 'принято', tag: 'tag tag-good', open: false }
  }
  if (i.revoked_at) {
    return { label: 'отозвано', tag: 'tag', open: false }
  }
  if (i.expires_at && Date.parse(i.expires_at) < now) {
    return { label: 'истекло', tag: 'tag', open: false }
  }
  return { label: 'ждёт ответа', tag: 'tag tag-info', open: true }
}

function projectText(i: Invitation): string {
  if (!i.project_id) {
    return 'без проекта'
  }
  const role = i.project_role === 'viewer' ? 'просмотр' : 'редактор'
  return `${i.project_name ?? 'проект'}, ${role}`
}

function errorText(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback
}

export function AdminInvitations() {
  const qc = useQueryClient()
  const [email, setEmail] = useState('')
  const [project, setProject] = useState('')
  const [role, setRole] = useState<Role>('editor')
  const [note, setNote] = useState('')
  const [revoking, setRevoking] = useState<Invitation | null>(null)
  const list = useQuery({ queryKey: ['admin', 'invitations'], queryFn: listInvitations })
  const projects = useQuery({ queryKey: ['projects'], queryFn: listProjects })

  const create = useMutation({
    mutationFn: () =>
      createInvitation({ email: email.trim(), ...(project ? { project_id: project, project_role: role } : {}) }),
    onSuccess: (inv) => {
      setEmail('')
      setNote(`Приглашение для ${inv.email} создано, письмо со ссылкой отправлено.`)
      void qc.invalidateQueries({ queryKey: ['admin', 'invitations'] })
    },
  })
  const revoke = useMutation({
    mutationFn: (id: string) => revokeInvitation(id),
    onSuccess: () => {
      setRevoking(null)
      void qc.invalidateQueries({ queryKey: ['admin', 'invitations'] })
    },
  })

  const projectOptions: SelectOption[] = [
    { value: '', label: 'Без проекта' },
    ...(projects.data?.items ?? []).map((p) => ({ value: p.id, label: p.name })),
  ]
  // NOTE: an invitation's state is read against the time the page opened; the list refreshes after each change.
  const [now] = useState(() => Date.now())
  const items = list.data?.items ?? []

  return (
    <section>
      <h1 className="sr-only">Приглашения</h1>
      <form
        id="invite-form"
        className="param-section"
        aria-labelledby="invite-new-h"
        onSubmit={(e) => {
          e.preventDefault()
          setNote('')
          create.mutate()
        }}
      >
        <h2 id="invite-new-h">Новое приглашение</h2>
        <div className="field-rows">
          <FieldRow id="invite-email" label="Email">
            <input
              id="invite-email"
              type="email"
              autoComplete="off"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </FieldRow>
          <FieldRow
            id="invite-project"
            label="Проект"
            help="Без проекта приглашение открывает регистрацию. С проектом человек после регистрации сразу становится его участником."
          >
            <Select id="invite-project" value={project} options={projectOptions} onChange={setProject} />
          </FieldRow>
          {project ? (
            <FieldRow id="invite-role" label="Роль в проекте">
              <Select id="invite-role" value={role} options={roleOptions} onChange={setRole} />
            </FieldRow>
          ) : null}
        </div>
        {create.isError ? (
          <p className="error">
            <Reflow>{errorText(create.error, 'Не удалось создать приглашение. Повторите.')}</Reflow>
          </p>
        ) : null}
      </form>
      <section aria-labelledby="invite-list-h">
        <h2 id="invite-list-h">Отправленные</h2>
        {list.isPending ? <p>Загрузка приглашений...</p> : null}
        {list.isError ? (
          <p className="error">
            <Reflow>{errorText(list.error, 'Не удалось загрузить приглашения. Обновите страницу.')}</Reflow>
          </p>
        ) : null}
        {list.isSuccess && items.length === 0 ? <p>Приглашений пока нет.</p> : null}
        {items.length > 0 ? (
          <div className="table-wrap">
            <table className="invites-table">
              <thead>
                <tr>
                  <th>Email</th>
                  <th>Проект</th>
                  <th>Состояние</th>
                  <th>Отправлено</th>
                  <th>Действует до</th>
                  <th>Пригласил</th>
                  <th>
                    <span className="sr-only">Действия</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {items.map((i) => {
                  const st = inviteState(i, now)
                  return (
                    <tr key={i.id}>
                      <td className="clamp-cell" title={i.email}>
                        {i.email}
                      </td>
                      <td>{projectText(i)}</td>
                      <td>
                        <span className={st.tag}>{st.label}</span>
                      </td>
                      <td className="nowrap">{formatDateTime(i.created_at)}</td>
                      <td className="nowrap">{formatDateTime(i.expires_at)}</td>
                      <td>{i.invited_by ?? 'нет данных'}</td>
                      <td>
                        {st.open ? (
                          <button type="button" className="btn btn-danger" onClick={() => setRevoking(i)}>
                            Отозвать
                          </button>
                        ) : null}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        ) : null}
      </section>
      <ActionBar label="Действия с приглашениями" status={note || undefined}>
        <button type="submit" form="invite-form" className="btn btn-primary" disabled={create.isPending}>
          Отправить приглашение
        </button>
      </ActionBar>
      <ConfirmDialog
        open={revoking !== null}
        title="Отозвать приглашение?"
        confirmLabel="Отозвать"
        cancelLabel="Отмена"
        busy={revoke.isPending}
        error={revoke.isError ? errorText(revoke.error, 'Не удалось отозвать. Повторите.') : undefined}
        onConfirm={() => {
          if (revoking) {
            revoke.mutate(revoking.id)
          }
        }}
        onCancel={() => setRevoking(null)}
      >
        <p>
          <Reflow>Ссылка из письма для {revoking?.email ?? ''} перестанет работать. Чтобы пригласить снова, отправьте новое приглашение.</Reflow>
        </p>
      </ConfirmDialog>
    </section>
  )
}
