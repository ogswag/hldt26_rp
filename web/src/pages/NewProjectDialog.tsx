import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'

import { createProject, type ObjectType } from '../api/client'
import { objectTypeLabel } from '../econ/view'
import { defaultsFromSchema, fieldList } from '../guest/store'
import { objectSchema } from '../offline/reference'
import { Reflow } from '../ui/Reflow'
import { Select } from '../ui/Select'

const types: ObjectType[] = ['warehouse', 'airport', 'hospital']
const typeOptions = types.map((t) => ({ value: t, label: objectTypeLabel(t) }))

type Props = {
  open: boolean
  onClose: () => void
}

export function NewProjectDialog({ open, onClose }: Props) {
  const ref = useRef<HTMLDialogElement>(null)
  const uid = useId()
  const nav = useNavigate()
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [type, setType] = useState<ObjectType>('warehouse')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [nameError, setNameError] = useState('')
  const nameRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const d = ref.current
    if (!d) {
      return
    }
    if (open && !d.open) {
      setName('')
      setType('warehouse')
      setError('')
      setNameError('')
      d.showModal()
    } else if (!open && d.open) {
      d.close()
    }
  }, [open])

  async function submit(e: FormEvent) {
    e.preventDefault()
    const title = name.trim()
    if (title === '') {
      setNameError('Укажите название проекта.')
      nameRef.current?.focus()
      return
    }
    setBusy(true)
    setError('')
    try {
      const schema = await objectSchema(type)
      const params = defaultsFromSchema(fieldList(schema))
      const p = await createProject({ name: title, object_type: type, params })
      void qc.invalidateQueries({ queryKey: ['projects'] })
      nav(`/p/${p.id}/object`)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Не удалось создать проект. Повторите попытку.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <dialog
      ref={ref}
      className="dialog"
      aria-labelledby={`${uid}-title`}
      onCancel={(e) => {
        e.preventDefault()
        if (!busy) {
          onClose()
        }
      }}
    >
      <form onSubmit={(e) => void submit(e)}>
        <h2 id={`${uid}-title`}>Новый проект</h2>
        <div className="field">
          <label htmlFor={`${uid}-name`}>Название</label>
          <input
            ref={nameRef}
            id={`${uid}-name`}
            value={name}
            autoFocus
            aria-invalid={nameError ? true : undefined}
            aria-describedby={nameError ? `${uid}-name-error` : undefined}
            onChange={(e) => {
              setName(e.target.value)
              setNameError('')
            }}
          />
          {nameError ? (
            <p id={`${uid}-name-error`} className="error">
              {nameError}
            </p>
          ) : null}
        </div>
        <div className="field">
          <label htmlFor={`${uid}-type`}>Тип объекта</label>
          <Select id={`${uid}-type`} value={type} options={typeOptions} onChange={setType} />
        </div>
        {error ? (
          <p className="error">
            <Reflow>{error}</Reflow>
          </p>
        ) : null}
        <div className="dialog-actions">
          <button type="submit" className="btn btn-primary" disabled={busy}>
            {busy ? 'Создаём проект...' : 'Создать'}
          </button>
          <button type="button" className="btn" disabled={busy} onClick={onClose}>
            Отмена
          </button>
        </div>
      </form>
    </dialog>
  )
}
