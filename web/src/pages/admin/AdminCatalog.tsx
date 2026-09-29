import { useMemo, useState, type ReactNode } from 'react'

import { downloadCatalog, downloadCatalogTemplate, type CatalogFileFormat } from '../../api/client'
import { CatalogFilters } from '../../catalog/CatalogFilters'
import { CatalogPager } from '../../catalog/CatalogPager'
import { CatalogTable } from '../../catalog/CatalogTable'
import { keys, pageOf, pageSize, sortFrom, sortSolutions, type CatalogSort } from '../../catalog/filter'
import { ImportOverlay } from '../../catalog/import/ImportOverlay'
import { newRobot, RobotOverlay } from '../../catalog/RobotOverlay'
import { useCatalogUndoShortcuts } from '../../catalog/undo'
import { useCatalogView } from '../../catalog/useCatalogView'
import { ActionBar } from '../../ui/ActionBar'
import { ChevronDownIcon } from '../../ui/icons'
import { Popover } from '../../ui/Popover'
import { Reflow } from '../../ui/Reflow'
import { Select, type SelectOption } from '../../ui/Select'

const sortOptions: SelectOption<CatalogSort>[] = [
  { value: 'name', label: 'Название' },
  { value: 'price_rub', label: 'Цена' },
  { value: 'payload_kg', label: 'Грузоподъёмность' },
]

const templates: { layout: 'robot' | 'catalog'; format: CatalogFileFormat; label: string }[] = [
  { layout: 'robot', format: 'xlsx', label: 'Одно решение, XLSX' },
  { layout: 'robot', format: 'csv', label: 'Одно решение, CSV' },
  { layout: 'catalog', format: 'xlsx', label: 'Каталог, XLSX' },
  { layout: 'catalog', format: 'csv', label: 'Каталог, CSV' },
]

// AdminCatalog is the catalog for admins: the archive, edits that save themselves in the robot overlay, and
// uploads of whole tables.
export function AdminCatalog() {
  const view = useCatalogView(true)
  const [importing, setImporting] = useState(false)
  const [note, setNote] = useState('')
  useCatalogUndoShortcuts()

  const sort = sortFrom(view.params.get(keys.sort), sortOptions)
  const ordered = useMemo(() => sortSolutions(view.filtered, sort), [view.filtered, sort])
  const page = pageOf(view.params, ordered.length)
  const from = (page - 1) * pageSize

  const download = (run: () => Promise<void>) => {
    setNote('')
    run().catch((err: unknown) => setNote(err instanceof Error ? err.message : 'Не удалось скачать файл. Повторите.'))
  }

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
        <Reflow>{view.items.length === 0 ? 'Каталог пуст. Добавьте решение или загрузите таблицу.' : 'Под фильтр не подходит ни один робот. Измените фильтр.'}</Reflow>
      </p>
    )
  } else {
    body = <CatalogTable rows={ordered.slice(from, from + pageSize)} onOpen={(s) => view.open(s.id)} admin />
  }

  return (
    <section>
      <h1 className="sr-only">Каталог</h1>
      <CatalogFilters
        items={view.items}
        filter={view.filter}
        match={view.match}
        onChange={view.setFilter}
        admin
        sort={<Select prefix="Сортировка" value={sort} options={sortOptions} onChange={(v) => view.setSort(v, 'name')} />}
      />
      {body}
      <CatalogPager total={ordered.length} page={page} onPage={view.setPage} />
      <CatalogTools status={note} onNew={() => view.open(newRobot)} onImport={() => setImporting(true)} onDownload={download} />
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
          admin
        />
      ) : null}
      {importing ? (
        <ImportOverlay
          onClose={() => setImporting(false)}
          onOpenRobot={(id) => {
            setImporting(false)
            void view.query.refetch().then(() => view.open(id))
          }}
        />
      ) : null}
    </section>
  )
}

type ToolsProps = {
  status: string
  onNew: () => void
  onImport: () => void
  onDownload: (run: () => Promise<void>) => void
}

// CatalogTools is the action bar: a new robot first, then the files. Below 1280 px the files fold into «Действия».
function CatalogTools({ status, onNew, onImport, onDownload }: ToolsProps) {
  const [menu, setMenu] = useState<'templates' | 'narrow' | null>(null)
  const act = (fn: () => void) => () => {
    setMenu(null)
    fn()
  }
  const template = (t: (typeof templates)[number]) => act(() => onDownload(() => downloadCatalogTemplate(t.layout, t.format)))
  return (
    <ActionBar label="Действия с каталогом" status={status || undefined}>
      <button type="button" className="btn btn-primary" onClick={onNew}>
        Новое решение
      </button>
      <div className="tools-wide">
        <button type="button" className="btn btn-text" onClick={onImport}>
          Загрузить таблицу
        </button>
        <button type="button" className="btn btn-text" onClick={() => onDownload(() => downloadCatalog('xlsx'))}>
          Скачать каталог
        </button>
        <Popover
          open={menu === 'templates'}
          onOpenChange={(open) => setMenu(open ? 'templates' : null)}
          label="Шаблоны"
          align="end"
          side="above"
          trigger={(t) => (
            <button type="button" className="btn btn-text" {...t}>
              Шаблоны
              <ChevronDownIcon size={16} />
            </button>
          )}
        >
          {templates.map((t) => (
            <button key={t.label} type="button" className="menu-item" onClick={template(t)}>
              {t.label}
            </button>
          ))}
        </Popover>
      </div>
      <Popover
        open={menu === 'narrow'}
        onOpenChange={(open) => setMenu(open ? 'narrow' : null)}
        label="Действия с каталогом"
        align="end"
        side="above"
        className="tools-narrow"
        trigger={(t) => (
          <button type="button" className="btn btn-text" {...t}>
            Действия
            <ChevronDownIcon size={16} />
          </button>
        )}
      >
        <button type="button" className="menu-item" onClick={act(onImport)}>
          Загрузить таблицу
        </button>
        <button type="button" className="menu-item" onClick={act(() => onDownload(() => downloadCatalog('xlsx')))}>
          Скачать каталог
        </button>
        {templates.map((t) => (
          <button key={t.label} type="button" className="menu-item" onClick={template(t)}>
            {`Шаблон: ${t.label}`}
          </button>
        ))}
      </Popover>
    </ActionBar>
  )
}
