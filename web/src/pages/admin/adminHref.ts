import { useLocation } from 'react-router-dom'

import { parseProjectPath, projectBase, tabHref } from '../../layout/nav'

// useAdminHref builds the address of an admin subtab where the page is: inside a project or outside one.
export function useAdminHref(): (sub: string, query?: string) => string {
  const loc = useLocation()
  const base = projectBase(parseProjectPath(loc.pathname).projectId, null)
  return (sub, query) => `${tabHref(base, 'admin', sub)}${query ? `?${query}` : ''}`
}
