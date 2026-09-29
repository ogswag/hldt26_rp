import { useParams } from 'react-router-dom'

import type { ObjectType } from '../api/client'
import { isObjectType } from '../guest/store'
import { storeId } from '../store/useProjectStore'
import { projectBase, tabHref, type TabId } from './nav'

export type ProjectRoute = {
  projectId: string | undefined
  demo: ObjectType | null
  // storeKey is the store the page reads: the project's, or the demo's.
  storeKey: string
  base: string
  href: (tab: TabId, sub?: string) => string
}

// useProjectRoute reads the project or demo of the address. Pages under /p/:projectId and /demo/:demo use it.
export function useProjectRoute(): ProjectRoute {
  const params = useParams()
  const demo = isObjectType(params.demo) ? params.demo : null
  const projectId = params.projectId
  const base = projectBase(projectId, demo)
  return {
    projectId,
    demo,
    storeKey: storeId(projectId, demo),
    base,
    href: (tab, sub) => tabHref(base, tab, sub),
  }
}
