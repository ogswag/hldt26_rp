import { useId, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'
import { createPortal } from 'react-dom'

import { ChevronDownIcon } from './icons'
import { abbreviationTitle } from './Abbreviation'
import { findByPrefix, placeList, popoverScale, popoverSize, viewportSize } from './listbox'

export type SelectOption<T extends string = string> = { value: T; label: string }

type Props<T extends string> = {
  value: T
  options: readonly SelectOption<T>[]
  onChange: (value: T) => void
  id?: string
  'aria-label'?: string
  // prefix names the field inside it («Сортировка: Цена»), for a bar of fields without labels above them.
  prefix?: string
  fitContent?: boolean
}

type Reveal = 'none' | 'nearest' | 'center'

const gap = 4
const margin = 8
const pageStep = 10
const typeaheadMs = 1000

function reveal(list: HTMLElement, el: HTMLElement, center: boolean) {
  const pad = parseFloat(getComputedStyle(list).paddingTop) || 0
  const top = el.offsetTop
  const bottom = top + el.offsetHeight
  if (center) {
    list.scrollTop = top - (list.clientHeight - el.offsetHeight) / 2
  } else if (top - pad < list.scrollTop) {
    list.scrollTop = top - pad
  } else if (bottom + pad > list.scrollTop + list.clientHeight) {
    list.scrollTop = bottom + pad - list.clientHeight
  }
}

// NOTE: the page draws the list, not the OS menu, so it keeps the site font and scales with zoom.
// The list is portaled out of the field's label (or into its modal dialog) so it never joins the label text.
export function Select<T extends string>({ value, options, onChange, id, 'aria-label': ariaLabel, prefix, fitContent }: Props<T>) {
  const uid = useId()
  const buttonId = id ?? `${uid}-button`
  const listId = `${uid}-list`
  const buttonRef = useRef<HTMLButtonElement>(null)
  const listRef = useRef<HTMLUListElement>(null)
  const [open, setOpen] = useState(false)
  const [host, setHost] = useState<HTMLElement | null>(null)
  const [active, setActive] = useState(0)
  const revealMode = useRef<Reveal>('none')
  const typed = useRef({ text: '', at: 0 })
  const labelPress = useRef(false)
  const found = options.findIndex((o) => o.value === value)
  const shown = found >= 0 ? found : options.length > 0 ? 0 : -1
  const last = options.length - 1
  const current = Math.min(active, last)
  const optionId = (i: number) => `${uid}-option-${i}`

  useLayoutEffect(() => {
    const list = listRef.current
    const button = buttonRef.current
    if (!open || !list || !button) {
      return
    }
    const font = getComputedStyle(button)
    list.style.fontFamily = font.fontFamily
    list.style.fontSize = font.fontSize
    if (!list.matches(':popover-open')) {
      list.showPopover()
    }
    const place = () => {
      const r = button.getBoundingClientRect()
      list.style.left = '0px'
      list.style.top = '0px'
      list.style.maxHeight = ''
      const scale = popoverScale(list)
      list.style.minWidth = `${r.width / scale}px`
      const p = placeList(r, popoverSize(list), viewportSize(), gap, margin)
      list.style.left = `${p.left / scale}px`
      list.style.top = `${p.top / scale}px`
      list.style.maxHeight = `${p.maxHeight / scale}px`
      list.classList.toggle('is-above', p.above)
    }
    const onScroll = (e: Event) => {
      if (e.target !== list) {
        place()
      }
    }
    // NOTE: the list lives in the top layer, so it closes once scrolling hides the field.
    const seen = new IntersectionObserver((entries) => {
      if (entries.some((entry) => !entry.isIntersecting)) {
        setOpen(false)
      }
    })
    // NOTE: a press on the field's own label is left to the label click, which toggles the list.
    const onPointerDown = (e: PointerEvent) => {
      const t = e.target instanceof Node ? e.target : null
      if (!t || list.contains(t) || button.contains(t)) {
        labelPress.current = false
        return
      }
      labelPress.current = Array.from(button.labels).some((l) => l.contains(t))
      if (!labelPress.current) {
        setOpen(false)
      }
    }
    place()
    seen.observe(button)
    window.addEventListener('resize', place)
    document.addEventListener('scroll', onScroll, true)
    document.addEventListener('pointerdown', onPointerDown, true)
    return () => {
      seen.disconnect()
      window.removeEventListener('resize', place)
      document.removeEventListener('scroll', onScroll, true)
      document.removeEventListener('pointerdown', onPointerDown, true)
      if (list.matches(':popover-open')) {
        list.hidePopover()
      }
    }
  }, [open])

  useLayoutEffect(() => {
    const mode = revealMode.current
    revealMode.current = 'none'
    const list = listRef.current
    const el = open && mode !== 'none' ? list?.children.item(current) : null
    if (list && el instanceof HTMLElement) {
      reveal(list, el, mode === 'center')
    }
  })

  function show(index: number) {
    const button = buttonRef.current
    if (options.length === 0 || !button) {
      return
    }
    setHost(button.closest('dialog') ?? document.body)
    setActive(Math.max(0, index))
    setOpen(true)
    revealMode.current = 'center'
  }

  function commit(index: number) {
    const o = options[index]
    setOpen(false)
    if (o && o.value !== value) {
      onChange(o.value)
    }
  }

  function moveTo(index: number) {
    setActive(Math.max(0, Math.min(last, index)))
    revealMode.current = 'nearest'
  }

  function typeahead(char: string, now: number) {
    const t = typed.current
    t.text = now - t.at > typeaheadMs ? char : t.text + char
    t.at = now
    const labels = options.map((o) => o.label)
    const hit = findByPrefix(labels, open ? current : shown, t.text)
    if (!open) {
      show(hit >= 0 ? hit : shown)
    } else if (hit >= 0) {
      moveTo(hit)
    }
  }

  function onKeyDown(e: KeyboardEvent<HTMLButtonElement>) {
    const { key } = e
    const now = e.timeStamp
    const typing = typed.current.text !== '' && now - typed.current.at <= typeaheadMs
    const char = key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey
    if (options.length === 0) {
      return
    }
    if (!open) {
      if (key === 'ArrowDown' || key === 'ArrowUp' || key === 'Enter' || (key === ' ' && !typing)) {
        show(shown)
      } else if (key === 'Home' || key === 'End') {
        show(key === 'Home' ? 0 : last)
      } else if (char) {
        typeahead(key, now)
      } else {
        return
      }
    } else if (key === 'Tab') {
      setOpen(false)
      return
    } else if ((key === 'ArrowDown' || key === 'ArrowUp') && e.altKey) {
      commit(current)
    } else if (key === 'ArrowDown' || key === 'ArrowUp') {
      moveTo(current + (key === 'ArrowDown' ? 1 : -1))
    } else if (key === 'PageDown' || key === 'PageUp') {
      moveTo(current + (key === 'PageDown' ? pageStep : -pageStep))
    } else if (key === 'Home' || key === 'End') {
      moveTo(key === 'Home' ? 0 : last)
    } else if (key === 'Enter' || (key === ' ' && !typing)) {
      commit(current)
    } else if (key === 'Escape') {
      setOpen(false)
    } else if (char) {
      typeahead(key, now)
    } else {
      return
    }
    e.preventDefault()
    e.stopPropagation()
  }

  return (
    <>
      <button
        ref={buttonRef}
        id={buttonId}
        type="button"
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listId : undefined}
        aria-activedescendant={open && current >= 0 ? optionId(current) : undefined}
        aria-label={ariaLabel ?? prefix}
        title={abbreviationTitle(options[shown]?.label ?? '')}
        data-value={value}
        className={fitContent ? 'select is-fit-content' : 'select'}
        onClick={() => {
          labelPress.current = false
          if (open) {
            setOpen(false)
          } else {
            show(shown)
          }
        }}
        onKeyDown={onKeyDown}
        onKeyUp={(e) => {
          if (e.key === ' ') {
            e.preventDefault()
          }
        }}
        onBlur={() => {
          if (!labelPress.current) {
            setOpen(false)
          }
        }}
      >
        {prefix ? <span className="select-prefix">{`${prefix}:`}</span> : null}
        <span className="select-text">
          <span className="select-value">{options[shown]?.label ?? ''}</span>
          {fitContent ? null : options.map((o) => (
            <span key={o.value} className="select-ghost" aria-hidden="true">
              {o.label}
            </span>
          ))}
        </span>
        <span className="select-chevron">
          <ChevronDownIcon />
        </span>
      </button>
      {open && host
        ? createPortal(
            <ul
              ref={listRef}
              id={listId}
              role="listbox"
              popover="manual"
              tabIndex={-1}
              aria-label={ariaLabel ?? prefix}
              aria-labelledby={(ariaLabel ?? prefix) ? undefined : buttonId}
              className="select-list"
              onMouseDown={(e) => e.preventDefault()}
            >
              {options.map((o, i) => (
                <li
                  key={o.value}
                  id={optionId(i)}
                  role="option"
                  aria-selected={i === current}
                  data-value={o.value}
                  title={abbreviationTitle(o.label)}
                  className={`select-option${i === current ? ' is-active' : ''}${i === shown ? ' is-selected' : ''}`}
                  onMouseMove={() => {
                    if (i !== current) {
                      setActive(i)
                    }
                  }}
                  onClick={() => commit(i)}
                >
                  <span className="select-label">{o.label}</span>
                </li>
              ))}
            </ul>,
            host,
          )
        : null}
    </>
  )
}
