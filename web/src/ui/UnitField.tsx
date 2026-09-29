import type { MouseEvent, ReactNode } from 'react'

// A press on the unit or the padding lands in the input, as it would on a plain field.
function focusInput(e: MouseEvent<HTMLSpanElement>) {
  const input = e.currentTarget.querySelector('input')
  if (input && e.target !== input && !input.disabled) {
    e.preventDefault()
    input.focus()
  }
}

// UnitField shows the unit inside the field box after the value. The label keeps the unit for screen readers.
export function UnitField({ unit, className, children }: { unit: string; className?: string; children: ReactNode }) {
  return (
    <span className={className ? `unit-field ${className}` : 'unit-field'} onMouseDown={focusInput}>
      {children}
      <span className="unit-field-unit" aria-hidden="true">
        {unit}
      </span>
    </span>
  )
}
