import { Fragment, useState, type ReactNode } from 'react'

import type { ScoreParts, Solution } from '../api/client'
import { RobotName } from '../catalog/CatalogTable'
import { CompareCheck, type CompareSelect } from '../catalog/CompareCheck'
import { openRow } from '../catalog/openRow'
import { formatRub } from '../econ/view'
import { cutTitle } from '../ui/cutTitle'
import { Reflow } from '../ui/Reflow'

import { stepsOf } from './explain'
import { estimatePayback, verdicts } from './format'
import { MatchExplain } from './MatchExplain'
import type { RankedRow } from './ranking'

type Props = {
  rows: RankedRow[]
  weights: ScoreParts
  objectType: string
  // inCalc marks the robots the calculation runs on: the variants' robots, or the suggestion while they are empty.
  inCalc: ReadonlySet<string>
  pick?: (row: RankedRow) => ReactNode
  compare?: CompareSelect
  onOpen: (s: Solution) => void
}

// RankedTable lists the catalog robots with the calculation's verdict and each robot's own buy case.
export function RankedTable({ rows, weights, objectType, inCalc, pick, compare, onOpen }: Props) {
  const [shown, setShown] = useState<ReadonlySet<string>>(new Set())
  const flip = (id: string) =>
    setShown((prev) => {
      const next = new Set(prev)
      if (!next.delete(id)) {
        next.add(id)
      }
      return next
    })
  const columns = 7 + (pick ? 1 : 0) + (compare ? 1 : 0)
  return (
    <div className="table-wrap">
      <table className="ranked-table">
        <colgroup>
          {compare ? <col className="is-check" /> : null}
          <col className="is-name" />
          <col className="is-verdict" />
          <col />
          <col className="is-count" />
          <col className="is-money" />
          <col className="is-years" />
          <col className="is-money" />
          {pick ? <col className="is-pick" /> : null}
        </colgroup>
        <thead>
          <tr>
            {compare ? (
              <th>
                <span className="sr-only">Сравнение</span>
              </th>
            ) : null}
            <th>Название</th>
            <th>Оценка</th>
            <th>Почему</th>
            <th className="num">Флот, шт</th>
            <th className="num">CAPEX</th>
            <th className="num">Окупаемость</th>
            <th className="num">Цена</th>
            {pick ? (
              <th>
                <span className="sr-only">Выбор</span>
              </th>
            ) : null}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const { solution: s, item } = row
            const verdict = item ? verdicts[item.status] : undefined
            const why = item?.reasons.join(' ') ?? ''
            const explainable = item !== null && stepsOf(item).length > 0
            const expanded = explainable && shown.has(s.id)
            return (
              <Fragment key={s.id}>
                <tr data-search-id={`robot:${s.id}`} className="is-openable" onClick={(e) => openRow(e, () => onOpen(s))}>
                  {compare ? (
                    <td>
                      <CompareCheck id={s.id} name={s.name} select={compare} />
                    </td>
                  ) : null}
                  <td>
                    <RobotName s={s} onOpen={onOpen}>
                      {inCalc.has(s.id) ? <span className="tag tag-info">в расчёте</span> : null}
                    </RobotName>
                    {s.vendor ? (
                      <div className="cell-note cut-line" onMouseEnter={cutTitle(s.vendor)}>
                        {s.vendor}
                      </div>
                    ) : null}
                  </td>
                  <td>{verdict ? <span className={`tag ${verdict.tag}`}>{verdict.label}</span> : 'нет данных'}</td>
                  <td>
                    <div className="clamp-2" onMouseEnter={cutTitle(why)}>
                      <Reflow>{why}</Reflow>
                    </div>
                    {explainable ? (
                      <button type="button" className="linkish" aria-expanded={expanded} onClick={() => flip(s.id)}>
                        {expanded ? 'Скрыть проверки' : 'Показать проверки'}
                      </button>
                    ) : null}
                  </td>
                  <td className="num">{item?.estimate ? item.estimate.fleet_size : ''}</td>
                  <td className="num">{item?.estimate ? formatRub(item.estimate.capex_rub) : ''}</td>
                  <td className="num">{estimatePayback(item)}</td>
                  <td className="num">{formatRub(s.price_rub)}</td>
                  {pick ? <td>{pick(row)}</td> : null}
                </tr>
                {expanded && item ? (
                  <tr className="match-explain-row">
                    <td colSpan={columns}>
                      <MatchExplain item={item} weights={weights} objectType={objectType} />
                    </td>
                  </tr>
                ) : null}
              </Fragment>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
