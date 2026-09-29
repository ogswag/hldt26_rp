import { useId, useState, type ReactNode } from 'react'

import { words } from '../search/match'
import { numberText } from './numberText'

// icon and count are for filter lists: a group's mark before the label, the number of matching items after it.
export type CheckOption = { value: string; label: string; icon?: ReactNode; count?: number }

type Props = {
  label: string
  options: readonly CheckOption[]
  value: readonly string[]
  onChange: (value: string[]) => void
  empty: string
  // plain drops the box and the «Выбрано» line, for a list that is a section of a panel.
  plain?: boolean
  // find is the name of a box above a long list; it shows the options whose words start with what is typed.
  find?: string
}

// hit tells whether every typed word starts a word of the label.
function hit(label: string, typed: readonly string[]): boolean {
  const w = words(label)
  return typed.every((t) => w.some((x) => x.startsWith(t)))
}

export function CheckList({ label, options, value, onChange, empty, plain, find }: Props) {
  const labelId = useId()
  const [typed, setTyped] = useState('')
  const chosen = new Set(value)
  const count = options.filter((o) => chosen.has(o.value)).length
  const query = words(typed)
  // NOTE: toggle works on every option, so a ticked one the find box hides stays ticked.
  const shown = query.length > 0 ? options.filter((o) => hit(o.label, query)) : options

  function toggle(v: string) {
    const next = new Set(chosen)
    if (next.has(v)) {
      next.delete(v)
    } else {
      next.add(v)
    }
    onChange(options.filter((o) => next.has(o.value)).map((o) => o.value))
  }

  return (
    <div className={plain ? 'check-list is-plain' : 'check-list'} role="group" aria-labelledby={labelId}>
      <span id={labelId} className="check-list-label">
        {label}
      </span>
      {options.length === 0 ? (
        <p className="field-hint">{empty}</p>
      ) : (
        <>
          {find ? (
            <input
              type="search"
              className="check-list-find"
              autoComplete="off"
              placeholder={find}
              aria-label={find}
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
            />
          ) : null}
          <div className={find ? 'check-list-box is-scroll' : 'check-list-box'}>
            {shown.map((o) => (
              <label key={o.value} className="check-list-item">
                <input type="checkbox" checked={chosen.has(o.value)} onChange={() => toggle(o.value)} />
                {o.icon}
                <span>{o.label}</span>
                {o.count !== undefined ? <span className="check-list-n">{numberText(o.count)}</span> : null}
              </label>
            ))}
            {shown.length === 0 ? <p className="field-hint check-list-none">Ничего не найдено.</p> : null}
          </div>
          {plain ? null : <span className="check-list-count">Выбрано: {count}</span>}
        </>
      )}
    </div>
  )
}
