import type { Process } from '../api/client'
import { numberText } from '../ui/numberText'

import { taskTypeOptions } from './defaults'

export type ProcessFieldKey =
  | 'name'
  | 'task_type'
  | 'units_per_day'
  | 'units_per_job'
  | 'max_wait_min'
  | 'max_cycle_min'
  | 'priority'
  | 'load_s'
  | 'unload_s'
  | 'headcount'

export type ProcessField = {
  key: ProcessFieldKey
  label: string
  help?: string
  unit?: string
  // value is the stored value as text, for search; undefined when the field is empty.
  value: (p: Process) => string | undefined
}

const num = (v: number | undefined) => (v === undefined ? undefined : numberText(v))

// processFields are the rows of the process editor, in order. The dialog draws them and the search finds them.
export const processFields: ProcessField[] = [
  { key: 'name', label: 'Имя', value: (p) => p.name || undefined },
  {
    key: 'task_type',
    label: 'Тип задачи',
    value: (p) => taskTypeOptions.find((o) => o.value === p.task_type)?.label ?? p.task_type,
  },
  { key: 'units_per_day', label: 'Спрос в сутки', value: (p) => num(p.demand.units_per_day) },
  {
    key: 'units_per_job',
    label: 'Единиц в задании',
    help: 'Единиц в одном задании робота: сколько строк или паллет перевозит один рейс. Симуляция делит спрос на это число. Пустое поле: 40 для штучного отбора, 1 для остальных задач.',
    value: (p) => num(p.demand.units_per_job),
  },
  {
    key: 'max_wait_min',
    label: 'Ожидание не дольше',
    help: 'SLA: наибольшее ожидание, пока задание не назначено роботу.',
    unit: 'мин',
    value: (p) => num(p.sla.max_wait_min),
  },
  {
    key: 'max_cycle_min',
    label: 'Цикл не дольше',
    help: 'SLA: наибольшее время полного цикла задания.',
    unit: 'мин',
    value: (p) => num(p.sla.max_cycle_min),
  },
  {
    key: 'priority',
    label: 'Приоритет, 0-9',
    help: 'Используется политикой диспетчеризации по SLA: больше значит срочнее. Целое число от 0 до 9.',
    value: (p) => num(p.sla.priority ?? 0),
  },
  {
    key: 'load_s',
    label: 'Время на заборе',
    help: 'Время операции на точке забора. Движение и работа механизмов робота считаются отдельно.',
    unit: 'с',
    value: (p) => num(p.durations.load_s),
  },
  {
    key: 'unload_s',
    label: 'Время на доставке',
    help: 'Время операции на точке доставки. Движение и работа механизмов робота считаются отдельно.',
    unit: 'с',
    value: (p) => num(p.durations.unload_s),
  },
  { key: 'headcount', label: 'Персонал базы', unit: 'чел', value: (p) => num(p.baseline_staff.headcount) },
]
