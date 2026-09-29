import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'

import {
  fetchSolution,
  type CalculateResult,
  type MapIssue,
  type SimJobStatus,
  type SimMode,
  type SimPolicy,
  type SimulationListItem,
  type SimulationRequest,
  type SolutionVariant,
} from '../api/client'
import { StoppedRun } from '../engine/client'
import { formatNum } from '../econ/view'
import { variantsSelector, fleetEdit } from '../fleet/records'
import { processesSelector } from '../projects/processRecords'
import { mapSelector } from '../map/records'
import { suggestedItem } from '../robots/suggestion'
import { ConfidenceBadge, RunExportButtons } from '../projects/RunParts'
import { runReference } from '../projects/runs'
import { StaleBanner } from '../projects/StaleBanner'
import { reasonText } from '../store/reasons'
import { useProjectStore, useStoreSelector, useUndoHistory } from '../store/useProjectStore'
import { ActionBar } from '../ui/ActionBar'
import { Select, type SelectOption } from '../ui/Select'
import { NumberInput } from '../ui/NumberInput'
import { parseNumberText } from '../ui/numberText'
import { Reflow } from '../ui/Reflow'
import { AbbreviationText } from '../ui/Abbreviation'
import { csvText, triggerDownload } from '../ui/download'
import { UnitField } from '../ui/UnitField'

import { exportFileName, runCsvRows } from './exportRun'
import { pct, policyLabels } from './format'
import { ReplayView } from './ReplayView'
import { SimSummary } from './SimSummary'
import { issuesOf, type SimSource } from './source'

const statusLabels: Record<SimJobStatus, string> = {
  queued: 'в очереди',
  running: 'считается',
  succeeded: 'готово',
  failed: 'ошибка',
  canceled: 'отменено',
}

const statusTag: Partial<Record<SimJobStatus, string>> = {
  running: 'tag-info',
  succeeded: 'tag-good',
  failed: 'tag-danger',
}

const policyOptions: SelectOption<SimPolicy>[] = (Object.keys(policyLabels) as SimPolicy[]).map((k) => ({
  value: k,
  label: policyLabels[k],
}))

const modeOptions: SelectOption<SimMode>[] = [
  { value: 'stochastic', label: 'Вероятностный (повторы)' },
  { value: 'deterministic', label: 'Детерминированный (1 прогон)' },
]

const active = (s: SimJobStatus) => s === 'queued' || s === 'running'

function fleetOf(v: SolutionVariant): number {
  return v.fleet.reduce((n, f) => n + (f.solution_id ? f.quantity : 0), 0)
}

// SimContext is what the page needs to know about the project or demo the runs belong to.
export type SimContext = {
  storeKey: string
  // results is the last calculation, whose suggested robot runs when no variant has one.
  results: CalculateResult | null | undefined
  robotsHref: string
  mapHref: string
  // historyHref is set for a project: its stale runs point to История. A demo has none.
  historyHref?: string
  stale: boolean
  // onStarted lets the owner of the page refresh what a new run changes.
  onStarted?: () => void
}

export function SimPage({ source, ctx }: { source: SimSource; ctx: SimContext }) {
  const qc = useQueryClient()
  const store = useProjectStore(ctx.storeKey)
  const [history] = useUndoHistory(ctx.storeKey)
  const listKey = ['sim-list', ...source.key]
  const simsQ = useQuery({
    queryKey: listKey,
    queryFn: () => source.list(),
    refetchInterval: (q) => (source.poll && q.state.data?.some((i) => active(i.status)) ? 1000 : false),
  })
  const [variantId, setVariantId] = useState('')
  const [form, setForm] = useState<Required<Pick<SimulationRequest, 'mode' | 'replications' | 'seed' | 'policy' | 'horizon_h' | 'sla_target_pct'>>>({
    mode: 'stochastic',
    replications: 30,
    seed: 0,
    policy: 'fifo',
    horizon_h: 8,
    sla_target_pct: 5,
  })
  const [search, setSearch] = useSearchParams()
  const runId = search.get('run')
  const setRunId = (id: string | null) => setSearch(id ? { run: id } : {}, { replace: true })
  const [error, setError] = useState('')
  const [mapIssues, setMapIssues] = useState<MapIssue[]>([])
  const [notice, setNotice] = useState<string[]>([])

  const selectMap = useMemo(() => mapSelector(), [])
  const hasMap = useStoreSelector(store, selectMap) !== null
  const selectVariants = useMemo(() => variantsSelector(), [])
  const variants = useStoreSelector(store, selectVariants)
  const selectProcesses = useMemo(() => processesSelector(), [])
  const processes = useStoreSelector(store, selectProcesses)
  const processNameOf = new Map(processes.map((p) => [p.code, p.name || p.code]))
  const withFleet = variants.filter((v) => fleetOf(v) > 0)
  const variant = variants.find((v) => v.id === variantId) ?? withFleet[0]
  // With no robot in any variant the run takes the robot and the count the last calculation suggested.
  const suggestion = withFleet.length === 0 ? suggestedItem(ctx.results) : null
  const suggested = suggestion?.estimate ? { name: suggestion.name, count: suggestion.estimate.fleet_size } : null
  const items = simsQ.data ?? []
  const running = items.find((i) => active(i.status))

  const shownRun = runId ?? items.find((i) => i.status === 'succeeded')?.run_id ?? null

  const solutionIds = [...new Set(variants.flatMap((v) => v.fleet.map((f) => f.solution_id).filter((x): x is string => Boolean(x))))]
  const solutionsQ = useQueries({
    queries: solutionIds.map((id) => ({
      queryKey: ['solution', id],
      queryFn: () => fetchSolution(id),
      staleTime: Infinity,
    })),
  })
  const names = new Map(solutionsQ.flatMap((q) => (q.data ? [[q.data.id, q.data.name] as const] : [])))

  const refreshRuns = () => {
    void qc.invalidateQueries({ queryKey: ['sim-list'] })
    void qc.invalidateQueries({ queryKey: ['sim-checks'] })
    void qc.invalidateQueries({ queryKey: ['demo-sim-checks'] })
  }
  const simConfig = () => ({
    mode: form.mode,
    replications: form.mode === 'stochastic' ? form.replications : undefined,
    seed: form.seed,
    policy: form.policy,
    horizon_h: form.horizon_h,
    sla_target_pct: form.sla_target_pct,
  })
  const createMut = useMutation({
    mutationFn: (extra?: { line_key?: string }) => source.create(variant?.id, simConfig(), extra),
    onMutate: () => {
      setError('')
      setMapIssues([])
      setNotice([])
    },
    onSuccess: (warnings) => {
      setRunId(null)
      setNotice(warnings)
      refreshRuns()
      ctx.onStarted?.()
    },
    onError: (err) => {
      if (err instanceof Error && err.message === StoppedRun) {
        return
      }
      setError(err instanceof Error ? err.message : 'Не удалось запустить симуляцию.')
      setMapIssues(issuesOf(err))
    },
  })

  const cancelMut = useMutation({
    mutationFn: (id: string) => source.cancel(id),
    onSettled: refreshRuns,
    onError: (err) => setError(err instanceof Error ? err.message : 'Не удалось отменить.'),
  })
  const retryMut = useMutation({
    mutationFn: (id: string) => (source.retry ? source.retry(id) : Promise.resolve()),
    onSettled: refreshRuns,
    onError: (err) => setError(err instanceof Error ? err.message : 'Не удалось повторить.'),
  })

  function applyFleet(v: SolutionVariant, fleet: SolutionVariant['fleet'], label?: string) {
    const edit = fleetEdit(store.getState(), v.id as string, fleet)
    const r = history.run(label ?? edit.label, edit.ops)
    if (r.outcome.status === 'rejected') {
      setError(`Изменение не применено. ${reasonText(r.outcome.reason)}`)
    }
  }

  function changeQty(v: SolutionVariant, idx: number, delta: number) {
    applyFleet(
      v,
      v.fleet.map((f, i) => (i === idx ? { ...f, quantity: Math.max(1, f.quantity + delta) } : f)),
    )
  }

  function takeN(v: SolutionVariant, lineKey: string, n: number) {
    applyFleet(
      v,
      v.fleet.map((f, i) => ((f.id || `fleet-${i + 1}`) === lineKey ? { ...f, quantity: Math.max(1, n) } : f)),
      `Взять ${n} в вариант`,
    )
  }

  function lineKeyOf(f: SolutionVariant['fleet'][number], i: number) {
    return f.id || `fleet-${i + 1}`
  }

  return (
    <section className="sim-page">
      <h1 className="sr-only">Событийная симуляция склада</h1>
      {ctx.historyHref ? <StaleBanner stale={ctx.stale} hrefHistory={ctx.historyHref} /> : null}
      {hasMap ? (
        <p>
          <Reflow>
            Карта проекта задана. <Link to={ctx.mapHref}>Открыть карту</Link>
          </Reflow>
        </p>
      ) : (
        <p className="stale-banner">
          <Reflow>
            У проекта нет карты. Симуляция возьмёт шаблон склада 84 x 52 м, результат будет предварительным.{' '}
            <Link to={ctx.mapHref}>Разметить план</Link>
          </Reflow>
        </p>
      )}

      <fieldset className="form-section">
        <legend>Параметры запуска</legend>
        {withFleet.length === 0 && !suggested ? (
          <p className="error">
            <Reflow>Ни в одном варианте нет роботов.</Reflow> <Link to={ctx.robotsHref}>Выбрать робота</Link>
          </p>
        ) : (
          <div className="sim-form is-quiet">
            {withFleet.length > 0 ? (
              <label>
                Вариант
                <Select
                  value={variant?.id ?? ''}
                  options={withFleet.map((v) => ({ value: v.id ?? '', label: `${v.name} (${fleetOf(v)} роб.)` }))}
                  onChange={setVariantId}
                />
              </label>
            ) : null}
            <label>
              Режим
              <Select value={form.mode} options={modeOptions} onChange={(mode) => setForm({ ...form, mode })} />
            </label>
            {form.mode === 'stochastic' ? (
              <label>
                Повторов
                <NumberInput
                  value={form.replications}
                  onText={(raw) => {
                    const n = parseNumberText(raw)
                    if (n !== undefined && Number.isFinite(n)) {
                      setForm({ ...form, replications: Math.max(2, Math.min(30, Math.round(n))) })
                    }
                  }}
                />
              </label>
            ) : null}
            <label>
              Номер случайной выборки
              <NumberInput
                value={form.seed}
                onText={(raw) => {
                  const n = parseNumberText(raw)
                  if (n !== undefined && Number.isFinite(n)) {
                    setForm({ ...form, seed: Math.max(0, Math.round(n)) })
                  }
                }}
              />
            </label>
            <label>
              Диспетчеризация
              <Select value={form.policy} options={policyOptions} onChange={(policy) => setForm({ ...form, policy })} />
            </label>
            <label>
              Горизонт
              <span className="sr-only">, ч</span>
              <UnitField unit="ч">
                <NumberInput
                  value={form.horizon_h}
                  onText={(raw) => {
                    const n = parseNumberText(raw)
                    if (n !== undefined && Number.isFinite(n)) {
                      setForm({ ...form, horizon_h: Math.max(0.25, Math.min(24, n)) })
                    }
                  }}
                />
              </UnitField>
            </label>
            <label>
              Допуск нарушений <AbbreviationText text="SLA" />
              <span className="sr-only">, %</span>
              <UnitField unit="%">
                <NumberInput
                  value={form.sla_target_pct}
                  onText={(raw) => {
                    const n = parseNumberText(raw)
                    if (n !== undefined && Number.isFinite(n)) {
                      setForm({ ...form, sla_target_pct: Math.max(0, Math.min(100, n)) })
                    }
                  }}
                />
              </UnitField>
            </label>
          </div>
        )}
        {suggested ? (
          <div className="fleet-quick">
            <p>
              <Reflow>
                {`Роботы в вариантах не выбраны, поэтому симуляция возьмёт предложение расчёта: ${suggested.name}, ${suggested.count} шт.`}
              </Reflow>{' '}
              <Link to={ctx.robotsHref}>Выбрать других</Link>
            </p>
          </div>
        ) : null}
        {variant ? (
          <div className="fleet-quick">
            <p>
              <Reflow>
                Флот варианта {variant.name}. Изменение количества сохраняет вариант и помечает прежние результаты устаревшими.
              </Reflow>
            </p>
            <ul>
              {variant.fleet.map((f, i) =>
                f.solution_id ? (
                  <li key={f.id ?? i}>
                    <Reflow>
                      {names.get(f.solution_id) ?? f.solution_id}: {f.quantity} шт
                      {f.task_codes && f.task_codes.length > 0
                        ? ` Процессы: ${f.task_codes.map((c) => processNameOf.get(c) ?? c).join(', ')}.`
                        : ' Все подходящие процессы.'}{' '}
                      <button type="button" onClick={() => changeQty(variant, i, -1)} disabled={f.quantity <= 1} aria-label="Минус один робот">
                        -1
                      </button>{' '}
                       <button type="button" onClick={() => changeQty(variant, i, 1)} aria-label="Плюс один робот">
                        +1
                      </button>{' '}
                      <button
                        type="button"
                        className="btn btn-text"
                        onClick={() => createMut.mutate({ line_key: lineKeyOf(f, i) })}
                        disabled={createMut.isPending || Boolean(running)}
                      >
                        Подобрать
                      </button>
                    </Reflow>
                  </li>
                ) : null,
              )}
            </ul>
          </div>
        ) : null}
        <ActionBar label="Запуск симуляции">
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => createMut.mutate()}
            disabled={(!variant && !suggested) || createMut.isPending || Boolean(running)}
          >
            {createMut.isPending ? (source.local ? 'Считаем...' : 'Ставим в очередь...') : 'Запустить симуляцию'}
          </button>
        </ActionBar>
        {error ? (
          <p className="error">
            <Reflow>{error}</Reflow>
          </p>
        ) : null}
        {mapIssues.length > 0 ? (
          <ul className="issue-list">
            {mapIssues
              .filter((i) => i.level === 'error')
              .map((i, k) => (
                <li key={k} className="error">
                  <Reflow>{i.message}</Reflow>
                </li>
              ))}
          </ul>
        ) : null}
        {notice.length > 0 ? (
          <ul className="risk-list is-warning">
            {notice.map((n) => (
              <li key={n} className="risk-warning">
                <Reflow>{n}</Reflow>
              </li>
            ))}
          </ul>
        ) : null}
      </fieldset>

      {running ? <JobProgress job={running} onCancel={() => cancelMut.mutate(running.id)} canceling={cancelMut.isPending} /> : null}
      {source.local && createMut.isPending ? <LocalProgress onCancel={() => cancelMut.mutate('')} /> : null}

      <History
        items={items}
        selected={shownRun}
        onOpen={setRunId}
        onRetry={source.retry ? (id) => retryMut.mutate(id) : undefined}
        busy={Boolean(running)}
      />

      {shownRun ? (
        <RunDetails
          key={shownRun}
          source={source}
          runId={shownRun}
          onTake={variant ? (key, n) => takeN(variant, key, n) : undefined}
        />
      ) : null}
    </section>
  )
}

function JobProgress({ job, onCancel, canceling }: { job: SimulationListItem; onCancel: () => void; canceling: boolean }) {
  return (
    <div className="job-progress" role="status" aria-live="polite">
      <p>
        <Reflow>
          Симуляция {statusLabels[job.status]}: {job.progress_pct}%, повторов {job.replications_done} из {job.replications_total}.
          {job.canceled_at ? ' Отмена запрошена.' : ''}
        </Reflow>
      </p>
      <progress max={100} value={job.progress_pct} />
      <button type="button" onClick={onCancel} disabled={canceling || Boolean(job.canceled_at)}>
        Отменить
      </button>
    </div>
  )
}

function LocalProgress({ onCancel }: { onCancel: () => void }) {
  return (
    <div className="job-progress" role="status" aria-live="polite">
      <p>
        <Reflow>Симуляция считается в этом браузере.</Reflow>
      </p>
      <progress />
      <button type="button" onClick={onCancel}>
        Отменить
      </button>
    </div>
  )
}

function History({
  items,
  selected,
  onOpen,
  onRetry,
  busy,
}: {
  items: SimulationListItem[]
  selected: string | null
  onOpen: (runId: string) => void
  onRetry?: (jobId: string) => void
  busy: boolean
}) {
  if (items.length === 0) {
    return <p><Reflow>Запусков симуляции ещё не было.</Reflow></p>
  }
  return (
    <>
      <h2>История симуляций</h2>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Дата</th>
              <th>Статус</th>
              <th>Флот</th>
              <th>Режим</th>
              <th><AbbreviationText text="SLA" /></th>
              <th className="num">Заданий в час</th>
              <th className="num">Нарушений</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {items.map((it) => {
              const brief = it.brief
              const cfg = it.config.sim
              return (
                <tr key={it.id} className={it.run_id === selected ? 'row-current' : undefined}>
                  <td>
                    {it.created_at ? new Date(it.created_at).toLocaleString('ru-RU') : ''}
                    {it.stale_vs_draft ? <div className="band">устарел</div> : null}
                  </td>
                  <td>
                    <span className={`tag ${statusTag[it.status] ?? ''}`}>{statusLabels[it.status]}</span>
                    {it.error_text ? (
                      <div className="error">
                        <Reflow>{it.error_text}</Reflow>
                      </div>
                    ) : null}
                  </td>
                  <td>
                    <Reflow>{brief.fleet ? brief.fleet.map((f) => `${f.name} x${f.quantity}`).join(', ') : ''}</Reflow>
                  </td>
                  <td>
                    <Reflow>
                      {cfg?.mode === 'deterministic' ? 'детерм.' : `${cfg?.replications ?? ''} повт.`}, {formatNum(cfg?.horizon_h, 2)} ч,{' '}
                      {policyLabels[cfg?.policy ?? 'fifo']}
                    </Reflow>
                  </td>
                  <td>
                    {brief.verdict === 'pass' ? <span className="tag tag-good">выполняется</span> : null}
                    {brief.verdict === 'fail' ? <span className="tag tag-danger">нарушается</span> : null}
                  </td>
                  <td className="num">{brief.kpi ? formatNum(brief.kpi.throughput_per_h.median, 1) : ''}</td>
                  <td className="num">{brief.kpi ? pct(brief.kpi.violation_rate.median) : ''}</td>
                  <td>
                    {it.status === 'succeeded' && it.run_id ? (
                      <button type="button" className="linkish" onClick={() => onOpen(it.run_id as string)}>
                        Открыть
                      </button>
                    ) : null}
                    {onRetry && (it.status === 'failed' || it.status === 'canceled') && !busy ? (
                      <button type="button" className="linkish" onClick={() => onRetry(it.id)}>
                        Повторить
                      </button>
                    ) : null}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </>
  )
}

function RunDetails({
  source,
  runId,
  onTake,
}: {
  source: SimSource
  runId: string
  onTake?: (lineKey: string, n: number) => void
}) {
  const qc = useQueryClient()
  const runQ = useQuery({ queryKey: ['simulation-run', ...source.key, runId], queryFn: () => source.run(runId) })
  const hasLog = Boolean(runQ.data?.artifact)
  const logQ = useQuery({
    queryKey: ['simulation-events', ...source.key, runId],
    queryFn: () => source.events(runId),
    enabled: hasLog,
    staleTime: Infinity,
  })
  const pinMut = useMutation({
    mutationFn: (pinned: boolean) => (source.pin ? source.pin(runId, pinned) : Promise.resolve()),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['simulation-run', ...source.key, runId] }),
  })

  if (runQ.isPending) {
    return <p>Загружаем результат...</p>
  }
  if (runQ.isError) {
    return (
      <p className="error">
        <Reflow>{runQ.error instanceof Error ? runQ.error.message : 'Не удалось загрузить запуск.'}</Reflow>
      </p>
    )
  }
  const data = runQ.data
  const summary = data.summary
  if (!summary) {
    return <p><Reflow>Результат появится после завершения симуляции.</Reflow></p>
  }
  const processNames = new Map(summary.result.processes.map((p) => [p.code, p.name]))
  return (
    <div className="calc-result">
      <h2>{source.local ? `Результат: ${summary.variant_name}` : `Результат: ${summary.variant_name}, версия проекта ${summary.version_no}`}</h2>
      <ConfidenceBadge level={data.run.confidence_level ?? summary.confidence_level} />
      {source.local ? null : (
        <>
          <p className="field-hint">{runReference({ kind: 'simulation', version_no: data.run.version_no })}.</p>
          <RunExportButtons runId={data.run.id} kind="simulation" disabled={data.run.status !== 'succeeded'} />
        </>
      )}
      {data.stale_vs_draft ? (
        <p className="stale-banner">
          <Reflow>
            Проект изменился после этого запуска (флот, процессы, карта или параметры). Результат показан для истории,
            запустите симуляцию заново.
          </Reflow>
        </p>
      ) : null}
      <SimSummary summary={summary} />
      {summary.fleet_search ? (
        <FleetSearchTable search={summary.fleet_search} onTake={onTake} />
      ) : null}
      {data.artifact && source.local ? (
        <p className="field-hint">
          <Reflow>Журнал: {data.artifact.event_count} событий.</Reflow>
        </p>
      ) : data.artifact ? (
        <p className="field-hint">
          <Reflow>
            Журнал: {data.artifact.event_count} событий, {formatNum(data.artifact.size_bytes / 1024, 0)} КБ в сжатом виде.{' '}
            {data.artifact.pinned
              ? 'Журнал закреплён и не удаляется.'
              : `Хранится до ${data.artifact.expires_at ? new Date(data.artifact.expires_at).toLocaleDateString('ru-RU') : 'срока хранения'}.`}{' '}
          </Reflow>
          <button type="button" className="linkish" onClick={() => pinMut.mutate(!data.artifact?.pinned)} disabled={pinMut.isPending}>
            {data.artifact.pinned ? 'Открепить' : 'Закрепить'}
          </button>
        </p>
      ) : (
        <p className="field-hint"><Reflow>Журнал событий удалён по сроку хранения. Итоги запуска сохранены.</Reflow></p>
      )}
      <div className="actions">
        <button
          type="button"
          className="btn btn-text"
          onClick={() =>
            triggerDownload(
              exportFileName(summary.variant_name, 'показатели', 'csv'),
              new Blob([csvText(runCsvRows(summary))], { type: 'text/csv;charset=utf-8' }),
            )
          }
        >
          Показатели CSV
        </button>
      </div>
      <h2>Воспроизведение повтора {summary.result.representative + 1}</h2>
      {logQ.isPending && hasLog ? <p><Reflow>Загружаем журнал событий...</Reflow></p> : null}
      {logQ.isError ? (
        <p className="error">
          <Reflow>{logQ.error instanceof Error ? logQ.error.message : 'Не удалось загрузить журнал.'}</Reflow>
        </p>
      ) : null}
      {logQ.data ? <ReplayView log={logQ.data} processNames={processNames} variantName={summary.variant_name} /> : null}
    </div>
  )
}
