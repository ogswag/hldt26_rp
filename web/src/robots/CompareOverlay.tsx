import { useId, useLayoutEffect, useMemo, useRef } from 'react'

import type { MatchItem, Solution } from '../api/client'
import { familyOf } from '../catalog/families'
import { FamilyMark } from '../catalog/FamilyMark'
import { useCatalogFields } from '../catalog/fields'
import { cutTitle } from '../ui/cutTitle'
import { CloseIcon } from '../ui/icons'
import { Reflow } from '../ui/Reflow'

import { compareGroups } from './compare'

type Props = {
  robots: readonly Solution[]
  // items are the calculation's verdicts and cases; a page without a calculation passes none.
  items?: readonly MatchItem[]
  onRemove: (id: string) => void
  onClose: () => void
}

// Picked robots side by side, one column each, in the robot overlay frame without the list.
export function CompareOverlay({ robots, items, onRemove, onClose }: Props) {
  const ref = useRef<HTMLDialogElement>(null)
  const uid = useId()
  const fields = useCatalogFields()
  const byId = useMemo(() => (items ? new Map(items.map((i) => [i.solution_id, i])) : null), [items])
  const groups = useMemo(() => compareGroups(robots, byId, fields), [robots, byId, fields])

  useLayoutEffect(() => {
    const d = ref.current
    if (d && !d.open) {
      d.showModal()
      d.querySelector<HTMLElement>('.compare-remove')?.focus()
    }
    return () => {
      if (d?.open) {
        d.close()
      }
    }
  }, [])

  const titleId = `${uid}-title`
  return (
    <dialog
      ref={ref}
      className="robot-overlay compare-overlay"
      aria-labelledby={titleId}
      onCancel={(e) => {
        e.preventDefault()
        onClose()
      }}
    >
      <div className="import-head">
        <h2 id={titleId} className="compare-title">
          Сравнение роботов
        </h2>
        <button type="button" className="icon-button robot-overlay-close" aria-label="Закрыть" title="Закрыть" onClick={onClose}>
          <CloseIcon size={18} />
        </button>
      </div>
      <div className="compare-body">
        {robots.length < 2 ? (
          <p>
            <Reflow>Для сравнения нужны хотя бы два робота. Отметьте ещё одного в списке.</Reflow>
          </p>
        ) : (
          <table className="compare-table">
            <thead>
              <tr>
                <th scope="col">
                  <span className="sr-only">Показатель</span>
                </th>
                {robots.map((s) => (
                  <th key={s.id} scope="col">
                    <div className="compare-robot">
                      <FamilyMark family={familyOf(s.family)} />
                      <span className="compare-robot-name">
                        {s.name}
                        {s.modification ? <span className="robot-modification">{s.modification}</span> : null}
                      </span>
                      <button
                        type="button"
                        className="icon-button compare-remove"
                        aria-label={`Убрать из сравнения: ${s.name}`}
                        title="Убрать из сравнения"
                        onClick={() => onRemove(s.id)}
                      >
                        <CloseIcon size={16} />
                      </button>
                    </div>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {groups.flatMap((g) => [
                <tr key={g.title} className="compare-group">
                  <th scope="colgroup" colSpan={robots.length + 1}>
                    {g.title}
                  </th>
                </tr>,
                ...g.rows.map((r) => (
                  <tr key={`${g.title}:${r.label}`}>
                    <th scope="row">{r.label}</th>
                    {r.cells.map((c, i) => (
                      <td key={robots[i].id}>
                        {c.href ? (
                          <a href={c.href} target="_blank" rel="noopener noreferrer">
                            {c.text}
                          </a>
                        ) : c.tag ? (
                          <span className={`tag ${c.tag}`}>{c.text}</span>
                        ) : r.clamp ? (
                          <div className="clamp-2" onMouseEnter={cutTitle(c.text)}>
                            <Reflow>{c.text}</Reflow>
                          </div>
                        ) : (
                          <Reflow>{c.text}</Reflow>
                        )}
                      </td>
                    ))}
                  </tr>
                )),
              ])}
            </tbody>
          </table>
        )}
      </div>
    </dialog>
  )
}
