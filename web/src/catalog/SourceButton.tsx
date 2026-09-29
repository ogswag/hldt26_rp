import { useState } from 'react'

import type { CatalogField, Solution } from '../api/client'
import { Popover } from '../ui/Popover'
import { Reflow } from '../ui/Reflow'
import { choiceLabel, dateText, fieldByCode } from './fields'
import { sourceOf } from './sources'
import { sourceHost } from './values'

type Props = {
  s: Solution
  field: CatalogField
  fields: readonly CatalogField[]
}

// SourceButton keeps where a value of the robot comes from behind an «i» next to the value: the link, how far the
// figure can be trusted, a note and the date.
export function SourceButton({ s, field, fields }: Props) {
  const [open, setOpen] = useState(false)
  const info = sourceOf(s, field.code)
  const trust = fieldByCode(fields, 'confidence')
  const label = `Откуда значение: ${field.label}`
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      label={label}
      align="end"
      className="field-help"
      trigger={(t) => (
        <button type="button" className="field-help-button" aria-label={label} title="Откуда значение" {...t}>
          i
        </button>
      )}
    >
      <p>
        {info.url ? (
          <a href={info.url} target="_blank" rel="noopener noreferrer">
            {sourceHost(info.url)}
          </a>
        ) : (
          'Ссылки на источник нет.'
        )}
      </p>
      {info.confidence && trust ? <p>{`Достоверность: ${choiceLabel(trust, info.confidence)}.`}</p> : null}
      {info.note ? (
        <p>
          <Reflow>{info.note}</Reflow>
        </p>
      ) : null}
      {info.date ? <p>{`Дата источника: ${dateText(info.date)}.`}</p> : null}
      {info.own ? null : (
        <p>
          <Reflow>У значения нет отдельного источника, показан источник карточки.</Reflow>
        </p>
      )}
    </Popover>
  )
}
