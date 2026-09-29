import type { ReactNode } from 'react'

import type { CatalogField, Solution } from '../api/client'
import { formatRub } from '../econ/view'
import { noData } from '../ui/noData'
import { Reflow } from '../ui/Reflow'
import { valueText } from './fields'
import { RobotPicture } from './RobotPicture'
import { SourceButton } from './SourceButton'
import { solutionValue, sourceHost, statusText } from './values'

export function Rows({ rows }: { rows: [string, ReactNode][] }) {
  return (
    <dl className="robot-rows">
      {rows.map(([k, v]) => (
        <div key={k}>
          <dt>{k}</dt>
          <dd>{v}</dd>
        </div>
      ))}
    </dl>
  )
}

// RobotMeta is the line under the title, over the thick rule: company and subtype at the left, the status tags at
// the right.
export function RobotMeta({ s }: { s: Solution }) {
  const status = statusText(s)
  const meta = [s.vendor, s.subtype].filter(Boolean).join(', ')
  return (
    <div className="robot-meta">
      <p>{meta ? <Reflow>{meta}</Reflow> : null}</p>
      {status || s.archived_at ? (
        <p className="robot-meta-tags">
          {status ? <span className="tag">{status}</span> : null}
          {s.archived_at ? <span className="tag">в архиве</span> : null}
        </p>
      ) : null}
    </div>
  )
}

function known(v: unknown): boolean {
  return v !== null && v !== undefined && v !== '' && !(Array.isArray(v) && v.length === 0)
}

// aboutCodes are the robot's other facts; the overlay lists those that are known.
const aboutCodes = ['family', 'kind', 'industry', 'scenario', 'region', 'ugt', 'market']

type Props = {
  s: Solution
  fields: readonly CatalogField[]
  titleId: string
}

// Read-only robot overlay: title, meta line, picture and description in the centre; the offer,
// the specs and the other known facts at the right.
export function RobotView({ s, fields, titleId }: Props) {
  const field = (code: string) => fields.find((f) => f.code === code)
  const text = (code: string) => valueText(field(code), solutionValue(s, code), noData)
  const priceField = field('price_rub')
  const priceRow: ReactNode =
    priceField && s.price_rub !== null ? (
      <>
        {formatRub(s.price_rub)}
        <SourceButton s={s} field={priceField} fields={fields} />
      </>
    ) : (
      formatRub(s.price_rub)
    )
  const offer: [string, ReactNode][] = [
    ['Цена', priceRow],
    [
      'Источник',
      s.source_url ? (
        <a key="source" href={s.source_url} target="_blank" rel="noopener noreferrer">
          {sourceHost(s.source_url)}
        </a>
      ) : (
        noData
      ),
    ],
    ['Дата источника', text('sourced_at')],
    ['Источник ТТХ', text('confidence')],
    ['Объекты', text('object_types')],
  ]
  const specs = fields.filter((f) => f.group === 'specs')
  const about = aboutCodes
    .map(field)
    .filter((f): f is CatalogField => f !== undefined && known(solutionValue(s, f.code)))

  return (
    <>
      <section className="robot-main" aria-labelledby={titleId}>
        <h2 id={titleId} className="robot-title">
          {s.name}
        </h2>
        <RobotMeta s={s} />
        <RobotPicture s={s} />
        {s.description ? (
          <p className="robot-description">
            <Reflow>{s.description}</Reflow>
          </p>
        ) : null}
      </section>
      <section className="robot-side" aria-label="Данные робота">
        <h3>Предложение</h3>
        <Rows rows={offer} />
        <h3>Характеристики</h3>
        <Rows
          rows={specs.map((f): [string, ReactNode] => [
            f.label,
            <>
              {text(f.code)}
              {known(solutionValue(s, f.code)) ? <SourceButton s={s} field={f} fields={fields} /> : null}
            </>,
          ])}
        />
        {about.length > 0 ? (
          <>
            <h3>О решении</h3>
            <Rows rows={about.map((f): [string, ReactNode] => [f.label, <Reflow key={f.code}>{text(f.code)}</Reflow>])} />
          </>
        ) : null}
      </section>
    </>
  )
}
