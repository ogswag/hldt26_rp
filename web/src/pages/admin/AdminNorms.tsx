import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef } from 'react'

import { fetchNorms, putAdminNorms, type CalcNorms } from '../../api/client'
import { FieldRow } from '../../ui/FieldRow'
import { NumberField } from '../../ui/NumberField'
import { Reflow } from '../../ui/Reflow'
import { UnitField } from '../../ui/UnitField'

type Key = keyof CalcNorms

const groups: { title: string; rows: { key: Key; label: string; unit: string; share?: boolean; help: string }[] }[] = [
  {
    title: 'Подбор флота',
    rows: [
      { key: 'utilization', label: 'Загрузка', unit: '%', share: true, help: 'Доля времени, когда робот занят работой. Источник: легенда датасета, AMR 70-85%.' },
      { key: 'availability', label: 'Доступность', unit: '%', share: true, help: 'Доля времени, когда робот доступен, включая зарядку. Источник: допущение команды.' },
      { key: 'reserve', label: 'Резерв', unit: '%', share: true, help: 'Запас роботов сверх расчётного числа. Источник: допущение команды.' },
      { key: 'robots_per_charger', label: 'Роботов на зарядку', unit: 'шт', help: 'Если у робота нет времени работы и зарядки, одна станция на столько роботов. Источник: допущение команды.' },
    ],
  },
  {
    title: 'CAPEX',
    rows: [
      { key: 'infra_frac', label: 'Инфраструктура', unit: '%', share: true, help: 'Доля цены оборудования на инфраструктуру площадки. Источник: допущение команды.' },
      { key: 'software_frac', label: 'ПО', unit: '%', share: true, help: 'Доля цены оборудования на программное обеспечение. Источник: допущение команды.' },
      { key: 'integration_yes_frac', label: 'Интеграция при WMS', unit: '%', share: true, help: 'Доля цены оборудования, если на объекте есть WMS или аналог. Источник: допущение команды.' },
      { key: 'integration_no_frac', label: 'Интеграция без WMS', unit: '%', share: true, help: 'Доля цены оборудования, если WMS нет. Источник: допущение команды.' },
      { key: 'commissioning_frac', label: 'ПНР', unit: '%', share: true, help: 'Пусконаладка, доля цены оборудования. Источник: допущение команды.' },
      { key: 'training_frac', label: 'Обучение', unit: '%', share: true, help: 'Обучение персонала, доля цены оборудования. Источник: допущение команды.' },
      { key: 'contingency_frac', label: 'Резерв CAPEX', unit: '%', share: true, help: 'Запас на непредвиденные статьи CAPEX. Источник: допущение команды.' },
      { key: 'delivery_frac', label: 'Доставка', unit: '%', share: true, help: 'Доставка, доля цены оборудования. Только в покупке. Источник: допущение команды.' },
    ],
  },
  {
    title: 'OPEX',
    rows: [
      { key: 'energy_kw', label: 'Мощность на робота', unit: 'кВт', help: 'Расчётная мощность одного робота. Источник: допущение команды.' },
      { key: 'energy_rub_per_kwh', label: 'Цена энергии', unit: '₽/кВт·ч', help: 'Цена киловатт-часа с НДС. Источник: допущение команды.' },
      { key: 'license_rub_per_robot', label: 'Лицензии', unit: '₽', help: 'Лицензии на робота в год, с НДС. Источник: допущение команды.' },
      { key: 'consumable_rub_per_robot', label: 'Расходники', unit: '₽', help: 'Расходники на робота в год, с НДС. Источник: допущение команды.' },
      { key: 'comm_rub_per_robot_year', label: 'Связь', unit: '₽', help: 'Связь на робота в год, с НДС. Источник: допущение команды.' },
      { key: 'robots_per_technician', label: 'Роботов на техника', unit: 'шт', help: 'Один техник на столько роботов в смену. Источник: допущение команды.' },
      { key: 'technician_wage_month_rub', label: 'Зарплата техника', unit: '₽', help: 'Зарплата техника в месяц, без НДС, до начислений. Источник: допущение команды.' },
      { key: 'default_service_frac', label: 'Сервис без ТТХ', unit: '%', share: true, help: 'Доля цены оборудования на сервис и ремонт, если у робота нет своей. Источник: допущение модели.' },
    ],
  },
  {
    title: 'Замены',
    rows: [
      { key: 'default_lifetime_years', label: 'Срок службы', unit: 'лет', help: 'Срок службы робота без данных в ТТХ. Источник: допущение модели.' },
      { key: 'battery_frac', label: 'Доля батареи', unit: '%', share: true, help: 'Цена замены батареи, доля цены робота. Источник: допущение модели.' },
      { key: 'battery_years', label: 'Срок батареи', unit: 'лет', help: 'Через сколько лет меняют батарею. Источник: допущение модели.' },
    ],
  },
  {
    title: 'RaaS',
    rows: [
      { key: 'raas_monthly_frac', label: 'Плата в месяц', unit: '%', share: true, help: 'Фиксированный тариф: доля цены оборудования в месяц. Источник: допущение команды.' },
      { key: 'raas_mix_fixed_share', label: 'Доля фиксированной части', unit: '%', share: true, help: 'Смешанный тариф: какая доля платы фиксированная. Источник: допущение команды.' },
    ],
  },
  {
    title: 'Налоги и ставка',
    rows: [
      { key: 'vat_rate', label: 'Ставка НДС', unit: '%', share: true, help: 'НДС новых наборов допущений. Пустые поля набора всё равно берут остальные нормативы.' },
      { key: 'discount_rate', label: 'Ставка дисконтирования', unit: '%', share: true, help: 'Годовая ставка NPV и IRR новых наборов. Простую окупаемость не меняет.' },
    ],
  },
]

function shown(n: CalcNorms, key: Key, share?: boolean): number {
  const v = n[key]
  return share ? Math.round(v * 10000) / 100 : v
}

export function AdminNorms() {
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['norms'], queryFn: fetchNorms })
  const mut = useMutation({
    mutationFn: putAdminNorms,
    onSuccess: (n) => {
      qc.setQueryData(['norms'], n)
      void qc.invalidateQueries({ queryKey: ['catalog-bundle'] })
    },
  })
  // The server takes the whole set of norms, so saves go one after another and each starts from what the last one stored.
  const queue = useRef<Promise<unknown>>(Promise.resolve())

  function save(key: Key, share: boolean | undefined, typed: number) {
    const value = share ? Math.round(typed * 100) / 10000 : typed
    queue.current = queue.current
      .then(() => {
        const stored = qc.getQueryData<CalcNorms>(['norms'])
        return stored ? mut.mutateAsync({ ...stored, [key]: value }) : undefined
      })
      .catch(() => undefined)
  }

  if (q.isPending) {
    return <p>Загружаем нормативы...</p>
  }
  if (q.isError || !q.data) {
    return (
      <p className="error">
        <Reflow>{q.error instanceof Error ? q.error.message : 'Не удалось загрузить нормативы.'}</Reflow>
      </p>
    )
  }
  const n = q.data
  return (
    <section>
      <h1 className="sr-only">Нормативы</h1>
      {mut.isError ? (
        <p className="error">
          <Reflow>{mut.error instanceof Error ? mut.error.message : 'Не удалось сохранить.'}</Reflow>
        </p>
      ) : null}
      {groups.map((g) => (
        <section key={g.title} className="param-section">
          <h2>{g.title}</h2>
          <div className="field-rows is-quiet">
            {g.rows.map((row) => (
              <FieldRow key={row.key} id={`norm-${row.key}`} label={row.label} unit={row.unit} help={row.help}>
                <UnitField unit={row.unit}>
                  <NumberField
                    id={`norm-${row.key}`}
                    saveOnPause={false}
                    value={shown(n, row.key, row.share)}
                    valid={(v) => Number.isFinite(v) && v >= 0}
                    onCommit={(v) => save(row.key, row.share, v)}
                  />
                </UnitField>
              </FieldRow>
            ))}
          </div>
        </section>
      ))}
    </section>
  )
}
