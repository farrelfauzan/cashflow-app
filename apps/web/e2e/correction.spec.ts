import {
  expect,
  test,
  type Locator,
  type Page,
  type Request,
  type TestInfo,
} from "@playwright/test";

import type {
  AccountInvoice,
  CorrectionInput,
  Lease,
  PaymentResult,
  Tenant,
  TenantAccount,
  TenantList,
  Unit,
} from "../src/lib/api/generated/model";
import {
  formatDate,
  formatMonthYear,
  jakartaToday,
} from "../src/lib/format/date";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  attachFullPageScreenshot,
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { minBodyFontPx, projectDigit, projectWord } from "./support/projects";
import { findOrCreateProperty } from "./support/property-fixtures";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA, type SeededStaff } from "./support/staff";

// "Koreksi nominal" on Detail penyewa, against the real API and its worker. Each admin test makes
// money of its own: a kos unit, a tenant and a lease that started on the 1st of this month, which
// the worker invoices within seconds; the payment test also records a cash payment. The dialog
// sends the new amount and a reason; the API takes the difference. Every amount checked is read
// from the account the API returns, never hard-coded. Pak Hendra is Admin of the whole of Griya
// Sejahtera and Supervisor in Kost Putri Anggrek, where he may not correct.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const REGION = "Uang penyewa";
const PRICE = 1_200_000;

const MIN_TOUCH_PX = 44;

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

// A tenant number no other run, project, test or retry uses; 0814-3 is used by no other spec.
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = projectDigit(testInfo);
  return `0814-3${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
}

// The first day of this month in Jakarta: a lease from it is invoiced at once.
function firstOfThisMonth(): string {
  return `${jakartaToday(new Date()).slice(0, 7)}-01`;
}

function orgIdOf(page: Page): string {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

// Opens the organization's home: straight in for one organization, by name for several.
async function openOrganization(page: Page, staff: SeededStaff, name: string) {
  await page.goto("/organizations");
  if (staff.organizations.length > 1) {
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name })
      .click();
  }
  await expect(page).toHaveURL(ORG_HOME_URL);
  return orgIdOf(page);
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

type Fixture = {
  orgId: string;
  tenant: Tenant;
  accountPath: string;
  invoice: AccountInvoice;
};

// A tenant of the test's own with one invoice: a kos unit rented by the month from the 1st.
async function tenantWithInvoice(
  page: Page,
  testInfo: TestInfo,
  testDigit: number,
  tenantBase: string,
): Promise<Fixture> {
  const orgId = await openOrganization(page, HENDRA, GRIYA);
  const property = await findOrCreateProperty(page, orgId, testInfo, {
    base: "Koreksi",
    kind: "kos",
  });
  const unit = await call<Unit>(
    page,
    "POST",
    `/orgs/${orgId}/properties/${property.id}/units`,
    201,
    {
      label: uniqueName(testInfo, `Kamar Koreksi ${testDigit}`),
      terms: [{ months: 1, price: PRICE }],
    },
  );
  const tenant = await call<Tenant>(
    page,
    "POST",
    `/orgs/${orgId}/tenants`,
    201,
    {
      name: uniqueName(testInfo, tenantBase),
      phone: tenantNumber(testInfo, testDigit),
      note: "",
    },
  );
  await call<Lease>(page, "POST", `/orgs/${orgId}/leases`, 201, {
    tenantId: tenant.id,
    unitId: unit.id,
    startDate: firstOfThisMonth(),
    termMonths: 1,
    depositAmount: PRICE,
  });
  const accountPath = `/orgs/${orgId}/tenants/${tenant.id}/account`;

  // The worker invoices the lease within seconds.
  let account = await call<TenantAccount>(page, "GET", accountPath, 200);
  await expect
    .poll(
      async () => {
        account = await call<TenantAccount>(page, "GET", accountPath, 200);
        return account.invoices.length;
      },
      { timeout: 20_000 },
    )
    .toBeGreaterThanOrEqual(1);
  const [invoice] = account.invoices;
  if (invoice === undefined) throw new Error("no invoice");
  return { orgId, tenant, accountPath, invoice };
}

async function openTenantPage(
  page: Page,
  orgId: string,
  tenant: Pick<Tenant, "id" | "name">,
) {
  await page.goto(`/o/${orgId}/tenants/${tenant.id}`);
  await expect(
    page.getByRole("heading", { level: 1, name: tenant.name }),
  ).toBeVisible();
  const region = page.getByRole("region", { name: REGION });
  await expect(region).toBeVisible();
  return region;
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

function isCorrectionPost(request: Request): boolean {
  return (
    request.method() === "POST" &&
    /\/api\/orgs\/[0-9a-f-]{36}\/corrections$/.test(request.url())
  );
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

async function height(locator: Locator): Promise<number> {
  const box = await locator.boundingBox();
  if (box === null) throw new Error("not visible");
  return box.height;
}

// The dialog's parts, by what a person reads.
function dialogParts(dialog: Locator) {
  return {
    amount: dialog.getByRole("textbox", { name: "Nominal baru" }),
    reason: dialog.getByRole("textbox", { name: "Alasan (wajib diisi)" }),
    save: dialog.getByRole("button", { name: /Simpan koreksi|Menyimpan…/ }),
    cancel: dialog.getByRole("button", { name: "Batal" }),
    close: dialog.getByRole("button", { name: "Tutup" }),
  };
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("corrects an invoice to a lower amount with a reason", async ({
    page,
  }, testInfo) => {
    test.setTimeout(90_000);
    const outsideRequests = trackOutsideRequests(page);
    const { orgId, tenant, accountPath, invoice } = await tenantWithInvoice(
      page,
      testInfo,
      1,
      "Wulan Prameswari",
    );
    const newAmount = invoice.billed - 50_000;
    const reason = uniqueName(testInfo, "Potongan kran rusak");
    const month = formatMonthYear(invoice.period);
    const title = `Koreksi tagihan ${month}`;

    const region = await openTenantPage(page, orgId, tenant);
    const invoices = region.getByRole("list", { name: "Daftar tagihan" });
    const row = rowTitled(invoices, month);
    const correct = row.getByRole("button", { name: title });
    await expect(correct).toHaveText("Koreksi");
    expect(await height(correct)).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
    await correct.click();

    const dialog = page.getByRole("dialog", { name: title });
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText(
      `${tenant.name} · ${invoice.unitLabel} · ${invoice.propertyName}`,
    );
    await expect(dialog).toContainText("Nominal sekarang");
    await expect(dialog).toContainText(formatRupiah(invoice.billed));
    await expect(dialog).toContainText(
      "Data lama tidak dihapus. Koreksi ini tercatat atas nama Anda dan muncul di laporan Supervisor.",
    );
    const parts = dialogParts(dialog);
    // "Nominal baru" takes the focus, large.
    await expect(parts.amount).toBeFocused();
    await expect(parts.amount).toHaveAttribute("inputmode", "numeric");
    expect(await fontSize(parts.amount)).toBe(20);
    for (const control of [parts.close, parts.cancel, parts.save]) {
      expect(await height(control)).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
    }
    expect(await height(parts.amount)).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
    const minFont = minBodyFontPx(testInfo);
    expect(
      await fontSize(
        dialog.getByText("Data lama tidak dihapus.", { exact: false }),
      ),
    ).toBeGreaterThanOrEqual(minFont);
    expect(
      await fontSize(dialog.getByText("Nominal sekarang")),
    ).toBeGreaterThanOrEqual(minFont);

    await parts.amount.fill(String(newAmount));
    await parts.reason.fill(reason);
    await attachViewportScreenshot(page, testInfo, "correction-dialog");

    const sent = page.waitForRequest(isCorrectionPost);
    await parts.save.click();
    const body = (await sent).postDataJSON() as CorrectionInput;
    expect(body).toMatchObject({
      invoiceId: invoice.id,
      newAmount,
      reason,
    });
    expect(body).not.toHaveProperty("amountDelta");
    expect(body).not.toHaveProperty("paymentId");
    expect(body.requestId).toMatch(/^[0-9a-f-]{36}$/);

    await expect(dialog).toHaveCount(0);
    await expect(page.getByText(`${title} disimpan.`)).toBeVisible();

    const account = await call<TenantAccount>(page, "GET", accountPath, 200);
    const corrected = account.invoices.find((each) => each.id === invoice.id);
    expect(corrected?.billed).toBe(newAmount);
    await expect(row).toContainText(formatRupiah(newAmount));
    await expect(row).toContainText(
      `dikoreksi dari ${formatRupiah(invoice.amount)}`,
    );
    const corrections = region.getByRole("list", { name: "Daftar koreksi" });
    const correctionRow = rows(corrections).filter({ hasText: reason });
    await expect(correctionRow).toHaveCount(1);
    await expect(correctionRow).toContainText(`Tagihan ${month}`);
    await expect(correctionRow).toContainText(`oleh ${HENDRA.name}`);

    await attachFullPageScreenshot(page, testInfo, "correction-after");
    expect(outsideRequests).toEqual([]);
  });

  test("asks for a reason first, and shows the API's answer for the same amount", async ({
    page,
  }, testInfo) => {
    test.setTimeout(90_000);
    const outsideRequests = trackOutsideRequests(page);
    const { orgId, tenant, accountPath, invoice } = await tenantWithInvoice(
      page,
      testInfo,
      2,
      "Yuni Ramadhani",
    );
    const month = formatMonthYear(invoice.period);
    const title = `Koreksi tagihan ${month}`;
    const posts: string[] = [];
    page.on("request", (request) => {
      if (isCorrectionPost(request)) posts.push(request.url());
    });

    const region = await openTenantPage(page, orgId, tenant);
    await rowTitled(region.getByRole("list", { name: "Daftar tagihan" }), month)
      .getByRole("button", { name: title })
      .click();
    const dialog = page.getByRole("dialog", { name: title });
    const parts = dialogParts(dialog);

    // No reason: the form says so and sends nothing.
    await parts.amount.fill(String(invoice.billed - 1_000));
    await parts.save.click();
    await expect(dialog.getByText("Alasan wajib diisi.")).toBeVisible();
    await expect(parts.reason).toBeFocused();
    // Only spaces count as none.
    await parts.reason.fill("   ");
    await parts.save.click();
    await expect(dialog.getByText("Alasan wajib diisi.")).toBeVisible();
    expect(posts).toEqual([]);

    // The amount it bills now: the API refuses (correction_no_change) under "Nominal baru". Read
    // from the account again rather than the fixture, so it is what the API compares against.
    const now = await call<TenantAccount>(page, "GET", accountPath, 200);
    const billedNow = now.invoices.find((row) => row.id === invoice.id)?.billed;
    if (billedNow === undefined)
      throw new Error("invoice gone from the account");
    await parts.amount.fill(String(billedNow));
    await expect(parts.amount).toHaveValue(
      formatRupiah(billedNow).replace("Rp", ""),
    );
    await parts.reason.fill(uniqueName(testInfo, "Cek ulang"));
    const answer = page.waitForResponse((response) =>
      isCorrectionPost(response.request()),
    );
    await parts.save.click();
    const response = await answer;
    expect(
      response.status(),
      `sent ${response.request().postData() ?? ""} for an invoice billing ${billedNow}`,
    ).toBe(409);
    const { error } = (await response.json()) as {
      error: { code: string; message: string; field?: string };
    };
    expect(error.code).toBe("correction_no_change");
    await expect(parts.amount).toHaveAccessibleDescription(error.message);
    await expect(parts.amount).toBeFocused();
    await expect(dialog).toBeVisible();
    expect(posts).toHaveLength(1);

    // "Batal" closes it with nothing changed.
    await parts.cancel.click();
    await expect(dialog).toHaveCount(0);
    expect(outsideRequests).toEqual([]);
  });

  test("corrects a payment's amount", async ({ page }, testInfo) => {
    test.setTimeout(90_000);
    const outsideRequests = trackOutsideRequests(page);
    const { orgId, tenant, accountPath, invoice } = await tenantWithInvoice(
      page,
      testInfo,
      3,
      "Yosefina Oktaviani",
    );
    const today = jakartaToday(new Date());
    const result = await call<PaymentResult>(
      page,
      "POST",
      `/orgs/${orgId}/payments`,
      201,
      {
        requestId: crypto.randomUUID(),
        tenantId: tenant.id,
        amount: invoice.billed,
        method: "cash",
        receivedOn: today,
        note: "",
      },
    );
    const newAmount = result.payment.net - 100_000;
    const reason = uniqueName(testInfo, "Salah hitung uang tunai");
    const title = `Koreksi pembayaran ${formatDate(today)}`;

    const region = await openTenantPage(page, orgId, tenant);
    const payments = region.getByRole("list", { name: "Daftar pembayaran" });
    const row = rowTitled(payments, `${formatDate(today)} · Tunai`);
    const correct = row.getByRole("button", { name: title });
    expect(await height(correct)).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
    await correct.click();

    const dialog = page.getByRole("dialog", { name: title });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText(tenant.name, { exact: true })).toBeVisible();
    await expect(dialog).toContainText(formatRupiah(result.payment.net));
    const parts = dialogParts(dialog);
    await parts.amount.fill(String(newAmount));
    await parts.reason.fill(reason);
    const sent = page.waitForRequest(isCorrectionPost);
    await parts.save.click();
    const body = (await sent).postDataJSON() as CorrectionInput;
    expect(body).toMatchObject({
      paymentId: result.payment.id,
      newAmount,
      reason,
    });
    expect(body).not.toHaveProperty("amountDelta");
    expect(body).not.toHaveProperty("invoiceId");
    await expect(dialog).toHaveCount(0);

    const account = await call<TenantAccount>(page, "GET", accountPath, 200);
    expect(
      account.payments.find((each) => each.id === result.payment.id)?.net,
    ).toBe(newAmount);
    await expect(row).toContainText(formatRupiah(newAmount));
    await expect(row).toContainText(
      `dikoreksi dari ${formatRupiah(result.payment.amount)}`,
    );
    await expect(
      rows(region.getByRole("list", { name: "Daftar koreksi" })).filter({
        hasText: reason,
      }),
    ).toContainText(`Pembayaran ${formatDate(today)}`);
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees no Koreksi action", async ({ page }) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, HENDRA, ANGGREK);
    const { items } = await call<TenantList>(
      page,
      "GET",
      `/orgs/${orgId}/tenants?q=${encodeURIComponent("Nadia Putri")}`,
      200,
    );
    const nadia = items.find((each) => each.name === "Nadia Putri");
    if (nadia === undefined) throw new Error("Nadia Putri is not seeded");
    const region = await openTenantPage(page, orgId, nadia);
    await expect(
      region.getByRole("heading", { level: 2, name: "Tagihan" }),
    ).toBeVisible();
    // generate-invoices (CI and seed) gives Nadia a paid invoice and its payment, so there
    // are rows a Koreksi action could sit on.
    await expect(
      region
        .getByRole("list", { name: "Daftar tagihan" })
        .locator(":scope > li"),
    ).not.toHaveCount(0);
    await expect(
      region
        .getByRole("list", { name: "Daftar pembayaran" })
        .locator(":scope > li"),
    ).not.toHaveCount(0);
    await expect(page.getByRole("button", { name: /^Koreksi/ })).toHaveCount(0);
    await expect(region.getByRole("button")).toHaveCount(0);
    expect(outsideRequests).toEqual([]);
  });
});
