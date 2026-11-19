import { expect, test } from "@playwright/test";

/**
 * Customer journey against the seeded demo dataset.
 * Anonymous discovery tests run without a session; the ordering journey is a
 * single test because each Playwright test gets a fresh cookie context.
 */

const DEMO = { email: "customer@demo.biteorbit.local", password: "Demo@1234" };
const FRESH = { email: `e2e-journey-${Date.now()}@example.test`, password: "Passw0rd123!" };

test.describe("discovery (anonymous)", () => {
  test("home shows seeded demo restaurants", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { name: /Great food/i })).toBeVisible();
    await expect(page.getByText(/Bitepur/i).first()).toBeVisible();
    const card = page
      .getByRole("link", { name: /Biryani Bandit|Tandoor Tales|Dosa Diner/ })
      .first();
    await expect(card).toBeVisible({ timeout: 15_000 });
  });

  test("search finds restaurants and dishes", async ({ page }) => {
    await page.goto("/");
    const search = page.getByLabel("Search food and restaurants");
    await search.fill("biryani");
    await expect(page.getByRole("listbox")).toBeVisible({ timeout: 10_000 });
    await search.press("Enter");
    await expect(page.getByText(/Results for/i)).toBeVisible({ timeout: 10_000 });
    await expect(page.locator("a[href^='/restaurants/']").first()).toBeVisible();
  });

  test("filters narrow the listing", async ({ page }) => {
    await page.goto("/?cuisine=pizza");
    await expect(page.locator("a[href^='/restaurants/']").first()).toBeVisible({ timeout: 10_000 });
  });

  test("restaurant menu renders with categories and ADD buttons", async ({ page }) => {
    await page.goto("/");
    await page.locator("a[href^='/restaurants/']").first().click();
    await expect(page.getByRole("heading", { name: /^Menu/ })).toBeVisible({ timeout: 15_000 });
    await expect(page.getByRole("button", { name: "ADD", exact: true }).first()).toBeVisible({
      timeout: 15_000,
    });
  });
});

test.describe("ordering journey (signed in)", () => {
  test("login → customise → cart → coupon → checkout → COD → tracking", async ({ page }) => {
    // register a fresh customer (clean coupon redemption state)
    await page.goto("/account");
    await page.getByRole("button", { name: /Need an account/i }).click();
    await page.getByLabel("Full name").fill("Journey Tester");
    await page.getByLabel("Email").fill(FRESH.email);
    await page.getByLabel("Password").fill(FRESH.password);
    await page.getByRole("button", { name: /Create account/i }).click();
    await expect(page.getByText(/Journey Tester/).first()).toBeVisible({ timeout: 15_000 });

    // add an address (required for checkout)
    await page.getByRole("button", { name: "Add address" }).first().click();
    await page.getByLabel("Recipient name").fill("Journey Tester");
    await page.getByLabel("Phone").fill("9876543210");
    await page.getByLabel("Address line 1").fill("9 Journey Lane");
    await page.getByLabel("Locality").fill("Rosewood Hills");
    await page.getByLabel("Pincode").fill("560001");
    await page.getByRole("button", { name: /Save address/i }).click();
    await expect(page.getByText(/9 Journey Lane/)).toBeVisible({ timeout: 15_000 });

    // pick a restaurant and add an item
    await page.goto("/");
    await page.waitForSelector("a[href^='/restaurants/']", { timeout: 20_000 });
    await page.locator("a[href^='/restaurants/']").first().click();
    const addBtns = page.getByRole("button", { name: "ADD", exact: true });
    await addBtns.first().waitFor({ state: "visible", timeout: 20_000 });
    await addBtns.first().click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await page.getByRole("button", { name: "Increase quantity" }).click();
    await page.getByRole("button", { name: /Add ·/ }).click();
    await expect(page.getByLabel(/Cart, 2 items/i)).toBeVisible({ timeout: 15_000 });

    // cart shows the line and subtotal
    await page.goto("/cart");
    await expect(page.getByText(/Item subtotal/i)).toBeVisible({ timeout: 15_000 });
    await page.getByRole("button", { name: /Proceed to checkout/i }).click();

    // checkout: address, coupon, tip, COD
    await expect(page.getByRole("heading", { name: /Checkout/i })).toBeVisible({ timeout: 20_000 });
    const addressSelect = page.getByLabel("Choose an address");
    await addressSelect.waitFor({ state: "visible", timeout: 20_000 });
    await addressSelect.selectOption({ index: 0 });
    await page.getByLabel("Coupon code").fill("NEWBITE");
    await page.getByRole("button", { name: "Apply" }).click();
    await expect(page.getByText(/applied/i).first()).toBeVisible({ timeout: 15_000 });
    await page.getByRole("button", { name: "₹30", exact: true }).click();
    await page.getByText("Cash on delivery").click();

    const place = page.getByRole("button", { name: /Place order ·/ });
    await expect(place).toBeVisible({ timeout: 20_000 });
    await place.click();
    await expect(page.getByRole("heading", { name: /Order placed!/i })).toBeVisible({ timeout: 25_000 });
    await expect(page.getByText(/BO-/)).toBeVisible();

    // tracking: order detail with timeline
    await page.goto("/orders");
    await expect(page.getByRole("heading", { name: /Your orders/i })).toBeVisible({ timeout: 15_000 });
    await page.locator("a[href^='/orders/']").first().click();
    await expect(page.getByText(/Price snapshot/i)).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText(/Live status|Order summary/)).toBeVisible();
  });
});
