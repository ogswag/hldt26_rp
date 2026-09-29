// breaks decides which gaps between words must not become line breaks. ui/Reflow draws the result.

const NBSP = '\u00a0'

const SHORT = new Set(['в', 'на', 'и', 'к', 'с', 'о', 'у', 'не', 'от', 'до', 'по', 'из', 'за', 'а', 'но', 'со', 'ко', 'во', 'об'])
const UNITS = new Set([
  'шт', 'кг', 'г', 'т', 'м', 'мм', 'см', 'км', 'м²', 'м³', 'л', 'ч', 'мин', 'сут', 'мес', 'год', 'года', 'лет',
  'чел', 'ед', 'тыс', 'млн', 'млрд', '%', '₽',
])
const SCALES = new Set(['тыс', 'млн', 'млрд'])

// short: a preposition or conjunction and the next word; count: a word and its number (ошибок 0); chain: a word
// and a number that has a unit (Флот 13 шт); unit: a number and its unit (13 шт, 1,2 млн ₽).
export type Gap = 'short' | 'count' | 'chain' | 'unit'

// A Piece is laid out on one line: a word, words joined by no-break spaces, or a keep of smaller pieces.
export type Piece =
  | { kind: 'word'; text: string; slash: boolean }
  | { kind: 'join'; parts: Piece[] }
  | { kind: 'keep'; parts: Piece[] }

// NOTE: a keep too wide for the line gives way at its weakest gap first, a number and its unit last.
const STRENGTH: Record<Gap, number> = { short: 1, count: 2, chain: 2, unit: 3 }

function core(w: string): string {
  return w.replace(/^[^\p{L}\p{N}%₽/]+|[^\p{L}\p{N}%₽/]+$/gu, '')
}

function isNumber(w: string): boolean {
  return /^[-+−]?\d+(?:[,.]\d+)?$/.test(core(w).replace(/[\u00a0\u202f]/g, ''))
}

function isUnit(w: string): boolean {
  const c = core(w)
  return UNITS.has(c) || /^[\p{L}₽%]+\/[\p{L}()·.²³]+$/u.test(c)
}

function isWordEnd(w: string): boolean {
  return /^[(«]?\p{L}[\p{L}-]*$/u.test(w)
}

function isShort(w: string): boolean {
  return /^[(«]?\p{L}+$/u.test(w) && SHORT.has(core(w).toLowerCase())
}

// words splits text at plain spaces and puts back digit groups the API writes with a plain space (1 500).
export function words(text: string): string[] {
  const raw = text.split(/ +/).filter(Boolean)
  const out: string[] = []
  for (let i = 0; i < raw.length; i++) {
    let w = raw[i]
    while (i + 1 < raw.length && /\d$/.test(w) && /^\d{3}(?![\p{L}\p{N}])/u.test(raw[i + 1])) {
      w += NBSP + raw[++i]
    }
    out.push(w)
  }
  return out
}

// gaps names the gap after each word but the last, or null where a line may break.
export function gaps(ws: string[]): (Gap | null)[] {
  const amount = ws.map(isNumber)
  for (let i = 1; i < ws.length; i++) {
    if (amount[i - 1] && SCALES.has(core(ws[i]))) {
      amount[i] = true
    }
  }
  const unitAfter = ws.map((_, i) => i + 1 < ws.length && amount[i] && isUnit(ws[i + 1]))
  return ws.slice(0, -1).map((w, i) => {
    if (unitAfter[i]) {
      return 'unit'
    }
    if (isShort(w)) {
      return 'short'
    }
    if (isWordEnd(w) && amount[i + 1]) {
      return unitAfter[i + 1] ? 'chain' : 'count'
    }
    return null
  })
}

function word(w: string): Piece {
  return { kind: 'word', text: w, slash: w.includes('/') && isUnit(w) }
}

// NOTE: a short word before a plain word keeps a no-break space rather than a keep: the browser still
// hyphenates the second word, while a keep would leave the short word alone on its line.
function piece(ws: string[], gs: Gap[]): Piece {
  if (ws.length === 1) {
    return word(ws[0])
  }
  if (gs.every((g) => g === 'short')) {
    return { kind: 'join', parts: ws.map(word) }
  }
  const weakest = Math.min(...gs.map((g) => STRENGTH[g]))
  const groups = [{ ws: [ws[0]], gs: [] as Gap[] }]
  gs.forEach((g, i) => {
    if (STRENGTH[g] === weakest) {
      groups.push({ ws: [ws[i + 1]], gs: [] })
    } else {
      groups[groups.length - 1].ws.push(ws[i + 1])
      groups[groups.length - 1].gs.push(g)
    }
  })
  return { kind: 'keep', parts: groups.map((x) => piece(x.ws, x.gs)) }
}

// layout turns text into pieces separated by plain spaces, where the line may break.
export function layout(text: string): Piece[] {
  const ws = words(text)
  if (ws.length === 0) {
    return []
  }
  const gs = gaps(ws)
  const out: Piece[] = []
  let run = { ws: [ws[0]], gs: [] as Gap[] }
  gs.forEach((g, i) => {
    if (g) {
      run.ws.push(ws[i + 1])
      run.gs.push(g)
    } else {
      out.push(piece(run.ws, run.gs))
      run = { ws: [ws[i + 1]], gs: [] }
    }
  })
  out.push(piece(run.ws, run.gs))
  return out
}
