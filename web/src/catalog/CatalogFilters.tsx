import { useId, useMemo, useState, type ReactNode } from 'react'

import type { Solution } from '../api/client'
import { AbbreviationText } from '../ui/Abbreviation'
import { CheckList, type CheckOption } from '../ui/CheckList'
import { ChevronDownIcon, SearchIcon } from '../ui/icons'
import { NumberField } from '../ui/NumberField'
import { Popover } from '../ui/Popover'
import { Select, type SelectOption } from '../ui/Select'
import { TextField } from '../ui/TextField'
import { UnitField } from '../ui/UnitField'
import { families } from './families'
import { FamilyMark } from './FamilyMark'
import {
  activeCount,
  emptyFilter,
  facetCounts,
  noStatus,
  vendorsByRobots,
  type ArchiveFilter,
  type CatalogFilter,
  type Matcher,
  type SpecsFilter,
} from './filter'
import { statusLabels } from './labels'

const objectOptions: SelectOption[] = [
  { value: '', label: 'Все' },
  { value: 'warehouse', label: 'Склад' },
  { value: 'airport', label: 'Аэропорт' },
  { value: 'hospital', label: 'Медучреждение' },
]

const specsOptions: SelectOption<SpecsFilter>[] = [
  { value: '', label: 'Все' },
  { value: 'complete', label: 'Полные' },
  { value: 'gaps', label: 'С пробелами' },
]

const archiveOptions: SelectOption<ArchiveFilter>[] = [
  { value: 'active', label: 'Активные' },
  { value: 'archived', label: 'Архив' },
  { value: 'all', label: 'Все' },
]

const statusOptions: [string, string][] = [
  ...Object.entries(statusLabels).map(([code, label]): [string, string] => [code, label.charAt(0).toUpperCase() + label.slice(1)]),
  [noStatus, 'Нет данных'],
]

// panelDefaults clears what the panel sets and keeps the search.
const panelDefaults: Partial<CatalogFilter> = {
  families: emptyFilter.families,
  statuses: emptyFilter.statuses,
  vendors: emptyFilter.vendors,
  objectType: emptyFilter.objectType,
  minPayloadKg: emptyFilter.minPayloadKg,
  maxWidthMm: emptyFilter.maxWidthMm,
  specs: emptyFilter.specs,
  archive: emptyFilter.archive,
}

type Props = {
  items: readonly Solution[]
  filter: CatalogFilter
  match: Matcher
  onChange: (patch: Partial<CatalogFilter>) => void
  // admin adds the archive and the specs gaps to the panel.
  admin?: boolean
  // sort is the page's order field; the robot overlay follows the page's order and has none.
  sort?: ReactNode
  // tools are page buttons after the sort, such as the comparison.
  tools?: ReactNode
  compact?: boolean
}

// Search field and the «Фильтры» panel over a catalog list. Typed fields apply 600 ms after
// the last key, lists and checkboxes at once. Every option counts the robots it would show.
export function CatalogFilters({ items, filter, match, onChange, admin, sort, tools, compact }: Props) {
  const [open, setOpen] = useState(false)
  const specsId = useId()
  const n = activeCount(filter)
  const familyCounts = useMemo(() => facetCounts(items, filter, 'families', match), [items, filter, match])
  const statusCounts = useMemo(() => facetCounts(items, filter, 'statuses', match), [items, filter, match])
  const vendorNames = useMemo(() => vendorsByRobots(items), [items])
  const vendorCounts = useMemo(() => facetCounts(items, filter, 'vendors', match), [items, filter, match])
  const familyOptions: CheckOption[] = families.map((f) => ({
    value: f.code,
    label: f.plural,
    icon: <FamilyMark family={f} />,
    count: familyCounts.get(f.code) ?? 0,
  }))
  const statusChecks: CheckOption[] = statusOptions.map(([value, label]) => ({ value, label, count: statusCounts.get(value) ?? 0 }))
  const vendorChecks: CheckOption[] = vendorNames.map((value) => ({ value, label: value, count: vendorCounts.get(value) ?? 0 }))

  return (
    <form className={compact ? 'catalog-filters is-compact' : 'catalog-filters'} role="search" onSubmit={(e) => e.preventDefault()}>
      <label className="catalog-search">
        <SearchIcon size={16} />
        <span className="sr-only">Поиск</span>
        <TextField type="search" placeholder="Поиск" value={filter.q} onCommit={(q) => onChange({ q })} />
      </label>
      <Popover
        open={open}
        onOpenChange={setOpen}
        label="Фильтры"
        align={compact ? 'end' : 'start'}
        className="catalog-filter-popover"
        trigger={(t) => (
          <button type="button" className="select catalog-filter-button" {...t}>
            <span className="select-text">
              <span>{n > 0 ? `Фильтры: ${n}` : 'Фильтры'}</span>
            </span>
            <span className="select-chevron">
              <ChevronDownIcon />
            </span>
          </button>
        )}
      >
        <div className="catalog-filter-panel">
          <CheckList label="Тип" options={familyOptions} value={filter.families} onChange={(v) => onChange({ families: v })} empty="" plain />
          <CheckList label="Статус" options={statusChecks} value={filter.statuses} onChange={(v) => onChange({ statuses: v })} empty="" plain />
          <CheckList
            label="Компания"
            find="Найти компанию"
            options={vendorChecks}
            value={filter.vendors}
            onChange={(v) => onChange({ vendors: v })}
            empty=""
            plain
          />
          <div className="catalog-filter-section">
            <label>
              Объект
              <Select value={filter.objectType} options={objectOptions} onChange={(objectType) => onChange({ objectType })} />
            </label>
          </div>
          <div className="catalog-filter-section" role="group" aria-labelledby={specsId}>
            <span id={specsId} className="check-list-label">
              Характеристики
            </span>
            <div className="catalog-filter-range">
              <label>
                Груз от
                <span className="sr-only">, кг</span>
                <UnitField unit="кг">
                  <NumberField optional value={filter.minPayloadKg} valid={(v) => v >= 0} onCommit={(v) => onChange({ minPayloadKg: v })} />
                </UnitField>
              </label>
              <label>
                Ширина до
                <span className="sr-only">, мм</span>
                <UnitField unit="мм">
                  <NumberField optional value={filter.maxWidthMm} valid={(v) => v > 0} onCommit={(v) => onChange({ maxWidthMm: v })} />
                </UnitField>
              </label>
            </div>
            {admin ? (
              <label>
                <AbbreviationText text="Полнота ТТХ" />
                <Select value={filter.specs} options={specsOptions} onChange={(specs) => onChange({ specs })} />
              </label>
            ) : (
              <label className="check-row">
                <input
                  type="checkbox"
                  checked={filter.specs === 'complete'}
                  onChange={(e) => onChange({ specs: e.target.checked ? 'complete' : '' })}
                />
                <span>
                  <AbbreviationText text="Только с полными ТТХ" />
                </span>
              </label>
            )}
          </div>
          {admin ? (
            <div className="catalog-filter-section">
              <label>
                В каталоге
                <Select value={filter.archive} options={archiveOptions} onChange={(archive) => onChange({ archive })} />
              </label>
            </div>
          ) : null}
          {n > 0 ? (
            <div className="catalog-filter-foot">
              <button type="button" className="btn btn-text" onClick={() => onChange(panelDefaults)}>
                Сбросить фильтры
              </button>
            </div>
          ) : null}
        </div>
      </Popover>
      {sort}
      {tools}
    </form>
  )
}
