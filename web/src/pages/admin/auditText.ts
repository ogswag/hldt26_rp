import type { AuditEvent, CatalogField, FieldChange } from '../../api/client'
import { changesText } from '../../catalog/changes'
import { numberText } from '../../ui/numberText'

export const auditSections: { value: string; label: string; actions: string[] }[] = [
  {
    value: 'solution',
    label: 'Каталог',
    actions: ['solution.create', 'solution.update', 'solution.archive', 'solution.restore', 'solution.duplicate', 'solution.image'],
  },
  { value: 'catalog_import', label: 'Загрузки каталога', actions: ['catalog.import'] },
  { value: 'norms', label: 'Нормативы', actions: ['norms.update'] },
  { value: 'invitation', label: 'Приглашения', actions: ['invite.create', 'invite.revoke', 'invite.accept'] },
  {
    value: 'project',
    label: 'Проекты',
    actions: ['member.add', 'member.role', 'member.remove', 'project.delete', 'project.restore', 'project.purge'],
  },
  { value: 'user', label: 'Пользователи', actions: ['auth.password_reset', 'auth.email_verified'] },
  { value: 'session', label: 'Сеансы', actions: ['auth.logout', 'auth.session_revoke'] },
]

export const actionLabels: Record<string, string> = {
  'solution.create': 'Решение добавлено',
  'solution.update': 'Решение изменено',
  'solution.archive': 'Решение в архиве',
  'solution.restore': 'Решение вернулось из архива',
  'solution.duplicate': 'Решение скопировано',
  'solution.image': 'Фото решения',
  'catalog.import': 'Таблица загружена',
  'norms.update': 'Нормативы изменены',
  'invite.create': 'Приглашение отправлено',
  'invite.revoke': 'Приглашение отозвано',
  'invite.accept': 'Приглашение принято',
  'member.add': 'Участник добавлен',
  'member.role': 'Роль участника изменена',
  'member.remove': 'Участник убран',
  'project.delete': 'Проект в корзине',
  'project.restore': 'Проект восстановлен',
  'project.purge': 'Проект стёрт',
  'auth.logout': 'Выход',
  'auth.session_revoke': 'Сеанс завершён',
  'auth.password_reset': 'Пароль сброшен',
  'auth.email_verified': 'Email подтверждён',
}

export function actionLabel(action: string): string {
  return actionLabels[action] ?? 'Другое действие'
}

const roleLabels: Record<string, string> = { editor: 'редактор', viewer: 'только просмотр', owner: 'владелец' }

function str(v: unknown): string {
  return typeof v === 'string' ? v : ''
}

function count(v: unknown): number {
  return typeof v === 'number' ? v : 0
}

function changesOf(v: unknown): FieldChange[] {
  return Array.isArray(v) ? (v as FieldChange[]) : []
}

// targetText names what the event is about, without an internal id.
export function targetText(e: AuditEvent): string {
  switch (e.target_type) {
    case 'solution':
      return e.target_name ?? (str(e.meta.name) || 'решение')
    case 'project':
      return e.target_name ?? 'удалённый проект'
    case 'catalog_import':
      return str(e.meta.file_name) || 'таблица'
    case 'invitation':
      return str(e.meta.email) || 'приглашение'
    default:
      return 'аккаунт'
  }
}

// detailLines say what changed: the fields of a catalog edit, the counts of an upload, a member's role.
export function detailLines(e: AuditEvent, fields: readonly CatalogField[]): string[] {
  const m = e.meta
  const out: string[] = []
  switch (e.action) {
    case 'solution.create':
    case 'solution.update':
      out.push(...changesText(fields, changesOf(m.changes)))
      break
    case 'solution.duplicate':
      out.push('Копия другого решения каталога.')
      break
    case 'solution.image':
      out.push(m.image_sha ? 'Фото загружено.' : 'Фото удалено.')
      break
    case 'catalog.import': {
      const parts = [
        `добавлено ${numberText(count(m.created))}`,
        `изменено ${numberText(count(m.updated))}`,
        `в архив ${numberText(count(m.archived))}`,
      ]
      if (count(m.photos) > 0) {
        parts.push(`ссылок на фото ${numberText(count(m.photos))}`)
      }
      out.push(`${m.mode === 'robot' ? 'Одно решение' : 'Каталог'}: ${parts.join(', ')}.`)
      break
    }
    case 'member.add':
    case 'member.role':
      if (roleLabels[str(m.role)]) {
        out.push(`Роль: ${roleLabels[str(m.role)]}.`)
      }
      break
    case 'project.delete':
      if (count(m.jobs_canceled) > 0) {
        out.push(`Остановлено задач: ${numberText(count(m.jobs_canceled))}.`)
      }
      break
  }
  if (m.import_id && e.action !== 'catalog.import') {
    out.push('По загрузке таблицы.')
  }
  return out
}
