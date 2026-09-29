import { useState, type ReactNode } from 'react'

import type { MatchItem, ScoreParts } from '../api/client'
import { formatRub } from '../econ/view'
import { Reflow } from '../ui/Reflow'

import { paybackText } from './format'
import { MatchExplain } from './MatchExplain'

// estimateText says what the robot costs the project on its own: the count, CAPEX and payback of buying it.
function estimateText(item: MatchItem): string {
  const e = item.estimate
  if (!e) {
    return item.price_rub === null ? 'Нет цены в каталоге: количество и окупаемость не посчитаны.' : 'Количество и окупаемость не посчитаны.'
  }
  const payback = e.payback_years === null ? 'не окупается' : `окупаемость ${paybackText(e.payback_years)}`
  return `${e.fleet_size} шт, CAPEX ${formatRub(e.capex_rub)}, ${payback}.`
}

type Props = {
  item: MatchItem | null
  why: string[]
  weights: ScoreParts
  objectType: string
  pick: ReactNode
}

// SuggestionPanel shows the robot the calculation suggests and why it leads the list.
export function SuggestionPanel({ item, why, weights, objectType, pick }: Props) {
  const [open, setOpen] = useState(false)
  return (
    <section className="robot-suggestion" aria-labelledby="robot-suggestion-title">
      <h2 id="robot-suggestion-title">Предложение</h2>
      {item ? (
        <>
          <div className="robot-suggestion-head">
            <h3>{item.name}</h3>
            {pick}
          </div>
          {item.vendor ? <p className="robot-suggestion-vendor">{item.vendor}</p> : null}
          <p>
            <Reflow>{estimateText(item)}</Reflow>
          </p>
          {why.length > 0 ? (
            <ul>
              {why.map((w) => (
                <li key={w}>
                  <Reflow>{w}</Reflow>
                </li>
              ))}
            </ul>
          ) : null}
          <button type="button" className="linkish" aria-expanded={open} onClick={() => setOpen(!open)}>
            {open ? 'Скрыть проверки' : 'Показать проверки'}
          </button>
          {open ? <MatchExplain item={item} weights={weights} objectType={objectType} /> : null}
        </>
      ) : (
        <p>
          <Reflow>Ни один робот из каталога не подходит к объекту. Причины в списке ниже.</Reflow>
        </p>
      )}
    </section>
  )
}
