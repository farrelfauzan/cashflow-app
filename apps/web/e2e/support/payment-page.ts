import { randomUUID } from "node:crypto";

import {
  expect,
  request as playwrightRequest,
  type APIRequestContext,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import type {
  MeResponse,
  PaymentLink,
  PaymentPage,
  PaymentResult,
  Tenant,
  TenantAccount,
  Unit,
} from "../../src/lib/api/generated/model";
import { jakartaToday } from "../../src/lib/format/date";
import { formatRupiah } from "../../src/lib/format/rupiah";
import { findOrCreateProperty } from "./property-fixtures";
import type { SeededStaff } from "./staff";

// Helpers of the tenant payment page specs (payment-page.spec.ts, payment-page-qris.spec.ts).
// The tenant has no account: the page opens in a browser with no session at all, and staff set
// up its data through the API with their own sessions.

export const GRIYA = "Griya Sejahtera";
export const BULANAN = "Kos Bulanan Sejahtera";

// seed-demo's bank account for Griya Sejahtera (services/api/internal/demoseed/data.go).
export const GRIYA_BANK = {
  bankName: "BCA",
  accountNumber: "0123 456 789",
  accountHolder: "Ani Rahmawati",
};

export function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

// "1" for the iPad project, "2" for the phone one.
export function projectDigit(testInfo: TestInfo): string {
  return testInfo.project.name === "phone" ? "2" : "1";
}

// A name no other run, project or retry uses.
export function uniqueName(testInfo: TestInfo, base: string): string {
  const project = testInfo.project.name === "phone" ? "Dua" : "Satu";
  const retry = testInfo.retry === 0 ? "" : ` r${testInfo.retry}`;
  return `${base} ${runId()} ${project}${retry}`;
}

// A tenant number no other run, project, test or retry uses. Each spec has its own prefix
// ("0814-8": payment-page.spec.ts, "0815-1": payment-page-qris.spec.ts,
// "0815-2": payment-page-partial.spec.ts, "0815-5": payment-page-telegram.spec.ts), used by no
// other spec.
export function tenantNumber(
  testInfo: TestInfo,
  prefix: string,
  testDigit: number,
): string {
  const run = runId();
  return `${prefix}${run.slice(0, 3)}-${run.slice(3)}${projectDigit(testInfo)}${testDigit}${testInfo.retry}`;
}

// "YYYY-MM-01" of this month and of earlier months, in Jakarta.
export function firstOfMonth(monthsAgo: number): string {
  const [year = 0, month = 1] = jakartaToday(new Date()).split("-").map(Number);
  return new Date(Date.UTC(year, month - 1 - monthsAgo, 1))
    .toISOString()
    .slice(0, 10);
}

// A staff member's API session, for setup only; the page itself never has one.
export async function staffApi(
  staff: SeededStaff,
  testInfo: TestInfo,
): Promise<APIRequestContext> {
  return playwrightRequest.newContext({
    baseURL: testInfo.project.use.baseURL,
    storageState: staff.storageState,
  });
}

export async function call<T>(
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

export async function orgIdByName(
  api: APIRequestContext,
  name: string,
): Promise<string> {
  const me = await call<MeResponse>(api, "GET", "/me", 200);
  const org = me.organizations.find((each) => each.name === name);
  if (org === undefined) throw new Error(`not a member of ${name}`);
  return org.id;
}

// The path of a new link for the invoice, /t/<token>, and its token.
export async function makeLink(
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

export type RentedTenant = {
  orgId: string;
  tenant: Tenant;
  account: TenantAccount;
  accountPath: string;
};

export type RentOptions = {
  // The test's digit in the tenant number.
  digit: number;
  // The tenant number's prefix of the spec (see tenantNumber).
  phonePrefix: string;
  // The kos room's monthly price.
  price: number;
  // The start of the room's label, distinct per spec: the specs share their property.
  unitBase: string;
};

// A tenant of the test's own in Griya Sejahtera, renting a kos room monthly since the 1st of
// last month: the API bills last month (now debt) and this month. Polled until the worker has
// made both invoices.
export async function rentSinceLastMonth(
  api: APIRequestContext,
  testInfo: TestInfo,
  options: RentOptions,
): Promise<RentedTenant> {
  const orgId = await orgIdByName(api, GRIYA);
  const tenant = await call<Tenant>(
    api,
    "POST",
    `/orgs/${orgId}/tenants`,
    201,
    {
      // Starts with W, so it sorts after every seeded tenant and never pushes one off the first
      // page of the tenant list (tenants.spec.ts counts the seeded rows there).
      name: uniqueName(testInfo, "Wulan Halaman Bayar"),
      phone: tenantNumber(testInfo, options.phonePrefix, options.digit),
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
      label: uniqueName(testInfo, `${options.unitBase} ${options.digit}`),
      terms: [{ months: 1, price: options.price }],
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

export async function recordCash(
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

export function thisMonthInvoiceId(account: TenantAccount): string {
  const month = firstOfMonth(0).slice(0, 7);
  const invoice = account.invoices.find((each) =>
    each.period.startsWith(month),
  );
  if (invoice === undefined) throw new Error("no invoice for this month");
  return invoice.id;
}

// The page as the API gives it to the tenant: no session, like the browser.
export async function readPage(page: Page, path: string): Promise<PaymentPage> {
  const response = await page.request.get(`/api${path}`);
  expect(response.status()).toBe(200);
  return (await response.json()) as PaymentPage;
}

// Every request the page makes that carries the token where it does not belong. It belongs only
// in the paths of the page document and of the page's own API calls: the page read, the QR
// status, making a QR, the QR image, "Saya sudah bayar", the "Bayar sebagian" preview and
// "Simpan janji". Never in a query string, a referer or a body. Paths are logged with the token
// masked.
export function trackTokenLeaks(
  page: Page,
  path: string,
  token: string,
): string[] {
  const api = `/api${path}`;
  const allowed = new Set([
    `GET ${api}`,
    `GET ${api}/status`,
    `POST ${api}/qris`,
    `GET ${api}/qris.png`,
    `POST ${api}/check`,
    `GET ${api}/preview`,
    `POST ${api}/promise`,
    `POST ${api}/telegram`,
  ]);
  const leaks: string[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    const isDocument =
      request.resourceType() === "document" && url.pathname === path;
    const isApiCall = allowed.has(`${request.method()} ${url.pathname}`);
    const where: string[] = [];
    if (`${url.search}${url.hash}`.includes(token)) where.push("query");
    if (!isDocument && !isApiCall && url.pathname.includes(token)) {
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

export async function expectTouchTarget(locator: Locator, min = 44) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(min);
}

// A client address of the test's own, sent as X-Forwarded-For by the test's browser. The API
// limits public payment page requests per client address (120 a minute), and every test reaches
// it from this machine: with one address, the QRIS polling of tests running side by side would
// spend that budget and answer 429 to unrelated tests. Each test plays one tenant's phone, so it
// gets its own address in 198.18.0.0/15 (reserved for testing), one /16 per spec. The Next.js
// /api rewrite passes the header on; behind Caddy in production the API reads Caddy's.
export function tenantClientAddress(
  testInfo: TestInfo,
  spec: 18 | 19 | 20 | 21,
): string {
  const key = `${runId()}|${testInfo.project.name}|${testInfo.titlePath.join("|")}|${testInfo.retry}`;
  let hash = 0;
  for (const char of key) hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
  return `198.${spec}.${(hash >>> 8) & 0xff}.${hash & 0xff}`;
}

// Bayar-Pilih: keeps "Bayar semua" and presses "Lanjut bayar Rp…", which opens how to pay it.
export async function payAll(page: Page, outstanding: number): Promise<void> {
  const next = page.getByRole("button", {
    name: `Lanjut bayar ${formatRupiah(outstanding)}`,
  });
  await expectTouchTarget(next);
  await next.click();
}
