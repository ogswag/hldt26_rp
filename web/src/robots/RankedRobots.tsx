import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'

import { fetchProject, type MatchItem, type ScoreParts } from '../api/client'
import { useCalcNotice } from '../econ/useCalcNotice'
import { useCalcResult } from '../econ/useCalcResult'
import { pickEdit, variantsSelector, type Edit } from '../fleet/records'
import { useProjectRoute } from '../layout/route'
import { reasonText } from '../store/reasons'
import { useProjectStore, useStoreSelector, useUndoHistory } from '../store/useProjectStore'
import { useReadOnly } from '../ui/readOnly'
import { Reflow } from '../ui/Reflow'

import { pickWarning } from './format'
import { PickMenu } from './PickMenu'
import type { RankedRow } from './ranking'
import { Slots } from './Slots'
import { suggestedItem } from './suggestion'
import { SuggestionPanel } from './SuggestionPanel'

// Ranking is what the robot list needs from the calculation: its verdicts and buy cases, the robots the
// calculation runs on, and the pick button of a row.
export type Ranking = {
  items: MatchItem[]
  weights: ScoreParts
  objectType: string
  inCalc: ReadonlySet<string>
  pick?: (row: RankedRow) => ReactNode
}

// RankedRobots is Роботы once the project can be calculated: the suggestion, the variants and every robot ranked
// by fit, then payback. Like Итог, it recalculates a project whose inputs changed.
export function RankedRobots({ list }: { list: (ranking: Ranking) => ReactNode }) {
  const route = useProjectRoute()
  const calc = useCalcResult()
  const readOnly = useReadOnly()
  const store = useProjectStore(route.storeKey)
  const [history] = useUndoHistory(route.storeKey)
  const selectVariants = useMemo(() => variantsSelector(), [])
  const variants = useStoreSelector(store, selectVariants)
  const [error, setError] = useState('')
  const projectQ = useQuery({
    queryKey: ['project', route.projectId],
    queryFn: () => fetchProject(route.projectId as string),
    enabled: Boolean(route.projectId),
  })

  // NOTE: one try per state of the inputs, so a calculation the server refuses is not repeated in a loop.
  const tried = useRef<string | undefined>(undefined)
  const hash = projectQ.data?.input_hash
  const { stale, loading, error: calcError, recalc } = calc
  const auto = Boolean(route.projectId) && stale && !loading && !calcError && !readOnly
  useEffect(() => {
    if (auto && tried.current !== hash) {
      tried.current = hash
      void recalc().catch(() => {})
    }
  }, [auto, hash, recalc])
  useCalcNotice({ active: calc.result !== null, loading, error: calcError, retry: () => void recalc().catch(() => {}) })

  if (!calc.result) {
    return (
      <section>
        <h1 className="sr-only">Роботы</h1>
        {calcError ? (
          <p className="error">
            <Reflow>{calcError}</Reflow>
          </p>
        ) : (
          <p>
            <Reflow>Подбираем роботов и считаем окупаемость...</Reflow>
          </p>
        )}
      </section>
    )
  }

  const result = calc.result
  const items = result.match?.items ?? []
  const best = suggestedItem(result)
  const names = new Map(items.map((i) => [i.solution_id, i.name]))
  const statuses = new Map(items.map((i) => [i.solution_id, i.status]))
  const inSlots = new Set(variants.flatMap((v) => v.fleet.flatMap((f) => (f.solution_id ? [f.solution_id] : []))))
  const inCalc = inSlots.size > 0 ? inSlots : new Set(best ? [best.solution_id] : [])
  const canPick = !readOnly && variants.length > 0

  const run = (edit: Edit) => {
    setError('')
    if (edit.ops.length === 0) {
      return
    }
    const r = history.run(edit.label, edit.ops)
    if (r.outcome.status === 'rejected') {
      setError(`Изменение не применено. ${reasonText(r.outcome.reason)}`)
    }
  }
  const pickMenu = (item: MatchItem | null | undefined, id: string, name: string) => (
    <PickMenu
      solutionId={id}
      name={name}
      variants={variants}
      warning={pickWarning(item)}
      onPick={(variantId) =>
        run(pickEdit(store.getState(), variantId, id, item?.estimate?.fleet_size ?? 1, item?.estimate?.process_codes ?? []))
      }
    />
  )

  return (
    <section>
      <h1 className="sr-only">Роботы</h1>
      <SuggestionPanel
        item={best}
        why={result.match?.best_why ?? []}
        weights={result.match.weights}
        objectType={result.object_type}
        pick={best && canPick ? pickMenu(best, best.solution_id, best.name) : null}
      />
      {variants.length > 0 ? <Slots variants={variants} names={names} statuses={statuses} state={() => store.getState()} run={run} /> : null}
      {error ? (
        <p className="error">
          <Reflow>{error}</Reflow>
        </p>
      ) : null}
      {list({
        items,
        weights: result.match.weights,
        objectType: result.object_type,
        inCalc,
        pick: canPick ? (row) => pickMenu(row.item, row.solution.id, row.solution.name) : undefined,
      })}
    </section>
  )
}
