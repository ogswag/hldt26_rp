import { useState } from 'react'

import { Popover } from './Popover'
import { Reflow } from './Reflow'

// HelpButton keeps a field's full name, note and range behind a "?" after the field's label, so every field
// row is one line high.
export function HelpButton({ label, text }: { label: string; text: string }) {
  const [open, setOpen] = useState(false)
  if (!text) {
    return null
  }
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      label={`Пояснение: ${label}`}
      align="start"
      className="field-help"
      trigger={(t) => (
        <button type="button" className="field-help-button" aria-label={`Пояснение: ${label}`} title="Пояснение" {...t}>
          ?
        </button>
      )}
    >
      <p>
        <Reflow>{text}</Reflow>
      </p>
    </Popover>
  )
}
