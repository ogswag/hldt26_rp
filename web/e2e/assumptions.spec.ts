import { expect, test, type Locator } from '@playwright/test'

import { apiProject, apiRegister, signIn } from './helpers.ts'

// topOf reads where an element sits in the document, scrolling aside, so a test can tell that nothing above it
// moved.
function topOf(el: Locator): Promise<number> {
  return el.evaluate((e) => Math.round(e.getBoundingClientRect().top + window.scrollY))
}

// Допущения recalculates after each edit. The run is reported in the corner, so the page never shifts, and a new
// assumption set takes the user to its name.
test('assumption sets: a new set focuses its name and a recalculation leaves the page still', async ({ page }) => {
  const session = await apiRegister('assumptions')
  const project = await apiProject(session, `E2E допущения ${Date.now()}`)
  const res = await session.api.post(`/api/projects/${project.id}/calculations`, { data: { seed: 1 } })
  expect(res.status(), await res.text()).toBe(200)
  await signIn(page, session)
  await page.goto(`/p/${project.id}/calc/assumptions`)
  const heading = page.getByRole('heading', { name: 'Что если' })
  await expect(heading).toBeVisible()

  await test.step('«Добавить набор» puts the cursor in the new set\'s name', async () => {
    await page.getByRole('button', { name: 'Добавить набор' }).click()
    const name = page.getByRole('textbox', { name: 'Имя', exact: true }).last()
    await expect(name).toBeFocused()
    await expect(name).toHaveValue(/^Набор \d+$/)
    await expect(page.getByRole('combobox', { name: 'Активный набор' })).toBeVisible()
  })

  await test.step('a changed VAT rate recalculates without moving the page', async () => {
    const vat = page.getByRole('textbox', { name: /^Ставка НДС/ }).last()
    await vat.fill('10')
    const before = await topOf(heading)
    await vat.press('Enter')
    for (let i = 0; i < 20; i++) {
      expect(await topOf(heading)).toBe(before)
      await page.waitForTimeout(100)
    }
    await expect(page.locator('.stale-banner')).toHaveCount(0)
  })

  await test.step('the discount rate starts at 15% and is a field of the set', async () => {
    const rate = page.getByRole('textbox', { name: /^Ставка дисконтирования/ }).last()
    await expect(rate).toHaveValue('15')
  })
  await session.api.dispose()
})
