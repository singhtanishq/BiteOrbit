import { expect, test } from "@playwright/test";

/** Restaurant portal on :3001 — queue management and menu edits. */

const BASE = "http://localhost:3001";

async function loginStaff(page) {
  await page.goto(`${BASE}/login`);
  await page.getByLabel("Email").fill("manager01@demo.biteorbit.local");
  await page.getByLabel("Password").fill("Demo@1234");
  await page.getByRole("button", { name: /Sign in/i }).click();
  await expect(page.getByRole("heading", { name: /dashboard/i })).toBeVisible({ timeout: 20_000 });
}

test.describe.serial("restaurant portal", () => {
  test("staff signs in and sees the dashboard", async ({ page }) => {
    await loginStaff(page);
    await expect(page.getByText(/Gross order value/i).first()).toBeVisible({ timeout: 20_000 });
  });

  test("order queue accepts and prepares a placed order", async ({ page }) => {
    await loginStaff(page);
    await page.goto(`${BASE}/orders`);
    await expect(page.getByRole("heading", { name: /Order queue/i })).toBeVisible({ timeout: 20_000 });

    const acceptBtn = page.getByRole("button", { name: /Accept order/i }).first();
    const hasNew = await acceptBtn.isVisible().catch(() => false);
    test.skip(!hasNew, "no placed orders in the queue right now");

    await acceptBtn.click();
    const prepare = page.getByRole("button", { name: /Start preparing/i }).first();
    await expect(prepare).toBeVisible({ timeout: 20_000 });
    await prepare.click();
    await expect(page.getByRole("button", { name: /Mark ready for pickup/i }).first()).toBeVisible({
      timeout: 20_000,
    });
  });

  test("menu management toggles availability", async ({ page }) => {
    await loginStaff(page);
    await page.goto(`${BASE}/menu`);
    await expect(page.getByRole("heading", { name: /Menu management/i })).toBeVisible({ timeout: 20_000 });
    const switches = page.getByRole("switch");
    await switches.first().waitFor({ state: "visible", timeout: 20_000 });
    const first = switches.first();
    const before = (await first.getAttribute("aria-checked")) ?? "true";
    await first.click();
    await expect(first).toHaveAttribute("aria-checked", before === "true" ? "false" : "true", {
      timeout: 15_000,
    });
    // restore
    await first.click();
    await expect(first).toHaveAttribute("aria-checked", before, { timeout: 15_000 });
  });

  test("kitchen ticket renders for an in-flight order", async ({ page }) => {
    await loginStaff(page);
    await page.goto(`${BASE}/orders`);
    const ticket = page.getByRole("button", { name: /Kitchen ticket/i }).first();
    const has = await ticket.isVisible().catch(() => false);
    test.skip(!has, "no in-flight orders right now");
    await ticket.click();
    await expect(page.getByText(/KITCHEN TICKET/)).toBeVisible();
    await page.getByRole("button", { name: /Close/i }).click();
  });
});
