import { randomBytes, randomUUID } from "node:crypto";

import {
  expect,
  request as playwrightRequest,
  test,
  type APIRequestContext,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import type {
  InvoiceList,
  MeResponse,
  PaymentLink,
  PaymentPage,
  PaymentResult,
  Tenant,
  TenantAccount,
  Unit,
} from "../src/lib/api/generated/model";
import { formatMonthYear, jakartaToday } from "../src/lib/format/date";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { findOrCreateProperty } from "./support/property-fixtures";
import { DIMAS, HENDRA, type SeededStaff } from "./support/staff";

// The tenant payment page /t/[token] (W15), read-only states, against the real API and its
// worker. The tenant has no account: the page opens in a browser with no session at all, and
// staff set up its data through the API with their own sessions. Pak Hendra is Admin of the
// whole of Griya Sejahtera, which seed-demo gives a bank account; Pak Dimas is Admin of Kos
// Bulanan Sejahtera, which has none. Every amount the page must show is read from the API.

const GRIYA = "Griya Sejahtera";
const BULANAN = "Kos Bulanan Sejahtera";
// seed-demo's bank account for Griya Sejahtera (services/api/internal/demoseed/data.go).
const GRIYA_BANK = {
  bankName: "BCA",
  accountNumber: "0123 456 789",
  accountHolder: "Ani Rahmawati",
};

// Minimum body text per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };

// The tenant: no cookies, no storage.
test.use({ storageState: { cookies: [], origins: [] } });

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

// A tenant number no other run, project, test or retry uses; 0814-8 is used by no other spec.
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = testInfo.project.name === "phone" ? "2" : "1";
  return `0814-8${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
}

// "YYYY-MM-01" of this month and of last month, in Jakarta.
function firstOfMonth(monthsAgo: number): string {
  const [year = 0, month = 1] = jakartaToday(new Date()).split("-").map(Number);
  return new Date(Date.UTC(year, month - 1 - monthsAgo, 1))
    .toISOString()
    .slice(0, 10);
}

// A staff member's API session, for setup only; the page itself never has one.
async function staffApi(
  staff: SeededStaff,
  testInfo: TestInfo,
): Promise<APIRequestContext> {
  const api = await playwrightRequest.newContext({
    baseURL: testInfo.project.use.baseURL,
    storageState: staff.storageState,
  });
  return api;
}

async function call<T>(
  api: APIRequestContext,
  method: "GET" | "POST",
  path: string,
  want: number,
  data?: object,
): Promise<T> {
  const response = await api.fetch(`/api${path}`, { method, data });
  expect(response.status(), await response.text()).toBe(want);
  return (await response.json()) as T;
}

async function orgIdByName(api: APIRequestContext, name: string) {
  const me = await call<MeResponse>(api, "GET", "/me", 200);
  const org = me.organizations.find((each) => each.name === name);
  if (org === undefined) throw new Error(`not a member of ${name}`);
  return org.id;
}

// The path of a new link for the invoice, /t/<token>, and its token.
async function makeLink(
  api: APIRequestContext,
  orgId: string,
  invoiceId: string,
): Promise<{ path: string; token: string }> {
  const link = await call<PaymentLink>(
    api,
    "POST",
    `/orgs/${orgId}/invoices/${invoiceId}/payment-links`,
    201,
  );
  const path = new URL(link.url, "http://localhost").pathname;
  const token = /^\/t\/([A-Za-z0-9_-]{43})$/.exec(path)?.[1];
  if (token === undefined) throw new Error("the link is not /t/<token>");
  return { path, token };
}

type RentedTenant = {
  orgId: string;
  tenant: Tenant;
  account: TenantAccount;
  accountPath: string;
};

// A tenant of the test's own in Griya Sejahtera, renting a kos room monthly since the 1st of
// last month: the API bills last month (now debt) and this month. Polled until the worker has
// made both invoices.
async function rentSinceLastMonth(
  api: APIRequestContext,
  testInfo: TestInfo,
  digit: number,
): Promise<RentedTenant> {
  const orgId = await orgIdByName(api, GRIYA);
  const tenant = await call<Tenant>(
    api,
    "POST",
    `/orgs/${orgId}/tenants`,
    201,
    {
      name: uniqueName(testInfo, "Bu Ratna Halaman Bayar"),
      phone: tenantNumber(testInfo, digit),
      note: "",
    },
  );
  const property = await findOrCreateProperty(
    { request: api },
    orgId,
    testInfo,
    {
      base: "Halaman Bayar",
      kind: "kos",
    },
  );
  const unit = await call<Unit>(
    api,
    "POST",
    `/orgs/${orgId}/properties/${property.id}/units`,
    201,
    {
      label: uniqueName(testInfo, `Kamar Bayar ${digit}`),
      terms: [{ months: 1, price: 1_100_000 }],
    },
  );
  await call(api, "POST", `/orgs/${orgId}/leases`, 201, {
    tenantId: tenant.id,
    unitId: unit.id,
    startDate: firstOfMonth(1),
    termMonths: 1,
    depositAmount: 0,
  });
  const accountPath = `/orgs/${orgId}/tenants/${tenant.id}/account`;
  let account = await call<TenantAccount>(api, "GET", accountPath, 200);
  await expect
    .poll(
      async () => {
        account = await call<TenantAccount>(api, "GET", accountPath, 200);
        // Totals and invoices are separate reads: wait for both.
        return (
          account.invoices.length >= 2 &&
          account.outstanding >=
            account.invoices.reduce((sum, each) => sum + each.billed, 0)
        );
      },
      { timeout: 20_000 },
    )
    .toBe(true);
  return { orgId, tenant, account, accountPath };
}

async function recordCash(
  api: APIRequestContext,
  rented: RentedTenant,
  amount: number,
): Promise<void> {
  await call<PaymentResult>(
    api,
    "POST",
    `/orgs/${rented.orgId}/payments`,
    201,
    {
      requestId: randomUUID(),
      tenantId: rented.tenant.id,
      amount,
      method: "cash",
      receivedOn: jakartaToday(new Date()),
    },
  );
}

function thisMonthInvoiceId(account: TenantAccount): string {
  const month = firstOfMonth(0).slice(0, 7);
  const invoice = account.invoices.find((each) =>
    each.period.startsWith(month),
  );
  if (invoice === undefined) throw new Error("no invoice for this month");
  return invoice.id;
}

// The page as the API gives it to the tenant: no session, like the browser.
async function readPage(page: Page, path: string): Promise<PaymentPage> {
  const response = await page.request.get(`/api${path}`);
  expect(response.status()).toBe(200);
  return (await response.json()) as PaymentPage;
}

// Every request the page makes that carries the token anywhere other than the two places it
// belongs: the page document itself and its one API read. Paths are logged with the token
// masked.
function trackTokenLeaks(page: Page, path: string, token: string): string[] {
  const leaks: string[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    const isDocument =
      request.resourceType() === "document" && url.pathname === path;
    const isApiRead =
      url.pathname === `/api${path}` && request.method() === "GET";
    const where: string[] = [];
    if (!isDocument && !isApiRead && request.url().includes(token)) {
      where.push("url");
    }
    if ((request.headers()["referer"] ?? "").includes(token)) {
      where.push("referer");
    }
    if ((request.postData() ?? "").includes(token)) where.push("body");
    if (where.length > 0) {
      leaks.push(
        `${request.method()} ${url.pathname.replace(token, "<token>")} (${where.join(", ")})`,
      );
    }
  });
  return leaks;
}

async function expectTouchTarget(locator: Locator, min = 44) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(min);
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

function summaryRow(page: Page, label: string): Locator {
  return page
    .getByRole("region", { name: "Ringkasan tagihan" })
    .getByRole("listitem")
    .filter({ hasText: label });
}

test("shows what the tenant owes, oldest first, and how to transfer", async ({
  page,
}, testInfo) => {
  const api = await staffApi(HENDRA, testInfo);
  const rented = await rentSinceLastMonth(api, testInfo, 1);
  await recordCash(api, rented, 400_000);
  const { path, token } = await makeLink(
    api,
    rented.orgId,
    thisMonthInvoiceId(rented.account),
  );
  await api.dispose();

  const expected = await readPage(page, path);
  expect(expected.paid).toBe(false);
  expect(expected.bankAccount).toEqual(GRIYA_BANK);
  expect(expected.openInvoices).toHaveLength(2);
  const [lastMonth, thisMonth] = expected.openInvoices;
  if (lastMonth === undefined || thisMonth === undefined) throw new Error();

  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  const outside = trackOutsideRequests(page);
  const leaks = trackTokenLeaks(page, path, token);
  await page.goto(path);

  const lastMonthName = formatMonthYear(firstOfMonth(1));
  const thisMonthName = formatMonthYear(firstOfMonth(0));
  await expect(
    page.getByRole("heading", {
      level: 1,
      name: `Bayar tagihan ${thisMonthName}`,
      exact: true,
    }),
  ).toBeVisible();
  await expect(page.getByText(GRIYA, { exact: true })).toBeVisible();
  await expect(
    page.getByText(
      `${rented.tenant.name} · ${thisMonth.unitLabel} ${thisMonth.propertyName}`,
      { exact: true },
    ),
  ).toBeVisible();

  // Last month, partly paid by FIFO, first and marked; then this month in full.
  const older = summaryRow(page, `Sisa ${lastMonthName}`);
  await expect(older).toContainText("dari bulan lalu");
  await expect(older).toContainText(formatRupiah(lastMonth.balance));
  const current = summaryRow(page, `Tagihan ${thisMonthName}`);
  await expect(current).toContainText(formatRupiah(thisMonth.balance));
  await expect(current).not.toContainText("dari bulan lalu");
  const rows = page
    .getByRole("region", { name: "Ringkasan tagihan" })
    .getByRole("listitem");
  await expect(rows).toHaveCount(2);
  await expect(rows.first()).toContainText(`Sisa ${lastMonthName}`);

  const total = page
    .getByRole("region", { name: "Ringkasan tagihan" })
    .getByText("Total tagihan")
    .locator("..");
  await expect(total).toContainText(formatRupiah(expected.outstanding));
  await expect(page.getByText("Saldo lebih")).toHaveCount(0);
  const allocationNote = page.getByText(
    "Tanpa denda dan bunga. Uang yang masuk melunasi tagihan tertua dulu.",
  );
  await expect(allocationNote).toBeVisible();
  expect(await fontSize(allocationNote)).toBeGreaterThanOrEqual(
    MIN_BODY_FONT_PX[testInfo.project.name] ?? 16,
  );

  // The bank block, exactly as seed-demo set it.
  const transfer = page.getByRole("region", { name: "Transfer bank" });
  await expect(transfer.getByText("Transfer ke rekening")).toBeVisible();
  await expect(transfer.getByText("BCA", { exact: true })).toBeVisible();
  await expect(
    transfer.getByText(GRIYA_BANK.accountNumber, { exact: true }),
  ).toBeVisible();
  await expect(
    transfer.getByText(`a.n. ${GRIYA_BANK.accountHolder}`, { exact: true }),
  ).toBeVisible();
  await expect(
    transfer.getByText(
      `Tulis nama Anda di berita transfer: ${rented.tenant.name}`,
    ),
  ).toBeVisible();
  await expect(
    transfer.getByText("Boleh sebagian, tanpa minimum."),
  ).toBeVisible();
  await expect(
    transfer.getByText("Admin mencatat pembayaran setelah dana masuk."),
  ).toBeVisible();
  await expect(page.getByText("Cara bayar belum diatur")).toHaveCount(0);

  // The one primary action.
  const copy = page.getByRole("button", { name: "Salin nomor rekening" });
  await expect(page.getByRole("button")).toHaveCount(1);
  await expectTouchTarget(copy);
  await copy.click();
  await expect(page.getByRole("status")).toHaveText("Nomor rekening disalin");
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    GRIYA_BANK.accountNumber,
  );

  await expect(
    page.getByText("Pembayaran QRIS segera tersedia.", { exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByText("Halaman ini menunjukkan Lunas setelah pembayaran dicatat."),
  ).toBeVisible();

  await attachFullPageScreenshot(page, testInfo, "payment-page-summary");
  expect(outside).toEqual([]);
  expect(leaks).toEqual([]);
});

test("shows Lunas once everything is paid", async ({ page }, testInfo) => {
  const api = await staffApi(HENDRA, testInfo);
  const rented = await rentSinceLastMonth(api, testInfo, 2);
  const invoiceId = thisMonthInvoiceId(rented.account);
  const { path } = await makeLink(api, rented.orgId, invoiceId);
  // Everything the API says the tenant owes, in one payment.
  await recordCash(api, rented, rented.account.outstanding);
  await api.dispose();

  const expected = await readPage(page, path);
  expect(expected.paid).toBe(true);
  expect(expected.credit).toBe(0);

  await page.goto(path);
  const thisMonthName = formatMonthYear(firstOfMonth(0));
  await expect(
    page.getByRole("heading", {
      level: 1,
      name: `Bayar tagihan ${thisMonthName}`,
      exact: true,
    }),
  ).toBeVisible();
  const paid = page.getByRole("region", { name: "Lunas" });
  await expect(paid.getByText("Lunas", { exact: true })).toBeVisible();
  await expect(
    paid.getByText("Terima kasih, semua tagihan sudah lunas."),
  ).toBeVisible();
  const details = page.getByRole("region", { name: "Rincian" });
  await expect(details).toContainText(`Tagihan ${thisMonthName}`);
  await expect(details).toContainText(formatRupiah(expected.invoice.amount));
  await expect(page.getByText("Saldo lebih")).toHaveCount(0);
  // Nothing to pay, so no transfer block, no button and no summary.
  await expect(page.getByRole("button")).toHaveCount(0);
  await expect(page.getByText("Transfer bank")).toHaveCount(0);
  await expect(page.getByText("Total tagihan")).toHaveCount(0);

  await attachFullPageScreenshot(page, testInfo, "payment-page-paid");
});

test("an unknown or malformed link shows the same not-found state", async ({
  page,
}, testInfo) => {
  const apiReads: string[] = [];
  page.on("request", (request) => {
    if (new URL(request.url()).pathname.startsWith("/api/t/")) {
      apiReads.push(request.method());
    }
  });
  const title = page.getByRole("heading", {
    level: 1,
    name: "Link tidak berlaku",
    exact: true,
  });
  const text = page.getByText(
    "Link pembayaran ini tidak ditemukan atau sudah tidak berlaku. Minta link baru ke pemilik.",
  );

  // Well formed, never issued: the API answers 404.
  const unknown = randomBytes(32).toString("base64url");
  const answered = page.waitForResponse((response) =>
    new URL(response.url()).pathname.endsWith(`/api/t/${unknown}`),
  );
  await page.goto(`/t/${unknown}`);
  expect((await answered).status()).toBe(404);
  await expect(title).toBeVisible();
  await expect(text).toBeVisible();
  await expect(page.getByRole("button")).toHaveCount(0);
  await attachFullPageScreenshot(page, testInfo, "payment-page-not-found");
  expect(apiReads).toEqual(["GET"]);

  // Malformed: the same state, and nothing is sent.
  await page.goto("/t/abc");
  await expect(title).toBeVisible();
  await expect(text).toBeVisible();
  await page.waitForTimeout(500);
  expect(apiReads).toEqual(["GET"]);
});

test("a link opened too often shows Terlalu sering dibuka", async ({
  page,
}, testInfo) => {
  const api = await staffApi(HENDRA, testInfo);
  const orgId = await orgIdByName(api, GRIYA);
  const unpaid = await call<InvoiceList>(
    api,
    "GET",
    `/orgs/${orgId}/invoices?status=unpaid&pageSize=1`,
    200,
  );
  const invoice = unpaid.items[0];
  if (invoice === undefined) throw new Error("seed-demo has no unpaid invoice");
  const { path } = await makeLink(api, orgId, invoice.id);
  await api.dispose();

  // The limit is 30 reads a minute per token.
  for (let i = 0; i < 30; i += 1) {
    expect((await page.request.get(`/api${path}`)).status()).toBe(200);
  }
  await page.goto(path);
  await expect(
    page.getByRole("heading", {
      level: 1,
      name: "Terlalu sering dibuka",
      exact: true,
    }),
  ).toBeVisible();
  await expect(page.getByText("Coba lagi beberapa menit lagi.")).toBeVisible();
  const retry = page.getByRole("button", { name: "Coba lagi" });
  await expectTouchTarget(retry);
  await attachFullPageScreenshot(page, testInfo, "payment-page-rate-limited");

  // "Coba lagi" reads again, and the limit still holds.
  const again = page.waitForResponse((response) =>
    new URL(response.url()).pathname.endsWith(`/api${path}`),
  );
  await retry.click();
  expect((await again).status()).toBe(429);
  await expect(
    page.getByRole("heading", { level: 1, name: "Terlalu sering dibuka" }),
  ).toBeVisible();
});

test("tells the tenant whom to contact when no bank account is set", async ({
  page,
}, testInfo) => {
  // Kos Bulanan Sejahtera has no bank account. Its tenant, unit and property counts are pinned
  // by pagination.spec.ts, so this test only makes a link for a seeded unpaid invoice.
  const api = await staffApi(DIMAS, testInfo);
  const orgId = await orgIdByName(api, BULANAN);
  const unpaid = await call<InvoiceList>(
    api,
    "GET",
    `/orgs/${orgId}/invoices?status=unpaid&pageSize=1`,
    200,
  );
  const invoice = unpaid.items[0];
  if (invoice === undefined) throw new Error("seed-demo has no unpaid invoice");
  const { path } = await makeLink(api, orgId, invoice.id);
  await api.dispose();

  const expected = await readPage(page, path);
  expect(expected.paid).toBe(false);
  expect(expected.bankAccount).toBeNull();

  await page.goto(path);
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await expect(
    page.getByText(
      `Cara bayar belum diatur. Hubungi ${BULANAN} untuk membayar.`,
      { exact: true },
    ),
  ).toBeVisible();
  await expect(page.getByText("Total tagihan")).toBeVisible();
  await expect(
    page.getByRole("region", { name: "Ringkasan tagihan" }),
  ).toContainText(formatRupiah(expected.outstanding));
  await expect(page.getByRole("region", { name: "Transfer bank" })).toHaveCount(
    0,
  );
  await expect(page.getByRole("button")).toHaveCount(0);
  await expect(
    page.getByText("Pembayaran QRIS segera tersedia.", { exact: true }),
  ).toHaveCount(0);
});

test("the page is kept out of search engines and sends no referrer", async ({
  page,
}) => {
  // A malformed token: the page renders without asking the API anything.
  const response = await page.goto("/t/abc");
  expect(response?.status()).toBe(200);
  // The header, too: the meta tag comes too late for the requests made while <head> is read.
  expect(response?.headers()["referrer-policy"]).toBe("no-referrer");
  await expect(
    page.getByRole("heading", { level: 1, name: "Link tidak berlaku" }),
  ).toBeVisible();

  await expect(page).toHaveTitle("Bayar tagihan");
  await expect(page.locator('head meta[name="referrer"]')).toHaveAttribute(
    "content",
    "no-referrer",
  );
  const robots = page.locator('head meta[name="robots"]');
  await expect(robots).toHaveAttribute("content", /noindex/);
  await expect(robots).toHaveAttribute("content", /nofollow/);
  // Tenant pages are not part of the staff PWA.
  await expect(page.locator('link[rel="manifest"]')).toHaveCount(0);
});
