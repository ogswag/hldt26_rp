import { expect, test, type Locator, type Page } from '@playwright/test'
import * as XLSX from 'xlsx'

import { adminSession, adminSignIn, readWorkbook, sheetRows, type Session } from './helpers.ts'

type Robot = {
  id: string
  name: string
  archived_at: string | null
  image_sha: string | null
  specs: { payload_kg: number | null }
}

// NOTE: every robot these tests make starts with prefix; afterAll archives them, so no ranking ever shows them.
const prefix = `E2E каталог ${Date.now()}`
const xlsxType = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64')

async function createRobot(admin: Session, values: Record<string, unknown>): Promise<Robot> {
  const res = await admin.api.post('/api/admin/solutions', { data: values })
  expect(res.status(), await res.text()).toBe(201)
  return ((await res.json()) as { solution: Robot }).solution
}

async function robot(admin: Session, id: string): Promise<Robot> {
  const res = await admin.api.get(`/api/solutions/${id}`)
  expect(res.ok(), await res.text()).toBeTruthy()
  return (await res.json()) as Robot
}

test.afterAll(async () => {
  const admin = await adminSession()
  const res = await admin.api.get(`/api/solutions?limit=500&archived=include&q=${encodeURIComponent(prefix)}`)
  expect(res.ok(), await res.text()).toBeTruthy()
  for (const s of ((await res.json()) as { items: Robot[] }).items) {
    if (s.name.startsWith(prefix) && !s.archived_at) {
      const gone = await admin.api.delete(`/api/admin/solutions/${s.id}`)
      expect(gone.status(), await gone.text()).toBe(204)
    }
  }
  await admin.api.dispose()
})

async function openCatalog(page: Page, q: string): Promise<void> {
  await adminSignIn(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/admin/catalog')
  const search = page.getByRole('searchbox', { name: 'Поиск', exact: true })
  await search.fill(q)
  await search.press('Enter')
}

function patched(page: Page, id: string) {
  return page.waitForResponse((r) => r.request().method() === 'PATCH' && new URL(r.url()).pathname === `/api/admin/solutions/${id}`)
}

async function dropPhoto(zone: Locator): Promise<void> {
  const data = await zone.page().evaluateHandle((b64) => {
    const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))
    const dt = new DataTransfer()
    dt.items.add(new File([bytes], 'robot.png', { type: 'image/png' }))
    return dt
  }, png.toString('base64'))
  await zone.dispatchEvent('dragover', { dataTransfer: data })
  await zone.dispatchEvent('drop', { dataTransfer: data })
}

test('an admin edits a robot in the overlay, undoes the edit, copies, archives and adds a photo', async ({ page }) => {
  const admin = await adminSession()
  const first = await createRobot(admin, { name: `${prefix} А`, vendor: 'ООО «Проба»', family: 'mobile', price_rub: 1_000_000, payload_kg: 500 })
  const second = await createRobot(admin, { name: `${prefix} Б`, vendor: 'ООО «Проба»', family: 'uav' })
  await openCatalog(page, prefix)

  const rows = page.locator('.catalog-table tbody tr')
  await expect(rows).toHaveCount(2)
  await rows.first().locator('.robot-name').hover()
  await expect(page.locator('.hover-card')).toContainText(first.name)
  await expect(page.locator('.hover-card')).toContainText('500 кг')
  await page.mouse.move(0, 0)

  await rows.first().click()
  const overlay = page.locator('dialog.robot-overlay[open]')
  await expect(overlay).toBeVisible()
  await expect(page).toHaveURL(new RegExp(`robot=${first.id}`))
  const title = overlay.locator('.robot-title-input')
  await expect(title).toHaveValue(first.name)

  await test.step('Up and Down move through the list the page shows', async () => {
    await page.keyboard.press('ArrowDown')
    await expect(title).toHaveValue(second.name)
    await expect(page).toHaveURL(new RegExp(`robot=${second.id}`))
    await page.keyboard.press('ArrowUp')
    await expect(title).toHaveValue(first.name)
  })

  await test.step('web searches for the name and company open in a new tab', async () => {
    const query = encodeURIComponent(`${first.name} ООО «Проба»`)
    const google = overlay.getByRole('link', { name: 'Найти в Google' })
    const yandex = overlay.getByRole('link', { name: 'Найти в Яндексе' })
    await expect(google).toHaveAttribute('href', `https://www.google.com/search?q=${query}`)
    await expect(yandex).toHaveAttribute('href', `https://yandex.ru/search/?text=${query}`)
    await expect(google).toHaveAttribute('target', '_blank')
  })

  const side = overlay.getByRole('region', { name: 'Данные робота' })
  const payload =side.getByRole('textbox', { name: /^Грузоподъёмность/ })
  await test.step('a field saves itself, and undo takes the edit back', async () => {
    const sent = patched(page, first.id)
    await payload.fill('750')
    await payload.press('Enter')
    expect((await sent).status()).toBe(200)
    await expect.poll(async () => (await robot(admin, first.id)).specs.payload_kg).toBe(750)

    await overlay.getByRole('listbox', { name: 'Роботы' }).focus()
    const undone = patched(page, first.id)
    await page.keyboard.press('Control+z')
    expect((await undone).status()).toBe(200)
    await expect(payload).toHaveValue('500')
    await expect.poll(async () => (await robot(admin, first.id)).specs.payload_kg).toBe(500)
  })

  await test.step('a copy opens in place and goes to the archive and back', async () => {
    await overlay.getByRole('button', { name: 'Дублировать' }).click()
    await expect(title).toHaveValue(`${first.name} (копия)`)
    await overlay.getByRole('button', { name: 'В архив' }).click()
    const confirm = page.getByRole('dialog', { name: 'Убрать робота в архив?' })
    await confirm.getByRole('button', { name: 'В архив' }).click()
    await expect(overlay.getByRole('button', { name: 'Вернуть из архива' })).toBeVisible()
    await overlay.getByRole('button', { name: 'Вернуть из архива' }).click()
    await expect(overlay.getByRole('button', { name: 'В архив' })).toBeVisible()
  })

  await test.step('a dropped photo replaces the placeholder', async () => {
    await overlay.getByRole('option', { name: first.name, exact: true }).click()
    await expect(title).toHaveValue(first.name)
    await expect(overlay.getByText('Нет фото')).toBeVisible()
    await dropPhoto(overlay.locator('.robot-picture'))
    await expect(overlay.getByRole('img', { name: `Фото: ${first.name}` })).toBeVisible()
    await expect.poll(async () => (await robot(admin, first.id)).image_sha).not.toBeNull()
  })

  await page.keyboard.press('Escape')
  await expect(overlay).toHaveCount(0)
  await expect(page).not.toHaveURL(/robot=/)
  await admin.api.dispose()
})

test('a catalog upload shows a conflict, takes the side the admin picks and reports errors', async ({ page }) => {
  const admin = await adminSession()
  const target = await createRobot(admin, { name: `${prefix} В`, vendor: 'ООО «Проба»', family: 'mobile', payload_kg: 500 })
  await openCatalog(page, prefix)

  const exported = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Скачать каталог' }).click()
  const wb = await readWorkbook(await exported)
  const sheet = wb.SheetNames[0]
  const grid = XLSX.utils.sheet_to_json<unknown[]>(wb.Sheets[sheet], { header: 1, defval: null })
  const head = grid[0] as string[]
  const col = (label: string) => {
    const i = head.indexOf(label)
    expect(i, label).toBeGreaterThanOrEqual(0)
    return i
  }
  const line = grid.findIndex((r) => r[0] === target.id)
  expect(line).toBeGreaterThan(0)
  grid[line][col('Грузоподъёмность, кг')] = 900
  const added = Array<unknown>(head.length).fill(null)
  added[col('Название')] = `${prefix} из файла`
  added[col('Тип')] = 'Мобильные роботы'
  const broken = Array<unknown>(head.length).fill(null)
  broken[col('Название')] = `${prefix} с ошибкой`
  broken[col('Цена, ₽')] = 'дорого'
  grid.push(added, broken)
  const out = XLSX.utils.book_new()
  XLSX.utils.book_append_sheet(out, XLSX.utils.aoa_to_sheet(grid), sheet)
  const buffer = XLSX.write(out, { type: 'buffer', bookType: 'xlsx' }) as Buffer

  const edit = await admin.api.patch(`/api/admin/solutions/${target.id}`, { data: { payload_kg: 700 } })
  expect(edit.status(), await edit.text()).toBe(200)

  await page.getByRole('button', { name: 'Загрузить таблицу' }).click()
  const upload = page.getByRole('dialog', { name: 'Загрузка таблицы' })
  await upload.getByLabel('Файл с каталогом').setInputFiles({ name: 'katalog.xlsx', mimeType: xlsxType, buffer })
  await expect(upload.getByRole('heading', { name: 'Проверка' })).toBeVisible()
  const counts = upload.locator('.import-counts')
  await expect(counts).toContainText('новые: 1')
  await expect(counts).toContainText('конфликты: 1')
  await expect(counts).toContainText('с ошибками: 1')
  await expect(counts).not.toContainText('изменены')

  const conflict = upload.getByRole('row').filter({ hasText: target.name })
  await expect(conflict).toContainText('Конфликт: Грузоподъёмность.')
  await conflict.getByRole('button', { name: 'Сравнить' }).click()
  await expect(upload.getByRole('radio', { name: /^Грузоподъёмность: из каталога/ })).toBeChecked()
  const fromFile = page.getByRole('radio', { name: /^Грузоподъёмность: из файла/ })
  await upload.locator('.import-pick label', { has: fromFile }).click()
  await expect(fromFile).toBeChecked()

  const report = page.waitForEvent('download')
  await upload.getByRole('button', { name: 'Скачать отчёт об ошибках' }).click()
  const errors = sheetRows(await readWorkbook(await report), 'Ошибки')
  const reported = errors.find((r) => r.includes(`${prefix} с ошибкой`))
  expect(reported?.at(-1)).toContain('дорого')

  await upload.getByRole('button', { name: 'Применить' }).click()
  await expect(upload.getByRole('heading', { name: 'Готово' })).toBeVisible()
  const summary = upload.locator('.import-summary')
  await expect(summary.locator('div').filter({ hasText: 'Добавлены' })).toContainText(`«${prefix} из файла»`)
  await expect(summary.locator('div').filter({ hasText: 'Изменены' })).toContainText(`«${target.name}»`)
  expect((await robot(admin, target.id)).specs.payload_kg).toBe(900)

  await upload.getByRole('button', { name: 'Готово', exact: true }).click()
  await expect(upload).toHaveCount(0)
  await expect(page.locator('.catalog-table tbody tr').filter({ hasText: `${prefix} из файла` })).toHaveCount(1)
  await admin.api.dispose()
})

test('a one-robot upload fills the template and opens the new robot', async ({ page }) => {
  await openCatalog(page, prefix)
  const name = `${prefix} из шаблона`

  const template = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Шаблоны' }).click()
  await page.getByRole('button', { name: 'Одно решение, XLSX' }).click()
  const wb = await readWorkbook(await template)
  const sheet = wb.SheetNames[0]
  const grid = XLSX.utils.sheet_to_json<unknown[]>(wb.Sheets[sheet], { header: 1, defval: null })
  const set = (label: string, v: unknown) => {
    const row = grid.find((r) => r[0] === label)
    expect(row, label).toBeTruthy()
    row![1] = v
  }
  set('Название', name)
  set('Тип', 'Роботы-манипуляторы')
  set('Цена', 2_750_000)
  set('Грузоподъёмность', 12)
  const out = XLSX.utils.book_new()
  XLSX.utils.book_append_sheet(out, XLSX.utils.aoa_to_sheet(grid), sheet)
  const buffer = XLSX.write(out, { type: 'buffer', bookType: 'xlsx' }) as Buffer

  await page.getByRole('button', { name: 'Загрузить таблицу' }).click()
  const upload = page.getByRole('dialog', { name: 'Загрузка таблицы' })
  await upload.getByLabel('Один робот').check()
  await upload.getByLabel('Файл с каталогом').setInputFiles({ name: 'robot.xlsx', mimeType: xlsxType, buffer })
  await expect(upload.getByRole('heading', { name })).toBeVisible()
  await expect(upload.getByRole('row', { name: /^Цена/ })).toContainText('2 750 000 ₽')

  await upload.getByRole('button', { name: 'Применить' }).click()
  await expect(upload.getByRole('heading', { name: 'Готово' })).toBeVisible()
  await upload.getByRole('button', { name: 'Открыть решение' }).click()
  const overlay = page.locator('dialog.robot-overlay[open]')
  await expect(overlay.locator('.robot-title-input')).toHaveValue(name)
  await expect(overlay.getByRole('region', { name: 'Данные робота' }).getByRole('textbox', { name: /^Грузоподъёмность/ })).toHaveValue('12')
})
