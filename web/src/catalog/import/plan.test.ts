import { describe, expect, it } from 'vitest'

import type { CatalogField, CatalogImport, ImportPlan, ImportRow } from '../../api/client'
import { numberText } from '../../ui/numberText'
import {
  applyBody,
  applyCount,
  columnName,
  defaultChoice,
  defaultRows,
  fieldCodes,
  mappingOf,
  matchesCatalog,
  outcome,
  sampleText,
  shownRows,
  takenDiffs,
  takesPhoto,
  withPick,
  type Choice,
} from './plan'

function row(key: string, cls: ImportRow['class'], extra: Partial<ImportRow> = {}): ImportRow {
  return { key, line: Number(key.slice(1)), class: cls, name: key, fields: [], ...extra }
}

const plan: ImportPlan = {
  three_way: true,
  rows: [
    row('L2', 'new', { fields: [{ code: 'name', file: 'Ёж' }, { code: 'price_rub', file: 5 }] }),
    row('L3', 'new', { duplicate_of: { id: 'a', name: 'Ёж' } }),
    row('L4', 'changed', { id: 'b', rev: 'r4', fields: [{ code: 'mass_kg', current: 1, file: 2, base: 1 }] }),
    row('L5', 'conflict', {
      id: 'c',
      rev: 'r5',
      fields: [
        { code: 'price_rub', base: 1, current: 2, file: 3, conflict: true },
        { code: 'uses', current: null, file: [{ industry: 'Логистика' }] },
      ],
    }),
    row('L6', 'unchanged', { id: 'd', rev: 'r6' }),
    row('L7', 'unchanged', { id: 'e', rev: 'r7', photo: 'https://example.com/e.jpg' }),
    row('L8', 'error', { errors: [{ field: 'price_rub', message: 'Цена: не число.' }] }),
  ],
  missing: [
    { id: 'm1', name: 'Старый' },
    { id: 'm2', name: 'Ещё старее' },
  ],
  counts: { new: 2, changed: 1, conflict: 1, unchanged: 2, error: 1 },
}

const imp: CatalogImport = {
  id: 'i',
  file_name: 'katalog.xlsx',
  mode: 'catalog',
  status: 'draft',
  sheets: ['Каталог'],
  sheet: 0,
  layout: 'table',
  stamp: null,
  columns: [
    { index: 0, header: 'Идентификатор', field: 'id', samples: [] },
    { index: 1, header: 'Название', field: 'name', samples: [] },
    { index: 2, header: 'Заметки', field: null, samples: [] },
    { index: 3, header: 'Цена', field: 'price_rub', samples: [] },
    { index: 4, header: 'Кейсы', field: 'cases', samples: [] },
    { index: 5, header: 'Фото', field: 'photo', samples: [] },
    { index: 6, header: 'Выгрузка', field: 'stamp', samples: [] },
  ],
  plan,
}

describe('import plan', () => {
  it('lists the fields the columns fill, without the id and the export stamp', () => {
    expect(fieldCodes(imp)).toEqual(['name', 'price_rub', 'cases', 'photo'])
    expect(mappingOf(imp)).toEqual({ '0': 'id', '1': 'name', '3': 'price_rub', '4': 'cases', '5': 'photo', '6': 'stamp' })
  })

  it.each<[number, string, string]>([
    [0, 'Цена', 'Цена'],
    [0, '', 'Колонка A'],
    [25, '', 'Колонка Z'],
    [26, '', 'Колонка AA'],
    [79, '', 'Колонка CB'],
  ])('names column %i with header «%s» as «%s»', (index, header, want) => {
    expect(columnName({ index, header })).toBe(want)
  })

  it('shows every row that does something', () => {
    expect(shownRows(plan).map((r) => r.key)).toEqual(['L2', 'L3', 'L4', 'L5', 'L7', 'L8'])
  })

  it('checks new, changed and conflict rows and photo links, but not errors or likely duplicates', () => {
    expect([...defaultRows(plan)]).toEqual(['L2', 'L4', 'L5', 'L7'])
    expect(defaultChoice(imp)).toEqual({ rows: new Set(['L2', 'L4', 'L5', 'L7']), fields: ['name', 'price_rub', 'cases', 'photo'], picks: {}, archive: false })
  })

  it.each<[string, string[], string[]]>([
    ['new rows take their name even without the name field', ['price_rub'], ['name', 'price_rub']],
    ['a new row takes only chosen fields', ['name'], ['name']],
  ])('%s', (_, fields, want) => {
    expect(takenDiffs(plan.rows[0], fields).map((d) => d.code)).toEqual(want)
  })

  it('takes the organizer uses with the «Кейсы» column', () => {
    expect(takenDiffs(plan.rows[3], ['cases']).map((d) => d.code)).toEqual(['uses'])
    expect(takenDiffs(plan.rows[3], ['price_rub']).map((d) => d.code)).toEqual(['price_rub'])
  })

  it('fetches a photo only when the photo field is chosen', () => {
    expect(takesPhoto(plan.rows[5], ['photo'])).toBe(true)
    expect(takesPhoto(plan.rows[5], ['name'])).toBe(false)
  })

  it('sends checked rows with their revisions, file picks only, and the missing robots to archive', () => {
    let picks = withPick({}, 'L5', 'price_rub', 'file')
    picks = withPick(picks, 'L4', 'mass_kg', 'catalog')
    picks = withPick(picks, 'L3', 'price_rub', 'file')
    const choice: Choice = { rows: new Set(['L2', 'L4', 'L5', 'L8']), fields: ['name', 'price_rub'], picks, archive: true }
    expect(applyBody(plan, choice)).toEqual({
      rows: [
        { key: 'L2', rev: '' },
        { key: 'L4', rev: 'r4' },
        { key: 'L5', rev: 'r5' },
      ],
      fields: ['name', 'price_rub'],
      picks: { L5: { price_rub: 'file' } },
      archive: ['m1', 'm2'],
    })
    expect(applyCount(plan, choice)).toBe(3)
  })

  it.each<[string, number, string[], string]>([
    ['a new robot', 0, ['name', 'photo'], 'Добавить.'],
    ['a likely duplicate', 1, ['name'], 'Добавить. Похоже на «Ёж» из каталога.'],
    ['a changed robot', 2, ['mass_kg'], 'Изменить: Масса.'],
    ['a change outside the chosen fields', 2, ['name'], 'Выбранные поля не меняются.'],
    ['a conflict and a change', 3, ['price_rub', 'cases'], 'Изменить: Применение. Конфликт: Цена.'],
    ['an unchanged robot with a photo link', 5, ['photo'], 'Загрузить фото по ссылке.'],
    ['an unchanged robot without the photo field', 5, ['name'], 'Без изменений.'],
    ['a row with errors', 6, ['name'], 'Не загрузится. Цена: не число.'],
  ])('describes %s', (_, index, fields, want) => {
    const labels: Record<string, string> = { price_rub: 'Цена', mass_kg: 'Масса', uses: 'Применение' }
    expect(outcome(plan.rows[index], fields, (code) => labels[code] ?? code)).toBe(want)
  })

  it('writes number cells of number fields the Russian way and leaves the rest as they are', () => {
    const price: CatalogField = { code: 'price_rub', label: 'Цена', kind: 'number', group: 'offer', editable: true }
    const name: CatalogField = { code: 'name', label: 'Название', kind: 'text', group: 'about', editable: true }
    expect(sampleText(price, '2750000')).toBe(numberText(2750000))
    expect(sampleText(price, '2.5')).toBe(numberText(2.5))
    expect(sampleText(price, 'дорого')).toBe('дорого')
    expect(sampleText(name, '2000')).toBe('2000')
    expect(sampleText(undefined, '2000')).toBe('2000')
  })

  it('compares values only when a row found its robot', () => {
    expect(matchesCatalog(plan)).toBe(true)
    expect(matchesCatalog({ ...plan, rows: [plan.rows[0], plan.rows[6]] })).toBe(false)
  })

  it('keeps missing robots unless asked', () => {
    expect(applyBody(plan, { ...defaultChoice(imp), archive: false }).archive).toEqual([])
  })
})
