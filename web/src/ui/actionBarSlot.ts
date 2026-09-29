import { createContext } from 'react'

// ActionBarSlot holds the element that page action bars portal into. Layout provides it.
export const ActionBarSlot = createContext<HTMLElement | null>(null)
