import { useInfiniteQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Link } from 'react-router-dom'

import { fetchAuditEvents, type AuditEvent } from '../../api/client'
import { useCatalogFields } from '../../catalog/fields'
import { formatDateTime } from '../../projects/runs'
import { CheckSelect } from '../../ui/CheckSelect'
import { Reflow } from '../../ui/Reflow'
import { Select, type SelectOption } from '../../ui/Select'
import { useAdminHref } from './adminHref'
import { actionLabel, actionLabels, auditSections, detailLines, targetText } from './auditText'

const pageSize = 100

const sectionOptions: SelectOption[] = [{ value: '', label: 'Все разделы' }, ...auditSections.map((s) => ({ value: s.value, label: s.label }))]

export function AdminAudit() {
  const fields = useCatalogFields()
  const href = useAdminHref()
  const [section, setSection] = useState('')
  const [actions, setActions] = useState<string[]>([])
  const shownActions = section ? (auditSections.find((s) => s.value === section)?.actions ?? []) : Object.keys(actionLabels)
  const chosen = actions.filter((a) => shownActions.includes(a))
  const q = useInfiniteQuery({
    queryKey: ['admin', 'audit', section, chosen],
    queryFn: ({ pageParam }) =>
      fetchAuditEvents({ target_type: section || undefined, actions: chosen, before: pageParam, limit: pageSize }),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => (last.items.length === pageSize ? last.items.at(-1)?.id : undefined),
  })
  const items: AuditEvent[] = q.data?.pages.flatMap((p) => p.items) ?? []

  return (
    <section>
      <h1 className="sr-only">Журнал</h1>
      <div className="actions filter-bar">
        <label>
          Раздел
          <Select
            value={section}
            options={sectionOptions}
            onChange={(v) => {
              setSection(v)
              setActions([])
            }}
          />
        </label>
        <div className="filter-field">
          <span aria-hidden="true">Действия</span>
          <CheckSelect
            label="Действия"
            options={shownActions.map((a) => ({ value: a, label: actionLabel(a) }))}
            value={chosen}
            onChange={setActions}
            empty="В этом разделе нет действий."
          />
        </div>
      </div>
      {q.isPending ? <p>Загрузка журнала...</p> : null}
      {q.isError ? (
        <p className="error">
          <Reflow>{q.error instanceof Error ? q.error.message : 'Не удалось загрузить журнал. Обновите страницу.'}</Reflow>
        </p>
      ) : null}
      {q.isSuccess && items.length === 0 ? <p>Записей с такими условиями нет.</p> : null}
      {items.length > 0 ? (
        <div className="table-wrap">
          <table className="audit-table">
            <colgroup>
              <col className="audit-col-at" />
              <col className="audit-col-who" />
              <col className="audit-col-action" />
              <col className="audit-col-target" />
              <col />
            </colgroup>
            <thead>
              <tr>
                <th>Время</th>
                <th>Кто</th>
                <th>Действие</th>
                <th>Объект</th>
                <th>Подробности</th>
              </tr>
            </thead>
            <tbody>
              {items.map((e) => {
                const lines = detailLines(e, fields)
                const robot = e.target_type === 'solution' && e.target_name && e.target_id
                return (
                  <tr key={e.id}>
                    <td className="nowrap">{formatDateTime(e.at)}</td>
                    <td className="clamp-cell" title={e.actor_email ?? undefined}>
                      {e.actor_email ?? 'система'}
                    </td>
                    <td>{actionLabel(e.action)}</td>
                    <td className="clamp-cell">
                      {robot ? <Link to={href('catalog', `robot=${e.target_id}`)}>{targetText(e)}</Link> : targetText(e)}
                    </td>
                    <td>
                      {lines.map((line, i) => (
                        <div key={i}>
                          <Reflow>{line}</Reflow>
                        </div>
                      ))}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      ) : null}
      {q.hasNextPage ? (
        <p>
          <button type="button" className="btn" disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
            {q.isFetchingNextPage ? 'Загружаем...' : 'Показать ещё'}
          </button>
        </p>
      ) : null}
    </section>
  )
}
