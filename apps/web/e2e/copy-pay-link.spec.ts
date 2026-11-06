import {
  expect,
  test,
  type Browser,
  type Locator,
  type Page,
  type Request,
  type TestInfo,
} from "@playwright/test";

import type {
  PaymentPage,
  Tenant,
  TenantAccount,
  TenantList,
  TenantOpenInvoice,
  Unit,
} from "../src/lib/api/generated/model";
import { formatMonthYear, jakartaToday } from "../src/lib/format/date";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { findOrCreateProperty } from "./support/property-fixtures";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA, RUDI } from "./support/staff";

// "Salin link bayar" on Detail penyewa, against the real API and its worker. Each press makes a
// new payment link (decision 39) for the tenant's newest open invoice (listTenantOpenInvoices)
// and copies it. Pak Hendra is Admin of the whole of Griya Sejahtera (invoice:share) and
// Supervisor in Kost Putri Anggrek (no invoice:share); Mas Rudi is Karyawan in Kost Putri Anggrek,
// with invoice:read and invoice:share on the tenants he collects (Ayu Lestari) and no
// payment:read. Tenant names start with W or Y, so they sort after every seeded tenant and
// the seeded rows keep the first page of the tenant list.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const BUTTON = "Salin link bayar";
const REGION = "Uang penyewa";
const COPIED = "Link bayar disalin. Tempel di WhatsApp penyewa.";
const FALLBACK = "Link bayar";
const LINK = /\/t\/([A-Za-z0-9_-]{43})$/;

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

// A tenant number no other run, project or retry uses; 0814-9 is used by no other spec.
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = testInfo.project.name === "phone" ? "2" : "1";
  return `0814-9${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
}

function firstOfThisMonth(): string {
  return `${jakartaToday(new Date()).slice(0, 7)}-01`;
}

function firstOfLastMonth(): string {
  const [year = 0, month = 1] = jakartaToday(new Date()).split("-").map(Number);
  return new Date(Date.UTC(year, month - 2, 1)).toISOString().slice(0, 10);
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

type Fixture = { tenant: Tenant; account: TenantAccount; accountPath: string };

// A tenant of the test's own renting a kos unit monthly from `startDate`, polled until the
// worker has made `invoices` invoices and the totals count them.
async function makeTenant(
  page: Page,
  testInfo: TestInfo,
  orgId: string,
  options: { digit: number; name: string; startDate: string; invoices: number },
): Promise<Fixture> {
  const tenant = await call<Tenant>(
    page,
    "POST",
    `/orgs/${orgId}/tenants`,
    201,
    {
      name: uniqueName(testInfo, options.name),
      phone: tenantNumber(testInfo, options.digit),
      note: "",
    },
  );
  const property = await findOrCreateProperty(page, orgId, testInfo, {
    base: "Link Bayar",
    kind: "kos",
  });
  const unit = await call<Unit>(
    page,
    "POST",
    `/orgs/${orgId}/properties/${property.id}/units`,
    201,
    {
      label: uniqueName(testInfo, `Kamar Link ${options.digit}`),
      terms: [{ months: 1, price: 950_000 }],
    },
  );
  await call(page, "POST", `/orgs/${orgId}/leases`, 201, {
    tenantId: tenant.id,
    unitId: unit.id,
    startDate: options.startDate,
    termMonths: 1,
    depositAmount: 950_000,
  });
  const accountPath = `/orgs/${orgId}/tenants/${tenant.id}/account`;
  let account = await call<TenantAccount>(page, "GET", accountPath, 200);
  await expect
    .poll(
      async () => {
        account = await call<TenantAccount>(page, "GET", accountPath, 200);
        return (
          account.invoices.length >= options.invoices && account.outstanding > 0
        );
      },
      { timeout: 20_000 },
    )
    .toBe(true);
  return { tenant, account, accountPath };
}

function isOpenInvoicesGet(request: Request): boolean {
  return (
    request.method() === "GET" &&
    /\/api\/orgs\/[^/]+\/tenants\/[^/]+\/open-invoices$/.test(request.url())
  );
}

// Opens Detail penyewa and waits for the account and the open invoices the button reads, so a
// missing button means hidden, not loading.
async function openTenantPage(page: Page, orgId: string, tenant: Tenant) {
  const openInvoices = page.waitForResponse((response) =>
    isOpenInvoicesGet(response.request()),
  );
  await page.goto(`/o/${orgId}/tenants/${tenant.id}`);
  expect((await openInvoices).status()).toBe(200);
  await expect(
    page.getByRole("heading", { level: 1, name: tenant.name }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("region", { name: REGION })
      .getByRole("list", { name: "Daftar tagihan" }),
  ).toBeVisible();
}

function isLinkPost(request: Request): boolean {
  return (
    request.method() === "POST" &&
    /\/api\/orgs\/[^/]+\/invoices\/[^/]+\/payment-links$/.test(request.url())
  );
}

async function readClipboard(page: Page): Promise<string> {
  return page.evaluate(() => navigator.clipboard.readText());
}

async function expectTouchTarget(locator: Locator, min = 44) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(min);
}

// The theme's primary background, read from a probe so the check follows the tokens.
async function primaryBackground(page: Page): Promise<string> {
  return page.evaluate(() => {
    const probe = document.createElement("div");
    probe.className = "bg-primary";
    document.body.append(probe);
    const color = getComputedStyle(probe).backgroundColor;
    probe.remove();
    return color;
  });
}

// What the link opens for a tenant: no session, no storage. The /t/ page itself comes with
// another branch; the API's read of the link is what it shows.
async function readAsTenant(
  browser: Browser,
  testInfo: TestInfo,
  token: string,
): Promise<PaymentPage> {
  const context = await browser.newContext({
    baseURL: testInfo.project.use.baseURL,
    storageState: { cookies: [], origins: [] },
  });
  try {
    const response = await context.request.get(`/api/t/${token}`);
    expect(response.status(), await response.text()).toBe(200);
    return (await response.json()) as PaymentPage;
  } finally {
    await context.close();
  }
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("copies a new link to the newest open invoice on each press", async ({
    page,
    browser,
  }, testInfo) => {
    test.setTimeout(90_000);
    await page
      .context()
      .grantPermissions(["clipboard-read", "clipboard-write"], {
        origin: testInfo.project.use.baseURL,
      });
    const outsideRequests = trackOutsideRequests(page);
    const posts: string[] = [];
    page.on("request", (request) => {
      if (isLinkPost(request)) posts.push(request.url());
    });
    const orgId = await openOrganization(page, GRIYA);
    // Started the 1st of last month: last month's invoice and this month's are both open.
    const { tenant, account } = await makeTenant(page, testInfo, orgId, {
      digit: 1,
      name: "Wartini Rahayu",
      startDate: firstOfLastMonth(),
      invoices: 2,
    });
    const thisMonth = account.invoices.find(
      (invoice) => invoice.period === firstOfThisMonth(),
    );
    if (thisMonth === undefined) throw new Error("no invoice this month");
    expect(
      account.invoices.filter((invoice) => invoice.outstanding > 0).length,
    ).toBeGreaterThanOrEqual(2);
    await openTenantPage(page, orgId, tenant);

    // An outline button next to "Ubah penyewa"; "Catat pembayaran" stays the one primary action.
    const button = page.getByRole("button", { name: BUTTON });
    await expect(button).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Ubah penyewa" }),
    ).toBeVisible();
    await expectTouchTarget(button);
    const primary = await primaryBackground(page);
    expect(
      await button.evaluate((el) => getComputedStyle(el).backgroundColor),
    ).not.toBe(primary);
    await attachViewportScreenshot(page, testInfo, "copy-pay-link-button");

    const first = page.waitForResponse((response) =>
      isLinkPost(response.request()),
    );
    await button.click();
    const firstResponse = await first;
    expect(firstResponse.status()).toBe(201);
    expect(firstResponse.url()).toContain(`/invoices/${thisMonth.id}/`);
    await expect(
      page.getByRole("status").filter({ hasText: COPIED }),
    ).toBeVisible();
    const firstUrl = await readClipboard(page);
    expect(firstUrl).toMatch(LINK);
    await attachViewportScreenshot(page, testInfo, "copy-pay-link-copied");

    // The link opens this month's invoice for someone without a session.
    const firstToken = LINK.exec(firstUrl)?.[1] ?? "";
    const opened = await readAsTenant(browser, testInfo, firstToken);
    expect(opened.invoice.id).toBe(thisMonth.id);
    // The page's heading, "Bayar tagihan <bulan ini>", names this period.
    expect(opened.invoice.period).toBe(firstOfThisMonth());
    expect(formatMonthYear(opened.invoice.period)).toBe(
      formatMonthYear(thisMonth.period),
    );
    expect(opened.tenantName).toBe(tenant.name);

    // Pressed again, twice in the same task: one new link, different from the first.
    const second = page.waitForResponse((response) =>
      isLinkPost(response.request()),
    );
    await button.evaluate((el: HTMLButtonElement) => {
      el.click();
      el.click();
    });
    expect((await second).status()).toBe(201);
    await expect(button).toBeEnabled();
    await expect(
      page.getByRole("status").filter({ hasText: COPIED }),
    ).toBeVisible();
    await expect.poll(() => readClipboard(page)).not.toBe(firstUrl);
    const secondUrl = await readClipboard(page);
    expect(secondUrl).toMatch(LINK);
    expect(posts).toHaveLength(2);
    // Nothing on the page shows either link.
    const content = await page.content();
    expect(content).not.toContain(firstToken);
    expect(content).not.toContain(LINK.exec(secondUrl)?.[1] ?? "");
    expect(outsideRequests).toEqual([]);
  });

  test("shows the link once to copy by hand when the browser cannot copy", async ({
    page,
  }, testInfo) => {
    test.setTimeout(90_000);
    await page.addInitScript(() => {
      for (const method of ["write", "writeText"]) {
        Object.defineProperty(navigator.clipboard, method, {
          configurable: true,
          value: () =>
            Promise.reject(new DOMException("Blocked", "NotAllowedError")),
        });
      }
    });
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const { tenant } = await makeTenant(page, testInfo, orgId, {
      digit: 2,
      name: "Yanto Saputra",
      startDate: firstOfThisMonth(),
      invoices: 1,
    });
    await openTenantPage(page, orgId, tenant);

    const answer = page.waitForResponse((response) =>
      isLinkPost(response.request()),
    );
    await page.getByRole("button", { name: BUTTON }).click();
    expect((await answer).status()).toBe(201);

    const fallback = page.getByRole("region", { name: FALLBACK, exact: true });
    await expect(fallback).toBeVisible();
    const field = fallback.getByRole("textbox", {
      name: FALLBACK,
      exact: true,
    });
    await expect(field).toHaveAttribute("readonly", "");
    await expect(field).toHaveValue(LINK);
    await expect(field).toBeFocused();
    const token = LINK.exec(await field.inputValue())?.[1] ?? "";
    await expect(page.getByText(COPIED)).toHaveCount(0);
    const copyAgain = fallback.getByRole("button", {
      name: "Salin",
      exact: true,
    });
    const close = fallback.getByRole("button", { name: "Tutup" });
    await expectTouchTarget(copyAgain);
    await expectTouchTarget(close);
    await attachViewportScreenshot(page, testInfo, "copy-pay-link-fallback");

    // Still refused: the link stays for copying by hand.
    await copyAgain.click();
    await expect(field).toHaveValue(LINK);

    await close.click();
    await expect(fallback).toHaveCount(0);
    expect(await page.content()).not.toContain(token);
    expect(outsideRequests).toEqual([]);
  });

  test("has no button when nothing is owed", async ({ page }, testInfo) => {
    test.setTimeout(90_000);
    const orgId = await openOrganization(page, GRIYA);
    const { tenant, account, accountPath } = await makeTenant(
      page,
      testInfo,
      orgId,
      {
        digit: 3,
        name: "Yusuf Ramadhan",
        startDate: firstOfThisMonth(),
        invoices: 1,
      },
    );
    await call(page, "POST", `/orgs/${orgId}/payments`, 201, {
      requestId: crypto.randomUUID(),
      tenantId: tenant.id,
      amount: account.outstanding,
      method: "cash",
      receivedOn: jakartaToday(new Date()),
      note: "",
    });
    const paid = await call<TenantAccount>(page, "GET", accountPath, 200);
    expect(paid.outstanding).toBe(0);

    await openTenantPage(page, orgId, tenant);
    await expect(
      page.getByRole("button", { name: "Ubah penyewa" }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: BUTTON })).toHaveCount(0);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees no Salin link bayar on a tenant with invoices", async ({
    page,
  }) => {
    const outsideRequests = trackOutsideRequests(page);
    const openInvoiceReads: string[] = [];
    page.on("request", (request) => {
      if (isOpenInvoicesGet(request)) openInvoiceReads.push(request.url());
    });
    const orgId = await openOrganization(page, ANGGREK);
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
    expect(account.invoices.length).toBeGreaterThan(0);
    await page.goto(`/o/${orgId}/tenants/${nadia.id}`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Nadia Putri" }),
    ).toBeVisible();
    await expect(
      page
        .getByRole("region", { name: REGION })
        .getByRole("list", { name: "Daftar tagihan" }),
    ).toBeVisible();

    await expect(page.getByRole("button", { name: BUTTON })).toHaveCount(0);
    // Without invoice:share the open invoices are never read.
    expect(openInvoiceReads).toEqual([]);
    expect(outsideRequests).toEqual([]);
  });
});

// Looks a seeded tenant up as Pak Hendra (Supervisor there, who may read every invoice): Mas
// Rudi cannot list tenants he does not collect. Returns the tenant's id and how many open
// invoices the organization has for them.
async function seededTenant(
  browser: Browser,
  testInfo: TestInfo,
  orgId: string,
  name: string,
): Promise<{ id: string; openInvoices: number }> {
  const context = await browser.newContext({
    baseURL: testInfo.project.use.baseURL,
    storageState: HENDRA.storageState,
  });
  try {
    const list = await context.request.get(
      `/api/orgs/${orgId}/tenants?q=${encodeURIComponent(name)}`,
    );
    expect(list.status(), await list.text()).toBe(200);
    const { items } = (await list.json()) as TenantList;
    const tenant = items.find((each) => each.name === name);
    if (tenant === undefined) throw new Error(`${name} is not seeded`);
    const open = await context.request.get(
      `/api/orgs/${orgId}/tenants/${tenant.id}/open-invoices`,
    );
    expect(open.status(), await open.text()).toBe(200);
    const invoices = (await open.json()) as TenantOpenInvoice[];
    return { id: tenant.id, openInvoices: invoices.length };
  } finally {
    await context.close();
  }
}

// Mas Rudi has one organization, so the picker goes straight in. Returns its id.
async function openOnlyOrganization(page: Page): Promise<string> {
  await page.goto("/organizations");
  await expect(page).toHaveURL(ORG_HOME_URL);
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

test.describe("Karyawan", () => {
  test.use({ storageState: RUDI.storageState });

  test("copies a link for a tenant he collects, without reading the account", async ({
    page,
    browser,
  }, testInfo) => {
    test.setTimeout(90_000);
    await page
      .context()
      .grantPermissions(["clipboard-read", "clipboard-write"], {
        origin: testInfo.project.use.baseURL,
      });
    const outsideRequests = trackOutsideRequests(page);
    const accountReads: string[] = [];
    page.on("request", (request) => {
      if (/\/tenants\/[0-9a-f-]{36}\/account/.test(request.url())) {
        accountReads.push(request.url());
      }
    });
    const orgId = await openOnlyOrganization(page);
    const ayu = await seededTenant(browser, testInfo, orgId, "Ayu Lestari");
    // What the button should share: Ayu's newest open invoice, as the API lists it to him.
    const open = await call<TenantOpenInvoice[]>(
      page,
      "GET",
      `/orgs/${orgId}/tenants/${ayu.id}/open-invoices`,
      200,
    );
    expect(open.length).toBeGreaterThan(0);
    const newest = open.reduce((latest, each) =>
      each.period >= latest.period ? each : latest,
    );

    const openInvoices = page.waitForResponse((response) =>
      isOpenInvoicesGet(response.request()),
    );
    await page.goto(`/o/${orgId}/tenants/${ayu.id}`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Ayu Lestari" }),
    ).toBeVisible();
    expect((await openInvoices).status()).toBe(200);
    // No money section without payment:read; the button stands on its own.
    await expect(page.getByRole("region", { name: REGION })).toHaveCount(0);
    const button = page.getByRole("button", { name: BUTTON });
    await expect(button).toBeVisible();
    await expectTouchTarget(button);
    await attachViewportScreenshot(page, testInfo, "copy-pay-link-karyawan");

    const answer = page.waitForResponse((response) =>
      isLinkPost(response.request()),
    );
    await button.click();
    const created = await answer;
    expect(created.status()).toBe(201);
    expect(created.url()).toContain(`/invoices/${newest.id}/`);
    await expect(
      page.getByRole("status").filter({ hasText: COPIED }),
    ).toBeVisible();
    const url = await readClipboard(page);
    expect(url).toMatch(LINK);
    const token = LINK.exec(url)?.[1] ?? "";
    const opened = await readAsTenant(browser, testInfo, token);
    expect(opened.invoice.id).toBe(newest.id);
    expect(opened.tenantName).toBe("Ayu Lestari");
    expect(await page.content()).not.toContain(token);
    await attachViewportScreenshot(
      page,
      testInfo,
      "copy-pay-link-karyawan-copied",
    );

    expect(accountReads).toEqual([]);
    expect(outsideRequests).toEqual([]);
  });

  test("has no button for a tenant he does not collect", async ({
    page,
    browser,
  }, testInfo) => {
    const orgId = await openOnlyOrganization(page);
    // Rina Wulandari owes money here and is collected by nobody.
    const rina = await seededTenant(browser, testInfo, orgId, "Rina Wulandari");
    expect(rina.openInvoices).toBeGreaterThan(0);
    // The API lists none of her invoices to him.
    const open = await call<TenantOpenInvoice[]>(
      page,
      "GET",
      `/orgs/${orgId}/tenants/${rina.id}/open-invoices`,
      200,
    );
    expect(open).toEqual([]);

    await page.goto(`/o/${orgId}/tenants/${rina.id}`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Penyewa tidak ditemukan" }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: BUTTON })).toHaveCount(0);
  });
});
