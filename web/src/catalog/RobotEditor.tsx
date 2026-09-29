import { useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'

import {
  adminCreateSolution,
  adminDeleteSolutionImage,
  adminPatchSolution,
  adminPutSolutionImage,
  ApiError,
  type CatalogField,
  type Solution,
} from '../api/client'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { useFieldDraft } from '../ui/fieldDraft'
import { FieldRow } from '../ui/FieldRow'
import { numberText } from '../ui/numberText'
import { Reflow } from '../ui/Reflow'
import { TextField } from '../ui/TextField'
import { FieldInput } from './FieldInput'
import { RobotPicture } from './RobotPicture'
import { recordEdit } from './undo'
import { putSolution } from './useCatalog'
import { solutionValue } from './values'
import { searchLinks, searchQuery } from './webSearch'

// maxPhotoBytes matches the server's cap on a robot photo.
const maxPhotoBytes = 8 << 20

// NOTE: the same order as the read view, so a field stays where the admin saw it.
const sections: [string, CatalogField['group']][] = [
  ['Предложение', 'offer'],
  ['Характеристики', 'specs'],
  ['О решении', 'about'],
]

// The title and the description are edited in the centre column.
const centre = new Set(['name', 'description'])

function errorText(err: unknown, code: string): string {
  if (err instanceof ApiError) {
    return err.details.find((d) => d.field === code)?.message ?? err.message
  }
  return err instanceof Error ? err.message : 'Не удалось сохранить. Повторите.'
}

// rangeText is the "?" of a number field: what it accepts, in the units it is typed in.
function rangeText(f: CatalogField): string | undefined {
  if (f.kind !== 'number' || (f.min === undefined && f.max === undefined)) {
    return undefined
  }
  const scale = (v: number) => numberText(f.percent ? v * 100 : v)
  const unit = f.unit ? ` ${f.unit}` : ''
  const parts = [f.min !== undefined ? `от ${scale(f.min)}` : '', f.max !== undefined ? `до ${scale(f.max)}` : ''].filter(Boolean)
  return `${f.label}: ${parts.join(' ')}${unit}${f.integer ? ', целое число' : ''}. Пустое поле значит «нет данных».`
}

function DescriptionField({ id, value, maxLength, onSave }: { id: string; value: string; maxLength?: number; onSave: (v: string | null) => void }) {
  // NOTE: Enter starts a new line here, so the text saves after a pause and on leaving the field only.
  const d = useFieldDraft((t) => onSave(t.trim() === '' ? null : t.trim()))
  return <textarea id={id} rows={8} maxLength={maxLength} value={d.text ?? value} onChange={d.onChange} onBlur={d.onBlur} />
}

type Props = {
  // s is null for a new robot until its name is saved.
  s: Solution | null
  fields: readonly CatalogField[]
  titleId: string
  onCreated: (s: Solution) => void
}

// Admin robot overlay. Every field saves itself; a new robot is created
// once its name is saved, and ⌘Z undoes this tab's edits (catalog/undo).
export function RobotEditor({ s, fields, titleId, onCreated }: Props) {
  const qc = useQueryClient()
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [photoBusy, setPhotoBusy] = useState(false)
  const [photoError, setPhotoError] = useState('')
  const [dropPhoto, setDropPhoto] = useState(false)
  // NOTE: a name typed while the new robot is being created is saved once it exists.
  const creating = useRef(false)
  const laterName = useRef('')

  const setError = (code: string, text: string) => setErrors((e) => ({ ...e, [code]: text }))

  const save = (robot: Solution, code: string, value: unknown) => {
    const label = fields.find((f) => f.code === code)?.label ?? 'Поле'
    setError(code, '')
    adminPatchSolution(robot.id, { [code]: value })
      .then((out) => {
        putSolution(qc, out.solution)
        recordEdit(robot.id, `${label} (${out.solution.name})`, out.changes)
      })
      .catch((err: unknown) => setError(code, errorText(err, code)))
  }

  const saveName = (name: string) => {
    if (!name) {
      setError('name', 'Введите название: без него решение не сохранить.')
      return
    }
    if (s) {
      save(s, 'name', name)
      return
    }
    if (creating.current) {
      laterName.current = name
      return
    }
    creating.current = true
    setError('name', '')
    adminCreateSolution({ name })
      .then((out) => {
        putSolution(qc, out.solution)
        onCreated(out.solution)
        if (laterName.current && laterName.current !== out.solution.name) {
          save(out.solution, 'name', laterName.current)
        }
      })
      .catch((err: unknown) => setError('name', errorText(err, 'name')))
      .finally(() => {
        creating.current = false
        laterName.current = ''
      })
  }

  const putPhoto = (robot: Solution, file: File) => {
    if (file.size > maxPhotoBytes) {
      setPhotoError('Фото больше 8 МБ. Уменьшите его и загрузите снова.')
      return
    }
    setPhotoBusy(true)
    setPhotoError('')
    adminPutSolutionImage(robot.id, file)
      .then((out) => putSolution(qc, out))
      .catch((err: unknown) => setPhotoError(err instanceof Error ? err.message : 'Не удалось загрузить фото. Повторите.'))
      .finally(() => setPhotoBusy(false))
  }

  const deletePhoto = (robot: Solution) => {
    setPhotoBusy(true)
    setPhotoError('')
    adminDeleteSolutionImage(robot.id)
      .then((out) => {
        putSolution(qc, out)
        setDropPhoto(false)
      })
      .catch((err: unknown) => setPhotoError(err instanceof Error ? err.message : 'Не удалось удалить фото. Повторите.'))
      .finally(() => setPhotoBusy(false))
  }

  const fieldError = (code: string) =>
    errors[code] ? (
      <p className="error">
        <Reflow>{errors[code]}</Reflow>
      </p>
    ) : null

  const nameField = fields.find((f) => f.code === 'name')
  const descField = fields.find((f) => f.code === 'description')

  return (
    <>
      <section className="robot-main" aria-labelledby={titleId}>
        <h2 id={titleId} className="sr-only">
          {s?.name || 'Новое решение'}
        </h2>
        <label htmlFor={`${titleId}-name`} className="sr-only">
          Название
        </label>
        <TextField
          id={`${titleId}-name`}
          className="robot-title-input"
          value={s?.name ?? ''}
          maxLength={nameField?.max_len}
          onCommit={saveName}
        />
        {fieldError('name')}
        {!s && !errors.name ? (
          <p className="field-hint">
            <Reflow>Введите название: с него начинается новое решение, остальные поля откроются после сохранения.</Reflow>
          </p>
        ) : null}
        {s ? (
          <>
            <p className="robot-search">
              {searchLinks(s).map((l) => (
                <a key={l.label} className="btn" href={l.href} target="_blank" rel="noopener noreferrer" title={searchQuery(s)}>
                  {l.label}
                </a>
              ))}
            </p>
            <RobotPicture s={s} edit={{ busy: photoBusy, onPut: (f) => putPhoto(s, f), onDelete: () => setDropPhoto(true) }} />
            {photoError ? (
              <p className="error">
                <Reflow>{photoError}</Reflow>
              </p>
            ) : null}
            <label htmlFor={`${titleId}-description`} className="robot-description-label">
              Описание
            </label>
            <div className="is-quiet">
              <DescriptionField
                id={`${titleId}-description`}
                value={s.description ?? ''}
                maxLength={descField?.max_len}
                onSave={(v) => save(s, 'description', v)}
              />
            </div>
            {fieldError('description')}
          </>
        ) : null}
      </section>
      <section className="robot-side is-quiet" aria-label="Данные робота">
        <fieldset className="robot-fields" disabled={!s}>
          {sections.map(([title, group]) => (
            <div key={group} className="param-section">
              <h3>{title}</h3>
              <div className="field-rows">
                {fields
                  .filter((f) => f.group === group && f.editable && !centre.has(f.code))
                  .map((f) => {
                    const id = `${titleId}-${f.code}`
                    return (
                      <FieldRow key={f.code} id={id} label={f.label} unit={f.unit} help={rangeText(f)}>
                        {s ? (
                          <FieldInput key={s.id} field={f} value={solutionValue(s, f.code)} id={id} onSave={(v) => save(s, f.code, v)} />
                        ) : (
                          <input id={id} disabled />
                        )}
                        {fieldError(f.code)}
                      </FieldRow>
                    )
                  })}
              </div>
            </div>
          ))}
        </fieldset>
      </section>
      <ConfirmDialog
        open={dropPhoto}
        title="Удалить фото?"
        confirmLabel="Удалить"
        cancelLabel="Отмена"
        busy={photoBusy}
        error={photoError || undefined}
        onConfirm={() => s && deletePhoto(s)}
        onCancel={() => setDropPhoto(false)}
      >
        <p>
          <Reflow>{`Фото «${s?.name ?? ''}» пропадёт из каталога. Вернуть его можно, только загрузив файл снова.`}</Reflow>
        </p>
      </ConfirmDialog>
    </>
  )
}
