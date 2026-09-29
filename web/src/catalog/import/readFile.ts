import type { ImportSheet } from '../../api/client'

// NOTE: one row and one column over the server's sheet limits, so an oversized sheet still gets the server's message.
const maxRows = 5002
const maxCols = 81

const delimiters = [';', ',', '\t'] as const

// readCatalogFile reads every sheet of an XLSX, XLS or CSV file as rows of text cells, the way the server takes them.
export async function readCatalogFile(name: string, data: ArrayBuffer): Promise<ImportSheet[]> {
  const bytes = new Uint8Array(data)
  const sheets = isSpreadsheet(name, bytes) ? await readWorkbook(data) : [{ name: baseName(name), rows: readCsv(bytes) }]
  return sheets.map((s) => ({ name: s.name, rows: trimRows(s.rows) })).filter((s) => s.rows.length > 0)
}

// isSpreadsheet tells a workbook (ZIP for XLSX, OLE for XLS, XML or HTML saved as .xls) from delimited text.
function isSpreadsheet(name: string, bytes: Uint8Array): boolean {
  if (/\.(csv|tsv|txt)$/i.test(name)) {
    return false
  }
  const zip = bytes[0] === 0x50 && bytes[1] === 0x4b && bytes[2] === 0x03 && bytes[3] === 0x04
  const ole = bytes[0] === 0xd0 && bytes[1] === 0xcf && bytes[2] === 0x11 && bytes[3] === 0xe0
  const markup = decodeText(bytes.subarray(0, 512)).trimStart().startsWith('<')
  return zip || ole || markup
}

function baseName(name: string): string {
  return name.replace(/\.[^.]*$/, '') || name
}

function readCsv(bytes: Uint8Array): string[][] {
  const text = decodeText(bytes)
  return parseCsv(text, sniffDelimiter(text))
}

// decodeText reads UTF-16 by its byte order mark, then UTF-8, and windows-1251 when the bytes are not valid UTF-8.
export function decodeText(bytes: Uint8Array): string {
  if (bytes[0] === 0xff && bytes[1] === 0xfe) {
    return new TextDecoder('utf-16le').decode(bytes)
  }
  if (bytes[0] === 0xfe && bytes[1] === 0xff) {
    return new TextDecoder('utf-16be').decode(bytes)
  }
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes)
  } catch {
    return new TextDecoder('windows-1251').decode(bytes)
  }
}

// sniffDelimiter picks the separator that splits the first rows into the same number of columns, then the most
// columns. A file with one column keeps the semicolon the organizer's files use.
export function sniffDelimiter(text: string): string {
  const sample = text.slice(0, 64 * 1024)
  let best: string = delimiters[0]
  let bestScore = [0, 0]
  for (const d of delimiters) {
    const widths = parseCsv(sample, d)
      .filter((r) => r.some((c) => c.trim() !== ''))
      .slice(0, 20)
      .map((r) => r.length)
    const counts = new Map<number, number>()
    widths.forEach((w) => counts.set(w, (counts.get(w) ?? 0) + 1))
    let width = 0
    let rows = 0
    counts.forEach((n, w) => {
      if (w > 1 && (n > rows || (n === rows && w > width))) {
        width = w
        rows = n
      }
    })
    if (rows > bestScore[0] || (rows === bestScore[0] && width > bestScore[1])) {
      best = d
      bestScore = [rows, width]
    }
  }
  return best
}

// parseCsv splits delimited text into rows. A quote opens a quoted cell only at the cell's start, as spreadsheet
// programs write it; a doubled quote inside is one quote, and line breaks inside quotes stay in the cell.
export function parseCsv(text: string, delimiter: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let cell = ''
  let quoted = false
  let fresh = true
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    if (quoted) {
      if (c !== '"') {
        cell += c
      } else if (text[i + 1] === '"') {
        cell += '"'
        i++
      } else {
        quoted = false
      }
      continue
    }
    if (c === '"' && fresh) {
      quoted = true
      fresh = false
    } else if (c === delimiter) {
      row.push(cell)
      cell = ''
      fresh = true
    } else if (c === '\r' || c === '\n') {
      row.push(cell)
      rows.push(row)
      row = []
      cell = ''
      fresh = true
      if (c === '\r' && text[i + 1] === '\n') {
        i++
      }
    } else {
      cell += c
      fresh = false
    }
  }
  if (cell !== '' || row.length > 0 || quoted) {
    row.push(cell)
    rows.push(row)
  }
  return rows
}

// trimRows drops empty cells at the end of each row and empty rows at the end, and caps the size.
export function trimRows(rows: string[][]): string[][] {
  const out = rows.slice(0, maxRows).map((r) => {
    let end = Math.min(r.length, maxCols)
    while (end > 0 && r[end - 1].trim() === '') {
      end--
    }
    return r.slice(0, end)
  })
  while (out.length > 0 && out[out.length - 1].length === 0) {
    out.pop()
  }
  return out
}

type DateCode = { y: number; m: number; d: number }
type Formats = { is_date: (format: string) => boolean; parse_date_code: (serial: number) => DateCode | null }

async function readWorkbook(data: ArrayBuffer): Promise<ImportSheet[]> {
  const XLSX = await import('xlsx')
  const formats = XLSX.SSF as Formats
  const wb = XLSX.read(data, { type: 'array', dense: true, cellNF: true, cellText: false })
  return wb.SheetNames.map((name) => {
    const cells = wb.Sheets[name]?.['!data'] ?? []
    const rows: string[][] = []
    for (let r = 0; r < Math.min(cells.length, maxRows); r++) {
      const src = cells[r] ?? []
      const row: string[] = []
      for (let c = 0; c < Math.min(src.length, maxCols); c++) {
        const cell = src[c]
        row.push(cell ? cellText(cell, formats) : '')
      }
      rows.push(row)
    }
    return { name, rows }
  })
}

type Cell = { t: string; v?: unknown; z?: string | number; l?: { Target: string } }

// cellText writes a cell the way the server parses it: shares with a percent sign, dates as YYYY-MM-DD, other
// numbers without grouping. An empty cell with a link gives the link.
export function cellText(cell: Cell, formats: Formats): string {
  const format = typeof cell.z === 'string' ? cell.z : ''
  let text = ''
  if (cell.t === 'n' && typeof cell.v === 'number') {
    if (format.includes('%')) {
      text = `${round(cell.v * 100)}%`
    } else if (format && formats.is_date(format)) {
      const d = formats.parse_date_code(cell.v)
      text = d ? isoDate(d.y, d.m, d.d) : String(cell.v)
    } else {
      text = String(round(cell.v))
    }
  } else if (cell.t === 'd' && cell.v instanceof Date) {
    text = isoDate(cell.v.getFullYear(), cell.v.getMonth() + 1, cell.v.getDate())
  } else if (cell.t === 's' || cell.t === 'b') {
    text = String(cell.v ?? '')
  }
  if (text.trim() === '' && cell.l?.Target) {
    return cell.l.Target
  }
  return text
}

// round drops binary noise such as 0.30000000000000004 that a formula leaves in a cell.
function round(v: number): number {
  return Number(v.toPrecision(15))
}

function isoDate(y: number, m: number, d: number): string {
  return `${String(y).padStart(4, '0')}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
}
