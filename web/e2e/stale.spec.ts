import { expect, test, type Page } from "@playwright/test";

import { apiProject, apiRegister, saved, signIn, type ProjectJSON } from "./helpers.ts";

// The «расчёт устарел» flag: an edit turns it on, undo of that edit or a recalculation turns it off. The project
// card, История and the API agree.
test("the stale flag follows an edit, its undo and a recalculation", async ({ page }) => {
  const session = await apiRegister("stale");
  const project = await apiProject(session, `E2E устаревание ${Date.now()}`);
  await signIn(page, session);
  const stale = async () =>
    ((await (await session.api.get(`/api/projects/${project.id}`)).json()) as ProjectJSON).stale;
  const cardTag = async () => {
    await page.goto("/");
    return page.locator(".project-card", { hasText: project.name }).locator(".tag");
  };
  const res = await session.api.post(`/api/projects/${project.id}/calculations`, {
    data: { seed: 1 },
  });
  expect(res.status(), await res.text()).toBe(200);
  await expect(await cardTag()).toHaveText("рассчитан");

  await test.step("an edit makes the result stale and its undo makes it current again", async () => {
    await page.goto(`/p/${project.id}/object/processes`);
    await setPriority(page, project.id, "7");
    await expect.poll(stale).toBe(true);
    await page.keyboard.press("Control+Z");
    await expect.poll(stale).toBe(false);
  });

  await test.step("the card and История show a stale result", async () => {
    await page.goto(`/p/${project.id}/object/processes`);
    await setPriority(page, project.id, "8");
    await expect.poll(stale).toBe(true);
    await expect(await cardTag()).toHaveText("расчёт устарел");
    await page.goto(`/p/${project.id}/calc/history`);
    await expect(page.locator(".stale-banner")).toBeVisible();
    await expect(page.locator('tr[data-run-kind="calculation"]').first()).toContainText("устарел");
  });

  await test.step("opening Итог recalculates and clears the flag", async () => {
    await page.goto(`/p/${project.id}/calc/summary`);
    await expect.poll(stale, { timeout: 30_000 }).toBe(false);
    await expect(await cardTag()).toHaveText("рассчитан");
  });
});

// setPriority opens the first process in its editor and saves a new SLA priority.
async function setPriority(page: Page, projectId: string, value: string): Promise<void> {
  await page.locator(".task-card-title").first().click();
  const field = page.getByRole("dialog").getByRole("textbox", { name: "Приоритет, 0-9" });
  await saved(page, projectId, async () => {
    await field.fill(value);
    await field.press("Tab");
  });
  await page.getByRole("dialog").getByRole("button", { name: "Готово" }).click();
}
