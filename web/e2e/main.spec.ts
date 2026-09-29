import { expect, test } from '@playwright/test'

import {
  H1500,
  PASSWORD,
  STACKER,
  accountMenu,
  downloadBytes,
  fleetItem,
  pick,
  postOps,
  readWorkbook,
  sessionOf,
  sheetRows,
  subtab,
  synced,
  tab,
  uniqueEmail,
  type ProjectJSON,
} from './helpers.ts'

test('warehouse project from registration to a report bound to one run', async ({ page }) => {
  const email = uniqueEmail('main')
  const projectName = `E2E склад ${Date.now()}`
  let projectId = ''

  await test.step('register', async () => {
    await page.goto('/register')
    await page.getByLabel('Email').fill(email)
    await page.getByLabel('Пароль (от 8 символов)').fill(PASSWORD)
    await page.getByRole('button', { name: 'Создать аккаунт' }).click()
    await expect(page.getByRole('heading', { name: 'Проекты', exact: true })).toBeVisible()
    const menu = await accountMenu(page)
    await expect(menu).toContainText(email)
    await page.keyboard.press('Escape')
  })

  await test.step('create a warehouse project from the start page', async () => {
    await page.getByRole('button', { name: 'Новый проект' }).click()
    const dialog = page.getByRole('dialog', { name: 'Новый проект' })
    await dialog.getByLabel('Название').fill(projectName)
    await expect(dialog.getByLabel('Тип объекта')).toContainText('Склад')
    await dialog.getByRole('button', { name: 'Создать', exact: true }).click()
    await page.waitForURL(/\/p\/[0-9a-f-]{36}\/object\/site$/)
    projectId = new URL(page.url()).pathname.split('/')[2] ?? ''
    await expect(page.getByRole('heading', { name: 'Площадка', level: 1 })).toBeVisible()
    await expect(page.locator('#floor_type')).toHaveAttribute('data-value', 'Промышленный бетон')
  })

  await test.step('typed parameters keep an explicit unknown value', async () => {
    await subtab(page, 'Инфраструктура')
    const wms = page.locator('#has_wms')
    await wms.click()
    await expect(page.getByRole('listbox').getByRole('option', { name: 'Неизвестно' })).toBeVisible()
    await wms.press('Escape')
    await expect(wms).toHaveAttribute('aria-expanded', 'false')
    // A number field is left empty for «Неизвестно»; its "?" says so.
    const power = page.locator('input#power_kw')
    await power.fill('')
    await power.blur()
    await synced(page)
    await page.reload()
    await expect(page.locator('input#power_kw')).toHaveValue('')
  })

  await test.step('baseline processes are a subtab of the object', async () => {
    await subtab(page, 'Процессы')
    await expect(page.getByRole('heading', { name: 'Базовые процессы' })).toBeVisible()
    await expect(page.locator('[data-process]')).toHaveCount(4)
    await expect(page.locator('[data-process]').first()).toHaveAttribute('data-process', 'inbound')
  })

  await test.step('the calculation runs by itself once the key tabs are reviewed', async () => {
    await subtab(page, 'Режим и объёмы')
    await subtab(page, 'Персонал')
    await synced(page)
    await tab(page, 'Расчёт')
    await expect(page).toHaveURL(new RegExp(`/p/${projectId}/calc/summary$`))
    await expect(page.getByRole('columnheader', { name: 'Покупка' })).toBeVisible()

    await tab(page, 'Роботы')
    await expect(page.getByRole('heading', { name: 'Роботы', exact: true })).toBeAttached()
    await expect(page.getByText('в расчёте', { exact: true })).toHaveCount(1)
  })

  await test.step('warehouse map is marked from the template', async () => {
    await tab(page, 'Объект')
    await subtab(page, 'Карта')
    await page.getByRole('button', { name: 'Шаблон склада' }).click()
    await expect(page.getByText(/Загружен шаблон склада/)).toBeVisible()
    await synced(page)
  })

  await test.step('simulation runs as a background job and exports its report', async () => {
    // A fleet of two robots, each on its own process, is quicker to set up through the API than on Роботы.
    const session = await sessionOf(page)
    const res = await session.api.get(`/api/projects/${projectId}`)
    const variant = ((await res.json()) as ProjectJSON).variants[1]
    await postOps(session.api, projectId, [
      fleetItem(variant.id, H1500, 'a0', 2, ['inbound']),
      fleetItem(variant.id, STACKER, 'a1', 1, ['putaway']),
    ], 'Собрать флот')
    await session.api.dispose()

    await tab(page, 'Расчёт')
    await subtab(page, 'Симуляция')
    await expect(page.getByText('Карта проекта задана.')).toBeVisible()
    await pick(page.getByLabel('Вариант'), { name: /Вариант 2/ })
    await pick(page.getByLabel('Режим'), { value: 'deterministic' })
    await page.getByLabel('Горизонт, ч').fill('1')
    await page.getByRole('button', { name: 'Запустить симуляцию' }).click()
    await expect(page.getByRole('cell', { name: 'готово', exact: true })).toBeVisible({ timeout: 120_000 })
    await expect(page.getByText('Уровень подтверждённости: Настроенный', { exact: true })).toBeVisible()
    const [pdf] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'Отчёт PDF' }).click()])
    expect(pdf.suggestedFilename()).toMatch(/^simulyaciya-warehouse-\d+\.pdf$/)
    expect((await downloadBytes(pdf)).subarray(0, 4).toString()).toBe('%PDF')
  })

  await test.step('history lists immutable runs and opens a historical calculation', async () => {
    // Straight to История: Итог would recalculate the changed project before the stale export is checked.
    await page.goto(`/p/${projectId}/calc/history`)
    await expect(page.locator('tr[data-run-kind="simulation"]').first()).toContainText('Настроенный')
    const firstCalc = page.locator('tr[data-run-kind="calculation"]').last()
    await expect(firstCalc).toContainText('устарел')
    await expect(firstCalc).toContainText('Предварительный')
    await firstCalc.getByRole('link', { name: 'Открыть' }).click()
    await expect(page.getByRole('heading', { name: /Расчёт экономики, версия проекта \d+/ })).toBeVisible()
    await expect(page.getByText(/Черновик проекта изменён после этого запуска/)).toBeVisible()
    await expect(page.getByText(/Неизвестные значения: .*Мощность электроснабжения/)).toBeVisible()
    const [pdf] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'Отчёт PDF' }).click()])
    expect(pdf.suggestedFilename()).toMatch(/^raschet-warehouse-\d+\.pdf$/)
    expect((await downloadBytes(pdf)).subarray(0, 4).toString()).toBe('%PDF')
  })

  await test.step('a stale current result is not exported until recalculated', async () => {
    await subtab(page, 'Экспорт')
    await expect(page.getByText(/Черновик изменился после последнего расчёта/)).toBeVisible()
    await expect(page.getByRole('button', { name: 'Скачать PDF' })).toBeDisabled()
    await page.getByRole('button', { name: 'Пересчитать' }).click()
    await expect(page.getByRole('button', { name: 'Скачать Excel' })).toBeEnabled()
    await expect(page.getByText('Уровень подтверждённости: Предварительный', { exact: true })).toBeVisible()
    await expect(page.locator('.run-identity')).toContainText(/Расчёт №\d+/)
    await expect(page.locator('.run-identity')).not.toContainText(/[0-9a-f]{8}-[0-9a-f-]{27}/)
    const [xlsx] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'Скачать Excel' }).click()])
    expect(xlsx.suggestedFilename()).toMatch(/^raschet-warehouse-\d+\.xlsx$/)
    const wb = await readWorkbook(xlsx)
    expect(wb.SheetNames).toEqual(['Итог', 'Варианты', 'Допущения и риски'])
    const text = wb.SheetNames.flatMap((name) => sheetRows(wb, name).flat()).join('\n')
    expect(sheetRows(wb, 'Итог')[0]).toEqual([
      'Вариант',
      'Оплата',
      'Состав решения',
      'Первоначальные вложения',
      'Ежегодные затраты',
      'Годовой денежный эффект',
      'Простая окупаемость, лет',
      'Затраты за горизонт',
    ])
    expect(text).toContain('Мощность электроснабжения')
    expect(text).not.toMatch(/[0-9a-f]{8}-[0-9a-f-]{27}/)
    expect(text).not.toMatch(/LINESTRING|econ-v|match-v|sim-v|seed|NPV|IRR|ROI/)
  })

  await test.step('project reopens from the list with its inputs', async () => {
    await page.goto('/')
    await page.getByRole('link', { name: projectName }).click()
    await expect(page).toHaveURL(new RegExp(`/p/${projectId}/object/site$`))
    await subtab(page, 'Инфраструктура')
    await expect(page.locator('input#power_kw')).toHaveValue('')
  })
})
