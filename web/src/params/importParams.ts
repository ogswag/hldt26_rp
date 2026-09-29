import type { FieldType, ObjectType, ParamValue, TimeRange } from '../api/client'
import { parseDimensions } from './dimensions'

export const maxImportBytes = 2 * 1024 * 1024

export type ImportField = {
  id: string
  label: string
  type: FieldType
  min?: number
  max?: number
  allow_unknown?: boolean
  options?: { value: string; label: string }[]
  aliases?: string[]
}

export type ImportIssue = {
  level: 'error' | 'info'
  row?: number
  field?: string
  message: string
}

export type ImportResult = {
  values: Record<string, ParamValue>
  issues: ImportIssue[]
  sheet: string | null
  importedCount: number
  fieldCount: number
}

type RawCell = string | number
type ColRole = 'id' | 'name' | 'unit' | 'value'

export function normalizeKey(s: string): string {
  let t = s.trim()
  t = t.replace(/^[^\p{L}\p{N}]+/u, '')
  t = t.replace(/ё/gi, 'е')
  t = t.replace(/\u00b2/g, '2')
  t = t.replace(/\u00b3/g, '3')
  t = t.replace(/[\u00d7\u2715]/g, 'x')
  t = t.replace(/[\u2013\u2014\u2212]/g, '-')
  t = t.replace(/\u00b7/g, ' ')
  t = t.replace(/\u00b0/g, '')
  t = t.toLowerCase()
  t = t.replace(/[^a-z0-9а-я]+/gi, ' ')
  t = t.replace(/\s+/g, ' ').trim()
  return t
}

export function pickSheetName(names: string[], objectType: ObjectType): string {
  const aliases: Record<ObjectType, string[]> = {
    warehouse: ['склад', 'warehouse'],
    airport: ['аэропорт', 'airport'],
    hospital: ['медучреждение', 'медицинское учреждение', 'hospital'],
  }
  const want = aliases[objectType].map(normalizeKey)
  for (const n of names) {
    const k = normalizeKey(n)
    if (want.some((w) => k === w || k.includes(w))) {
      return n
    }
  }
  return names[0] ?? ''
}

export function parseCsv(text: string): RawCell[][] {
  const src = text.replace(/^\uFEFF/, '').replace(/\r\n/g, '\n').replace(/\r/g, '\n')
  if (src.trim() === '') {
    return []
  }
  const firstLine = src.split('\n', 1)[0] ?? ''
  const semi = (firstLine.match(/;/g) ?? []).length
  const comma = (firstLine.match(/,/g) ?? []).length
  const delim = semi >= comma ? ';' : ','
  const rows: RawCell[][] = []
  let row: string[] = []
  let cell = ''
  let i = 0
  let inQuotes = false
  while (i < src.length) {
    const ch = src[i]
    if (inQuotes) {
      if (ch === '"') {
        if (src[i + 1] === '"') {
          cell += '"'
          i += 2
          continue
        }
        inQuotes = false
        i += 1
        continue
      }
      cell += ch
      i += 1
      continue
    }
    if (ch === '"') {
      inQuotes = true
      i += 1
      continue
    }
    if (ch === delim) {
      row.push(cell)
      cell = ''
      i += 1
      continue
    }
    if (ch === '\n') {
      row.push(cell)
      rows.push(row)
      row = []
      cell = ''
      i += 1
      continue
    }
    cell += ch
    i += 1
  }
  row.push(cell)
  rows.push(row)
  return rows.filter((r) => r.some((c) => String(c).trim() !== ''))
}

export function importRows(fields: ImportField[], rows: RawCell[][], sheet: string | null): ImportResult {
  const issues: ImportIssue[] = []
  const values: Record<string, ParamValue> = {}
  const fieldCount = fields.length
  if (fields.length === 0) {
    return { values, issues, sheet, importedCount: 0, fieldCount }
  }
  if (rows.length === 0) {
    issues.push({
      level: 'error',
      message: 'Файл пустой. Скачайте шаблон, заполните колонку Значение и загрузите снова.',
    })
    addMissing(fields, values, issues)
    return { values, issues, sheet, importedCount: 0, fieldCount }
  }

  const byId = new Map<string, ImportField>()
  const byLabel = new Map<string, ImportField>()
  for (const f of fields) {
    byId.set(f.id, f)
    for (const a of f.aliases ?? []) {
      byLabel.set(normalizeKey(a), f)
    }
    byLabel.set(normalizeKey(f.label), f)
  }

  const header = findHeader(rows)
  const nameCol = header?.cols.name ?? 0
  const idCol = header?.cols.id
  const valueCol = header?.cols.value ?? (maxCols(rows) > 2 ? 2 : 1)
  const dataStart = header ? header.row + 1 : 0

  const seen = new Map<string, number>()
  let unknownWithValue = 0

  for (let i = dataStart; i < rows.length; i++) {
    const row = rows[i] ?? []
    const excelRow = i + 1
    const idRaw = idCol !== undefined ? cellStr(row[idCol]) : ''
    const nameRaw = cellStr(row[nameCol])
    const valueRaw = valueCol < row.length ? row[valueCol] : ''

    if (isSectionName(nameRaw) && idRaw === '') {
      continue
    }
    if (isEmpty(valueRaw)) {
      continue
    }

    const field = resolveField(byId, byLabel, idRaw, nameRaw)
    if (!field) {
      unknownWithValue += 1
      issues.push({
        level: 'error',
        row: excelRow,
        message: `Строка ${excelRow}: "${displayName(nameRaw, idRaw)}" не входит в схему. Значение не импортировано. Удалите строку или скачайте шаблон.`,
      })
      continue
    }

    const parsed = parseValue(field, valueRaw)
    if (!parsed.ok) {
      issues.push({
        level: 'error',
        row: excelRow,
        field: field.id,
        message: `Строка ${excelRow}: ${parsed.error} Значение не подставлено. Исправьте ячейку или введите значение в форме.`,
      })
      continue
    }

    const prevRow = seen.get(field.id)
    if (prevRow !== undefined) {
      issues.push({
        level: 'info',
        row: excelRow,
        field: field.id,
        message: `Поле ${field.label} (${field.id}) повторяется в строке ${excelRow}. Использовано последнее корректное значение.`,
      })
    }
    seen.set(field.id, excelRow)
    values[field.id] = parsed.value
  }

  addMissing(fields, values, issues)

  if (Object.keys(values).length === 0 && unknownWithValue >= 3) {
    issues.unshift({
      level: 'error',
      message:
        'Не удалось сопоставить строки со схемой этого типа объекта. Нужен лист Склад, Аэропорт или Медучреждение (по выбранному типу) либо скачанный шаблон.',
    })
  }

  return {
    values,
    issues,
    sheet,
    importedCount: Object.keys(values).length,
    fieldCount,
  }
}

export async function importParamsFile(opts: {
  fields: ImportField[]
  objectType: ObjectType
  filename: string
  data: ArrayBuffer
}): Promise<ImportResult> {
  if (opts.data.byteLength > maxImportBytes) {
    return {
      values: {},
      issues: [
        {
          level: 'error',
          message: `Файл больше ${maxImportBytes / (1024 * 1024)} МБ. Уменьшите файл или загрузите csv-шаблон.`,
        },
      ],
      sheet: null,
      importedCount: 0,
      fieldCount: opts.fields.length,
    }
  }
  const lower = opts.filename.toLowerCase()
  const asCsv = lower.endsWith('.csv') || (!looksLikeZip(opts.data) && !lower.endsWith('.xlsx') && !lower.endsWith('.xls'))
  if (asCsv && !lower.endsWith('.xlsx') && !lower.endsWith('.xls')) {
    const text = new TextDecoder('utf-8').decode(opts.data)
    return importRows(opts.fields, parseCsv(text), null)
  }
  try {
    const XLSX = await import('xlsx')
    const wb = XLSX.read(opts.data, { type: 'array' })
    if (!wb.SheetNames.length) {
      return importRows(opts.fields, [], null)
    }
    const sheet = pickSheetName(wb.SheetNames, opts.objectType)
    const ws = wb.Sheets[sheet]
    if (!ws) {
      return importRows(opts.fields, [], sheet)
    }
    const aoa = XLSX.utils.sheet_to_json<unknown[]>(ws, { header: 1, defval: '', raw: true })
    const rows: RawCell[][] = aoa.map((row) => (Array.isArray(row) ? row.map(toRawCell) : []))
    return importRows(opts.fields, rows, sheet)
  } catch {
    return {
      values: {},
      issues: [
        {
          level: 'error',
          message:
            'Не удалось прочитать Excel. Сохраните файл как xlsx или csv с колонками Параметр и Базовое значение, либо скачайте шаблон.',
        },
      ],
      sheet: null,
      importedCount: 0,
      fieldCount: opts.fields.length,
    }
  }
}

function looksLikeZip(data: ArrayBuffer): boolean {
  if (data.byteLength < 2) {
    return false
  }
  const u8 = new Uint8Array(data)
  return u8[0] === 0x50 && u8[1] === 0x4b
}

function toRawCell(v: unknown): RawCell {
  if (typeof v === 'number' && Number.isFinite(v)) {
    return v
  }
  if (typeof v === 'string') {
    return v
  }
  if (v === null || v === undefined) {
    return ''
  }
  if (typeof v === 'boolean') {
    return v ? 'true' : 'false'
  }
  return String(v)
}

function maxCols(rows: RawCell[][]): number {
  let n = 0
  for (const r of rows) {
    if (r.length > n) {
      n = r.length
    }
  }
  return n
}

function findHeader(rows: RawCell[][]): { row: number; cols: Partial<Record<ColRole, number>> } | null {
  const limit = Math.min(rows.length, 15)
  for (let i = 0; i < limit; i++) {
    const cols: Partial<Record<ColRole, number>> = {}
    const row = rows[i] ?? []
    for (let c = 0; c < row.length; c++) {
      const role = headerRole(normalizeKey(cellStr(row[c])))
      if (role && cols[role] === undefined) {
        cols[role] = c
      }
    }
    if (cols.value !== undefined && (cols.name !== undefined || cols.id !== undefined)) {
      return { row: i, cols }
    }
  }
  return null
}

function headerRole(norm: string): ColRole | null {
  if (norm === 'id' || norm === 'field' || norm === 'поле' || norm === 'код') {
    return 'id'
  }
  if (
    norm === 'параметр' ||
    norm === 'parameter' ||
    norm === 'param' ||
    norm === 'название' ||
    norm === 'name'
  ) {
    return 'name'
  }
  if (norm === 'ед изм' || norm === 'единица' || norm === 'unit' || norm === 'ед') {
    return 'unit'
  }
  if (norm.startsWith('ед ')) {
    return 'unit'
  }
  if (
    norm === 'значение' ||
    norm === 'базовое значение' ||
    norm === 'value' ||
    norm === 'base'
  ) {
    return 'value'
  }
  if (norm.startsWith('базовое')) {
    return 'value'
  }
  return null
}

function resolveField(
  byId: Map<string, ImportField>,
  byLabel: Map<string, ImportField>,
  idRaw: string,
  nameRaw: string,
): ImportField | undefined {
  const id = idRaw.trim()
  if (id && byId.has(id)) {
    return byId.get(id)
  }
  const nk = normalizeKey(nameRaw)
  if (nk && byLabel.has(nk)) {
    return byLabel.get(nk)
  }
  if (id && byLabel.has(normalizeKey(id))) {
    return byLabel.get(normalizeKey(id))
  }
  if (nameRaw.trim() && byId.has(nameRaw.trim())) {
    return byId.get(nameRaw.trim())
  }
  return undefined
}

function isSectionName(name: string): boolean {
  const n = name.trim()
  if (!n) {
    return true
  }
  const k = normalizeKey(n)
  if (k === '') {
    return true
  }
  if (k === 'параметр') {
    return true
  }
  if (k.startsWith('демо датасет')) {
    return true
  }
  return false
}

function isEmpty(v: RawCell | undefined): boolean {
  if (v === undefined) {
    return true
  }
  if (typeof v === 'number') {
    return false
  }
  return v.trim() === ''
}

function cellStr(v: RawCell | undefined): string {
  if (v === undefined) {
    return ''
  }
  return String(v)
}

function displayName(nameRaw: string, idRaw: string): string {
  const n = nameRaw.trim()
  if (n) {
    return n
  }
  const id = idRaw.trim()
  if (id) {
    return id
  }
  return 'без названия'
}

function parseValue(
  field: ImportField,
  raw: RawCell,
): { ok: true; value: ParamValue } | { ok: false; error: string } {
  if (field.allow_unknown && isUnknownCell(raw)) {
    return { ok: true, value: null }
  }
  if (field.type === 'boolean') {
    const b = parseBoolean(raw)
    if (b === null) {
      return { ok: false, error: `поле ${field.label} (${field.id}) должно быть Да, Нет или Неизвестно.` }
    }
    return { ok: true, value: b }
  }
  if (field.type === 'enum') {
    const s = String(raw).trim()
    if (s === '') {
      return { ok: false, error: `поле ${field.label} (${field.id}) пустое.` }
    }
    if (field.options && field.options.length > 0 && !field.options.some((o) => o.value === s || o.label === s)) {
      return { ok: false, error: `поле ${field.label} (${field.id}) должно быть одним из списка.` }
    }
    const opt = field.options?.find((o) => o.value === s || o.label === s)
    return { ok: true, value: opt ? opt.value : s }
  }
  if (field.type === 'multi_enum') {
    const parts = String(raw)
      .split(/[,;|]/)
      .map((p) => p.trim())
      .filter(Boolean)
    if (parts.length === 0) {
      return { ok: false, error: `поле ${field.label} (${field.id}) пустое.` }
    }
    const vals: string[] = []
    for (const p of parts) {
      const opt = field.options?.find((o) => o.value === p || o.label === p)
      if (field.options && field.options.length > 0 && !opt) {
        return { ok: false, error: `поле ${field.label} (${field.id}) содержит недопустимое значение.` }
      }
      vals.push(opt ? opt.value : p)
    }
    return { ok: true, value: vals }
  }
  if (field.type === 'time_range') {
    const tr = parseTimeRange(raw)
    if (!tr) {
      return { ok: false, error: `поле ${field.label} (${field.id}) должно быть интервалом ЧЧ:ММ-ЧЧ:ММ.` }
    }
    return { ok: true, value: tr }
  }
  if (field.type === 'dimensions') {
    const d = parseDimensions(String(raw))
    if (!d) {
      return { ok: false, error: `поле ${field.label} (${field.id}) должно быть в виде ДxШxВ, например 1200x800x1600.` }
    }
    const bad = [d.length, d.width, d.height].find(
      (n) => (field.min !== undefined && n < field.min) || (field.max !== undefined && n > field.max),
    )
    if (bad !== undefined) {
      return {
        ok: false,
        error: `размер ${fmtNum(bad)} в поле ${field.label} (${field.id}) вне диапазона. Укажите числа от ${fmtNum(field.min ?? 0)} до ${fmtNum(field.max ?? bad)}.`,
      }
    }
    return { ok: true, value: d }
  }
  if (field.type === 'string') {
    const s = String(raw).trim()
    if (s === '') {
      return { ok: false, error: `поле ${field.label} (${field.id}) пустое.` }
    }
    return { ok: true, value: s }
  }
  const n = parseNumber(raw)
  if (n === null) {
    return { ok: false, error: `поле ${field.label} (${field.id}) должно быть числом.` }
  }
  if (field.type === 'integer' && n !== Math.trunc(n)) {
    return { ok: false, error: `поле ${field.label} (${field.id}) должно быть целым числом.` }
  }
  if (field.min !== undefined && n < field.min) {
    return {
      ok: false,
      error: `значение ${field.label} (${field.id}) меньше минимума ${fmtNum(field.min)}. Укажите число от ${fmtNum(field.min)} до ${fmtNum(field.max ?? field.min)}.`,
    }
  }
  if (field.max !== undefined && n > field.max) {
    return {
      ok: false,
      error: `значение ${field.label} (${field.id}) больше максимума ${fmtNum(field.max)}. Укажите число от ${fmtNum(field.min ?? field.max)} до ${fmtNum(field.max)}.`,
    }
  }
  return { ok: true, value: n }
}

export function parseNumber(raw: RawCell): number | null {
  if (typeof raw === 'number') {
    return Number.isFinite(raw) ? raw : null
  }
  let s = raw.trim().replace(/\u00a0/g, '').replace(/\s+/g, '')
  if (s === '') {
    return null
  }
  if (/^-?\d{1,3}(\.\d{3})+,\d+$/.test(s)) {
    s = s.replace(/\./g, '').replace(',', '.')
  } else if (/^-?\d{1,3}(,\d{3})+\.\d+$/.test(s)) {
    s = s.replace(/,/g, '')
  } else if (/^-?\d+,\d+$/.test(s)) {
    s = s.replace(',', '.')
  }
  const n = Number(s)
  return Number.isFinite(n) ? n : null
}

function fmtNum(v: number): string {
  return String(v)
}

function isUnknownCell(raw: RawCell): boolean {
  const s = String(raw).trim().toLowerCase()
  return s === 'неизвестно' || s === 'unknown' || s === 'n/a' || s === 'na'
}

export function parseBoolean(raw: RawCell): boolean | null {
  if (typeof raw === 'number') {
    if (raw === 1) {
      return true
    }
    if (raw === 0) {
      return false
    }
    return null
  }
  const s = raw.trim().toLowerCase()
  if (s === 'да' || s.startsWith('да ') || s === 'yes' || s === 'true' || s === '1') {
    return true
  }
  if (s === 'нет' || s === 'no' || s === 'false' || s === '0') {
    return false
  }
  return null
}

function parseTimeRange(raw: RawCell): TimeRange | null {
  const s = String(raw).trim()
  const m = /^(\d{2}:\d{2})\s*(?:-|до)\s*(\d{2}:\d{2})$/.exec(s)
  if (!m || !m[1] || !m[2]) {
    return null
  }
  return { start: m[1], end: m[2] }
}

function addMissing(fields: ImportField[], values: Record<string, ParamValue>, issues: ImportIssue[]): void {
  for (const f of fields) {
    if (Object.prototype.hasOwnProperty.call(values, f.id)) {
      continue
    }
    issues.push({
      level: 'info',
      field: f.id,
      message: `Поле ${f.label} (${f.id}) в файле нет. В форме осталось прежнее значение. Заполните вручную, если нужно другое.`,
    })
  }
}
