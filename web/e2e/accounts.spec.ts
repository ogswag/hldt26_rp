import { expect, request as playwrightRequest, test, type Page } from '@playwright/test'

import { accountMenu, apiProject, apiRegister, PASSWORD, signIn, uniqueEmail } from './helpers.ts'

type Letter = { template: string; payload: { link?: string } }

// letterToken reads the local outbox, which the API serves only when APP_ENV is local.
async function letterToken(baseURL: string, to: string, template: string): Promise<string> {
  const anon = await playwrightRequest.newContext({ baseURL })
  try {
    const res = await anon.get(`/api/dev/outbox?to=${encodeURIComponent(to)}`)
    expect(res.ok(), await res.text()).toBeTruthy()
    const body = (await res.json()) as { items: Letter[] }
    const letter = body.items.find((i) => i.template === template)
    expect(letter, `no ${template} letter for ${to}: ${JSON.stringify(body.items)}`).toBeTruthy()
    const link = letter?.payload.link ?? ''
    const token = link.split('#token=')[1] ?? ''
    expect(token, `letter link without a token: ${link}`).toBeTruthy()
    return token
  } finally {
    await anon.dispose()
  }
}

test('a deleted project waits in the trash and comes back', async ({ page, baseURL }) => {
  const owner = await apiRegister('trash')
  const project = await apiProject(owner, 'Проект для корзины')
  await signIn(page, owner)
  await page.goto('/')

  const card = page.locator('.project-card').filter({ hasText: 'Проект для корзины' })
  await expect(card).toBeVisible()
  await card.getByRole('button', { name: 'Удалить' }).click()
  await expect(page.getByText('в корзине. Он хранится 30 дней.')).toBeVisible()
  await expect(card).toHaveCount(0)

  await (await accountMenu(page)).getByRole('link', { name: 'Корзина' }).click()
  const trashed = page.locator('.project-card').filter({ hasText: 'Проект для корзины' })
  await expect(trashed).toBeVisible()
  await expect(trashed).toContainText('будет стёрт через 30 дней')

  await test.step('the project answers that it is deleted while it waits', async () => {
    const res = await owner.api.get(`/api/projects/${project.id}`)
    expect(res.status()).toBe(410)
    expect((await res.json()).code).toBe('project_deleted')
  })

  await trashed.getByRole('button', { name: 'Восстановить' }).click()
  await expect(page.getByText('восстановлен.')).toBeVisible()
  await page.goto('/')
  await expect(page.locator('.project-card').filter({ hasText: 'Проект для корзины' })).toBeVisible()

  await test.step('a purge from the trash cannot be taken back', async () => {
    await page.locator('.project-card').filter({ hasText: 'Проект для корзины' }).getByRole('button', { name: 'Удалить' }).click()
    await page.goto('/trash')
    await page.locator('.project-card').filter({ hasText: 'Проект для корзины' }).getByRole('button', { name: 'Удалить навсегда' }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toContainText('Восстановить их будет нельзя.')
    await dialog.getByRole('button', { name: 'Удалить навсегда' }).click()
    await expect(page.getByText('Корзина пуста.')).toBeVisible()
    expect((await owner.api.get(`/api/projects/${project.id}`)).status()).toBe(404)
  })
  expect(baseURL).toBeTruthy()
})

test('a new account confirms its email and can reset the password', async ({ page, baseURL }) => {
  const email = uniqueEmail('reset')
  await page.goto('/register')
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Пароль (от 8 символов)').fill(PASSWORD)
  await page.getByRole('button', { name: 'Создать аккаунт' }).click()
  await expect(page.getByRole('heading', { name: 'Проекты', exact: true })).toBeVisible()
  await expect(await accountMenu(page)).toContainText(email)

  const verify = await letterToken(baseURL ?? 'http://localhost', email, 'verify_email')
  await page.goto(`/verify-email#token=${verify}`)
  await expect(page.getByText('Адрес подтверждён.')).toBeVisible()

  await page.goto('/reset-password')
  await page.getByLabel('Email').fill(email)
  await page.getByRole('button', { name: 'Прислать ссылку' }).click()
  await expect(page.getByText('письмо со ссылкой уже отправлено', { exact: false })).toBeVisible()

  const reset = await letterToken(baseURL ?? 'http://localhost', email, 'reset_password')
  await page.goto(`/reset-password#token=${reset}`)
  await page.getByLabel('Пароль (от 8 символов)').fill('e2e-password-2')
  await page.getByRole('button', { name: 'Сохранить пароль' }).click()
  await expect(page.getByRole('heading', { name: 'Вход' })).toBeVisible()

  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Пароль').fill('e2e-password-2')
  await page.getByRole('button', { name: 'Войти' }).click()
  await expect(page.getByRole('heading', { name: 'Проекты', exact: true })).toBeVisible()
  await expect(await accountMenu(page)).toContainText(email)
})

test('an invitation to a project makes the newcomer an editor', async ({ page, baseURL }) => {
  const owner = await apiRegister('inviter')
  const project = await apiProject(owner, 'Проект с приглашением')
  const guest = uniqueEmail('invited')
  const res = await owner.api.post(`/api/projects/${project.id}/members`, { data: { email: guest, role: 'editor' } })
  expect(res.status(), await res.text()).toBe(202)

  const token = await letterToken(baseURL ?? 'http://localhost', guest, 'invite')
  await page.goto(`/register#token=${token}`)
  await expect(page.getByText('Приглашение в проект «Проект с приглашением»')).toBeVisible()
  await expect(page.getByLabel('Email')).toHaveValue(guest)
  await page.getByLabel('Пароль (от 8 символов)').fill(PASSWORD)
  await page.getByRole('button', { name: 'Создать аккаунт' }).click()
  await expect(await accountMenu(page)).toContainText(guest)

  await page.goto('/')
  await expect(page.locator('.project-card').filter({ hasText: 'Проект с приглашением' })).toBeVisible()
  await openAndEdit(page, project.id)
})

// openAndEdit proves the invited person really has editor rights, not just the project in the list.
async function openAndEdit(page: Page, projectId: string) {
  await page.goto(`/p/${projectId}/object/site`)
  await expect(page.getByRole('heading', { name: 'Площадка', level: 1 })).toBeVisible()
  await expect(page.locator('input#area_total_m2')).toBeEnabled()
}
