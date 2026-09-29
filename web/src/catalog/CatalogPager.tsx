import { numberText } from '../ui/numberText'
import { Reflow } from '../ui/Reflow'
import { pageSize } from './filter'

// CatalogPager moves through a list longer than one page; a list that fits one page has none.
export function CatalogPager({ total, page, onPage }: { total: number; page: number; onPage: (page: number) => void }) {
  const pages = Math.ceil(total / pageSize)
  if (pages <= 1) {
    return null
  }
  const from = (page - 1) * pageSize + 1
  const to = Math.min(page * pageSize, total)
  return (
    <div className="catalog-pager">
      <p>
        <Reflow>{`Показаны с ${numberText(from)} по ${numberText(to)} из ${numberText(total)}.`}</Reflow>
      </p>
      <div className="actions">
        <button type="button" disabled={page <= 1} onClick={() => onPage(page - 1)}>
          Назад
        </button>
        <button type="button" disabled={page >= pages} onClick={() => onPage(page + 1)}>
          Дальше
        </button>
      </div>
    </div>
  )
}
