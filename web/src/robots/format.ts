import type { MatchItem } from '../api/client'
import { formatYears } from '../econ/view'

// verdicts name the calculation's verdict on a robot and the tag that shows it.
export const verdicts: Record<string, { label: string; tag: string }> = {
  recommended: { label: 'подходит', tag: 'tag-good' },
  needs_review: { label: 'проверить', tag: 'tag-warning' },
  excluded: { label: 'не подходит', tag: 'tag-danger' },
}

// pickWarning says why a robot the matching did not recommend is risky to put into a variant; empty when it is fine.
export function pickWarning(item: MatchItem | null | undefined): string {
  if (!item || (item.status !== 'excluded' && item.status !== 'needs_review')) {
    return ''
  }
  const head = item.status === 'excluded' ? 'Подбор исключил робота.' : 'Подбор просит проверить робота.'
  const reasons = item.reasons.map((r) => r.trim()).filter(Boolean)
  return [head, ...reasons, 'Вариант посчитается с предупреждением.'].join(' ')
}

// estimatePayback prints the robot's own payback: «нет цены» when only the price is missing.
export function estimatePayback(item: MatchItem | null): string {
  const e = item?.estimate
  if (!e) {
    return item && item.status !== 'excluded' && item.price_rub === null ? 'нет цены' : ''
  }
  return e.payback_years === null ? 'не окупается' : paybackText(e.payback_years)
}

// paybackText prints a payback in years; a few weeks, which round to zero, read «меньше 0,1 года».
export function paybackText(years: number): string {
  return Number(years.toFixed(1)) === 0 ? 'меньше 0,1 года' : formatYears(years)
}
