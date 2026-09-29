import { useQuery } from '@tanstack/react-query'
import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router-dom'

import { fetchSolutions, type ObjectSchema, type ObjectType } from '../api/client'
import { kindLabels } from '../catalog/labels'
import { formatRub } from '../econ/view'
import { tabHref } from '../layout/nav'
import { CloseIcon, SearchIcon } from '../ui/icons'
import { commandKey, shortcut } from '../ui/keys'
import { placeList, popoverScale, popoverSize, viewportSize } from '../ui/listbox'
import { focusIn, nextFrame, reveal, waitFor } from '../ui/reveal'
import { jumpable, type SearchEntry } from './entries'
import { prepare, rank } from './match'
import { Preview } from './Preview'
import { loadRecent, saveRecent } from './recent'
import type { Access } from '../layout/access'
import { setSearchTarget } from './target'
import { useSearchIndex } from './useSearchIndex'

type Props = {
  storeKey: string
  base: string
  projectId: string | null
  demo: ObjectType | null
  objectType: ObjectType | null
  schema: ObjectSchema | null
  withMap: boolean
  navAccess: Record<string, Access>
  readOnly: boolean
}

// NOTE: arrows move the list at once and the page 250 ms after the last press, so holding a key does not load
// every page on the way.
const jumpDelayMs = 250
const robotDelayMs = 200
const projectLimit = 12
const robotLimit = 5
const pageStep = 5
const gap = 4
const margin = 8

type Session = {
  // pushed: the search already added its one history entry; later jumps replace it.
  pushed: boolean
  navigated: boolean
  timer: number
  pending: SearchEntry | null
  shown: SearchEntry | null
  abort: AbortController | null
  open: boolean
}

function useRobotEntries(open: boolean, query: string, base: string, objectType: ObjectType | null): SearchEntry[] {
  const [q, setQ] = useState('')
  useEffect(() => {
    const t = setTimeout(() => setQ(query.trim()), robotDelayMs)
    return () => clearTimeout(t)
  }, [query])
  const enabled = open && base !== '' && q.length >= 2
  const robotsQ = useQuery({
    queryKey: ['search-robots', q, objectType],
    queryFn: () => fetchSolutions({ q, object_type: objectType ?? undefined, limit: robotLimit }),
    enabled,
    staleTime: 60_000,
  })
  const current = q === query.trim()
  return useMemo(() => {
    if (!enabled || !current) {
      return []
    }
    return (robotsQ.data?.items ?? []).slice(0, robotLimit).map((s, i) => {
      const about = [s.vendor, s.kind ? (kindLabels[s.kind] ?? s.kind) : null].filter(Boolean).join(', ')
      return {
        id: `robot:${s.id}`,
        kind: 'robot' as const,
        label: s.name,
        path: ['Роботы'],
        note: about,
        value: s.price_rub != null ? formatRub(s.price_rub) : undefined,
        to: tabHref(base, 'robots'),
        hint: `${about ? `${about}. ` : ''}Цена: ${formatRub(s.price_rub)}.`,
        order: 100_000 + i,
      }
    })
  }, [enabled, current, robotsQ.data, base])
}

// focusHeading puts the focus on the page's heading after a jump to a whole page, so a screen reader starts there.
async function focusHeading(signal: AbortSignal): Promise<void> {
  await nextFrame()
  await nextFrame()
  if (signal.aborted) {
    return
  }
  const h = document.querySelector<HTMLElement>('main h1') ?? document.querySelector<HTMLElement>('main')
  if (h) {
    h.tabIndex = -1
    h.focus({ preventScroll: true })
  }
  window.scrollTo({ top: 0 })
}

// press clicks the page's own button for a command once the page shows it; a button that cannot act now is
// revealed instead, so its page says why.
async function press(searchId: string, signal: AbortSignal): Promise<void> {
  await nextFrame()
  const el = await waitFor(searchId, signal)
  if (!el || signal.aborted) {
    return
  }
  if (el instanceof HTMLButtonElement && !el.disabled) {
    el.click()
    return
  }
  el.scrollIntoView({ block: 'center' })
  focusIn(el)
}

// Finds anything in the project and takes the user there.
export function SearchBox(p: Props) {
  const uid = useId()
  const listId = `${uid}-list`
  const navigate = useNavigate()
  const fieldRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLUListElement>(null)
  const previewRef = useRef<HTMLDivElement>(null)
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)
  const [focused, setFocused] = useState(false)
  const [phoneOpen, setPhoneOpen] = useState(false)
  const [active, setActive] = useState(-1)
  const [hover, setHover] = useState<number | null>(null)
  const session = useRef<Session>({ pushed: false, navigated: false, timer: 0, pending: null, shown: null, abort: null, open: false })
  const scope = p.storeKey

  const indexInput = useMemo(
    () => ({
      open,
      storeKey: p.storeKey,
      base: p.base,
      projectId: p.projectId,
      demo: p.demo,
      schema: p.schema,
      withMap: p.withMap,
      navAccess: p.navAccess,
      readOnly: p.readOnly,
    }),
    [open, p.storeKey, p.base, p.projectId, p.demo, p.schema, p.withMap, p.navAccess, p.readOnly],
  )
  const { entries, mapDoc } = useSearchIndex(indexInput)
  const prepared = useMemo(() => prepare(entries), [entries])
  const robots = useRobotEntries(open, query, p.base, p.objectType)
  const blank = query.trim() === ''
  const results = useMemo(() => {
    if (!open) {
      return []
    }
    if (blank) {
      const byId = new Map(entries.map((e) => [e.id, e]))
      return loadRecent(scope)
        .map((id) => byId.get(id))
        .filter((e): e is SearchEntry => e !== undefined)
    }
    return [...rank(prepared, query, window.location.pathname, projectLimit), ...robots]
  }, [open, blank, entries, prepared, query, robots, scope])

  const showList = open && (results.length > 0 || !blank)
  const canHover = typeof window !== 'undefined' && window.matchMedia('(hover: hover) and (pointer: fine)').matches
  const looked = hover !== null ? results[hover] : active >= 0 && results[active] && !jumpable(results[active]) ? results[active] : undefined
  const previewEntry = showList && canHover && !phoneOpen ? looked : undefined

  // go shows an entry on the page: it opens the entry's address and reveals it. The first jump of a search adds
  // one history entry, the others replace it, so Back returns to where the search started.
  function go(entry: SearchEntry, phase: 'preview' | 'confirmed', replace: boolean): boolean {
    const s = session.current
    let moved = false
    if (entry.to && entry.to !== window.location.pathname) {
      navigate(entry.to, { replace })
      moved = true
    }
    s.shown = entry
    setSearchTarget(entry, phase)
    s.abort?.abort()
    const abort = new AbortController()
    s.abort = abort
    const confirmed = phase === 'confirmed'
    if (entry.kind === 'tab' || entry.kind === 'project') {
      if (confirmed) {
        void focusHeading(abort.signal)
      }
    } else if (entry.click) {
      if (confirmed) {
        void press(entry.click, abort.signal)
      }
    } else {
      const at = confirmed ? (entry.focusAt ?? entry.id) : (entry.anchor ?? entry.id)
      void reveal(at, { focus: confirmed, signal: abort.signal })
    }
    return moved
  }

  function reset() {
    const s = session.current
    clearTimeout(s.timer)
    s.pending = null
    s.shown = null
    s.navigated = false
    s.pushed = false
    s.open = false
    setOpen(false)
    setActive(-1)
    setHover(null)
  }

  function clear() {
    reset()
    setQuery('')
    session.current.open = true
    setOpen(true)
    inputRef.current?.focus()
  }

  // confirm ends the search on an entry. run is false when the user only left the field: a command then waits.
  function confirm(entry: SearchEntry, run: boolean) {
    const replace = session.current.pushed
    saveRecent(scope, entry.id)
    reset()
    setQuery('')
    setPhoneOpen(false)
    if (entry.run) {
      if (run) {
        entry.run()
      }
      return
    }
    if (entry.click && !run) {
      return
    }
    if (entry.closed) {
      if (run && entry.to) {
        navigate(entry.to, { replace })
      }
      return
    }
    go(entry, 'confirmed', replace)
  }

  // leave ends the search without Enter: what the arrows showed stays, and focus goes to it.
  function leave(focus: boolean) {
    const s = session.current
    const target = s.pending && jumpable(s.pending) ? s.pending : s.shown
    if (target && s.navigated && focus) {
      confirm(target, false)
      return
    }
    if (target && s.navigated && s.pending === target) {
      go(target, 'preview', s.pushed)
    }
    if (target && s.navigated) {
      saveRecent(scope, target.id)
    }
    reset()
    setPhoneOpen(false)
  }

  function move(to: number) {
    if (results.length === 0) {
      return
    }
    const i = Math.max(0, Math.min(results.length - 1, to))
    const s = session.current
    setActive(i)
    setHover(null)
    s.navigated = true
    const entry = results[i]
    listRef.current?.children.item(i)?.scrollIntoView({ block: 'nearest' })
    clearTimeout(s.timer)
    s.pending = entry
    s.timer = window.setTimeout(() => {
      s.pending = null
      if (jumpable(entry)) {
        if (go(entry, 'preview', s.pushed)) {
          s.pushed = true
        }
      }
    }, jumpDelayMs)
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    const s = session.current
    switch (e.key) {
      case 'ArrowDown':
      case 'ArrowUp':
        if (!s.open) {
          s.open = true
          setOpen(true)
        }
        move(active < 0 ? (e.key === 'ArrowDown' ? 0 : results.length - 1) : active + (e.key === 'ArrowDown' ? 1 : -1))
        break
      case 'PageDown':
      case 'PageUp':
        if (!showList) {
          return
        }
        move(Math.max(0, active) + (e.key === 'PageDown' ? pageStep : -pageStep))
        break
      case 'Enter': {
        const entry = results[active >= 0 ? active : 0]
        if (!showList || !entry) {
          return
        }
        confirm(entry, true)
        break
      }
      case 'Escape':
        // NOTE: the map and the dialogs close on Escape too; the search takes this one.
        e.stopPropagation()
        if (showList && s.navigated) {
          leave(true)
        } else if (showList || query) {
          reset()
          setQuery('')
        } else {
          setPhoneOpen(false)
          inputRef.current?.blur()
        }
        break
      case 'Tab':
        if (!showList || !s.navigated) {
          return
        }
        leave(true)
        break
      default:
        return
    }
    e.preventDefault()
  }

  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (!commandKey(e) || e.shiftKey || e.altKey || (e.code !== 'KeyK' && e.key.toLowerCase() !== 'k')) {
        return
      }
      e.preventDefault()
      const input = inputRef.current
      // NOTE: on a narrow window the field is hidden until the magnifier opens it.
      if (input && input.offsetParent !== null) {
        input.focus()
        input.select()
      } else {
        setPhoneOpen(true)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  useEffect(() => {
    if (phoneOpen) {
      inputRef.current?.focus()
    }
  }, [phoneOpen])

  useEffect(() => () => clearTimeout(session.current.timer), [])

  // NOTE: the preview opens before the list is placed, so the placing can measure it.
  useLayoutEffect(() => {
    const pv = previewRef.current
    if (pv && !pv.matches(':popover-open')) {
      pv.showPopover()
    }
  })

  useLayoutEffect(() => {
    const list = listRef.current
    const field = fieldRef.current
    if (!showList || !list || !field) {
      return
    }
    if (!list.matches(':popover-open')) {
      list.showPopover()
    }
    const place = () => {
      const r = field.getBoundingClientRect()
      const scale = popoverScale(list)
      list.style.maxHeight = ''
      const size = popoverSize(list)
      const at = placeList({ left: r.right - size.width, top: r.top, bottom: r.bottom }, size, viewportSize(), gap, margin)
      list.style.left = `${at.left / scale}px`
      list.style.top = `${at.top / scale}px`
      list.style.maxHeight = `${at.maxHeight / scale}px`
      const pv = previewRef.current
      if (pv) {
        const previewScale = popoverScale(pv)
        const left = at.left - gap - popoverSize(pv).width
        pv.style.left = `${left / previewScale}px`
        pv.style.top = `${at.top / previewScale}px`
        pv.style.visibility = left >= margin ? '' : 'hidden'
      }
    }
    place()
    window.addEventListener('resize', place)
    return () => window.removeEventListener('resize', place)
  })

  const announce = !open || blank ? '' : results.length > 0 ? `Найдено: ${results.length}` : 'Ничего не найдено'
  const optionId = (i: number) => `${uid}-option-${i}`

  return (
    <>
      <div className="search" ref={fieldRef} data-open={phoneOpen ? '' : undefined}>
        <SearchIcon size={16} />
        <input
          ref={inputRef}
          type="search"
          role="combobox"
          aria-label="Поиск по проекту"
          aria-autocomplete="list"
          aria-expanded={showList}
          aria-controls={showList ? listId : undefined}
          aria-activedescendant={showList && active >= 0 ? optionId(active) : undefined}
          placeholder="Поиск"
          autoComplete="off"
          spellCheck={false}
          value={query}
          onChange={(e) => {
            clearTimeout(session.current.timer)
            session.current.pending = null
            session.current.open = true
            setQuery(e.target.value)
            setOpen(true)
            setActive(-1)
            setHover(null)
          }}
          onFocus={() => {
            setFocused(true)
            session.current.open = true
            setOpen(true)
          }}
          onBlur={() => {
            setFocused(false)
            // NOTE: switching to another window keeps the search as it was.
            if (!document.hasFocus() || !session.current.open) {
              return
            }
            leave(false)
          }}
          onKeyDown={onKeyDown}
        />
        {!focused && !query ? (
          <kbd className="search-kbd" aria-hidden="true">
            {shortcut('K')}
          </kbd>
        ) : null}
        {query ? (
          <button
            type="button"
            className="icon-button search-clear"
            aria-label="Очистить поиск"
            title="Очистить поиск"
            onMouseDown={(e) => e.preventDefault()}
            onClick={clear}
          >
            <CloseIcon size={16} />
          </button>
        ) : null}
        <button
          type="button"
          className="icon-button search-close"
          aria-label="Закрыть поиск"
          title="Закрыть поиск"
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => {
            reset()
            setQuery('')
            setPhoneOpen(false)
          }}
        >
          <CloseIcon size={16} />
        </button>
        <span className="sr-only" aria-live="polite">
          {announce}
        </span>
      </div>
      <button type="button" className="icon-button search-open" aria-label="Поиск" title="Поиск" onClick={() => setPhoneOpen(true)}>
        <SearchIcon size={20} />
      </button>
      {showList
        ? createPortal(
            <ul
              ref={listRef}
              id={listId}
              role="listbox"
              popover="manual"
              aria-label="Результаты поиска"
              className="select-list search-list"
              onMouseDown={(e) => e.preventDefault()}
              onMouseLeave={() => setHover(null)}
            >
              {results.length === 0 ? (
                <li className="search-empty" role="presentation">
                  Ничего не найдено
                </li>
              ) : null}
              {results.map((r, i) => (
                <li
                  key={r.id}
                  id={optionId(i)}
                  role="option"
                  aria-selected={i === active}
                  aria-disabled={r.closed ? true : undefined}
                  className={`select-option search-option${i === active ? ' is-active' : ''}${r.closed ? ' is-closed' : ''}`}
                  onMouseMove={() => {
                    if (hover !== i) {
                      setHover(i)
                    }
                  }}
                  onClick={() => confirm(r, true)}
                >
                  <span className="search-option-label">{r.label}</span>
                  <span className="search-path">
                    {[r.path.join(' / '), r.closed ? 'вкладка закрыта' : r.kind === 'field' || r.kind === 'processField' ? r.value : undefined]
                      .filter(Boolean)
                      .join(', ')}
                  </span>
                </li>
              ))}
            </ul>,
            document.body,
          )
        : null}
      {previewEntry
        ? createPortal(
            <div ref={previewRef} popover="manual" className="search-preview" aria-hidden="true">
              <Preview entry={previewEntry} index={entries} mapDoc={mapDoc} />
            </div>,
            document.body,
          )
        : null}
    </>
  )
}
