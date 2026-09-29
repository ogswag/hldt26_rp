import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'

import { fetchProject, fetchRuns, type RunKind } from '../api/client'
import { RunExportButtons } from '../projects/RunParts'
import {
  canExportRun,
  canOpenRun,
  confidenceLabel,
  formatDateTime,
  runBriefText,
  runKindLabel,
  runOpenHref,
  runStatusLabel,
} from '../projects/runs'
import { Reflow } from '../ui/Reflow'
import { Select, type SelectOption } from '../ui/Select'

type Filter = 'all' | RunKind

const filterOptions: SelectOption<Filter>[] = [
  { value: 'all', label: 'все запуски' },
  { value: 'calculation', label: 'расчёты экономики' },
  { value: 'simulation', label: 'симуляции' },
]

export function Runs() {
  const { projectId } = useParams()
  const [filter, setFilter] = useState<Filter>('all')
  const [exportFor, setExportFor] = useState<string | null>(null)
  const projectQ = useQuery({
    queryKey: ['project', projectId],
    queryFn: () => fetchProject(projectId as string),
    enabled: Boolean(projectId),
  })
  const runsQ = useQuery({
    queryKey: ['runs', projectId],
    queryFn: () => fetchRuns(projectId as string),
    enabled: Boolean(projectId),
    refetchInterval: (q) => (q.state.data?.items.some((i) => i.status === 'queued' || i.status === 'running') ? 2000 : false),
  })

  if (!projectId) {
    return (
      <section>
        <h1 className="sr-only">История запусков</h1>
        <p><Reflow>История запусков есть только у сохранённого проекта. Войдите и создайте проект.</Reflow></p>
      </section>
    )
  }

  if (projectQ.isPending || runsQ.isPending) {
    return (
      <section>
        <h1 className="sr-only">История запусков</h1>
        <p>Загрузка...</p>
      </section>
    )
  }

  if (projectQ.isError || runsQ.isError) {
    const err = projectQ.error ?? runsQ.error
    return (
      <section>
        <h1 className="sr-only">История запусков</h1>
        <p className="error">
          <Reflow>{err instanceof Error ? err.message : 'Не удалось загрузить историю.'}</Reflow>
        </p>
      </section>
    )
  }

  const project = projectQ.data
  const items = runsQ.data.items.filter((i) => filter === 'all' || i.kind === filter)

  return (
    <section>
      <h1 className="sr-only">История запусков</h1>
      {project.stale ? (
        <p className="stale-banner">
          <Reflow>
            Текущий черновик отличается от последнего расчёта. Старые запуски можно открыть и выгрузить, но они описывают
            прежние входы. <Link to={`/p/${projectId}/calc`}>Открыть «Расчёт»</Link>
          </Reflow>
        </p>
      ) : null}
      <div className="actions filter-bar">
        <label>
          Показать
          <Select value={filter} options={filterOptions} onChange={setFilter} />
        </label>
      </div>
      {items.length === 0 ? (
        <p><Reflow>Ещё нет сохранённых запусков. Расчёт появится здесь, когда вы откроете вкладку «Расчёт».</Reflow></p>
      ) : (
        <div className="table-wrap">
          <table className="runs-table">
            <thead>
              <tr>
                <th>Версия</th>
                <th>Запуск</th>
                <th>Итог</th>
                <th>Подтверждённость</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {items.map((run) => (
                <tr
                  key={run.id}
                  className={run.is_current ? 'row-current' : undefined}
                  data-run-id={run.id}
                  data-run-kind={run.kind}
                  data-search-id={`run:${run.id}`}
                >
                  <td>
                    {run.version_no}
                    {run.is_current ? <div className="band">текущий</div> : null}
                    {run.stale_vs_draft ? <div className="band">устарел</div> : null}
                  </td>
                  <td>
                    {runKindLabel(run.kind)}
                    <div className="band">
                      {formatDateTime(run.created_at)}, {runStatusLabel(run.status)}
                    </div>
                  </td>
                  <td>{runBriefText(run)}</td>
                  <td title={confidenceLabel(run.confidence_level).meaning}>{confidenceLabel(run.confidence_level).name}</td>
                  <td>
                    {canOpenRun(run) ? (
                      <Link to={runOpenHref(projectId, run)}>Открыть</Link>
                    ) : run.kind === 'simulation' ? (
                      <Link to={`/p/${projectId}/calc/sim`}>Симуляция</Link>
                    ) : null}
                    {canExportRun(run) ? (
                      exportFor === run.id ? (
                        <RunExportButtons runId={run.id} kind={run.kind} />
                      ) : (
                        <div>
                          <button type="button" className="linkish" onClick={() => setExportFor(run.id)}>
                            Отчёт
                          </button>
                        </div>
                      )
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
