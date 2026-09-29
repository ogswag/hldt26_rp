import { describe, expect, it } from 'vitest'

import type { SchemaField } from '../api/client'
import {
  calcFingerprint,
  coerceParam,
  isUUID,
  normalizeClock,
  normalizeOverrides,
  overridesForRequest,
  sanitizeParams,
} from './store'

const fields: SchemaField[] = [
  {
    id: 'has_wms',
    label: 'WMS',
    unit: '-',
    type: 'boolean',
    required: true,
    allow_unknown: true,
    default: true,
  },
  {
    id: 'floor_type',
    label: 'Пол',
    unit: '-',
    type: 'enum',
    required: true,
    default: 'Промышленный бетон',
    options: [
      { value: 'Промышленный бетон', label: 'Промышленный бетон' },
      { value: 'Эпоксид', label: 'Эпоксид' },
    ],
  },
  {
    id: 'shift_window',
    label: 'Окно',
    unit: 'чч:мм',
    type: 'time_range',
    required: true,
    default: { start: '08:00', end: '22:00' },
  },
  {
    id: 'aisle_working_m',
    label: 'Проход',
    unit: 'м',
    type: 'number',
    required: true,
    default: 2.8,
    min: 1.5,
    max: 4.5,
  },
  {
    id: 'task_mix',
    label: 'Процессы',
    unit: '-',
    type: 'multi_enum',
    required: true,
    default: ['inbound', 'outbound'],
    options: [
      { value: 'inbound', label: 'Приёмка' },
      { value: 'outbound', label: 'Отгрузка' },
    ],
  },
]

describe('calcFingerprint', () => {
  it('ignores param key order and include id order', () => {
    const a = calcFingerprint(
      'warehouse',
      { b: 2, a: 1 },
      ['x', 'y'],
      { volume_factor: 1.2 },
    )
    const b = calcFingerprint(
      'warehouse',
      { a: 1, b: 2 },
      ['y', 'x'],
      { volume_factor: 1.2 },
    )
    expect(a).toBe(b)
  })

  it('changes when overrides change', () => {
    const a = calcFingerprint('warehouse', { n: 1 }, [], {})
    const b = calcFingerprint('warehouse', { n: 1 }, [], { labor_factor: 1.2 })
    expect(a).not.toBe(b)
  })
})

describe('overridesForRequest', () => {
  it('omits empty object', () => {
    expect(overridesForRequest({})).toBeUndefined()
    expect(overridesForRequest(normalizeOverrides(undefined))).toBeUndefined()
  })
})

describe('normalizeClock', () => {
  it('strips seconds and pads hours', () => {
    expect(normalizeClock('08:00:00')).toBe('08:00')
    expect(normalizeClock('8:00')).toBe('08:00')
    expect(normalizeClock('08:00:00.500')).toBe('08:00')
    expect(normalizeClock('25:00')).toBeUndefined()
  })
})

describe('sanitizeParams', () => {
  it('drops illegal types and fills defaults', () => {
    const out = sanitizeParams(fields, {
      has_wms: 'Да',
      floor_type: 'дерево',
      shift_window: { start: '08:00:00', end: '22:00:00' },
      aisle_working_m: 0.5,
      extra: 1,
    })
    expect(out.has_wms).toBe(true)
    expect(out.floor_type).toBe('Промышленный бетон')
    expect(out.shift_window).toEqual({ start: '08:00', end: '22:00' })
    expect(out.aisle_working_m).toBe(2.8)
    expect(out.task_mix).toEqual(['inbound', 'outbound'])
    expect(out).not.toHaveProperty('extra')
  })

  it('keeps valid values', () => {
    const out = sanitizeParams(fields, {
      has_wms: false,
      floor_type: 'Эпоксид',
      shift_window: { start: '07:00', end: '19:00' },
      aisle_working_m: 2,
      task_mix: ['inbound'],
    })
    expect(out.has_wms).toBe(false)
    expect(out.floor_type).toBe('Эпоксид')
    expect(out.shift_window).toEqual({ start: '07:00', end: '19:00' })
    expect(out.aisle_working_m).toBe(2)
    expect(out.task_mix).toEqual(['inbound'])
  })
})

describe('coerceParam', () => {
  it('rejects a string boolean', () => {
    expect(coerceParam(fields[0], 'Да')).toBeUndefined()
  })
})

describe('isUUID', () => {
  it('accepts catalog ids', () => {
    expect(isUUID('5760e938-9a43-45a7-b8e8-f4f2e6383930')).toBe(true)
    expect(isUUID('H1500')).toBe(false)
  })
})
