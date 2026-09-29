import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState } from 'react'

import {
  ApiError,
  applyCatalogImport,
  createCatalogImport,
  deleteCatalogImport,
  fetchCatalogImport,
  mapCatalogImport,
  type CatalogImport,
  type ImportMode,
  type ImportSheet,
} from '../../api/client'
import { CloseIcon } from '../../ui/icons'
import { Reflow } from '../../ui/Reflow'
import { useCatalogFields } from '../fields'
import { CheckStep } from './CheckStep'
import { ColumnsStep } from './ColumnsStep'
import { DoneStep } from './DoneStep'
import { FileStep } from './FileStep'
import { applyBody, applyCount, defaultChoice, mappingOf, type Choice, type Mapping } from './plan'
import { readCatalogFile } from './readFile'

const steps = [
  { id: 'file', label: 'Файл' },
  { id: 'columns', label: 'Колонки' },
  { id: 'check', label: 'Проверка' },
  { id: 'done', label: 'Готово' },
] as const

type Step = (typeof steps)[number]['id']

// NOTE: the server takes 10 MB of cells; a workbook is packed, so a larger file is refused before it is read.
const maxFileBytes = 20 << 20
const pollMs = 1500

type Props = {
  onClose: () => void
  // onOpenRobot shows a robot the upload saved, in place of this overlay.
  onOpenRobot: (id: string) => void
}

function errorText(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback
}

// dropDraft removes an upload that was never applied; the server would drop it after a day anyway.
function dropDraft(draft: { current: string | null }) {
  if (draft.current) {
    void deleteCatalogImport(draft.current).catch(() => undefined)
    draft.current = null
  }
}

function refreshCatalog(qc: QueryClient) {
  void qc.invalidateQueries({ queryKey: ['solutions'] })
  void qc.invalidateQueries({ queryKey: ['solution'] })
  void qc.invalidateQueries({ queryKey: ['search-robots'] })
}

// Uploads a table into the catalog in four steps: the file, its columns, a preview to pick from, and
// the result. Uses the robot overlay frame without the list: the steps run in a row along the top.
export function ImportOverlay({ onClose, onOpenRobot }: Props) {
  const ref = useRef<HTMLDialogElement>(null)
  const uid = useId()
  const qc = useQueryClient()
  const fields = useCatalogFields()
  const [mode, setMode] = useState<ImportMode>('catalog')
  const [file, setFile] = useState<{ name: string; sheets: ImportSheet[] } | null>(null)
  const [imp, setImp] = useState<CatalogImport | null>(null)
  const [step, setStep] = useState<Step>('file')
  const [choice, setChoice] = useState<Choice | null>(null)
  // pending is the column mapping the admin chose while the server has not answered yet.
  const [pending, setPending] = useState<Mapping | null>(null)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  // latest numbers the requests, so an answer that comes after a newer request is dropped.
  const latest = useRef(0)
  const draftId = useRef<string | null>(null)

  useEffect(() => {
    const d = ref.current
    if (d && !d.open) {
      d.showModal()
      d.querySelector<HTMLElement>('.import-drop .linkish')?.focus()
    }
    return () => {
      dropDraft(draftId)
      if (d?.open) {
        d.close()
      }
    }
  }, [])

  const photosId = imp?.status === 'photos' ? imp.id : null
  useEffect(() => {
    if (!photosId) {
      return
    }
    let stopped = false
    let timer = 0
    const poll = () => {
      fetchCatalogImport(photosId)
        .then((next) => {
          if (stopped) {
            return
          }
          setImp(next)
          if (next.status === 'photos') {
            timer = window.setTimeout(poll, pollMs)
          } else {
            refreshCatalog(qc)
          }
        })
        .catch(() => {
          if (!stopped) {
            timer = window.setTimeout(poll, pollMs * 2)
          }
        })
    }
    timer = window.setTimeout(poll, pollMs)
    return () => {
      stopped = true
      window.clearTimeout(timer)
    }
  }, [photosId, qc])

  // run does one request at a time from the overlay's point of view: a newer one makes an older answer stale.
  const run = <T,>(label: string, work: () => Promise<T>, done: (out: T) => void, fallback: string, failed?: (err: unknown) => void) => {
    const n = ++latest.current
    setBusy(label)
    setError('')
    work()
      .then((out) => {
        if (n === latest.current) {
          done(out)
        }
      })
      .catch((err: unknown) => {
        if (n === latest.current) {
          setError(errorText(err, fallback))
          failed?.(err)
        }
      })
      .finally(() => {
        if (n === latest.current) {
          setBusy('')
          setPending(null)
        }
      })
  }

  const show = (next: CatalogImport) => {
    setImp(next)
    draftId.current = next.status === 'draft' ? next.id : null
    setChoice(defaultChoice(next))
  }

  const start = (m: ImportMode, name: string, sheets: ImportSheet[]) => {
    dropDraft(draftId)
    run(
      'Сверяем файл с каталогом...',
      () => createCatalogImport({ file_name: name, mode: m, sheets }),
      (next) => {
        show(next)
        setStep(next.plan ? 'check' : 'columns')
      },
      'Не удалось прочитать файл. Повторите.',
    )
  }

  const load = (f: File) => {
    if (f.size > maxFileBytes) {
      setError('Файл больше 20 МБ. Разбейте его на части и загрузите по очереди.')
      return
    }
    run(
      'Читаем файл...',
      async () => readCatalogFile(f.name, await f.arrayBuffer()),
      (sheets) => {
        if (sheets.length === 0) {
          setError('В файле нет данных: все листы пустые. Проверьте файл.')
          return
        }
        setFile({ name: f.name, sheets })
        start(mode, f.name, sheets)
      },
      'Файл не прочитался. Сохраните его как XLSX или CSV и загрузите снова.',
    )
  }

  const switchMode = (m: ImportMode) => {
    setMode(m)
    if (file) {
      start(m, file.name, file.sheets)
    }
  }

  const remap = (sheet: number, mapping?: Mapping) => {
    if (!imp) {
      return
    }
    setPending(mapping ?? null)
    run('Сверяем файл с каталогом...', () => mapCatalogImport(imp.id, sheet, mapping), show, 'Не удалось сохранить колонки. Повторите.')
  }

  const apply = () => {
    const p = imp?.plan
    if (!imp || !p || !choice) {
      return
    }
    const id = imp.id
    run(
      'Сохраняем в каталог...',
      () => applyCatalogImport(id, applyBody(p, choice)),
      (next) => {
        draftId.current = null
        setImp(next)
        setStep('done')
        refreshCatalog(qc)
      },
      'Не удалось сохранить. Повторите.',
      (err) => {
        // NOTE: the catalog moved since the preview; the preview is read again and the admin's picks stay.
        if (err instanceof ApiError && err.code === 'catalog_changed') {
          fetchCatalogImport(id)
            .then(setImp)
            .catch(() => undefined)
        }
      },
    )
  }

  const download = (work: () => Promise<void>) => {
    setError('')
    work().catch((err: unknown) => setError(errorText(err, 'Не удалось скачать файл. Повторите.')))
  }

  const close = () => {
    if (imp && imp.status !== 'draft') {
      refreshCatalog(qc)
    }
    onClose()
  }

  const plan = imp?.plan
  const summary = imp?.summary
  const applied = Boolean(imp && imp.status !== 'draft')
  const reachable = (s: Step): boolean => {
    switch (s) {
      case 'file':
      case 'columns':
        return !applied && (s === 'file' || imp !== null)
      case 'check':
        return !applied && Boolean(plan)
      case 'done':
        return applied
    }
  }
  const saved = summary ? [...summary.applied.created, ...summary.applied.updated] : []
  const opened = imp?.mode === 'robot' && saved.length === 1 ? saved[0] : null
  const current = steps.find((s) => s.id === step) ?? steps[0]
  const nothing = !plan || !choice || (applyCount(plan, choice) === 0 && !(choice.archive && plan.missing.length > 0))

  const primaryLabel = step === 'check' ? 'Применить' : step === 'done' ? (opened ? 'Открыть решение' : 'Готово') : 'Дальше'
  const primaryOff = step === 'file' ? !imp : step === 'columns' ? !plan || pending !== null : step === 'check' && nothing
  const onPrimary = () => {
    if (step === 'check') {
      apply()
    } else if (step === 'done') {
      if (opened) {
        onOpenRobot(opened.id)
      } else {
        close()
      }
    } else {
      setStep(step === 'file' && !plan ? 'columns' : 'check')
    }
  }
  const back: Step | null = step === 'columns' ? 'file' : step === 'check' ? 'columns' : null

  return (
    <dialog
      ref={ref}
      className="robot-overlay import-overlay"
      aria-label="Загрузка таблицы"
      onCancel={(e) => {
        e.preventDefault()
        close()
      }}
    >
      <div className="import-head">
        <nav className="import-steps" aria-label="Шаги загрузки">
          <ol>
            {steps.map((s, i) => (
              <li key={s.id}>
                <button
                  type="button"
                  className="import-step"
                  aria-current={s.id === step ? 'step' : undefined}
                  disabled={s.id !== step && (!reachable(s.id) || busy !== '')}
                  onClick={() => setStep(s.id)}
                >
                  <span className="import-step-n">{i + 1}</span>
                  {s.label}
                </button>
              </li>
            ))}
          </ol>
        </nav>
        {file && step !== 'file' ? (
          <p className="import-file-name">{`${file.name}, ${imp?.mode === 'robot' ? 'один робот' : 'каталог целиком'}`}</p>
        ) : null}
        <button type="button" className="icon-button robot-overlay-close" aria-label="Закрыть" title="Закрыть" onClick={close}>
          <CloseIcon size={18} />
        </button>
      </div>
      <div className="import-body">
        <section className="import-main" aria-labelledby={`${uid}-step`}>
          <h2 id={`${uid}-step`} className="sr-only">
            {current.label}
          </h2>
          {busy ? (
            <p className="import-busy" role="status">
              {busy}
            </p>
          ) : null}
          {error ? (
            <p className="error" role="alert">
              <Reflow>{error}</Reflow>
            </p>
          ) : null}
          {step === 'file' ? (
            <FileStep mode={mode} fileName={file?.name ?? null} busy={busy !== ''} onMode={switchMode} onFile={load} onDownload={download} />
          ) : null}
          {step === 'columns' && imp ? (
            <ColumnsStep imp={imp} mapping={pending ?? mappingOf(imp)} fields={fields} onSheet={(sheet) => remap(sheet)} onMapping={(m) => remap(imp.sheet, m)} />
          ) : null}
          {step === 'check' && imp && plan && choice ? (
            <CheckStep imp={imp} plan={plan} fields={fields} choice={choice} onChoice={setChoice} onMode={switchMode} onDownload={download} />
          ) : null}
          {step === 'done' && summary ? <DoneStep summary={summary} loading={imp?.status === 'photos'} /> : null}
        </section>
      </div>
      <div className="robot-overlay-foot">
        <p className="robot-keys">
          <span>
            <kbd>Esc</kbd> закрыть
          </span>
        </p>
        <div className="robot-actions">
          <button type="button" className="btn btn-primary" disabled={primaryOff || busy !== ''} onClick={onPrimary}>
            {primaryLabel}
          </button>
          {back ? (
            <button type="button" className="btn" disabled={busy !== ''} onClick={() => setStep(back)}>
              Назад
            </button>
          ) : null}
          {opened ? (
            <button type="button" className="btn" onClick={close}>
              Закрыть
            </button>
          ) : null}
        </div>
      </div>
    </dialog>
  )
}
