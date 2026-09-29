import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'

import { fetchProject } from '../api/client'
import { Reflow } from '../ui/Reflow'

import { SimPage } from './SimPage'
import { serverSource } from './source'

export function ProjectSim({ projectId }: { projectId: string }) {
  const qc = useQueryClient()
  const projectQ = useQuery({ queryKey: ['project', projectId], queryFn: () => fetchProject(projectId) })
  const source = useMemo(() => serverSource(projectId), [projectId])
  const project = projectQ.data

  if (projectQ.isPending) {
    return (
      <section>
        <h1 className="sr-only">Симуляция</h1>
        <p>Загрузка...</p>
      </section>
    )
  }
  if (projectQ.isError || !project) {
    return (
      <section>
        <h1 className="sr-only">Симуляция</h1>
        <p className="error">
          <Reflow>{projectQ.error instanceof Error ? projectQ.error.message : 'Не удалось загрузить проект.'}</Reflow>
        </p>
      </section>
    )
  }
  if (project.object_type !== 'warehouse') {
    return (
      <section>
        <h1 className="sr-only">Симуляция</h1>
        <p>
          <Reflow>Симуляция пока доступна только для склада.</Reflow>
        </p>
      </section>
    )
  }
  return (
    <SimPage
      source={source}
      ctx={{
        storeKey: projectId,
        results: project.results,
        robotsHref: `/p/${projectId}/robots`,
        mapHref: `/p/${projectId}/object/map`,
        historyHref: `/p/${projectId}/calc/history`,
        stale: Boolean(project.stale),
        onStarted: () => void qc.invalidateQueries({ queryKey: ['project', projectId] }),
      }}
    />
  )
}
