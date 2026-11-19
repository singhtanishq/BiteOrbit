import { expect, test } from "@playwright/test";

/** Registration, address management, logout and sign-in in one session. */

const EMAIL = `e2e-customer-${Date.now()}@example.test`;

test("register, add address, logout, sign back in", async ({ page }) => {
  await page.goto("/account");
  await page.getByRole("button", { name: /Need an account/i }).click();
  await page.getByLabel("Full name").fill("E2E Tester");
  await page.getByLabel("Email").fill(EMAIL);
  await page.getByLabel("Password").fill("Passw0rd123!");
  await page.getByRole("button", { name: /Create account/i }).click();
  await expect(page.getByText(/E2E Tester/).first()).toBeVisible({ timeout: 15_000 });

  // add an address in a demo locality
  await page.getByRole("button", { name: "Add address" }).first().click();
  await page.getByLabel("Recipient name").fill("E2E Tester");
  await page.getByLabel("Phone").fill("9876543210");
  await page.getByLabel("Address line 1").fill("7 Testers Way");
  await page.getByLabel("Locality").fill("Rosewood Hills");
  await page.getByLabel("Pincode").fill("560001");
  await page.getByRole("button", { name: /Save address/i }).click();
  await expect(page.getByText(/7 Testers Way/)).toBeVisible({ timeout: 15_000 });

  // logout kills the session
  await page.getByRole("button", { name: /Sign out/i }).click();
  await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeVisible({ timeout: 15_000 });

  // sign back in
  await page.getByLabel("Email").fill(EMAIL);
  await page.getByLabel("Password").fill("Passw0rd123!");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page.getByText(/E2E Tester/).first()).toBeVisible({ timeout: 15_000 });

  // the seeded demo shortcut also works
  await page.getByRole("button", { name: /Sign out/i }).click();
  await page.getByRole("button", { name: /Fill demo credentials/i }).click();
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page.getByText(/Ishaan Verma/).first()).toBeVisible({ timeout: 15_000 });
});
