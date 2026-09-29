import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

import { expect, test } from '@playwright/test'

import { apiProject, apiRegister, signIn, synced } from './helpers.ts'

const testdata = join(dirname(fileURLToPath(import.meta.url)), '..', 'src', 'params', 'testdata')

test('parameter import reports bad cells and saves after the fix', async ({ page }) => {
  const owner = await apiRegister('import')
  const project = await apiProject(owner, 'Импорт параметров E2E')
  await signIn(page, owner)
  await page.goto(`/p/${project.id}/object/site`)
  await expect(page.getByRole('heading', { name: 'Площадка', level: 1 })).toBeVisible()

  const upload = page.locator('input[type="file"]')
  await upload.setInputFiles(join(testdata, 'warehouse_partial.csv'))
  const report = page.locator('.import-report')
  await expect(report).toContainText('Ячейки с ошибкой не подставлены')
  await expect(report.locator('li.error')).not.toHaveCount(0)
  const working = page.locator('input#aisle_working_m')
  await expect(working).toHaveValue('2,8')
  await expect(page.locator('.field-row', { has: working }).locator('p.error')).toBeVisible()

  const fixed = readFileSync(join(testdata, 'warehouse_partial.csv'), 'utf8')
    .replace('между стеллажами;м;0.5', 'между стеллажами;м;2.6')
    .replace(/^Неизвестный показатель.*$/m, '')
  await upload.setInputFiles({ name: 'warehouse_fixed.csv', mimeType: 'text/csv', buffer: Buffer.from(fixed, 'utf8') })
  await expect(report).not.toContainText('Ячейки с ошибкой не подставлены')
  await expect(working).toHaveValue('2,6')

  // A file is one action, so what it filled in is saved as one transaction, without a button.
  await synced(page)
  const saved = await owner.api.get(`/api/projects/${project.id}`)
  expect(((await saved.json()) as { params: Record<string, unknown> }).params.aisle_working_m).toBe(2.6)

  const area = page.locator('input#area_total_m2')
  await area.fill('5')
  await area.blur()
  const areaError = page.locator('.field-row', { has: area }).locator('p.error')
  await expect(areaError).toHaveText('Укажите число от 10\u00a0000 до 100\u00a0000.')
  const kept = await owner.api.get(`/api/projects/${project.id}`)
  expect(((await kept.json()) as { params: Record<string, unknown> }).params.area_total_m2).not.toBe(5)

  await area.fill('20000')
  await area.blur()
  await expect(areaError).toHaveCount(0)
  await page.reload()
  await expect(page.locator('input#aisle_working_m')).toHaveValue('2,6')
  await expect(page.locator('input#area_total_m2')).toHaveValue('20\u00a0000')
})
