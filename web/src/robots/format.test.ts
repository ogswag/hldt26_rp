import { describe, expect, it } from 'vitest'

import type { MatchItem } from '../api/client'

import { pickWarning } from './format'

const item = (status: string, reasons: string[]) => ({ status, reasons }) as MatchItem

describe('pickWarning', () => {
  it('names the reasons of an excluded robot', () => {
    expect(pickWarning(item('excluded', ['Ширина прохода меньше минимальной.']))).toBe(
      'Подбор исключил робота. Ширина прохода меньше минимальной. Вариант посчитается с предупреждением.',
    )
  })

  it('asks to check a robot that needs review', () => {
    expect(pickWarning(item('needs_review', ['Нет минимальной ширины проезда.', ' ']))).toBe(
      'Подбор просит проверить робота. Нет минимальной ширины проезда. Вариант посчитается с предупреждением.',
    )
  })

  it('stays silent for a recommended robot and for no robot', () => {
    expect(pickWarning(item('recommended', ['Подходит.']))).toBe('')
    expect(pickWarning(undefined)).toBe('')
    expect(pickWarning(null)).toBe('')
  })
})
