import { expect, test } from '@playwright/test'

import { accountMenu, apiProject, apiRegister, PASSWORD } from './helpers.ts'

test('a login survives a reload, lives only in an HttpOnly cookie and ends on logout', async ({ page }) => {
  const owner = await apiRegister('session')
  const project = await apiProject(owner, 'Сессия E2E')

  await page.goto('/login')
  await page.getByLabel('Email').fill(owner.user.email)
  await page.getByLabel('Пароль').fill(PASSWORD)
  await page.getByRole('button', { name: 'Войти' }).click()
  await expect(page.getByRole('link', { name: 'Сессия E2E' })).toBeVisible()

  await page.reload()
  await expect(page.getByRole('link', { name: 'Сессия E2E' })).toBeVisible()
  await expect(await accountMenu(page)).toContainText(owner.user.email)
  await page.keyboard.press('Escape')

  const cookie = (await page.context().cookies()).find((c) => c.name === 'session' || c.name === '__Host-session')
  expect(cookie?.httpOnly).toBe(true)
  expect(cookie?.sameSite).toBe('Lax')
  const storage = await page.evaluate(() => JSON.stringify({ ...window.localStorage }))
  expect(storage).not.toContain(cookie?.value ?? 'no-cookie')
  expect(await page.evaluate(() => document.cookie)).not.toContain(cookie?.value ?? 'no-cookie')

  // A write without the CSRF token is refused, even from the page itself.
  const forged = await page.evaluate(async (id) => {
    const res = await fetch(`/api/projects/${id}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: '{}' })
    return { status: res.status, code: ((await res.json()) as { code?: string }).code }
  }, project.id)
  expect(forged).toEqual({ status: 403, code: 'csrf' })

  await (await accountMenu(page)).getByRole('button', { name: 'Выйти' }).click()
  await expect(page.getByRole('heading', { name: 'Демо' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Демо' })).toBeVisible()
  const menu = await accountMenu(page)
  await expect(menu.getByRole('link', { name: 'Войти' })).toBeVisible()
  await expect(menu).not.toContainText(owner.user.email)

  // Logging out in the browser does not end the owner's other session.
  const still = await owner.api.get(`/api/projects/${project.id}`)
  expect(still.status()).toBe(200)
})
