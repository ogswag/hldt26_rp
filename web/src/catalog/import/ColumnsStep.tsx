import { useId } from 'react'

import type { CatalogField, CatalogImport } from '../../api/client'
import { cutTitle } from '../../ui/cutTitle'
import { Reflow } from '../../ui/Reflow'
import { Select, type SelectOption } from '../../ui/Select'
import { fieldByCode } from '../fields'
import { columnName, sampleText, type Mapping } from './plan'

type Props = {
  imp: CatalogImport
  // mapping is what the columns fill, the admin's last choice included while it is being saved.
  mapping: Mapping
  fields: readonly CatalogField[]
  onSheet: (sheet: number) => void
  onMapping: (mapping: Mapping) => void
}

// ColumnsStep shows each column of the file with a few of its values and the field it fills.
export function ColumnsStep({ imp, mapping, fields, onSheet, onMapping }: Props) {
  const uid = useId()
  const options: SelectOption[] = [{ value: '', label: 'не загружать' }, ...fields.map((f) => ({ value: f.code, label: f.label }))]
  const sheets: SelectOption[] = imp.sheets.map((name, i) => ({ value: String(i), label: name }))

  // NOTE: a field fills one column, so choosing it for a column frees the column that had it.
  const setColumn = (index: number, code: string) => {
    const next: Mapping = {}
    for (const [col, f] of Object.entries(mapping)) {
      if (col !== String(index) && f !== code) {
        next[col] = f
      }
    }
    if (code) {
      next[String(index)] = code
    }
    onMapping(next)
  }

  return (
    <>
      {imp.sheets.length > 1 ? (
        <div className="field import-sheet">
          <label htmlFor={`${uid}-sheet`}>Лист</label>
          <Select id={`${uid}-sheet`} value={String(imp.sheet)} options={sheets} onChange={(v) => onSheet(Number(v))} />
        </div>
      ) : null}
      {imp.mapping_error ? (
        <p className="error" role="alert">
          <Reflow>{imp.mapping_error}</Reflow>
        </p>
      ) : null}
      <div className="table-wrap">
        <table className="import-columns">
          <colgroup>
            <col className="is-column" />
            <col />
            <col className="is-field" />
          </colgroup>
          <thead>
            <tr>
              <th>Колонка в файле</th>
              <th>Значения</th>
              <th>Поле каталога</th>
            </tr>
          </thead>
          <tbody>
            {imp.columns.map((c) => {
              const name = columnName(c)
              const field = fieldByCode(fields, mapping[String(c.index)] ?? '')
              const samples = c.samples.map((v) => sampleText(field, v)).join('; ')
              return (
                <tr key={c.index}>
                  <td>{name}</td>
                  <td>
                    <div className="clamp-2 cell-note" onMouseEnter={cutTitle(samples)}>
                      {samples || 'пусто'}
                    </div>
                  </td>
                  <td>
                    <Select aria-label={`Поле для колонки «${name}»`} value={mapping[String(c.index)] ?? ''} options={options} onChange={(v) => setColumn(c.index, v)} />
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </>
  )
}
