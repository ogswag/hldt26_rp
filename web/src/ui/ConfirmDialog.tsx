import { useEffect, useId, useRef, type ReactNode } from 'react'

import { Reflow } from './Reflow'

type Props = {
  open: boolean
  title: string
  children: ReactNode
  confirmLabel: string
  cancelLabel: string
  busy?: boolean
  error?: string
  onConfirm: () => void
  onCancel: () => void
}

// NOTE: the confirm action is destructive, so focus starts on cancel.
export function ConfirmDialog(p: Props) {
  const ref = useRef<HTMLDialogElement>(null)
  const cancelRef = useRef<HTMLButtonElement>(null)
  const titleId = useId()

  useEffect(() => {
    const d = ref.current
    if (!d) {
      return
    }
    if (p.open && !d.open) {
      d.showModal()
      cancelRef.current?.focus()
    } else if (!p.open && d.open) {
      d.close()
    }
  }, [p.open])

  return (
    <dialog
      ref={ref}
      className="dialog"
      aria-labelledby={titleId}
      onCancel={(e) => {
        e.preventDefault()
        if (!p.busy) {
          p.onCancel()
        }
      }}
    >
      <h2 id={titleId}>{p.title}</h2>
      <div className="dialog-body">{p.children}</div>
      {p.error ? (
        <p className="error">
          <Reflow>{p.error}</Reflow>
        </p>
      ) : null}
      <div className="dialog-actions">
        <button type="button" className="btn btn-danger" disabled={p.busy} onClick={p.onConfirm}>
          {p.confirmLabel}
        </button>
        <button ref={cancelRef} type="button" className="btn" disabled={p.busy} onClick={p.onCancel}>
          {p.cancelLabel}
        </button>
      </div>
    </dialog>
  )
}
