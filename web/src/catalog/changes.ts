import type { CatalogField, FieldChange } from '../api/client'
import { fieldByCode, fieldLabel, usesCode, valueText } from './fields'

const clipAt = 80

function clip(s: string): string {
  return s.length > clipAt ? `${s.slice(0, clipAt).trimEnd()}...` : s
}

function blank(v: unknown): boolean {
  return v === null || v === undefined || v === '' || (Array.isArray(v) && v.length === 0)
}

// changeText says what one field edit did: «Цена: было 1 200 000 ₽, стало 1 300 000 ₽».
export function changeText(fields: readonly CatalogField[], c: FieldChange): string {
  const label = fieldLabel(fields, c.field)
  if (c.field === usesCode) {
    return `${label}: обновлено из таблицы организатора`
  }
  const f = fieldByCode(fields, c.field)
  const before = clip(valueText(f, c.before))
  const after = clip(valueText(f, c.after))
  if (blank(c.before)) {
    return `${label}: ${after}`
  }
  if (blank(c.after)) {
    return `${label}: было ${before}, теперь пусто`
  }
  return `${label}: было ${before}, стало ${after}`
}

// changesText lists up to max edits and counts the rest.
export function changesText(fields: readonly CatalogField[], list: readonly FieldChange[], max = 4): string[] {
  const shown = list.slice(0, max).map((c) => changeText(fields, c))
  if (list.length > max) {
    shown.push(`и ещё ${list.length - max}`)
  }
  return shown
}
