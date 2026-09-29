import type { SolutionVariant } from '../api/client'
import { fleetEdit, renameEdit, type Edit } from '../fleet/records'
import type { State } from '../store/apply'
import { NumberField } from '../ui/NumberField'
import { Reflow } from '../ui/Reflow'
import { TextField } from '../ui/TextField'
import { UnitField } from '../ui/UnitField'

import { verdicts } from './format'

type Props = {
  variants: SolutionVariant[]
  // names maps a catalog id to the robot's name.
  names: ReadonlyMap<string, string>
  // statuses maps a catalog id to the matching status, so a robot the matching did not recommend is marked.
  statuses: ReadonlyMap<string, string>
  state: () => State
  run: (edit: Edit) => void
}

// Slots are the project's variants: the robots picked on this page and how many of each. Итог compares the
// variants that have robots; while none has, the calculation and the simulation take the suggestion.
export function Slots({ variants, names, statuses, state, run }: Props) {
  const empty = variants.every((v) => !v.fleet.some((f) => f.solution_id))
  return (
    <section className="robot-slots" aria-labelledby="robot-slots-title">
      <h2 id="robot-slots-title">Варианты</h2>
      {empty ? (
        <p>
          <Reflow>Роботы не выбраны, поэтому расчёт и симуляция берут предложение.</Reflow>
        </p>
      ) : null}
      <div className="slot-grid">
        {variants.map((v) => {
          const fleet = v.fleet.filter((f) => f.solution_id)
          const setFleet = (next: typeof v.fleet) => run(fleetEdit(state(), v.id as string, next))
          return (
            <div className="slot is-quiet" key={v.id}>
              <label>
                Название
                <TextField
                  value={v.name}
                  maxLength={200}
                  onCommit={(name) => {
                    if (name && name !== v.name) {
                      run(renameEdit(v.id as string, name))
                    }
                  }}
                />
              </label>
              {fleet.length === 0 ? (
                <p className="slot-empty">Роботов нет</p>
              ) : (
                <ul className="slot-fleet">
                  {fleet.map((f) => {
                    const name = names.get(f.solution_id as string) ?? 'Робота нет в каталоге'
                    const status = statuses.get(f.solution_id as string)
                    const verdict = status && status !== 'recommended' ? verdicts[status] : undefined
                    return (
                      <li key={f.id}>
                        <span className="slot-robot">
                          <Reflow>{name}</Reflow>
                          {verdict ? <span className={`tag ${verdict.tag}`}>{verdict.label}</span> : null}
                        </span>
                        <UnitField unit="шт">
                          <NumberField
                            aria-label={`Количество, ${name}, шт`}
                            value={f.quantity}
                            valid={(n) => Number.isInteger(n) && n >= 1 && n <= 1000}
                            onCommit={(quantity) =>
                              setFleet(v.fleet.map((x) => (x.id === f.id ? { ...x, quantity } : x)))
                            }
                          />
                        </UnitField>
                        <button
                          type="button"
                          className="btn btn-danger"
                          onClick={() => setFleet(v.fleet.filter((x) => x.id !== f.id))}
                        >
                          Убрать
                        </button>
                      </li>
                    )
                  })}
                </ul>
              )}
            </div>
          )
        })}
      </div>
    </section>
  )
}
