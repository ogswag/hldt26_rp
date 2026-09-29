import { use, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

import { ActionBarSlot } from './actionBarSlot'
import { useReadOnly } from './readOnly'

type Props = {
  label: string
  // status is one line about the page on the left end of the bar.
  status?: ReactNode
  // children are the page's buttons, with the primary one at the right end.
  children?: ReactNode
}

// ActionBar is the strip along the bottom of the window with the page's actions. NOTE: it renders into a slot
// next to <main>, so containers and transforms inside a page never trap the fixed bar.
export function ActionBar({ label, status, children }: Props) {
  const slot = use(ActionBarSlot)
  const readOnly = useReadOnly()
  if (!slot || (readOnly && !status)) {
    return null
  }
  return createPortal(
    <div className="action-bar" role="group" aria-label={label}>
      <p className="action-bar-status" aria-live="polite">
        {status}
      </p>
      {readOnly ? null : <div className="action-bar-buttons">{children}</div>}
    </div>,
    slot,
  )
}
