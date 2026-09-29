import { useQuery, type QueryClient } from '@tanstack/react-query'

import { fetchSolutions, type Solution, type SolutionsResponse } from '../api/client'

// The API lists at most 500 robots in one page; the catalog is loaded whole and filtered in the browser.
const catalogLimit = 500

// catalogKey names the loaded catalog: active robots, or every robot with the archive for admins.
export function catalogKey(withArchive: boolean) {
  return ['solutions', 'catalog', withArchive ? 'all' : 'active'] as const
}

export function useCatalog(withArchive: boolean) {
  return useQuery({
    queryKey: catalogKey(withArchive),
    queryFn: () => fetchSolutions({ limit: catalogLimit, archived: withArchive ? 'include' : undefined }),
    staleTime: 60_000,
  })
}

// putSolution puts a robot the admin saved into every loaded catalog, so the lists change without a reload. An
// archived robot leaves the active list; a restored one joins it.
export function putSolution(qc: QueryClient, s: Solution): void {
  for (const withArchive of [true, false]) {
    qc.setQueryData<SolutionsResponse>(catalogKey(withArchive), (old) => {
      if (!old) {
        return old
      }
      const listed = withArchive || !s.archived_at
      const rest = old.items.filter((x) => x.id !== s.id)
      const has = rest.length !== old.items.length
      const items = !listed ? rest : has ? old.items.map((x) => (x.id === s.id ? s : x)) : [...old.items, s]
      return { ...old, items, total: items.length }
    })
  }
  qc.setQueryData(['solution', s.id], s)
  void qc.invalidateQueries({ queryKey: ['search-robots'] })
}
