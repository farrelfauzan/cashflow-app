import {
  expect,
  test,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import type {
  Property,
  PropertyKind,
  Tenant,
  Unit,
  UnitTermInput,
} from "../src/lib/api/generated/model";
import {
  formatDate,
  formatDayMonth,
  jakartaToday,
} from "../src/lib/format/date";
import {
  attachFullPageScreenshot,
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA } from "./support/staff";

// "Masa sewa" in "Sewakan unit" (decision 32: a lease is billed every 1, 3, 6 or 12 months, at the
// unit's price for that term), against the real API (seed-demo data). Pak Hendra is Admin of the
// whole of Griya Sejahtera. Each test makes its own property, unit and tenant through the API, with
// names and numbers no other run, project or retry uses (numbers 0813-4..., which no other spec
// uses). Property names start with "Wisma" and tenant names with W or Y, so they sort after every
// seeded property and tenant, and the seeded lists keep their first page.

const GRIYA = "Griya Sejahtera";
const NO_PRICE_MESSAGE =
  "Unit ini belum punya harga sewa. Atur harganya di halaman properti.";

// Minimum body text per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };

function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

// A name no other run, project, test or retry uses.
function uniqueName(testInfo: TestInfo, base: string): string {
  const project = testInfo.project.name === "phone" ? "Dua" : "Satu";
  const retry = testInfo.retry === 0 ? "" : ` r${testInfo.retry}`;
  return `${base} ${runId()} ${project}${retry}`;
}

// A tenant number no other spec, run, project, test or retry uses (0813-4...).
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = testInfo.project.name === "phone" ? "2" : "1";
  return `0813-4${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
}

// A calendar date `months` months from today in Jakarta, on the 10th (every month has one, so no
// month end can move it).
function tenthOfMonth(months: number): string {
  const [year = 0, month = 1] = jakartaToday(new Date()).split("-").map(Number);
  return new Date(Date.UTC(year, month - 1 + months, 10))
    .toISOString()
    .slice(0, 10);
}

async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

async function choose(
  page: Page,
  scope: Locator,
  field: string,
  option: string,
) {
  await scope.getByRole("combobox", { name: field }).click();
  await page.getByRole("option", { name: option, exact: true }).click();
}

// The options a select offers, in order, while it is open; it is closed again afterwards.
async function optionsOf(
  page: Page,
  scope: Locator,
  field: string,
): Promise<string[]> {
  await scope.getByRole("combobox", { name: field }).click();
  const options = page.getByRole("option");
  await expect(options.first()).toBeVisible();
  const texts = (await options.allTextContents()).map((text) => text.trim());
  await page.keyboard.press("Escape");
  await expect(options).toHaveCount(0);
  return texts;
}

// Opens Griya Sejahtera (Pak Hendra belongs to two organizations) and returns its id.
async function openGriya(page: Page): Promise<string> {
  await page.goto("/organizations");
  await page
    .getByRole("list", { name: "Daftar usaha" })
    .getByRole("link", { name: GRIYA })
    .click();
  await expect(page).toHaveURL(ORG_HOME_URL);
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

// Setup through the API, as the signed-in member (the page's cookies).
async function post<T>(page: Page, path: string, data: object): Promise<T> {
  const response = await page.request.post(`/api${path}`, { data });
  expect(response.status(), await response.text()).toBe(201);
  return (await response.json()) as T;
}

// A property of the test's own with one free unit offering the given terms, and a tenant with no
// lease yet.
async function createFixtures(
  page: Page,
  orgId: string,
  testInfo: TestInfo,
  fixture: {
    property: string;
    kind: PropertyKind;
    unit: string;
    terms: UnitTermInput[];
    tenant: string;
    testDigit: number;
  },
) {
  const property = await post<Property>(page, `/orgs/${orgId}/properties`, {
    name: uniqueName(testInfo, fixture.property),
    kind: fixture.kind,
    address: "",
  });
  const unit = await post<Unit>(
    page,
    `/orgs/${orgId}/properties/${property.id}/units`,
    { label: fixture.unit, terms: fixture.terms },
  );
  const tenant = await post<Tenant>(page, `/orgs/${orgId}/tenants`, {
    name: uniqueName(testInfo, fixture.tenant),
    phone: tenantNumber(testInfo, fixture.testDigit),
    note: "",
  });
  return { property, unit, tenant };
}

// The tenant's page, then "Sewakan unit" with the property and unit chosen.
async function openLeaseForm(
  page: Page,
  orgId: string,
  fixtures: { property: Property; unit: Unit; tenant: Tenant },
): Promise<Locator> {
  await page.goto(`/o/${orgId}/tenants/${fixtures.tenant.id}`);
  await expect(
    page.getByRole("heading", { level: 1, name: fixtures.tenant.name }),
  ).toBeVisible();
  await page
    .getByRole("region", { name: "Belum ada sewa" })
    .getByRole("button", { name: "Sewakan unit" })
    .click();
  const lease = page.getByRole("dialog", { name: "Sewakan unit" });
  await choose(page, lease, "Properti", fixtures.property.name);
  // No unit yet: nothing to choose.
  const term = lease.getByRole("combobox", { name: "Masa sewa" });
  await expect(term).toBeDisabled();
  await expect(term).toHaveText("Pilih unit dulu");
  await choose(page, lease, "Unit", fixtures.unit.label);
  return lease;
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("rents a unit by the year: the choices step by a year and the request carries the term", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openGriya(page);
    const fixtures = await createFixtures(page, orgId, testInfo, {
      property: "Wisma Uji Tahunan",
      kind: "kos",
      unit: "Kamar 1",
      terms: [
        { months: 1, price: 1_250_000 },
        { months: 12, price: 13_500_000 },
      ],
      tenant: "Wiryo Tahunan",
      testDigit: 1,
    });
    const lease = await openLeaseForm(page, orgId, fixtures);

    // The unit's terms, shortest first; the monthly one is chosen to begin with.
    const term = lease.getByRole("combobox", { name: "Masa sewa" });
    await expect(term).toHaveText("Per bulan · Rp1.250.000");
    expect(await optionsOf(page, lease, "Masa sewa")).toEqual([
      "Per bulan · Rp1.250.000",
      "Per tahun · Rp13.500.000",
    ]);
    await choose(page, lease, "Masa sewa", "Per tahun · Rp13.500.000");
    await expect(term).toHaveText("Per tahun · Rp13.500.000");
    const hint = lease.getByText(
      "Tagihan dibuat sekali tiap tahun, dibayar di muka.",
      { exact: false },
    );
    await expect(hint).toBeVisible();
    expect(await fontSize(hint)).toBeGreaterThanOrEqual(
      MIN_BODY_FONT_PX[testInfo.project.name] ?? 16,
    );
    await expectTouchTarget(term);
    await term.click();
    await expect(page.getByRole("option").first()).toBeVisible();
    await attachViewportScreenshot(page, testInfo, "lease-form-term-options");
    await page.keyboard.press("Escape");
    await expect(page.getByRole("option")).toHaveCount(0);

    // Started 13 months ago: its periods start then, a year later (last month), and a year after
    // that, the latest billing start the API allows.
    const start = tenthOfMonth(-13);
    const secondYear = tenthOfMonth(-1);
    const thirdYear = tenthOfMonth(11);
    await lease.getByLabel("Mulai sewa").fill(start);
    await expect(lease).toContainText(
      `Juga jadi tanggal tagihan tiap tahun. Mulai ${formatDayMonth(start)}, jatuh tempo tiap ${formatDayMonth(start)}.`,
    );
    const billing = lease.getByRole("group", { name: "Mulai tagih dari" });
    await expect(billing).toContainText(
      `Semua tahun sejak ${formatDate(start)} dibuatkan tagihan.`,
    );
    await expect(lease).toContainText(
      `jadi setiap tahun sejak ${formatDate(start)} ikut ditagih`,
    );
    await billing
      .getByRole("radio", { name: /Mulai dari tahun yang saya pilih/ })
      .check();
    expect(await optionsOf(page, billing, "Tahun mulai tagih")).toEqual([
      formatDate(secondYear),
      formatDate(thirdYear),
    ]);
    await choose(page, billing, "Tahun mulai tagih", formatDate(secondYear));
    await expect(lease).toContainText(
      `jadi tahun berjalan sejak ${formatDate(secondYear)} ikut ditagih`,
    );
    await attachFullPageScreenshot(page, testInfo, "lease-form-term");

    const saved = page.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname.endsWith("/leases"),
    );
    await lease.getByRole("button", { name: "Simpan sewa" }).click();
    expect((await saved).postDataJSON()).toMatchObject({
      tenantId: fixtures.tenant.id,
      unitId: fixtures.unit.id,
      startDate: start,
      billedFrom: secondYear,
      termMonths: 12,
    });
    await expect(lease).toHaveCount(0);

    const card = page.getByRole("region", {
      name: `${fixtures.unit.label} · ${fixtures.property.name}`,
    });
    await expect(card).toContainText("Sewa per tahun");
    await expect(card).toContainText("Rp13.500.000");
    await expect(card).toContainText(formatDate(secondYear));

    expect(outsideRequests).toEqual([]);
  });

  test("offers a ruko unit only its yearly term", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openGriya(page);
    const fixtures = await createFixtures(page, orgId, testInfo, {
      property: "Wisma Uji Ruko Tahunan",
      kind: "ruko",
      unit: "Ruko 1",
      terms: [{ months: 12, price: 50_000_000 }],
      tenant: "Yohanes Ruko",
      testDigit: 2,
    });
    const lease = await openLeaseForm(page, orgId, fixtures);

    await expect(lease.getByRole("combobox", { name: "Masa sewa" })).toHaveText(
      "Per tahun · Rp50.000.000",
    );
    expect(await optionsOf(page, lease, "Masa sewa")).toEqual([
      "Per tahun · Rp50.000.000",
    ]);
    await lease.getByRole("button", { name: "Batal" }).click();
    await expect(lease).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });

  test("refuses a unit with no price under Masa sewa before sending anything", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openGriya(page);
    const fixtures = await createFixtures(page, orgId, testInfo, {
      property: "Wisma Uji Tanpa Harga",
      kind: "kos",
      unit: "Kamar 1",
      terms: [{ months: 1 }],
      tenant: "Wening Tanpa Harga",
      testDigit: 3,
    });
    const leaseRequests: string[] = [];
    page.on("request", (request) => {
      if (new URL(request.url()).pathname.endsWith("/leases")) {
        leaseRequests.push(request.method());
      }
    });
    const lease = await openLeaseForm(page, orgId, fixtures);

    // The only term has no price: it shows, but cannot be chosen.
    const term = lease.getByRole("combobox", { name: "Masa sewa" });
    await expect(term).toHaveText("Pilih masa sewa");
    await term.click();
    const option = page.getByRole("option", {
      name: "Per bulan · belum ada harga",
      exact: true,
    });
    await expect(option).toHaveAttribute("aria-disabled", "true");
    await page.keyboard.press("Escape");
    await expect(option).toHaveCount(0);

    await lease.getByLabel("Mulai sewa").fill(jakartaToday(new Date()));
    await lease.getByRole("button", { name: "Simpan sewa" }).click();
    await expect(lease.getByText(NO_PRICE_MESSAGE)).toBeVisible();
    await expect(term).toHaveAttribute("aria-invalid", "true");
    await expect(term).toBeFocused();
    expect(leaseRequests).toEqual([]);
    await attachFullPageScreenshot(page, testInfo, "lease-form-no-price");

    expect(outsideRequests).toEqual([]);
  });
});
