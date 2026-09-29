import type { MatchItem, MatchStep, ScoreParts } from '../api/client'
import { formatRub } from '../econ/view'
import { numberText } from '../ui/numberText'

const ruleLabels: Record<string, string> = {
  aisle_width: 'Ширина проезда',
  payload_kg: 'Грузоподъёмность',
  temp_range: 'Рабочая температура',
  ceiling_height: 'Высота потолков',
  robot_mass_floor: 'Нагрузка на пол',
  lift_capacity: 'Грузоподъёмность лифта',
  door_width: 'Ширина проёмов',
  turning_envelope: 'Место для разворота',
  process_object_type: 'Тип объекта',
  task_capability: 'Задача процесса',
  data_quality: 'Полнота данных',
  ttx_confidence: 'Источник данных',
  price_budget: 'Бюджет CAPEX',
}

// ruleLabel names a matching rule for people; a rule the table does not know reads as a plain check.
export function ruleLabel(id: string): string {
  return ruleLabels[id] ?? 'Проверка'
}

const unitLabels: Record<string, string> = { mm: 'мм', kg: 'кг', 'kg/m2': 'кг/м²', '°C': '°C' }

// stepValue prints a value of a step with its unit; a step that compares nothing (a match of the object type)
// has no value and prints nothing.
export function stepValue(v: number | null, unit?: string): string {
  if (v === null || v === undefined) {
    return ''
  }
  if (unit === 'rub') {
    return formatRub(v)
  }
  const label = unit ? (unitLabels[unit] ?? '') : ''
  return label ? `${numberText(v, 2)} ${label}` : numberText(v, 2)
}

// stepsOf lists every check of a robot in the order the API ran them.
export function stepsOf(item: MatchItem): MatchStep[] {
  return item.explanation ?? [...(item.hard ?? []), ...(item.soft ?? []), ...(item.missing_evidence ?? [])]
}

export type StepVerdict = { label: string; tag: string }

// stepVerdict says how a check ended: a hard check that fails rules the robot out, a soft one asks for a look.
export function stepVerdict(st: MatchStep): StepVerdict {
  if (st.outcome === 'pass') {
    return { label: 'выполнено', tag: 'tag-good' }
  }
  if (st.outcome === 'fail') {
    return st.kind === 'hard' ? { label: 'не выполнено', tag: 'tag-danger' } : { label: 'проверить', tag: 'tag-warning' }
  }
  return { label: 'нет данных', tag: 'tag-warning' }
}

export const partLabels: [keyof ScoreParts, string][] = [
  ['fit', 'Соответствие объекту'],
  ['process_match', 'Тип объекта и задачи'],
  ['data_quality', 'Полнота данных'],
  ['price_band', 'Цена и бюджет'],
]

// partRows are the score of a robot by part, each with its weight, and the total.
export function partRows(item: MatchItem, weights: ScoreParts): [string, string][] {
  return [
    ...partLabels.map(([key, label]): [string, string] => [
      label,
      `${numberText(item.score_parts[key], 2)}, вес ${numberText(weights[key], 2)}`,
    ]),
    ['Итоговая оценка', numberText(item.score, 2)],
  ]
}
