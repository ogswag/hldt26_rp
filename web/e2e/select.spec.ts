import { expect, test } from '@playwright/test'

import { apiProject, apiRegister, pick, signIn, synced } from './helpers.ts'

test('dropdown list is drawn by the page and works with mouse and keyboard', async ({ page }) => {
  await page.goto('/demo/warehouse/robots')
  await page.getByRole('button', { name: 'Фильтры' }).click()
  const panel = page.getByRole('dialog', { name: 'Фильтры' })
  const field = panel.getByRole('combobox', { name: 'Объект' })
  await expect(field).toHaveAccessibleName('Объект')
  await expect(field.locator('.select-value')).toHaveText('Все')
  const bodyFont = await page.evaluate(() => getComputedStyle(document.body).fontFamily)
  await expect(field).toHaveCSS('font-family', bodyFont)

  await test.step('the open list uses the field font and keeps rows in proportion', async () => {
    await field.click()
    const list = page.getByRole('listbox')
    await expect(list.getByRole('option')).toHaveText(['Все', 'Склад', 'Аэропорт', 'Медучреждение'])
    await expect(list.getByRole('option', { name: 'Все' })).toHaveAttribute('aria-selected', 'true')
    const fieldSize = await field.evaluate((el) => getComputedStyle(el).fontSize)
    const first = list.getByRole('option').first()
    await expect(first).toHaveCSS('font-size', fieldSize)
    await expect(first).toHaveCSS('font-family', bodyFont)
    const ratio = await first.evaluate(
      (el) => el.getBoundingClientRect().height / parseFloat(getComputedStyle(el).fontSize),
    )
    expect(ratio).toBeLessThan(2.5)

    await list.getByRole('option', { name: 'Аэропорт' }).click()
    await expect(list).toBeHidden()
    await expect(panel).toBeVisible()
    await expect(field).toHaveAttribute('data-value', 'airport')
    await expect(field).toBeFocused()
    await expect(page).toHaveURL(/[?&]object=airport/)
  })

  await test.step('keyboard opens, moves, jumps by letter and closes the list, not the panel', async () => {
    await field.press('ArrowDown')
    await expect(field).toHaveAttribute('aria-expanded', 'true')
    await field.press('ArrowDown')
    await expect(page.getByRole('option', { name: 'Медучреждение' })).toHaveAttribute('aria-selected', 'true')
    await field.press('Escape')
    await expect(field).toHaveAttribute('aria-expanded', 'false')
    await expect(panel).toBeVisible()
    await expect(field).toHaveAttribute('data-value', 'airport')

    await field.dispatchEvent('keydown', { key: 'м' })
    await expect(page.getByRole('option', { name: 'Медучреждение' })).toHaveAttribute('aria-selected', 'true')
    await field.press('Enter')
    await expect(field).toHaveAttribute('aria-expanded', 'false')
    await expect(field).toHaveAttribute('data-value', 'hospital')

    await field.press('Home')
    await expect(page.getByRole('option', { name: 'Все' })).toHaveAttribute('aria-selected', 'true')
    await field.press('Tab')
    await expect(field).toHaveAttribute('aria-expanded', 'false')
    await expect(field).not.toBeFocused()
    await expect(field).toHaveAttribute('data-value', 'hospital')
  })

  await test.step('the label toggles the list and an outside click closes the list and the panel', async () => {
    const label = panel.locator('label').filter({ has: page.getByRole('combobox', { name: 'Объект' }) })
    await label.click({ position: { x: 4, y: 6 } })
    await expect(field).toHaveAttribute('aria-expanded', 'true')
    await label.click({ position: { x: 4, y: 6 } })
    await expect(field).toHaveAttribute('aria-expanded', 'false')

    await field.click()
    await page.locator('main h2').first().click()
    await expect(page.getByRole('listbox')).toBeHidden()
    await expect(panel).toBeHidden()
    await expect(page.getByRole('button', { name: 'Фильтры: 1' })).toBeVisible()
    await expect(page).toHaveURL(/[?&]object=hospital/)
  })
})

test('map editor picks kinds from dropdowns and points from checkbox lists', async ({ page }) => {
  const owner = await apiRegister('lists')
  const project = await apiProject(owner, 'Списки карты E2E')
  await signIn(page, owner)
  await page.goto(`/p/${project.id}/object/map`)
  const counts = page.locator('.map-layout')
  await expect(counts).toHaveAttribute('data-points', /\d+/)
  const before = (await counts.getAttribute('data-points')) ?? ''

  await test.step('point kind dropdown keeps map shortcuts away from the focused field', async () => {
    await page.getByText('Точки, рёбра и зоны').click()
    await page.locator('details.map-props table').first().getByRole('button').first().click()
    const kind = page.locator('.map-side').getByRole('combobox', { name: 'Тип', exact: true })
    const original = (await kind.getAttribute('data-value')) ?? ''
    const other = original === 'gate' ? 'task' : 'gate'
    await pick(kind, { value: other })
    await expect(kind).toHaveAttribute('data-value', other)
    await expect(kind).toBeFocused()
    await kind.press('Delete')
    await kind.press('z')
    await expect(kind).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByRole('button', { name: 'Зона', exact: true })).toHaveAttribute('aria-pressed', 'false')
    await kind.press('Escape')
    await expect(counts).toHaveAttribute('data-points', before)
    await pick(kind, { value: original })
    await expect(kind).toHaveAttribute('data-value', original)
  })

  await test.step('flow points are checkboxes in a dropdown and survive a reload', async () => {
    const pickupOf = async () => {
      await page.locator('tr[data-flow]').first().locator('button.check-select').first().click()
      return page.getByRole('dialog', { name: 'Забор' })
    }
    let pickup = await pickupOf()
    const count = () => pickup.getByText(/^Выбрано: \d+$/)
    const n = Number(((await count().textContent()) ?? '').replace(/\D/g, ''))
    const free = pickup.getByRole('checkbox', { checked: false }).first()
    const name = await free.evaluate((el) => el.closest('label')?.textContent ?? '')
    await free.check()
    await expect(count()).toHaveText(`Выбрано: ${n + 1}`)
    await page.keyboard.press('Escape')
    await expect(pickup).toBeHidden()
    await synced(page)
    await page.reload()
    pickup = await pickupOf()
    await expect(pickup.getByRole('checkbox', { name, exact: true })).toBeChecked()
    await expect(count()).toHaveText(`Выбрано: ${n + 1}`)
    await page.keyboard.press('Escape')
  })

  await test.step('a new narrow aisle lists edges as checkboxes', async () => {
    const resources = page.locator('.map-props').filter({ has: page.getByRole('heading', { name: 'Общие ресурсы' }) })
    await pick(resources.getByRole('combobox', { name: 'Тип ресурса' }), { name: 'Узкий участок' })
    await resources.getByRole('button', { name: 'Добавить ресурс' }).click()
    const added = resources.locator('tr[data-resource]').filter({ hasText: 'Узкий участок' }).last()
    await expect(added.getByRole('textbox').first()).toBeFocused()
    await added.locator('button.check-select').click()
    const edges = page.getByRole('dialog', { name: 'Рёбра' })
    await edges.getByRole('checkbox').first().check()
    await expect(edges.getByText('Выбрано: 1')).toBeVisible()
  })
})
