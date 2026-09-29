import type { MouseEvent } from 'react'

// openRow opens the robot on a click anywhere in its row, except on the row's own controls and while the user
// selects text in it.
export function openRow(e: MouseEvent<HTMLElement>, open: () => void): void {
  const target = e.target instanceof Element ? e.target : null
  if (target?.closest('a, button, input, label, textarea, [role="menu"], [role="dialog"], [popover]')) {
    return
  }
  if (window.getSelection()?.toString()) {
    return
  }
  open()
}
