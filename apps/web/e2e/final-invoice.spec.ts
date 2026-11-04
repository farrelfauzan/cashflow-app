import {
  expect,
  test,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import type {
  Lease,
  MeteredChargeList,
  Tenant,
  TenantAccount,
  Unit,
} from "../src/lib/api/generated/model";
import { formatMonthYear, jakartaToday } from "../src/lib/format/date";
import {
  attachFullPageScreenshot,
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { findOrCreateMeteredTypes } from "./support/meter-fixtures";
import { findOrCreateProperty } from "./support/property-fixtures";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA } from "./support/staff";

// The final invoice ("Tagihan akhir"), against the real API and its worker. The test makes its
// own kos unit, tenant and lease (started on the 1st of last month, so the worker invoices it at
// once), records last month's Listrik after those invoices exist, so no regular invoice bills it,
// and moves the tenant out today. The API makes the final invoice in the move-out request. Then
// Detail penyewa and Penagihan must name it. Pak Hendra is Admin of the whole of Griya Sejahtera.

const GRIYA = "Griya Sejahtera";
const SEARCH = "Cari penyewa atau unit…";
const FINAL_LABEL = "Tagihan akhir";

function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

// A name no other run, project or retry uses.
function uniqueName(testInfo: TestInfo, base: string): string {
  const project = testInfo.project.name === "phone" ? "Dua" : "Satu";
  const retry = testInfo.retry === 0 ? "" : ` r${testInfo.retry}`;
  return `${base} ${runId()} ${project}${retry}`;
}

// A tenant number no other run, project or retry uses; 0814-7 is used by no other spec.
function tenantNumber(testInfo: TestInfo): string {
  const run = runId();
  const project = testInfo.project.name === "phone" ? "2" : "1";
  return `0814-7${run.slice(0, 3)}-${run.slice(3)}${project}1${testInfo.retry}`;
}

// A month in Jakarta, YYYY-MM, `offset` months from today's.
function monthFrom(offset: number): string {
  const [year = 0, month = 1] = jakartaToday(new Date()).split("-").map(Number);
  return new Date(Date.UTC(year, month - 1 + offset, 1))
    .toISOString()
    .slice(0, 7);
}

async function call<T>(
  page: Page,
  method: "GET" | "POST" | "PUT",
  path: string,
  want: number,
  data?: object,
): Promise<T> {
  const response = await page.request.fetch(`/api${path}`, { method, data });
  expect(response.status(), await response.text()).toBe(want);
  return (await response.json()) as T;
}

function rows(list: Locator): Locator {
  return list.locator(":scope > li");
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("names the invoice made at move-out Tagihan akhir on Detail penyewa and Penagihan", async ({
    page,
  }, testInfo) => {
    test.setTimeout(120_000);
    const outsideRequests = trackOutsideRequests(page);
    await page.goto("/organizations");
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name: GRIYA })
      .click();
    await expect(page).toHaveURL(ORG_HOME_URL);
    const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
    if (orgId === undefined) throw new Error("not on an organization's page");
    const today = jakartaToday(new Date());

    // A tenant renting a kos unit from the 1st of last month, expecting Listrik from a meter.
    const { listrik } = await findOrCreateMeteredTypes(page, orgId);
    const property = await findOrCreateProperty(page, orgId, testInfo, {
      base: "Tagihan Akhir",
      kind: "kos",
    });
    const unit = await call<Unit>(
      page,
      "POST",
      `/orgs/${orgId}/properties/${property.id}/units`,
      201,
      {
        label: uniqueName(testInfo, "Kamar Akhir"),
        terms: [{ months: 1, price: 1_200_000 }],
      },
    );
    const tenant = await call<Tenant>(
      page,
      "POST",
      `/orgs/${orgId}/tenants`,
      201,
      {
        name: uniqueName(testInfo, "Yusuf Hidayat"),
        phone: tenantNumber(testInfo),
        note: "",
      },
    );
    const created = await call<Lease>(
      page,
      "POST",
      `/orgs/${orgId}/leases`,
      201,
      {
        tenantId: tenant.id,
        unitId: unit.id,
        startDate: `${monthFrom(-1)}-01`,
        termMonths: 1,
        depositAmount: 0,
      },
    );
    const lease = await call<Lease>(
      page,
      "PUT",
      `/orgs/${orgId}/leases/${created.id}/charges`,
      200,
      { charges: [], meteredChargeTypeIds: [listrik.id] },
    );
    const metersPath = `/orgs/${orgId}/leases/${lease.id}/metered-charges`;

    // The worker has made the regular invoices once the next one is a month ahead of this one;
    // only then does an amount recorded for last month stay unbilled.
    await expect
      .poll(
        async () => {
          const { nextInvoice } = await call<MeteredChargeList>(
            page,
            "GET",
            metersPath,
            200,
          );
          return (nextInvoice?.period ?? "") > `${monthFrom(0)}-01`;
        },
        { timeout: 20_000 },
      )
      .toBe(true);
    await call(
      page,
      "PUT",
      `${metersPath}/${listrik.id}/${monthFrom(-1)}`,
      200,
      { amount: 187_500, meterStart: 300, meterEnd: 410 },
    );
    await call<Lease>(
      page,
      "POST",
      `/orgs/${orgId}/leases/${lease.id}/move-out`,
      200,
      { endDate: today },
    );

    // The move-out request made the final invoice: one, for the month of the move-out day.
    const accountPath = `/orgs/${orgId}/tenants/${tenant.id}/account`;
    const account = await call<TenantAccount>(page, "GET", accountPath, 200);
    const finals = account.invoices.filter((each) => each.kind === "final");
    expect(finals).toHaveLength(1);
    const [final] = finals;
    if (final === undefined) throw new Error("no final invoice");
    expect(final.period).toBe(today);
    const [line] = final.lines;
    expect(final.lines).toHaveLength(1);
    expect(line?.label).toMatch(/^Listrik /);
    const finalTitle = `${FINAL_LABEL} ${formatMonthYear(today)}`;

    // Detail penyewa, "Tagihan": the final invoice's row, with its metered line.
    await page.goto(`/o/${orgId}/tenants/${tenant.id}`);
    await expect(
      page.getByRole("heading", { level: 1, name: tenant.name }),
    ).toBeVisible();
    const region = page.getByRole("region", { name: "Uang penyewa" });
    await expect(region).toBeVisible();
    const invoices = region.getByRole("list", { name: "Daftar tagihan" });
    const finalRow = rows(invoices).filter({
      has: page.getByRole("heading", { level: 3, name: finalTitle }),
    });
    await expect(finalRow).toHaveCount(1);
    await expect(finalRow).toContainText(line?.label ?? "");
    await expect(finalRow).toContainText("Rp187.500");
    // The regular invoices keep their plain month, so only one row is named "Tagihan akhir".
    await expect(
      invoices.getByRole("heading", { level: 3, name: FINAL_LABEL }),
    ).toHaveCount(1);
    await finalRow.scrollIntoViewIfNeeded();
    await attachViewportScreenshot(page, testInfo, "final-invoice-account");

    // Penagihan, this month: the tenant's rows, one of them with the "Tagihan akhir" chip.
    await page.goto(`/o/${orgId}/billing`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Penagihan" }),
    ).toBeVisible();
    await page.getByRole("searchbox", { name: SEARCH }).fill(tenant.name);
    const list = page.getByRole("list", { name: "Daftar tagihan" });
    // Wait for the search to narrow the month to the tenant's own invoices, so the screenshot
    // shows them and not the unfiltered list.
    const rowsOf = list.getByRole("listitem");
    const found = rowsOf.filter({ hasText: tenant.name });
    await expect(found.first()).toBeVisible();
    await expect(rowsOf.filter({ hasNotText: tenant.name })).toHaveCount(0);
    const finalChip = found.getByText(FINAL_LABEL, { exact: true });
    await expect(finalChip).toHaveCount(1);
    await expect(finalChip).toBeVisible();
    // The chip sits on the row that bills Listrik alone; the regular invoice has none.
    await expect(
      found.filter({ has: page.getByText(FINAL_LABEL, { exact: true }) }),
    ).toContainText("Rp187.500");
    await attachFullPageScreenshot(page, testInfo, "final-invoice-billing");
    expect(outsideRequests).toEqual([]);
  });
});
