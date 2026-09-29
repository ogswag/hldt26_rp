import { Fragment, useEffect, useId, useMemo, useRef, useState, type ReactNode } from 'react'

import { downloadImportErrors, type CatalogField, type CatalogImport, type FieldDiff, type ImportMode, type ImportPlan, type ImportRow, type RowClass } from '../../api/client'
import { CheckSelect } from '../../ui/CheckSelect'
import { numberText } from '../../ui/numberText'
import { Reflow } from '../../ui/Reflow'
import { familyOf, type Family } from '../families'
import { FamilyMark } from '../FamilyMark'
import { fieldByCode, fieldLabel, valueText } from '../fields'
import { useCatalog } from '../useCatalog'
import { columnOf, fieldCodes, matchesCatalog, outcome, pickOf, sampleText, shownRows, takenDiffs, withPick, type Choice, type Side } from './plan'

const classTags: { cls: RowClass; label: string; tag: string }[] = [
  { cls: 'new', label: 'новые', tag: 'tag-info' },
  { cls: 'changed', label: 'изменены', tag: '' },
  { cls: 'conflict', label: 'конфликты', tag: 'tag-warning' },
  { cls: 'unchanged', label: 'без изменений', tag: '' },
  { cls: 'error', label: 'с ошибками', tag: 'tag-danger' },
]

type Props = {
  imp: CatalogImport
  plan: ImportPlan
  fields: readonly CatalogField[]
  choice: Choice
  onChoice: (c: Choice) => void
  onMode: (mode: ImportMode) => void
  onDownload: (run: () => Promise<void>) => void
}

function twoWayText(imp: CatalogImport): string {
  const why = imp.stamp ? 'Выгрузка, из которой сделан файл, не найдена' : 'В файле нет колонки «Выгрузка»'
  return `${why}, поэтому файл сравнивается с каталогом напрямую: заполненная ячейка заменяет значение, пустая его не трогает.`
}

// CheckStep previews the upload: what each robot of the file would do, which fields to take and which side of a
// conflict wins.
export function CheckStep({ imp, plan, fields, choice, onChoice, onMode, onDownload }: Props) {
  const label = (code: string) => fieldLabel(fields, code)
  const fieldOptions = fieldCodes(imp).map((code) => ({ value: code, label: label(code) }))
  const other: ImportMode = imp.mode === 'robot' ? 'catalog' : 'robot'
  const robot = imp.mode === 'robot' ? plan.rows[0] : undefined
  const tags = classTags.filter((t) => (plan.counts[t.cls] ?? 0) > 0)
  const catalog = useCatalog(true).data?.items
  const known = useMemo(() => new Map((catalog ?? []).map((x) => [x.id, x.family])), [catalog])
  // familyFor is a row's group: the file's value when it has one, else the catalog robot's.
  const familyFor = (row: ImportRow): Family => {
    const fromFile = row.fields.find((d) => d.code === 'family')?.file
    return familyOf(typeof fromFile === 'string' ? fromFile : row.id ? known.get(row.id) : null)
  }

  return (
    <>
      {imp.hint ? (
        <div className="stale-banner import-hint">
          <p>
            <Reflow>{imp.hint}</Reflow>
          </p>
          <button type="button" className="btn" onClick={() => onMode(other)}>
            {other === 'robot' ? 'Загрузить как один робот' : 'Загрузить как каталог'}
          </button>
        </div>
      ) : null}
      {!plan.three_way && matchesCatalog(plan) ? (
        <p className="note-muted">
          <Reflow>{twoWayText(imp)}</Reflow>
        </p>
      ) : null}
      <div className="import-tools">
        {robot ? null : (
          <p className="import-counts">
            {tags.map((t) => (
              <span key={t.cls} className={t.tag ? `tag ${t.tag}` : 'tag'}>{`${t.label}: ${numberText(plan.counts[t.cls] ?? 0)}`}</span>
            ))}
          </p>
        )}
        <div className="import-fields">
          <span aria-hidden="true">Поля</span>
          <CheckSelect label="Поля" options={fieldOptions} value={choice.fields} onChange={(v) => onChoice({ ...choice, fields: v })} empty="Колонок с полями нет." />
        </div>
        {(plan.counts.error ?? 0) > 0 ? (
          <button type="button" className="btn btn-text" onClick={() => onDownload(() => downloadImportErrors(imp.id, 'xlsx'))}>
            Скачать отчёт об ошибках
          </button>
        ) : null}
      </div>
      {robot ? (
        <RobotCheck imp={imp} row={robot} threeWay={plan.three_way} fields={fields} choice={choice} onChoice={onChoice} />
      ) : (
        <RowsTable plan={plan} fields={fields} choice={choice} onChoice={onChoice} familyFor={familyFor} />
      )}
      {plan.missing.length > 0 ? <Missing plan={plan} archive={choice.archive} onArchive={(archive) => onChoice({ ...choice, archive })} /> : null}
    </>
  )
}

type RowsProps = {
  plan: ImportPlan
  fields: readonly CatalogField[]
  choice: Choice
  onChoice: (c: Choice) => void
  familyFor: (row: ImportRow) => Family
}

function RowsTable({ plan, fields, choice, onChoice, familyFor }: RowsProps) {
  const allRef = useRef<HTMLInputElement>(null)
  const [open, setOpen] = useState<ReadonlySet<string>>(new Set())
  const rows = shownRows(plan)
  const selectable = rows.filter((r) => r.class !== 'error')
  const all = selectable.length > 0 && selectable.every((r) => choice.rows.has(r.key))
  const some = selectable.some((r) => choice.rows.has(r.key))
  const label = (code: string) => fieldLabel(fields, code)

  useEffect(() => {
    if (allRef.current) {
      allRef.current.indeterminate = some && !all
    }
  }, [some, all])

  const toggle = (key: string, on: boolean) => {
    const next = new Set(choice.rows)
    if (on) {
      next.add(key)
    } else {
      next.delete(key)
    }
    onChoice({ ...choice, rows: next })
  }
  const toggleAll = () => {
    const next = new Set(choice.rows)
    selectable.forEach((r) => (all ? next.delete(r.key) : next.add(r.key)))
    onChoice({ ...choice, rows: next })
  }
  const flip = (key: string) => {
    const next = new Set(open)
    if (!next.delete(key)) {
      next.add(key)
    }
    setOpen(next)
  }

  if (rows.length === 0) {
    return (
      <p>
        <Reflow>Файл совпадает с каталогом: загружать нечего.</Reflow>
      </p>
    )
  }
  return (
    <div className="table-wrap">
      <table className="import-rows">
        <colgroup>
          <col className="is-check" />
          <col className="is-line" />
          <col className="is-name" />
          <col />
        </colgroup>
        <thead>
          <tr>
            <th>
              <input ref={allRef} type="checkbox" aria-label="Все строки" checked={all} disabled={selectable.length === 0} onChange={toggleAll} />
            </th>
            <th className="num">Строка</th>
            <th>Название</th>
            <th>Что будет</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const diffs = takenDiffs(row, choice.fields)
            const expandable = (row.class === 'changed' || row.class === 'conflict') && diffs.length > 0
            const expanded = expandable && open.has(row.key)
            const name = row.name || 'Без названия'
            return (
              <Fragment key={row.key}>
                <tr className={row.class === 'error' ? 'is-error' : undefined}>
                  <td>
                    <input
                      type="checkbox"
                      aria-label={`Загрузить строку ${numberText(row.line)}, «${name}»`}
                      checked={row.class !== 'error' && choice.rows.has(row.key)}
                      disabled={row.class === 'error'}
                      onChange={(e) => toggle(row.key, e.target.checked)}
                    />
                  </td>
                  <td className="num">{numberText(row.line)}</td>
                  <td>
                    <span className="import-row-name">
                      <FamilyMark family={familyFor(row)} />
                      <span className="cut-line">{name}</span>
                      {row.archived ? <span className="tag">в архиве</span> : null}
                    </span>
                  </td>
                  <td className={row.class === 'error' ? 'error' : undefined}>
                    <Reflow>{outcome(row, choice.fields, label)}</Reflow>
                    {expandable ? (
                      <>
                        {' '}
                        <button type="button" className="linkish" aria-expanded={expanded} onClick={() => flip(row.key)}>
                          {expanded ? 'Скрыть' : 'Сравнить'}
                        </button>
                      </>
                    ) : null}
                  </td>
                </tr>
                {expanded ? (
                  <tr className="import-diff-row">
                    <td />
                    <td colSpan={3}>
                      <DiffTable row={row} diffs={diffs} threeWay={plan.three_way} fields={fields} choice={choice} onChoice={onChoice} />
                    </td>
                  </tr>
                ) : null}
              </Fragment>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

type DiffProps = {
  row: ImportRow
  diffs: readonly FieldDiff[]
  threeWay: boolean
  fields: readonly CatalogField[]
  choice: Choice
  onChoice: (c: Choice) => void
}

// DiffTable compares a robot's fields: at the export the file came from, in the catalog now, and in the file. The
// value that will be saved is marked; in a conflict the admin clicks the one to keep.
function DiffTable({ row, diffs, threeWay, fields, choice, onChoice }: DiffProps) {
  return (
    <table className="import-diff">
      <thead>
        <tr>
          <th>Поле</th>
          {threeWay ? <th>При выгрузке</th> : null}
          <th>Сейчас в каталоге</th>
          <th>В файле</th>
        </tr>
      </thead>
      <tbody>
        {diffs.map((d) => {
          const f = fieldByCode(fields, d.code)
          return (
            <tr key={d.code} className={d.conflict ? 'is-conflict' : undefined}>
              <td>{fieldLabel(fields, d.code)}</td>
              {threeWay ? <td>{valueText(f, d.base)}</td> : null}
              {d.conflict ? (
                <PickCells row={row.key} d={d} fields={fields} choice={choice} onChoice={onChoice} />
              ) : (
                <>
                  <td>{valueText(f, d.current)}</td>
                  <td className="is-taken">{valueText(f, d.file)}</td>
                </>
              )}
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

type PickProps = { row: string; d: FieldDiff; fields: readonly CatalogField[]; choice: Choice; onChoice: (c: Choice) => void }

// PickCells are the catalog's and the file's value of a conflicting field; the one clicked is saved.
function PickCells({ row, d, fields, choice, onChoice }: PickProps) {
  const name = useId()
  const f = fieldByCode(fields, d.code)
  const label = fieldLabel(fields, d.code)
  const side = pickOf(choice.picks, row, d.code)
  const cell = (s: Side, value: unknown, from: string) => {
    const text = valueText(f, value)
    return (
      <td className={side === s ? 'import-pick is-picked' : 'import-pick'}>
        <label>
          <input
            type="radio"
            className="sr-only"
            name={name}
            checked={side === s}
            aria-label={`${label}: ${from}, ${text}`}
            onChange={() => onChoice({ ...choice, picks: withPick(choice.picks, row, d.code, s) })}
          />
          {text}
        </label>
      </td>
    )
  }
  return (
    <>
      {cell('catalog', d.current, 'из каталога')}
      {cell('file', d.file, 'из файла')}
    </>
  )
}

type RobotProps = {
  imp: CatalogImport
  row: ImportRow
  threeWay: boolean
  fields: readonly CatalogField[]
  choice: Choice
  onChoice: (c: Choice) => void
}

// RobotCheck shows the one robot of a one-robot upload field by field, with the catalog's value where it differs
// and the reason next to a value that did not read.
function RobotCheck({ imp, row, threeWay, fields, choice, onChoice }: RobotProps) {
  const errors = row.errors ?? []
  const columns = imp.columns.flatMap((c) => (c.field && c.field !== 'id' && c.field !== 'stamp' ? [{ index: c.index, code: c.field, sample: c.samples[0] }] : []))
  const known = new Set(columns.map((c) => c.code))
  const loose = errors.filter((e) => !known.has(e.field))
  const exists = row.class === 'changed' || row.class === 'conflict' || row.class === 'unchanged'
  const label = (code: string) => fieldLabel(fields, code)

  return (
    <section className="import-robot">
      <h3 className="import-robot-title">{row.name || 'Без названия'}</h3>
      <p className={row.class === 'error' ? 'error' : 'robot-meta'}>
        <Reflow>{outcome(row, choice.fields, label)}</Reflow>
      </p>
      {loose.map((e) => (
        <p key={e.field} className="error">
          <Reflow>{e.message}</Reflow>
        </p>
      ))}
      {row.duplicate_of ? (
        <label className="check-row">
          <input
            type="checkbox"
            checked={choice.rows.has(row.key)}
            onChange={(e) => {
              const next = new Set(choice.rows)
              if (e.target.checked) {
                next.add(row.key)
              } else {
                next.delete(row.key)
              }
              onChoice({ ...choice, rows: next })
            }}
          />
          Всё равно добавить как новое решение
        </label>
      ) : null}
      <div className="table-wrap">
        <table className="import-diff import-robot-fields">
          <thead>
            <tr>
              <th>Поле</th>
              {exists && threeWay ? <th>При выгрузке</th> : null}
              {exists ? <th>Сейчас в каталоге</th> : null}
              <th>В файле</th>
              <th>
                <span className="sr-only">Что будет</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {columns.map(({ index, code, sample }) => {
              const f = fieldByCode(fields, code)
              const d = row.fields.find((x) => columnOf(x.code) === code)
              const wrong = errors.filter((e) => e.field === code)
              const taken = choice.fields.includes(code) || (row.class === 'new' && code === 'name')
              let note: ReactNode = ''
              if (wrong.length > 0) {
                note = <span className="error">{wrong.map((e) => e.message).join(' ')}</span>
              } else if (d && !taken) {
                note = 'поле не выбрано'
              }
              const pick = Boolean(d?.conflict && taken && wrong.length === 0)
              const fileText = d ? valueText(f, d.file) : sample ? sampleText(f, sample) : 'пусто'
              return (
                <tr key={index} className={wrong.length > 0 ? 'is-error' : d?.conflict ? 'is-conflict' : undefined}>
                  <td>{label(code)}</td>
                  {exists && threeWay ? <td>{d ? valueText(f, d.base) : ''}</td> : null}
                  {pick && d ? (
                    <PickCells row={row.key} d={d} fields={fields} choice={choice} onChoice={onChoice} />
                  ) : (
                    <>
                      {exists ? <td>{d ? valueText(f, d.current) : 'совпадает'}</td> : null}
                      <td className={d && taken && exists ? 'is-taken' : undefined}>{fileText}</td>
                    </>
                  )}
                  <td>{note}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </section>
  )
}

function Missing({ plan, archive, onArchive }: { plan: ImportPlan; archive: boolean; onArchive: (archive: boolean) => void }) {
  const name = useId()
  return (
    <fieldset className="import-choice import-missing">
      <legend>{`В каталоге, но не в файле: ${numberText(plan.missing.length)}`}</legend>
      <label className="check-row">
        <input type="radio" name={name} checked={!archive} onChange={() => onArchive(false)} />
        Оставить
      </label>
      <label className="check-row">
        <input type="radio" name={name} checked={archive} onChange={() => onArchive(true)} />
        В архив
      </label>
      <details>
        <summary>Кого нет в файле</summary>
        <p>
          <Reflow>{plan.missing.map((m) => m.name).join(', ')}</Reflow>
        </p>
      </details>
    </fieldset>
  )
}
