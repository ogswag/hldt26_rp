import type { ObjectType, SchemaTab } from '../api/client'
import { isObjectType } from '../guest/store'

export type TabId = 'admin' | 'object' | 'robots' | 'calc'

export type Subtab = { id: string; label: string }

export type Tab = { id: TabId; label: string }

export const tabs: Tab[] = [
  { id: 'admin', label: 'Админ' },
  { id: 'object', label: 'Объект' },
  { id: 'robots', label: 'Роботы' },
  { id: 'calc', label: 'Расчёт' },
]

export const calcSubtabs: Subtab[] = [
  { id: 'summary', label: 'Итог' },
  { id: 'items', label: 'Детализация' },
  { id: 'sim', label: 'Симуляция' },
  { id: 'assumptions', label: 'Допущения' },
  { id: 'method', label: 'Формулы и источники' },
  { id: 'export', label: 'Экспорт' },
  { id: 'history', label: 'История' },
]

export function calcSubtabsOf(demo: boolean): Subtab[] {
  return demo ? calcSubtabs.filter((tab) => tab.id !== 'history') : calcSubtabs
}

export const adminSubtabs: Subtab[] = [
  { id: 'catalog', label: 'Каталог' },
  { id: 'norms', label: 'Нормативы' },
  { id: 'invitations', label: 'Приглашения' },
  { id: 'audit', label: 'Журнал' },
]

// shownTabs lists the tabs of the row. Админ is there for an admin only; nobody else sees it, not even closed.
export function shownTabs(admin: boolean): Tab[] {
  return admin ? tabs : tabs.filter((t) => t.id !== 'admin')
}

export const processesSubtab: Subtab = { id: 'processes', label: 'Процессы' }
export const mapSubtab: Subtab = { id: 'map', label: 'Карта' }

// objectSubtabs lists the parameter tabs of the schema, then Процессы, then Карта where the map exists.
export function objectSubtabs(schemaTabs: readonly SchemaTab[], withMap: boolean): Subtab[] {
  const out: Subtab[] = schemaTabs.map((t) => ({ id: t.id, label: t.label }))
  out.push(processesSubtab)
  if (withMap) {
    out.push(mapSubtab)
  }
  return out
}

// A project lives under /p/<id>, a demo under /demo/<type>. Every step sits under that base.
export function projectBase(projectId: string | null | undefined, demo: ObjectType | null | undefined): string {
  if (projectId) {
    return `/p/${projectId}`
  }
  return demo ? `/demo/${demo}` : ''
}

export function tabHref(base: string, tab: TabId, sub?: string): string {
  return sub ? `${base}/${tab}/${sub}` : `${base}/${tab}`
}

export type ProjectLocation = {
  projectId: string | null
  demo: ObjectType | null
  tab: TabId | null
  sub: string | null
}

function isTab(v: string | undefined): v is TabId {
  return tabs.some((t) => t.id === v)
}

// parseAdminPath reads /admin and /admin/<sub>, the admin tab outside a project. Any other address gives null.
export function parseAdminPath(pathname: string): { sub: string | null } | null {
  const parts = pathname.split('/').filter(Boolean)
  if (parts[0] !== 'admin') {
    return null
  }
  return { sub: parts[1] ?? null }
}

export function parseProjectPath(pathname: string): ProjectLocation {
  const parts = pathname.split('/').filter(Boolean)
  const none: ProjectLocation = { projectId: null, demo: null, tab: null, sub: null }
  if (parts.length < 2) {
    return none
  }
  const [kind, key, tab, sub] = parts
  const step = { tab: isTab(tab) ? tab : null, sub: isTab(tab) ? (sub ?? null) : null }
  if (kind === 'p') {
    return { projectId: key, demo: null, ...step }
  }
  if (kind === 'demo' && isObjectType(key)) {
    return { projectId: null, demo: key, ...step }
  }
  return none
}
