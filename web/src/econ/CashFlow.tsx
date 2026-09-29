import type { CalculateResult, FlowRow } from '../api/client'

import { AbbreviationText } from '../ui/Abbreviation'
import { Reflow } from '../ui/Reflow'
import { variantColumns } from './ScenarioTable'
import { formatRub, scenarioLabel } from './view'

const blank = (v: number | undefined) => (v ? formatRub(v) : '')

function FlowTable({ rows }: { rows: FlowRow[] }) {
  return (
    <div className="table-wrap">
      <table className="num-table">
        <thead>
          <tr>
            <th>Год</th>
            <th>Эффект года</th>
            <th>Замена батарей</th>
            <th>Повторная покупка</th>
            <th>Итого за год</th>
            <th>Накопленный поток</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.year}>
              <td>{r.year === 0 ? 'Вложения' : r.year}</td>
              <td>{r.year === 0 ? '' : formatRub(r.effect_rub)}</td>
              <td>{blank(r.battery_rub)}</td>
              <td>{blank(r.replacement_rub)}</td>
              <td>{formatRub(r.net_rub)}</td>
              <td>{formatRub(r.cumulative_rub)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// CashFlowTables shows the yearly schedule every payback, NPV and ROI figure is read from.
export function CashFlowTables({ result }: { result: CalculateResult }) {
  const cols =
    result.variants && result.variants.length > 0
      ? variantColumns(result)
      : result.scenarios.map((sc) => ({ key: sc.kind, label: scenarioLabel(sc.kind), sc }))
  const shown = cols.filter((c) => c.sc && c.sc.kind !== 'baseline' && c.sc.cash_flow && c.sc.cash_flow.length > 0)
  if (shown.length === 0) {
    return null
  }
  return (
    <>
      <h2>Денежный поток по годам</h2>
      <p>
        <Reflow>
          Эффект года: OPEX базы минус постоянный OPEX сценария. Замена батарей и повторная покупка робота попадают в год,
          когда они происходят. Остаточная стоимость оборудования на конец горизонта не учитывается.
        </Reflow>
      </p>
      {shown.map((c) => (
        <div key={c.key}>
          <h3>
            <AbbreviationText text={c.label} />
          </h3>
          <FlowTable rows={c.sc?.cash_flow ?? []} />
        </div>
      ))}
    </>
  )
}
