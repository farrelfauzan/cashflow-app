import {
  expect,
  test,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import type {
  Lease,
  PaymentResult,
  Tenant,
  TenantAccount,
  TenantList,
  Unit,
} from "../src/lib/api/generated/model";
import {
  formatDate,
  formatMonth,
  formatMonthYear,
  jakartaToday,
} from "../src/lib/format/date";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { findOrCreateProperty } from "./support/property-fixtures";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA, RUDI, SARI, type SeededStaff } from "./support/staff";

// The money section of Detail penyewa ("Uang penyewa"), read-only, against the real API and its
// worker. seed-demo makes no invoices, so the admin test makes its own: a kos unit, a tenant and
// a lease entered late (started on the 10th two months ago), which the API invoices for every
// period since its start; then a cash payment of the oldest invoice plus Rp200.000 (FIFO pays the
// oldest in full and part of the next) and a correction of the newest invoice. Every figure the
// page must show is read from the account the API returns, never hard-coded. Pak Hendra is Admin
// of the whole of Griya Sejahtera and Supervisor in Kost Putri Anggrek; Mbak Sari is Admin limited
// to Kos Melati; Mas Rudi is Karyawan in Kost Putri Anggrek, without payment:read.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const REGION = "Uang penyewa";
// "Catat janji" is built (e2e/tenant-promise.spec.ts); these two are not yet.
const LATER_BUTTONS = /^(Kirim link bayar|Koreksi nominal)$/;
const MINUS = "−";

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

// A tenant number no other run, project or retry uses; 0814-1 is used by no other spec.
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = testInfo.project.name === "phone" ? "2" : "1";
  return `0814-1${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
}

// The 10th of the month `months` from today in Jakarta (every month has one).
function tenthOfMonth(months: number): string {
  const [year = 0, month = 1] = jakartaToday(new Date()).split("-").map(Number);
  return new Date(Date.UTC(year, month - 1 + months, 10))
    .toISOString()
    .slice(0, 10);
}

function orgIdOf(page: Page): string {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

// Opens the organization's tenants page: straight in for one organization, by name for several.
async function openTenants(page: Page, staff: SeededStaff, name: string) {
  await page.goto("/organizations");
  if (staff.organizations.length > 1) {
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name })
      .click();
  }
  await expect(page).toHaveURL(ORG_HOME_URL);
  await page.goto(`${page.url()}/tenants`);
  await expect(
    page.getByRole("heading", { level: 1, name: "Penyewa" }),
  ).toBeVisible();
  const list = page.getByRole("list", { name: "Daftar penyewa" });
  await expect(list.getByRole("listitem").first()).toBeVisible();
  return list;
}

async function openTenant(page: Page, list: Locator, name: string) {
  await list
    .getByRole("listitem")
    .filter({ hasText: name })
    .getByRole("link")
    .first()
    .click();
  await expect(page).toHaveURL(/\/tenants\/[0-9a-f-]{36}$/);
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
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

// The direct rows of a list, without the lists nested in them (invoice lines, allocations).
function rows(list: Locator): Locator {
  return list.locator(":scope > li");
}

function rowTitled(list: Locator, title: string): Locator {
  return rows(list).filter({
    has: list.page().getByRole("heading", { level: 3, name: title }),
  });
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees the totals, invoices, payments with their allocations and corrections", async ({
    page,
  }, testInfo) => {
    test.setTimeout(90_000);
    const outsideRequests = trackOutsideRequests(page);
    await openTenants(page, HENDRA, GRIYA);
    const orgId = orgIdOf(page);
    const today = jakartaToday(new Date());

    // A kos unit of the test's own, rented by the month, and a lease entered late.
    const property = await findOrCreateProperty(page, orgId, testInfo, {
      base: "Tagihan",
      kind: "kos",
    });
    const unit = await call<Unit>(
      page,
      "POST",
      `/orgs/${orgId}/properties/${property.id}/units`,
      201,
      {
        label: uniqueName(testInfo, "Kamar Tagihan"),
        terms: [{ months: 1, price: 1_100_000 }],
      },
    );
    const tenant = await call<Tenant>(
      page,
      "POST",
      `/orgs/${orgId}/tenants`,
      201,
      {
        name: uniqueName(testInfo, "Yulia Kartika"),
        phone: tenantNumber(testInfo, 1),
        note: "",
      },
    );
    await call<Lease>(page, "POST", `/orgs/${orgId}/leases`, 201, {
      tenantId: tenant.id,
      unitId: unit.id,
      startDate: tenthOfMonth(-2),
      termMonths: 1,
      depositAmount: 1_100_000,
    });
    const accountPath = `/orgs/${orgId}/tenants/${tenant.id}/account`;

    // Every period since the start is invoiced, by the API or its worker, within seconds.
    let account = await call<TenantAccount>(page, "GET", accountPath, 200);
    await expect
      .poll(
        async () => {
          account = await call<TenantAccount>(page, "GET", accountPath, 200);
          return account.invoices.length;
        },
        { timeout: 20_000 },
      )
      .toBeGreaterThanOrEqual(2);

    const [oldest] = account.invoices;
    if (oldest === undefined) throw new Error("no invoice");
    const result = await call<PaymentResult>(
      page,
      "POST",
      `/orgs/${orgId}/payments`,
      201,
      {
        requestId: crypto.randomUUID(),
        tenantId: tenant.id,
        amount: oldest.billed + 200_000,
        method: "cash",
        receivedOn: today,
        note: "Titip ke Mas Dimas",
      },
    );
    const newest = account.invoices.at(-1);
    if (newest === undefined) throw new Error("no invoice");
    await call(page, "POST", `/orgs/${orgId}/corrections`, 201, {
      requestId: crypto.randomUUID(),
      invoiceId: newest.id,
      amountDelta: -50_000,
      reason: "Potongan kran rusak",
    });
    account = await call<TenantAccount>(page, "GET", accountPath, 200);

    const [first, second] = account.invoices;
    const latest = account.invoices.at(-1);
    const paid = account.payments.find(
      (payment) => payment.id === result.payment.id,
    );
    if (
      first === undefined ||
      second === undefined ||
      latest === undefined ||
      paid === undefined
    ) {
      throw new Error("the account lost an invoice or the payment");
    }
    // FIFO: the oldest is paid in full, the next only in part.
    expect(first.status).toBe("paid");
    expect(paid.allocations.map((each) => each.invoiceId)).toEqual([
      first.id,
      second.id,
    ]);

    // The tenant's page, as an admin opens it after choosing the organization.
    await page.goto(`/o/${orgId}/tenants/${tenant.id}`);
    await expect(
      page.getByRole("heading", { level: 1, name: tenant.name }),
    ).toBeVisible();

    const region = page.getByRole("region", { name: REGION });
    await expect(region).toBeVisible();

    // The tiles, straight from the totals.
    const debt = region.getByRole("group", { name: "Tunggakan", exact: true });
    await expect(debt).toContainText(formatRupiah(account.overdue));
    if (account.overdue === 0) {
      await expect(debt).toContainText("Tidak ada tunggakan");
      await expect(
        region.getByRole("group", { name: "Tunggakan tertua" }),
      ).toContainText("—");
    }
    if (account.outstanding > account.overdue) {
      await expect(debt).toContainText(
        `Total belum dibayar ${formatRupiah(account.outstanding)}`,
      );
    }
    await expect(
      region.getByRole("group", { name: "Saldo lebih bayar" }),
    ).toContainText(formatRupiah(account.credit));
    await expect(
      region.getByRole("group", { name: "Saldo lebih bayar" }),
    ).toContainText("Dipakai otomatis untuk tagihan berikutnya");

    // "Tagihan", newest period first.
    await expect(
      region.getByRole("heading", { level: 2, name: "Tagihan" }),
    ).toBeVisible();
    const invoices = region.getByRole("list", { name: "Daftar tagihan" });
    await expect(rows(invoices)).toHaveCount(account.invoices.length);
    await expect(rows(invoices).first()).toContainText(
      formatMonthYear(latest.period),
    );
    await expect(rows(invoices).last()).toContainText(
      formatMonthYear(first.period),
    );
    const firstRow = rowTitled(invoices, formatMonthYear(first.period));
    await expect(firstRow).toContainText("Lunas");
    await expect(firstRow).toContainText("1 pembayaran");
    await expect(firstRow).toContainText(formatRupiah(first.billed));
    const secondRow = rowTitled(invoices, formatMonthYear(second.period));
    await expect(secondRow).toContainText(/Sebagian|Terlambat \d+ hari/);
    await expect(secondRow).toContainText(
      `Sisa ${formatRupiah(second.outstanding)}`,
    );
    await expect(secondRow).toContainText(
      `dibayar ${formatRupiah(second.paid)}`,
    );
    // The corrected invoice shows what it bills now and what it first billed.
    const latestRow = rowTitled(invoices, formatMonthYear(latest.period));
    await expect(latestRow).toContainText(formatRupiah(latest.billed));
    await expect(latestRow).toContainText(
      `dikoreksi dari ${formatRupiah(latest.amount)}`,
    );
    // Its lines, as snapshotted.
    for (const line of first.lines) {
      await expect(firstRow).toContainText(line.label);
      await expect(firstRow).toContainText(formatRupiah(line.amount));
    }

    // "Pembayaran dan alokasi": the payment and the invoices it paid.
    await expect(
      region.getByRole("heading", { level: 2, name: "Pembayaran dan alokasi" }),
    ).toBeVisible();
    const payments = region.getByRole("list", { name: "Daftar pembayaran" });
    const paymentRow = rowTitled(payments, `${formatDate(today)} · Tunai`);
    await expect(paymentRow).toContainText(formatRupiah(paid.net));
    await expect(paymentRow).toContainText("Titip ke Mas Dimas");
    await expect(paymentRow).toContainText(`dicatat ${HENDRA.name}`);
    const [toFirst, toSecond] = paid.allocations;
    if (toFirst === undefined || toSecond === undefined) {
      throw new Error("the payment has no allocations");
    }
    const allocations = paymentRow.getByRole("listitem");
    await expect(allocations).toHaveText([
      `→ ${formatMonth(first.period)} ${formatRupiah(toFirst.amount)} (lunas)`,
      `→ ${formatMonth(second.period)} ${formatRupiah(toSecond.amount)}`,
    ]);

    // "Koreksi nominal".
    await expect(
      region.getByRole("heading", { level: 2, name: "Koreksi nominal" }),
    ).toBeVisible();
    const corrections = region.getByRole("list", { name: "Daftar koreksi" });
    await expect(rows(corrections)).toHaveCount(1);
    await expect(rows(corrections).first()).toContainText(
      `Tagihan ${formatMonthYear(latest.period)}`,
    );
    await expect(rows(corrections).first()).toContainText(`${MINUS}Rp50.000`);
    await expect(rows(corrections).first()).toContainText(
      "Potongan kran rusak",
    );
    await expect(rows(corrections).first()).toContainText(
      `oleh ${HENDRA.name}`,
    );

    // None of the board's later buttons; the only thing to press in the section is "Koreksi" on
    // each invoice and payment (e2e/correction.spec.ts covers its dialog).
    await expect(page.getByRole("button", { name: LATER_BUTTONS })).toHaveCount(
      0,
    );
    await expect(region.getByRole("button")).toHaveCount(
      account.invoices.length + account.payments.length,
    );
    await expect(
      region.getByRole("button", { name: /^Koreksi (tagihan|pembayaran) / }),
    ).toHaveCount(account.invoices.length + account.payments.length);
    // The lease card still follows.
    await expect(
      page.getByRole("region", { name: `${unit.label} · ${property.name}` }),
    ).toBeVisible();

    const minFont = MIN_BODY_FONT_PX[testInfo.project.name] ?? 16;
    expect(
      await fontSize(paymentRow.getByText("Titip ke Mas Dimas")),
    ).toBeGreaterThanOrEqual(minFont);
    expect(
      await fontSize(firstRow.getByText("1 pembayaran")),
    ).toBeGreaterThanOrEqual(minFont);

    // From the top of the page, so the screenshot shows the header and the tiles.
    await page.evaluate(() => window.scrollTo(0, 0));
    await attachViewportScreenshot(page, testInfo, "tenant-account");
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Admin limited to one property", () => {
  test.use({ storageState: SARI.storageState });

  test("is told she cannot see the money of a tenant with a lease elsewhere", async ({
    page,
  }) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openTenants(page, SARI, GRIYA);
    // Rina's second lease is in Kontrakan Mawar, outside Kos Melati: the API answers 403.
    await openTenant(page, list, "Rina Wulandari");
    await expect(
      page.getByText("Anda tidak bisa melihat tagihan penyewa ini."),
    ).toBeVisible();
    await expect(page.getByRole("region", { name: REGION })).toHaveCount(0);
    await expect(
      page.getByRole("region", { name: /Kamar 1 · Kos Melati/ }),
    ).toBeVisible();
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Karyawan", () => {
  test.use({ storageState: RUDI.storageState });

  test("sees no money section without payment:read, and nothing asks for it", async ({
    page,
  }) => {
    const outsideRequests = trackOutsideRequests(page);
    const accountRequests: string[] = [];
    page.on("request", (request) => {
      if (/\/tenants\/[0-9a-f-]{36}\/account/.test(request.url())) {
        accountRequests.push(request.url());
      }
    });
    const list = await openTenants(page, RUDI, ANGGREK);
    await openTenant(page, list, "Ayu Lestari");
    await expect(page.getByRole("region").first()).toBeVisible();

    await expect(page.getByRole("region", { name: REGION })).toHaveCount(0);
    await expect(page.getByText("Memuat tagihan…")).toHaveCount(0);
    await expect(
      page.getByText("Anda tidak bisa melihat tagihan penyewa ini."),
    ).toHaveCount(0);
    expect(accountRequests).toEqual([]);
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("reads the money section with nothing to press", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openTenants(page, HENDRA, ANGGREK);
    await openTenant(page, list, "Nadia Putri");
    const orgId = orgIdOf(page);
    const { items } = await call<TenantList>(
      page,
      "GET",
      `/orgs/${orgId}/tenants?q=${encodeURIComponent("Nadia Putri")}`,
      200,
    );
    const nadia = items.find((each) => each.name === "Nadia Putri");
    if (nadia === undefined) throw new Error("Nadia Putri is not seeded");
    const account = await call<TenantAccount>(
      page,
      "GET",
      `/orgs/${orgId}/tenants/${nadia.id}/account`,
      200,
    );

    const region = page.getByRole("region", { name: REGION });
    await expect(region).toBeVisible();
    await expect(
      region.getByRole("group", { name: "Tunggakan", exact: true }),
    ).toContainText(formatRupiah(account.overdue));
    await expect(
      region.getByRole("group", { name: "Saldo lebih bayar" }),
    ).toContainText(formatRupiah(account.credit));
    for (const name of [
      "Tagihan",
      "Pembayaran dan alokasi",
      "Koreksi nominal",
    ]) {
      await expect(region.getByRole("heading", { level: 2, name })).toHaveCount(
        1,
      );
    }
    if (account.invoices.length === 0) {
      await expect(region).toContainText("Belum ada tagihan.");
    }
    if (account.payments.length === 0) {
      await expect(region).toContainText("Belum ada pembayaran.");
    }
    if (
      account.invoices.every((each) => each.corrections.length === 0) &&
      account.payments.every((each) => each.corrections.length === 0)
    ) {
      await expect(region).toContainText("Koreksi nominal: belum ada.");
    }

    await expect(region.getByRole("button")).toHaveCount(0);
    await expect(
      page.getByRole("button", {
        name: /Ubah penyewa|Sewakan unit|Kirim link bayar|Catat janji|Koreksi nominal/,
      }),
    ).toHaveCount(0);

    // From the top of the page, so the screenshot shows the header and the tiles.
    await page.evaluate(() => window.scrollTo(0, 0));
    await attachViewportScreenshot(page, testInfo, "tenant-account-supervisor");
    expect(outsideRequests).toEqual([]);
  });
});
