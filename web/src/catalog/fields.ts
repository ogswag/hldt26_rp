import { useQuery } from '@tanstack/react-query'

import { fetchCatalogDictionaries, type CatalogField } from '../api/client'
import { formatRub, formatYears, objectTypeLabel } from '../econ/view'
import { numberText } from '../ui/numberText'
import { familyOf } from './families'

// usesCode is the organizer's list of uses (industry, scenario, cases, price) that its file carries per robot.
export const usesCode = 'uses'

// useCatalogFields loads the catalog field table the server keeps: labels, units, choices and ranges.
export function useCatalogFields(): CatalogField[] {
  const q = useQuery({ queryKey: ['catalog', 'dictionaries'], queryFn: fetchCatalogDictionaries, staleTime: Infinity })
  return q.data?.fields ?? []
}

export function fieldByCode(fields: readonly CatalogField[], code: string): CatalogField | undefined {
  return fields.find((f) => f.code === code)
}

// fieldLabel names a field for people; a code the table does not know stays as a neutral word.
export function fieldLabel(fields: readonly CatalogField[], code: string): string {
  if (code === usesCode) {
    return 'Применение'
  }
  return fieldByCode(fields, code)?.label ?? 'Поле'
}

function empty(v: unknown): boolean {
  return v === null || v === undefined || v === '' || (Array.isArray(v) && v.length === 0)
}

// choiceLabel names a choice; groups and object types take the names the app gives them everywhere else.
export function choiceLabel(f: CatalogField, code: unknown): string {
  if (f.code === 'family') {
    return familyOf(String(code)).label
  }
  if (f.code === 'object_types') {
    return objectTypeLabel(String(code))
  }
  return f.choices?.find((c) => c.code === code)?.label ?? String(code)
}

export function dateText(v: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(v)
  return m ? `${m[3]}.${m[2]}.${m[1]}` : v
}

// valueText prints a stored value the way the catalog shows it: «5%», «1 200 000 ₽», «Склад, Аэропорт».
export function valueText(f: CatalogField | undefined, v: unknown, blank = 'пусто'): string {
  if (empty(v)) {
    return blank
  }
  if (!f) {
    if (Array.isArray(v)) {
      return `записей: ${numberText(v.length)}`
    }
    return typeof v === 'number' ? numberText(v) : String(v)
  }
  switch (f.kind) {
    case 'number': {
      if (typeof v !== 'number') {
        return String(v)
      }
      if (f.percent) {
        return `${numberText(v * 100, 2)}%`
      }
      if (f.unit === '₽') {
        return formatRub(v)
      }
      if (f.code === 'lifetime_years') {
        return formatYears(v)
      }
      return f.unit ? `${numberText(v)} ${f.unit}` : numberText(v)
    }
    case 'choice':
      return choiceLabel(f, v)
    case 'choices':
      return Array.isArray(v) ? v.map((c) => choiceLabel(f, c)).join(', ') : choiceLabel(f, v)
    case 'date':
      return typeof v === 'string' ? dateText(v) : String(v)
    default:
      return String(v)
  }
}
