import { useEffect, useLayoutEffect, useRef, useState, type FocusEvent, type PointerEvent as ReactPointerEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

import { placeList, popoverScale, popoverSize, viewportSize } from './listbox'

type Props = {
  // card draws what the card says; it runs only while the card is open.
  card: () => ReactNode
  children: ReactNode
  className?: string
}

const openMs = 400
const closeMs = 150
const gap = 6
const margin = 8

function byPointer(e: ReactPointerEvent): boolean {
  return e.pointerType === 'mouse' || e.pointerType === 'pen'
}

// NOTE: a card opens on focus that the Tab key moved, not on focus a closing dialog or a script hands back.
let tabbedAt = Number.NEGATIVE_INFINITY
let tracking = false

function trackTab() {
  if (tracking) {
    return
  }
  tracking = true
  window.addEventListener(
    'keydown',
    (e) => {
      if (e.key === 'Tab') {
        tabbedAt = e.timeStamp
      }
    },
    true,
  )
}

// Opens 400 ms after the pointer rests
// on the element, or at once when the element gets keyboard focus, and closes when the pointer leaves the element
// and the card, on Esc, on a press and on scroll. It holds information only, never controls. Touch never opens it.
export function HoverCard({ card, children, className }: Props) {
  const anchor = useRef<HTMLSpanElement>(null)
  const panel = useRef<HTMLDivElement>(null)
  const timer = useRef(0)
  const [open, setOpen] = useState(false)

  const clear = () => window.clearTimeout(timer.current)
  const later = (next: boolean, ms: number) => {
    clear()
    timer.current = window.setTimeout(() => setOpen(next), ms)
  }
  useEffect(() => {
    trackTab()
    return clear
  }, [])

  useLayoutEffect(() => {
    const el = panel.current
    const a = anchor.current
    if (!open || !el || !a) {
      return
    }
    // NOTE: the card sits in the top layer, so a table's scroll box or the sticky tab row never covers it.
    if (!el.matches(':popover-open')) {
      el.showPopover()
    }
    el.style.left = '0px'
    el.style.top = '0px'
    const scale = popoverScale(el)
    const p = placeList(a.getBoundingClientRect(), popoverSize(el), viewportSize(), gap, margin)
    el.style.left = `${p.left / scale}px`
    el.style.top = `${p.top / scale}px`
    const close = () => {
      clear()
      setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        close()
      }
    }
    window.addEventListener('scroll', close, true)
    window.addEventListener('keydown', onKey, true)
    window.addEventListener('pointerdown', close, true)
    return () => {
      window.removeEventListener('scroll', close, true)
      window.removeEventListener('keydown', onKey, true)
      window.removeEventListener('pointerdown', close, true)
      if (el.matches(':popover-open')) {
        el.hidePopover()
      }
    }
  }, [open])

  const onFocus = (e: FocusEvent) => {
    if (e.timeStamp - tabbedAt < 500) {
      clear()
      setOpen(true)
    }
  }

  return (
    <span
      ref={anchor}
      className={className ? `hover-card-anchor ${className}` : 'hover-card-anchor'}
      onPointerEnter={(e) => byPointer(e) && later(true, openMs)}
      onPointerLeave={(e) => byPointer(e) && later(false, closeMs)}
      onFocus={onFocus}
      onBlur={() => {
        clear()
        setOpen(false)
      }}
    >
      {children}
      {open
        ? createPortal(
            <div
              ref={panel}
              popover="manual"
              className="hover-card"
              onPointerEnter={clear}
              onPointerLeave={(e) => byPointer(e) && later(false, closeMs)}
            >
              {card()}
            </div>,
            document.body,
          )
        : null}
    </span>
  )
}
