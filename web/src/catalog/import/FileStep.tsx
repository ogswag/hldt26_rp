import { useId } from 'react'

import { downloadCatalog, downloadCatalogTemplate, type ImportMode } from '../../api/client'
import { FileDrop } from '../../ui/FileDrop'
import { Reflow } from '../../ui/Reflow'
import { modeLabels } from './plan'

const fileTypes = '.xlsx,.xls,.csv,.txt,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,application/vnd.ms-excel,text/csv'

type Props = {
  mode: ImportMode
  fileName: string | null
  busy: boolean
  onMode: (mode: ImportMode) => void
  onFile: (file: File) => void
  // onDownload runs a download and reports its failure in the overlay.
  onDownload: (run: () => Promise<void>) => void
}

// FileStep takes the file and what it holds: one robot or the whole catalog.
export function FileStep(p: Props) {
  const name = useId()
  return (
    <>
      <FileDrop accept={fileTypes} label="Файл с каталогом" onFile={p.onFile} disabled={p.busy} className="import-drop">
        {(choose) => (
          <p>
            Перетащите файл сюда или{' '}
            <button type="button" className="linkish" disabled={p.busy} onClick={choose}>
              выберите его
            </button>
            . Подходят XLS, XLSX и CSV.
          </p>
        )}
      </FileDrop>
      {p.fileName ? (
        <p className="import-file">
          <Reflow>{`Файл: ${p.fileName}`}</Reflow>
        </p>
      ) : null}
      <fieldset className="import-choice" disabled={p.busy}>
        <legend>Что в файле</legend>
        {(['robot', 'catalog'] as const).map((m) => (
          <label key={m} className="check-row">
            <input type="radio" name={name} checked={p.mode === m} onChange={() => p.onMode(m)} />
            {modeLabels[m]}
          </label>
        ))}
      </fieldset>
      <p className="import-links">
        <Reflow>Шаблоны:</Reflow>{' '}
        <button type="button" className="linkish" onClick={() => p.onDownload(() => downloadCatalogTemplate('robot', 'xlsx'))}>
          одного решения
        </button>
        ,{' '}
        <button type="button" className="linkish" onClick={() => p.onDownload(() => downloadCatalogTemplate('catalog', 'xlsx'))}>
          каталога
        </button>
        . <Reflow>Чтобы поправить каталог, </Reflow>
        <button type="button" className="linkish" onClick={() => p.onDownload(() => downloadCatalog('xlsx'))}>
          скачайте его
        </button>
        <Reflow>, исправьте и загрузите обратно.</Reflow>
      </p>
    </>
  )
}
