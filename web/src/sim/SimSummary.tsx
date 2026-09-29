import { AbbreviationText } from '../ui/Abbreviation'
import { Reflow } from '../ui/Reflow'
import { formatNum } from '../econ/view'
import { confidenceLabel } from '../projects/runs'
import type { SimulationSummary } from '../api/client'

import { dur, pct, policyLabels, range } from './format'
import { formatClock } from './replay'

export function SimSummary({ summary }: { summary: SimulationSummary }) {
  const r = summary.result
  const k = r.kpi
  return (
    <>
      <p className={r.verdict === 'pass' ? 'verdict pass' : 'verdict fail'}>
        <Reflow>{r.verdict_text}</Reflow>
      </p>
      <p>
        <Reflow>
          {r.config.mode === 'deterministic' ? 'Детерминированный прогон' : `${r.replications} повторов`}.
          Горизонт {formatClock(r.horizon_s)}, окна поступления{' '}
          {r.windows.map((w) => `${formatNum(w.start_h, 2)}-${formatNum(w.end_h, 2)} ч`).join(', ')}, активных часов в сутках{' '}
          {formatNum(r.active_hours_per_day, 1)}. Политика {policyLabels[r.config.policy ?? 'fifo']}. Карта{' '}
          {summary.map_source === 'project' ? 'проекта' : 'шаблонная'}, уровень подтверждённости {confidenceLabel(summary.confidence_level).name.toLowerCase()}.
          Расчёт{' '}
          {summary.duration_ms < 1000 ? `${summary.duration_ms} мс` : `${formatNum(summary.duration_ms / 1000, 1)} с`}.
        </Reflow>
      </p>
      {summary.econ_check ? (
        <p className={summary.econ_check.flag ? 'error' : undefined}>
          <Reflow>{summary.econ_check.text}</Reflow>
        </p>
      ) : null}

      <h3>Показатели (медиана, в скобках min - max по повторам)</h3>
      <div className="table-wrap">
        <table className="num-table">
          <tbody>
            <tr>
              <th>Заданий поступило / выполнено</th>
              <td>
                {range(k.arrived, (v) => formatNum(v, 0))} / {range(k.completed, (v) => formatNum(v, 0))}
              </td>
            </tr>
            <tr>
              <th>Производительность, заданий в час</th>
              <td>{range(k.throughput_per_h, (v) => formatNum(v, 1))}</td>
            </tr>
            <tr>
              <th>Доля нарушений <AbbreviationText text="SLA" /></th>
              <td>
                {range(k.violation_rate, pct)}, <AbbreviationText text="p90" /> {pct(k.violation_rate.p90)} при допуске {formatNum(r.sla_target_pct, 1)}%
              </td>
            </tr>
            <tr>
              <th>Ожидание назначения, среднее / <AbbreviationText text="p95" /></th>
              <td>
                {range(k.wait_mean_s, dur)} / {range(k.wait_p95_s, dur)}
              </td>
            </tr>
            <tr>
              <th>Цикл задания, среднее / <AbbreviationText text="p95" /></th>
              <td>
                {range(k.cycle_mean_s, dur)} / {range(k.cycle_p95_s, dur)}
              </td>
            </tr>
            <tr>
              <th>Очередь заданий, средняя / максимум</th>
              <td>
                {range(k.queue_mean, (v) => formatNum(v, 1))} / {range(k.queue_max, (v) => formatNum(v, 0))}
              </td>
            </tr>
            <tr>
              <th>Загрузка флота / зарядка / простой</th>
              <td>
                {range(k.fleet_utilization, pct)} / {range(k.charging_share, pct)} / {range(k.idle_share, pct)}
                {r.battery_modeled ? '' : ' (заряд не моделируется)'}
              </td>
            </tr>
            <tr>
              <th>Пробег флота, км</th>
              <td>{range(k.distance_km, (v) => formatNum(v, 1))}</td>
            </tr>
          </tbody>
        </table>
      </div>

      {r.bottlenecks.length > 0 ? (
        <>
          <h3>Где теряется время</h3>
          <ul className="risk-list">
            {r.bottlenecks.map((b) => (
              <li key={b.id} className={b.primary ? 'risk-high' : undefined}>
                <Reflow>
                  {b.text}
                  {b.kind !== 'fleet' ? ` Доля времени флота в очереди ${pct(b.fleet_share)}.` : ''}
                </Reflow>
              </li>
            ))}
          </ul>
        </>
      ) : null}

      <h3>Процессы</h3>
      <div className="table-wrap">
        <table className="num-table">
          <thead>
            <tr>
              <th>Процесс</th>
              <th>Ожидалось заданий</th>
              <th>Поступило / выполнено</th>
              <th>Нарушения <AbbreviationText text="SLA" /></th>
              <th>Ожидание <AbbreviationText text="p95" /> / <AbbreviationText text="SLA" /></th>
              <th>Цикл <AbbreviationText text="p95" /> / <AbbreviationText text="SLA" /></th>
              <th>Очередь макс.</th>
            </tr>
          </thead>
          <tbody>
            {r.processes.map((p) => (
              <tr key={p.code}>
                <td>
                  <Reflow>{p.name}</Reflow>
                  {p.priority > 0 ? (
                    <div className="band">
                      <Reflow>приоритет {p.priority}</Reflow>
                    </div>
                  ) : null}
                  {p.units_per_job > 1 ? (
                    <div className="band">
                      <Reflow>{formatNum(p.units_per_job, 0)} ед в задании</Reflow>
                    </div>
                  ) : null}
                </td>
                {p.covered ? (
                  <>
                    <td>{formatNum(p.expected_jobs, 0)}</td>
                    <td>
                      {formatNum(p.arrived.median, 0)} / {formatNum(p.completed.median, 0)}
                    </td>
                    <td className={p.violation_rate.median > 0 ? 'error' : undefined}>{range(p.violation_rate, pct)}</td>
                    <td>
                      {dur(p.wait_p95_s.median)} / {p.max_wait_s > 0 ? dur(p.max_wait_s) : 'нет'}
                    </td>
                    <td>
                      {dur(p.cycle_p95_s.median)} / {p.max_cycle_s > 0 ? dur(p.max_cycle_s) : 'нет'}
                    </td>
                    <td>{formatNum(p.queue_max.median, 0)}</td>
                  </>
                ) : (
                  <td colSpan={6} className="note-cell">
                    <Reflow>
                      не моделируется: нет подходящих роботов или потока на карте
                    </Reflow>
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <h3>Общие ресурсы</h3>
      <div className="table-wrap">
        <table className="num-table">
          <thead>
            <tr>
              <th>Ресурс</th>
              <th>Вместимость</th>
              <th>Загрузка</th>
              <th>Очередь средняя / макс.</th>
              <th>Ожидание среднее / макс.</th>
            </tr>
          </thead>
          <tbody>
            {r.resources.map((res) => (
              <tr key={res.id}>
                <td>
                  <Reflow>{res.name}</Reflow>
                  {res.auto ? <div className="band"><Reflow>найден по ширине</Reflow></div> : null}
                </td>
                <td>{res.capacity}</td>
                <td>{range(res.utilization, pct)}</td>
                <td>
                  {formatNum(res.queue_mean.median, 2)} / {formatNum(res.queue_max.median, 0)}
                </td>
                <td>
                  {dur(res.wait_mean_s.median)} / {dur(res.wait_max_s.median)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <h3>Флот</h3>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Робот</th>
              <th>Класс</th>
              <th className="num">Кол-во</th>
              <th>Процессы</th>
              <th>Скорость, ширина</th>
              <th>Автономность, заряд</th>
            </tr>
          </thead>
          <tbody>
            {r.fleet.map((f) => (
              <tr key={f.key}>
                <td>
                  <Reflow>{f.name}</Reflow>
                </td>
                <td>
                  <Reflow>{f.profile_label}</Reflow>
                </td>
                <td className="num">{f.quantity}</td>
                <td>
                  <Reflow>{f.processes.join(', ') || 'нет'}</Reflow>
                </td>
                <td>
                  <Reflow>
                    {formatNum(f.speed_mps, 2)} м/с, {formatNum(f.width_m, 2)} м
                  </Reflow>
                </td>
                <td>
                  <Reflow>
                    {formatNum(f.endurance_h, 1)} ч, {formatNum(f.charge_min, 0)} мин
                  </Reflow>
                  {f.assumed && f.assumed.length > 0 ? (
                    <div className="band">
                      <Reflow>приняты значения класса: {f.assumed.join(', ')}</Reflow>
                    </div>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {r.warnings.length > 0 || summary.map_warnings.length > 0 ? (
        <>
          <h3>Предупреждения</h3>
          <ul className="risk-list is-warning">
            {[...r.warnings, ...summary.map_warnings].map((w) => (
              <li key={w} className="risk-warning">
                <Reflow>{w}</Reflow>
              </li>
            ))}
          </ul>
        </>
      ) : null}
      <details>
        <summary>Допущения модели</summary>
        <ul>
          {r.assumptions.map((a) => (
            <li key={a}>
              <Reflow>{a}</Reflow>
            </li>
          ))}
        </ul>
      </details>
    </>
  )
}
