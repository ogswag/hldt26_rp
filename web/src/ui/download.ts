function csvCell(v: string | number): string {
  const s = String(v)
  if (/[;"\n]/.test(s)) {
    return `"${s.replace(/"/g, '""')}"`
  }
  return s
}

// csvText joins rows with `;` and starts the file with a byte order mark, so Excel reads it as UTF-8.
export function csvText(rows: (string | number)[][]): string {
  const lines = rows.map((r) => r.map(csvCell).join(';'))
  return `﻿${lines.join('\n')}\n`
}

// csvNumber writes a number for a spreadsheet with the Russian locale: a decimal comma, no digit groups.
export function csvNumber(v: number, digits = 2): string {
  if (!Number.isFinite(v)) {
    return ''
  }
  return String(Number(v.toFixed(digits))).replace('.', ',')
}

export function triggerDownload(filename: string, blob: Blob): void {
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = filename
  a.click()
  URL.revokeObjectURL(a.href)
}
