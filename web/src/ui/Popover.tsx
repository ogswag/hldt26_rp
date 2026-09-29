import { useEffect, useId, useLayoutEffect, useRef, type ReactNode } from 'react'

import { clampLeft, popoverScale } from './listbox'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  // trigger gets the props the button needs to control the panel.
  trigger: (props: { 'aria-expanded': boolean; 'aria-controls': string; onClick: () => void }) => ReactNode
  children: ReactNode
  label: string
  align?: 'start' | 'end'
  // side 'above' opens the panel over its button, for buttons in the bar at the bottom of the window.
  side?: 'below' | 'above'
  className?: string
}

const margin = 16

// Popover is a panel under (or over) a button: a click opens it, a click outside or Esc closes it and Esc returns focus
// to the button. It works the same with a mouse and on touch; nothing opens on hover alone.
export function Popover({ open, onOpenChange, trigger, children, label, align = 'start', side = 'below', className }: Props) {
  const id = useId()
  const box = useRef<HTMLDivElement>(null)
  const panel = useRef<HTMLDivElement>(null)

  // NOTE: near the window edge the panel moves back in, so the page never grows and scrolls sideways. It moves
  // by `left`, not a transform: the untransformed box would still widen the page.
  useLayoutEffect(() => {
    const el = panel.current
    const anchor = box.current
    if (!open || !el || !anchor) {
      return
    }
    const fit = () => {
      el.style.left = ''
      el.style.right = ''
      const r = el.getBoundingClientRect()
      const left = clampLeft(r.left, r.width, document.documentElement.clientWidth, margin)
      if (left !== r.left) {
        const scale = popoverScale(el)
        el.style.left = `${(left - anchor.getBoundingClientRect().left) / scale}px`
        el.style.right = 'auto'
      }
    }
    fit()
    window.addEventListener('resize', fit)
    return () => window.removeEventListener('resize', fit)
  }, [open])

  useEffect(() => {
    if (!open) {
      return
    }
    const onDown = (e: PointerEvent) => {
      const target = e.target as Element
      // NOTE: a Select inside the panel portals its list to the body; picking from it is not a click outside.
      if (!box.current?.contains(target) && !target.closest?.('.select-list')) {
        onOpenChange(false)
      }
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        // NOTE: preventDefault keeps a dialog around the popover open; only the popover closes.
        e.preventDefault()
        e.stopPropagation()
        onOpenChange(false)
        box.current?.querySelector<HTMLElement>(':scope > [aria-controls]')?.focus()
      }
    }
    window.addEventListener('pointerdown', onDown)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('pointerdown', onDown)
      window.removeEventListener('keydown', onKey)
    }
  }, [open, onOpenChange])

  return (
    <div className={className ? `popover ${className}` : 'popover'} ref={box}>
      {trigger({ 'aria-expanded': open, 'aria-controls': id, onClick: () => onOpenChange(!open) })}
      {open ? (
        <div id={id} ref={panel} className={`popover-panel is-${align}${side === 'above' ? ' is-above' : ''}`} role="dialog" aria-label={label}>
          {children}
        </div>
      ) : null}
    </div>
  )
}
