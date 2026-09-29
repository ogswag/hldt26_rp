// Field labels of the Допущения subtab. The page draws them and the search finds them, so they live here once.

export const whatIfLabels = {
  price: 'Цена изделия',
  volume: 'Объём операций',
  labor: 'ФОТ',
} as const

export type SetFieldKey =
  | 'name'
  | 'vat'
  | 'incl'
  | 'recover'
  | 'labor'
  | 'discount'
  | 'utilization'
  | 'availability'
  | 'reserve'
  | 'service'
  | 'delivery'
  | 'comm'
  | 'wage'

export const setFields: { key: SetFieldKey; label: string; fullLabel?: string }[] = [
  { key: 'name', label: 'Имя' },
  { key: 'vat', label: 'Ставка НДС' },
  { key: 'incl', label: 'Цены включают НДС' },
  { key: 'recover', label: 'НДС к вычету' },
  { key: 'labor', label: 'Денежная экономия труда', fullLabel: 'Доля денежной экономии труда' },
  { key: 'discount', label: 'Ставка дисконтирования', fullLabel: 'Ставка дисконтирования для NPV и IRR' },
  { key: 'utilization', label: 'Загрузка', fullLabel: 'Загрузка флота' },
  { key: 'availability', label: 'Доступность' },
  { key: 'reserve', label: 'Резерв' },
  { key: 'service', label: 'Доля сервиса', fullLabel: 'Доля сервиса и ремонта' },
  { key: 'delivery', label: 'Доставка' },
  { key: 'comm', label: 'Связь', fullLabel: 'Связь на робота в год' },
  { key: 'wage', label: 'Зарплата техника', fullLabel: 'Зарплата техника в месяц' },
]

export function setFieldLabel(key: SetFieldKey): string {
  return setFields.find((f) => f.key === key)?.label ?? key
}
