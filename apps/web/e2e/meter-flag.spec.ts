import {
  expect,
  test,
  type Locator,
  type Page,
  type Response,
  type TestInfo,
} from "@playwright/test";

import {
  currentPeriod,
  METER_CHIP_CLEAR_LABEL,
  METER_LIST_LABEL,
  previousMonth,
} from "../src/components/client/invoice/utils";
import type {
  Lease,
  MissingMeterReading,
  MissingMeterReadingList,
  Tenant,
  Unit,
} from "../src/lib/api/generated/model";
import {
  formatDayMonth,
  formatMonthYear,
  jakartaToday,
} from "../src/lib/format/date";
import { findOrCreateMeteredTypes } from "./support/meter-fixtures";
import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { minBodyFontPx, projectDigit, projectWord } from "./support/projects";
import { findOrCreateProperty } from "./support/property-fixtures";
import { isPhone, ORG_HOME_URL } from "./support/shell";
import { HENDRA } from "./support/staff";

// "Meteran belum dicatat" on Penagihan (W14), against the real API. The test makes its own unit,
// tenant and lease (started on the 1st of last month in Jakarta, with Listrik switched on and
// nothing recorded) and reads the screen. Other specs write to the same organization at the same
// time, so every figure is read from the answer the page itself received, never from a number
// read earlier. Pak Hendra is Admin of the whole of Griya Sejahtera (charge:read).

const GRIYA = "Griya Sejahtera";
const SHOW = "Tampilkan";
const CHIP = METER_CHIP_CLEAR_LABEL;
const ROW_CHIP = "Meteran belum dicatat";
const HINT =
  "Tagihan tetap terbit tanpa listrik atau air. Yang dicatat terlambat masuk ke tagihan bulan berikutnya.";
const LINK = "Lihat penyewa";

const PAGE_SIZE = 20;

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

// A tenant number no other run, project or retry uses; 0814-6 is used by no other spec
// (lease-metered.spec.ts takes digit 1 of it, this one digit 2).
function tenantNumber(testInfo: TestInfo): string {
  const run = runId();
  const project = projectDigit(testInfo);
  return `0814-6${run.slice(0, 3)}-${run.slice(3)}${project}2${testInfo.retry}`;
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
  method: "POST" | "PUT",
  path: string,
  want: number,
  data: object,
): Promise<T> {
  const response = await page.request.fetch(`/api${path}`, { method, data });
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

// An answer to listMissingMeterReadings with this page size (the box asks for 1, the list for
// PAGE_SIZE) and, when given, this page.
function missingAnswer(
  page: Page,
  pageSize: number,
  listPage?: number,
): Promise<Response> {
  return page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      response.request().method() === "GET" &&
      /\/api\/orgs\/[0-9a-f-]{36}\/metered-charges\/missing$/.test(
        url.pathname,
      ) &&
      url.searchParams.get("pageSize") === String(pageSize) &&
      (listPage === undefined ||
        url.searchParams.get("page") === String(listPage))
    );
  });
}

async function listOf(answer: Promise<Response>) {
  const response = await answer;
  expect(response.status(), await response.text()).toBe(200);
  return (await response.json()) as MissingMeterReadingList;
}

// The page of the list (PAGE_SIZE rows) that holds the lease right now, read from the API.
async function pageOfLease(
  page: Page,
  orgId: string,
  leaseId: string,
): Promise<number> {
  let seen = 0;
  for (let from = 1; ; from++) {
    const response = await page.request.get(
      `/api/orgs/${orgId}/metered-charges/missing`,
      { params: { page: from, pageSize: 100 } },
    );
    expect(response.status(), await response.text()).toBe(200);
    const list = (await response.json()) as MissingMeterReadingList;
    const index = list.items.findIndex((item) => item.leaseId === leaseId);
    if (index >= 0) return Math.floor((seen + index) / PAGE_SIZE) + 1;
    seen += list.items.length;
    expect(seen, "the lease is on the list").toBeLessThan(list.total);
  }
}

// Opens the list page that holds the lease and returns the API's row for it. Other specs add
// leases to the same list, which sorts by tenant name, so the page is read again if the row moved.
async function openListWith(
  page: Page,
  orgId: string,
  leaseId: string,
): Promise<MissingMeterReading> {
  for (let attempt = 0; attempt < 4; attempt++) {
    const n = await pageOfLease(page, orgId, leaseId);
    const answer = missingAnswer(page, PAGE_SIZE, n);
    await page.goto(`/o/${orgId}/billing?meter=missing&page=${n}`);
    const item = (await listOf(answer)).items.find(
      (row) => row.leaseId === leaseId,
    );
    if (item !== undefined) return item;
  }
  throw new Error("the lease did not stay on one page of the list");
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("flags a lease with no meter amount, lists it and clears the view", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const { listrik } = await findOrCreateMeteredTypes(page, orgId);
    const property = await findOrCreateProperty(page, orgId, testInfo, {
      base: "Meteran",
      kind: "kos",
    });
    const unit = await call<Unit>(
      page,
      "POST",
      `/orgs/${orgId}/properties/${property.id}/units`,
      201,
      {
        label: uniqueName(testInfo, "Kamar Meter 2"),
        terms: [{ months: 1, price: 1_250_000 }],
      },
    );
    const tenant = await call<Tenant>(
      page,
      "POST",
      `/orgs/${orgId}/tenants`,
      201,
      {
        name: uniqueName(testInfo, "Yuliana Kartika"),
        phone: tenantNumber(testInfo),
        note: "",
      },
    );
    const lastMonth = previousMonth(currentPeriod(jakartaToday(new Date())));
    const lease = await call<Lease>(
      page,
      "POST",
      `/orgs/${orgId}/leases`,
      201,
      {
        tenantId: tenant.id,
        unitId: unit.id,
        startDate: `${lastMonth}-01`,
        termMonths: 1,
        depositAmount: 1_250_000,
      },
    );
    // Listrik is expected every month; Air is not. Nothing is recorded.
    await call<Lease>(
      page,
      "PUT",
      `/orgs/${orgId}/leases/${lease.id}/charges`,
      200,
      {
        charges: [],
        meteredChargeTypeIds: [listrik.id],
      },
    );

    // The box: last month's name and the API's own total.
    const counted = missingAnswer(page, 1);
    await page.goto(`/o/${orgId}/billing`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Penagihan" }),
    ).toBeVisible();
    const flag = await listOf(counted);
    expect(flag.total).toBeGreaterThanOrEqual(1);
    const title = `Meteran ${formatMonthYear(`${lastMonth}-01`)} belum dicatat untuk ${flag.total} sewa`;
    const box = page.getByRole("status").filter({ hasText: title });
    await expect(box).toBeVisible();
    await expect(box).toContainText(HINT);
    const min = minBodyFontPx(testInfo);
    expect(await fontSize(box.getByText(title))).toBeGreaterThanOrEqual(min);
    expect(await fontSize(box.getByText(HINT))).toBeGreaterThanOrEqual(min);
    const show = box.getByRole("button", { name: SHOW, exact: true });
    await expectTouchTarget(show);
    await attachFullPageScreenshot(page, testInfo, "meter-flag");

    // "Tampilkan": a different view; the invoices give way to the chip and the count.
    const firstPage = missingAnswer(page, PAGE_SIZE, 1);
    await show.click();
    await expect(page).toHaveURL(/[?&]meter=missing(&|$)/);
    const list = await listOf(firstPage);
    const chip = page.getByRole("button", { name: CHIP });
    await expect(chip).toBeVisible();
    await expectTouchTarget(chip);
    await expect(chip).toContainText(ROW_CHIP);
    await expect(
      page.getByText(`Menampilkan ${list.total} sewa`),
    ).toBeVisible();
    await expect(page.getByText("Terkumpul")).toHaveCount(0);
    await expect(box).toHaveCount(0);

    // The lease, on the page that holds it (the list sorts by tenant name).
    const item = await openListWith(page, orgId, lease.id);
    expect(item.tenantName).toBe(tenant.name);
    expect(item.unitLabel).toBe(unit.label);
    expect(item.propertyName).toBe(property.name);
    expect(item.missing.map((type) => type.name)).toEqual([listrik.name]);
    const next =
      item.nextInvoice === undefined
        ? "—"
        : `terbit ${formatDayMonth(item.nextInvoice.issuedOn)}`;
    const row = page
      .getByRole("list", { name: METER_LIST_LABEL })
      .getByRole("listitem")
      .filter({ hasText: tenant.name });
    await expect(row).toHaveCount(1);
    await expect(row).toContainText(unit.label);
    await expect(row).toContainText(property.name);
    await expect(row.getByText(ROW_CHIP)).toBeVisible();
    if (isPhone(testInfo)) {
      await expect(row).toContainText(`Belum dicatat: ${listrik.name}`);
      await expect(row).toContainText(`Tagihan berikutnya ${next}`);
    } else {
      await expect(row.getByText(listrik.name, { exact: true })).toBeVisible();
      await expect(row.getByText(next, { exact: true })).toBeVisible();
      for (const header of [
        "Penyewa",
        "Unit",
        "Belum dicatat",
        "Tagihan berikutnya",
        "Aksi",
      ]) {
        await expect(
          page.getByRole("main").getByText(header, { exact: true }),
        ).toBeVisible();
      }
    }
    await expect(row).not.toContainText("Air");
    expect(
      await fontSize(row.getByText(tenant.name, { exact: true })),
    ).toBeGreaterThanOrEqual(min);
    const link = row.getByRole("link", { name: LINK, exact: true });
    await expectTouchTarget(link);
    await attachFullPageScreenshot(page, testInfo, "meter-missing");

    // "Lihat penyewa" is the way to Detail penyewa, where "Catat meteran" is.
    await link.click();
    await expect(page).toHaveURL(`/o/${orgId}/tenants/${tenant.id}`);
    await expect(
      page.getByRole("heading", { level: 1, name: tenant.name }),
    ).toBeVisible();
    await page.goBack();
    await expect(page).toHaveURL(/[?&]meter=missing(&|$)/);
    await expect(chip).toBeVisible();

    // The chip takes the view off: the invoices and the box are back, the param is gone.
    await chip.click();
    await expect(page).not.toHaveURL(/meter=/);
    await expect(page.getByText("Terkumpul")).toBeVisible();
    await expect(page.getByRole("button", { name: CHIP })).toHaveCount(0);
    await expect(
      page.getByRole("status").filter({ hasText: "belum dicatat untuk" }),
    ).toBeVisible();

    expect(outsideRequests).toEqual([]);
  });

  test("ignores a meter value it does not know", async ({ page }) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    await page.goto(`/o/${orgId}/billing?meter=all`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Penagihan" }),
    ).toBeVisible();
    await expect(page.getByText("Terkumpul")).toBeVisible();
    await expect(page.getByRole("button", { name: CHIP })).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });
});
