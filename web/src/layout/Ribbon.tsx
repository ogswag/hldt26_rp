import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useLayoutEffect, useRef, useState, type RefObject } from 'react'
import { Link, useNavigate } from 'react-router-dom'

import { copyProject, listProjects, type ObjectSchema, type ObjectType } from '../api/client'
import { demoName } from '../guest/seed'
import { SearchBox } from '../search/SearchBox'
import { ChevronDownIcon, WarningIcon } from '../ui/icons'
import { Popover } from '../ui/Popover'
import { Reflow } from '../ui/Reflow'
import type { Access } from './access'
import { AccountMenu } from './AccountMenu'
import { shownTabs, tabHref, type Subtab, type TabId } from './nav'
import { PresenceFaces } from './PresenceFaces'

type Props = {
  // name is the open project, empty while it loads or outside one.
  name: string
  // admin shows the Админ tab: the user is an admin and this is not a demo.
  admin: boolean
  // atStart is true on the start page, the Проекты tab outside a project.
  atStart: boolean
  demo: ObjectType | null
  base: string
  tab: TabId | null
  sub: string | null
  access: Record<TabId, Access> | null
  subtabs: Subtab[]
  errorTabs: readonly string[]
  storeKey: string
  projectId: string | null
  readOnly: boolean
  objectType: ObjectType | null
  schema: ObjectSchema | null
  withMap: boolean
  // navAccess is the access of every tab and subtab, keyed "calc" and "calc:summary".
  navAccess: Record<string, Access>
}

// ClosedReason explains why a tab is closed and links to what opens it.
function ClosedReason({ access, base, onGo }: { access: Access; base: string; onGo: () => void }) {
  if (access.open) {
    return null
  }
  return (
    <>
      <p>
        <Reflow>{access.reason}</Reflow>
      </p>
      {access.link ? (
        <p>
          <Link to={tabHref(base, access.link.tab, access.link.sub)} onClick={onGo}>
            {access.link.label}
          </Link>
        </p>
      ) : null}
    </>
  )
}

function ClosedTab({ label, access, base }: { label: string; access: Access; base: string }) {
  const [open, setOpen] = useState(false)
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      label={`Почему закрыта вкладка «${label}»`}
      className="closed-tab"
      trigger={(t) => (
        <button type="button" className="ribbon-tab" aria-disabled="true" {...t}>
          {label}
        </button>
      )}
    >
      <ClosedReason access={access} base={base} onGo={() => setOpen(false)} />
    </Popover>
  )
}

const demoTypes: ObjectType[] = ['warehouse', 'airport', 'hospital']

// ProjectMenu is the open project's name at the left end of the tab row. It opens the project list, the other
// projects, a copy of this one and its export.
function ProjectMenu({ p }: { p: Props }) {
  const [open, setOpen] = useState(false)
  const nav = useNavigate()
  const qc = useQueryClient()
  const signedIn = p.projectId !== null
  const list = useQuery({ queryKey: ['projects'], queryFn: listProjects, enabled: signedIn && open })
  const copy = useMutation({
    mutationFn: (id: string) => copyProject(id),
    onSuccess: (c) => {
      void qc.invalidateQueries({ queryKey: ['projects'] })
      setOpen(false)
      nav(`/p/${c.id}/object`)
    },
  })
  const others = signedIn
    ? (list.data?.items ?? []).filter((x) => x.id !== p.projectId).slice(0, 5).map((x) => ({ key: x.id, name: x.name, to: `/p/${x.id}/object` }))
    : demoTypes.filter((t) => t !== p.demo).map((t) => ({ key: t, name: demoName(t), to: `/demo/${t}/object` }))
  const exportOpen = p.access?.calc.open === true
  const close = () => setOpen(false)
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      label="Проект"
      className="project-menu"
      trigger={(t) => (
        <button type="button" className="project-button" title={p.name || undefined} {...t}>
          <span className="project-name">{p.name || 'Проект'}</span>
          <ChevronDownIcon size={16} />
        </button>
      )}
    >
      <div className="menu-section">
        <Link className="menu-item" to="/" onClick={close}>
          Все проекты
        </Link>
      </div>
      {others.length > 0 ? (
        <div className="menu-section">
          <p className="menu-heading">{signedIn ? 'Другие проекты' : 'Другие демо'}</p>
          <ul className="tab-menu-list">
            {others.map((o) => (
              <li key={o.key}>
                <Link className="menu-item menu-item-name" to={o.to} onClick={close}>
                  <span>{o.name}</span>
                </Link>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {signedIn || exportOpen ? (
        <div className="menu-section">
          {signedIn ? (
            <button
              type="button"
              className="menu-item"
              disabled={copy.isPending}
              onClick={() => copy.mutate(p.projectId as string)}
            >
              {copy.isPending ? 'Копируем...' : 'Копия проекта'}
            </button>
          ) : null}
          {exportOpen ? (
            <Link className="menu-item" to={tabHref(p.base, 'calc', 'export')} onClick={close}>
              Экспорт
            </Link>
          ) : null}
          {copy.isError ? (
            <p className="menu-note error">
              <Reflow>
                {copy.error instanceof Error ? copy.error.message : 'Не удалось скопировать проект.'} Повторите попытку.
              </Reflow>
            </p>
          ) : null}
        </div>
      ) : null}
    </Popover>
  )
}

// NavMenu stands in for both rows below 720 px: one button names the tab and the subtab, the list holds every
// tab with the subtabs of the open one under it.
function NavMenu({ p }: { p: Props }) {
  const [open, setOpen] = useState(false)
  const list = shownTabs(p.admin)
  const current = list.find((t) => t.id === p.tab)
  const sub = p.subtabs.find((s) => s.id === p.sub)
  const broken = (id: string) => p.tab === 'object' && p.errorTabs.includes(id)
  const close = () => setOpen(false)
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      label="Разделы"
      className="nav-menu"
      trigger={(t) => (
        <button type="button" className="subtab-menu-button" {...t}>
          <span className="nav-menu-text">{current ? (sub ? `${current.label}: ${sub.label}` : current.label) : 'Разделы'}</span>
          {p.subtabs.some((s) => broken(s.id)) ? <ErrorMark text="в подразделах есть ошибки" /> : null}
          <ChevronDownIcon size={16} />
        </button>
      )}
    >
      <ul className="tab-menu-list">
        {list.map((t) => {
          const a = p.access?.[t.id] ?? { open: true as const }
          return (
            <li key={t.id}>
              {a.open ? (
                <Link
                  className="menu-item"
                  to={tabHref(p.base, t.id)}
                  aria-current={p.tab === t.id && !sub ? 'page' : undefined}
                  onClick={close}
                >
                  {t.label}
                </Link>
              ) : (
                <div className="menu-item is-closed" aria-disabled="true">
                  <span>{t.label}</span>
                  <span className="menu-item-note">
                    <ClosedReason access={a} base={p.base} onGo={close} />
                  </span>
                </div>
              )}
              {p.tab === t.id && p.subtabs.length > 0 ? (
                <ul className="tab-menu-list nav-menu-subs">
                  {p.subtabs.map((s) => (
                    <li key={s.id}>
                      <Link
                        className="menu-item"
                        to={tabHref(p.base, t.id, s.id)}
                        aria-current={p.sub === s.id ? 'page' : undefined}
                        onClick={close}
                      >
                        {s.label}
                        {broken(s.id) ? <ErrorMark /> : null}
                      </Link>
                    </li>
                  ))}
                </ul>
              ) : null}
            </li>
          )
        })}
      </ul>
    </Popover>
  )
}

function ErrorMark({ text = 'есть ошибки' }: { text?: string }) {
  return (
    <span className="subtab-mark">
      <WarningIcon size={14} />
      <span className="sr-only"> ({text})</span>
    </span>
  )
}

// SubtabMenu stands in for the subtab row when the subtabs do not fit: one button with the current subtab, all
// of them in a list. The button carries the error mark of any subtab, so no error hides in the list.
function SubtabMenu({ p }: { p: Props }) {
  const [open, setOpen] = useState(false)
  const current = p.subtabs.find((s) => s.id === p.sub)
  const broken = (id: string) => p.tab === 'object' && p.errorTabs.includes(id)
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      label="Подразделы"
      className="subtab-menu"
      trigger={(t) => (
        <button type="button" className="subtab-menu-button" {...t}>
          <span>{current?.label ?? 'Подразделы'}</span>
          {p.subtabs.some((s) => broken(s.id)) ? <ErrorMark text="в подразделах есть ошибки" /> : null}
          <ChevronDownIcon size={16} />
        </button>
      )}
    >
      <ul className="tab-menu-list">
        {p.subtabs.map((s) => (
          <li key={s.id}>
            <Link
              className="menu-item"
              to={tabHref(p.base, p.tab as TabId, s.id)}
              aria-current={p.sub === s.id ? 'page' : undefined}
              onClick={() => setOpen(false)}
            >
              {s.label}
              {broken(s.id) ? <ErrorMark /> : null}
            </Link>
          </li>
        ))}
      </ul>
    </Popover>
  )
}

// useSubtabsFit tells whether every subtab fits in the row. The list keeps its full width even while the menu
// stands in for it, so the answer does not depend on which one is shown.
function useSubtabsFit(row: RefObject<HTMLDivElement | null>, list: RefObject<HTMLUListElement | null>, key: string): boolean {
  const [fits, setFits] = useState(true)
  useLayoutEffect(() => {
    const r = row.current
    const l = list.current
    if (!r || !l) {
      return
    }
    const check = () => {
      const cs = getComputedStyle(r)
      const room = r.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight)
      setFits(l.getBoundingClientRect().width <= room + 0.5)
    }
    check()
    const ro = new ResizeObserver(check)
    ro.observe(r)
    ro.observe(l)
    return () => ro.disconnect()
  }, [row, list, key])
  return fits
}

export function Ribbon(p: Props) {
  const rowRef = useRef<HTMLDivElement>(null)
  const subRef = useRef<HTMLUListElement>(null)
  const inProject = p.base !== ''
  const withSubtabs = p.subtabs.length > 0 && p.tab !== null
  const fits = useSubtabsFit(rowRef, subRef, withSubtabs ? `${p.tab}:${p.subtabs.map((s) => s.id).join(' ')}` : '')

  return (
    <header className="ribbon">
      <div className="ribbon-top">
        {inProject ? (
          <>
            <ProjectMenu p={p} />
            <span className="ribbon-sep" aria-hidden="true" />
            <nav className="ribbon-tabs" aria-label="Разделы">
              <ul>
                {shownTabs(p.admin).map((t) => {
                  const a = p.access?.[t.id] ?? { open: true as const }
                  return (
                    <li key={t.id}>
                      {a.open ? (
                        <Link
                          className="ribbon-tab"
                          to={tabHref(p.base, t.id)}
                          aria-current={p.tab === t.id ? 'page' : undefined}
                        >
                          {t.label}
                        </Link>
                      ) : (
                        <ClosedTab label={t.label} access={a} base={p.base} />
                      )}
                    </li>
                  )
                })}
              </ul>
            </nav>
          </>
        ) : (
          <nav className="ribbon-tabs ribbon-home-tabs" aria-label="Разделы">
            <ul>
              <li>
                <Link className="ribbon-tab" to="/" aria-current={p.atStart ? 'page' : undefined}>
                  Проекты
                </Link>
              </li>
              {p.admin ? (
                <li>
                  <Link className="ribbon-tab" to={tabHref('', 'admin')} aria-current={p.tab === 'admin' ? 'page' : undefined}>
                    Админ
                  </Link>
                </li>
              ) : null}
            </ul>
          </nav>
        )}
        <div className="ribbon-right">
          <SearchBox
            storeKey={p.storeKey}
            base={p.base}
            projectId={p.projectId}
            demo={p.demo}
            objectType={p.objectType}
            schema={p.schema}
            withMap={p.withMap}
            navAccess={p.navAccess}
            readOnly={p.readOnly}
          />
          {p.projectId ? <PresenceFaces projectId={p.projectId} /> : null}
          <AccountMenu storeKey={p.storeKey} readOnly={p.readOnly} catalog={p.tab === 'admin'} />
        </div>
      </div>
      {inProject ? (
        <div className="ribbon-sub ribbon-sub-phone">
          <NavMenu p={p} />
        </div>
      ) : null}
      {withSubtabs ? (
        <div
          className={inProject ? 'ribbon-sub ribbon-sub-wide' : 'ribbon-sub ribbon-sub-wide is-home'}
          ref={rowRef}
          data-menu={fits ? undefined : ''}
        >
          <nav aria-label="Подразделы">
            <ul ref={subRef}>
              {p.subtabs.map((s) => {
                const current = p.sub === s.id
                return (
                  <li key={s.id}>
                    <Link
                      className="subtab"
                      to={tabHref(p.base, p.tab as TabId, s.id)}
                      aria-current={current ? 'page' : undefined}
                    >
                      {s.label}
                      {p.tab === 'object' && p.errorTabs.includes(s.id) ? <ErrorMark /> : null}
                    </Link>
                  </li>
                )
              })}
            </ul>
          </nav>
          {fits ? null : <SubtabMenu p={p} />}
        </div>
      ) : null}
    </header>
  )
}
