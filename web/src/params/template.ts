import type { ObjectType, ParamsMap, SchemaField } from '../api/client'
import { csvText, triggerDownload } from '../ui/download'

import { asDimensions, formatDimensions } from './dimensions'

export function buildTemplateRows(fields: SchemaField[], values: ParamsMap): (string | number)[][] {
  const rows: (string | number)[][] = [
    ['id', 'Параметр', 'Ед. изм.', 'Значение', 'min', 'max', 'Примечание'],
  ]
  for (const f of fields) {
    const current = values[f.id]
    const value = current === undefined ? f.default : current
    rows.push([
      f.id,
      f.label,
      f.unit,
      templateValue(value),
      f.min === undefined ? '' : f.min,
      f.max === undefined ? '' : f.max,
      f.note ?? '',
    ])
  }
  return rows
}

export async function downloadParamsTemplate(opts: {
  format: 'xlsx' | 'csv'
  objectType: ObjectType
  sheetName: string
  fields: SchemaField[]
  values: ParamsMap
}): Promise<void> {
  const rows = buildTemplateRows(opts.fields, opts.values)
  const base = `params-${opts.objectType}`
  if (opts.format === 'csv') {
    triggerDownload(`${base}.csv`, new Blob([csvText(rows)], { type: 'text/csv;charset=utf-8' }))
    return
  }
  const XLSX = await import('xlsx')
  const wb = XLSX.utils.book_new()
  const ws = XLSX.utils.aoa_to_sheet(rows)
  const sheet = opts.sheetName.slice(0, 31) || opts.objectType
  XLSX.utils.book_append_sheet(wb, ws, sheet)
  XLSX.writeFile(wb, `${base}.xlsx`)
}

function templateValue(value: unknown): string | number {
  if (value === null || value === undefined) {
    return 'Неизвестно'
  }
  if (typeof value === 'boolean') {
    return value ? 'Да' : 'Нет'
  }
  if (Array.isArray(value)) {
    return value.join(', ')
  }
  if (typeof value === 'object' && value && 'start' in value && 'end' in value) {
    const tr = value as { start: string; end: string }
    return `${tr.start}-${tr.end}`
  }
  const d = asDimensions(value)
  if (d) {
    return formatDimensions(d)
  }
  if (typeof value === 'number' || typeof value === 'string') {
    return value
  }
  return String(value)
}
