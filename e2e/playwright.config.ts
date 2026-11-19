import { defineConfig, devices } from "@playwright/test";

/**
 * E2E tests run against the locally running stack:
 *   Go API :8080, customer :3000, restaurant :3001, courier :3002, admin :3003
 * Start everything with `pnpm dev` (or the individual apps) plus migrate+seed.
 */
const BASE = process.env.E2E_CUSTOMER_URL ?? "http://localhost:3000";

export default defineConfig({
  testDir: "./tests",
  timeout: 45_000,
  expect: { timeout: 10_000 },
  fullyParallel: false, // sequential: the suite shares one seeded database
  retries: 0,
  workers: 1,
  reporter: [["list"]],
  use: {
    baseURL: BASE,
    trace: "retain-on-failure",
    screenshot: "off",
    locale: "en-IN",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"], viewport: { width: 1366, height: 900 } },
    },
  ],
});
