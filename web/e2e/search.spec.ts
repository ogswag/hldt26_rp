import { expect, test } from "@playwright/test";

// The search in the tab row: Cmd/ctrl+K, arrows that move the page, Enter that focuses the found field, and one
// history entry per search.
test("search walks the results with arrows and focuses the pick", async ({ page }) => {
  await page.goto("/demo/warehouse/object/site");
  const box = page.getByRole("combobox", { name: "Поиск по проекту" });
  await expect(page.getByRole("textbox", { name: /^Площадь склада/ })).toBeVisible();
  // NOTE: the e2e browser reports Windows, so the app listens for ctrl, as for ctrl+Z in the other specs.
  await page.keyboard.press("Control+k");
  await expect(box).toBeFocused();

  await test.step("prefixes of two words find the field", async () => {
    await box.fill("пер баз");
    const first = page
      .getByRole("listbox", { name: "Результаты поиска" })
      .getByRole("option")
      .first();
    await expect(first).toContainText("Персонал базы");
    await expect(first).toContainText("Объект / Процессы");
  });

  await test.step("an arrow step moves the page, Enter opens the field in its dialog", async () => {
    await box.press("ArrowDown");
    await expect(page).toHaveURL(/\/object\/processes$/);
    await box.press("Enter");
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("textbox", { name: /^Персонал базы/ })).toBeFocused();
    await expect(dialog.locator(".search-hit")).toHaveCount(1);
    await dialog.getByRole("button", { name: "Готово" }).click();
  });

  await test.step("Back returns to where the search started", async () => {
    await page.goBack();
    await expect(page).toHaveURL(/\/object\/site$/);
  });
});

test("a search command runs on Enter", async ({ page }) => {
  await page.goto("/demo/warehouse/object/site");
  const box = page.getByRole("combobox", { name: "Поиск по проекту" });
  await box.click();
  await box.fill("тема тёмная");
  await box.press("Enter");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
});

test("on a phone the magnifier opens the search over the tab row", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto("/demo/warehouse/object/site");
  await page.getByRole("button", { name: "Поиск", exact: true }).click();
  const box = page.getByRole("combobox", { name: "Поиск по проекту" });
  await expect(box).toBeFocused();
  await box.fill("площадь скл");
  await page.getByRole("option", { name: /Площадь склада/ }).click();
  await expect(page.getByRole("textbox", { name: /^Площадь склада/ })).toBeFocused();
});
