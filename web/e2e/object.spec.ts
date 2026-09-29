import { expect, test, type Page } from "@playwright/test";

import { synced } from "./helpers.js";

// A demo lives in the store, in IndexedDB, exactly like a project on the server does in its tables.
const storedArea = (page: Page) =>
  page.evaluate(async () => {
    const db = await new Promise<IDBDatabase>((resolve, reject) => {
      const r = indexedDB.open("robots-store");
      r.onsuccess = () => resolve(r.result);
      r.onerror = () => reject(r.error);
    });
    const snap = await new Promise<
      { confirmed: Record<string, Record<string, Record<string, unknown>>> } | undefined
    >((resolve) => {
      const q = db
        .transaction("snapshots", "readonly")
        .objectStore("snapshots")
        .get("demo-warehouse:guest");
      q.onsuccess = () => resolve(q.result as never);
      q.onerror = () => resolve(undefined);
    });
    const params = snap?.confirmed?.project?.project?.params as Record<string, unknown> | undefined;
    return params?.area_total_m2;
  });

test("the object form has no Сохранить or Рассчитать button", async ({ page }) => {
  await page.goto("/demo/warehouse/object/site");
  await expect(page.locator("input#area_total_m2")).toBeVisible();
  await expect(page.getByRole("button", { name: "Сохранить", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Рассчитать", exact: true })).toHaveCount(0);
});

test("a field saves itself and the change is there after a reload", async ({ page }) => {
  await page.goto("/demo/warehouse/object/site");
  const area = page.locator("input#area_total_m2");
  await expect(area).toBeVisible();
  await expect.poll(() => storedArea(page)).toBe(20000);

  // Typing alone does not save; leaving the field does.
  await area.fill("32000");
  await area.blur();
  await synced(page);
  await expect.poll(() => storedArea(page)).toBe(32000);

  await page.reload();
  await expect(page.locator("input#area_total_m2")).toHaveValue("32\u00a0000");
});

test("a dropdown saves as soon as it is picked", async ({ page }) => {
  await page.goto("/demo/warehouse/object/site");
  const floor = page.locator("#floor_type");
  await expect(floor).toBeVisible();
  await floor.click();
  await page.getByRole("listbox").getByRole("option").nth(1).click();
  const picked = await floor.getAttribute("data-value");
  await synced(page);

  await page.reload();
  await expect(page.locator("#floor_type")).toHaveAttribute("data-value", picked ?? "");
});

test("a value out of range stays in the field, is not saved and marks its subtab", async ({
  page,
}) => {
  await page.goto("/demo/warehouse/object/site");
  const area = page.locator("input#area_total_m2");
  await expect(area).toBeVisible();
  await area.fill("5");
  await area.blur();
  const field = page.locator(".field-row", { has: area });
  await expect(field.locator("p.error")).toHaveText(
    "Укажите число от 10\u00a0000 до 100\u00a0000.",
  );
  const site = page
    .getByRole("navigation", { name: "Подразделы" })
    .getByRole("link", { name: /^Площадка/ });
  await expect(site).toContainText("есть ошибки");
  expect(await storedArea(page)).toBe(20000);

  await area.fill("45000");
  await area.blur();
  await expect(field.locator("p.error")).toHaveCount(0);
  await expect(site).not.toContainText("есть ошибки");
  await expect.poll(() => storedArea(page)).toBe(45000);
});

test("undo and redo show in the fields, and a value that failed its check stays", async ({
  page,
}) => {
  await page.goto("/demo/warehouse/object/site");
  const area = page.locator("input#area_total_m2");
  const ceiling = page.locator("input#ceiling_height_m");
  await expect(area).toHaveValue("20\u00a0000");

  await area.fill("21000");
  await area.press("Enter");
  await area.blur();
  await expect.poll(() => storedArea(page)).toBe(21000);

  await ceiling.fill("123");
  await ceiling.blur();
  await expect(page.locator(".field-row", { has: ceiling }).locator("p.error")).toBeVisible();

  // NOTE: the Desktop Chrome device reports Windows, so the app takes ctrl for undo on every host.
  await page.keyboard.press("Control+Z");
  await expect(area).toHaveValue("20\u00a0000");
  await expect(ceiling).toHaveValue("123");
  await expect.poll(() => storedArea(page)).toBe(20000);

  await page.keyboard.press("Control+Shift+Z");
  await expect(area).toHaveValue("21\u00a0000");
});

test("a field explains itself behind its question mark", async ({ page }) => {
  await page.goto("/demo/warehouse/object/site");
  const help = page.getByRole("button", { name: "Пояснение: Высота потолков в зоне хранения" });
  // NOTE: a quiet field shows its "?" while the row is hovered.
  await page.locator("input#ceiling_height_m").hover();
  await help.click();
  const note = page.getByRole("dialog", { name: "Пояснение: Высота потолков в зоне хранения" });
  await expect(note).toContainText("от 5 до 16 м");
  await page.keyboard.press("Escape");
  await expect(note).toBeHidden();
  await expect(help).toBeFocused();
});

test.describe("on a phone", () => {
  // NOTE: the "?" stays in sight only where the pointer is coarse, which a resized desktop window does not report.
  test.use({ viewport: { width: 375, height: 812 }, hasTouch: true, isMobile: true });

  test("help panels stay inside the window", async ({ page }) => {
    await page.goto("/demo/warehouse/object/infra");
    const buttons = page.getByRole("button", { name: /^Пояснение: / });
    await expect(buttons.first()).toBeVisible();
    for (const button of await buttons.all()) {
      await button.click();
      const panel = page.getByRole("dialog", { name: /^Пояснение: / });
      await expect(panel).toBeVisible();
      const edges = await panel.evaluate((el) => {
        const r = el.getBoundingClientRect();
        return { left: r.left, right: r.right, page: document.documentElement.scrollWidth };
      });
      expect(edges.left).toBeGreaterThanOrEqual(0);
      expect(edges.right).toBeLessThanOrEqual(375);
      expect(edges.page).toBe(375);
      await page.keyboard.press("Escape");
      await expect(panel).toBeHidden();
    }
  });
});

test("a unit sits inside its field and controls in one row keep one height", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/demo/warehouse/object/infra");
  // The label shows the short name of the quantity; the unit stays in the accessible name.
  const power = page.getByLabel("Доступная мощность (кВт)");
  await expect(power).toBeVisible();
  await expect(page.locator(".unit-field", { has: power })).toContainText("кВт");
  await expect(page.getByText("можно", { exact: false })).toHaveCount(0);

  const height = (selector: string) =>
    page.locator(selector).evaluate((el) => el.getBoundingClientRect().height);
  const box = await page
    .locator(".unit-field", { has: power })
    .evaluate((el) => el.getBoundingClientRect().height);
  expect(await height("#has_wms")).toBe(box);
  expect(await height("#erp_system")).toBe(box);
});
