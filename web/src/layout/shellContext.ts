import { createContext, use } from 'react'

import type { Access } from './access'

// ReportTabErrors lets the object form say which of its tabs hold a value that failed a check, and in which
// project: the shell drops a report that belongs to another one.
export const ReportTabErrors = createContext<(report: { key: string; tabs: string[] }) => void>(() => {})

// TabAccessContext holds which tabs and subtabs of the open project are open, keyed "calc" and "calc:summary",
// so a page can hold back work another tab gates.
export const TabAccessContext = createContext<Record<string, Access>>({})

export function useTabAccess(): Record<string, Access> {
  return use(TabAccessContext)
}
