import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Link } from 'react-router-dom'

import {
  downloadGuestExport,
  downloadProjectExport,
  fetchCalculation,
  fetchProject,
  type ExportFormat,
  type ObjectType,
} from '../api/client'
import { demoMode } from '../api/demo'
import { ScenarioTable } from '../econ/ScenarioTable'
import { useCalcResult } from '../econ/useCalcResult'
import { PreliminaryNote } from '../engine/PreliminaryNote'
import { overridesForRequest } from '../guest/store'
import { ConfidenceBadge, RunIdentity } from '../projects/RunParts'
import { formatDateTime } from '../projects/runs'
import { ActionBar } from '../ui/ActionBar'
import { Reflow } from '../ui/Reflow'

export function Export() {
  const calc = useCalcResult()
  const [busy, setBusy] = useState<ExportFormat | 'recalc' | null>(null)
  const [error, setError] = useState('')
  const projectId = calc.projectId
  const projectQ = useQuery({
    queryKey: ['project', projectId],
    queryFn: () => fetchProject(projectId as string),
    enabled: Boolean(projectId),
  })
  const currentRunId = projectQ.data?.current_run_id ?? null
  const runQ = useQuery({
    queryKey: ['calculation', currentRunId],
    queryFn: () => fetchCalculation(currentRunId as string),
    enabled: Boolean(currentRunId),
  })

  if (calc.needType) {
    return (
      <section>
        <h1 className="sr-only">Экспорт</h1>
        <p><Reflow>Сначала выберите тип объекта и выполните расчёт.</Reflow></p>
        <p>
          <Link to={calc.href('object')}>К выбору объекта</Link>
        </p>
      </section>
    )
  }

  if (calc.loading && !calc.result) {
    return (
      <section>
        <h1 className="sr-only">Экспорт</h1>
        <p><Reflow>Готовим результат расчёта...</Reflow></p>
      </section>
    )
  }

  if (calc.error && !calc.result) {
    return (
      <section>
        <h1 className="sr-only">Экспорт</h1>
        <p className="error">
          <Reflow>{calc.error}</Reflow>
        </p>
        <p>
          <Link to={calc.href('object')}>К параметрам</Link>
        </p>
      </section>
    )
  }

  const result = calc.result
  const objectType = calc.objectType as ObjectType
  const stale = Boolean(projectId && calc.stale)
  const run = runQ.data?.run

  async function download(format: ExportFormat) {
    setError('')
    setBusy(format)
    try {
      if (projectId) {
        await downloadProjectExport(projectId, format, objectType)
      } else {
        await downloadGuestExport(
          {
            object_type: objectType,
            params: calc.params,
            include_ids: calc.includeIds,
            overrides: overridesForRequest(calc.storedOverrides),
            seed: result?.seed,
          },
          format,
        )
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Не удалось скачать файл.')
    } finally {
      setBusy(null)
    }
  }

  async function recalc() {
    setError('')
    setBusy('recalc')
    try {
      await calc.recalc()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Не удалось пересчитать.')
    } finally {
      setBusy(null)
    }
  }

  return (
    <section>
      <h1 className="sr-only">Экспорт</h1>
      {stale ? (
        <div className="stale-banner">
          <p>
            <Reflow>
              Черновик изменился после последнего расчёта. Отчёт по текущему результату недоступен, чтобы не выдать старые
              цифры за актуальные. Пересчитайте проект или скачайте отчёт исторического запуска.
            </Reflow>
          </p>
          <div className="actions">
            <button type="button" disabled={busy !== null} onClick={() => void recalc()}>
              {busy === 'recalc' ? 'Пересчитываем...' : 'Пересчитать'}
            </button>
            <Link className="button-link" to={calc.href('calc', 'history')}>
              История запусков
            </Link>
          </div>
        </div>
      ) : null}
      {projectId && run && !stale ? (
        <>
          <ConfidenceBadge level={run.confidence_level} />
          <RunIdentity run={run} />
        </>
      ) : null}
      {!projectId ? <ConfidenceBadge level="preliminary" /> : null}
      <PreliminaryNote preliminary={calc.preliminary} />
      {error ? (
        <p className="error">
          <Reflow>{error}</Reflow>
        </p>
      ) : null}
      <ActionBar
        label="Скачивание отчёта"
        status={
          stale
            ? 'Расчёт устарел: пересчитайте, чтобы скачать отчёт'
            : calc.preliminary
              ? 'Предварительный расчёт: отчёт появится после расчёта на сервере'
              : run
                ? `Отчёт по расчёту от ${formatDateTime(run.created_at)}`
                : ''
        }
      >
        <button
          type="button"
          className="btn btn-primary"
          data-search-id="action:pdf"
          disabled={busy !== null || !result || stale || calc.preliminary}
          onClick={() => void download('pdf')}
        >
          {busy === 'pdf' ? 'Готовим PDF...' : 'Скачать PDF'}
        </button>
        <button
          type="button"
          data-search-id="action:xlsx"
          disabled={busy !== null || !result || stale || calc.preliminary}
          onClick={() => void download('xlsx')}
        >
          {busy === 'xlsx' ? 'Готовим Excel...' : 'Скачать Excel'}
        </button>
      </ActionBar>
      {demoMode ? (
        <p>
          <Reflow>
            Это демо работает без сервера: расчёт идёт в браузере, а проект остаётся в нём же. Отчёт с идентификатором
            запуска выдаёт рабочая установка.
          </Reflow>
        </p>
      ) : !projectId ? (
        <p>
          <Reflow>
            Демо-расчёт не хранится на сервере, поэтому отчёт не привязан к запуску. Чтобы получить отчёт с
            идентификатором запуска, войдите: изменённое демо можно сохранить как проект на странице проектов.
          </Reflow>
        </p>
      ) : null}
      {result && !stale ? (
        <>
          <ScenarioTable result={result} />
        </>
      ) : null}
    </section>
  )
}
