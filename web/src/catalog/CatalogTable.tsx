import type { ReactNode } from 'react'

import type { Solution } from '../api/client'
import { formatRub, missingSpecs } from '../econ/view'
import { AbbreviationText } from '../ui/Abbreviation'
import { cutTitle } from '../ui/cutTitle'
import { HoverCard } from '../ui/HoverCard'
import { CompareCheck, type CompareSelect } from './CompareCheck'
import { familyOf } from './families'
import { FamilyMark } from './FamilyMark'
import { openRow } from './openRow'
import { RobotHoverCard } from './RobotHoverCard'
import { statusText } from './values'

// RobotName is a robot's name cell: the group mark and the name on one line, cut with «...» when long, then the
// industry of a repeated catalog row and the tags. The name opens the robot and shows the hover card.
export function RobotName({ s, onOpen, children }: { s: Solution; onOpen: (s: Solution) => void; children?: ReactNode }) {
  return (
    <div className="robot-name-cell">
      <HoverCard card={() => <RobotHoverCard s={s} />} className="robot-name-anchor">
        <button type="button" className="robot-name" onClick={() => onOpen(s)}>
          <FamilyMark family={familyOf(s.family)} />
          <span className="robot-name-text">{s.name}</span>
        </button>
      </HoverCard>
      {s.modification ? <span className="robot-modification">{s.modification}</span> : null}
      {children}
    </div>
  )
}

type Props = {
  rows: readonly Solution[]
  onOpen: (s: Solution) => void
  // inCalc marks the robots the project's calculation runs on.
  inCalc?: ReadonlySet<string>
  // admin adds the specs gaps and the archive mark.
  admin?: boolean
  // compare adds a column of boxes that pick robots for the comparison.
  compare?: CompareSelect
}

// Lists catalog robots, one line per robot. Filters add and drop rows, so the columns keep their
// widths; the group mark stands for the type.
export function CatalogTable({ rows, onOpen, inCalc, admin, compare }: Props) {
  return (
    <div className="table-wrap">
      <table className={admin ? 'catalog-table is-admin' : 'catalog-table'}>
        <colgroup>
          {compare ? <col className="is-check" /> : null}
          <col />
          <col className="is-vendor" />
          <col className="is-subtype" />
          <col className="is-status" />
          {admin ? <col className="is-gaps" /> : null}
          <col className="is-money" />
        </colgroup>
        <thead>
          <tr>
            {compare ? (
              <th>
                <span className="sr-only">Сравнение</span>
              </th>
            ) : null}
            <th>Название</th>
            <th>Компания</th>
            <th>Подтип</th>
            <th>Статус</th>
            {admin ? (
              <th>
                Нет в <AbbreviationText text="ТТХ" />
              </th>
            ) : null}
            <th className="num">Цена</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((s) => {
            const status = statusText(s)
            return (
              <tr key={s.id} data-search-id={`robot:${s.id}`} className="is-openable" onClick={(e) => openRow(e, () => onOpen(s))}>
                {compare ? (
                  <td>
                    <CompareCheck id={s.id} name={s.name} select={compare} />
                  </td>
                ) : null}
                <td>
                  <RobotName s={s} onOpen={onOpen}>
                    {inCalc?.has(s.id) ? <span className="tag tag-info">в расчёте</span> : null}
                    {admin && s.archived_at ? <span className="tag">в архиве</span> : null}
                  </RobotName>
                </td>
                <td className="cut-cell" onMouseEnter={cutTitle(s.vendor ?? '')}>
                  {s.vendor ?? 'нет данных'}
                </td>
                <td className="cut-cell" onMouseEnter={cutTitle(s.subtype ?? '')}>
                  {s.subtype ?? 'нет данных'}
                </td>
                <td className="cut-cell">{status ? <span className="tag">{status}</span> : 'нет данных'}</td>
                {admin ? (
                  <td className="cut-cell" onMouseEnter={cutTitle(missingSpecs(s.data_quality.missing))}>
                    {missingSpecs(s.data_quality.missing)}
                  </td>
                ) : null}
                <td className="num">{formatRub(s.price_rub)}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
