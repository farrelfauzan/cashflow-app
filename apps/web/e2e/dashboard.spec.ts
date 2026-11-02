import {
  expect,
  test,
  type Page,
  type Response,
  type TestInfo,
} from "@playwright/test";

import type {
  Dashboard,
  Lease,
  Tenant,
  TenantAccount,
  Unit,
} from "../src/lib/api/generated/model";
import {
  formatLongDate,
  formatMonth,
  jakartaToday,
} from "../src/lib/format/date";
import { formatRupiah, formatSignedRupiah } from "../src/lib/format/rupiah";
import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { findOrCreateProperty } from "./support/property-fixtures";
import { HOME_HEADING, ORG_HOME_URL } from "./support/shell";
import { HENDRA, RUDI, type SeededStaff } from "./support/staff";

// Beranda, the organization's home, against the real API and its worker. seed-demo makes no
// invoices, so the admin test makes its own debt: a kos unit, a tenant and a lease entered late
// (started on the 10th two months ago), which the API invoices for every period since its start,
// and a correction with a reason only this run uses. Other specs write money to the same
// organization at the same time, so every figure is checked against the getDashboard response
// the page itself received, never against a number read earlier. Pak Hendra is Admin of the
// whole of Griya Sejahtera and Supervisor in Kost Putri Anggrek; Mas Rudi is Karyawan in Kost
// Putri Anggrek, without report:read.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const REGION = "Ringkasan uang";

// Minimum body text per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };

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

// A tenant number no other run, project or retry uses; 0814-2 is used by no other spec.
function tenantNumber(testInfo: TestInfo): string {
  const run = runId();
  const project = testInfo.project.name === "phone" ? "2" : "1";
  return `0814-2${run.slice(0, 3)}-${run.slice(3)}${project}0${testInfo.retry}`;
}

// The 10th of the month `months` from today in Jakarta (every month has one).
function tenthOfMonth(months: number): string {
  const [year = 0, month = 1] = jakartaToday(new Date()).split("-").map(Number);
  return new Date(Date.UTC(year, month - 1 + months, 10))
    .toISOString()
    .slice(0, 10);
}

// Setup through the API, as the signed-in member (the page's cookies).
async function call<T>(
  page: Page,
  method: "GET" | "POST",
  path: string,
  want: number,
  data?: object,
): Promise<T> {
  const response = await page.request.fetch(`/api${path}`, { method, data });
  expect(response.status(), await response.text()).toBe(want);
  return (await response.json()) as T;
}

function isDashboardResponse(response: Response): boolean {
  return (
    response.request().method() === "GET" &&
    /\/api\/orgs\/[0-9a-f-]{36}\/dashboard(\?|$)/.test(response.url())
  );
}

// Opens the organization's home (straight in for one organization, by name for several) and
// returns the getDashboard answer the page received.
async function openHome(
  page: Page,
  staff: SeededStaff,
  name: string,
): Promise<Dashboard> {
  const dashboardResponse = page.waitForResponse(isDashboardResponse);
  await page.goto("/organizations");
  if (staff.organizations.length > 1) {
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name })
      .click();
  }
  await expect(page).toHaveURL(ORG_HOME_URL);
  const response = await dashboardResponse;
  expect(response.status()).toBe(200);
  return (await response.json()) as Dashboard;
}

function orgIdOf(page: Page): string {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

// What the page must show for one getDashboard answer: the greeting and date, the month's card,
// the arrears total, "Belum bayar" and "Koreksi terbaru".
async function expectDashboard(page: Page, dashboard: Dashboard) {
  const today = jakartaToday(new Date());
  await expect(
    page.getByRole("heading", { level: 1, name: HOME_HEADING }),
  ).toBeVisible();
  await expect(page.getByText(formatLongDate(today))).toBeVisible();

  const region = page.getByRole("region", { name: REGION });
  const month = region.getByRole("group", {
    name: `Uang sewa bulan ${formatMonth(`${dashboard.period}-01`)}`,
  });
  await expect(month).toContainText(
    `${formatRupiah(dashboard.collected)} sudah masuk`,
  );
  if (dashboard.billed === 0) {
    await expect(month).toContainText("Belum ada tagihan bulan ini.");
  } else if (dashboard.outstanding === 0) {
    await expect(month).toContainText(
      `Dari ${formatRupiah(dashboard.billed)}. Semua sudah lunas.`,
    );
  } else {
    await expect(month).toContainText(
      `Dari ${formatRupiah(dashboard.billed)}. Masih kurang ${formatRupiah(dashboard.outstanding)} dari ${dashboard.tenantsOutstanding} penyewa.`,
    );
  }

  const total = region.getByRole("group", { name: "Tunggakan", exact: true });
  await expect(total).toContainText(formatRupiah(dashboard.overdue));
  if (dashboard.overdue > 0) {
    await expect(total).toContainText(
      `${dashboard.tenantsInArrears} penyewa terlambat bayar`,
    );
  } else {
    await expect(total).toContainText("Tidak ada tunggakan");
  }

  await expect(
    page.getByRole("heading", {
      level: 2,
      name: `Belum bayar · ${dashboard.tenantsInArrears} orang`,
    }),
  ).toBeVisible();
  const arrears = page.getByRole("list", { name: "Penyewa yang belum bayar" });
  await expect(arrears.locator(":scope > li")).toHaveCount(
    dashboard.arrears.length,
  );
  for (const [index, tenant] of dashboard.arrears.entries()) {
    const row = arrears.locator(":scope > li").nth(index);
    await expect(row).toContainText(tenant.tenantName);
    await expect(row).toContainText(formatRupiah(tenant.overdue));
    await expect(row).toContainText(`Terlambat ${tenant.ageDays} hari`);
  }

  await expect(
    page.getByRole("heading", { level: 2, name: "Koreksi terbaru" }),
  ).toBeVisible();
  if (dashboard.recentCorrections.length > 0) {
    const corrections = page.getByRole("list", {
      name: "Daftar koreksi terbaru",
    });
    await expect(corrections.locator(":scope > li")).toHaveCount(
      dashboard.recentCorrections.length,
    );
  }
}

async function fontSize(page: Page, text: string): Promise<number> {
  return page
    .getByText(text, { exact: true })
    .first()
    .evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees the month's money, who is in arrears and the newest corrections", async ({
    page,
  }, testInfo) => {
    test.setTimeout(90_000);
    const outsideRequests = trackOutsideRequests(page);
    await openHome(page, HENDRA, GRIYA);
    const orgId = orgIdOf(page);

    // Debt of the test's own: a lease entered late, invoiced for every period since its start.
    const property = await findOrCreateProperty(page, orgId, testInfo, {
      base: "Beranda",
      kind: "kos",
    });
    const unit = await call<Unit>(
      page,
      "POST",
      `/orgs/${orgId}/properties/${property.id}/units`,
      201,
      {
        label: uniqueName(testInfo, "Kamar Beranda"),
        terms: [{ months: 1, price: 950_000 }],
      },
    );
    const tenant = await call<Tenant>(
      page,
      "POST",
      `/orgs/${orgId}/tenants`,
      201,
      {
        name: uniqueName(testInfo, "Rina Puspitasari"),
        phone: tenantNumber(testInfo),
        note: "",
      },
    );
    await call<Lease>(page, "POST", `/orgs/${orgId}/leases`, 201, {
      tenantId: tenant.id,
      unitId: unit.id,
      startDate: tenthOfMonth(-2),
      termMonths: 1,
      depositAmount: 950_000,
    });
    const accountPath = `/orgs/${orgId}/tenants/${tenant.id}/account`;
    let account = await call<TenantAccount>(page, "GET", accountPath, 200);
    await expect
      .poll(
        async () => {
          account = await call<TenantAccount>(page, "GET", accountPath, 200);
          return account.overdue;
        },
        { timeout: 20_000 },
      )
      .toBeGreaterThan(0);
    const newest = account.invoices.at(-1);
    if (newest === undefined) throw new Error("no invoice");
    const reason = uniqueName(testInfo, "Diskon kamar bocor");
    await call(page, "POST", `/orgs/${orgId}/corrections`, 201, {
      requestId: crypto.randomUUID(),
      invoiceId: newest.id,
      amountDelta: -25_000,
      reason,
    });

    // Beranda again, checked against the answer it rendered.
    const dashboard = await openHome(page, HENDRA, GRIYA);
    expect(dashboard.tenantsInArrears).toBeGreaterThan(0);
    expect(dashboard.overdue).toBeGreaterThanOrEqual(account.overdue);
    await expectDashboard(page, dashboard);

    // The correction just made is the newest, unless ten more came in meanwhile.
    const made = dashboard.recentCorrections.find(
      (correction) => correction.reason === reason,
    );
    if (made !== undefined) {
      const row = page
        .getByRole("list", { name: "Daftar koreksi terbaru" })
        .locator(":scope > li")
        .filter({ hasText: reason });
      await expect(row).toContainText(tenant.name);
      await expect(row).toContainText(formatSignedRupiah(-25_000));
      await expect(row).toContainText(`oleh ${HENDRA.name}`);
    }

    // Text sizes from CLAUDE.md "UX rules".
    const min = MIN_BODY_FONT_PX[testInfo.project.name] ?? 16;
    expect(
      await fontSize(page, formatLongDate(jakartaToday(new Date()))),
    ).toBeGreaterThanOrEqual(min);
    const firstArrears = dashboard.arrears[0];
    if (firstArrears !== undefined) {
      expect(
        await fontSize(page, `Terlambat ${firstArrears.ageDays} hari`),
      ).toBeGreaterThanOrEqual(min);
    }
    await attachFullPageScreenshot(page, testInfo, "dashboard");

    // A name in "Belum bayar" opens Detail penyewa.
    if (firstArrears !== undefined) {
      const link = page
        .getByRole("list", { name: "Penyewa yang belum bayar" })
        .getByRole("link", { name: firstArrears.tenantName });
      const box = await link.boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
      await link.click();
      await expect(page).toHaveURL(
        new RegExp(`/tenants/${firstArrears.tenantId}$`),
      );
      await expect(
        page.getByRole("heading", { level: 1, name: firstArrears.tenantName }),
      ).toBeVisible();
    }
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("reads the summary with nothing to press", async ({
    page,
  }, testInfo) => {
    const dashboard = await openHome(page, HENDRA, ANGGREK);
    await expectDashboard(page, dashboard);
    await expect(page.getByRole("main").getByRole("button")).toHaveCount(0);
    await attachFullPageScreenshot(page, testInfo, "dashboard-supervisor");
  });
});

test.describe("Karyawan", () => {
  test.use({ storageState: RUDI.storageState });

  test("is greeted without the money, and nothing asks for it", async ({
    page,
  }) => {
    const asked: string[] = [];
    page.on("request", (request) => {
      if (/\/dashboard(\?|$)/.test(request.url())) asked.push(request.url());
    });
    await page.goto("/organizations");
    await expect(page).toHaveURL(ORG_HOME_URL);
    await expect(
      page.getByRole("heading", { level: 1, name: HOME_HEADING }),
    ).toBeVisible();
    await expect(
      page.getByText("Pilih menu untuk mulai bekerja."),
    ).toBeVisible();
    await expect(page.getByRole("region", { name: REGION })).toHaveCount(0);
    expect(asked).toEqual([]);
  });
});
