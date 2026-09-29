import { describe, expect, it } from 'vitest'
import * as XLSX from 'xlsx'

import { decodeText, parseCsv, readCatalogFile, sniffDelimiter, trimRows } from './readFile'

function bytesOf(text: string): ArrayBuffer {
  return new TextEncoder().encode(text).buffer as ArrayBuffer
}

// cp1251 encodes Cyrillic and ASCII text the way Excel saves «CSV (разделители - запятые)» on a Russian Windows.
function cp1251(text: string): ArrayBuffer {
  const out = Array.from(text, (ch) => {
    const code = ch.charCodeAt(0)
    if (code < 0x80) {
      return code
    }
    if (ch === 'ё') {
      return 0xb8
    }
    if (ch === 'Ё') {
      return 0xa8
    }
    if (code >= 0x410 && code <= 0x44f) {
      return code - 0x410 + 0xc0
    }
    throw new Error(`no cp1251 byte for ${ch}`)
  })
  return new Uint8Array(out).buffer
}

function utf16le(text: string): ArrayBuffer {
  const out = new Uint8Array(2 + text.length * 2)
  out.set([0xff, 0xfe])
  for (let i = 0; i < text.length; i++) {
    out[2 + i * 2] = text.charCodeAt(i) & 0xff
    out[3 + i * 2] = text.charCodeAt(i) >> 8
  }
  return out.buffer
}

describe('readCatalogFile', () => {
  it('writes shares, dates and numbers of a workbook the way the server parses them', async () => {
    const sheet = XLSX.utils.aoa_to_sheet([
      ['Название', 'Сервис в год', 'Дата источника', 'Цена', 'Скорость'],
      ['Робот', 0.05, 46293, 1200000, 0.1 + 0.2],
    ])
    sheet.B2.z = '0%'
    sheet.C2.z = 'dd.mm.yyyy'
    sheet.D2.z = '#,##0 "₽"'
    const wb = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(wb, sheet, 'Каталог')
    XLSX.utils.book_append_sheet(wb, XLSX.utils.aoa_to_sheet([]), 'Пустой')
    const data = XLSX.write(wb, { type: 'array', bookType: 'xlsx' }) as ArrayBuffer

    const sheets = await readCatalogFile('katalog.xlsx', data)
    expect(sheets).toEqual([
      {
        name: 'Каталог',
        rows: [
          ['Название', 'Сервис в год', 'Дата источника', 'Цена', 'Скорость'],
          ['Робот', '5%', '2026-09-28', '1200000', '0.3'],
        ],
      },
    ])
  })

  it('takes the link of an empty cell', async () => {
    const sheet = XLSX.utils.aoa_to_sheet([['Название', 'Источник'], ['Робот', '']])
    sheet.B2 = { t: 's', v: '', l: { Target: 'https://example.com/robot' } }
    const wb = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(wb, sheet, 'Лист1')
    const data = XLSX.write(wb, { type: 'array', bookType: 'xlsx' }) as ArrayBuffer

    const [s] = await readCatalogFile('links.xlsx', data)
    expect(s.rows[1]).toEqual(['Робот', 'https://example.com/robot'])
  })

  it.each<[string, string, ArrayBuffer, string[][]]>([
    ['UTF-8 with semicolons', 'katalog.csv', bytesOf('Название;Цена\r\nРобот «Ёж»;950 000\r\n'), [['Название', 'Цена'], ['Робот «Ёж»', '950 000']]],
    ['UTF-8 with a byte order mark', 'katalog.csv', bytesOf('﻿Название;Компания\nЁж;ООО\n'), [['Название', 'Компания'], ['Ёж', 'ООО']]],
    [
      'windows-1251 with commas and a line break in quotes',
      'katalog.csv',
      cp1251('Название,Описание\r\nТележка,"Везёт груз,\r\nсама"\r\n'),
      [['Название', 'Описание'], ['Тележка', 'Везёт груз,\r\nсама']],
    ],
    ['UTF-16 with tabs, as Excel saves Unicode text', 'katalog.txt', utf16le('Название\tЦена\r\nЁж\t5\r\n'), [['Название', 'Цена'], ['Ёж', '5']]],
    ['text saved under an .xls name', 'katalog.xls', bytesOf('Название;Цена\nЁж;5\n'), [['Название', 'Цена'], ['Ёж', '5']]],
  ])('reads %s', async (_, name, data, rows) => {
    const sheets = await readCatalogFile(name, data)
    expect(sheets).toEqual([{ name: 'katalog', rows }])
  })

  it('gives no sheet for an empty file', async () => {
    expect(await readCatalogFile('pusto.csv', bytesOf('\r\n;;\r\n'))).toEqual([])
  })
})

describe('decodeText', () => {
  it('falls back to windows-1251 when the bytes are not UTF-8', () => {
    expect(decodeText(new Uint8Array(cp1251('Погрузчик')))).toBe('Погрузчик')
  })
})

describe('sniffDelimiter', () => {
  it.each<[string, string, string]>([
    ['semicolons', 'Название;Цена;Компания\nА;1,5;Б\nВ;2;Г\n', ';'],
    ['commas with decimal points', 'Название,Цена\nА,1.5\nВ,2\n', ','],
    ['tabs', 'Название\tЦена\nА\t1,5\n', '\t'],
    ['commas inside quotes', 'Название;Описание\nА;"раз, два, три"\nБ;"четыре, пять"\n', ';'],
    ['one column', 'Название\nА\nБ\n', ';'],
  ])('finds %s', (_, text, want) => {
    expect(sniffDelimiter(text)).toBe(want)
  })
})

describe('parseCsv', () => {
  it.each<[string, string, string[][]]>([
    ['doubled quotes', 'a;"он сказал ""да"""\n', [['a', 'он сказал "да"']]],
    ['a quote inside an unquoted cell', 'a;дюйм 5"\n', [['a', 'дюйм 5"']]],
    ['no line break at the end', 'a;b', [['a', 'b']]],
    ['empty cells', ';;\n', [['', '', '']]],
    ['bare line feeds and carriage returns', 'a\rb\nc', [['a'], ['b'], ['c']]],
  ])('keeps %s', (_, text, rows) => {
    expect(parseCsv(text, ';')).toEqual(rows)
  })
})

describe('trimRows', () => {
  it('drops empty cells at row ends and empty rows at the end', () => {
    expect(
      trimRows([
        ['a', 'b', ' '],
        ['', ''],
        ['c'],
        [''],
        [],
      ]),
    ).toEqual([['a', 'b'], [], ['c']])
  })

  it('stops one row past the limit, so the server still reports the sheet as too large', () => {
    const rows = Array.from({ length: 6000 }, (_, i) => [String(i)])
    expect(trimRows(rows)).toHaveLength(5002)
  })
})
