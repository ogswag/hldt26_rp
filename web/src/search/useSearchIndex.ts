import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useSyncExternalStore } from 'react'
import { useNavigate } from 'react-router-dom'

import {
  copyProject,
  fetchProject,
  fetchRuns,
  listProjects,
  type CalculateResult,
  type MapDocument,
  type ObjectSchema,
  type ObjectType,
} from '../api/client'
import { useAuth } from '../auth/useAuth'
import { adminShown, type Access } from '../layout/access'
import { mapFromState } from '../map/records'
import { useProjectStore, useUndoHistory } from '../store/useProjectStore'
import { undoStep } from '../store/useUndoShortcuts'
import { applyThemeChoice } from '../ui/theme'
import type { SearchEntry } from './entries'
import {
  assumptionEntries,
  commandEntries,
  mapEntries,
  navEntries,
  processEntries,
  projectEntries,
  resultEntries,
  runEntries,
  schemaEntries,
  type AccessOf,
  type Draft,
} from './sources'

export type IndexInput = {
  open: boolean
  storeKey: string
  base: string
  projectId: string | null
  demo: ObjectType | null
  schema: ObjectSchema | null
  withMap: boolean
  navAccess: Record<string, Access>
  readOnly: boolean
}

const openAccess: Access = { open: true }

// useSearchIndex builds the list the search looks through, and only while the search is open: the project's
// records, the last calculation and the lists fetched for it.
export function useSearchIndex(p: IndexInput): { entries: SearchEntry[]; mapDoc: MapDocument | null } {
  const auth = useAuth()
  const signedIn = Boolean(auth.user)
  const admin = adminShown(auth.user?.role, p.demo !== null)
  const qc = useQueryClient()
  const navigate = useNavigate()
  const store = useProjectStore(p.storeKey)
  const state = useSyncExternalStore(store.subscribe, store.getState)
  const [history, undo] = useUndoHistory(p.storeKey)
  const inProject = p.base !== ''

  const projectQ = useQuery({
    queryKey: ['project', p.projectId],
    queryFn: () => fetchProject(p.projectId as string),
    enabled: Boolean(p.projectId),
  })
  const runsQ = useQuery({
    queryKey: ['runs', p.projectId],
    queryFn: () => fetchRuns(p.projectId as string),
    enabled: p.open && Boolean(p.projectId),
  })
  const projectsQ = useQuery({ queryKey: ['projects'], queryFn: listProjects, enabled: p.open && signedIn })
  const copy = useMutation({
    mutationFn: (id: string) => copyProject(id),
    onSuccess: (c) => {
      void qc.invalidateQueries({ queryKey: ['projects'] })
      navigate(`/p/${c.id}/object`)
    },
  })
  const copyMutate = copy.mutate

  const saved = projectQ.data?.results
  // A demo keeps its last result in the query cache; reading it here never starts a calculation.
  const result: CalculateResult | null = useMemo(() => {
    if (!p.open) {
      return null
    }
    if (p.projectId) {
      return saved && Array.isArray(saved.scenarios) && saved.scenarios.length > 0 ? saved : null
    }
    if (p.demo) {
      return qc.getQueriesData<CalculateResult>({ queryKey: ['calc', 'guest', p.demo] }).find(([, d]) => d)?.[1] ?? null
    }
    return null
  }, [p.open, p.projectId, p.demo, saved, qc])

  const mapDoc = useMemo(() => (p.open && p.withMap && store.ready ? mapFromState(state) : null), [p.open, p.withMap, store, state])

  const entries = useMemo(() => {
    if (!p.open) {
      return []
    }
    const accessOf: AccessOf = (tab, sub) => p.navAccess[sub ? `${tab}:${sub}` : tab] ?? openAccess
    const drafts: Draft[] = []
    const calc = accessOf('calc', null)
    drafts.push(...navEntries(p.base, p.schema, p.withMap, p.demo !== null, accessOf, admin))
    if (inProject) {
      if (store.ready) {
        if (p.schema) {
          drafts.push(...schemaEntries(p.base, p.schema, state))
        }
        drafts.push(...processEntries(p.base, state))
        if (p.withMap) {
          drafts.push(...mapEntries(p.base, state))
        }
        if (calc.open && result) {
          drafts.push(...assumptionEntries(p.base, state, result, Boolean(p.projectId)))
        }
      }
      if (calc.open && result) {
        drafts.push(...resultEntries(p.base, result))
      }
      if (p.projectId) {
        const runsAccess = accessOf('calc', 'history')
        drafts.push(...runEntries(p.base, runsQ.data?.items ?? [], runsAccess.open ? undefined : runsAccess.reason))
      }
    }
    drafts.push(
      ...commandEntries({
        base: p.base,
        projectId: p.projectId,
        signedIn,
        readOnly: p.readOnly,
        calcOpen: calc.open,
        undo: undo.undo,
        redo: undo.redo,
        act: {
          undo: () => undoStep(history, false),
          redo: () => undoStep(history, true),
          theme: applyThemeChoice,
          copy: () => p.projectId && copyMutate(p.projectId),
        },
      }),
    )
    drafts.push(...projectEntries(signedIn, projectsQ.data?.items ?? [], p.projectId, p.demo))
    return drafts.map((d, order) => ({ ...d, order }))
  }, [p, inProject, admin, store, state, result, runsQ.data, projectsQ.data, signedIn, undo, history, copyMutate])

  return { entries, mapDoc }
}
