import { useEffect, useLayoutEffect, useRef, type ReactNode } from 'react'
import { Link, Navigate, useParams } from 'react-router-dom'

import { type CalculateResult, type EconSimCheck } from '../api/client'
import { CashFlowTables } from '../econ/CashFlow'
import { RiskList, ScenarioTable } from '../econ/ScenarioTable'
import { useCalcNotice } from '../econ/useCalcNotice'
import { useCalcResult } from '../econ/useCalcResult'
import { raised, useSimChecks } from '../econ/useSimChecks'
import { PreliminaryNote } from '../engine/PreliminaryNote'
import {
  formatNum,
  formatPayback,
  formatRub,
  lineScenarioLabel,
  sensitivityParamLabel,
  workKindLabel,
} from '../econ/view'
import { calcSubtabsOf } from '../layout/nav'
import { useProjectRoute } from '../layout/route'
import { ActionBar } from '../ui/ActionBar'
import { useReadOnly } from '../ui/readOnly'
import { Reflow } from '../ui/Reflow'
import { AbbreviationText, abbreviationTitle } from '../ui/Abbreviation'

import { Assumptions } from './CalcAssumptions'

function Status({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <h1 className="sr-only">{title}</h1>
      {children}
    </section>
  )
}

// Calc is the Расчёт tab: the economics and the comparison of the same calculation, split into subtabs. A
// project calculates itself when it opens here with changed inputs; each such run lands in История once.
export function Calc() {
  const { tab } = useParams()
  const route = useProjectRoute()
  const subtabs = calcSubtabsOf(route.demo !== null).filter((t) => t.id !== 'sim')
  const calc = useCalcResult()
  const checks = useSimChecks(calc.result)
  const readOnly = useReadOnly()
  const tried = useRef(false)
  const sub = subtabs.find((t) => t.id === tab)
  const title = sub?.label ?? 'Расчёт'

  const { stale, loading, error, recalc } = calc
  const auto = Boolean(route.projectId) && stale && !loading && !error && !readOnly
  useEffect(() => {
    if (auto && !tried.current) {
      tried.current = true
      void recalc().catch(() => {})
    }
  }, [auto, recalc])
  useCalcNotice({
    active: Boolean(sub) && calc.result !== null,
    loading,
    error,
    retry: () => void recalc().catch(() => {}),
  })

  if (!sub) {
    return <Navigate to={route.href('calc', subtabs[0].id)} replace />
  }

  if (calc.loading && !calc.result) {
    return (
      <Status title={title}>
        <p><Reflow>Считаем CAPEX, OPEX и окупаемость...</Reflow></p>
      </Status>
    )
  }

  if (calc.error && !calc.result) {
    return (
      <Status title={title}>
        <p className="error">
          <Reflow>{calc.error}</Reflow>
        </p>
        {calc.details.length > 0 ? (
          <ul>
            {calc.details.map((d) => (
              <li key={d.field} className="error">
                <Reflow>{d.message}</Reflow>
              </li>
            ))}
          </ul>
        ) : null}
        <p>
          <Reflow>
            Исправьте параметры и повторите. <Link to={route.href('object')}>Открыть «Объект»</Link>
          </Reflow>
        </p>
      </Status>
    )
  }

  const result = calc.result
  if (!result) {
    return (
      <Status title={title}>
        <p><Reflow>Расчёта ещё нет. Нажмите «Пересчитать».</Reflow></p>
        <ActionBar label="Действия с расчётом">
          <button type="button" className="btn btn-primary" onClick={() => void calc.recalc()} disabled={calc.loading}>
            Пересчитать
          </button>
        </ActionBar>
      </Status>
    )
  }

  return (
    <section>
      <h1 className="sr-only">{title}</h1>
      <PreliminaryNote preliminary={calc.preliminary} />
      {sub.id === 'summary' ? <Summary result={result} checks={checks} /> : null}
      {sub.id === 'items' ? (
        <>
          <Breakdown result={result} />
          <CashFlowTables result={result} />
          <Sensitivity result={result} />
        </>
      ) : null}
      {sub.id === 'assumptions' ? (
        <Assumptions
          result={result}
          stored={calc.storedOverrides}
          projectId={calc.projectId}
          onRun={(ov) => calc.recalc(ov)}
          onRecalc={() => void calc.recalc().catch(() => {})}
        />
      ) : null}
      {sub.id === 'method' ? <Method result={result} checks={checks} /> : null}
      <ActionBar
        label="Действия с расчётом"
        status={
          calc.loading
            ? 'Считаем...'
            : calc.stale
              ? 'Параметры изменились после расчёта'
              : calc.preliminary
                ? 'Предварительный расчёт в браузере'
                : undefined
        }
      >
        <button
          type="button"
          className="btn btn-primary"
          data-search-id="action:recalc"
          onClick={() => void calc.recalc().catch(() => {})}
          disabled={calc.loading}
        >
          Пересчитать
        </button>
      </ActionBar>
    </section>
  )
}

function Summary({ result, checks }: { result: CalculateResult; checks: EconSimCheck[] }) {
  const route = useProjectRoute()
  const apart = raised(checks)
  return (
    <>
      {apart.length > 0 ? (
        <p className="stale-banner">
          <Reflow>
            Симуляция и экономика разошлись больше чем на 15%: {apart.map((c) => `«${c.variant_name}»`).join(', ')}. Не
            используйте цифры этих вариантов без проверки: подробности в{' '}
            <Link to={route.href('calc', 'method')}>«Формулах и источниках»</Link>.
          </Reflow>
        </p>
      ) : null}
      <ScenarioTable result={result} />
      {result.interpretation && result.interpretation.length > 0 ? (
        <div className="interpretation">
          <h2>Как читать цифры</h2>
          {result.interpretation.map((t) => (
            <p key={t}>
              <Reflow>{t}</Reflow>
            </p>
          ))}
        </div>
      ) : null}
      <RiskList risks={result.risks} />
    </>
  )
}

function Method({ result, checks }: { result: CalculateResult; checks: EconSimCheck[] }) {
  return (
    <>
      <SimChecks checks={checks} />
      {result.shared ? (
        <p>
          <Reflow>
            Флот {result.shared.fleet_size} шт, пик {formatNum(result.shared.peak_ops, 2)} {result.shared.unit},
            производительность {formatNum(result.shared.throughput, 2)} {result.shared.unit}, процесс:{' '}
            {workKindLabel(result.shared.work_kind)}.
          </Reflow>
        </p>
      ) : null}
      {result.formulas && result.formulas.length > 0 ? (
        <>
          <h2>Формулы</h2>
          <ul className="formulas">
            {result.formulas.map((f) => (
              <li key={f.id} className="formula" data-search-id={`formula:${f.id}`}>
                {/* NOTE: the markup is MathML the API builds from its own formula list, not user input. */}
                {f.mathml ? <FormulaMath mathml={f.mathml} text={f.text} /> : <div className="formula-math"><AbbreviationText text={f.text} /></div>}
                <span className="formula-unit"><AbbreviationText text={f.unit} /></span>
              </li>
            ))}
          </ul>
        </>
      ) : null}
      {result.sources && result.sources.length > 0 ? (
        <>
          <h2>Источники</h2>
          <ul>
            {result.sources.map((s) => (
              <li key={s}>
                <Reflow>{s}</Reflow>
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </>
  )
}

const checkMethod: Record<EconSimCheck['model'], string> = {
  map: 'Симуляция на карте',
  quick: 'Быстрая проверка без карты',
}

function checkState(c: EconSimCheck): { label: string; tag: string } {
  if (!c.run_id && c.model === 'map') {
    return { label: 'не запускалась', tag: 'tag-info' }
  }
  if (c.stale) {
    return { label: 'устарела', tag: 'tag-warning' }
  }
  return c.flag ? { label: 'расхождение', tag: 'tag-danger' } : { label: 'сходится', tag: 'tag-good' }
}

function SimChecks({ checks }: { checks: EconSimCheck[] }) {
  const route = useProjectRoute()
  if (checks.length === 0) {
    return null
  }
  return (
    <>
      <h2>Проверка симуляцией</h2>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Вариант</th>
              <th>Способ проверки</th>
              <th>Результат</th>
              <th>Состояние</th>
            </tr>
          </thead>
          <tbody>
            {checks.map((c) => {
              const state = checkState(c)
              const rerun = state.label === 'не запускалась' || state.label === 'устарела'
              return (
                <tr key={`${c.variant_id}:${c.model}`}>
                  <td>
                    <Reflow>{c.variant_name}</Reflow>
                  </td>
                  <td>{checkMethod[c.model]}</td>
                  <td>
                    <Reflow>{c.text ?? ''}</Reflow>
                  </td>
                  <td>
                    <span className={`tag ${state.tag}`}>{state.label}</span>
                    {rerun ? (
                      <>
                        {' '}
                        <Link to={route.href('calc', 'sim')}>Открыть «Симуляцию»</Link>
                      </>
                    ) : null}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </>
  )
}

function FormulaMath({ mathml, text }: { mathml: string; text: string }) {
  const ref = useRef<HTMLDivElement>(null)
  const normalized = wrapMathContents(mathml)

  useLayoutEffect(() => {
    const box = ref.current
    const math = box?.querySelector('math')
    if (!box || !math) {
      return
    }
    const fit = () => {
      math.style.fontSize = '1em'
      const width = math.scrollWidth
      const available = box.clientWidth
      const scale = width > 0 && available > 0 ? Math.min(1, Math.max(0, available - 2) / width) : 1
      math.style.fontSize = `${scale}em`
    }
    const observer = new ResizeObserver(fit)
    observer.observe(box)
    void document.fonts?.ready.then(fit)
    fit()
    return () => observer.disconnect()
  }, [normalized])

  return (
    <>
      <div ref={ref} className="formula-math" title={abbreviationTitle(text)} dangerouslySetInnerHTML={{ __html: normalized }} />
      <div className="formula-text-fallback"><AbbreviationText text={text} /></div>
    </>
  )
}

function wrapMathContents(mathml: string): string {
  const match = /^<math([^>]*)>([\s\S]*)<\/math>$/.exec(mathml.trim())
  if (!match) {
    return mathml
  }
  const body = match[2]
  if (/^<mrow>[\s\S]*<\/mrow>$/.test(body)) {
    return mathml
  }
  return `<math${match[1]}><mrow>${body}</mrow></math>`
}

function bucketLabel(bucket: string): string {
  switch (bucket) {
    case 'capex':
      return 'CAPEX'
    case 'replacement':
      return 'Замены'
    default:
      return 'OPEX'
  }
}

function Breakdown({ result }: { result: CalculateResult }) {
  if (!result.breakdown || result.breakdown.length === 0) {
    return null
  }
  return (
    <>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Сценарий</th>
              <th>Тип</th>
              <th>Статья</th>
              <th className="num">Сумма</th>
              <th>Заметка</th>
            </tr>
          </thead>
          <tbody>
            {result.breakdown.map((line, idx) => (
              <tr key={`${line.scenario}:${line.id}:${idx}`} data-search-id={`line:${idx}`}>
                <td>
                  <Reflow>{lineScenarioLabel(result, line.scenario)}</Reflow>
                </td>
                <td><AbbreviationText text={bucketLabel(line.bucket)} /></td>
                <td>
                  <Reflow>{line.label}</Reflow>
                </td>
                <td className="num">{formatRub(line.rub)}</td>
                <td>
                  <Reflow>{line.note ?? ''}</Reflow>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  )
}

function Sensitivity({ result }: { result: CalculateResult }) {
  const rows = result.sensitivity
  if (!rows || rows.length === 0) {
    return null
  }
  const grouped = rows.some((row) => row.variant_name)
  const groups: { name: string; rows: typeof rows }[] = []
  if (grouped) {
    for (const row of rows) {
      const name = row.variant_name || 'Вариант'
      const last = groups[groups.length - 1]
      if (last && last.name === name) {
        last.rows.push(row)
      } else {
        groups.push({ name, rows: [row] })
      }
    }
  }
  const body = (items: typeof rows) =>
    items.map((row) => (
      <tr key={`${row.variant_id ?? ''}:${row.param}:${row.delta_pct}`}>
        <td>
          <Reflow>{sensitivityParamLabel(row.param)}</Reflow>
        </td>
        <td>
          {row.delta_pct > 0 ? '+' : ''}
          {row.delta_pct}%
        </td>
        <td>
          <Reflow>{formatPayback(row.buy)}</Reflow>
        </td>
        <td>{formatRub(row.buy.npv_rub)}</td>
        <td>{formatRub(row.buy.tco_rub)}</td>
        <td>
          <Reflow>{formatPayback(row.raas)}</Reflow>
        </td>
        <td>{formatRub(row.raas.npv_rub)}</td>
        <td>{formatRub(row.raas.tco_rub)}</td>
      </tr>
    ))
  return (
    <>
      <h2>Чувствительность ±20%</h2>
      {grouped
        ? groups.map((g) => (
            <section key={g.name}>
              <h3>{g.name}</h3>
              <SensitivityTable>{body(g.rows)}</SensitivityTable>
            </section>
          ))
        : <SensitivityTable>{body(rows)}</SensitivityTable>}
    </>
  )
}

function SensitivityTable({ children }: { children: ReactNode }) {
  return (
    <div className="table-wrap">
      <table className="num-table">
        <thead>
          <tr>
            <th>Параметр</th>
            <th>Сдвиг</th>
            <th>Покупка, окупаемость</th>
            <th>Покупка, <AbbreviationText text="NPV" /></th>
            <th>Покупка, <AbbreviationText text="TCO" /></th>
            <th><AbbreviationText text="RaaS" />, окупаемость</th>
            <th><AbbreviationText text="RaaS" />, <AbbreviationText text="NPV" /></th>
            <th><AbbreviationText text="RaaS" />, <AbbreviationText text="TCO" /></th>
          </tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  )
}
