import { useEffect, useRef, useState } from 'react'

const savePauseMs = 600

// Saves 600 ms after the last key, and at once on Enter and on leaving the field.
// Without saveOnPause only finish saves, for a field whose save starts a server calculation.
export function useSaveTimer(save: () => void, saveOnPause = true) {
  const timer = useRef<number | undefined>(undefined)
  // The timer outlives the render that set it, so it calls the newest save, not the one it was created with.
  const now = useRef(save)
  useEffect(() => {
    now.current = save
  })
  useEffect(() => () => window.clearTimeout(timer.current), [])
  return {
    typed: () => {
      window.clearTimeout(timer.current)
      if (saveOnPause) {
        timer.current = window.setTimeout(() => now.current(), savePauseMs)
      }
    },
    finish: () => {
      window.clearTimeout(timer.current)
      save()
    },
  }
}

// useFieldDraft keeps what the user types and saves it by useSaveTimer. Until then the field shows the typed text,
// not the stored value.
export function useFieldDraft(save: (text: string) => void, saveOnPause = true) {
  const [text, setText] = useState<string | null>(null)
  const timer = useSaveTimer(() => {
    if (text !== null) {
      save(text)
    }
  }, saveOnPause)
  const finish = () => {
    timer.finish()
    setText(null)
  }
  return {
    text,
    onChange: (e: { target: { value: string } }) => {
      setText(e.target.value)
      timer.typed()
    },
    onBlur: finish,
    onKeyDown: (e: { key: string }) => {
      if (e.key === 'Enter') {
        finish()
      }
    },
  }
}
