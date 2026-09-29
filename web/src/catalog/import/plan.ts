import type { CatalogField, CatalogImport, FieldDiff, ImportApply, ImportColumn, ImportMode, ImportPlan, ImportRow } from '../../api/client'
import { numberText } from '../../ui/numberText'
import { usesCode } from '../fields'

export const modeLabels: Record<ImportMode, string> = {
  robot: 'Один робот',
  catalog: 'Каталог целиком: добавить и обновить',
}

export type Side = 'file' | 'catalog'

// Picks hold the side taken for each conflicting field, by row key and field code. The catalog side is the default.
export type Picks = Record<string, Record<string, Side>>

// Choice is what the admin picked on Проверка.
export type Choice = {
  rows: ReadonlySet<string>
  // fields are the field codes to take from the file.
  fields: readonly string[]
  picks: Picks
  archive: boolean
}

// NOTE: the upload reads these columns itself, to find the robot and the export; they are not fields to take.
const ownColumns = new Set(['id', 'stamp'])

// casesCode is the organizer's «Кейсы» column; its rows reach the preview as the robot's uses.
const casesCode = 'cases'

// Mapping holds the field code each column fills, by column index.
export type Mapping = Record<string, string>

export function mappingOf(imp: CatalogImport): Mapping {
  return Object.fromEntries(imp.columns.flatMap((c) => (c.field ? [[String(c.index), c.field]] : [])))
}

// columnName is the column's header, or its letter as a spreadsheet shows it when the header is empty.
export function columnName(c: Pick<ImportColumn, 'index' | 'header'>): string {
  if (c.header) {
    return c.header
  }
  let letters = ''
  for (let n = c.index + 1; n > 0; n = Math.floor((n - 1) / 26)) {
    letters = String.fromCharCode(65 + ((n - 1) % 26)) + letters
  }
  return `Колонка ${letters}`
}

// sampleText shows a file cell as people read it: a plain number in a number field gets grouped digits and a
// decimal comma.
export function sampleText(f: CatalogField | undefined, sample: string): string {
  return f?.kind === 'number' && /^-?\d+(\.\d+)?$/.test(sample) ? numberText(Number(sample)) : sample
}

// matchesCatalog tells whether a row of the file found its robot in the catalog, so that values are compared.
export function matchesCatalog(plan: ImportPlan): boolean {
  return plan.rows.some((r) => r.class === 'changed' || r.class === 'conflict' || r.class === 'unchanged')
}

// fieldCodes lists the fields the file's columns fill, in column order.
export function fieldCodes(imp: CatalogImport): string[] {
  return imp.columns.flatMap((c) => (c.field && !ownColumns.has(c.field) ? [c.field] : []))
}

// columnOf names the column a diff comes from: the organizer's uses come from «Кейсы».
export function columnOf(code: string): string {
  return code === usesCode ? casesCode : code
}

// acts reports whether applying the row would do anything: unchanged rows only bring a photo link.
export function acts(row: ImportRow): boolean {
  return row.class !== 'unchanged' || Boolean(row.photo)
}

// shownRows are the rows Проверка lists; unchanged rows without a photo are only counted.
export function shownRows(plan: ImportPlan): ImportRow[] {
  return plan.rows.filter(acts)
}

// defaultRows checks every row that does something, except errors and likely duplicates of catalog robots.
export function defaultRows(plan: ImportPlan): Set<string> {
  return new Set(plan.rows.filter((r) => acts(r) && r.class !== 'error' && !r.duplicate_of).map((r) => r.key))
}

// defaultChoice is the starting choice for a fresh preview: every field, the catalog side of conflicts, no archive.
export function defaultChoice(imp: CatalogImport): Choice {
  return { rows: imp.plan ? defaultRows(imp.plan) : new Set(), fields: fieldCodes(imp), picks: {}, archive: false }
}

// takenDiffs are the diffs of a row that the chosen fields cover. A new robot always takes its name.
export function takenDiffs(row: ImportRow, fields: readonly string[]): FieldDiff[] {
  return row.fields.filter((d) => fields.includes(columnOf(d.code)) || (row.class === 'new' && d.code === 'name'))
}

// pickOf is the side taken for one conflicting field.
export function pickOf(picks: Picks, row: string, code: string): Side {
  return picks[row]?.[code] ?? 'catalog'
}

// withPick sets the side of one conflicting field.
export function withPick(picks: Picks, row: string, code: string, side: Side): Picks {
  return { ...picks, [row]: { ...picks[row], [code]: side } }
}

// takesPhoto reports whether applying the row fetches its photo link.
export function takesPhoto(row: ImportRow, fields: readonly string[]): boolean {
  return Boolean(row.photo) && fields.includes('photo')
}

// outcome says in words what applying a row does with the chosen fields.
export function outcome(row: ImportRow, fields: readonly string[], label: (code: string) => string): string {
  const photo = takesPhoto(row, fields)
  switch (row.class) {
    case 'error':
      return ['Не загрузится.', ...(row.errors ?? []).map((e) => e.message)].join(' ')
    case 'unchanged':
      return photo ? 'Загрузить фото по ссылке.' : 'Без изменений.'
    case 'new': {
      const text = photo ? 'Добавить с фото по ссылке.' : 'Добавить.'
      return row.duplicate_of ? `${text} Похоже на «${row.duplicate_of.name}» из каталога.` : text
    }
  }
  const taken = takenDiffs(row, fields)
  const names = (conflict: boolean) =>
    taken
      .filter((d) => Boolean(d.conflict) === conflict)
      .map((d) => label(d.code))
      .join(', ')
  const parts = [names(false) && `Изменить: ${names(false)}.`, names(true) && `Конфликт: ${names(true)}.`, photo ? 'Загрузить фото по ссылке.' : '']
  return parts.filter(Boolean).join(' ') || 'Выбранные поля не меняются.'
}

// applyBody builds the apply request: the checked rows with the revisions the preview showed.
export function applyBody(plan: ImportPlan, c: Choice): ImportApply {
  const rows = plan.rows.filter((r) => c.rows.has(r.key) && r.class !== 'error')
  const picks: Picks = {}
  for (const r of rows) {
    const taken = Object.entries(c.picks[r.key] ?? {}).filter(([, side]) => side === 'file')
    if (taken.length > 0) {
      picks[r.key] = Object.fromEntries(taken)
    }
  }
  return {
    rows: rows.map((r) => ({ key: r.key, rev: r.rev ?? '' })),
    fields: [...c.fields],
    picks,
    archive: c.archive ? plan.missing.map((m) => m.id) : [],
  }
}

// applyCount is the number of checked rows that will be saved.
export function applyCount(plan: ImportPlan, c: Choice): number {
  return plan.rows.filter((r) => c.rows.has(r.key) && r.class !== 'error').length
}
