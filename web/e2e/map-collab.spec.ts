import { expect, test, type Browser, type Page } from "@playwright/test";

import { accountMenu, apiProject, apiRegister, signIn, synced, type Session } from "./helpers.ts";

async function openMap(browser: Browser, session: Session, projectId: string): Promise<Page> {
  const page = await (await browser.newContext()).newPage();
  await signIn(page, session);
  await page.goto(`/p/${projectId}/object/map`);
  await expect(page.getByRole("toolbar", { name: "Инструменты карты" })).toBeVisible();
  await page.getByText("Точки, рёбра и зоны").click();
  await synced(page);
  return page;
}

const counts = (page: Page) => page.locator(".map-layout");

async function points(page: Page): Promise<number> {
  return Number(await counts(page).getAttribute("data-points"));
}

async function setX(page: Page, id: string, meters: string) {
  const field = page.getByRole("textbox", { name: `${id} X`, exact: true });
  await field.fill(meters);
  await field.press("Enter");
}

test("two people edit one map at once", async ({ browser }) => {
  const owner = await apiRegister("map-owner");
  const editor = await apiRegister("map-editor");
  const project = await apiProject(owner, "Совместная карта E2E");
  const share = await owner.api.post(`/api/projects/${project.id}/members`, {
    data: { email: editor.user.email, role: "editor" },
  });
  expect(share.ok(), await share.text()).toBeTruthy();

  const a = await openMap(browser, owner, project.id);
  const b = await openMap(browser, editor, project.id);
  const n = await points(a);

  await test.step("each browser names who else has the map open", async () => {
    await expect(a.getByLabel(`В проекте: ${editor.user.email}`)).toBeVisible({ timeout: 5_000 });
    await expect(b.getByLabel(`В проекте: ${owner.user.email}`)).toBeVisible({ timeout: 5_000 });
  });

  await test.step("a point added in one browser shows in the other within 2 s", async () => {
    const form = a
      .locator("form.inline-form")
      .filter({ has: a.getByRole("button", { name: "Добавить точку" }) });
    await form.getByLabel("X новой точки, м").fill("30");
    await form.getByLabel("Y новой точки, м").fill("20");
    await form.getByRole("button", { name: "Добавить точку" }).click();
    await expect(counts(a)).toHaveAttribute("data-points", String(n + 1));
    await expect(counts(b)).toHaveAttribute("data-points", String(n + 1), { timeout: 2_000 });
    await expect(b.getByRole("button", { name: "T1", exact: true })).toBeVisible();
  });

  await test.step("two people move different points at the same time", async () => {
    await Promise.all([setX(a, "S1", "12"), setX(b, "S2", "14")]);
    for (const page of [a, b]) {
      await expect(page.getByRole("textbox", { name: "S1 X", exact: true })).toHaveValue("12");
      await expect(page.getByRole("textbox", { name: "S2 X", exact: true })).toHaveValue("14");
    }
    await synced(a);
    await synced(b);
  });

  await test.step("a move sent after the point was deleted goes to the refused list", async () => {
    await b.context().setOffline(true);
    await setX(b, "S3", "40");
    await expect(b.locator(".account-button")).toHaveAttribute("data-sync", "offline");
    await expect(b.locator(".account-dot")).toBeVisible();
    await expect(await accountMenu(b)).toContainText("Нет сети, 1 правка ждёт отправки.");
    await b.keyboard.press("Escape");
    await a.getByRole("button", { name: "S3", exact: true }).click();
    await a.getByRole("button", { name: "Удалить точку и её рёбра" }).click();
    await expect(counts(a)).toHaveAttribute("data-points", String(n));
    await b.context().setOffline(false);
    await synced(b);
    await expect(counts(b)).toHaveAttribute("data-points", String(n));
    await expect(b.locator(".account-dot")).toBeVisible();
  });

  await test.step("the refused move is written again from the list", async () => {
    const list = await accountMenu(b);
    await expect(list).toContainText("Не применено: 1 правка");
    await expect(list.locator(".rejected-list").getByText("Переместить точку")).toBeVisible();
    await expect(list.getByText(/Объект уже удалён/)).toBeVisible();
    await list.getByRole("button", { name: "Восстановить" }).click();
    await expect(list).not.toContainText("Не применено");
    await expect(b.locator(".account-dot")).toHaveCount(0);
    await b.keyboard.press("Escape");
    await expect(b.getByRole("textbox", { name: "S3 X", exact: true })).toHaveValue("40");
    await expect(counts(a)).toHaveAttribute("data-points", String(n + 1), { timeout: 2_000 });
    await expect(a.getByRole("textbox", { name: "S3 X", exact: true })).toHaveValue("40");
  });

  await test.step("undo leaves alone a point someone already brought back", async () => {
    await a.locator(".map-page .legend").click();
    // NOTE: the Desktop Chrome device reports Windows, so the app takes ctrl for undo on every host.
    await a.keyboard.press("Control+Z");
    await expect(counts(a)).toHaveAttribute("data-points", String(n + 1));
    await expect(a.locator(".account-dot")).toHaveCount(0);
  });

  await test.step("undo takes back a deletion of its own", async () => {
    await a.getByRole("button", { name: "S4", exact: true }).click();
    await a.getByRole("button", { name: "Удалить точку и её рёбра" }).click();
    await expect(counts(a)).toHaveAttribute("data-points", String(n));
    await expect(counts(b)).toHaveAttribute("data-points", String(n), { timeout: 2_000 });
    await a.locator(".map-page .legend").click();
    await a.keyboard.press("Control+Z");
    await expect(counts(a)).toHaveAttribute("data-points", String(n + 1));
    await expect(b.getByRole("button", { name: "S4", exact: true })).toBeVisible({
      timeout: 2_000,
    });
  });

  await test.step("the map survives a reload", async () => {
    await a.reload();
    await a.getByText("Точки, рёбра и зоны").click();
    await expect(a.getByRole("textbox", { name: "S1 X", exact: true })).toHaveValue("12");
    await expect(counts(a)).toHaveAttribute("data-points", String(n + 1));
  });
});
