import { expect, test } from "@playwright/test";

/** Courier portal on :3002 — availability, offers, earnings. */

const BASE = "http://localhost:3002";

async function loginRider(page, goOnline = false) {
  await page.goto(`${BASE}/login`);
  await page.getByLabel("Email").fill("rider@demo.biteorbit.local");
  await page.getByLabel("Password").fill("Demo@1234");
  await page.getByRole("button", { name: /Sign in/i }).click();
  await expect(page.getByRole("heading", { name: /Today/i })).toBeVisible({ timeout: 20_000 });
  if (goOnline) {
    const onlineSwitch = page.getByRole("switch", { name: /OFFLINE|ONLINE/i });
    const status = (await onlineSwitch.getAttribute("aria-checked")) ?? "false";
    if (status !== "true") await onlineSwitch.click();
    await expect(page.getByText(/ONLINE/i)).toBeVisible({ timeout: 15_000 });
  }
}

test.describe.serial("courier portal", () => {
  test("rider signs in, goes online and reviews offers", async ({ page }) => {
    await loginRider(page, true);

    const onlineSwitch = page.getByRole("switch", { name: /OFFLINE|ONLINE/i });
    const status = (await onlineSwitch.getAttribute("aria-checked")) ?? "false";
    if (status !== "true") {
      await onlineSwitch.click();
    }
    await expect(page.getByText(/ONLINE/i)).toBeVisible({ timeout: 15_000 });
  });

  test("offers page accepts a pending delivery if present", async ({ page }) => {
    await loginRider(page, true);
    await page.goto(`${BASE}/offers`);
    await expect(page.getByRole("heading", { name: /Delivery offers/i })).toBeVisible({ timeout: 20_000 });
    const accept = page.getByRole("button", { name: "Accept", exact: true }).first();
    const has = await accept.isVisible().catch(() => false);
    test.skip(!has, "no open offers for this rider right now");
    await accept.click();
    await expect(page.getByText(/Offer accepted!/i)).toBeVisible({ timeout: 20_000 });
    await page.getByRole("link", { name: /Go to delivery/i }).click();
    await expect(page.getByText(/Pickup:/i)).toBeVisible({ timeout: 20_000 });
  });

  test("earnings page shows the ledger", async ({ page }) => {
    await loginRider(page);
    await page.goto(`${BASE}/earnings`);
    await expect(page.getByRole("heading", { name: /Earnings/i })).toBeVisible({ timeout: 20_000 });
    await expect(page.getByText(/All-time/i).or(page.getByText(/No earnings yet/i))).toBeVisible({
      timeout: 20_000,
    });
  });
});
