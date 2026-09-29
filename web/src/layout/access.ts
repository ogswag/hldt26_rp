import type { ObjectType, SchemaTab } from '../api/client'
import type { TabId } from './nav'

export type Access = { open: true } | { open: false; reason: string; link?: { label: string; tab: TabId; sub?: string } }

export type AccessInput = {
  demo: boolean
  objectType: ObjectType | null
  // loaded is false until the project, its schema and its store have arrived.
  loaded: boolean
  hasResult: boolean
  schemaTabs: readonly SchemaTab[]
  reviewed: readonly string[]
  errorTabs: readonly string[]
  hasFleet: boolean
  // suggestion is what the last calculation suggested: a robot with a count, a robot without a price, or nothing.
  suggestion: 'ready' | 'no_count' | 'none'
}

const open: Access = { open: true }

function names(list: readonly SchemaTab[]): string {
  return list.map((t) => t.label).join(', ')
}

function brokenAccess(p: AccessInput): Access | null {
  const broken = p.schemaTabs.filter((t) => p.errorTabs.includes(t.id))
  if (broken.length === 0) {
    return null
  }
  return {
    open: false,
    reason: `Исправьте ошибки в параметрах: ${names(broken)}. С ошибкой расчёт не выполнить.${p.hasResult ? ' Прошлые запуски открыты в Истории.' : ''}`,
    link: { label: `Открыть «${broken[0].label}»`, tab: 'object', sub: broken[0].id },
  }
}

function calcAccess(p: AccessInput): Access {
  if (p.demo) {
    return open
  }
  if (!p.loaded) {
    return { open: false, reason: 'Загружаем проект...' }
  }
  const broken = brokenAccess(p)
  if (broken) {
    // История stays: its runs do not depend on the current inputs. The other subtabs close (subtabAccess).
    return p.hasResult ? open : broken
  }
  if (p.hasResult) {
    return open
  }
  const missing = p.schemaTabs.filter((t) => t.key && !p.reviewed.includes(t.id))
  if (missing.length > 0) {
    return {
      open: false,
      reason: `Проверьте параметры объекта: ${names(missing)}. От них зависит расчёт.`,
      link: { label: `Открыть «${missing[0].label}»`, tab: 'object', sub: missing[0].id },
    }
  }
  return open
}

function simAccess(p: AccessInput): Access {
  if (p.objectType !== null && p.objectType !== 'warehouse') {
    return { open: false, reason: 'Симуляция пока доступна только для склада.' }
  }
  if (p.demo) {
    return open
  }
  if (!p.loaded) {
    return { open: false, reason: 'Загружаем проект...' }
  }
  if (p.hasFleet || p.suggestion === 'ready') {
    return open
  }
  if (!p.hasResult) {
    return {
      open: false,
      reason: 'Симуляции нужен робот. Его предложит расчёт, когда параметры объекта будут проверены.',
      link: { label: 'Открыть Расчёт', tab: 'calc' },
    }
  }
  if (p.suggestion === 'no_count') {
    return {
      open: false,
      reason: 'У предложенного робота нет цены в каталоге, поэтому расчёт не знает, сколько их нужно. Выберите робота на вкладке Роботы.',
      link: { label: 'Открыть Роботы', tab: 'robots' },
    }
  }
  return {
    open: false,
    reason: 'Ни один робот из каталога не подходит к объекту, симулировать нечего. Причины на вкладке Роботы.',
    link: { label: 'Открыть Роботы', tab: 'robots' },
  }
}

// subtabAccess decides whether a subtab of an open tab shows. A calculated project with errors keeps only История
// of Расчёт. Симуляция also needs a warehouse and a robot.
export function subtabAccess(p: AccessInput, tab: TabId, sub: string | null): Access {
  if (tab !== 'calc') {
    return open
  }
  if (sub === 'sim') {
    if (!p.demo && p.loaded) {
      const broken = brokenAccess(p)
      if (broken) {
        return broken
      }
    }
    return simAccess(p)
  }
  if (sub === 'history' || p.demo || !p.loaded) {
    return open
  }
  return brokenAccess(p) ?? open
}

// adminShown tells whether the row has Админ: for an admin only, and never in a demo, where everyone is a guest.
export function adminShown(role: string | undefined, demo: boolean): boolean {
  return role === 'admin' && !demo
}

// tabAccess decides which tabs are open. Админ, Объект and Роботы always are (the admin pages check the role
// themselves); Расчёт waits for the object tabs.
export function tabAccess(p: AccessInput): Record<TabId, Access> {
  return {
    admin: open,
    object: open,
    robots: open,
    calc: calcAccess(p),
  }
}
