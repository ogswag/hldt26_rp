import { expect, test, type Locator } from '@playwright/test'

import { apiRegister, saved, schemaDefaults, signIn, type ProjectJSON } from './helpers.ts'

type Calculated = ProjectJSON & {
  results: { variants?: { name: string; fleet: { name: string; quantity: number }[] }[] } | null
}

// Роботы suggests a robot as soon as the project is calculated, which opens Симуляция in Расчёт; a pick puts the robot into
// a variant that Итог then counts, and undo takes it back.
test('the calculation suggests a robot and a pick goes into a variant', async ({ page }) => {
  const session = await apiRegister('robots')
  const params = await schemaDefaults(session.api)
  const created = await session.api.post('/api/projects', {
    data: { name: `E2E подбор ${Date.now()}`, object_type: 'warehouse', params },
  })
  expect(created.status(), await created.text()).toBe(201)
  const project = (await created.json()) as ProjectJSON
  const calc = await session.api.post(`/api/projects/${project.id}/calculations`, { data: { seed: 1 } })
  expect(calc.status(), await calc.text()).toBe(200)
  const current = async () => (await (await session.api.get(`/api/projects/${project.id}`)).json()) as Calculated
  await signIn(page, session)
  await page.goto(`/p/${project.id}/robots`)

  const suggestion = page.locator('.robot-suggestion')
  const name = ((await suggestion.locator('h3').textContent()) ?? '').trim()
  expect(name).not.toBe('')
  await expect(suggestion).toContainText(/Окупается|окупаемость/)
  const tabs = page.getByRole('navigation', { name: 'Разделы', exact: true })
  await expect(tabs.getByRole('link', { name: 'Расчёт', exact: true })).toBeVisible()
  await expect(page.locator('.ranked-table tbody tr').first()).toContainText(name)

  const slot = page.locator('.slot').nth(1)
  await test.step('a pick lands in the chosen variant and the calculation counts it', async () => {
    await suggestion.getByRole('button', { name: 'Выбрать' }).click()
    await saved(page, project.id, () => page.getByRole('dialog', { name: `Выбрать ${name}` }).getByRole('button', { name: 'Вариант 2' }).click())
    await expect(slot).toContainText(name)
    await expect
      .poll(async () => {
        const p = await current()
        return !p.stale && (p.results?.variants ?? []).some((v) => v.name === 'Вариант 2' && v.fleet[0]?.name === name)
      }, { timeout: 30_000 })
      .toBe(true)
  })

  await test.step('undo takes the robot back out', async () => {
    await page.keyboard.press('Control+z')
    await expect(slot).toContainText('Роботов нет')
    await expect.poll(async () => (await current()).variants[1].fleet.length).toBe(0)
  })
})

test('the robot overlay shows a user the robot without anything to edit', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/demo/warehouse/robots')
  const names = page.locator('.ranked-table tbody tr .robot-name')
  const first = ((await names.first().textContent()) ?? '').trim()
  const second = ((await names.nth(1).textContent()) ?? '').trim()
  await names.first().click()

  const overlay = page.locator('dialog.robot-overlay[open]')
  await expect(overlay.getByRole('heading', { level: 2 })).toHaveText(first)
  const side = overlay.getByRole('region', { name: 'Данные робота' })
  await expect(side.getByRole('textbox')).toHaveCount(0)
  await expect(side.getByRole('combobox')).toHaveCount(0)
  await expect(overlay.getByRole('button', { name: 'Дублировать' })).toHaveCount(0)
  await expect(overlay.getByRole('button', { name: 'В архив' })).toHaveCount(0)
  await expect(overlay.getByText('Загрузить фото')).toHaveCount(0)

  await page.keyboard.press('ArrowDown')
  await expect(overlay.getByRole('heading', { level: 2 })).toHaveText(second)
  await page.keyboard.press('Escape')
  await expect(overlay).toHaveCount(0)
  await expect(page).not.toHaveURL(/robot=/)
})

test('a row of the ranking shows the checks behind its verdict', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/demo/warehouse/robots')
  const first = page.locator('.ranked-table tbody tr.is-openable').first()
  await first.getByRole('button', { name: 'Показать проверки' }).click()
  const explain = page.locator('.ranked-table .match-explain').first()
  await expect(explain.getByText('Итоговая оценка')).toBeVisible()
  await expect(explain.getByText('вес 0,4')).toBeVisible()
  const steps = explain.locator('.match-steps tbody tr')
  await expect(steps.first()).toBeVisible()
  await expect(explain.locator('.match-steps').getByText('Высота потолков', { exact: true })).toBeVisible()
  // Users read names of checks and fields, never codes.
  await expect(explain).not.toContainText(/aisle_|_mm|_kg|rule/)
  await first.getByRole('button', { name: 'Скрыть проверки' }).click()
  await expect(page.locator('.ranked-table .match-explain')).toHaveCount(0)
})

// pick ticks a comparison box; the address decides its state, so it is checked after the page has followed it.
async function pick(box: Locator) {
  await box.click()
  await expect(box).toBeChecked()
}

test('robots picked in the list are compared side by side', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/demo/warehouse/robots')
  const boxes = page.locator('.ranked-table tbody tr.is-openable .compare-check')
  const compare = page.getByRole('button', { name: /^Сравнить: \d+$/ })
  await expect(compare).toHaveCount(0)

  await pick(boxes.nth(0))
  await expect(compare).toBeDisabled()
  await pick(boxes.nth(1))
  await expect(compare).toBeEnabled()
  await expect(page).toHaveURL(/cmp=/)

  await compare.click()
  const dialog = page.getByRole('dialog', { name: 'Сравнение роботов' })
  await expect(dialog.locator('thead th')).toHaveCount(3)
  for (const group of ['Идентификация', 'Технические характеристики', 'Экономика', 'Применимость', 'Качество данных']) {
    await expect(dialog.getByRole('columnheader', { name: group })).toBeVisible()
  }
  await expect(dialog.getByRole('rowheader', { name: 'Окупаемость' })).toBeVisible()
  await expect(dialog).not.toContainText(/_mm|_kg|_rub|recommended|needs_review/)

  await dialog.getByRole('button', { name: /^Убрать из сравнения/ }).first().click()
  await expect(dialog).toHaveCount(0)
  await expect(page).toHaveURL(/cmp=[^,&]+(&|$)/)
  await page.getByRole('button', { name: 'Снять выбор' }).click()
  await expect(page).not.toHaveURL(/cmp=/)
})

test('the comparison holds six robots', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/demo/warehouse/robots')
  const boxes = page.locator('.ranked-table tbody tr.is-openable .compare-check')
  for (let i = 0; i < 6; i++) {
    await pick(boxes.nth(i))
  }
  await expect(boxes.nth(6)).toBeDisabled()
  await expect(boxes.nth(2)).toBeEnabled()
})

test('a value of the robot says where it comes from', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/demo/warehouse/robots')
  await page.locator('.ranked-table tbody tr .robot-name').first().click()
  const side = page.locator('dialog.robot-overlay[open]').getByRole('region', { name: 'Данные робота' })
  await side.getByRole('button', { name: 'Откуда значение: Цена' }).click()
  await expect(page.getByRole('dialog', { name: 'Откуда значение: Цена' })).toContainText(/Дата источника|Ссылки на источник нет|отдельного источника/)
  await expect(side.getByRole('textbox')).toHaveCount(0)
})
