import type { SearchEntry } from './entries'

// normalize brings text and query to one spelling: lower case, ё as е, digit groups joined ("20 000" is 20000),
// a decimal comma as a point, punctuation as spaces.
export function normalize(s: string): string {
  return s
    .toLocaleLowerCase('ru-RU')
    .replaceAll('ё', 'е')
    .replace(/[  ]/g, ' ')
    .replace(/(\d)\s+(?=\d{3}(?!\d))/g, '$1')
    .replace(/(\d),(?=\d)/g, '$1.')
    .replace(/\.(?!\d)|(?<!\d)\./g, ' ')
    .replace(/[«»"'()[\],:;/!?№]/g, ' ')
}

export function words(s: string): string[] {
  return normalize(s)
    .split(/[\s-]+/)
    .filter(Boolean)
}

// withinOne tells whether a and b differ by at most one insertion, deletion, substitution or swap of neighbours.
function withinOne(a: string, b: string): boolean {
  if (Math.abs(a.length - b.length) > 1) {
    return false
  }
  const m = a.length
  const n = b.length
  const d: number[][] = Array.from({ length: m + 1 }, (_, i) => Array.from({ length: n + 1 }, (_, j) => (i === 0 ? j : j === 0 ? i : 0)))
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1
      d[i][j] = Math.min(d[i - 1][j] + 1, d[i][j - 1] + 1, d[i - 1][j - 1] + cost)
      if (i > 1 && j > 1 && a[i - 1] === b[j - 2] && a[i - 2] === b[j - 1]) {
        d[i][j] = Math.min(d[i][j], d[i - 2][j - 2] + 1)
      }
    }
  }
  return d[m][n] <= 1
}

const fuzzyFrom = 5

// wordHit is 1 when q starts a word, 0.6 when it starts one with a single typo (queries of five letters and up),
// 0 otherwise.
function wordHit(q: string, list: readonly string[]): number {
  if (list.some((w) => w.startsWith(q))) {
    return 1
  }
  if (q.length < fuzzyFrom || /\d/.test(q)) {
    return 0
  }
  for (const w of list) {
    for (const len of [q.length - 1, q.length, q.length + 1]) {
      if (len <= w.length && withinOne(q, w.slice(0, len))) {
        return 0.6
      }
    }
  }
  return 0
}

// weights rank where a word was found: the entry's own label counts most, its value least.
const weights = { label: 10, full: 7, path: 4, note: 2, value: 1.5 } as const

type Prepared = { entry: SearchEntry; fields: [keyof typeof weights, string[]][]; label: string }

export function prepare(entries: readonly SearchEntry[]): Prepared[] {
  return entries.map((entry) => ({
    entry,
    label: normalize(entry.label),
    fields: [
      ['label', words(entry.label)],
      ['full', words([entry.fullLabel ?? '', ...(entry.aliases ?? [])].join(' '))],
      ['path', words(entry.path.join(' '))],
      ['note', words(entry.note ?? '')],
      ['value', words(entry.value ?? '')],
    ],
  }))
}

// score is 0 when a word of the query is found nowhere; every word has to start a word of the entry.
export function score(p: Prepared, query: readonly string[]): number {
  let total = 0
  let inLabel = 0
  for (const q of query) {
    let best = 0
    for (const [field, list] of p.fields) {
      const s = wordHit(q, list) * weights[field]
      if (s > best) {
        best = s
      }
    }
    if (best === 0) {
      return 0
    }
    if (best >= weights.label) {
      inLabel++
    }
    total += best
  }
  if (inLabel === query.length) {
    total += 5
  }
  if (p.label.startsWith(query.join(' '))) {
    total += 3
  }
  return total
}

// rank returns the entries that match every word of the query, best first. here is the address the user is on:
// entries of that page get a small lead. Ties keep the index order.
export function rank(prepared: readonly Prepared[], query: string, here: string, limit: number): SearchEntry[] {
  const q = words(query)
  if (q.length === 0) {
    return []
  }
  const hits: { entry: SearchEntry; s: number }[] = []
  for (const p of prepared) {
    let s = score(p, q)
    if (s === 0) {
      continue
    }
    if (p.entry.to && p.entry.to === here) {
      s += 1
    }
    hits.push({ entry: p.entry, s })
  }
  hits.sort((a, b) => b.s - a.s || a.entry.order - b.entry.order)
  return hits.slice(0, limit).map((h) => h.entry)
}
