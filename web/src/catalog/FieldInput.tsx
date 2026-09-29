import type { CatalogField } from '../api/client'
import { CheckSelect } from '../ui/CheckSelect'
import { noData } from '../ui/noData'
import { NumberField } from '../ui/NumberField'
import { Select } from '../ui/Select'
import { TextField } from '../ui/TextField'
import { UnitField } from '../ui/UnitField'
import { valueText } from './fields'
import { choiceOptions } from './values'

// inRange checks a typed number against the field's range, in the units the field shows.
function inRange(f: CatalogField, shown: number): boolean {
  const v = f.percent ? shown / 100 : shown
  if (f.integer && !Number.isInteger(v)) {
    return false
  }
  return (f.min === undefined || v >= f.min) && (f.max === undefined || v <= f.max)
}

type Props = {
  field: CatalogField
  value: unknown
  id: string
  // onSave gets the value in stored units; null clears the field.
  onSave: (v: unknown) => void
}

// Edits one catalog field by its kind. A share is typed as percent, a choice may
// stay «Неизвестно».
export function FieldInput({ field: f, value, id, onSave }: Props) {
  const text = typeof value === 'string' ? value : ''
  switch (f.kind) {
    case 'text':
      return <TextField id={id} value={text} placeholder={noData} maxLength={f.max_len} onCommit={(t) => onSave(t === '' ? null : t)} />
    case 'url':
      return <TextField id={id} type="url" value={text} placeholder={noData} maxLength={f.max_len} onCommit={(t) => onSave(t === '' ? null : t)} />
    case 'date':
      return <TextField id={id} type="date" value={text.slice(0, 10)} onCommit={(t) => onSave(t === '' ? null : t)} />
    case 'number': {
      const n = typeof value === 'number' ? (f.percent ? value * 100 : value) : null
      const input = (
        <NumberField
          id={id}
          optional
          placeholder={noData}
          value={n}
          valid={(v) => inRange(f, v)}
          onCommit={(v) => onSave(v === undefined ? null : f.percent ? v / 100 : v)}
        />
      )
      return f.unit ? <UnitField unit={f.unit}>{input}</UnitField> : input
    }
    case 'choice':
      return (
        <Select id={id} value={text} options={[{ value: '', label: 'Неизвестно' }, ...choiceOptions(f)]} onChange={(v) => onSave(v === '' ? null : v)} />
      )
    case 'choices':
      return (
        <CheckSelect
          label={f.label}
          options={choiceOptions(f)}
          value={Array.isArray(value) ? value.filter((v): v is string => typeof v === 'string') : []}
          onChange={(v) => onSave(v)}
          empty="Вариантов нет."
        />
      )
    default:
      return <span id={id}>{valueText(f, value, noData)}</span>
  }
}
