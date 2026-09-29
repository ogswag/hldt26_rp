import { describe, expect, it } from 'vitest'

import type { SearchEntry } from './entries'
import { normalize, prepare, rank } from './match'

const entry = (id: string, over: Partial<SearchEntry>): SearchEntry => ({ id, kind: 'field', label: id, path: [], order: 0, ...over })

const index = prepare([
  entry('width', { label: 'Ширина', fullLabel: 'Ширина склада', path: ['Объект', 'Площадка'], value: '48 м', order: 1 }),
  entry('area', { label: 'Площадь склада', path: ['Объект', 'Площадка'], value: '20 000 м²', order: 2, to: '/p/1/object/site' }),
  entry('staff', { label: 'Персонал базы', path: ['Объект', 'Процессы', 'Приёмка'], value: '12', order: 3 }),
  entry('vat', { label: 'Ставка НДС', path: ['Расчёт', 'Допущения'], aliases: ['налог'], value: '20%', order: 4 }),
  entry('shift', { label: 'Смены в сутки', path: ['Объект', 'Персонал'], note: 'Сколько смен работает склад', order: 5 }),
  entry('price', { label: 'Цена изделия', path: ['Расчёт', 'Допущения', 'Что если'], value: '1 250 000 ₽', order: 6 }),
])

describe('normalize', () => {
  const cases: [string, string][] = [
    ['Приёмка', 'приемка'],
    ['20 000 м²', '20000 м²'],
    ['1 250 000 ₽', '1250000 ₽'],
    ['2,5%', '2.5%'],
    ['«Склад»', ' склад '],
    ['Ширина.', 'ширина '],
  ]
  for (const [input, want] of cases) {
    it(input, () => expect(normalize(input)).toBe(want))
  }
})

describe('rank', () => {
  const cases: { name: string; query: string; want: string[]; here?: string }[] = [
    { name: 'word prefixes across label and path', query: 'шир скл', want: ['width'] },
    { name: 'ё and е are one letter', query: 'приемка', want: ['staff'] },
    { name: 'label beats path', query: 'склад', want: ['area', 'width', 'shift'] },
    { name: 'one typo in a long word, label before path', query: 'пресонал', want: ['staff', 'shift'] },
    { name: 'short words need an exact prefix', query: 'нбс', want: [] },
    { name: 'alias', query: 'налог', want: ['vat'] },
    { name: 'value with grouped digits', query: '20000', want: ['area'] },
    { name: 'value typed with a space', query: '1 250 000', want: ['price'] },
    { name: 'note is searched', query: 'сколько смен', want: ['shift'] },
    { name: 'every word must match', query: 'ширина ндс', want: [] },
    { name: 'empty query finds nothing', query: '  ', want: [] },
  ]
  for (const c of cases) {
    it(c.name, () => {
      expect(rank(index, c.query, c.here ?? '', 12).map((e) => e.id)).toEqual(c.want)
    })
  }

  it('gives entries of the open page a lead on a tie', () => {
    const tie = prepare([entry('a', { label: 'Площадь', order: 1 }), entry('b', { label: 'Площадь', order: 2, to: '/here' })])
    expect(rank(tie, 'площ', '/here', 12).map((e) => e.id)).toEqual(['b', 'a'])
    expect(rank(tie, 'площ', '/elsewhere', 12).map((e) => e.id)).toEqual(['a', 'b'])
  })
})
