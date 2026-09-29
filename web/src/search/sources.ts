import type { CalculateResult, ObjectSchema, ObjectType, RunHistoryItem } from '../api/client'
import { formatRub, lineScenarioLabel, objectTypeLabel } from '../econ/view'
import { demoName } from '../guest/seed'
import type { Access } from '../layout/access'
import { adminSubtabs, calcSubtabsOf, objectSubtabs, shownTabs, tabHref, type Subtab, type TabId } from '../layout/nav'
import { featureLabel, labelOf, obstacleKinds, pointKinds, pointOption, zoneKinds } from '../map/document'
import { mapFromState } from '../map/records'
import { unitOf } from '../params/unit'
import { valueText } from '../params/valueText'
import { setFields, whatIfLabels, type SetFieldKey } from '../projects/assumptionFields'
import { assumptionSetsSelector, overridesOf, paramsOf, sharedCostsSelector } from '../projects/econRecords'
import { processFields } from '../projects/processFields'
import { processesSelector } from '../projects/processRecords'
import { formatDateTime, runBriefText, runKindLabel } from '../projects/runs'
import type { State } from '../store/apply'
import { redoKeys, undoKeys } from '../store/useUndoShortcuts'
import { numberText } from '../ui/numberText'
import type { ThemeChoice } from '../ui/theme'
import type { SearchEntry } from './entries'

export type Draft = Omit<SearchEntry, 'order'>

export type AccessOf = (tab: TabId, sub: string | null) => Access

const reason = (a: Access) => (a.open ? undefined : a.reason)
const withUnit = (text: string | undefined, unit: string | undefined) => (text && unit ? `${text} ${unit}` : text)
const percent = (share: number) => `${numberText(Math.round(share * 10000) / 100)}%`

function subtabsOf(tab: TabId, schema: ObjectSchema | null, withMap: boolean, demo: boolean): Subtab[] {
  switch (tab) {
    case 'admin':
      return adminSubtabs
    case 'object':
      return objectSubtabs(schema?.tabs ?? [], withMap)
    case 'calc':
      return calcSubtabsOf(demo)
    default:
      return []
  }
}

// navEntries lists the tabs and their subtabs; a closed one keeps its reason. Outside a project (base '') only
// Админ has a tab, and only for an admin.
export function navEntries(
  base: string,
  schema: ObjectSchema | null,
  withMap: boolean,
  demo: boolean,
  accessOf: AccessOf,
  admin: boolean,
): Draft[] {
  const out: Draft[] = []
  const list = shownTabs(admin).filter((t) => base !== '' || t.id === 'admin')
  for (const t of list) {
    const a = accessOf(t.id, null)
    out.push({ id: `page:${t.id}`, kind: 'tab', label: t.label, path: [], to: tabHref(base, t.id), closed: reason(a) })
    const subs = subtabsOf(t.id, schema, withMap, demo)
    for (const s of subs) {
      const sa = a.open ? accessOf(t.id, s.id) : a
      out.push({
        id: `page:${t.id}:${s.id}`,
        kind: 'tab',
        label: s.label,
        path: [t.label],
        to: tabHref(base, t.id, s.id),
        closed: reason(sa),
      })
    }
  }
  return out
}

// schemaEntries lists the parameter groups and every field with its current value.
export function schemaEntries(base: string, schema: ObjectSchema, state: State): Draft[] {
  const params = paramsOf(state)
  const out: Draft[] = []
  for (const g of schema.groups) {
    const tab = schema.tabs.find((t) => t.id === g.tab)?.label ?? g.tab
    const to = tabHref(base, 'object', g.tab)
    const own = g.label !== tab
    if (own) {
      out.push({ id: `section:${g.id}`, kind: 'section', label: g.label, path: ['Объект', tab], to })
    }
    for (const f of g.fields) {
      const v = f.id in params ? params[f.id] : f.default
      out.push({
        id: `field:${f.id}`,
        kind: 'field',
        label: f.short || f.label,
        fullLabel: f.short && f.short !== f.label ? f.label : undefined,
        note: f.note,
        aliases: f.aliases,
        value: withUnit(valueText(f, v), v === null ? undefined : unitOf(f) || undefined),
        path: own ? ['Объект', tab, g.label] : ['Объект', tab],
        to,
        section: `group:${g.id}`,
      })
    }
  }
  return out
}

// assumptionEntries lists the what-if and, in a saved project, the assumption sets and the shared costs.
export function assumptionEntries(base: string, state: State, result: CalculateResult | null, saved: boolean): Draft[] {
  const to = tabHref(base, 'calc', 'assumptions')
  const ov = overridesOf(state)
  const whatIf = ['Расчёт', 'Допущения', 'Что если']
  const catalog = result ? (result.scenarios.find((s) => s.kind === 'buy')?.solution_id ?? null) : null
  const price = ov.price_rub ?? (catalog ? result?.match.items.find((i) => i.solution_id === catalog)?.price_rub : undefined)
  const out: Draft[] = [
    { id: 'whatif:price', kind: 'field', label: whatIfLabels.price, value: price == null ? undefined : formatRub(price), path: whatIf, to, section: 'whatif' },
    { id: 'whatif:volume', kind: 'field', label: whatIfLabels.volume, value: percent(ov.volume_factor ?? 1), path: whatIf, to, section: 'whatif' },
    { id: 'whatif:labor', kind: 'field', label: whatIfLabels.labor, value: percent(ov.labor_factor ?? 1), path: whatIf, to, section: 'whatif' },
  ]
  if (!saved) {
    return out
  }
  const sets = assumptionSetsSelector()(state)
  const active = sets.find((a) => a.is_active)
  if (sets.length > 0) {
    out.push({ id: 'set:active', kind: 'field', label: 'Активный набор', value: active?.name, path: ['Расчёт', 'Допущения', 'Наборы допущений'], to })
  }
  for (const a of sets) {
    if (!a.id) {
      continue
    }
    const normText = (stored: number | null | undefined, share: boolean): string | undefined => {
      if (stored == null) {
        return 'по нормативу'
      }
      return share ? percent(stored) : formatRub(stored)
    }
    const values: Record<SetFieldKey, string | undefined> = {
      name: a.name,
      vat: percent(a.vat_rate),
      incl: a.prices_include_vat ? 'Да' : 'Нет',
      recover: a.vat_recoverable ? 'Да' : 'Нет',
      labor: percent(a.labor_cash_share),
      discount: percent(a.discount_rate),
      utilization: normText(a.utilization, true),
      availability: normText(a.availability, true),
      reserve: normText(a.reserve, true),
      service: normText(a.service_share, true),
      delivery: normText(a.delivery_share, true),
      comm: normText(a.comm_rub_per_robot_year, false),
      wage: normText(a.technician_wage_month_rub, false),
    }
    for (const f of setFields) {
      out.push({
        id: `set:${a.id}:${f.key}`,
        kind: 'field',
        label: f.label,
        fullLabel: f.fullLabel,
        value: values[f.key],
        path: ['Расчёт', 'Допущения', `Набор «${a.name}»`],
        to,
        section: `set:${a.id}`,
      })
    }
  }
  for (const c of sharedCostsSelector()(state)) {
    if (!c.id) {
      continue
    }
    out.push({
      id: `cost:${c.id}`,
      kind: 'field',
      label: c.label,
      value: `${formatRub(c.rub)}, ${c.bucket === 'capex' ? 'CAPEX' : 'OPEX/год'}`,
      path: ['Расчёт', 'Допущения', 'Общая инфраструктура'],
      to,
      section: 'costs',
    })
  }
  return out
}

// processEntries lists the process cards and the fields of their editor, which opens only on confirm.
export function processEntries(base: string, state: State): Draft[] {
  const to = tabHref(base, 'object', 'processes')
  const out: Draft[] = []
  for (const p of processesSelector()(state)) {
    if (!p.id) {
      continue
    }
    const name = p.name || p.code
    out.push({ id: `process:${p.id}`, kind: 'process', label: name, path: ['Объект', 'Процессы'], to, section: 'processes' })
    for (const f of processFields) {
      out.push({
        id: `process:${p.id}:${f.key}`,
        kind: 'processField',
        label: f.label,
        value: withUnit(f.value(p), f.unit),
        path: ['Объект', 'Процессы', name],
        to,
        anchor: `process:${p.id}`,
        section: `process:${p.id}`,
      })
    }
  }
  return out
}

// mapEntries lists the points, zones, obstacles and flows of the map. The map page selects the object and brings
// it into view; the panel with its fields takes the focus.
export function mapEntries(base: string, state: State): Draft[] {
  const doc = mapFromState(state)
  if (!doc) {
    return []
  }
  const to = tabHref(base, 'object', 'map')
  const at = (group: string) => ['Объект', 'Карта', group]
  const common = { kind: 'map' as const, to, anchor: 'map:canvas', focusAt: 'map:selection' }
  const out: Draft[] = []
  for (const p of doc.layers.points) {
    out.push({ ...common, id: `map:${p.id}`, label: pointOption(p), path: at(labelOf(pointKinds, p.kind)) })
  }
  for (const z of doc.layers.zones) {
    out.push({ ...common, id: `map:${z.id}`, label: z.name || featureLabel(z), note: labelOf(zoneKinds, z.kind), path: at('Зоны') })
  }
  for (const o of doc.layers.obstacles) {
    out.push({ ...common, id: `map:${o.id}`, label: o.name || featureLabel(o), note: labelOf(obstacleKinds, o.kind), path: at('Препятствия') })
  }
  const names = new Map(processesSelector()(state).map((p) => [p.code, p.name || p.code]))
  for (const f of doc.layers.flows ?? []) {
    out.push({
      ...common,
      id: `map:flow:${f.process_code}`,
      label: `Поток «${names.get(f.process_code) ?? 'процесс'}»`,
      path: at('Потоки'),
    })
  }
  return out
}

// resultEntries lists what the last calculation shows: cost lines, formulas and risks. It never starts one.
export function resultEntries(base: string, result: CalculateResult): Draft[] {
  const out: Draft[] = []
  result.breakdown?.forEach((line, i) => {
    out.push({
      id: `line:${i}`,
      kind: 'result',
      label: line.label,
      value: formatRub(line.rub),
      note: line.note,
      path: ['Расчёт', 'Детализация', lineScenarioLabel(result, line.scenario)],
      to: tabHref(base, 'calc', 'items'),
      section: `lines:${line.scenario}`,
    })
  })
  for (const f of result.formulas ?? []) {
    out.push({
      id: `formula:${f.id}`,
      kind: 'result',
      label: f.text,
      value: f.unit,
      path: ['Расчёт', 'Формулы и источники', 'Формулы'],
      to: tabHref(base, 'calc', 'method'),
      section: 'formulas',
    })
  }
  for (const r of result.risks ?? []) {
    out.push({ id: `risk:${r.id}`, kind: 'result', label: r.text, path: ['Расчёт', 'Итог', 'Риски'], to: tabHref(base, 'calc', 'summary'), section: 'risks' })
  }
  return out
}

export function runEntries(base: string, runs: readonly RunHistoryItem[], closed: string | undefined): Draft[] {
  return runs.map((r) => ({
    id: `run:${r.id}`,
    kind: 'run',
    label: `${runKindLabel(r.kind)}, запуск ${r.version_no}`,
    value: formatDateTime(r.created_at),
    note: runBriefText(r),
    path: ['Расчёт', 'История'],
    to: tabHref(base, 'calc', 'history'),
    closed,
    section: 'runs',
  }))
}

export type ProjectItem = { id: string; name: string; object_type: string }

// projectEntries lists the other projects, or the other demos for a guest.
export function projectEntries(signedIn: boolean, projects: readonly ProjectItem[], currentId: string | null, demo: ObjectType | null): Draft[] {
  if (signedIn) {
    return projects
      .filter((p) => p.id !== currentId)
      .map((p) => ({
        id: `project:${p.id}`,
        kind: 'project',
        label: p.name,
        note: objectTypeLabel(p.object_type),
        path: ['Проекты'],
        to: `/p/${p.id}/object`,
        hint: 'Открывает этот проект.',
      }))
  }
  return (['warehouse', 'airport', 'hospital'] as const)
    .filter((t) => t !== demo)
    .map((t) => ({ id: `demo:${t}`, kind: 'project', label: demoName(t), path: ['Проекты'], to: `/demo/${t}/object`, hint: 'Открывает этот демо-проект.' }))
}

export type CommandInput = {
  base: string
  projectId: string | null
  signedIn: boolean
  readOnly: boolean
  calcOpen: boolean
  undo: string | null
  redo: string | null
  act: { undo: () => void; redo: () => void; theme: (c: ThemeChoice) => void; copy: () => void }
}

// commandEntries lists what the search can do besides moving. A write goes through the page's own button
// (click), so it is one undoable step like a press of that button.
export function commandEntries(c: CommandInput): Draft[] {
  const inProject = c.base !== ''
  const path = ['Команда']
  const out: Draft[] = []
  const add = (d: Omit<Draft, 'kind' | 'path'>) => out.push({ ...d, kind: 'command', path })
  if (inProject && c.undo) {
    add({ id: 'cmd:undo', label: `Отменить: ${c.undo}`, aliases: ['отмена'], run: c.act.undo, hint: `Отменяет последнее действие в этой вкладке, как ${undoKeys}.` })
  }
  if (inProject && c.redo) {
    add({ id: 'cmd:redo', label: `Вернуть: ${c.redo}`, aliases: ['повтор'], run: c.act.redo, hint: `Возвращает отменённое действие, как ${redoKeys}.` })
  }
  if (inProject && !c.readOnly) {
    add({ id: 'cmd:add-process', label: 'Добавить процесс', to: tabHref(c.base, 'object', 'processes'), click: 'action:add-process', hint: 'Добавляет процесс и открывает его редактор.' })
    add({ id: 'cmd:import', label: 'Импорт параметров из Excel или CSV', aliases: ['загрузить', 'файл'], to: tabHref(c.base, 'object'), click: 'action:import', hint: 'Открывает выбор файла с параметрами объекта.' })
  }
  if (inProject && c.calcOpen) {
    if (!c.readOnly) {
      add({ id: 'cmd:recalc', label: 'Пересчитать', aliases: ['расчёт'], to: tabHref(c.base, 'calc', 'summary'), click: 'action:recalc', hint: 'Считает проект заново на сервере.' })
    }
    add({ id: 'cmd:pdf', label: 'Скачать PDF', aliases: ['отчёт', 'экспорт'], to: tabHref(c.base, 'calc', 'export'), click: 'action:pdf', hint: 'Скачивает отчёт по текущему расчёту.' })
    add({ id: 'cmd:xlsx', label: 'Скачать Excel', aliases: ['xlsx', 'отчёт', 'экспорт'], to: tabHref(c.base, 'calc', 'export'), click: 'action:xlsx', hint: 'Скачивает таблицы текущего расчёта.' })
    if (c.projectId && !c.readOnly) {
      add({ id: 'cmd:add-set', label: 'Добавить набор допущений', to: tabHref(c.base, 'calc', 'assumptions'), click: 'action:add-set', hint: 'Добавляет набор и ставит курсор в его имя.' })
      add({ id: 'cmd:add-cost', label: 'Добавить статью инфраструктуры', to: tabHref(c.base, 'calc', 'assumptions'), click: 'action:add-cost', hint: 'Добавляет строку общей инфраструктуры.' })
    }
  }
  if (c.signedIn) {
    add({ id: 'cmd:new-project', label: 'Новый проект', aliases: ['создать'], to: '/', click: 'action:new-project', hint: 'Открывает окно нового проекта.' })
    if (c.projectId) {
      add({ id: 'cmd:copy', label: 'Копия проекта', aliases: ['копировать'], run: c.act.copy, hint: 'Создаёт копию этого проекта и открывает её.' })
    }
    add({ id: 'cmd:trash', label: 'Корзина', aliases: ['удалённые'], to: '/trash', hint: 'Открывает удалённые проекты. Корзина хранит их 30 дней.' })
  } else {
    add({ id: 'cmd:login', label: 'Войти', aliases: ['вход'], to: '/login', hint: 'Открывает вход в аккаунт.' })
  }
  if (inProject) {
    add({ id: 'cmd:projects', label: 'Все проекты', to: '/', hint: 'Открывает список проектов.' })
  }
  const themes: [ThemeChoice, string][] = [
    ['auto', 'Тема: авто'],
    ['light', 'Тема: светлая'],
    ['dark', 'Тема: тёмная'],
  ]
  for (const [choice, label] of themes) {
    add({ id: `cmd:theme-${choice}`, label, aliases: ['оформление'], run: () => c.act.theme(choice), hint: 'Меняет оформление приложения.' })
  }
  return out
}
