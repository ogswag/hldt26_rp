import { plural } from '../fleet/format'
import type { SyncErrorKind, SyncStatus } from '../store/sync'

function waiting(n: number): string {
  return n > 0 ? `, ${n} ${plural(n, 'правка ждёт', 'правки ждут', 'правок ждут')} отправки` : ''
}

const errors: Record<SyncErrorKind, string> = {
  network: 'Нет сети',
  server: 'Сервер не отвечает',
  auth: 'Войдите снова, чтобы правки ушли на сервер',
  too_large: 'Правка слишком большая. Разделите её на несколько',
  schema: 'Приложение обновилось. Обновите страницу',
  forbidden: 'У вас только просмотр этого проекта',
  gone: 'Проект удалён. Его можно вернуть из корзины',
}

// syncProblem says what is wrong with saving, or null while edits reach the server. "Сохранено" is never
// shown: saving is expected, only its failure is news.
export function syncProblem(s: SyncStatus, unsent: number): { text: string; gone: boolean } | null {
  if (s.phase === 'offline') {
    return { text: `Нет сети${waiting(unsent)}. Они уйдут, когда связь вернётся.`, gone: false }
  }
  if (s.phase !== 'error') {
    return null
  }
  const kind = s.error ?? 'server'
  const retried = kind === 'network' || kind === 'server' || kind === 'auth'
  return { text: `${errors[kind]}${retried ? waiting(unsent) : ''}.`, gone: kind === 'gone' }
}
