#!/usr/bin/env node
// Builds docs/export/documentation.pdf and documentation.docx from docs/SUMMARY.md.
// Needs pandoc, zip, unzip and Google Chrome (or Chromium). Override the browser with CHROME=/path.
import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { basename, dirname, join, normalize, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const docs = join(root, 'docs')
const outDir = join(docs, 'export')
const title = 'Платформа подбора роботизированных решений'
const subtitle = 'Сопроводительная документация'

function run(cmd, args, opts = {}) {
  return execFileSync(cmd, args, { stdio: ['ignore', 'pipe', 'inherit'], ...opts })
}

function findChrome() {
  const candidates = [
    process.env.CHROME,
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/Applications/Google Chrome Dev.app/Contents/MacOS/Google Chrome Dev',
    '/Applications/Chromium.app/Contents/MacOS/Chromium',
    '/usr/bin/google-chrome',
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser',
  ]
  const found = candidates.find((c) => c && existsSync(c))
  if (!found) {
    throw new Error('Chrome not found: set CHROME=/path/to/chrome')
  }
  return found
}

// readSummary returns the parts of docs/SUMMARY.md in order: { part, items: [{ title, path }] }.
function readSummary() {
  const parts = []
  let current = { part: '', items: [] }
  parts.push(current)
  for (const raw of readFileSync(join(docs, 'SUMMARY.md'), 'utf8').split('\n')) {
    const line = raw.trim()
    const head = /^# (.+)$/.exec(line)
    if (head) {
      if (head[1] === 'Оглавление') {
        continue
      }
      current = { part: head[1], items: [] }
      parts.push(current)
      continue
    }
    const link = /^(?:- )?\[([^\]]+)\]\(([^)]+\.md)\)$/.exec(line)
    if (link) {
      current.items.push({ title: link[1], path: normalize(link[2]) })
    }
  }
  return parts.filter((p) => p.items.length > 0)
}

function chapterId(path) {
  return 'ch-' + path.replace(/\.md$/, '').replace(/[^a-z0-9]+/gi, '-').toLowerCase()
}

// tableWidths rewrites the separator row of each pipe table so the columns get widths in proportion to their text.
function tableWidths(lines) {
  const out = []
  for (let i = 0; i < lines.length; i++) {
    const sep = /^\|(\s*:?-{3,}:?\s*\|)+\s*$/.test(lines[i])
    if (!sep || i === 0 || !lines[i - 1].startsWith('|')) {
      out.push(lines[i])
      continue
    }
    const rows = [lines[i - 1]]
    for (let j = i + 1; j < lines.length && lines[j].startsWith('|'); j++) {
      rows.push(lines[j])
    }
    const cells = rows.map((r) => r.replace(/^\||\|\s*$/g, '').split(/(?<!\\)\|/).map((c) => c.trim()))
    const n = cells[0].length
    const weights = []
    for (let c = 0; c < n; c++) {
      const longest = Math.max(...cells.map((r) => (r[c] ?? '').length))
      weights.push(Math.max(8, Math.min(longest, 60)))
    }
    out.push('|' + weights.map((w) => '-'.repeat(w)).join('|') + '|')
  }
  return out
}

// chapterText prepares one file: headings go one level down, links point at chapters inside the document.
function chapterText(item, byPath, first) {
  const abs = join(docs, item.path)
  const fileDir = dirname(item.path)
  const lines = readFileSync(abs, 'utf8').split('\n')
  const out = []
  let fence = false
  let seenTitle = false
  for (const line of lines) {
    if (/^\s*```/.test(line)) {
      fence = !fence
      out.push(line)
      continue
    }
    if (fence) {
      out.push(line)
      continue
    }
    const h = /^(#{1,6})\s+(.*)$/.exec(line)
    if (h) {
      if (h[1].length === 1 && !seenTitle) {
        seenTitle = true
        const level = first ? '#' : '##'
        out.push(`${level} ${item.title} {#${chapterId(item.path)}}`)
      } else {
        out.push('#'.repeat(Math.min(h[1].length + 1, 6)) + ' ' + h[2])
      }
      continue
    }
    out.push(line)
  }
  let text = out.join('\n')
  text = text.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (whole, label, target) => {
    if (/^(https?:|mailto:|#)/.test(target)) {
      return whole
    }
    const [file] = target.split('#')
    if (file.startsWith('export/') || file.includes('/export/')) {
      return label
    }
    const resolved = normalize(join(fileDir, file))
    if (byPath.has(resolved)) {
      return `[${label}](#${chapterId(resolved)})`
    }
    return label
  })
  return tableWidths(text.split('\n')).join('\n')
}

function combined(parts) {
  const byPath = new Set(parts.flatMap((p) => p.items.map((i) => i.path)))
  const date = new Date().toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' })
  const body = [`---\ntitle: ${title}\nsubtitle: ${subtitle}\nlang: ru-RU\ntoc-title: Содержание\ndate: ${date}\n---\n`]
  let firstDone = false
  for (const { part, items } of parts) {
    if (part) {
      body.push(`# ${part}\n`)
    }
    for (const item of items) {
      const first = !part && !firstDone
      firstDone = true
      body.push(chapterText(item, byPath, first) + '\n')
    }
  }
  return body.join('\n')
}

// referenceDocx makes a pandoc reference.docx with plain fonts, table rules and a page break before each part.
function referenceDocx(work) {
  const ref = join(work, 'reference.docx')
  writeFileSync(ref, run('pandoc', ['--print-default-data-file', 'reference.docx']))
  const dir = join(work, 'ref')
  mkdirSync(dir)
  run('unzip', ['-q', ref, '-d', dir])
  const stylesPath = join(dir, 'word/styles.xml')
  let s = readFileSync(stylesPath, 'utf8')
  const font = '<w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial" w:eastAsia="Arial" />'
  s = s.replace(/<w:rFonts w:asciiTheme="minorHAnsi"[^>]*\/>/, font)
  s = s.replace(/<w:rFonts w:asciiTheme="majorHAnsi"[^>]*\/>/g, font)
  s = s.replace('<w:lang w:val="en-US"', '<w:lang w:val="ru-RU"')
  s = s.replace('<w:sz w:val="24" />\n        <w:szCs w:val="24" />', '<w:sz w:val="21" />\n        <w:szCs w:val="21" />')
  s = s.replace(/<w:color w:val="0F4761"[^>]*\/>/g, '<w:color w:val="111827" />')
  s = s.replace(
    /(w:styleId="Heading1">[\s\S]*?<w:pPr>)/,
    '$1\n      <w:pageBreakBefore />',
  )
  s = s.replace(
    /<w:tblPr>\s*<w:tblInd w:w="0" w:type="dxa" \/>/,
    '<w:tblPr>\n      <w:tblInd w:w="0" w:type="dxa" />\n      <w:tblBorders>\n        <w:top w:val="single" w:sz="4" w:color="D1D5DB" />\n        <w:bottom w:val="single" w:sz="4" w:color="D1D5DB" />\n        <w:insideH w:val="single" w:sz="4" w:color="D1D5DB" />\n      </w:tblBorders>',
  )
  s = s.replace(
    '<w:tblStylePr w:type="firstRow">\n      <w:tcPr>',
    '<w:tblStylePr w:type="firstRow">\n      <w:rPr><w:b /></w:rPr>\n      <w:tcPr>',
  )
  writeFileSync(stylesPath, s)
  rmSync(ref)
  run('zip', ['-q', '-r', ref, '.'], { cwd: dir })
  return ref
}

function main() {
  const work = mkdtempSync(join(tmpdir(), 'docs-build-'))
  try {
    mkdirSync(outDir, { recursive: true })
    const md = join(work, 'documentation.md')
    writeFileSync(md, combined(readSummary()))
    const reader = ['-f', 'commonmark_x-smart']

    const docx = join(outDir, 'documentation.docx')
    run('pandoc', [...reader, md, '-o', docx, '--toc', '--toc-depth=2', `--reference-doc=${referenceDocx(work)}`])
    console.log('wrote', relative(root, docx))

    const html = join(work, 'documentation.html')
    run('pandoc', [
      ...reader, md, '-o', html, '--standalone', '--embed-resources', '--toc', '--toc-depth=2',
      `--css=${join(root, 'scripts/docs/pdf.css')}`, '--metadata', `pagetitle=${title}`,
    ])
    const pdf = join(outDir, 'documentation.pdf')
    execFileSync(findChrome(), [
      '--headless=new', '--disable-gpu', '--no-sandbox', '--no-pdf-header-footer',
      '--generate-pdf-document-outline', `--print-to-pdf=${pdf}`, `file://${html}`,
    ], { stdio: 'ignore' })
    console.log('wrote', relative(root, pdf))
  } finally {
    rmSync(work, { recursive: true, force: true })
  }
}

main()
