import { useId, useLayoutEffect, useRef, useState } from 'react'

import { CheckList, type CheckOption } from './CheckList'
import { ChevronDownIcon } from './icons'
import { placeList, popoverScale, popoverSize, viewportSize } from './listbox'

type Props = {
  // label names the list; with rowLabel it also names the button («Приёмка: забор»).
  label: string
  rowLabel?: string
  options: readonly CheckOption[]
  value: readonly string[]
  onChange: (value: string[]) => void
  empty: string
}

const gap = 4
const margin = 8

// CheckSelect is a CheckList folded into one field, for tables where every row has its own list: the button
// names what is chosen, the checkboxes open under it. NOTE: like Select, the panel sits in the top layer, so a
// table's scroll box never clips it.
export function CheckSelect({ label, rowLabel, options, value, onChange, empty }: Props) {
  const id = useId()
  const buttonRef = useRef<HTMLButtonElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)
  const chosen = options.filter((o) => value.includes(o.value))
  const summary =
    chosen.length === 0
      ? 'не выбрано'
      : chosen.length <= 2
        ? chosen.map((o) => o.label).join(', ')
        : `${chosen.length} из ${options.length}`

  useLayoutEffect(() => {
    const panel = panelRef.current
    const button = buttonRef.current
    if (!open || !panel || !button) {
      return
    }
    if (!panel.matches(':popover-open')) {
      panel.showPopover()
    }
    const place = () => {
      const r = button.getBoundingClientRect()
      panel.style.left = '0px'
      panel.style.top = '0px'
      panel.style.maxHeight = ''
      const scale = popoverScale(panel)
      panel.style.minWidth = `${r.width / scale}px`
      const p = placeList(r, popoverSize(panel), viewportSize(), gap, margin)
      panel.style.left = `${p.left / scale}px`
      panel.style.top = `${p.top / scale}px`
      panel.style.maxHeight = `${p.maxHeight / scale}px`
    }
    const onScroll = (e: Event) => {
      if (!(e.target instanceof Node) || !panel.contains(e.target)) {
        place()
      }
    }
    const seen = new IntersectionObserver((entries) => {
      if (entries.some((entry) => !entry.isIntersecting)) {
        setOpen(false)
      }
    })
    const onPointerDown = (e: PointerEvent) => {
      const t = e.target instanceof Node ? e.target : null
      if (t && !panel.contains(t) && !button.contains(t)) {
        setOpen(false)
      }
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        // NOTE: preventDefault keeps a dialog around the list open; only the list closes.
        e.preventDefault()
        e.stopPropagation()
        setOpen(false)
        button.focus()
      }
    }
    place()
    seen.observe(button)
    window.addEventListener('resize', place)
    document.addEventListener('scroll', onScroll, true)
    document.addEventListener('pointerdown', onPointerDown, true)
    document.addEventListener('keydown', onKey, true)
    return () => {
      seen.disconnect()
      window.removeEventListener('resize', place)
      document.removeEventListener('scroll', onScroll, true)
      document.removeEventListener('pointerdown', onPointerDown, true)
      document.removeEventListener('keydown', onKey, true)
      if (panel.matches(':popover-open')) {
        panel.hidePopover()
      }
    }
  }, [open])

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        className="select check-select"
        aria-expanded={open}
        aria-controls={id}
        aria-label={`${rowLabel ? `${rowLabel}: ${label.toLowerCase()}` : label}, ${summary}`}
        title={chosen.map((o) => o.label).join(', ')}
        onClick={() => setOpen(!open)}
      >
        <span className="select-text">
          <span className={chosen.length === 0 ? 'select-value is-empty' : 'select-value'}>{summary}</span>
        </span>
        <span className="select-chevron">
          <ChevronDownIcon />
        </span>
      </button>
      {open ? (
        <div id={id} ref={panelRef} popover="manual" className="check-select-panel" role="dialog" aria-label={label}>
          <CheckList label={label} options={options} value={value} onChange={onChange} empty={empty} />
        </div>
      ) : null}
    </>
  )
}
