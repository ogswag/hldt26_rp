import type { ParamValue, ParamsMap, SchemaField, SchemaGroup } from '../api/client'
import { FieldControl } from '../params/FieldControl'
import { helpText } from '../params/help'
import { unitOf } from '../params/unit'
import { FieldRow } from '../ui/FieldRow'
import { AbbreviationText } from '../ui/Abbreviation'
import { Reflow } from '../ui/Reflow'

type Props = {
  groups: SchemaGroup[]
  values: ParamsMap
  errors: Record<string, string>
  // onChange shows what is being typed; onCommit saves that field.
  onChange: (f: SchemaField, v: ParamValue | undefined) => void
  onCommit: (f: SchemaField) => void
  onEnter: (f: SchemaField) => void
  onLeave: (f: SchemaField) => void
}

const wideTypes = new Set(['multi_enum'])

function ParamField({ f, value, error, onChange, onCommit, onEnter, onLeave }: {
  f: SchemaField
  value: ParamValue | undefined
  error: string | undefined
  onChange: (v: ParamValue | undefined) => void
  onCommit: () => void
  onEnter: () => void
  onLeave: () => void
}) {
  const shown = f.short || f.label
  return (
    <FieldRow
      id={f.id}
      data-search-id={`field:${f.id}`}
      label={shown}
      fullLabel={f.label}
      unit={unitOf(f) || undefined}
      help={helpText(f)}
      wide={wideTypes.has(f.type)}
      data-field-type={f.type}
      data-composite={f.type === 'dimensions' ? 'dimensions' : undefined}
      onFocus={onEnter}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget)) {
          onLeave()
        }
      }}
    >
      <FieldControl field={f} value={value} onChange={onChange} onCommit={onCommit} />
      {error ? (
        <p className="error">
          <Reflow>{error}</Reflow>
        </p>
      ) : null}
    </FieldRow>
  )
}

// ParamGrid lays the fields of one tab out as rows: a one-line label, the field, and a "?" with the full name,
// note and range. Wide enough, the rows run in two columns. A tab of one group needs no group heading: the
// subtab already names it.
export function ParamGrid(p: Props) {
  const single = p.groups.length === 1
  return (
    <>
      {p.groups.map((g) => (
        <section key={g.id} className="param-section" data-search-id={`section:${g.id}`} aria-label={single ? g.label : undefined} aria-labelledby={single ? undefined : `group-${g.id}`}>
          {single ? null : <h2 id={`group-${g.id}`}><AbbreviationText text={g.label} /></h2>}
          <div className="field-rows is-quiet">
            {g.fields.map((f) => (
              <ParamField
                key={f.id}
                f={f}
                value={p.values[f.id]}
                error={p.errors[f.id]}
                onChange={(v) => p.onChange(f, v)}
                onCommit={() => p.onCommit(f)}
                onEnter={() => p.onEnter(f)}
                onLeave={() => p.onLeave(f)}
              />
            ))}
          </div>
        </section>
      ))}
    </>
  )
}
