import { randomUUID } from 'node:crypto'

import { expect, test } from '@playwright/test'

import { apiProject, apiRegister, PASSWORD, signIn } from './helpers.ts'

// Error pages: they stay inside the shell, say what happened and offer one way on.
test('an unknown address shows the not found page in the shell', async ({ page }) => {
  await page.goto('/nope')
  await expect(page.getByRole('heading', { name: 'Страница не найдена' })).toBeVisible()
  await expect(page.getByText('Ошибка 404')).toBeVisible()
  await expect(page.getByRole('combobox', { name: 'Поиск по проекту' })).toBeVisible()
  await page.getByRole('main').getByRole('link', { name: 'К проектам' }).click()
  await expect(page).toHaveURL(/\/$/)
})

test('a project that does not exist says so', async ({ page }) => {
  const session = await apiRegister('errors-missing')
  await signIn(page, session)
  await page.goto(`/p/${randomUUID()}/object`)
  await expect(page.getByRole('heading', { name: 'Проект не найден' })).toBeVisible()
  await session.api.dispose()
})

test('a guest opening a saved project signs in and comes back to it', async ({ page }) => {
  const session = await apiRegister('errors-guest')
  const project = await apiProject(session, `E2E вход ${Date.now()}`)
  await page.goto(`/p/${project.id}/object`)
  await expect(page.getByRole('heading', { name: 'Нужно войти' })).toBeVisible()
  await page.getByRole('main').getByRole('link', { name: 'Войти' }).click()
  await expect(page).toHaveURL(/\/login\?next=/)
  await page.getByLabel('Email').fill(session.user.email)
  await page.getByLabel('Пароль').fill(PASSWORD)
  await page.getByRole('main').getByRole('button', { name: 'Войти' }).click()
  await expect(page).toHaveURL(new RegExp(`/p/${project.id}/object`))
  await session.api.dispose()
})

test('a server fault names the request for a report', async ({ page }) => {
  const session = await apiRegister('errors-down')
  const project = await apiProject(session, `E2E сбой ${Date.now()}`)
  await signIn(page, session)
  await page.route(`**/api/projects/${project.id}`, (route) =>
    route.fulfill({ status: 502, contentType: 'text/html', body: '<html>Bad Gateway</html>', headers: { 'X-Request-Id': 'req-e2e-502' } }),
  )
  await page.goto(`/p/${project.id}/object`)
  await expect(page.getByRole('heading', { name: 'Сервер не отвечает' })).toBeVisible()
  await expect(page.getByText('Ошибка 502')).toBeVisible()
  await expect(page.getByText('req-e2e-502')).toBeVisible()
  await session.api.dispose()
})
