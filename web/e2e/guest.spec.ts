import { expect, test } from '@playwright/test'

import { downloadBytes, readWorkbook, sheetRows, subtab, synced, tab } from './helpers.ts'

test('a guest calculates and exports a demo without saving commercial inputs on the server', async ({ page, request }) => {
  const writes: string[] = []
  page.on('request', (req) => {
    const path = new URL(req.url()).pathname
    if (req.method() !== 'GET' && path.startsWith('/api/') && !path.startsWith('/api/guest/')) {
      writes.push(`${req.method()} ${path}`)
    }
  })

  await page.goto('/demo/warehouse/object')
  await subtab(page, 'Персонал')
  const wage = page.locator('input#wage_picker_month_rub')
  await wage.fill('123457')
  await wage.blur()
  await synced(page)

  await tab(page, 'Расчёт')
  await expect(page.getByRole('columnheader', { name: /, покупка$/ }).first()).toBeVisible()
  const stored = await page.evaluate(() => JSON.stringify(window.localStorage))
  expect(stored).toContain('123457')

  await subtab(page, 'Экспорт')
  await expect(page).toHaveURL(/\/demo\/warehouse\/calc\/export$/)
  // NOTE: a string, not a regex: Reflow joins short words with a no-break space, and only string matching folds it.
  await expect(page.getByText('Демо-расчёт не хранится на сервере')).toBeVisible()
  await expect(page.getByText('Уровень подтверждённости: Предварительный', { exact: true })).toBeVisible()
  const [pdf] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'Скачать PDF' }).click()])
  expect(pdf.suggestedFilename()).toBe('ocenka-warehouse.pdf')
  expect((await downloadBytes(pdf)).subarray(0, 4).toString()).toBe('%PDF')
  const [xlsx] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'Скачать Excel' }).click()])
  expect(xlsx.suggestedFilename()).toBe('ocenka-warehouse.xlsx')
  const wb = await readWorkbook(xlsx)
  expect(wb.SheetNames).toEqual(['Итог', 'Варианты', 'Допущения и риски'])
  expect(sheetRows(wb, 'Итог').flat()).toContain('Не сохранён')

  // A demo keeps no runs, so Расчёт has no История.
  await expect(page.getByRole('navigation', { name: 'Подразделы' }).getByRole('link', { name: 'История' })).toHaveCount(0)

  expect(writes).toEqual([])

  const denied = await request.post('/api/projects', { data: { name: 'гость', object_type: 'warehouse' } })
  expect(denied.status()).toBe(403)
  const list = await request.get('/api/projects')
  expect(list.status()).toBe(403)
})
