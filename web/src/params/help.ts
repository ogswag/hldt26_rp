import type { SchemaField } from '../api/client'
import { numberText } from '../ui/numberText'
import { unitOf } from './unit'

function sentence(text: string): string {
  const t = text.trim()
  return t === '' || /[.!?]$/.test(t) ? t : `${t}.`
}

// A complete server text takes precedence; older schemas still compose help from the label, note and range.
export function helpText(f: SchemaField): string {
  if (f.help !== undefined) {
    return f.help.trim()
  }
  const unit = unitOf(f) ? ` ${unitOf(f)}` : ''
  const range =
    f.min !== undefined && f.max !== undefined && f.min !== f.max
      ? `Допустимое значение: от ${numberText(f.min)} до ${numberText(f.max)}${unit}`
      : ''
  const name = f.short && f.short !== f.label ? f.label : ''
  // A list offers «Неизвестно» as a choice; any other field is left empty for it.
  const unknown = f.allow_unknown && f.type !== 'boolean' && f.type !== 'enum' ? 'Если значение неизвестно, оставьте поле пустым.' : ''
  return [sentence(name), sentence(f.note ?? ''), sentence(range), unknown].filter(Boolean).join(' ')
}
