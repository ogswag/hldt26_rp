import { expect, test, type Browser, type Page } from "@playwright/test";

import { apiProject, apiRegister, signIn, type Session } from "./helpers.ts";

async function openAt(browser: Browser, session: Session, path: string): Promise<Page> {
  const page = await (await browser.newContext()).newPage();
  await signIn(page, session);
  await page.goto(path);
  await expect(page.locator(".account-button")).toBeVisible();
  return page;
}

const faces = (page: Page, email: string) => page.getByLabel(`В проекте: ${email}`);

test("your own other tab is not a person, and people on any page show", async ({ browser }) => {
  const owner = await apiRegister("presence-owner");
  const editor = await apiRegister("presence-editor");
  const project = await apiProject(owner, "Присутствие E2E");
  const share = await owner.api.post(`/api/projects/${project.id}/members`, {
    data: { email: editor.user.email, role: "editor" },
  });
  expect(share.ok(), await share.text()).toBeTruthy();

  const a = await openAt(browser, owner, `/p/${project.id}/robots`);
  // A second tab of the same browser and the same account.
  const b = await a.context().newPage();
  await b.goto(`/p/${project.id}/object`);
  await expect(b.locator(".account-button")).toBeVisible();
  const c = await openAt(browser, editor, `/p/${project.id}/robots`);

  await test.step("the editor sees the owner once, and the owner sees only the editor", async () => {
    await expect(faces(c, owner.user.email)).toBeVisible({ timeout: 5_000 });
    await expect(c.locator(".face")).toHaveCount(1);
    for (const tab of [a, b]) {
      await expect(faces(tab, editor.user.email)).toBeVisible({ timeout: 5_000 });
      await expect(tab.locator(".face")).toHaveCount(1);
      await expect(faces(tab, owner.user.email)).toHaveCount(0);
    }
  });

  await test.step("a tab opened later sees a person who is not on the map or on the processes", async () => {
    const d = await a.context().newPage();
    await d.goto(`/p/${project.id}/object/map`);
    await expect(faces(d, editor.user.email)).toBeVisible({ timeout: 5_000 });
    await expect(d.locator(".face")).toHaveCount(1);
    await d.close();
  });

  await test.step("a closed tab takes its face away", async () => {
    await c.close({ runBeforeUnload: true });
    for (const tab of [a, b]) {
      await expect(tab.locator(".face")).toHaveCount(0, { timeout: 5_000 });
    }
  });
});
