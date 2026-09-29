import type { ParamValue, SchemaField } from '../api/client'
import { numberText } from '../ui/numberText'

// valueText writes a parameter value the way a person reads it: grouped digits, the label of a choice, Да or
// Нет. The unit is left to the caller.
export function valueText(field: SchemaField, value: ParamValue | undefined): string {
  if (value === undefined) {
    return 'не задано'
  }
  if (value === null) {
    return 'Неизвестно'
  }
  if (typeof value === 'boolean') {
    return value ? 'Да' : 'Нет'
  }
  if (typeof value === 'number') {
    return numberText(value)
  }
  const label = (v: string) => field.options?.find((o) => o.value === v)?.label ?? v
  if (typeof value === 'string') {
    return field.type === 'enum' ? label(value) : value
  }
  if (Array.isArray(value)) {
    return value.map(label).join(', ')
  }
  if ('start' in value) {
    return `${value.start} - ${value.end}`
  }
  return `${numberText(value.length)} x ${numberText(value.width)} x ${numberText(value.height)}`
}
