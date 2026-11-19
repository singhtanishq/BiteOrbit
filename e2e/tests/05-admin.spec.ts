import { expect, test } from "@playwright/test";

/** Admin console on :3003 — overview, applications, orders, audit, support. */

const BASE = "http://localhost:3003";

async function loginAdmin(page) {
  await page.goto(`${BASE}/login`);
  await page.getByLabel("Email").fill("admin@demo.biteorbit.local");
  await page.getByLabel("Password").fill("Demo@1234");
  await page.getByRole("button", { name: /Sign in/i }).click();
  await expect(page.getByRole("heading", { name: /Platform overview/i })).toBeVisible({ timeout: 20_000 });
}

/** Navigate within the console; re-authenticates once if the session was lost. */
async function adminGoTo(page, path: string) {
  await page.goto(`${BASE}${path}`);
  await page.waitForTimeout(300);
  if (page.url().includes("/login")) {
    await loginAdmin(page);
    await page.goto(`${BASE}${path}`);
  }
  await expect(page.getByRole("heading", { name: /Platform staff sign-in/i })).not.toBeVisible({
    timeout: 20_000,
  });
}

test.describe.serial("admin console", () => {
  test("admin signs in and sees platform metrics", async ({ page }) => {
    await loginAdmin(page);
    await expect(page.getByText(/Gross order value/i).first()).toBeVisible({ timeout: 20_000 });
    await expect(page.getByText(/Order status distribution/i)).toBeVisible({ timeout: 20_000 });
  });

  test("restaurant applications can be reviewed", async ({ page }) => {
    await adminGoTo(page, "/restaurants");
    await expect(page.getByRole("heading", { name: /Restaurants & applications/i })).toBeVisible({ timeout: 20_000 });
    const review = page.getByRole("button", { name: /Review…/i }).first();
    const has = await review.isVisible().catch(() => false);
    test.skip(!has, "no pending applications right now");
    await review.click();
    await page.getByLabel("Review note").fill("E2E review: documents verified (demo simulation).");
    await page.getByRole("button", { name: /Submit review/i }).click();
    await expect(page.getByText(/demo simulation/i)).toBeVisible({ timeout: 20_000 });
  });

  test("order search works and audit log renders", async ({ page }) => {
    await adminGoTo(page, "/orders");
    await expect(page.getByRole("heading", { name: /^Orders/ })).toBeVisible({ timeout: 20_000 });
    await page.getByLabel("Search orders").fill("BO-");
    await page.getByRole("button", { name: "Search" }).click();
    await expect(page.locator("table tbody tr").first()).toBeVisible({ timeout: 20_000 });

    await adminGoTo(page, "/audit");
    await expect(page.getByRole("heading", { name: /Audit log/i })).toBeVisible();
    await expect(page.locator("ul li").first()).toBeVisible({ timeout: 20_000 });
  });

  test("support queue opens a thread", async ({ page }) => {
    await adminGoTo(page, "/support");
    await expect(page.getByRole("heading", { name: /Support queue/i })).toBeVisible({ timeout: 20_000 });
    const first = page.locator("ul li button").first();
    const has = await first.isVisible().catch(() => false);
    test.skip(!has, "no tickets in the queue");
    await first.click();
    await expect(page.getByText(/INTERNAL — never shown to customer/i).first()).toBeVisible({
      timeout: 20_000,
    });
  });
});
