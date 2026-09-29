import { Link } from 'react-router-dom'

import type { ImportSummary, RobotRef } from '../../api/client'
import { useAdminHref } from '../../pages/admin/adminHref'
import { numberText } from '../../ui/numberText'
import { Reflow } from '../../ui/Reflow'

// NOTE: longer lists end with a count; the journal lists every robot.
const namesShown = 10

function names(list: readonly RobotRef[]): string {
  const shown = list.slice(0, namesShown).map((r) => `«${r.name}»`)
  const rest = list.length - shown.length
  return rest > 0 ? `${shown.join(', ')} и ещё ${numberText(rest)}` : shown.join(', ')
}

// DoneStep reports what the upload saved and how the photo links are loading.
export function DoneStep({ summary, loading }: { summary: ImportSummary; loading: boolean }) {
  const adminHref = useAdminHref()
  const { created, updated, archived } = summary.applied
  const rows: [string, readonly RobotRef[]][] = [
    ['Добавлены', created],
    ['Изменены', updated],
    ['Убраны в архив', archived],
  ]
  const photosOk = summary.photos_done - summary.photo_errors.length

  return (
    <>
      <dl className="robot-rows import-summary">
        {rows.map(([label, list]) =>
          list.length > 0 ? (
            <div key={label}>
              <dt>{label}</dt>
              <dd>
                <Reflow>{`${numberText(list.length)}: ${names(list)}`}</Reflow>
              </dd>
            </div>
          ) : null,
        )}
        <div>
          <dt>Без изменений</dt>
          <dd>{numberText(summary.unchanged)}</dd>
        </div>
      </dl>
      {summary.photos_total > 0 ? (
        loading ? (
          <div className="job-progress" role="status">
            <p>
              <Reflow>{`Загружаем фото по ссылкам: ${numberText(summary.photos_done)} из ${numberText(summary.photos_total)}.`}</Reflow>
            </p>
            <progress max={summary.photos_total} value={summary.photos_done} aria-label="Фото по ссылкам" />
          </div>
        ) : (
          <p>
            <Reflow>{`Фото по ссылкам загружены: ${numberText(photosOk)} из ${numberText(summary.photos_total)}.`}</Reflow>
          </p>
        )
      ) : null}
      {summary.photo_errors.length > 0 ? (
        <div className="import-photo-errors">
          <p className="error">
            <Reflow>Эти фото не загрузились. Проверьте ссылки или загрузите фото в карточке робота.</Reflow>
          </p>
          <ul>
            {summary.photo_errors.map((e) => (
              <li key={`${e.solution_id}-${e.url}`}>
                <Reflow>{`«${e.name}»: ${e.message}`}</Reflow>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {summary.photos_skipped ? (
        <p className="note-muted">
          <Reflow>{`Ссылок на фото не загружено: ${numberText(summary.photos_skipped)}. За одну загрузку берётся до 200 фото: загрузите файл ещё раз, чтобы взять остальные.`}</Reflow>
        </p>
      ) : null}
      {summary.photos_interrupted ? (
        <p className="note-muted">
          <Reflow>Загрузка фото прервалась: сервер перезапускался. Загрузите файл ещё раз, чтобы взять оставшиеся фото.</Reflow>
        </p>
      ) : null}
      <p>
        <Reflow>Каждая правка записана в журнал.</Reflow> <Link to={adminHref('audit')}>Открыть журнал</Link>
      </p>
    </>
  )
}
