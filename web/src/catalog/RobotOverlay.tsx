import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'

import { adminArchiveSolution, adminDuplicateSolution, adminRestoreSolution, type Solution } from '../api/client'
import { undoKeys } from '../store/useUndoShortcuts'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { CloseIcon } from '../ui/icons'
import { isTyping } from '../ui/keys'
import { numberText } from '../ui/numberText'
import { Reflow } from '../ui/Reflow'
import { CatalogFilters } from './CatalogFilters'
import { familyOf } from './families'
import { FamilyMark } from './FamilyMark'
import { useCatalogFields } from './fields'
import type { CatalogFilter, Matcher } from './filter'
import { RobotEditor } from './RobotEditor'
import { RobotView } from './RobotView'
import { putSolution } from './useCatalog'

// newRobot stands for the draft of a robot that has no name yet, in place of an id.
export const newRobot = 'new'

type Props = {
  // items is the loaded catalog; list is what the page's filter and order leave of it.
  items: readonly Solution[]
  list: readonly Solution[]
  selected: string
  filter: CatalogFilter
  match: Matcher
  onFilter: (patch: Partial<CatalogFilter>) => void
  onSelect: (id: string) => void
  // onClose gets the robot shown last, so the page can show its row.
  onClose: (last: string | null) => void
  admin?: boolean
}

function errorText(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback
}

// Window-size dialog: the list at the
// left, the picture and description in the centre, the offer and specs at the right. Up and Down move through the
// list the page shows.
export function RobotOverlay(p: Props) {
  const ref = useRef<HTMLDialogElement>(null)
  const uid = useId()
  const qc = useQueryClient()
  const fields = useCatalogFields()
  const listRef = useRef<HTMLElement>(null)
  const [listOpen, setListOpen] = useState(false)
  // born is the robot created from the draft here: its editor stays the draft's, so typing goes on uninterrupted.
  const [born, setBorn] = useState<string | null>(null)
  const [archiving, setArchiving] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const robot = p.items.find((s) => s.id === p.selected) ?? null
  const draft = Boolean(p.admin) && p.selected === newRobot
  const index = p.list.findIndex((s) => s.id === p.selected)
  const optionId = (id: string) => `${uid}-robot-${id}`
  const titleId = `${uid}-title`

  // NOTE: the dialog opens once; later robots show in the same dialog. The focus starts in the list, or in the name
  // of a new robot; the page puts it back when the dialog closes. It opens in a layout effect, before the scroll
  // below, because a closed dialog cannot scroll.
  const firstFocus = useRef(draft ? '.robot-title-input' : '.robot-list')
  useLayoutEffect(() => {
    const d = ref.current
    if (d && !d.open) {
      d.showModal()
      d.querySelector<HTMLElement>(firstFocus.current)?.focus()
    }
    return () => {
      if (d?.open) {
        d.close()
      }
    }
  }, [])

  // NOTE: the list starts on the open robot, once its row exists (a link opens the dialog before the catalog has
  // loaded), and again each time the folded list is shown. A pick inside the list moves it only as far as the row
  // needs.
  const centred = useRef(false)
  const inList = index >= 0
  useLayoutEffect(() => {
    const row = document.getElementById(`${uid}-robot-${p.selected}`)
    if (row) {
      row.scrollIntoView({ block: centred.current ? 'nearest' : 'center' })
      centred.current = true
    }
  }, [uid, p.selected, inList, listOpen])

  // NOTE: the folded list covers its own button, so a press anywhere outside it closes it. A Select in the filter
  // panel portals its list to the body; picking from it is not a press outside.
  useEffect(() => {
    if (!listOpen) {
      return
    }
    const onDown = (e: PointerEvent) => {
      const target = e.target as Element
      if (!listRef.current?.contains(target) && !target.closest('.robot-list-toggle, .select-list')) {
        setListOpen(false)
      }
    }
    window.addEventListener('pointerdown', onDown)
    return () => window.removeEventListener('pointerdown', onDown)
  }, [listOpen])

  const select = (id: string) => {
    setError('')
    setListOpen(false)
    p.onSelect(id)
  }

  // close first takes the focus out of a field, so an edit typed in the last 600 ms still saves.
  const close = () => {
    if (document.activeElement instanceof HTMLElement && ref.current?.contains(document.activeElement)) {
      document.activeElement.blur()
    }
    p.onClose(robot?.id ?? null)
  }

  const onKeyDown = (e: KeyboardEvent<HTMLDialogElement>) => {
    if (isTyping(e.target) || e.altKey || e.metaKey || e.ctrlKey || p.list.length === 0) {
      return
    }
    const last = p.list.length - 1
    let next: number
    switch (e.key) {
      case 'ArrowDown':
        next = index < 0 ? 0 : Math.min(index + 1, last)
        break
      case 'ArrowUp':
        next = index < 0 ? 0 : Math.max(index - 1, 0)
        break
      case 'Home':
        next = 0
        break
      case 'End':
        next = last
        break
      default:
        return
    }
    e.preventDefault()
    select(p.list[next].id)
  }

  const act = (run: () => Promise<Solution>, fallback: string, after?: (s: Solution) => void) => {
    setBusy(true)
    setError('')
    run()
      .then((s) => {
        putSolution(qc, s)
        after?.(s)
      })
      .catch((err: unknown) => setError(errorText(err, fallback)))
      .finally(() => setBusy(false))
  }

  const archive = (s: Solution) =>
    act(
      async () => {
        await adminArchiveSolution(s.id)
        return { ...s, archived_at: new Date().toISOString() }
      },
      'Не удалось убрать в архив. Повторите.',
      () => setArchiving(false),
    )

  const count = index >= 0 ? `${numberText(index + 1)} из ${numberText(p.list.length)}` : `В списке: ${numberText(p.list.length)}`

  return (
    <dialog
      ref={ref}
      className={p.admin ? 'robot-overlay is-admin' : 'robot-overlay'}
      aria-labelledby={titleId}
      onKeyDown={onKeyDown}
      onCancel={(e) => {
        e.preventDefault()
        close()
      }}
    >
      <div className={listOpen ? 'robot-overlay-body is-list-open' : 'robot-overlay-body'}>
        <aside ref={listRef} className="robot-overlay-list" aria-label="Список роботов">
          <CatalogFilters items={p.items} filter={p.filter} match={p.match} onChange={p.onFilter} admin={p.admin} compact />
          {p.list.length === 0 ? (
            <p className="robot-list-empty">
              <Reflow>Под фильтр не подходит ни один робот. Измените фильтр.</Reflow>
            </p>
          ) : (
            <ul
              className="robot-list"
              role="listbox"
              aria-label="Роботы"
              tabIndex={0}
              aria-activedescendant={index >= 0 ? optionId(p.list[index].id) : undefined}
            >
              {p.list.map((s) => (
                <li
                  key={s.id}
                  id={optionId(s.id)}
                  role="option"
                  aria-selected={s.id === p.selected}
                  className="robot-list-item"
                  onClick={() => select(s.id)}
                >
                  <FamilyMark family={familyOf(s.family)} />
                  <span className="robot-list-name">{s.name}</span>
                </li>
              ))}
            </ul>
          )}
        </aside>
        <div className="robot-detail">
          <div className="robot-detail-tools">
            <button
              type="button"
              className="btn robot-list-toggle"
              aria-expanded={listOpen}
              onClick={() => {
                centred.current = false
                setListOpen((v) => !v)
              }}
            >
              Список
            </button>
            <button type="button" className="icon-button robot-overlay-close" aria-label="Закрыть" title="Закрыть" onClick={close}>
              <CloseIcon size={18} />
            </button>
          </div>
          <div className="robot-detail-body">
            {p.admin && (robot || draft) ? (
              <RobotEditor
                key={robot && robot.id !== born ? robot.id : newRobot}
                s={robot}
                fields={fields}
                titleId={titleId}
                onCreated={(s) => {
                  setBorn(s.id)
                  p.onSelect(s.id)
                }}
              />
            ) : robot ? (
              <RobotView s={robot} fields={fields} titleId={titleId} />
            ) : (
              <section className="robot-main">
                <h2 id={titleId} className="robot-title">
                  Робот не найден
                </h2>
                <p>
                  <Reflow>Этого робота нет в каталоге: его убрали в архив или ссылка неполная. Выберите робота в списке.</Reflow>
                </p>
              </section>
            )}
          </div>
        </div>
      </div>
      <p className="sr-only" aria-live="polite">
        {robot?.name ?? ''}
      </p>
      {error ? (
        <p className="error robot-overlay-error" role="alert">
          <Reflow>{error}</Reflow>
        </p>
      ) : null}
      <div className="robot-overlay-foot">
        <p className="robot-keys">
          <span>
            <kbd>Вверх</kbd> <kbd>Вниз</kbd> соседний робот
          </span>
          <span>
            <kbd>Esc</kbd> закрыть
          </span>
          {p.admin ? (
            <span>
              <kbd>{undoKeys}</kbd> отменить правку
            </span>
          ) : null}
        </p>
        <p className="robot-count">{count}</p>
        <div className="robot-actions">
          {robot?.source_url ? (
            <a className="btn" href={robot.source_url} target="_blank" rel="noopener noreferrer">
              Источник
            </a>
          ) : null}
          {p.admin && robot ? (
            <>
              <button
                type="button"
                className="btn"
                disabled={busy}
                onClick={() => act(() => adminDuplicateSolution(robot.id), 'Не удалось создать копию. Повторите.', (s) => select(s.id))}
              >
                Дублировать
              </button>
              {robot.archived_at ? (
                <button
                  type="button"
                  className="btn"
                  disabled={busy}
                  onClick={() => act(() => adminRestoreSolution(robot.id), 'Не удалось вернуть из архива. Повторите.')}
                >
                  Вернуть из архива
                </button>
              ) : (
                <button type="button" className="btn btn-danger" disabled={busy} onClick={() => setArchiving(true)}>
                  В архив
                </button>
              )}
            </>
          ) : null}
        </div>
      </div>
      <ConfirmDialog
        open={archiving}
        title="Убрать робота в архив?"
        confirmLabel="В архив"
        cancelLabel="Отмена"
        busy={busy}
        error={archiving && error ? error : undefined}
        onConfirm={() => robot && archive(robot)}
        onCancel={() => setArchiving(false)}
      >
        <p>
          <Reflow>{`«${robot?.name ?? ''}» пропадёт из каталога и из подбора. Расчёты, в которых он уже есть, считаются как прежде. Вернуть его можно из архива.`}</Reflow>
        </p>
      </ConfirmDialog>
    </dialog>
  )
}
