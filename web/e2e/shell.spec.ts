import { expect, test } from '@playwright/test'

import { accountMenu, adminSession, adminSignIn, apiProject, apiRegister, pick, signIn, subtab, tab } from './helpers.js'

test('the calculation opens once the key object tabs are reviewed', async ({ page }) => {
  const session = await apiRegister('shell')
  const project = await apiProject(session, 'Склад Подольск')
  await signIn(page, session)
  await page.setViewportSize({ width: 1280, height: 800 })

  await page.goto(`/p/${project.id}/object`)
  await expect(page).toHaveURL(new RegExp(`/p/${project.id}/object/site$`))
  await expect(page.locator('.project-button')).toContainText('Склад Подольск')
  const tabs = page.getByRole('navigation', { name: 'Разделы', exact: true })
  await expect(tabs.getByRole('link', { name: 'Объект' })).toHaveAttribute('aria-current', 'page')
  await expect(page.getByRole('heading', { name: 'Площадка', level: 1 })).toBeVisible()
  await expect(page.getByText('Сохранено', { exact: true })).toHaveCount(0)

  const calc = tabs.getByRole('button', { name: 'Расчёт' })
  await expect(calc).toHaveAttribute('aria-disabled', 'true')
  // NOTE: Playwright will not click an aria-disabled element; a person can, and gets the reason.
  await calc.click({ force: true })
  const why = page.getByRole('dialog', { name: 'Почему закрыта вкладка «Расчёт»' })
  await expect(why).toContainText('Проверьте параметры объекта: Режим и объёмы, Персонал.')
  await why.getByRole('link', { name: 'Открыть «Режим и объёмы»' }).click()
  await expect(page).toHaveURL(/\/object\/volumes$/)
  await calc.click({ force: true })
  await expect(why).toContainText('Проверьте параметры объекта: Персонал.')
  await page.keyboard.press('Escape')

  await subtab(page, 'Персонал')
  await tab(page, 'Расчёт')
  await expect(page).toHaveURL(/\/calc\/summary$/)
  await expect(page.getByRole('columnheader', { name: 'Покупка' })).toBeVisible()
  await expect(page.getByRole('navigation', { name: 'Подразделы' }).getByRole('link', { name: 'История' })).toBeVisible()

  await subtab(page, 'Формулы и источники')
  await expect(page.getByRole('heading', { name: 'Формулы', exact: true })).toBeVisible()
})

test('an admin has the Админ tab on the start page and before Объект, and no Админ in the account menu', async ({ page }) => {
  const admin = await adminSession()
  const project = await apiProject(admin, `E2E админ ${Date.now()}`)
  await admin.api.dispose()
  await adminSignIn(page)
  await page.setViewportSize({ width: 1280, height: 800 })

  await page.goto('/')
  const tabs = page.getByRole('navigation', { name: 'Разделы', exact: true })
  await expect(tabs.getByRole('listitem')).toHaveText(['Проекты', 'Админ'])
  await tab(page, 'Админ')
  await expect(page).toHaveURL(/\/admin\/catalog$/)
  await expect(page.getByRole('navigation', { name: 'Подразделы' }).getByRole('link')).toHaveText(['Каталог', 'Приглашения', 'Журнал'])
  const menu = await accountMenu(page)
  await expect(menu).not.toContainText('Админ')
  await page.keyboard.press('Escape')

  await page.goto(`/p/${project.id}/object`)
  await expect(tabs.getByRole('listitem')).toHaveText(['Админ', 'Объект', 'Роботы', 'Расчёт'])
  await tab(page, 'Админ')
  await expect(page).toHaveURL(new RegExp(`/p/${project.id}/admin/catalog$`))
  await subtab(page, 'Журнал')
  await expect(page).toHaveURL(new RegExp(`/p/${project.id}/admin/audit$`))
})

test('a user has no Админ tab, and the admin pages say they are for admins', async ({ page }) => {
  const session = await apiRegister('noadmin')
  const project = await apiProject(session, 'Склад без админки')
  await signIn(page, session)
  await page.setViewportSize({ width: 1280, height: 800 })

  await page.goto('/')
  const tabs = page.getByRole('navigation', { name: 'Разделы', exact: true })
  await expect(tabs.getByRole('listitem')).toHaveText(['Проекты'])
  const menu = await accountMenu(page)
  await expect(menu).not.toContainText('Админ')
  await page.keyboard.press('Escape')

  await page.goto(`/p/${project.id}/object`)
  await expect(tabs.getByRole('listitem')).toHaveText(['Объект', 'Роботы', 'Расчёт'])
  await page.goto('/admin/catalog')
  await expect(page.getByRole('heading', { name: 'Нет доступа' })).toBeVisible()
})

test('the account menu holds the theme, and the theme survives a reload', async ({ page }) => {
  const session = await apiRegister('theme')
  await signIn(page, session)
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Проекты', exact: true })).toBeVisible()

  let menu = await accountMenu(page)
  await expect(menu).toContainText(session.user.email)
  await expect(menu.getByRole('link', { name: 'Корзина' })).toBeVisible()
  await pick(menu.getByRole('combobox', { name: 'Тема' }), { name: 'Тёмная' })
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.keyboard.press('Escape')
  await expect(menu).toBeHidden()

  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  menu = await accountMenu(page)
  await pick(menu.getByRole('combobox', { name: 'Тема' }), { name: 'Авто' })
  await expect(page.locator('html')).not.toHaveAttribute('data-theme', /.+/)
})

test('a guest opens the demos, where every tab is open', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Демо' })).toBeVisible()
  await expect(page.getByText('Вы не вошли')).toHaveCount(0)
  await page.getByRole('link', { name: /^Демо Склад/ }).click()
  await expect(page).toHaveURL(/\/demo\/warehouse\/object\/site$/)

  const tabs = page.getByRole('navigation', { name: 'Разделы', exact: true })
  for (const name of ['Объект', 'Роботы', 'Расчёт']) {
    await expect(tabs.getByRole('link', { name, exact: true })).toBeVisible()
  }
  const subtabs = page.getByRole('navigation', { name: 'Подразделы' })
  await expect(subtabs.getByRole('link', { name: 'Процессы' })).toBeVisible()
  await expect(subtabs.getByRole('link', { name: 'Карта' })).toBeVisible()
  await tab(page, 'Расчёт')
  await expect(subtabs.getByRole('link', { name: 'Симуляция' })).toBeVisible()

  await page.goto('/demo/airport/object')
  await expect(subtabs.getByRole('link', { name: 'Процессы' })).toBeVisible()
  await expect(subtabs.getByRole('link', { name: 'Карта' })).toHaveCount(0)
  await tab(page, 'Расчёт')
  await subtab(page, 'Симуляция')
  await expect(page.getByText('Симуляция пока доступна только для склада.')).toBeVisible()
})

test('on a phone one menu holds the tabs and the subtabs, and it closes after a pick', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 })
  await page.goto('/demo/warehouse/object')
  await expect(page.getByRole('navigation', { name: 'Разделы', exact: true })).toBeHidden()
  await expect(page.locator('.project-button')).toContainText('Демо Склад')
  const menuButton = page.locator('.nav-menu .subtab-menu-button')
  await expect(menuButton).toHaveText('Объект: Площадка')
  await menuButton.click()
  const menu = page.getByRole('dialog', { name: 'Разделы', exact: true })
  await expect(menu.getByRole('link', { name: 'Площадка' })).toHaveAttribute('aria-current', 'page')
  await menu.getByRole('link', { name: 'Персонал' }).click()
  await expect(page).toHaveURL(/\/demo\/warehouse\/object\/staff$/)
  await expect(menu).toBeHidden()
  await expect(menuButton).toHaveText('Объект: Персонал')

  await menuButton.click()
  await menu.getByRole('link', { name: 'Роботы' }).click()
  await expect(page).toHaveURL(/\/demo\/warehouse\/robots$/)
  await expect(menuButton).toHaveText('Роботы')
})

test('subtabs that do not fit become one menu, and the row comes back when they fit', async ({ page }) => {
  // NOTE: a larger browser font stands in for zoom; the tab row stays, the subtabs no longer fit next to it.
  await page.setViewportSize({ width: 720, height: 812 })
  await page.goto('/demo/warehouse/object/processes')
  await page.addStyleTag({ content: ':root { font-size: 150%; }' })
  const row = page.locator('.ribbon-sub-wide')
  const menuButton = row.locator('.subtab-menu-button')
  await expect(menuButton).toHaveText('Процессы')
  await menuButton.click()
  const menu = page.getByRole('dialog', { name: 'Подразделы', exact: true })
  await expect(menu.getByRole('link', { name: 'Процессы' })).toHaveAttribute('aria-current', 'page')
  await menu.getByRole('link', { name: 'Персонал' }).click()
  await expect(page).toHaveURL(/\/demo\/warehouse\/object\/staff$/)
  await expect(menu).toBeHidden()
  await expect(menuButton).toHaveText('Персонал')

  await page.setViewportSize({ width: 1600, height: 800 })
  await expect(page.getByRole('navigation', { name: 'Подразделы' }).getByRole('link', { name: 'Персонал' })).toBeVisible()
  await expect(menuButton).toHaveCount(0)
})
