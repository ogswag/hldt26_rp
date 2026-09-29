import type { NavigateFunction } from 'react-router-dom'

export type ToastAction = { label: string; run: (navigate: NavigateFunction) => void }
export type Toast = { id: number; message: string; action?: ToastAction; durationMs: number }

const defaultDurationMs = 8000

let current: Toast | null = null
let seq = 0
const listeners = new Set<() => void>()

function emit() {
  for (const l of listeners) {
    l()
  }
}

export function showToast(t: { message: string; action?: ToastAction; durationMs?: number }): void {
  seq += 1
  current = { id: seq, message: t.message, action: t.action, durationMs: t.durationMs ?? defaultDurationMs }
  emit()
}

export function dismissToast(id: number): void {
  if (current?.id === id) {
    current = null
    emit()
  }
}

export function subscribeToast(onChange: () => void): () => void {
  listeners.add(onChange)
  return () => listeners.delete(onChange)
}

export function currentToast(): Toast | null {
  return current
}
