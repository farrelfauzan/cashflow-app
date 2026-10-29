import { defineConfig, devices } from "@playwright/test";

// End-to-end runs need the Go API (migrated, seeded with seed-demo, WHATSAPP_PROVIDER=devlog
// and DEVLOG_FILE set); see "End-to-end tests" in README.md. All settings come from the
// environment so a run can stay off ports other work uses:
// - E2E_PORT: the app's port (default 3000). Its origin must be in the API's ALLOWED_ORIGINS.
// - API_DEV_ORIGIN: the Go API (default http://localhost:8080). The /api rewrite in a
//   production build exists only when this is set at build time (next.config.ts).
// - API_INTERNAL_URL: where server components reach the API (default API_DEV_ORIGIN).
// - DEVLOG_FILE: the API's devlog file; the login helper reads codes from it.
// - E2E_REUSE_SERVER=1: reuse a server already on E2E_PORT instead of failing. Off by default
//   so an unrelated server (or one built without the rewrite) is never tested by accident.
// - E2E_SERVER=dev: run `next dev` instead of a production build, for e2e/dev-smoke.spec.ts.
//   Next.js checks instant navigation only in development, and reports a failure in the dev
//   overlay, so only a dev server shows it.
const port = Number(process.env.E2E_PORT ?? 3000);
const baseURL = `http://localhost:${port}`;
const apiOrigin = process.env.API_DEV_ORIGIN?.trim() || "http://localhost:8080";
const apiInternalUrl = process.env.API_INTERNAL_URL?.trim() || apiOrigin;
const isCI = Boolean(process.env.CI);
const devServer = process.env.E2E_SERVER === "dev";
// One id per run (minutes since the epoch, four digits), set here once and inherited by the
// workers through the environment. Records a test leaves in the database, such as invited
// members, use it, so a second run on the same local database does not collide with the first.
process.env.E2E_RUN_ID ??= String(
  Math.floor(Date.now() / 60_000) % 10_000,
).padStart(4, "0");

// Chromium only, at the two target sizes from CLAUDE.md "UX rules".
export default defineConfig({
  testDir: "./e2e",
  // The dev smoke spec runs only against `next dev`, and only it does.
  ...(devServer
    ? { testMatch: /(dev-smoke\.spec|\.setup)\.ts$/ }
    : { testIgnore: /dev-smoke\.spec\.ts$/ }),
  fullyParallel: true,
  forbidOnly: isCI,
  retries: isCI ? 2 : 0,
  workers: isCI ? 1 : undefined,
  reporter: [[isCI ? "github" : "list"], ["html", { open: "never" }]],
  use: {
    baseURL,
    trace: "on-first-retry",
  },
  projects: [
    // Signs in once per run through the real login code flow and saves the session for both
    // size projects. No retries: each attempt spends one of the phone's 3 code requests per
    // 15 minutes, and a failed login is not flaky.
    {
      name: "setup",
      testMatch: /.*\.setup\.ts/,
      retries: 0,
    },
    {
      name: "ipad",
      dependencies: ["setup"],
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 1180, height: 820 },
        hasTouch: true,
      },
    },
    {
      name: "phone",
      dependencies: ["setup"],
      use: {
        ...devices["Pixel 7"],
        viewport: { width: 390, height: 844 },
        isMobile: true,
        hasTouch: true,
      },
    },
  ],
  // A production build, as CI and users get it, with the /api rewrite to the API because there
  // is no Caddy in front of it here.
  webServer: {
    command: devServer
      ? `pnpm exec next dev --port ${port}`
      : "pnpm build && pnpm start",
    url: baseURL,
    reuseExistingServer: process.env.E2E_REUSE_SERVER === "1",
    timeout: 180_000,
    env: {
      PORT: String(port),
      API_DEV_ORIGIN: apiOrigin,
      API_INTERNAL_URL: apiInternalUrl,
    },
  },
});
