// The parameter checks the form runs before it writes. The operation schema leaves the ranges of an object
// type to the server, which refuses a bad value after the fact; a form must say so while the field is still
// in front of the person, so it checks the same schema here.

import type { ParamsMap, SchemaField } from '../api/client'
import { coerceParam } from '../guest/store'
import { numberText } from '../ui/numberText'
import { asDimensions } from './dimensions'

function num(v: number): string {
  return numberText(v)
}

function message(f: SchemaField, v: unknown): string {
  if (v === undefined || v === null || v === '') {
    return 'Заполните поле.'
  }
  if (f.type === 'number' || f.type === 'integer') {
    if (typeof v !== 'number' || !Number.isFinite(v)) {
      return 'Укажите число.'
    }
    if (f.type === 'integer' && !Number.isInteger(v)) {
      return 'Укажите целое число.'
    }
    if (f.min !== undefined && f.max !== undefined) {
      return `Укажите число от ${num(f.min)} до ${num(f.max)}.`
    }
    if (f.min !== undefined) {
      return `Не меньше ${num(f.min)}.`
    }
    if (f.max !== undefined) {
      return `Не больше ${num(f.max)}.`
    }
    return 'Укажите число.'
  }
  if (f.type === 'enum' || f.type === 'multi_enum') {
    return 'Выберите значение из списка.'
  }
  if (f.type === 'dimensions') {
    const d = asDimensions(v)
    if (!d || ![d.length, d.width, d.height].every(Number.isFinite)) {
      return 'Укажите длину, ширину и высоту.'
    }
    if (f.min !== undefined && f.max !== undefined) {
      return `Каждый размер от ${num(f.min)} до ${num(f.max)} ${f.unit}.`
    }
    return 'Укажите положительные числа.'
  }
  if (f.type === 'time_range') {
    return 'Укажите время в формате ЧЧ:ММ.'
  }
  return 'Недопустимое значение.'
}

// checkParams returns a message for every field whose value the schema would not take. An empty result means
// the values can be written.
export function checkParams(fields: readonly SchemaField[], values: ParamsMap): Record<string, string> {
  const out: Record<string, string> = {}
  for (const f of fields) {
    const v = values[f.id]
    if (v === undefined) {
      if (f.required && !f.allow_unknown) {
        out[f.id] = 'Заполните поле.'
      }
      continue
    }
    if (coerceParam(f, v) === undefined) {
      out[f.id] = message(f, v)
    }
  }
  return out
}
