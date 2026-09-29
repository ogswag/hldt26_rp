import { describe, expect, it } from 'vitest'

import type { SchemaField } from '../api/client'
import { checkParams } from './check'

const fields: SchemaField[] = [
  { id: 'area', label: 'Площадь', type: 'number', unit: 'м2', required: true, min: 100, max: 1000, default: 500 },
  { id: 'floors', label: 'Этажи', type: 'integer', unit: 'шт', required: true, min: 1, max: 3, default: 1 },
  { id: 'floor', label: 'Пол', type: 'enum', unit: '-', required: true, default: 'concrete', options: [{ value: 'concrete', label: 'Бетон' }] },
  { id: 'power', label: 'Мощность', type: 'number', unit: 'кВт', required: true, allow_unknown: true, min: 0, default: null },
] as unknown as SchemaField[]

describe('checkParams', () => {
  it('takes the values the schema takes', () => {
    expect(checkParams(fields, { area: 500, floors: 2, floor: 'concrete', power: null })).toEqual({})
  })

  it('names the range of a number out of it', () => {
    expect(checkParams(fields, { area: 5, floors: 1, floor: 'concrete', power: 0 })).toEqual({
      area: 'Укажите число от 100 до 1\u00a0000.',
    })
  })

  it('refuses a fraction where the schema wants a whole number', () => {
    expect(checkParams(fields, { area: 500, floors: 1.5, floor: 'concrete', power: 0 }).floors).toBe('Укажите целое число.')
  })

  it('refuses a value outside the list', () => {
    expect(checkParams(fields, { area: 500, floors: 1, floor: 'wood', power: 0 }).floor).toBe('Выберите значение из списка.')
  })

  it('asks for a required field nobody filled', () => {
    expect(checkParams(fields, { floors: 1, floor: 'concrete', power: 0 }).area).toBe('Заполните поле.')
  })

  it('lets a field marked unknown stay unknown', () => {
    expect(checkParams(fields, { area: 500, floors: 1, floor: 'concrete' })).toEqual({})
  })
})
