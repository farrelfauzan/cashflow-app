import {
  expect,
  test,
  type Page,
  type Request,
  type TestInfo,
} from "@playwright/test";

import type { InvoiceList, PaymentLink } from "../src/lib/api/generated/model";
import { HOME_HEADING, ORG_HOME_URL, openSection } from "./support/shell";
import { HENDRA } from "./support/staff";

// Runs only against `next dev` (E2E_SERVER=dev, see playwright.config.ts). Next.js checks in
// development that every page load and every client navigation can show something at once: data
// read on the server outside a <Suspense> boundary is reported as a console error ("Next.js
// encountered uncached data during prerendering or a navigation") and in the dev overlay. A
// production build never reports it, so the main e2e run cannot catch it. This spec opens every
// staff screen both ways and fails on any console error.

const GRIYA = "Griya Sejahtera";
// Seeded in Griya Sejahtera (services/api/internal/demoseed).
const PROPERTY = "Kos Melati";
const TENANT = "Rina Wulandari";

test.use({ storageState: HENDRA.storageState });

// Every console error and uncaught page error, as text.
function collectErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  page.on("pageerror", (error) => errors.push(error.message));
  return errors;
}

// `next dev` compiles a route the first time it is asked for, which on a CI runner can take far
// longer than a page load, so the tests and the first sight of a heading get room for it.
test.describe.configure({ timeout: 180_000 });
const COLD_COMPILE_MS = 60_000;

// The requests each page is still waiting for: data, RSC payloads and JS chunks. The dev server's
// own traffic (HMR, the dev overlay) is left out, because it never settles, which is why
// Playwright's "networkidle" made this spec flaky.
const DEV_SERVER_PATH = /^\/(?:_next\/webpack-hmr|__nextjs)/;
const inFlightByPage = new WeakMap<Page, Set<Request>>();

function trackInFlight(page: Page) {
  const inFlight = new Set<Request>();
  inFlightByPage.set(page, inFlight);
  page.on("request", (request) => {
    if (!DEV_SERVER_PATH.test(new URL(request.url()).pathname)) {
      inFlight.add(request);
    }
  });
  const done = (request: Request) => inFlight.delete(request);
  page.on("requestfinished", done);
  page.on("requestfailed", done);
}

test.beforeEach(({ page }) => {
  trackInFlight(page);
});

// The page is settled once none of its requests has been open for QUIET_MS.
const QUIET_MS = 500;
const SETTLE_MAX_MS = 30_000;
// Next.js reports a failed check shortly after the screen renders.
const LATE_REPORT_MS = 1_000;

// Waits for the screen's own requests to finish, then for Next.js's late report, before the
// errors are read. A request still open after SETTLE_MAX_MS does not fail the test (this spec
// checks for errors, not speed); it is recorded on the report and as a warning in the CI log, so a slow request stays visible.
async function settle(page: Page) {
  const inFlight = inFlightByPage.get(page);
  if (inFlight === undefined) throw new Error("requests are not tracked");
  const deadline = Date.now() + SETTLE_MAX_MS;
  let quietSince = Date.now();
  while (Date.now() - quietSince < QUIET_MS) {
    if (Date.now() > deadline) {
      const urls = [...inFlight].map((r) => r.url()).join(", ");
      test.info().annotations.push({
        type: "still loading after 30 s",
        description: urls,
      });
      // A workflow command, so a green CI run shows it too (the report is uploaded only on a
      // failure). Playwright passes a test's stdout through, the github reporter included.
      console.log(
        `::warning title=dev-smoke::${test.info().title}: still loading after 30 s: ${urls}`,
      );
      break;
    }
    if (inFlight.size > 0) quietSince = Date.now();
    await page.waitForTimeout(100);
  }
  await page.waitForTimeout(LATE_REPORT_MS);
}

async function expectHeading(page: Page, name: string | RegExp) {
  await expect(
    page.getByRole("heading", { level: 1, name, exact: true }),
  ).toBeVisible({ timeout: COLD_COMPILE_MS });
  await settle(page);
}

async function clickMenu(page: Page, testInfo: TestInfo, label: string) {
  await openSection(page, testInfo, label);
}

// Opens a tenant from the list on screen (the table on iPad, the cards on the phone).
async function openTenant(page: Page) {
  await page
    .getByRole("list", { name: "Daftar penyewa" })
    .filter({ visible: true })
    .getByRole("listitem")
    .filter({ hasText: TENANT })
    .getByRole("link")
    .first()
    .click();
}

async function openGriya(page: Page): Promise<string> {
  await page.goto("/organizations");
  await page
    .getByRole("list", { name: "Daftar usaha" })
    .getByRole("link", { name: GRIYA })
    .click();
  await expect(page).toHaveURL(ORG_HOME_URL);
  return page.url();
}

test("every staff screen loads with no Next.js error", async ({ page }) => {
  const errors = collectErrors(page);
  const home = await openGriya(page);
  await expectHeading(page, HOME_HEADING);

  // Full page loads: each route rendered from the root.
  const screens: [string, string][] = [
    ["properties", "Properti"],
    ["tenants", "Penyewa"],
    ["billing", "Penagihan"],
    ["members", "Anggota tim"],
    ["tenants/import", "Impor penyewa"],
  ];
  for (const [segment, heading] of screens) {
    await page.goto(`${home}/${segment}`);
    await expectHeading(page, heading);
  }
  await page.goto(`${home}/properties`);
  await page.getByRole("link", { name: PROPERTY }).first().click();
  await expectHeading(page, PROPERTY);
  await page.reload();
  await expectHeading(page, PROPERTY);

  await page.goto(`${home}/tenants`);
  await openTenant(page);
  await expectHeading(page, TENANT);
  await page.reload();
  await expectHeading(page, TENANT);

  expect(errors).toEqual([]);
});

// The shell's getOrgMe, which starts as soon as the shell hydrates.
const ORG_ME_PATH = /^\/api\/orgs\/[^/]+\/me$/;
const SHELL_MODULE = "client/shell/org-ability-provider.tsx";

// Holds back every JS chunk that carries `viewModule` (and not the shell) until the shell's
// getOrgMe response has reached the browser, so the view hydrates with getOrgMe already in the
// shared query cache. On a fast machine this order is rare; here it happens every time. Returns
// how many chunks it held back, so a test can check the race really took place. Development
// chunks name the modules they carry, which is how a chunk is matched. (A failed getOrgMe is
// not a risk: a view mounting on a query with an error and no data reads it as pending.)
async function hydrateViewAfterOrgMe(
  page: Page,
  viewModule: string,
): Promise<() => number> {
  let held = 0;
  const orgMeLoaded = page
    .waitForResponse((r) => ORG_ME_PATH.test(new URL(r.url()).pathname), {
      timeout: 30_000,
    })
    .then((r) => r.finished());
  await page.route("**/_next/static/chunks/**", async (route) => {
    const response = await route.fetch();
    const body = await response.text();
    if (body.includes(viewModule) && !body.includes(SHELL_MODULE)) {
      held += 1;
      await orgMeLoaded;
      // Let TanStack Query write the response to the cache before the chunk arrives.
      await new Promise((resolve) => setTimeout(resolve, 300));
    }
    await route.fulfill({ response, body });
  });
  return () => held;
}

type RaceScreen = {
  name: string;
  viewModule: string;
  // The screen's URL, from the organization's home.
  url: (page: Page, home: string) => Promise<string>;
  // What the screen shows once getOrgMe and its list have loaded.
  heading: string;
};

// The views that read getOrgMe. Only the properties view ever showed something from getOrgMe
// alone; the others wait for their own list, so they guard against that coming back.
const listScreen = (segment: string) => async (_page: Page, home: string) =>
  `${home}/${segment}`;
const RACE_SCREENS: RaceScreen[] = [
  {
    name: "properties",
    viewModule: "client/property/properties-view.tsx",
    url: listScreen("properties"),
    heading: "Properti",
  },
  {
    name: "members",
    viewModule: "client/member/members-view.tsx",
    url: listScreen("members"),
    heading: "Anggota tim",
  },
  {
    name: "tenants",
    viewModule: "client/tenant/tenants-view.tsx",
    url: listScreen("tenants"),
    heading: "Penyewa",
  },
  {
    name: "property units",
    viewModule: "client/property/property-units-view.tsx",
    url: async (page, home) => {
      await page.goto(`${home}/properties`);
      await page.getByRole("link", { name: PROPERTY }).first().click();
      await expectHeading(page, PROPERTY);
      return page.url();
    },
    heading: PROPERTY,
  },
];

// A view whose JS arrives after the shell has loaded getOrgMe must still hydrate to the markup
// the server rendered (the loading state), then show what it always showed.
for (const screen of RACE_SCREENS) {
  test(`the ${screen.name} screen hydrates when its view loads after getOrgMe`, async ({
    page,
  }) => {
    const home = await openGriya(page);
    await expectHeading(page, HOME_HEADING);
    const url = await screen.url(page, home);

    const errors = collectErrors(page);
    const held = await hydrateViewAfterOrgMe(page, screen.viewModule);
    await page.goto(url);
    await expectHeading(page, screen.heading);

    expect(held()).toBeGreaterThan(0);
    expect(errors).toEqual([]);
  });
}

test("every client navigation inside an organization shows no Next.js error", async ({
  page,
}, testInfo) => {
  const errors = collectErrors(page);
  await openGriya(page);
  await expectHeading(page, HOME_HEADING);

  // The shell stays on screen; only the page below it changes.
  await clickMenu(page, testInfo, "Properti");
  await expectHeading(page, "Properti");
  await page.getByRole("link", { name: PROPERTY }).first().click();
  await expectHeading(page, PROPERTY);

  await clickMenu(page, testInfo, "Penyewa");
  await expectHeading(page, "Penyewa");
  await openTenant(page);
  await expectHeading(page, TENANT);

  await clickMenu(page, testInfo, "Penyewa");
  await expectHeading(page, "Penyewa");
  await page.getByRole("link", { name: "Impor dari spreadsheet" }).click();
  await expectHeading(page, "Impor penyewa");

  await clickMenu(page, testInfo, "Anggota tim");
  await expectHeading(page, "Anggota tim");

  await clickMenu(page, testInfo, "Beranda");
  await expectHeading(page, HOME_HEADING);

  expect(errors).toEqual([]);
});

// The tenant payment page reads its params in a client component under <Suspense>; a mistake
// there shows only in development. A real link, so the page's one API read answers 200 (a 404
// would log "Failed to load resource" as a console error).
test("the tenant payment page loads with no Next.js error", async ({
  page,
  browser,
}) => {
  const home = await openGriya(page);
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(home)?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  const unpaid = await page.request.get(`/api/orgs/${orgId}/invoices`, {
    params: { status: "unpaid", pageSize: 1 },
  });
  expect(unpaid.status()).toBe(200);
  const invoice = ((await unpaid.json()) as InvoiceList).items[0];
  if (invoice === undefined) throw new Error("seed-demo has no unpaid invoice");
  const made = await page.request.post(
    `/api/orgs/${orgId}/invoices/${invoice.id}/payment-links`,
  );
  expect(made.status()).toBe(201);
  const { url } = (await made.json()) as PaymentLink;
  const path = new URL(url, "http://localhost").pathname;

  // The tenant has no session.
  const context = await browser.newContext({
    storageState: { cookies: [], origins: [] },
  });
  const tenantPage = await context.newPage();
  trackInFlight(tenantPage);
  const errors = collectErrors(tenantPage);

  await tenantPage.goto(path);
  await expectHeading(tenantPage, /^Bayar tagihan /);
  await tenantPage.reload();
  await expectHeading(tenantPage, /^Bayar tagihan /);
  await context.close();

  expect(errors).toEqual([]);
});
