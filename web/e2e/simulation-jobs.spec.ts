import { expect, test } from '@playwright/test'

import { apiProject, apiRegister, heavyLoad, pick, signIn, type Session } from './helpers.ts'

test('a simulation job can be canceled and retried', async ({ page }) => {
  const owner = await apiRegister('jobs')
  const project = await apiProject(owner, 'Задания симуляции E2E')
  const variantId = await heavyLoad(owner, project)
  await signIn(page, owner)
  await page.goto(`/p/${project.id}/calc/sim`)
  await expect(page.getByRole('heading', { name: 'Событийная симуляция склада' })).toBeVisible()

  await pick(page.getByLabel('Вариант'), { value: variantId })
  await pick(page.getByLabel('Режим'), { value: 'stochastic' })
  await page.getByLabel('Повторов').fill('30')
  await page.getByLabel('Горизонт, ч').fill('24')
  await page.getByRole('button', { name: 'Запустить симуляцию' }).click()
  const progress = page.locator('.job-progress')
  await expect(progress).toBeVisible()
  await progress.getByRole('button', { name: 'Отменить' }).click()
  await expect(page.getByRole('cell', { name: 'отменено', exact: true })).toBeVisible({ timeout: 30_000 })
  await expect(progress).toBeHidden()

  const canceled = await simulationRuns(owner, project.id)
  expect(canceled).toHaveLength(1)
  expect(canceled[0].status).toBe('canceled')

  await page.getByRole('button', { name: 'Повторить' }).click()
  await expect(page.getByRole('cell', { name: 'готово', exact: true })).toBeVisible({ timeout: 150_000 })
  await expect(page.getByRole('button', { name: 'Отчёт PDF' })).toBeEnabled()

  const retried = await simulationRuns(owner, project.id)
  expect(retried).toEqual([{ ...canceled[0], status: 'succeeded' }])
})

type RunRow = { id: string; kind: string; status: string; job_id: string | null }

async function simulationRuns(owner: Session, projectId: string) {
  const res = await owner.api.get(`/api/projects/${projectId}/runs`)
  expect(res.ok()).toBeTruthy()
  return ((await res.json()) as { items: RunRow[] }).items
    .filter((r) => r.kind === 'simulation')
    .map(({ id, kind, status, job_id }) => ({ id, kind, status, job_id }))
}
