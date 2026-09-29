import { useCallback, useMemo, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'

import type { Solution } from '../api/client'
import { compareCap, filterSolutions, keys, matcher, pageSize, readCompare, readFilter, writeFilter, type CatalogFilter } from './filter'
import { useCatalog } from './useCatalog'

function afterPaint(fn: () => void) {
  requestAnimationFrame(() => requestAnimationFrame(fn))
}

// useCatalogView is a catalog page: the loaded catalog, the filter and the open robot from the address, and the
// moves between them. The page orders the list; the overlay walks it in that order.
export function useCatalogView(withArchive: boolean) {
  const [params, setParams] = useSearchParams()
  const query = useCatalog(withArchive)
  const items = useMemo(() => query.data?.items ?? [], [query.data])
  const match = useMemo(() => matcher(items), [items])
  const filter = useMemo(() => readFilter(params), [params])
  const filtered = useMemo(() => filterSolutions(items, filter, match), [items, filter, match])
  const opener = useRef<Element | null>(null)
  const compareParam = params.get(keys.compare)
  const compare = useMemo(() => readCompare(compareParam), [compareParam])

  const update = useCallback(
    (change: (p: URLSearchParams) => void) =>
      setParams(
        (p) => {
          const next = new URLSearchParams(p)
          change(next)
          return next
        },
        { replace: true },
      ),
    [setParams],
  )

  const setFilter = useCallback((patch: Partial<CatalogFilter>) => setParams((p) => writeFilter(p, patch), { replace: true }), [setParams])

  const setSort = (sort: string, first: string) =>
    update((p) => {
      if (sort === first) {
        p.delete(keys.sort)
      } else {
        p.set(keys.sort, sort)
      }
      p.delete(keys.page)
    })

  const putCompare = useCallback(
    (ids: readonly string[]) => update((p) => (ids.length > 0 ? p.set(keys.compare, ids.join(',')) : p.delete(keys.compare))),
    [update],
  )

  const toggleCompare = useCallback(
    (id: string) => {
      if (compare.includes(id)) {
        putCompare(compare.filter((x) => x !== id))
      } else if (compare.length < compareCap) {
        putCompare([...compare, id])
      }
    },
    [compare, putCompare],
  )

  const setPage = (page: number) => update((p) => (page > 1 ? p.set(keys.page, String(page)) : p.delete(keys.page)))

  const open = (id: string) => {
    opener.current = document.activeElement
    update((p) => p.set(keys.robot, id))
  }

  // close shows the page of the robot seen last and puts the focus on its name, so the list goes on from there.
  const close = (ordered: readonly Solution[], last: string | null) => {
    const at = last ? ordered.findIndex((s) => s.id === last) : -1
    update((p) => {
      p.delete(keys.robot)
      if (at >= 0) {
        const page = Math.floor(at / pageSize) + 1
        if (page > 1) {
          p.set(keys.page, String(page))
        } else {
          p.delete(keys.page)
        }
      }
    })
    afterPaint(() => {
      const row = last ? document.querySelector<HTMLElement>(`[data-search-id="robot:${last}"] .robot-name`) : null
      const back = row ?? (opener.current instanceof HTMLElement && opener.current.isConnected ? opener.current : null)
      back?.focus()
    })
  }

  return {
    query,
    items,
    match,
    filter,
    filtered,
    params,
    selected: params.get(keys.robot),
    compare,
    toggleCompare,
    clearCompare: () => putCompare([]),
    setFilter,
    setSort,
    setPage,
    open,
    select: (id: string) => update((p) => p.set(keys.robot, id)),
    close,
    // showOnly leaves one search in the address: a robot the project search found.
    showOnly: (q: string) => setParams(writeFilter(new URLSearchParams(), { q }), { replace: true }),
  }
}
