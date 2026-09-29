import { useQuery } from '@tanstack/react-query'
import { useMemo, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'

import { fetchProject } from '../api/client'
import { CatalogFilters } from '../catalog/CatalogFilters'
import { CatalogPager } from '../catalog/CatalogPager'
import { CatalogTable } from '../catalog/CatalogTable'
import type { CompareSelect } from '../catalog/CompareCheck'
import { compareCap, keys, pageOf, pageSize, sortFrom, sortSolutions, type CatalogSort } from '../catalog/filter'
import { RobotOverlay } from '../catalog/RobotOverlay'
import { useCatalogView } from '../catalog/useCatalogView'
import { useProjectRoute } from '../layout/route'
import { useTabAccess } from '../layout/shellContext'
import { CompareOverlay } from '../robots/CompareOverlay'
import { RankedRobots, type Ranking } from '../robots/RankedRobots'
import { RankedTable } from '../robots/RankedTable'
import { rankRows, type RankSort } from '../robots/ranking'
import { useOnSearchTarget } from '../search/target'
import { numberText } from '../ui/numberText'
import { Reflow } from '../ui/Reflow'
import { Select, type SelectOption } from '../ui/Select'

const sortOptions: SelectOption<CatalogSort>[] = [
  { value: 'name', label: 'Название' },
  { value: 'price_rub', label: 'Цена' },
  { value: 'payload_kg', label: 'Грузоподъёмность' },
]

const rankedSortOptions: SelectOption<RankSort>[] = [
  { value: 'fit', label: 'Соответствие' },
  { value: 'payback', label: 'Окупаемость' },
  { value: 'capex', label: 'CAPEX' },
  { value: 'price_rub', label: 'Цена' },
  { value: 'payload_kg', label: 'Грузоподъёмность' },
  { value: 'name', label: 'Название' },
]

// Robots is the catalog. Once the project can be calculated, the calculation suggests a robot, ranks the rest,
// and a pick goes into one of the project's variants.
export function Robots() {
  const route = useProjectRoute()
  const nav = useTabAccess()
  const projectQ = useQuery({
    queryKey: ['project', route.projectId],
    queryFn: () => fetchProject(route.projectId as string),
    enabled: Boolean(route.projectId),
  })
  const calc = route.demo ? undefined : (nav['calc:summary'] ?? nav.calc)
  if (route.projectId && projectQ.isPending) {
    return (
      <section>
        <h1 className="sr-only">Роботы</h1>
        <p>Загрузка...</p>
      </section>
    )
  }
  if (!calc || calc.open) {
    return <RankedRobots list={(ranking) => <RobotCatalog ranking={ranking} />} />
  }
  const lead = (
    <p>
      <Reflow>{`Предложение робота появится после расчёта. ${calc.reason}`}</Reflow>
      {calc.link ? (
        <>
          {' '}
          <Link to={route.href(calc.link.tab, calc.link.sub)}>{calc.link.label}</Link>
        </>
      ) : null}
    </p>
  )
  return <RobotCatalog lead={lead} />
}

// RobotCatalog lists the whole catalog, filtered in the browser (catalog/filter). A row opens the robot overlay.
function RobotCatalog({ ranking, lead }: { ranking?: Ranking; lead?: ReactNode }) {
  const view = useCatalogView(false)
  // A robot found by the search is shown by its name, with the other filters back at their defaults.
  useOnSearchTarget((t) => {
    if (t.entry.kind === 'robot') {
      view.showOnly(t.entry.label)
    }
  })

  const sortParam = view.params.get(keys.sort)
  const rankSort = sortFrom(sortParam, rankedSortOptions)
  const plainSort = sortFrom(sortParam, sortOptions)
  const rankItems = ranking?.items
  const ranked = useMemo(() => (rankItems ? rankRows(view.filtered, rankItems, rankSort) : null), [view.filtered, rankItems, rankSort])
  const ordered = useMemo(() => (ranked ? ranked.map((r) => r.solution) : sortSolutions(view.filtered, plainSort)), [ranked, view.filtered, plainSort])
  const page = pageOf(view.params, ordered.length)
  const from = (page - 1) * pageSize

  const sort = ranking ? (
    <Select prefix="Сортировка" value={rankSort} options={rankedSortOptions} onChange={(v) => view.setSort(v, 'fit')} />
  ) : (
    <Select prefix="Сортировка" value={plainSort} options={sortOptions} onChange={(v) => view.setSort(v, 'name')} />
  )

  const open = (s: { id: string }) => view.open(s.id)

  const [comparing, setComparing] = useState(false)
  const picked = useMemo(() => view.compare.flatMap((id) => view.items.find((s) => s.id === id) ?? []), [view.compare, view.items])
  const compare: CompareSelect = useMemo(
    () => ({ ids: new Set(view.compare), full: view.compare.length >= compareCap, toggle: view.toggleCompare }),
    [view.compare, view.toggleCompare],
  )
  const tools =
    view.compare.length > 0 ? (
      <>
        <button type="button" className="btn" disabled={picked.length < 2} onClick={() => setComparing(true)}>
          {`Сравнить: ${numberText(picked.length)}`}
        </button>
        <button type="button" className="btn btn-text" onClick={view.clearCompare}>
          Снять выбор
        </button>
      </>
    ) : null

  let body: ReactNode
  if (view.query.isPending) {
    body = (
      <p>
        <Reflow>Загрузка каталога...</Reflow>
      </p>
    )
  } else if (view.query.isError) {
    body = (
      <p className="error">
        <Reflow>{view.query.error instanceof Error ? view.query.error.message : 'Не удалось загрузить каталог. Обновите страницу.'}</Reflow>
      </p>
    )
  } else if (ordered.length === 0) {
    body = (
      <p>
        <Reflow>{view.items.length === 0 ? 'Каталог пуст. Его заполняет администратор.' : 'Под фильтр не подходит ни один робот. Измените фильтр.'}</Reflow>
      </p>
    )
  } else if (ranking && ranked) {
    body = (
      <RankedTable
        rows={ranked.slice(from, from + pageSize)}
        weights={ranking.weights}
        objectType={ranking.objectType}
        inCalc={ranking.inCalc}
        pick={ranking.pick}
        compare={compare}
        onOpen={open}
      />
    )
  } else {
    body = <CatalogTable rows={ordered.slice(from, from + pageSize)} onOpen={open} compare={compare} />
  }

  return (
    <section>
      {ranking ? (
        <h2>Все роботы</h2>
      ) : (
        <>
          <h1 className="sr-only">Роботы</h1>
          {lead}
        </>
      )}
      <CatalogFilters items={view.items} filter={view.filter} match={view.match} onChange={view.setFilter} sort={sort} tools={tools} />
      {body}
      <CatalogPager total={ordered.length} page={page} onPage={view.setPage} />
      {comparing ? (
        <CompareOverlay
          robots={picked}
          items={ranking?.items}
          onRemove={(id) => {
            if (picked.length <= 2) {
              setComparing(false)
            }
            view.toggleCompare(id)
          }}
          onClose={() => setComparing(false)}
        />
      ) : null}
      {view.selected ? (
        <RobotOverlay
          items={view.items}
          list={ordered}
          selected={view.selected}
          filter={view.filter}
          match={view.match}
          onFilter={view.setFilter}
          onSelect={view.select}
          onClose={(last) => view.close(ordered, last)}
        />
      ) : null}
    </section>
  )
}
