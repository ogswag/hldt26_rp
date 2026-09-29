import { expect, test } from '@playwright/test'

import { apiRegister, signIn, synced } from './helpers.js'

// A guest works in the demos; everything stays in the browser. After signing in, a changed demo can become a
// project, with the whole content carried over, not just the parameters that were on screen.
test('a changed demo becomes a project after signing in', async ({ page }) => {
  const session = await apiRegister('import')

  await page.goto('/demo/warehouse/object/site')
  const area = page.locator('input#area_total_m2')
  await area.fill('37500')
  await area.blur()
  await synced(page)

  await signIn(page, session)
  await page.goto('/')
  const offer = page.locator('.note-muted', { hasText: 'Демо Склад' })
  await expect(offer).toContainText('Вы меняли «Демо Склад» в этом браузере до входа.')
  await offer.getByRole('button', { name: 'Сохранить как проект' }).click()
  await page.waitForURL(/\/p\/[0-9a-f-]{36}\/object\/site$/)
  const projectId = new URL(page.url()).pathname.split('/')[2] ?? ''

  const res = await session.api.get(`/api/projects/${projectId}`)
  expect(res.ok(), await res.text()).toBeTruthy()
  const project = (await res.json()) as {
    name: string
    object_type: string
    params: Record<string, unknown>
    processes: { code: string }[]
  }
  expect(project.name).toBe('Демо Склад')
  expect(project.object_type).toBe('warehouse')
  expect(project.params.area_total_m2).toBe(37500)
  expect(project.processes.map((p) => p.code)).toEqual(['inbound', 'putaway', 'piece_pick', 'outbound'])

  const journal = await session.api.get(`/api/projects/${projectId}/operations?after=0`)
  const ops = (await journal.json()) as { items: { client_id: string }[] }
  expect(ops.items[0].client_id).toBe('import')

  // Saved once, the offer does not come back for the same demo.
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Проекты', exact: true })).toBeVisible()
  await expect(page.locator('.note-muted', { hasText: 'Демо Склад' })).toHaveCount(0)
})
