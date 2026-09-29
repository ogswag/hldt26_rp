import { describe, expect, it } from 'vitest'

import type { SchemaField } from '../api/client'
import { coerceParam } from '../guest/store'
import { checkParams } from './check'
import { asDimensions, formatDimensions, parseDimensions } from './dimensions'
import { importRows } from './importParams'

const pallet: SchemaField = {
  id: 'pallet_size_mm',
  label: 'Средние габариты паллеты',
  unit: 'мм',
  type: 'dimensions',
  required: true,
  default: { length: 1200, width: 800, height: 1600 },
  min: 300,
  max: 3000,
}

describe('parseDimensions', () => {
  it.each([
    ['1200x800x1600', { length: 1200, width: 800, height: 1600 }],
    ['1200 X 800 X 1600', { length: 1200, width: 800, height: 1600 }],
    ['300\u00d7200\u00d7150', { length: 300, width: 200, height: 150 }],
    ['300х200х150,5', { length: 300, width: 200, height: 150.5 }],
    ['1200x800', null],
    ['1200x0x1600', null],
    ['1200x-800x1600', null],
    ['', null],
  ])('%s', (text, want) => {
    expect(parseDimensions(text)).toEqual(want)
  })

  it('writes the form a file template uses', () => {
    expect(formatDimensions({ length: 1200, width: 800, height: 1600 })).toBe('1200x800x1600')
  })
})

describe('asDimensions', () => {
  it('reads the object and the string projects kept before it', () => {
    expect(asDimensions({ length: 1, width: 2, height: 3 })).toEqual({ length: 1, width: 2, height: 3 })
    expect(asDimensions('1x2x3')).toEqual({ length: 1, width: 2, height: 3 })
    expect(asDimensions({ length: 1, width: 2 })).toBeNull()
    expect(asDimensions({ length: 1, width: 2, height: 3, depth: 4 })).toBeNull()
    expect(asDimensions(12)).toBeNull()
  })
})

describe('dimensions field', () => {
  it('coerces a legacy string to the object and refuses parts out of range', () => {
    expect(coerceParam(pallet, '1200x1000x1600')).toEqual({ length: 1200, width: 1000, height: 1600 })
    expect(coerceParam(pallet, { length: 1200, width: 100, height: 1600 })).toBeUndefined()
    expect(coerceParam(pallet, { length: 1200, width: NaN, height: 1600 })).toBeUndefined()
  })

  it('says which input is missing or out of range', () => {
    expect(checkParams([pallet], { pallet_size_mm: { length: 1200, width: NaN, height: 1600 } })).toEqual({
      pallet_size_mm: 'Укажите длину, ширину и высоту.',
    })
    expect(checkParams([pallet], { pallet_size_mm: { length: 1200, width: 800, height: 9000 } })).toEqual({
      pallet_size_mm: 'Каждый размер от 300 до 3\u00a0000 мм.',
    })
    expect(checkParams([pallet], { pallet_size_mm: { length: 1200, width: 800, height: 1600 } })).toEqual({})
  })

  it('imports a cell written as LxWxH', () => {
    const rows = [
      ['Средние габариты паллеты (Д\u00d7Ш\u00d7В)', 'мм', '1200\u00d71000\u00d71600'],
    ]
    const fields = [{ id: pallet.id, label: pallet.label, type: pallet.type, min: 300, max: 3000, aliases: ['Средние габариты паллеты (ДxШxВ)'] }]
    const result = importRows(fields, rows, null)
    expect(result.values.pallet_size_mm).toEqual({ length: 1200, width: 1000, height: 1600 })
    const bad = importRows(fields, [['Средние габариты паллеты', 'мм', '1200x80x1600']], null)
    expect(bad.values.pallet_size_mm).toBeUndefined()
    expect(bad.issues.some((i) => i.level === 'error' && i.message.includes('вне диапазона'))).toBe(true)
  })
})
