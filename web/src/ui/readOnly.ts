import { createContext, use } from 'react'

export const ReadOnlyContext = createContext(false)

export function useReadOnly(): boolean {
  return use(ReadOnlyContext)
}
