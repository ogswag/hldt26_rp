const formats = new Map<number, Intl.NumberFormat>()

// numberText writes a number the Russian way, rounded to at most digits decimals: digits in groups of three,
// from 1 000 on, and a decimal comma (20 000, 3,5).
export function numberText(v: number, digits = 6): string {
  let f = formats.get(digits)
  if (!f) {
    f = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: digits, useGrouping: true })
    formats.set(digits, f)
  }
  return f.format(v)
}

// parseNumberText reads what a person typed: spaces between groups and a comma or a point for decimals.
// A blank field is undefined; anything else that is not a number is NaN, so the check can say so.
export function parseNumberText(text: string): number | undefined {
  const t = text.replace(/[\s\u00a0\u202f]/g, '').replace(',', '.')
  if (t === '') {
    return undefined
  }
  return /^[-+]?(\d+\.?\d*|\.\d+)(e[-+]?\d+)?$/i.test(t) ? Number(t) : NaN
}
