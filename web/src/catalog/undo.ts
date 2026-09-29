import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useLayoutEffect, useRef, useSyncExternalStore } from 'react'

import { adminPatchSolution, type FieldChange, type FieldValues } from '../api/client'
import { commandKey, isTyping, onMac } from '../ui/keys'
import { showToast } from '../ui/toast'
import { putSolution } from './useCatalog'

// Step is one catalog edit made in this browser tab: one robot's fields as they were and as they became.
type Step = { id: string; label: string; before: FieldValues; after: FieldValues }

type Stack = { done: Step[]; undone: Step[]; busy: boolean }

// NOTE: the stack belongs to this tab and lives until it closes, like the project undo history.
let stack: Stack = { done: [], undone: [], busy: false }
const listeners = new Set<() => void>()
const depth = 100

function set(next: Stack) {
  stack = next
  listeners.forEach((l) => l())
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

// recordEdit puts a saved edit on the stack, from the changes the server reported for it.
export function recordEdit(id: string, label: string, changes: readonly FieldChange[]): void {
  if (changes.length === 0) {
    return
  }
  const before = Object.fromEntries(changes.map((c) => [c.field, c.before]))
  const after = Object.fromEntries(changes.map((c) => [c.field, c.after]))
  set({ ...stack, done: [...stack.done, { id, label, before, after }].slice(-depth), undone: [] })
}

export type CatalogUndo = {
  undo: string | null
  redo: string | null
  step: (redo: boolean) => void
}

// useCatalogUndo undoes and redoes this tab's catalog edits by saving the other side of the step back.
export function useCatalogUndo(): CatalogUndo {
  const qc = useQueryClient()
  const s = useSyncExternalStore(subscribe, () => stack)
  const step = (redo: boolean) => {
    const from = redo ? stack.undone : stack.done
    const top = from.at(-1)
    if (!top || stack.busy) {
      return
    }
    set({ ...stack, busy: true })
    adminPatchSolution(top.id, redo ? top.after : top.before)
      .then((out) => {
        putSolution(qc, out.solution)
        const rest = from.slice(0, -1)
        set(
          redo
            ? { done: [...stack.done, top], undone: rest, busy: false }
            : { done: rest, undone: [...stack.undone, top], busy: false },
        )
      })
      .catch((err: unknown) => {
        set({ ...stack, busy: false })
        const why = err instanceof Error ? err.message : 'Повторите.'
        showToast({ message: `${redo ? 'Повтор не применён' : 'Отмена не применена'}. ${why}` })
      })
  }
  return { undo: s.done.at(-1)?.label ?? null, redo: s.undone.at(-1)?.label ?? null, step }
}

// useCatalogUndoShortcuts gives the catalog page the undo keys of a project page: Cmd+Z and Cmd+Shift+Z on macOS,
// ctrl+Z, ctrl+Shift+Z and ctrl+Y elsewhere. A field keeps its own undo while it has the focus.
export function useCatalogUndoShortcuts(): void {
  const { step } = useCatalogUndo()
  const latest = useRef(step)
  useLayoutEffect(() => {
    latest.current = step
  })
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const key = e.key.toLowerCase()
      const redoY = key === 'y' && !onMac
      if (!commandKey(e) || (key !== 'z' && !redoY) || isTyping(e.target)) {
        return
      }
      e.preventDefault()
      latest.current(redoY || e.shiftKey)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
}
