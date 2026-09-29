import type { Process } from '../api/client'

export const taskTypeOptions = [
  { value: 'pallet_inbound', label: 'Приёмка паллет' },
  { value: 'pallet_putaway', label: 'Размещение' },
  { value: 'pallet_outbound', label: 'Отгрузка паллет' },
  { value: 'piece_pick', label: 'Мелкоштучный отбор' },
  { value: 'pallet_move', label: 'Паллетное перемещение' },
  { value: 'cleaning', label: 'Уборка' },
]

export function defaultProcesses(objectType: string): Process[] {
  if (objectType !== 'warehouse') {
    return []
  }
  return [
    {
      code: 'inbound',
      name: 'Приёмка',
      task_type: 'pallet_inbound',
      is_baseline: true,
      demand: { units_per_day: 1000, unit: 'поддон' },
      sla: { max_wait_min: 30, max_cycle_min: 45 },
      durations: { load_s: 90, unload_s: 60, travel_s: 180 },
      baseline_staff: { headcount: 8, role: 'приёмка' },
      sort_order: 0,
    },
    {
      code: 'putaway',
      name: 'Размещение',
      task_type: 'pallet_putaway',
      is_baseline: true,
      demand: { units_per_day: 1000, unit: 'поддон' },
      sla: { max_wait_min: 20, max_cycle_min: 40 },
      durations: { load_s: 60, unload_s: 60, travel_s: 240 },
      baseline_staff: { headcount: 6, role: 'погрузчик' },
      sort_order: 1,
    },
    {
      code: 'piece_pick',
      name: 'Мелкоштучный отбор',
      task_type: 'piece_pick',
      is_baseline: true,
      demand: { units_per_day: 100000, unit: 'строка' },
      sla: { max_wait_min: 15, max_cycle_min: 25 },
      durations: { load_s: 20, unload_s: 15, travel_s: 90 },
      baseline_staff: { headcount: 100, role: 'отборщик' },
      sort_order: 2,
    },
    {
      code: 'outbound',
      name: 'Отгрузка',
      task_type: 'pallet_outbound',
      is_baseline: true,
      demand: { units_per_day: 1000, unit: 'поддон' },
      sla: { max_wait_min: 25, max_cycle_min: 40 },
      durations: { load_s: 60, unload_s: 90, travel_s: 180 },
      baseline_staff: { headcount: 10, role: 'отгрузка' },
      sort_order: 3,
    },
  ]
}

