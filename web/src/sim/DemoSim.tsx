import { useMemo } from 'react'

import { engineCatalog } from '../engine/fallback'
import type { CalcBundle } from '../econ/useCalcResult'
import { useProjectRoute } from '../layout/route'
import { stateFingerprint } from '../store/fingerprint'
import { useProjectStore, useStoreSelector } from '../store/useProjectStore'
import { Reflow } from '../ui/Reflow'

import { demoRunStore } from './demoStore'
import { SimPage } from './SimPage'
import { demoSource } from './source'

// DemoSim runs the event simulation of a demo in this browser and keeps its last runs next to the demo.
export function DemoSim({ calc }: { calc: CalcBundle }) {
  const route = useProjectRoute()
  const store = useProjectStore(route.storeKey)
  const fingerprint = useStoreSelector(store, () => stateFingerprint(store.getState()))
  const source = useMemo(
    () => demoSource({ demo: route.storeKey, state: () => store.getState(), fingerprint, runs: demoRunStore, catalog: engineCatalog }),
    [route.storeKey, store, fingerprint],
  )
  if (calc.objectType !== null && calc.objectType !== 'warehouse') {
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
        storeKey: route.storeKey,
        results: calc.result,
        robotsHref: route.href('robots'),
        mapHref: route.href('object', 'map'),
        stale: false,
      }}
    />
  )
}
