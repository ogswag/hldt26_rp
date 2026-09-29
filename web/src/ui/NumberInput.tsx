import { useState, type InputHTMLAttributes } from 'react'

import { numberText, parseNumberText } from './numberText'

type Props = Omit<InputHTMLAttributes<HTMLInputElement>, 'type' | 'value' | 'onChange'> & {
  value: number | null | undefined
  // onText gets what is typed; parseNumberText turns it into a number.
  onText: (text: string) => void
}

// NumberInput shows its number with grouped digits (20 000). Focus keeps that text, so a selection made on focus
// survives; spaces are optional when typing. Text that is not a number stays as typed, so the check next to it
// can point at it.
export function NumberInput({ value, onText, onFocus, onBlur, ...rest }: Props) {
  // text is what the field holds while it is being edited; bad is the last text that was not a number.
  const [text, setText] = useState<string | null>(null)
  const [bad, setBad] = useState('')
  const shown =
    text !== null
      ? text
      : typeof value === 'number' && Number.isFinite(value)
        ? numberText(value)
        : Number.isNaN(value)
          ? bad
          : ''
  return (
    <input
      {...rest}
      type="text"
      inputMode="decimal"
      autoComplete="off"
      value={shown}
      onFocus={(e) => {
        setText(e.currentTarget.value)
        onFocus?.(e)
      }}
      onChange={(e) => {
        setText(e.target.value)
        onText(e.target.value)
      }}
      onBlur={(e) => {
        const typed = text ?? ''
        setBad(Number.isNaN(parseNumberText(typed)) ? typed : '')
        setText(null)
        onBlur?.(e)
      }}
    />
  )
}
