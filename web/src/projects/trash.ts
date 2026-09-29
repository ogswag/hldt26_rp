// How long a deleted project is still kept, in the words the trash page shows.

import type { TrashItem } from '../api/client'

const day = new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long' })

export function daysLeft(purgeAt: string | null, now = Date.now()): number {
  if (!purgeAt) {
    return 0
  }
  return Math.max(0, Math.ceil((new Date(purgeAt).getTime() - now) / 86_400_000))
}

function dayWord(n: number): string {
  const ones = n % 10
  const tens = n % 100
  if (ones === 1 && tens !== 11) {
    return 'день'
  }
  if (ones >= 2 && ones <= 4 && (tens < 12 || tens > 14)) {
    return 'дня'
  }
  return 'дней'
}

export function keptText(item: TrashItem, now = Date.now()): string {
  const deleted = item.deleted_at ? `Удалён ${day.format(new Date(item.deleted_at))}` : 'Удалён'
  const left = daysLeft(item.purge_at, now)
  return left > 0 ? `${deleted} · будет стёрт через ${left} ${dayWord(left)}` : `${deleted} · будет стёрт со дня на день`
}
