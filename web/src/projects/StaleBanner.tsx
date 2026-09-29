import { Link } from 'react-router-dom'

import { Reflow } from '../ui/Reflow'

type Props = {
  stale: boolean
  hrefHistory?: string
}

export function StaleBanner({ stale, hrefHistory }: Props) {
  if (!stale) {
    return null
  }
  return (
    <p className="stale-banner">
      <Reflow>
        Входы изменились после последнего расчёта. Показан старый запуск, он остаётся в истории. Пересчитайте, чтобы
        получить актуальный результат.
        {hrefHistory ? (
          <>
            {' '}
            <Link to={hrefHistory}>История запусков</Link>
          </>
        ) : null}
      </Reflow>
    </p>
  )
}
