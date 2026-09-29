import type { ReactElement } from 'react'

import type { ParamValue, SchemaField, TimeRange } from '../api/client'
import { normalizeClock } from '../guest/store'
import { useSaveTimer } from '../ui/fieldDraft'
import { noData } from '../ui/noData'
import { NumberInput } from '../ui/NumberInput'
import { parseNumberText } from '../ui/numberText'
import { Select, type SelectOption } from '../ui/Select'
import { UnitField } from '../ui/UnitField'
import { asDimensions, dimensionParts } from './dimensions'
import { unitOf } from './unit'

type Props = {
  field: SchemaField
  value: ParamValue | undefined
  // onChange shows what is being typed; onCommit saves it. A picker does both at once.
  onChange: (v: ParamValue | undefined) => void
  onCommit: () => void
}

const yesNo: SelectOption[] = [
  { value: 'true', label: 'Да' },
  { value: 'false', label: 'Нет' },
]

const unknownOption: SelectOption = { value: 'unknown', label: 'Неизвестно' }

function withUnit(f: SchemaField, input: ReactElement): ReactElement {
  const unit = unitOf(f)
  return unit ? <UnitField unit={unit}>{input}</UnitField> : input
}

function isTimeRange(v: unknown): v is TimeRange {
  return Boolean(v) && typeof v === 'object' && !Array.isArray(v) && 'start' in (v as object) && 'end' in (v as object)
}

export function FieldControl({ field, value, onChange, onCommit }: Props) {
  const unknown = value === null
  const showUnknown = Boolean(field.allow_unknown)
  const { typed, finish } = useSaveTimer(onCommit)
  // A picker has no half-typed state: the choice is the whole value, so it is saved as soon as it is made.
  const pick = (v: ParamValue | undefined) => {
    onChange(v)
    onCommit()
  }

  if (field.type === 'boolean') {
    const current = unknown ? 'unknown' : value === true ? 'true' : value === false ? 'false' : field.default === true ? 'true' : 'false'
    return (
      <Select
        id={field.id}
        value={current}
        options={showUnknown ? [...yesNo, unknownOption] : yesNo}
        onChange={(v) => pick(v === 'unknown' ? null : v === 'true')}
      />
    )
  }

  if (field.type === 'enum') {
    const current = unknown ? 'unknown' : typeof value === 'string' ? value : String(field.default ?? '')
    const options = field.options ?? []
    return (
      <Select
        id={field.id}
        value={current}
        options={showUnknown ? [...options, unknownOption] : options}
        fitContent
        onChange={(v) => pick(v === 'unknown' ? null : v)}
      />
    )
  }

  if (field.type === 'multi_enum') {
    const selected = Array.isArray(value) ? value : []
    return (
      <div className="multi-enum">
        {(field.options ?? []).map((o) => {
          const checked = selected.includes(o.value)
          return (
            <label key={o.value} className="check-row">
              <input
                type="checkbox"
                checked={checked}
                onChange={(e) => pick(e.target.checked ? [...selected, o.value] : selected.filter((x) => x !== o.value))}
              />
              {o.label}
            </label>
          )
        })}
      </div>
    )
  }

  if (field.type === 'time_range') {
    const tr = isTimeRange(value) ? value : isTimeRange(field.default) ? field.default : { start: '08:00', end: '20:00' }
    return (
      <div className="time-range">
        <input
          id={field.id}
          type="time"
          step={60}
          aria-label="Начало"
          value={tr.start}
          onChange={(e) => pick({ start: normalizeClock(e.target.value) ?? e.target.value, end: tr.end })}
        />
        <span className="sr-only">до</span>
        <input
          type="time"
          step={60}
          aria-label="Окончание"
          value={tr.end}
          onChange={(e) => pick({ start: tr.start, end: normalizeClock(e.target.value) ?? e.target.value })}
        />
      </div>
    )
  }

  if (field.type === 'dimensions') {
    const d = asDimensions(value) ?? asDimensions(field.default) ?? { length: NaN, width: NaN, height: NaN }
    // The first part takes the field id, so the field label and error links land on it.
    return (
      <div className="dims" role="group" aria-label={field.label}>
        {dimensionParts.map((p, k) => {
          const id = k === 0 ? field.id : `${field.id}-${p.key}`
          const part = d[p.key]
          return (
            <div key={p.key} className="dims-part">
              <label htmlFor={id} className="dims-label">
                {p.label}
              </label>
              {withUnit(
                field,
                <NumberInput
                  id={id}
                  name={id}
                  value={part}
                  onText={(raw) => {
                    onChange({ ...d, [p.key]: parseNumberText(raw) ?? NaN })
                    typed()
                  }}
                  onBlur={finish}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      finish()
                    }
                  }}
                />,
              )}
            </div>
          )
        })}
      </div>
    )
  }

  if (field.type === 'string') {
    return withUnit(
      field,
      <input
        id={field.id}
        name={field.id}
        type="text"
        placeholder={noData}
        value={typeof value === 'string' ? value : ''}
        onChange={(e) => {
          onChange(e.target.value)
          typed()
        }}
        onBlur={finish}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            finish()
          }
        }}
      />,
    )
  }

  // NOTE: a number field that allows "unknown" is left empty for it; the "?" says so (params/help.ts).
  return withUnit(
    field,
    <NumberInput
      id={field.id}
      name={field.id}
      placeholder={noData}
      value={typeof value === 'number' ? value : undefined}
      onText={(raw) => {
        onChange(parseNumberText(raw))
        typed()
      }}
      onBlur={finish}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          finish()
        }
      }}
    />,
  )
}
