import type { Reason } from './apply'

const texts: Record<Reason, string> = {
  target_missing: 'Объект уже удалён.',
  ref_missing: 'Связанный объект удалён.',
  unique: 'Такой объект уже есть.',
  limit: 'Достигнут предел числа объектов.',
  invalid_value: 'Недопустимое значение.',
  immutable: 'Это поле нельзя изменить.',
  forbidden: 'Нет прав на изменение проекта.',
  schema_version: 'Приложение устарело. Обновите страницу.',
  project_deleted: 'Проект удалён.',
}

// reasonText explains in Russian why a change was not applied.
export function reasonText(reason: Reason): string {
  return texts[reason]
}
