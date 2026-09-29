import type { HTMLAttributes, ReactNode } from 'react'

import { cutTitle } from './cutTitle'
import { AbbreviationText } from './Abbreviation'
import { HelpButton } from './HelpButton'

type Props = Omit<HTMLAttributes<HTMLDivElement>, 'className'> & {
  // id is the field's own id, so the label points at it.
  id: string
  label: string
  // unit stays in the field's accessible name; the field shows it itself (UnitField).
  unit?: string
  // help is what the "?" says; the full label goes there when the row shows a short one.
  help?: string
  fullLabel?: string
  wide?: boolean
  children: ReactNode
}

// One spec-sheet row: a one-line label with its "?" after it, and the field.
// Rows go in a `.field-rows` grid inside a `.param-section`.
export function FieldRow({ id, label, unit, help, fullLabel, wide, children, ...rest }: Props) {
  return (
    <div {...rest} className={wide ? 'field-row is-wide' : 'field-row'}>
      <div className="field-row-head">
        <label
          htmlFor={id}
          className="field-row-label"
          title={fullLabel && fullLabel !== label ? fullLabel : undefined}
          onMouseEnter={fullLabel && fullLabel !== label ? undefined : cutTitle(label)}
        >
          <AbbreviationText text={label} />
          {unit ? <span className="sr-only"> ({unit})</span> : null}
        </label>
        {help ? (
          <div className="field-row-help">
            <HelpButton label={fullLabel ?? label} text={help} />
          </div>
        ) : null}
      </div>
      <div className="field-row-control">{children}</div>
    </div>
  )
}
