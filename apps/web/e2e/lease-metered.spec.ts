import {
  expect,
  test,
  type Locator,
  type Page,
  type Request,
  type TestInfo,
} from "@playwright/test";

import type { Lease, Tenant, Unit } from "../src/lib/api/generated/model";
import { jakartaToday } from "../src/lib/format/date";
import { findOrCreateMeteredTypes } from "./support/meter-fixtures";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { minBodyFontPx, projectDigit, projectWord } from "./support/projects";
import { findOrCreateProperty } from "./support/property-fixtures";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA } from "./support/staff";

// "Dari meteran" in a lease's "Biaya tambahan": the metered types (Listrik, Air) as switches with
// no amount, against the real API (seed-demo data). Pak Hendra is Admin of the whole of Griya
// Sejahtera (lease:manage) and Supervisor in Kost Putri Anggrek (no lease:manage). Each test
// changes only its own unit, tenant and lease, in a property reused across runs
// (support/property-fixtures.ts).

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const PANEL = "Biaya tambahan";
const SAVE = "Simpan biaya tambahan";
const SECTION = "Dari meteran";
const NOTE =
  "Nyalakan yang ditagih dari meteran. Jumlahnya dicatat tiap bulan lewat Catat meteran.";
const HINT = "Dicatat dari meteran tiap bulan";

function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

// A name no other run, project or retry uses.
function uniqueName(testInfo: TestInfo, base: string): string {
  const project = projectWord(testInfo);
  const retry = testInfo.retry === 0 ? "" : ` r${testInfo.retry}`;
  return `${base} ${runId()} ${project}${retry}`;
}

// A tenant number no other run, project or retry uses; 0814-6 is used by no other spec.
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = projectDigit(testInfo);
  return `0814-6${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
}

async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

// Setup through the API, as the signed-in member (the page's cookies).
async function call<T>(
  page: Page,
  path: string,
  want: number,
  data: object,
): Promise<T> {
  const response = await page.request.fetch(`/api${path}`, {
    method: "POST",
    data,
  });
  expect(response.status(), await response.text()).toBe(want);
  return (await response.json()) as T;
}

// Signs into the organization by name and returns its id.
async function openOrganization(page: Page, name: string): Promise<string> {
  await page.goto("/organizations");
  await page
    .getByRole("list", { name: "Daftar usaha" })
    .getByRole("link", { name })
    .click();
  await expect(page).toHaveURL(ORG_HOME_URL);
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

function isChargesSave(request: Request): boolean {
  return (
    request.method() === "PUT" &&
    /\/api\/orgs\/[^/]+\/leases\/[^/]+\/charges$/.test(request.url())
  );
}

function switchOf(panel: Locator, name: string): Locator {
  return panel.getByRole("switch", { name, exact: true });
}

// Saves the panel and returns the request the page sent and the lease the API answered.
async function save(
  page: Page,
  panel: Locator,
): Promise<{
  body: { charges: unknown[]; meteredChargeTypeIds?: string[] };
  lease: Lease;
}> {
  const [request] = await Promise.all([
    page.waitForRequest(isChargesSave),
    panel.getByRole("button", { name: SAVE }).click(),
  ]);
  const response = await request.response();
  expect(response?.status(), await response?.text()).toBe(200);
  return {
    body: request.postDataJSON() as {
      charges: unknown[];
      meteredChargeTypeIds?: string[];
    },
    lease: (await response?.json()) as Lease,
  };
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("turns Listrik on and Air off, saves, and sees the same again", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const { listrik, air } = await findOrCreateMeteredTypes(page, orgId);
    const property = await findOrCreateProperty(page, orgId, testInfo, {
      base: "Meteran",
      kind: "kos",
    });
    const unit = await call<Unit>(
      page,
      `/orgs/${orgId}/properties/${property.id}/units`,
      201,
      {
        label: uniqueName(testInfo, "Kamar Meter 1"),
        terms: [{ months: 1, price: 1_250_000 }],
      },
    );
    const tenant = await call<Tenant>(page, `/orgs/${orgId}/tenants`, 201, {
      name: uniqueName(testInfo, "Wahyu Setiawan"),
      phone: tenantNumber(testInfo, 1),
      note: "",
    });
    await call<Lease>(page, `/orgs/${orgId}/leases`, 201, {
      tenantId: tenant.id,
      unitId: unit.id,
      startDate: jakartaToday(new Date()),
      termMonths: 1,
      depositAmount: 1_250_000,
    });

    await page.goto(`/o/${orgId}/tenants/${tenant.id}`);
    await expect(
      page.getByRole("heading", { level: 1, name: tenant.name }),
    ).toBeVisible();
    const card = page.getByRole("region", {
      name: `${unit.label} · ${property.name}`,
    });
    await card.getByRole("button", { name: PANEL, exact: true }).click();
    const panel = page.getByRole("dialog", { name: PANEL });
    await expect(panel).toBeVisible();

    // A new lease expects no meter: both types are listed, switched off, with no amount field.
    await expect(
      panel.getByRole("heading", { level: 3, name: SECTION }),
    ).toBeVisible();
    await expect(panel).toContainText(NOTE);
    await expect(panel).not.toContainText("tidak diatur di sini");
    await expect(switchOf(panel, listrik.name)).not.toBeChecked();
    await expect(switchOf(panel, air.name)).not.toBeChecked();
    await expect(
      panel.getByRole("textbox", { name: `Jumlah ${listrik.name}` }),
    ).toHaveCount(0);

    const min = minBodyFontPx(testInfo);
    const row = panel
      .locator("label")
      .filter({ has: page.getByText(listrik.name, { exact: true }) });
    await expectTouchTarget(row);
    expect(
      await fontSize(row.getByText(listrik.name, { exact: true })),
    ).toBeGreaterThanOrEqual(min);
    expect(await fontSize(row.getByText(HINT))).toBeGreaterThanOrEqual(min);
    expect(await fontSize(panel.getByText(NOTE))).toBeGreaterThanOrEqual(min);
    await expectTouchTarget(panel.getByRole("button", { name: SAVE }));

    // Clicking the row's label flips the switch too.
    await row.getByText(listrik.name, { exact: true }).click();
    await expect(switchOf(panel, listrik.name)).toBeChecked();
    await expect(switchOf(panel, air.name)).not.toBeChecked();
    await attachViewportScreenshot(page, testInfo, "lease-metered");

    const first = await save(page, panel);
    expect(first.body.charges).toEqual([]);
    expect(first.body.meteredChargeTypeIds).toEqual([listrik.id]);
    expect(first.lease.meteredChargeTypes).toEqual([
      { chargeTypeId: listrik.id, name: listrik.name },
    ]);
    await expect(panel).toHaveCount(0);

    // Opened again: the same.
    await card.getByRole("button", { name: PANEL, exact: true }).click();
    await expect(panel).toBeVisible();
    await expect(switchOf(panel, listrik.name)).toBeChecked();
    await expect(switchOf(panel, air.name)).not.toBeChecked();

    // All off sends an empty list, not nothing: the API keeps what is left out.
    await switchOf(panel, listrik.name).click();
    await expect(switchOf(panel, listrik.name)).not.toBeChecked();
    const second = await save(page, panel);
    expect(second.body.meteredChargeTypeIds).toEqual([]);
    expect(second.lease.meteredChargeTypes).toEqual([]);
    await expect(panel).toHaveCount(0);
    await card.getByRole("button", { name: PANEL, exact: true }).click();
    await expect(switchOf(panel, listrik.name)).not.toBeChecked();
    await expect(switchOf(panel, air.name)).not.toBeChecked();

    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("cannot open Biaya tambahan", async ({ page }) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, ANGGREK);
    await page.goto(`/o/${orgId}/tenants`);
    const list = page.getByRole("list", { name: "Daftar penyewa" });
    await list
      .getByRole("listitem")
      .filter({ hasText: "Nadia Putri" })
      .getByRole("link")
      .click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Nadia Putri" }),
    ).toBeVisible();
    await expect(page.getByRole("region").first()).toBeVisible();
    await expect(page.getByRole("button", { name: PANEL })).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });
});
