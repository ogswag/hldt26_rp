import type { InputHTMLAttributes } from 'react'

import { useFieldDraft } from './fieldDraft'

type Props = Omit<InputHTMLAttributes<HTMLInputElement>, 'value' | 'defaultValue' | 'onChange' | 'onBlur' | 'onKeyDown'> & {
  value: string
  onCommit: (v: string) => void
  // saveOnPause false saves only on Enter and on leaving the field (useFieldDraft).
  saveOnPause?: boolean
}

// TextField is a text field that saves itself (useFieldDraft). It saves the text trimmed.
export function TextField({ value, onCommit, saveOnPause, ...rest }: Props) {
  const { text, ...field } = useFieldDraft((t) => onCommit(t.trim()), saveOnPause)
  return <input autoComplete="off" {...rest} value={text ?? value} {...field} />
}
