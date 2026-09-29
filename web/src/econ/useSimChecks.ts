import { useQuery } from '@tanstack/react-query'

import { fetchProject, fetchSimChecks, type CalculateResult, type EconSimCheck } from '../api/client'
import { simChecks, type SimRunBrief } from '../engine/client'
import { useProjectRoute } from '../layout/route'
import type { DemoRun } from '../sim/demoRuns'
import { demoRunStore } from '../sim/demoStore'
import { stateFingerprint } from '../store/fingerprint'
import { useProjectStore, useStoreSelector } from '../store/useProjectStore'

const none: EconSimCheck[] = []

function briefOf(run: DemoRun): SimRunBrief {
  const s = run.summary
  return { run_id: run.id, variant_id: s.variant_id, variant_hash: s.variant_hash ?? '', econ_check: s.econ_check }
}

// useSimChecks lists how each variant of the project stands against a simulation. A warehouse is checked by the
// runs on its map, read when the page opens and again when a run ends; the other objects carry a quick check in
// the calculation itself.
export function useSimChecks(result: CalculateResult | null | undefined): EconSimCheck[] {
  const route = useProjectRoute()
  const projectId = route.projectId
  const store = useProjectStore(route.storeKey)
  const demoFingerprint = useStoreSelector(store, () => (projectId ? '' : stateFingerprint(store.getState())))
  const warehouse = result?.object_type === 'warehouse'

  const projectQ = useQuery({
    queryKey: ['project', projectId],
    queryFn: () => fetchProject(projectId as string),
    enabled: Boolean(projectId) && warehouse,
  })
  const hash = projectQ.data?.input_hash
  const server = useQuery({
    queryKey: ['sim-checks', projectId, hash],
    queryFn: async () => (await fetchSimChecks(projectId as string)).items,
    enabled: Boolean(projectId) && warehouse && hash !== undefined,
  })
  const demo = useQuery({
    queryKey: ['demo-sim-checks', route.storeKey, demoFingerprint],
    queryFn: async () => simChecks(store.getState(), (await demoRunStore.list(route.storeKey)).map(briefOf)),
    enabled: !projectId && warehouse && store.ready,
  })

  if (!warehouse) {
    return result?.sim_checks ?? none
  }
  return (projectId ? server.data : demo.data) ?? none
}

// raised lists the checks that found the simulation and the economics apart on inputs the variant still has.
export function raised(checks: EconSimCheck[]): EconSimCheck[] {
  return checks.filter((c) => c.flag && !c.stale)
}
