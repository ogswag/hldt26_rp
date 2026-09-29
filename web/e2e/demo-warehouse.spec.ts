import { expect, test } from '@playwright/test'

import { subtab, tab } from './helpers.ts'

test('Демо Склад comes with a map, three variants and a simulation that replays in the browser', async ({ page }) => {
  const writes: string[] = []
  const crashes: string[] = []
  page.on('request', (req) => {
    const path = new URL(req.url()).pathname
    if (req.method() !== 'GET' && path.startsWith('/api/') && !path.startsWith('/api/guest/')) {
      writes.push(`${req.method()} ${path}`)
    }
  })
  page.on('pageerror', (err) => crashes.push(err.message))

  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/demo/warehouse/object')

  await subtab(page, 'Карта')
  await expect(page).toHaveURL(/\/demo\/warehouse\/object\/map$/)
  await expect(page.locator('canvas').first()).toBeVisible()
  await expect(page.getByText('0,1 м в пикселе, план 84 x 52 м.')).toBeVisible()
  await expect(page.getByText(/Проверка карты: ошибок 0,/)).toBeVisible()
  await expect(page.getByRole('row', { name: /Приёмка/ })).toBeVisible()

  await tab(page, 'Роботы')
  await expect(page.getByRole('heading', { name: 'Варианты' })).toBeVisible()
  const names = page.getByLabel('Название')
  await expect(names).toHaveCount(3)
  await expect(names.nth(0)).toHaveValue('Смешанный флот')
  await expect(names.nth(1)).toHaveValue('AMR комплектация')
  await expect(names.nth(2)).toHaveValue('Паллетный штабелёр')
  await expect(page.getByRole('button', { name: /Выбрать/ }).first()).toBeVisible()

  await tab(page, 'Расчёт')
  await expect(page).toHaveURL(/\/calc\/summary$/)
  for (const name of ['Смешанный флот', 'AMR комплектация', 'Паллетный штабелёр']) {
    await expect(page.getByRole('columnheader', { name: new RegExp(name) }).first()).toBeVisible()
  }
  // NOTE: the demo's fleets agree with the geometric check, so it never opens on the divergence banner.
  await expect(page.getByText('Симуляция и экономика разошлись')).toHaveCount(0)

  await subtab(page, 'Симуляция')
  await page.getByRole('button', { name: 'Запустить симуляцию' }).click()
  await expect(page.getByText('готово', { exact: true })).toBeVisible({ timeout: 60_000 })
  await expect(page.getByRole('heading', { name: /Воспроизведение повтора/ })).toBeVisible()

  // NOTE: \s also matches the no-break space Reflow puts before a short word.
  const clock = page.getByText(/^\d\d:\d\d:\d\d\sиз\s08:00:00/)
  await expect(clock).toHaveText(/^00:00:00/)
  await page.getByRole('button', { name: 'Пуск' }).click()
  await expect(clock).not.toHaveText(/^00:00:00/, { timeout: 20_000 })
  await page.getByRole('button', { name: 'Пауза' }).click()

  // The run is kept in the browser next to the demo.
  await page.reload()
  await expect(page.getByText('готово', { exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: /^Результат: / })).toBeVisible()

  expect(crashes).toEqual([])
  expect(writes).toEqual([])
})
