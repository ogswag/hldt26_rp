import type { ReactNode } from 'react'

import type { CalculateResult, EconRisk, MetricOrigin, ScenarioResult, VariantResult } from '../api/client'

import { Reflow } from '../ui/Reflow'
import { AbbreviationText } from '../ui/Abbreviation'
import { bandLabel, formatNum, formatPayback, formatPct, formatRub, formatYearsOrNone, originFor, scenarioLabel, tariffLabel } from './view'

const kinds = ['baseline', 'buy', 'raas'] as const
const compareCap = 6

function byKind(result: CalculateResult): Record<string, ScenarioResult | undefined> {
  const out: Record<string, ScenarioResult | undefined> = {}
  for (const sc of result.scenarios) {
    out[sc.kind] = sc
  }
  return out
}

function PaybackCell({ sc }: { sc: ScenarioResult | undefined }) {
  const text = formatPayback(sc)
  const label = sc && sc.kind !== 'baseline' ? bandLabel(sc.payback_band) : ''
  const band = label === text ? '' : label
  return (
    <>
      <Reflow>{text}</Reflow>
      {band ? (
        <div className="band">
          <Reflow>{band}</Reflow>
        </div>
      ) : null}
    </>
  )
}

function OriginHint({ origins, metric }: { origins: MetricOrigin[] | undefined; metric: string }) {
  const o = originFor(origins, metric)
  if (!o) {
    return null
  }
  return (
    <div className="origin">
      <Reflow>{o.note}</Reflow>
    </div>
  )
}

type MetricRow = {
  key: string
  label: ReactNode
  origin?: string
  cell: (sc: ScenarioResult | undefined) => ReactNode
}

const notBaseline = (sc: ScenarioResult | undefined) => sc !== undefined && sc.kind !== 'baseline'

const metricRows: MetricRow[] = [
  { key: 'fleet', label: 'Флот', cell: (sc) => formatNum(sc?.fleet_size ?? null, 0) },
  { key: 'capex', label: <AbbreviationText text="CAPEX" />, origin: 'capex_rub', cell: (sc) => formatRub(sc?.capex_rub) },
  { key: 'opex', label: <AbbreviationText text="OPEX/год" />, origin: 'opex_year_rub', cell: (sc) => formatRub(sc?.opex_year_rub) },
  { key: 'effect', label: 'Денежный эффект/год', origin: 'annual_effect_rub', cell: (sc) => formatRub(sc?.annual_effect_rub) },
  { key: 'payback', label: 'Простая окупаемость', origin: 'payback_years', cell: (sc) => <PaybackCell sc={sc} /> },
  {
    key: 'discounted',
    label: 'Дисконтированная окупаемость',
    origin: 'discounted_payback_years',
    cell: (sc) => (notBaseline(sc) ? formatYearsOrNone(sc?.discounted_payback_years) : 'нет данных'),
  },
  { key: 'npv', label: <AbbreviationText text="NPV" />, origin: 'npv_rub', cell: (sc) => formatRub(sc?.npv_rub) },
  { key: 'irr', label: <AbbreviationText text="IRR" />, origin: 'irr_pct', cell: (sc) => (notBaseline(sc) ? formatPct(sc?.irr_pct, 'нет') : 'нет данных') },
  { key: 'roi', label: <AbbreviationText text="ROI за горизонт" />, origin: 'roi_pct', cell: (sc) => formatPct(sc?.roi_pct, 'нет данных') },
  { key: 'tco', label: <AbbreviationText text="Затраты за горизонт (TCO)" />, origin: 'tco_rub', cell: (sc) => formatRub(sc?.tco_rub) },
]

function MetricRows({ cols, origins }: { cols: { key: string; sc: ScenarioResult | undefined }[]; origins: MetricOrigin[] | undefined }) {
  return (
    <tbody>
      {metricRows.map((row) => (
        <tr key={row.key}>
          <th>
            {row.label}
            {row.origin ? <OriginHint origins={origins} metric={row.origin} /> : null}
          </th>
          {cols.map((c) => (
            <td key={c.key}>{row.cell(c.sc)}</td>
          ))}
        </tr>
      ))}
    </tbody>
  )
}

export function ScenarioTable({ result }: { result: CalculateResult }) {
  if (result.variants && result.variants.length > 0) {
    return <VariantCompareTable result={result} />
  }
  const map = byKind(result)
  const cols = kinds.map((k) => ({ key: k, sc: map[k] }))
  return (
    <div className="table-wrap">
      <table className="num-table">
        <thead>
          <tr>
            <th>Показатель</th>
            {kinds.map((k) => (
              <th key={k}>{scenarioLabel(k)}</th>
            ))}
          </tr>
        </thead>
        <MetricRows cols={cols} origins={result.origins} />
      </table>
    </div>
  )
}

export type Col = { key: string; label: string; sc: ScenarioResult | undefined; variant?: VariantResult }

export function variantColumns(result: CalculateResult): Col[] {
  const cols: Col[] = [
    {
      key: 'baseline',
      label: 'База',
      sc: result.scenarios.find((s) => s.kind === 'baseline'),
    },
  ]
  const variants = (result.variants ?? []).slice(0, compareCap)
  for (const vr of variants) {
    for (const sc of vr.scenarios) {
      const fin = sc.kind === 'buy' ? 'покупка' : `RaaS ${tariffLabel(sc.tariff)}`
      cols.push({
        key: `${vr.variant_id}:${sc.kind}:${sc.tariff ?? ''}`,
        label: `${vr.name}, ${fin}`,
        sc,
        variant: vr,
      })
    }
  }
  return cols
}

export function VariantCompareTable({ result }: { result: CalculateResult }) {
  const cols = variantColumns(result)
  const extra = (result.variants ?? []).length > compareCap
  return (
    <div className="table-wrap">
      {extra ? <p><Reflow>Сравнение ограничено шестью техническими вариантами.</Reflow></p> : null}
      <table className="num-table">
        <thead>
          <tr>
            <th>Показатель</th>
            {cols.map((c) => (
              <th key={c.key}><AbbreviationText text={c.label} /></th>
            ))}
          </tr>
        </thead>
        <MetricRows cols={cols} origins={result.origins} />
      </table>
      <FleetSources result={result} />
    </div>
  )
}

function FleetSources({ result }: { result: CalculateResult }) {
  const variants = (result.variants ?? []).slice(0, compareCap)
  if (variants.length === 0) {
    return null
  }
  return (
    <>
      <h2>Состав и источник цены</h2>
      {variants.map((vr) => (
        <div key={vr.variant_id}>
          <p>
            <strong>{vr.name}</strong>
          </p>
          {vr.fleet.length === 0 ? <p className="field-hint">Флот пустой.</p> : null}
          {vr.chargers ? <p>Зарядных станций: {formatNum(vr.chargers, 0)}</p> : null}
          <ul>
            {vr.fleet.map((f) => (
              <li key={`${vr.variant_id}:${f.solution_id}`}>
                <Reflow>
                  {f.name}: {f.quantity} шт Цена{' '}
                  {f.price_source === 'project_override' ? 'проектная' : 'каталог'}
                  {f.project_price_rub !== null && f.project_price_rub !== undefined
                    ? ` ${formatRub(f.project_price_rub)}`
                    : ''}
                  {f.catalog_price_rub !== null && f.catalog_price_rub !== undefined
                    ? ` (каталог ${formatRub(f.catalog_price_rub)})`
                    : ''}
                  {f.price_override_reason ? `. ${f.price_override_reason}` : ''}
                </Reflow>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </>
  )
}

export function RiskList({ risks }: { risks: EconRisk[] | undefined }) {
  if (!risks || risks.length === 0) {
    return null
  }
  return (
    <>
      <h2>Риски и ограничения</h2>
      <ul className="risk-list">
        {risks.map((r) => (
          <li key={r.id} className={`risk-${r.level}`} data-search-id={`risk:${r.id}`}>
            <Reflow>{r.text}</Reflow>
          </li>
        ))}
      </ul>
    </>
  )
}

