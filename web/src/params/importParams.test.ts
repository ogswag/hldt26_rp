import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import {
  importParamsFile,
  importRows,
  normalizeKey,
  parseBoolean,
  parseCsv,
  parseNumber,
  type ImportField,
} from './importParams'
import { buildTemplateRows } from './template'
import type { SchemaField } from '../api/client'
import { csvText } from '../ui/download'

const dir = dirname(fileURLToPath(import.meta.url))

type FixtureField = { id: string; label: string; unit: string; type: ImportField['type']; aliases?: string[] }
type FixtureFile = { warehouse: FixtureField[]; airport: FixtureField[]; hospital: FixtureField[] }

const schemaFields = JSON.parse(readFileSync(join(dir, 'testdata/schema_fields.json'), 'utf8')) as FixtureFile

function asImportFields(list: FixtureField[]): ImportField[] {
  return list.map((f) => ({ id: f.id, label: f.label, type: f.type, aliases: f.aliases }))
}

function readText(name: string): string {
  return readFileSync(join(dir, 'testdata', name), 'utf8')
}

function readBin(name: string): ArrayBuffer {
  const buf = readFileSync(join(dir, 'testdata', name))
  return buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength)
}

const mini: ImportField[] = [
  { id: 'area_total_m2', label: 'Общая площадь склада', type: 'number', min: 10000, max: 100000 },
  { id: 'floors', label: 'Количество этажей (мезонинов)', type: 'integer', min: 1, max: 3 },
  { id: 'floor_type', label: 'Тип напольного покрытия', type: 'string' },
]

describe('normalizeKey', () => {
  it('folds yo, superscript and times', () => {
    expect(normalizeKey('Объём приёмки (поддоны/сутки)')).toBe(normalizeKey('Объём приёмки (поддоны/сутки)'))
    expect(normalizeKey('Средние габариты паллеты (Д\u00d7Ш\u00d7В)')).toBe(
      normalizeKey('Средние габариты паллеты (ДxШxВ)'),
    )
    expect(normalizeKey('м\u00b2')).toBe('м2')
  })
})

describe('parseBoolean', () => {
  it('reads yes no unknown', () => {
    expect(parseBoolean('Да')).toBe(true)
    expect(parseBoolean('Нет')).toBe(false)
    expect(parseBoolean('yes')).toBe(true)
    expect(parseBoolean(1)).toBe(true)
    expect(parseBoolean('неизвестно')).toBe(null)
  })
})

describe('parseNumber', () => {
  it('accepts comma decimal and spaces', () => {
    expect(parseNumber('1,5')).toBe(1.5)
    expect(parseNumber('20 000')).toBe(20000)
    expect(parseNumber(3.5)).toBe(3.5)
  })
})

describe('importRows csv', () => {
  it('maps organizer warehouse csv onto all schema ids', () => {
    const fields = asImportFields(schemaFields.warehouse)
    const result = importRows(fields, parseCsv(readText('warehouse_organizer.csv')), null)
    expect(result.importedCount).toBe(fields.length)
    expect(result.values.area_total_m2).toBe(20000)
    expect(result.values.floor_type).toBe('Промышленный бетон')
    expect(result.values.aisle_working_m).toBe(2.8)
    expect(result.values.inbound_pallets_per_day).toBe(1000)
    expect(result.values.pallet_size_mm).toEqual({ length: 1200, width: 800, height: 1600 })
    expect(result.values.unit_size_mm).toEqual({ length: 300, width: 200, height: 150 })
    const errors = result.issues.filter((i) => i.level === 'error')
    expect(errors).toEqual([])
    const missing = result.issues.filter((i) => i.level === 'info' && i.message.includes('в файле нет'))
    expect(missing).toEqual([])
  })

  it('reports unknown and out of range cells and keeps the rest', () => {
    const fields: ImportField[] = [
      ...mini,
      { id: 'aisle_working_m', label: 'Ширина рабочих проходов между стеллажами', type: 'number', min: 1.5, max: 4.5 },
    ]
    const result = importRows(fields, parseCsv(readText('warehouse_partial.csv')), null)
    expect(result.values.area_total_m2).toBe(20000)
    expect(result.values.floor_type).toBe('Промышленный бетон')
    expect(result.values.floors).toBeUndefined()
    expect(result.values.aisle_working_m).toBeUndefined()
    expect(result.issues.some((i) => i.level === 'error' && i.message.includes('не входит в схему'))).toBe(true)
    expect(result.issues.some((i) => i.field === 'aisle_working_m' && i.message.includes('меньше минимума'))).toBe(
      true,
    )
    expect(result.issues.some((i) => i.field === 'floors' && i.level === 'info')).toBe(true)
  })

  it('maps template id column', () => {
    const csv = ['id;Параметр;Ед. изм.;Значение', 'area_total_m2;ignored;м2;15000', 'floors;x;шт;2'].join('\n')
    const result = importRows(mini, parseCsv(csv), null)
    expect(result.values.area_total_m2).toBe(15000)
    expect(result.values.floors).toBe(2)
  })

  it('roundtrips generated csv template', () => {
    const schemaMini: SchemaField[] = mini.map((f) => ({
      id: f.id,
      label: f.label,
      unit: '-',
      type: f.type,
      required: true,
      default: f.id === 'floor_type' ? 'бетон' : 1,
      min: f.min,
      max: f.max,
    }))
    const values = { area_total_m2: 20000, floors: 1, floor_type: 'Промышленный бетон' }
    const csv = csvText(buildTemplateRows(schemaMini, values))
    const result = importRows(mini, parseCsv(csv), null)
    expect(result.values).toEqual(values)
    expect(result.issues.filter((i) => i.level === 'error')).toEqual([])
  })
})

describe('importParamsFile xlsx', () => {
  it('reads organizer workbook sheets for all object types', async () => {
    const data = readBin('organizer_datasets.xlsx')
    for (const objectType of ['warehouse', 'airport', 'hospital'] as const) {
      const fields = asImportFields(schemaFields[objectType])
      const result = await importParamsFile({
        fields,
        objectType,
        filename: 'organizer_datasets.xlsx',
        data,
      })
      const errors = result.issues.filter((i) => i.level === 'error')
      expect(errors, objectType).toEqual([])
      expect(result.importedCount, objectType).toBe(fields.length)
    }
  })
})
