import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'

import {
  fetchCalculation,
  fetchProjectVersion,
  type CalculationResponse,
  type ObjectType,
  type ProjectSnapshot,
  type SchemaField,
} from '../api/client'
import { RiskList, ScenarioTable } from '../econ/ScenarioTable'
import { formatNum, formatRub, objectTypeLabel, tariffLabel } from '../econ/view'
import { isObjectType } from '../guest/store'
import { errorKind } from '../layout/errorKind'
import { ErrorPage } from '../layout/ErrorPage'
import { objectSchema } from '../offline/reference'
import { unitOf } from '../params/unit'
import { valueText } from '../params/valueText'
import { ConfidenceBadge, RunExportButtons, RunIdentity } from '../projects/RunParts'
import { Reflow } from '../ui/Reflow'
import { AbbreviationText } from '../ui/Abbreviation'


export function RunView() {
  const { projectId, runId } = useParams()
  const runQ = useQuery({
    queryKey: ['calculation', runId],
    queryFn: () => fetchCalculation(runId as string),
    enabled: Boolean(runId),
  })
  const run = runQ.data?.run
  const snapQ = useQuery({
    queryKey: ['project-version', projectId, run?.project_version_id],
    queryFn: () => fetchProjectVersion(projectId as string, run?.project_version_id as string),
    enabled: Boolean(projectId && run?.project_version_id),
    staleTime: Infinity,
  })
  const objectType = snapQ.data?.object_type
  const schemaQ = useQuery({
    queryKey: ['schema', objectType],
    queryFn: () => objectSchema(objectType as ObjectType),
    enabled: isObjectType(objectType ?? null),
    staleTime: Infinity,
  })

  if (runQ.isPending) {
    return (
      <section>
        <h1>Запуск расчёта</h1>
        <p>Загрузка...</p>
      </section>
    )
  }
  if (runQ.isError || !runQ.data.run) {
    const info = runQ.isError ? errorKind(runQ.error) : { kind: 'projectNotFound' as const }
    const missing = info.kind === 'projectNotFound'
    return (
      <ErrorPage
        {...info}
        title={missing ? 'Запуск не найден' : undefined}
        primary={missing && projectId ? { label: 'К истории запусков', to: `/p/${projectId}/calc/history` } : undefined}
        onRetry={() => void runQ.refetch()}
      />
    )
  }

  const data: CalculationResponse = runQ.data
  const meta = runQ.data.run
  return (
    <section>
      <h1>
        Расчёт экономики, версия проекта {meta.version_no}
      </h1>
      <p>
        <Reflow>
          {data.project_name ? `Проект ${data.project_name}. ` : ''}
          {objectTypeLabel(data.object_type)}. Результат и входы этого запуска неизменяемы.
        </Reflow>
      </p>
      {data.stale_vs_draft ? (
        <p className="stale-banner">
          <Reflow>
            Черновик проекта изменён после этого запуска. Здесь показан исторический результат. Отчёт по нему можно
            скачать, он будет помечен как исторический.{' '}
            <Link to={`/p/${projectId}/calc`}>Открыть актуальный расчёт</Link>
          </Reflow>
        </p>
      ) : null}
      <p>
        <Link to={`/p/${projectId}/calc/history`}>К истории запусков</Link>
      </p>
      <ConfidenceBadge level={meta.confidence_level} />
      <RunIdentity run={meta} />
      <RunExportButtons runId={meta.id} kind="calculation" />

      <ScenarioTable result={data} />
      <RiskList risks={data.risks} />

      {data.variants && data.variants.length > 0 ? (
        <>
          <h2>Состав вариантов</h2>
          {data.variants.map((v) => (
            <div key={v.variant_id} className="fleet-row">
              <strong>{v.name}</strong>
              {v.fleet.length === 0 ? (
                <p className="field-hint"><Reflow>Флот не задан.</Reflow></p>
              ) : (
                <ul className="reason-list">
                  {v.fleet.map((f) => (
                    <li key={f.solution_id}>
                      <Reflow>
                        {f.name}: {f.quantity} шт, цена {formatRub(f.cash_price_rub)}.
                        {f.price_source === 'project_override'
                          ? ` (проектная цена, причина: ${f.price_override_reason || 'не указана'})`
                          : ' (каталог)'}
                      </Reflow>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          ))}
        </>
      ) : null}

      {snapQ.data ? <SnapshotInputs snapshot={snapQ.data} fields={schemaQ.data?.groups.flatMap((g) => g.fields) ?? []} /> : null}
      {snapQ.isError ? <p className="error"><Reflow>Не удалось загрузить снимок входов.</Reflow></p> : null}

      {data.assumptions.length > 0 ? (
        <details>
          <summary>Допущения расчёта</summary>
          <ul>
            {data.assumptions.map((a) => (
              <li key={a}>
                <Reflow>{a}</Reflow>
              </li>
            ))}
          </ul>
        </details>
      ) : null}
    </section>
  )
}

function SnapshotInputs({ snapshot, fields }: { snapshot: ProjectSnapshot; fields: SchemaField[] }) {
  const d = snapshot.draft
  const params = d.params ?? {}
  const unknown = fields.filter((f) => params[f.id] === null)
  return (
    <>
      <h2>Входы запуска</h2>
      {unknown.length > 0 ? (
        <p className="stale-banner">
          <Reflow>Неизвестные значения: {unknown.map((f) => f.label).join(', ')}.</Reflow>
        </p>
      ) : null}
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Процесс</th>
              <th>Спрос в сутки</th>
              <th><AbbreviationText text="SLA" /> ожидание / цикл, мин</th>
              <th>Персонал</th>
            </tr>
          </thead>
          <tbody>
            {d.processes.map((p) => (
              <tr key={p.code}>
                <td>
                  <Reflow>{p.name}</Reflow>
                </td>
                <td>
                  <Reflow>
                    {formatNum(p.demand.units_per_day ?? 0, 0)} {p.demand.unit ?? ''}
                  </Reflow>
                </td>
                <td>
                  {formatNum(p.sla.max_wait_min ?? 0, 0)} / {formatNum(p.sla.max_cycle_min ?? 0, 0)}
                </td>
                <td>
                  <Reflow>
                    {formatNum(p.baseline_staff.headcount ?? 0, 0)} {p.baseline_staff.role ?? ''}
                  </Reflow>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="field-hint">
        <Reflow>
          Финансирование:{' '}
          {d.variants
            .map((v) => `${v.name}: ${v.financing.map((f) => (f.kind === 'raas' ? `RaaS ${tariffLabel(f.tariff ?? undefined)}` : 'покупка')).join(', ')}`)
            .join('; ')}
          . Карта {d.map ? 'задана' : 'не задана'}.
        </Reflow>
      </p>
      {fields.length > 0 ? (
        <details>
          <summary>Параметры объекта</summary>
          <div className="table-wrap">
            <table>
              <tbody>
                {fields.map((f) => (
                  <tr key={f.id}>
                    <th>{f.label}</th>
                    <td>
                      <Reflow>
                        {valueText(f, params[f.id])} {params[f.id] !== null && params[f.id] !== undefined ? unitOf(f) : ''}
                      </Reflow>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      ) : null}
    </>
  )
}
