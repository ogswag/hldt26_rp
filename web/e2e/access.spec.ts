import { expect, test } from '@playwright/test'

import { apiProject, apiRegister, signIn } from './helpers.ts'

test('another user cannot open or export the owner project', async ({ page }) => {
  const owner = await apiRegister('owner')
  const intruder = await apiRegister('intruder')
  const project = await apiProject(owner, 'Закрытый склад E2E')
  const calc = await owner.api.post(`/api/projects/${project.id}/calculations`, { data: {} })
  expect(calc.ok()).toBeTruthy()
  const runId = ((await calc.json()) as { run_id: string }).run_id

  const own = await owner.api.get(`/api/projects/${project.id}/runs`)
  expect(own.status()).toBe(200)

  for (const [method, path] of [
    ['GET', `/api/projects/${project.id}`],
    ['GET', `/api/projects/${project.id}/runs`],
    ['GET', `/api/calculations/${runId}`],
    ['POST', `/api/calculations/${runId}/export?format=pdf`],
    ['POST', `/api/projects/${project.id}/export?format=xlsx`],
    ['POST', `/api/projects/${project.id}/transactions`],
    ['GET', `/api/projects/${project.id}/snapshot`],
    ['DELETE', `/api/projects/${project.id}`],
  ] as const) {
    const res = await intruder.api.fetch(path, { method, data: method === 'GET' ? undefined : {} })
    expect(res.status(), `${method} ${path}`).toBe(404)
  }

  await signIn(page, intruder)
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'Новый проект' })).toBeVisible()
  await expect(page.getByText('Загрузка списка...')).toHaveCount(0)
  await expect(page.locator('.project-card')).toHaveCount(0)
  for (const path of ['object', 'calc', 'calc/history', `calc/history/${runId}`, 'calc/sim', 'sim']) {
    await page.goto(`/p/${project.id}/${path}`)
    await expect(page.getByRole('heading', { name: 'Проект не найден' }), path).toBeVisible()
    await expect(page.getByText('Закрытый склад E2E')).toHaveCount(0)
  }

  const still = await owner.api.get(`/api/projects/${project.id}`)
  expect(still.status()).toBe(200)
  expect(((await still.json()) as { name: string }).name).toBe('Закрытый склад E2E')
})

test('a viewer can inspect a project but cannot edit it', async ({ page }) => {
  const owner = await apiRegister('viewer-owner')
  const viewer = await apiRegister('viewer')
  const project = await apiProject(owner, 'Склад только для чтения')
  const share = await owner.api.post(`/api/projects/${project.id}/members`, {
    data: { email: viewer.user.email, role: 'viewer' },
  })
  expect(share.status(), await share.text()).toBe(201)

  await signIn(page, viewer)
  await page.goto(`/p/${project.id}/object`)
  await expect(page.getByRole('heading', { name: 'Площадка', level: 1 })).toBeVisible()
  await expect(page.locator('fieldset.page-scope')).toHaveAttribute('disabled', '')
  await expect(page.locator('input#area_total_m2')).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Демо-значения' })).toHaveCount(0)

  const before = await viewer.api.get(`/api/projects/${project.id}/snapshot`)
  expect(before.status()).toBe(200)
  const denied = await viewer.api.post(`/api/projects/${project.id}/transactions`, {
    data: { client_id: 'viewer', schema_version: 1, txs: [] },
  })
  expect(denied.status()).toBe(403)
})
