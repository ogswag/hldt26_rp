import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

import { expect, test } from '@playwright/test'

import { apiRegister, schemaDefaults, signIn } from './helpers.ts'

const testdata = join(dirname(fileURLToPath(import.meta.url)), '..', 'src', 'map', 'testdata')

test('a PDF plan is rendered in the browser and sizes the map', async ({ page }) => {
  const owner = await apiRegister('plan')
  const params = await schemaDefaults(owner.api)
  const res = await owner.api.post('/api/projects', { data: { name: 'План PDF E2E', object_type: 'warehouse', params } })
  expect(res.status(), await res.text()).toBe(201)
  const project = (await res.json()) as { id: string }
  await signIn(page, owner)
  await page.goto(`/p/${project.id}/object/map`)
  await expect(page.getByRole('toolbar', { name: 'Инструменты карты' })).toBeVisible()

  // NOTE: plan.pdf has two A4 landscape pages (842 x 595 pt); the first is drawn at scale 4, 0,1 m per pixel.
  await page.locator('input.file-input').setInputFiles(join(testdata, 'plan.pdf'))
  await expect(page.getByRole('button', { name: 'Скрыть план' })).toBeVisible()
  await expect(page.locator('.map-props').filter({ has: page.getByRole('heading', { name: 'Масштаб' }) })).toContainText(
    'план 336,8 x 238 м',
  )
  await expect(page.getByText('В PDF 2 страницы. Используется только первая')).toBeVisible()
})
