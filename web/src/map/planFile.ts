import type { MapSourceKind } from '../api/client'
import { plural } from '../fleet/format'
import { numberText } from '../ui/numberText'

export const maxPlanBytes = 30 * 1024 * 1024
const maxPlanSide = 4000

export type Plan = {
  kind: MapSourceKind
  name: string
  width: number
  height: number
  canvas: HTMLCanvasElement
  note: string
}

export class PlanError extends Error {}

function kindOf(file: File): MapSourceKind | null {
  const name = file.name.toLowerCase()
  if (file.type === 'application/pdf' || name.endsWith('.pdf')) {
    return 'pdf'
  }
  if (file.type === 'image/png' || name.endsWith('.png')) {
    return 'png'
  }
  if (file.type === 'image/jpeg' || name.endsWith('.jpg') || name.endsWith('.jpeg')) {
    return 'jpeg'
  }
  return null
}

function canvasOf(width: number, height: number): HTMLCanvasElement {
  const c = document.createElement('canvas')
  c.width = Math.max(1, Math.round(width))
  c.height = Math.max(1, Math.round(height))
  return c
}

async function loadImage(file: File, kind: MapSourceKind): Promise<Plan> {
  const bitmap = await createImageBitmap(file).catch(() => {
    throw new PlanError('Не удалось прочитать изображение. Проверьте, что файл PNG или JPEG не повреждён.')
  })
  const scale = Math.min(1, maxPlanSide / Math.max(bitmap.width, bitmap.height))
  const canvas = canvasOf(bitmap.width * scale, bitmap.height * scale)
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    throw new PlanError('Браузер не дал нарисовать план. Обновите страницу и повторите.')
  }
  ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
  bitmap.close()
  return {
    kind,
    name: file.name,
    width: canvas.width,
    height: canvas.height,
    canvas,
    note: scale < 1 ? `План уменьшен до ${canvas.width} x ${canvas.height} пикселей.` : '',
  }
}

export function pagesNote(pages: number): string {
  if (pages <= 1) {
    return ''
  }
  const word = plural(pages, 'страница', 'страницы', 'страниц')
  return `В PDF ${numberText(pages)} ${word}. Используется только первая, многостраничные планы пока не поддерживаются.`
}

async function loadPdf(file: File): Promise<Plan> {
  const pdfjs = await import('pdfjs-dist')
  const worker = await import('pdfjs-dist/build/pdf.worker.min.mjs?url')
  pdfjs.GlobalWorkerOptions.workerSrc = worker.default
  const data = new Uint8Array(await file.arrayBuffer())
  const task = pdfjs.getDocument({ data })
  let doc
  try {
    doc = await task.promise
  } catch {
    void task.destroy()
    throw new PlanError('Не удалось открыть PDF. Файл повреждён или защищён паролем.')
  }
  try {
    const page = await doc.getPage(1)
    const base = page.getViewport({ scale: 1 })
    const scale = Math.min(4, maxPlanSide / Math.max(base.width, base.height))
    const viewport = page.getViewport({ scale })
    const canvas = canvasOf(viewport.width, viewport.height)
    await page.render({ canvas, viewport }).promise
    return { kind: 'pdf', name: file.name, width: canvas.width, height: canvas.height, canvas, note: pagesNote(doc.numPages) }
  } finally {
    void task.destroy()
  }
}

// loadPlan renders the plan locally. The file never leaves the browser.
export async function loadPlan(file: File): Promise<Plan> {
  const kind = kindOf(file)
  if (!kind) {
    throw new PlanError('Поддерживаются только PNG, JPEG и PDF. Сохраните план в одном из этих форматов.')
  }
  if (file.size > maxPlanBytes) {
    throw new PlanError('Файл больше 30 МБ. Уменьшите разрешение или экспортируйте одну страницу.')
  }
  return kind === 'pdf' ? loadPdf(file) : loadImage(file, kind)
}

const dbName = 'robot-plans'
const storeName = 'plans'

function openDb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(dbName, 1)
    req.onupgradeneeded = () => req.result.createObjectStore(storeName)
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}

type StoredPlan = { kind: MapSourceKind; name: string; blob: Blob }

// savePlanLocally keeps the rendered plan in this browser only, so a reload does not lose the background.
export async function savePlanLocally(projectId: string, plan: Plan): Promise<void> {
  try {
    const blob = await new Promise<Blob | null>((resolve) => plan.canvas.toBlob(resolve, 'image/png'))
    if (!blob) {
      return
    }
    const db = await openDb()
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(storeName, 'readwrite')
      tx.objectStore(storeName).put({ kind: plan.kind, name: plan.name, blob } satisfies StoredPlan, projectId)
      tx.oncomplete = () => resolve()
      tx.onerror = () => reject(tx.error)
    })
    db.close()
  } catch {
    // NOTE: local storage is a convenience; the map works without the background.
  }
}

export async function loadLocalPlan(projectId: string): Promise<Plan | null> {
  try {
    const db = await openDb()
    const stored = await new Promise<StoredPlan | undefined>((resolve, reject) => {
      const tx = db.transaction(storeName, 'readonly')
      const req = tx.objectStore(storeName).get(projectId)
      req.onsuccess = () => resolve(req.result as StoredPlan | undefined)
      req.onerror = () => reject(req.error)
    })
    db.close()
    if (!stored) {
      return null
    }
    const file = new File([stored.blob], stored.name, { type: 'image/png' })
    const plan = await loadImage(file, 'png')
    return { ...plan, kind: stored.kind, name: stored.name, note: '' }
  } catch {
    return null
  }
}

export async function forgetLocalPlan(projectId: string): Promise<void> {
  try {
    const db = await openDb()
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(storeName, 'readwrite')
      tx.objectStore(storeName).delete(projectId)
      tx.oncomplete = () => resolve()
      tx.onerror = () => reject(tx.error)
    })
    db.close()
  } catch {
    // NOTE: nothing to clean when IndexedDB is unavailable.
  }
}
