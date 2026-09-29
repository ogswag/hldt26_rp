import { useQuery } from '@tanstack/react-query'
import { lazy, Suspense, useEffect } from 'react'
import { Navigate, useParams } from 'react-router-dom'

import { fetchProject, type ObjectType } from '../api/client'
import { warmCatalog } from '../engine/fallback'
import { isObjectType } from '../guest/store'
import { useProjectRoute } from '../layout/route'
import { ObjectForm } from '../object/ObjectForm'
import { objectSchema } from '../offline/reference'
import { reviewTabEdit } from '../projects/econRecords'
import { useProjectStore, useStoreSelector } from '../store/useProjectStore'
import { useReadOnly } from '../ui/readOnly'
import { Processes } from './Processes'

const MapPage = lazy(() => import('./MapPage').then((m) => ({ default: m.MapPage })))

function Status({ text, error }: { text: string; error?: boolean }) {
  return (
    <section>
      <h1 className="sr-only">Объект</h1>
      <p className={error ? 'error' : undefined}>{text}</p>
    </section>
  )
}

function message(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback
}

export function ObjectPage() {
  const route = useProjectRoute()
  const { tab } = useParams()
  const readOnly = useReadOnly()
  const { projectId } = route

  const projectQ = useQuery({
    queryKey: ['project', projectId],
    queryFn: () => fetchProject(projectId as string),
    enabled: Boolean(projectId),
  })
  const project = projectQ.data
  const objectType: ObjectType | null = projectId
    ? project && isObjectType(project.object_type)
      ? project.object_type
      : null
    : route.demo

  const schemaQ = useQuery({
    queryKey: ['schema', objectType],
    queryFn: () => objectSchema(objectType as ObjectType),
    enabled: objectType !== null,
  })
  const schema = schemaQ.data
  const store = useProjectStore(route.storeKey)
  const ready = useStoreSelector(store, () => store.ready)
  const paramTab = schema?.tabs.some((t) => t.id === tab) ? tab : undefined

  // Opening a parameter tab counts as reviewing it. It steers the tabs only, so it stays out of undo; a demo
  // keeps no marks, so looking around never counts as changing it.
  useEffect(() => {
    if (!paramTab || !ready || readOnly || !projectId) {
      return
    }
    const edit = reviewTabEdit(store.getState(), paramTab)
    if (edit.ops.length > 0) {
      store.dispatch(edit.label, edit.ops)
    }
  }, [paramTab, ready, readOnly, projectId, store])

  // A demo calculates in the browser when the server is gone, so the catalog is fetched here, on the first
  // step, while the network is still there.
  useEffect(() => {
    if (!projectId) {
      warmCatalog()
    }
  }, [projectId])

  if (tab === 'processes') {
    return <Processes />
  }
  if (tab === 'map') {
    return (
      <Suspense fallback={<Status text="Загрузка карты..." />}>
        <MapPage />
      </Suspense>
    )
  }
  if (projectId && projectQ.isPending) {
    return <Status text="Загрузка проекта..." />
  }
  if (!objectType || schemaQ.isPending) {
    return <Status text="Загрузка схемы..." />
  }
  if (schemaQ.isError || !schema) {
    return <Status text={message(schemaQ.error, 'Не удалось загрузить схему. Обновите страницу.')} error />
  }
  if (!paramTab) {
    return <Navigate to={route.href('object', schema.tabs[0]?.id)} replace />
  }
  if (!ready) {
    return <Status text="Загрузка..." />
  }
  return (
    <ObjectForm
      key={route.storeKey}
      objectType={objectType}
      schema={schema}
      tab={paramTab}
      storeKey={route.storeKey}
      project={project}
    />
  )
}
