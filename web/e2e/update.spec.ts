import { expect, test, type Page } from "@playwright/test";

import { subtab } from "./helpers.ts";

// The worker is registered only in a production build, which is what the compose stack serves.

async function openSim(page: Page): Promise<void> {
  await page.getByRole("navigation", { name: "Разделы", exact: true }).getByRole("link", { name: "Расчёт", exact: true }).click();
  await page.getByRole("navigation", { name: "Подразделы" }).getByRole("link", { name: "Симуляция", exact: true }).click();
}

// shellOfWorker asks the controlling worker which files its build needs, as the page does.
async function shellOfWorker(page: Page): Promise<string[]> {
  return page.evaluate(
    () =>
      new Promise<string[]>((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error("the worker did not answer")), 3000);
        const on = (e: MessageEvent) => {
          const shell = (e.data as { shell?: string[] } | null)?.shell;
          if (shell) {
            clearTimeout(timer);
            navigator.serviceWorker.removeEventListener("message", on);
            resolve(shell);
          }
        };
        navigator.serviceWorker.addEventListener("message", on);
        navigator.serviceWorker.controller?.postMessage("version");
      }),
  );
}

// entryOfPage is the path of the entry chunk this page runs.
function entryOfPage(page: Page): Promise<string> {
  return page.evaluate(() => new URL((document.querySelector('script[type="module"][src*="/assets/index-"]') as HTMLScriptElement).src).pathname);
}

// installNewBuild plays what happens when a newer worker takes over: the controller changes, and the worker
// answers the page's question with a shell that no longer has this page's entry chunk.
async function installNewBuild(page: Page): Promise<void> {
  await page.evaluate(() => {
    const sw = navigator.serviceWorker;
    sw.dispatchEvent(new Event("controllerchange"));
    sw.dispatchEvent(new MessageEvent("message", { data: { version: "e2e-next", shell: ["/index.html"] } }));
  });
}

test("a new build installs without a toast, and the old tab reloads itself only when that is safe", async ({ page }) => {
  await page.goto("/demo/warehouse/object");
  await expect.poll(() => page.evaluate(() => navigator.serviceWorker.controller !== null)).toBe(true);
  await subtab(page, "Персонал");
  const oldPage = () => page.evaluate(() => (window as unknown as { oldPage?: boolean }).oldPage === true);
  await page.evaluate(() => {
    (window as unknown as { oldPage: boolean }).oldPage = true;
  });

  await test.step("the worker of this build carries this page's entry chunk, so the page is not behind", async () => {
    expect(await shellOfWorker(page)).toContain(await entryOfPage(page));
    expect(await page.evaluate(async () => (await navigator.serviceWorker.getRegistration())?.waiting ?? null)).toBeNull();
  });

  await test.step("a field has focus while the new build takes over: nothing reloads and nothing is shown", async () => {
    await page.locator("input#wage_picker_month_rub").focus();
    await installNewBuild(page);
    await page.waitForTimeout(1500);
    expect(await oldPage()).toBe(true);
    await expect(page.getByText("Доступна новая версия")).toHaveCount(0);
  });

  await test.step("the next move to another subtab reloads onto the new page, once", async () => {
    await page.locator("input#wage_picker_month_rub").blur();
    await Promise.all([
      page.waitForEvent("load"),
      page.getByRole("navigation", { name: "Подразделы" }).getByRole("link", { name: /^Процессы/ }).click(),
    ]);
    await expect(page).toHaveURL(/\/demo\/warehouse\/object\/processes$/);
    expect(await oldPage()).toBe(false);
    await expect(page.getByText("Доступна новая версия")).toHaveCount(0);
  });

  await test.step("the reloaded page and the worker agree, and nothing asks for a second reload", async () => {
    expect(await shellOfWorker(page)).toContain(await entryOfPage(page));
    await page.evaluate(() => {
      (window as unknown as { oldPage: boolean }).oldPage = true;
    });
    await subtab(page, "Персонал");
    await page.waitForTimeout(1500);
    expect(await oldPage()).toBe(true);
  });

  await test.step("a dialog that is open holds the reload too", async () => {
    await installNewBuild(page);
    await page.evaluate(() => {
      const d = document.createElement("dialog");
      d.id = "e2e-dialog";
      document.body.appendChild(d);
      d.show();
    });
    await page.getByRole("navigation", { name: "Подразделы" }).getByRole("link", { name: /^Процессы/ }).click();
    await page.waitForTimeout(1000);
    expect(await oldPage()).toBe(true);
    await page.evaluate(() => document.querySelector<HTMLDialogElement>("#e2e-dialog")?.remove());
  });
});

test.describe("a code chunk that is gone", () => {
  test.use({ serviceWorkers: "block" });

  test("reloads the page once by itself and opens the page", async ({ page }) => {
    let failed = 0;
    await page.route("**/assets/Sim-*.js", (route) => {
      if (failed === 0) {
        failed++;
        return route.fulfill({ status: 404, body: "gone" });
      }
      return route.continue();
    });
    let loads = 0;
    page.on("load", () => loads++);
    await page.goto("/demo/warehouse/object");
    await openSim(page);
    await expect(page).toHaveURL(/\/demo\/warehouse\/calc\/sim$/);
    await expect(page.getByRole("heading", { name: "Вышла новая версия" })).toHaveCount(0);
    await expect(page.locator(".app")).toBeVisible();
    await expect.poll(() => loads).toBe(2);
    expect(failed).toBe(1);
  });

  test("shows «Вышла новая версия» when the reload did not help, and does not loop", async ({ page }) => {
    await page.route("**/assets/Sim-*.js", (route) => route.fulfill({ status: 404, body: "gone" }));
    let loads = 0;
    page.on("load", () => loads++);
    await page.goto("/demo/warehouse/object");
    await openSim(page);
    await expect(page.getByRole("heading", { name: "Вышла новая версия" })).toBeVisible();
    await page.waitForTimeout(1500);
    expect(loads).toBe(2);
  });
});
