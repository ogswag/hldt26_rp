import { useState } from 'react'

import { downloadRunExport, type ExportFormat, type RunKind, type RunMeta } from '../api/client'
import { Reflow } from '../ui/Reflow'

import { confidenceLabel, confidenceLevels, formatDateTime, runReference, runStatusLabel } from './runs'

export function ConfidenceBadge({ level }: { level: string | undefined }) {
  const current = confidenceLabel(level)
  return (
    <div className="confidence" aria-label={`Уровень подтверждённости: ${current.name}`}>
      <p className="confidence-title">
        Уровень подтверждённости: <strong>{current.name}</strong>
      </p>
      <ol className="confidence-ladder">
        {confidenceLevels.map((lv) => (
          <li key={lv} className={lv === (level ?? 'preliminary') ? 'confidence-step current' : 'confidence-step'}>
            {confidenceLabel(lv).name}
          </li>
        ))}
      </ol>
      <p className="field-hint">
        <Reflow>{current.meaning}</Reflow>
      </p>
    </div>
  )
}

export function RunIdentity({ run }: { run: RunMeta }) {
  return (
    <dl className="run-identity">
      <dt>Запуск</dt>
      <dd>
        {runReference(run)} ({runStatusLabel(run.status)}
        {run.is_current ? ', текущий результат проекта' : ''})
      </dd>
      <dt>Версия проекта</dt>
      <dd>{run.version_no}</dd>
      <dt>Создан</dt>
      <dd>{formatDateTime(run.created_at)}</dd>
    </dl>
  )
}

export function RunExportButtons({ runId, kind, disabled }: { runId: string; kind: RunKind; disabled?: boolean }) {
  const [busy, setBusy] = useState<ExportFormat | null>(null)
  const [error, setError] = useState('')

  async function download(format: ExportFormat) {
    setError('')
    setBusy(format)
    try {
      await downloadRunExport(runId, kind, format)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Не удалось скачать отчёт.')
    } finally {
      setBusy(null)
    }
  }

  return (
    <>
      <div className="actions">
        <button type="button" disabled={disabled || busy !== null} onClick={() => void download('pdf')}>
          {busy === 'pdf' ? 'Готовим PDF...' : 'Отчёт PDF'}
        </button>
        <button type="button" disabled={disabled || busy !== null} onClick={() => void download('xlsx')}>
          {busy === 'xlsx' ? 'Готовим Excel...' : 'Отчёт Excel'}
        </button>
      </div>
      {error ? (
        <p className="error">
          <Reflow>{error}</Reflow>
        </p>
      ) : null}
    </>
  )
}
