import { randomUUID } from 'node:crypto'
import { readFile } from 'node:fs/promises'

import { expect, request as playwrightRequest, test, type APIRequestContext, type Download, type Locator, type Page } from '@playwright/test'
import * as XLSX from 'xlsx'

import type { MapDocument } from '../src/api/client'
import { commandOps } from '../src/map/records'
import type { RawOp, State } from '../src/store/apply'
import { schemaVersion } from '../src/store/schema.gen'

export const H1500 = '5760e938-9a43-45a7-b8e8-f4f2e6383930'
export const STACKER = '2ffc706d-fe43-4c2b-baad-a624a95ad3ce'
export const PASSWORD = 'e2e-password-1'
// ADMIN is the demo admin that compose creates (DEMO_ADMIN_EMAIL, DEMO_ADMIN_PASSWORD).
export const ADMIN = { email: 'admin@demo.local', password: 'demo-admin' }

// Session is one signed-in user. api keeps that user's cookie and sends the CSRF token, like the web client.
export type Session = {
  api: APIRequestContext
  user: { id: string; email: string; role: string }
}

type Variant = {
  id: string
  name: string
  status: string
  sort_order: number
  fleet: Record<string, unknown>[]
  financing: Record<string, unknown>[]
}

export type ProjectJSON = {
  id: string
  name: string
  variants: Variant[]
  stale: boolean
  current_run_id: string | null
}

export function uniqueEmail(tag: string): string {
  return `e2e-${tag}-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`
}

// apiRegister creates a user in a request context of its own, so several users can act in one test.
export async function apiRegister(tag: string): Promise<Session> {
  const baseURL = test.info().project.use.baseURL
  const anon = await playwrightRequest.newContext({ baseURL, extraHTTPHeaders: { Origin: baseURL ?? 'http://localhost' } })
  const res = await anon.post('/api/auth/register', { data: { email: uniqueEmail(tag), password: PASSWORD } })
  expect(res.status(), await res.text()).toBe(200)
  const body = (await res.json()) as { user: Session['user']; csrf_token: string }
  const api = await playwrightRequest.newContext({
    baseURL,
    storageState: await anon.storageState(),
    extraHTTPHeaders: { Origin: baseURL ?? 'http://localhost', 'X-CSRF-Token': body.csrf_token },
  })
  await anon.dispose()
  return { api, user: body.user }
}

// adminSession signs the demo admin in, in a request context of its own.
export async function adminSession(): Promise<Session> {
  const baseURL = test.info().project.use.baseURL
  const anon = await playwrightRequest.newContext({ baseURL, extraHTTPHeaders: { Origin: baseURL ?? 'http://localhost' } })
  const res = await anon.post('/api/auth/login', { data: ADMIN })
  expect(res.status(), await res.text()).toBe(200)
  const body = (await res.json()) as { user: Session['user']; csrf_token: string }
  const api = await playwrightRequest.newContext({
    baseURL,
    storageState: await anon.storageState(),
    extraHTTPHeaders: { Origin: baseURL ?? 'http://localhost', 'X-CSRF-Token': body.csrf_token },
  })
  await anon.dispose()
  return { api, user: body.user }
}

// sessionOf turns a browser that signed in through the UI into an API session for setup the UI no longer offers.
export async function sessionOf(page: Page): Promise<Session> {
  const baseURL = test.info().project.use.baseURL
  const res = await page.request.get('/api/auth/me')
  expect(res.ok(), await res.text()).toBeTruthy()
  const body = (await res.json()) as { user: Session['user']; csrf_token: string }
  const api = await playwrightRequest.newContext({
    baseURL,
    storageState: await page.context().storageState(),
    extraHTTPHeaders: { Origin: baseURL ?? 'http://localhost', 'X-CSRF-Token': body.csrf_token },
  })
  return { api, user: body.user }
}

export async function schemaDefaults(request: APIRequestContext): Promise<Record<string, unknown>> {
  const res = await request.get('/api/object-types/warehouse/schema')
  expect(res.ok()).toBeTruthy()
  const schema = (await res.json()) as { groups: { fields: { id: string; default: unknown }[] }[] }
  const out: Record<string, unknown> = {}
  for (const g of schema.groups) {
    for (const f of g.fields) {
      out[f.id] = f.default
    }
  }
  return out
}

// postOps sends one transaction of operations, the way the app does, and insists the server took it.
export async function postOps(request: APIRequestContext, projectId: string, ops: RawOp[], label = 'e2e'): Promise<void> {
  const res = await request.post(`/api/projects/${projectId}/transactions`, {
    data: { client_id: 'e2e', schema_version: schemaVersion, txs: [{ tx_id: randomUUID(), label, ops }] },
  })
  expect(res.ok(), await res.text()).toBeTruthy()
  const body = (await res.json()) as { results: { status: string; reason?: string; details?: unknown }[] }
  expect(body.results[0].status, JSON.stringify(body.results[0])).toBe('applied')
}

// snapshot is the project as records, the same view the store works on.
export async function snapshot(request: APIRequestContext, projectId: string): Promise<State> {
  const res = await request.get(`/api/projects/${projectId}/snapshot`)
  expect(res.ok(), await res.text()).toBeTruthy()
  return ((await res.json()) as { collections: State }).collections
}

export function fleetItem(variantId: string, solutionId: string, order: string, quantity: number, taskCodes: string[]): RawOp {
  return {
    op: 'insert',
    coll: 'fleet_items',
    id: randomUUID(),
    value: {
      order,
      variant_id: variantId,
      solution_id: solutionId,
      quantity,
      task_codes: taskCodes,
      price_override_rub: null,
      price_override_reason: '',
    },
  }
}

// apiMap writes the template map through operations, exactly as the map editor's "load template" does.
async function apiMap(request: APIRequestContext, projectId: string): Promise<void> {
  const res = await request.get(`/api/projects/${projectId}/map/template`)
  expect(res.ok(), await res.text()).toBeTruthy()
  const template = (await res.json()) as MapDocument
  const st = await snapshot(request, projectId)
  await postOps(request, projectId, commandOps(st, { type: 'replace', doc: template }, template.page!), 'Загрузить шаблон карты')
}

// apiProject creates a warehouse project with a mixed fleet in the second variant and the template map.
export async function apiProject(session: Session, name: string): Promise<ProjectJSON> {
  const request = session.api
  const params = await schemaDefaults(request)
  let res = await request.post('/api/projects', { data: { name, object_type: 'warehouse', params } })
  expect(res.status(), await res.text()).toBe(201)
  const project = (await res.json()) as ProjectJSON
  const variant = project.variants[1]
  await postOps(request, project.id, [
    fleetItem(variant.id, H1500, 'a0', 1, ['inbound']),
    fleetItem(variant.id, STACKER, 'a1', 1, ['putaway']),
  ], 'Собрать флот')
  await apiMap(request, project.id)
  res = await request.get(`/api/projects/${project.id}`)
  return (await res.json()) as ProjectJSON
}

// heavyLoad makes the first variant run for several seconds so a job can be canceled from the UI.
export async function heavyLoad(session: Session, project: ProjectJSON): Promise<string> {
  const request = session.api
  const st = await snapshot(request, project.id)
  const picking = Object.values(st.processes ?? {}).find((p) => (p as { code: string }).code === 'piece_pick') as
    | { id: string; demand: Record<string, unknown> }
    | undefined
  expect(picking, 'the warehouse defaults have a piece_pick process').toBeTruthy()
  const variant = project.variants[0]
  await postOps(request, project.id, [
    { op: 'set', coll: 'processes', id: picking!.id, path: 'demand', value: { ...picking!.demand, units_per_day: 100000, units_per_job: 1 } },
    fleetItem(variant.id, H1500, 'a0', 20, []),
  ], 'Нагрузить проект')
  return variant.id
}

export type Choice = { name: string | RegExp } | { value: string }

// pick opens a dropdown field and chooses an option by its visible name or value.
export async function pick(field: Locator, choice: Choice): Promise<void> {
  await field.click()
  const list = field.page().getByRole('listbox')
  const option = 'value' in choice ? list.locator(`[role="option"][data-value="${choice.value}"]`) : list.getByRole('option', { name: choice.name })
  await option.click()
  await expect(field).toHaveAttribute('aria-expanded', 'false')
}

// subtab opens a subtab from the row under the tabs, the way a person does.
export async function subtab(page: Page, name: string): Promise<void> {
  const link = page.getByRole('navigation', { name: 'Подразделы' }).getByRole('link', { name: new RegExp(`^${name}`) })
  await link.click()
  await expect(link).toHaveAttribute('aria-current', 'page')
}

// tab opens a top tab that is open; a closed one is a button, not a link.
export async function tab(page: Page, name: string): Promise<void> {
  const link = page.getByRole('navigation', { name: 'Разделы', exact: true }).getByRole('link', { name, exact: true })
  await link.click()
  await expect(link).toHaveAttribute('aria-current', 'page')
}

// synced waits until every edit of the open project reached the server (or the browser, for a demo).
export async function synced(page: Page): Promise<void> {
  await expect(page.locator('.account-button')).toHaveAttribute('data-sync', 'saved')
}

// accountMenu opens the menu behind the round button in the top right corner.
export async function accountMenu(page: Page): Promise<Locator> {
  await page.locator('.account-button').click()
  const menu = page.getByRole('dialog', { name: 'Аккаунт и настройки' })
  await expect(menu).toBeVisible()
  return menu
}

// signIn logs the page's browser context in as the session's user; the app then reads the cookie through /api/auth/me.
export async function signIn(page: Page, session: Session): Promise<void> {
  const res = await page.request.post('/api/auth/login', { data: { email: session.user.email, password: PASSWORD } })
  expect(res.status(), await res.text()).toBe(200)
}

// adminSignIn logs the page's browser context in as the demo admin.
export async function adminSignIn(page: Page): Promise<void> {
  const res = await page.request.post('/api/auth/login', { data: ADMIN })
  expect(res.status(), await res.text()).toBe(200)
}

export async function downloadBytes(download: Download): Promise<Buffer> {
  const path = await download.path()
  return readFile(path)
}

export async function readWorkbook(download: Download): Promise<XLSX.WorkBook> {
  return XLSX.read(await downloadBytes(download), { type: 'buffer' })
}

export function sheetRows(wb: XLSX.WorkBook, name: string): string[][] {
  const sheet = wb.Sheets[name]
  expect(sheet, `sheet ${name}`).toBeTruthy()
  return XLSX.utils.sheet_to_json<string[]>(sheet, { header: 1, raw: false, defval: '' })
}

// saved runs action and waits until the store has sent the change and the server accepted it.
export async function saved(page: Page, projectId: string, action: () => Promise<void>): Promise<void> {
  const sent = page.waitForResponse(
    (r) => r.request().method() === 'POST' && new URL(r.url()).pathname === `/api/projects/${projectId}/transactions`,
  )
  await action()
  const res = await sent
  expect(res.status(), await res.text()).toBe(200)
  const body = (await res.json()) as { results: { status: string; reason?: string }[] }
  expect(body.results.map((x) => x.reason ?? x.status)).toEqual(body.results.map(() => 'applied'))
  await synced(page)
}
