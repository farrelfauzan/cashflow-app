import { expect, test, type Page, type TestInfo } from "@playwright/test";

import { ORG_HOME_URL, openMenu } from "./support/shell";
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

// Next.js reports a failed check shortly after the screen renders, so wait for the page to
// settle before reading the errors.
async function settle(page: Page) {
  await page.waitForLoadState("networkidle");
  await page.waitForTimeout(1_000);
}

async function expectHeading(page: Page, name: string) {
  await expect(
    page.getByRole("heading", { level: 1, name, exact: true }),
  ).toBeVisible();
  await settle(page);
}

async function clickMenu(page: Page, testInfo: TestInfo, label: string) {
  const menu = await openMenu(page, testInfo);
  await menu.getByRole("link", { name: label, exact: true }).click();
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
  await expectHeading(page, "Beranda");

  // Full page loads: each route rendered from the root.
  const screens: [string, string][] = [
    ["properties", "Properti"],
    ["tenants", "Penyewa"],
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

test("every client navigation inside an organization shows no Next.js error", async ({
  page,
}, testInfo) => {
  const errors = collectErrors(page);
  await openGriya(page);
  await expectHeading(page, "Beranda");

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
  await expectHeading(page, "Beranda");

  expect(errors).toEqual([]);
});
