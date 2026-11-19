import { expect, test } from "@playwright/test";
import { mkdirSync } from "node:fs";
import path from "node:path";

/**
 * Genuine application screenshots for the README. Captured from the running
 * stack with the seeded demo dataset; no mockups or composites.
 * Run: pnpm --filter @biteorbit/e2e screenshots
 */
const OUT = path.resolve(__dirname, "..", "..", "docs", "screenshots");
mkdirSync(OUT, { recursive: true });

const DEMO_CUSTOMER = { email: "customer@demo.biteorbit.local", password: "Demo@1234" };

async function login(page, email: string, password: string, origin: string) {
  // the customer app signs in via /account; portals have /login
  const loginPath = origin === "" ? "/account" : "/login";
  await page.goto(`${origin}${loginPath}`);
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: /Sign in/i }).click();
  await page.waitForTimeout(1200);
}

test("customer home desktop + mobile", async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto("/");
  await page.waitForSelector("a[href^='/restaurants/']", { timeout: 20_000 });
  await page.screenshot({ path: path.join(OUT, "customer-home-desktop.png"), fullPage: false });

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  await page.waitForTimeout(800);
  await page.screenshot({ path: path.join(OUT, "customer-home-mobile.png"), fullPage: false });
});

test("restaurant detail and menu", async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto("/?query=biryani");
  await page.waitForSelector("a[href^='/restaurants/']", { timeout: 20_000 });
  await page.locator("a[href^='/restaurants/']").first().click();
  await page.waitForSelector("button:has-text('ADD')", { timeout: 20_000 });
  await page.screenshot({ path: path.join(OUT, "restaurant-detail-menu.png"), fullPage: false });
});

test("item customisation sheet", async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto("/");
  await page.waitForSelector("a[href^='/restaurants/']", { timeout: 20_000 });
  await page.locator("a[href^='/restaurants/']").first().click();
  await page.waitForSelector("button:has-text('ADD')", { timeout: 20_000 });
  await page.locator("button:has-text('ADD')").first().click();
  await page.waitForSelector("[role='dialog']", { timeout: 10_000 });
  await page.screenshot({ path: path.join(OUT, "item-customisation.png") });
});

test("cart with breakdown", async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await login(page, DEMO_CUSTOMER.email, DEMO_CUSTOMER.password, "");
  // build a cart if empty: add an item
  await page.goto("/");
  await page.waitForSelector("a[href^='/restaurants/']", { timeout: 20_000 });
  await page.locator("a[href^='/restaurants/']").first().click();
  await page.waitForSelector("button:has-text('ADD')", { timeout: 20_000 });
  await page.locator("button:has-text('ADD')").first().click();
  await page.waitForSelector("[role='dialog']");
  await page.getByRole("button", { name: /Add ·/ }).click();
  await page.waitForTimeout(800);
  await page.goto("/cart");
  await page.waitForTimeout(800);
  await page.screenshot({ path: path.join(OUT, "cart.png"), fullPage: false });
});

test("checkout with price breakdown", async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await login(page, DEMO_CUSTOMER.email, DEMO_CUSTOMER.password, "");
  await page.goto("/cart");
  const hasItems = await page.getByRole("button", { name: /Proceed to checkout/i }).isVisible().catch(() => false);
  test.skip(!hasItems, "cart empty — run the cart screenshot first");
  await page.getByRole("button", { name: /Proceed to checkout/i }).click();
  await page.waitForSelector("text=Order summary", { timeout: 20_000 });
  await page.waitForTimeout(800);
  await page.screenshot({ path: path.join(OUT, "checkout-price-breakdown.png"), fullPage: true });
});

test("order tracking", async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await login(page, DEMO_CUSTOMER.email, DEMO_CUSTOMER.password, "");
  await page.goto("/orders");
  await page.waitForSelector("a[href^='/orders/']", { timeout: 20_000 });
  await page.locator("a[href^='/orders/']").first().click();
  await expect(page.getByText(/Live status|Order summary/).first()).toBeVisible({ timeout: 20_000 });
  await page.waitForTimeout(600);
  await page.screenshot({ path: path.join(OUT, "order-tracking.png"), fullPage: false });
});

test("customer order history", async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await login(page, DEMO_CUSTOMER.email, DEMO_CUSTOMER.password, "");
  await page.goto("/orders");
  await page.waitForSelector("a[href^='/orders/']", { timeout: 20_000 });
  await page.screenshot({ path: path.join(OUT, "customer-order-history.png"), fullPage: false });
});

test("restaurant dashboard", async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await login(page, "manager01@demo.biteorbit.local", "Demo@1234", "http://localhost:3001");
  await page.goto("http://localhost:3001/dashboard");
  await page.waitForSelector("text=Gross order value", { timeout: 20_000 });
  await page.screenshot({ path: path.join(OUT, "restaurant-dashboard.png"), fullPage: false });
});

test("restaurant order management", async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await login(page, "manager01@demo.biteorbit.local", "Demo@1234", "http://localhost:3001");
  await page.goto("http://localhost:3001/orders");
  await page.waitForSelector("text=Order queue", { timeout: 20_000 });
  await page.waitForTimeout(600);
  await page.screenshot({ path: path.join(OUT, "restaurant-order-management.png"), fullPage: false });
});

test("restaurant menu management", async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await login(page, "manager01@demo.biteorbit.local", "Demo@1234", "http://localhost:3001");
  await page.goto("http://localhost:3001/menu");
  await page.waitForSelector("text=Menu management", { timeout: 20_000 });
  await page.waitForTimeout(600);
  await page.screenshot({ path: path.join(OUT, "restaurant-menu-management.png"), fullPage: false });
});

test("courier dashboard + active delivery", async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await login(page, "rider@demo.biteorbit.local", "Demo@1234", "http://localhost:3002");
  await page.goto("http://localhost:3002/dashboard");
  await page.waitForSelector("text=Today", { timeout: 20_000 });
  await page.screenshot({ path: path.join(OUT, "courier-dashboard.png"), fullPage: false });
});

test("admin overview + order management + restaurant review", async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await login(page, "admin@demo.biteorbit.local", "Demo@1234", "http://localhost:3003");
  await page.goto("http://localhost:3003/overview");
  await page.waitForSelector("text=Platform overview", { timeout: 20_000 });
  await page.waitForTimeout(600);
  await page.screenshot({ path: path.join(OUT, "admin-overview.png"), fullPage: false });

  await page.goto("http://localhost:3003/orders");
  await page.waitForSelector("table", { timeout: 20_000 });
  await page.screenshot({ path: path.join(OUT, "admin-order-management.png"), fullPage: false });

  await page.goto("http://localhost:3003/restaurants");
  await page.waitForSelector("text=Restaurants & applications", { timeout: 20_000 });
  await page.waitForTimeout(600);
  await page.screenshot({ path: path.join(OUT, "admin-restaurant-review.png"), fullPage: false });
});

test("support ticket views", async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 900 });
  await login(page, DEMO_CUSTOMER.email, DEMO_CUSTOMER.password, "");
  await page.goto("/support");
  await page.waitForTimeout(800);
  await page.screenshot({ path: path.join(OUT, "support-ticket.png"), fullPage: false });
});
