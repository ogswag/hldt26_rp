import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Link, Outlet, useLocation } from 'react-router-dom'

import { fetchProject, type ObjectType } from '../api/client'
import { useAuth } from '../auth/useAuth'
import { demoName } from '../guest/seed'
import { objectSchema } from '../offline/reference'
import { hasFleet, objectTypeOf, reviewedTabsOf } from '../projects/econRecords'
import { suggestionState } from '../robots/suggestion'
import { equal } from '../store/apply'
import { storeId, useProjectStore, useReportRoute, useStoreSelector } from '../store/useProjectStore'
import { useUndoShortcuts } from '../store/useUndoShortcuts'
import { ActionBarSlot } from '../ui/actionBarSlot'
import { ReadOnlyContext } from '../ui/readOnly'
import { Reflow } from '../ui/Reflow'
import { ToastHost } from '../ui/ToastHost'
import { adminShown, subtabAccess, tabAccess, type Access, type AccessInput } from './access'
import { errorKind } from './errorKind'
import { ErrorPage } from './ErrorPage'
import {
  adminSubtabs,
  calcSubtabsOf,
  objectSubtabs,
  parseAdminPath,
  parseProjectPath,
  shownTabs,
  tabHref,
  tabs,
  type Subtab,
  type TabId,
} from './nav'
import { useStable } from '../ui/useStable'
import { Ribbon } from './Ribbon'
import { ReportTabErrors, TabAccessContext } from './shellContext'

const noAccess: Record<string, Access> = {}

// navAccessOf lists the access of every shown tab and its subtabs, keyed "calc" and "calc:summary".
function navAccessOf(
  access: Record<TabId, Access>,
  input: AccessInput,
  objectSubs: Subtab[],
  admin: boolean,
): Record<string, Access> {
  const out: Record<string, Access> = {}
  for (const t of shownTabs(admin)) {
    const a = access[t.id]
    out[t.id] = a
    const subs = subtabsOf(t.id, objectSubs, input.demo)
    for (const s of subs) {
      out[`${t.id}:${s.id}`] = a.open ? subtabAccess(input, t.id, s.id) : a
    }
  }
  return out
}

function subtabsOf(tab: TabId | null, schemaTabs: Subtab[], demo: boolean): Subtab[] {
  switch (tab) {
    case 'admin':
      return adminSubtabs
    case 'object':
      return schemaTabs
    case 'calc':
      return calcSubtabsOf(demo)
    default:
      return []
  }
}

// ClosedPage stands in for a tab opened by its address while it is still closed.
function ClosedPage({ label, access, base }: { label: string; access: Access; base: string }) {
  if (access.open) {
    return null
  }
  return (
    <section>
      <h1 className="sr-only">{label}</h1>
      <p>
        <Reflow>{access.reason}</Reflow>
      </p>
      {access.link ? (
        <p>
          <Link to={tabHref(base, access.link.tab, access.link.sub)}>{access.link.label}</Link>
        </p>
      ) : null}
    </section>
  )
}

export function Layout() {
  const loc = useLocation()
  const auth = useAuth()
  const where = parseProjectPath(loc.pathname)
  const adminWhere = parseAdminPath(loc.pathname)
  const admin = adminShown(auth.user?.role, where.demo !== null)
  // Outside a project only the admin pages have a tab: /admin/<sub>.
  const tab: TabId | null = where.tab ?? (adminWhere ? 'admin' : null)
  const sub = where.tab ? where.sub : (adminWhere?.sub ?? null)
  const storeKey = storeId(where.projectId, where.demo)
  const base = where.projectId ? `/p/${where.projectId}` : where.demo ? `/demo/${where.demo}` : ''
  const store = useProjectStore(storeKey)
  useReportRoute(storeKey, loc.pathname)
  const ready = useStoreSelector(store, () => store.ready)
  const storedType = useStoreSelector(store, objectTypeOf)
  const reviewed = useStoreSelector(store, reviewedTabsOf, equal)
  const fleet = useStoreSelector(store, hasFleet)
  // Form errors belong to the project they were found in; another project starts clean.
  const [errors, setErrors] = useState<{ key: string; tabs: string[] }>({ key: '', tabs: [] })
  const errorTabs = errors.key === storeKey ? errors.tabs : []
  const [barSlot, setBarSlot] = useState<HTMLDivElement | null>(null)

  // NOTE: on Админ the keys belong to the catalog edits, not to the project around it.
  useUndoShortcuts(tab === 'admin' ? '' : storeKey)

  const projectQ = useQuery({
    queryKey: ['project', where.projectId],
    queryFn: () => fetchProject(where.projectId as string),
    enabled: Boolean(where.projectId),
  })
  const project = projectQ.data
  const objectType: ObjectType | null =
    project && (project.object_type === 'warehouse' || project.object_type === 'airport' || project.object_type === 'hospital')
      ? project.object_type
      : (where.demo ?? storedType)

  const schemaQ = useQuery({
    queryKey: ['schema', objectType],
    queryFn: () => objectSchema(objectType as ObjectType),
    enabled: objectType !== null,
  })
  const schemaTabs = schemaQ.data?.tabs ?? []

  const inProject = base !== ''
  const readOnly = project?.access === 'viewer'
  // The admin pages edit the catalog, not the project, so a viewer of the project around them still edits.
  const pageReadOnly = readOnly && tab !== 'admin'
  const accessInput: AccessInput = {
    demo: where.demo !== null,
    objectType,
    loaded: ready && schemaQ.isSuccess && (where.projectId ? projectQ.isSuccess : true),
    hasResult: Boolean(project?.current_run_id || project?.results),
    schemaTabs,
    reviewed,
    errorTabs,
    hasFleet: fleet,
    suggestion: suggestionState(project?.results),
  }
  const access = inProject ? tabAccess(accessInput) : null
  // navAccess tells the search which tabs and subtabs are closed. It keeps its identity while nothing in it
  // changes, so the search does not rebuild its list on every render of the layout.
  const navAccess = useStable(access ? navAccessOf(access, accessInput, objectSubtabs(schemaTabs, true), admin) : noAccess)

  const withMap = objectType === 'warehouse'
  // A user who is not an admin gets no admin subtabs: the page under them says the section is closed.
  const subtabs = tab === 'admin' && !admin ? [] : subtabsOf(tab, objectSubtabs(schemaTabs, withMap), where.demo !== null)
  const name = where.projectId ? (project?.name ?? '') : where.demo ? demoName(where.demo) : ''
  const tabOpen = tab && access ? access[tab] : null
  const current = tab && tabOpen?.open ? subtabAccess(accessInput, tab, sub) : tabOpen
  const failed = where.projectId && projectQ.isError ? projectQ.error : null
  const subLabel = tabOpen?.open ? subtabs.find((s) => s.id === sub)?.label : undefined
  const closedLabel = subLabel ?? tabs.find((t) => t.id === tab)?.label ?? ''

  return (
    <div className="app">
      <Ribbon
        name={name}
        admin={admin}
        atStart={loc.pathname === '/'}
        demo={where.demo}
        base={base}
        tab={tab}
        sub={sub}
        access={access}
        subtabs={subtabs}
        errorTabs={errorTabs}
        storeKey={storeKey}
        projectId={where.projectId}
        readOnly={readOnly}
        objectType={objectType}
        schema={schemaQ.data ?? null}
        withMap={withMap}
        navAccess={navAccess}
      />
      <ReadOnlyContext value={pageReadOnly}>
        <TabAccessContext value={navAccess}>
          <ActionBarSlot value={barSlot}>
            <ReportTabErrors value={setErrors}>
              <main className="page">
                <fieldset className="page-scope" disabled={pageReadOnly} aria-label={pageReadOnly ? 'Только просмотр' : undefined}>
                  {failed ? (
                    <ErrorPage {...errorKind(failed, { guest: !auth.user })} onRetry={() => void projectQ.refetch()} />
                  ) : current && !current.open ? (
                    <ClosedPage label={closedLabel} access={current} base={base} />
                  ) : (
                    <Outlet />
                  )}
                </fieldset>
              </main>
            </ReportTabErrors>
          </ActionBarSlot>
        </TabAccessContext>
      </ReadOnlyContext>
      <div ref={setBarSlot} className="action-bar-slot" />
      <ToastHost />
    </div>
  )
}
