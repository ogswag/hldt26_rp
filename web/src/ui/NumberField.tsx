import type { InputHTMLAttributes } from 'react'

import { useFieldDraft } from './fieldDraft'
import { numberText, parseNumberText } from './numberText'

type Props = Omit<InputHTMLAttributes<HTMLInputElement>, 'type' | 'value' | 'defaultValue' | 'onChange' | 'onBlur' | 'onKeyDown'> & {
  value: number | null | undefined
  // valid says whether a typed number may be saved; a refused one is dropped when the field loses focus.
  valid?: (v: number) => boolean
  // saveOnPause false saves only on Enter and on leaving the field (useFieldDraft).
  saveOnPause?: boolean
} & (
    | { optional?: false; onCommit: (v: number) => void }
    // An optional field saves a cleared field as undefined.
    | { optional: true; onCommit: (v: number | undefined) => void }
  )

// NumberField is a number field that saves itself (useFieldDraft) and shows grouped digits (20 000). Spaces are
// optional when typing, and a comma or a point both work as the decimal sign.
export function NumberField({ value, valid, optional, onCommit, saveOnPause, ...rest }: Props) {
  const { text, ...field } = useFieldDraft((t) => {
    const n = parseNumberText(t)
    if (n === undefined) {
      if (optional) {
        onCommit(undefined)
      }
      return
    }
    if (Number.isFinite(n) && (!valid || valid(n))) {
      onCommit(n)
    }
  }, saveOnPause)
  const shown = text ?? (typeof value === 'number' && Number.isFinite(value) ? numberText(value) : '')
  return <input {...rest} type="text" inputMode="decimal" autoComplete="off" value={shown} {...field} />
}
