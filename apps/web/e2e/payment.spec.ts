import {
  expect,
  test,
  type Locator,
  type Page,
  type Request,
  type TestInfo,
} from "@playwright/test";

import type {
  PaymentInput,
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
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { minBodyFontPx, projectDigit, projectWord } from "./support/projects";
import { findOrCreateProperty } from "./support/property-fixtures";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA } from "./support/staff";

// "Catat pembayaran" (W12) on Detail penyewa, against the real API and its worker. Each test
// makes its own tenant; one with a lease that starts on the 1st of this month in Jakarta is
// invoiced by the API within seconds. Every figure the panel must show is read from the
// recordPayment answer the page received or from the account the API returns, never
// hard-coded. Pak Hendra is Admin of the whole of Griya Sejahtera (payment:record) and Supervisor
// in Kost Putri Anggrek (no payment:record).

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const PANEL = "Catat pembayaran";
const REGION = "Uang penyewa";

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

// A tenant number no other run, project or retry uses; 0814-4 is used by no other spec.
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = projectDigit(testInfo);
  return `0814-4${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
}

// A calendar date `days` from today in Jakarta.
function jakartaDay(days: number): string {
  const [year = 0, month = 1, day = 1] = jakartaToday(new Date())
    .split("-")
    .map(Number);
  return new Date(Date.UTC(year, month - 1, day + days))
    .toISOString()
    .slice(0, 10);
}

function firstOfThisMonth(): string {
  return `${jakartaToday(new Date()).slice(0, 7)}-01`;
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

// A tenant of the test's own; with `rent`, a kos unit rented from the 1st of this month and its
// invoice, polled until the worker has made it.
async function makeTenant(
  page: Page,
  testInfo: TestInfo,
  orgId: string,
  options: { digit: number; name: string; rent: boolean },
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
  const accountPath = `/orgs/${orgId}/tenants/${tenant.id}/account`;
  if (options.rent) {
    const property = await findOrCreateProperty(page, orgId, testInfo, {
      base: "Pembayaran",
      kind: "kos",
    });
    const unit = await call<Unit>(
      page,
      "POST",
      `/orgs/${orgId}/properties/${property.id}/units`,
      201,
      {
        label: uniqueName(testInfo, `Kamar Bayar ${options.digit}`),
        terms: [{ months: 1, price: 1_100_000 }],
      },
    );
    await call(page, "POST", `/orgs/${orgId}/leases`, 201, {
      tenantId: tenant.id,
      unitId: unit.id,
      startDate: firstOfThisMonth(),
      termMonths: 1,
      depositAmount: 1_100_000,
    });
  }
  let account = await call<TenantAccount>(page, "GET", accountPath, 200);
  if (options.rent) {
    await expect
      .poll(
        async () => {
          account = await call<TenantAccount>(page, "GET", accountPath, 200);
          // The API reads the totals and the invoices in separate statements, so one answer
          // can list a fresh invoice while its totals do not count it yet: wait for both.
          return account.invoices.length >= 1 && account.outstanding > 0;
        },
        { timeout: 20_000 },
      )
      .toBe(true);
  }
  return { tenant, account, accountPath };
}

async function openTenantPage(page: Page, orgId: string, tenant: Tenant) {
  await page.goto(`/o/${orgId}/tenants/${tenant.id}`);
  await expect(
    page.getByRole("heading", { level: 1, name: tenant.name }),
  ).toBeVisible();
  await expect(page.getByRole("region", { name: REGION })).toBeVisible();
}

async function openPanel(page: Page): Promise<Locator> {
  await page.getByRole("button", { name: PANEL }).click();
  const panel = page.getByRole("dialog", { name: PANEL });
  await expect(panel).toBeVisible();
  return panel;
}

function isPaymentPost(request: Request): boolean {
  return (
    request.method() === "POST" &&
    /\/api\/orgs\/[^/]+\/payments$/.test(request.url())
  );
}

async function expectTouchTarget(locator: Locator, min = 44) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(min);
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
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

async function background(locator: Locator): Promise<string> {
  return locator.evaluate((el) => getComputedStyle(el).backgroundColor);
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("records a partial cash payment and sees what it paid", async ({
    page,
  }, testInfo) => {
    test.setTimeout(90_000);
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const { tenant, account, accountPath } = await makeTenant(
      page,
      testInfo,
      orgId,
      { digit: 1, name: "Wulan Pratiwi", rent: true },
    );
    const [unpaid] = account.invoices;
    if (unpaid === undefined) throw new Error("no invoice");
    await openTenantPage(page, orgId, tenant);

    // One primary action: "Catat pembayaran" is filled, "Ubah sewa" is not.
    const primary = await primaryBackground(page);
    const record = page.getByRole("button", { name: PANEL });
    expect(await background(record)).toBe(primary);
    await expectTouchTarget(record);
    const editLease = page
      .getByRole("button", { name: "Ubah sewa" })
      .filter({ visible: true });
    await expect(editLease).toHaveCount(1);
    expect(await background(editLease)).not.toBe(primary);

    const panel = await openPanel(page);
    await expect(panel).toContainText(tenant.name);
    // The summary, from the account.
    const summary = panel.getByRole("region", { name: "Total belum dibayar" });
    await expect(summary).toContainText(formatRupiah(account.outstanding));
    await expect(summary).toContainText(
      `1 tagihan belum lunas: ${formatMonthYear(unpaid.period)} sisa ${formatRupiah(unpaid.outstanding)}`,
    );
    // The defaults: cash, today.
    await expect(panel.getByRole("radio", { name: "Tunai" })).toBeChecked();
    await expect(panel.getByLabel("Tanggal diterima")).toHaveValue(
      jakartaToday(new Date()),
    );

    await panel.getByLabel("Jumlah diterima").fill("400000");
    await expect(panel.getByLabel("Jumlah diterima")).toHaveValue("400.000");
    await panel.getByLabel("Catatan (opsional)").fill("Dititip ke penjaga kos");

    const minFont = minBodyFontPx(testInfo);
    expect(
      await fontSize(
        panel.getByText("Hanya angka. Boleh kurang atau lebih dari tunggakan."),
      ),
    ).toBeGreaterThanOrEqual(minFont);
    expect(
      await fontSize(summary.getByText(/tagihan belum lunas/)),
    ).toBeGreaterThanOrEqual(minFont);
    for (const name of ["Tunai", "Transfer bank"]) {
      await expectTouchTarget(
        panel.locator("label").filter({ hasText: name }),
        56,
      );
    }
    await expectTouchTarget(panel.getByRole("button", { name: "Batal" }));
    await expectTouchTarget(
      panel.getByRole("button", { name: "Simpan pembayaran" }),
    );
    // From the top of the form, so the screenshot shows the summary.
    await panel.locator("form").evaluate((form) => form.scrollTo(0, 0));
    await attachViewportScreenshot(page, testInfo, "payment-form");

    const answer = page.waitForResponse((response) =>
      isPaymentPost(response.request()),
    );
    await panel.getByRole("button", { name: "Simpan pembayaran" }).click();
    const response = await answer;
    expect(response.status()).toBe(201);
    const result = (await response.json()) as PaymentResult;
    const sent = response.request().postDataJSON() as PaymentInput;
    expect(sent.amount).toBe(400_000);
    expect(sent.method).toBe("cash");
    expect(sent.note).toBe("Dititip ke penjaga kos");

    // "Pembayaran tercatat", in the same panel.
    await expect(
      panel.getByRole("heading", { name: "Pembayaran tercatat" }),
    ).toBeVisible();
    await expect(panel).toContainText(
      `${formatRupiah(result.payment.amount)} · Tunai · diterima ${formatDate(result.payment.receivedOn)} · dicatat ${HENDRA.name}`,
    );
    const [paidInvoice] = result.paid;
    if (paidInvoice === undefined) throw new Error("nothing paid");
    expect(paidInvoice.outstanding).toBeGreaterThan(0);
    const paidRows = panel
      .getByRole("list", { name: "Tagihan yang dibayar" })
      .getByRole("listitem");
    await expect(paidRows).toHaveCount(result.paid.length);
    await expect(paidRows.first()).toContainText(
      `Tagihan ${formatMonthYear(paidInvoice.period)} · ${formatRupiah(paidInvoice.amount)}`,
    );
    await expect(paidRows.first()).toContainText(
      `Sisa ${formatRupiah(paidInvoice.outstanding)}`,
    );
    await expect(panel).toContainText(
      `Sisa tunggakan${formatRupiah(result.overdue)}`,
    );
    if (result.outstanding !== result.overdue) {
      await expect(panel).toContainText(
        `Total belum dibayar${formatRupiah(result.outstanding)}`,
      );
    }
    await expect(panel).not.toContainText("Saldo lebih bayar");
    await expect(panel).toContainText(
      "Salah catat? Betulkan dengan Koreksi nominal pada pembayaran ini. Catatan lama tidak dihapus.",
    );
    await expectTouchTarget(panel.getByRole("button", { name: "Selesai" }));
    await attachViewportScreenshot(page, testInfo, "payment-result");

    await panel.getByRole("button", { name: "Selesai" }).click();
    await expect(panel).toHaveCount(0);

    // The page behind shows the payment and the new totals.
    const after = await call<TenantAccount>(page, "GET", accountPath, 200);
    const region = page.getByRole("region", { name: REGION });
    const payments = region.getByRole("list", { name: "Daftar pembayaran" });
    await expect(payments.locator(":scope > li")).toHaveCount(1);
    await expect(payments).toContainText(
      `${formatDate(result.payment.receivedOn)} · Tunai`,
    );
    await expect(payments).toContainText(formatRupiah(result.payment.net));
    await expect(
      region.getByRole("group", { name: "Tunggakan", exact: true }),
    ).toContainText(formatRupiah(after.overdue));
    if (after.outstanding > after.overdue) {
      await expect(
        region.getByRole("group", { name: "Tunggakan", exact: true }),
      ).toContainText(`Total belum dibayar ${formatRupiah(after.outstanding)}`);
    }
    expect(outsideRequests).toEqual([]);
  });

  test("records an overpayment by transfer on a past day, and the rest is credit", async ({
    page,
  }, testInfo) => {
    test.setTimeout(90_000);
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const { tenant, account, accountPath } = await makeTenant(
      page,
      testInfo,
      orgId,
      { digit: 2, name: "Yohana Sari", rent: true },
    );
    await openTenantPage(page, orgId, tenant);
    const panel = await openPanel(page);

    const yesterday = jakartaDay(-1);
    const amount = account.outstanding + 250_000;
    await panel.getByLabel("Jumlah diterima").fill(String(amount));
    await panel.getByRole("radio", { name: "Transfer bank" }).click();
    await expect(
      panel.getByRole("radio", { name: "Transfer bank" }),
    ).toBeChecked();
    await panel.getByLabel("Tanggal diterima").fill(yesterday);
    await panel
      .getByLabel("Catatan (opsional)")
      .fill("  Transfer dari rekening BCA suami  ");

    const answer = page.waitForResponse((response) =>
      isPaymentPost(response.request()),
    );
    await panel.getByRole("button", { name: "Simpan pembayaran" }).click();
    const response = await answer;
    expect(response.status()).toBe(201);
    const result = (await response.json()) as PaymentResult;
    const sent = response.request().postDataJSON() as PaymentInput;
    expect(sent).toMatchObject({
      tenantId: tenant.id,
      amount,
      method: "transfer",
      receivedOn: yesterday,
      note: "Transfer dari rekening BCA suami",
    });
    expect(Number.isInteger(sent.amount)).toBe(true);

    await expect(
      panel.getByRole("heading", { name: "Pembayaran tercatat" }),
    ).toBeVisible();
    await expect(panel).toContainText(
      `${formatRupiah(result.payment.amount)} · Transfer bank · diterima ${formatDate(yesterday)}`,
    );
    const paidRows = panel
      .getByRole("list", { name: "Tagihan yang dibayar" })
      .getByRole("listitem");
    await expect(paidRows).toHaveCount(result.paid.length);
    for (const [index, paid] of result.paid.entries()) {
      expect(paid.outstanding).toBe(0);
      await expect(paidRows.nth(index)).toContainText(
        `Tagihan ${formatMonthYear(paid.period)} · ${formatRupiah(paid.amount)}`,
      );
      await expect(paidRows.nth(index)).toContainText("Lunas");
    }
    expect(result.credit).toBeGreaterThan(0);
    await expect(panel).toContainText(
      `Saldo lebih bayar${formatRupiah(result.credit)}`,
    );
    await expect(panel).toContainText(
      "Otomatis dipakai untuk tagihan berikutnya.",
    );
    await expect(panel).toContainText(
      `Sisa tunggakan${formatRupiah(result.overdue)}`,
    );

    await panel.getByRole("button", { name: "Selesai" }).click();
    await expect(panel).toHaveCount(0);
    const after = await call<TenantAccount>(page, "GET", accountPath, 200);
    await expect(
      page
        .getByRole("region", { name: REGION })
        .getByRole("group", { name: "Saldo lebih bayar" }),
    ).toContainText(formatRupiah(after.credit));
    expect(outsideRequests).toEqual([]);
  });

  test("stops an empty amount, zero and a future date before sending", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const { tenant } = await makeTenant(page, testInfo, orgId, {
      digit: 3,
      name: "Zaenal Arifin",
      rent: false,
    });
    const posts: string[] = [];
    page.on("request", (request) => {
      if (isPaymentPost(request)) posts.push(request.url());
    });
    await openTenantPage(page, orgId, tenant);
    const panel = await openPanel(page);
    // Without a lease the subtitle is the name alone, and nothing is owed.
    await expect(panel).toContainText("Tidak ada tagihan yang belum lunas.");

    const save = panel.getByRole("button", { name: "Simpan pembayaran" });
    const amount = panel.getByLabel("Jumlah diterima");
    await save.click();
    await expect(panel.getByText("Isi jumlah yang diterima.")).toBeVisible();
    await expect(amount).toBeFocused();

    await amount.fill("0");
    await expect(panel.getByText("Isi jumlah yang diterima.")).toHaveCount(0);
    await save.click();
    await expect(panel.getByText("Jumlah harus lebih dari Rp0.")).toBeVisible();

    await amount.fill("100000");
    await panel.getByLabel("Tanggal diterima").fill(jakartaDay(1));
    await save.click();
    await expect(
      panel.getByText("Tanggal tidak boleh setelah hari ini."),
    ).toBeVisible();
    await expect(panel.getByLabel("Tanggal diterima")).toHaveAttribute(
      "aria-invalid",
      "true",
    );

    await panel.getByLabel("Tanggal diterima").fill("");
    await save.click();
    await expect(panel.getByText("Isi tanggal diterima.")).toBeVisible();

    expect(posts).toEqual([]);
    await panel.getByRole("button", { name: "Batal" }).click();
    await expect(panel).toHaveCount(0);
    expect(posts).toEqual([]);
    expect(outsideRequests).toEqual([]);
  });

  test("records one payment when save is pressed twice quickly", async ({
    page,
  }, testInfo) => {
    test.setTimeout(90_000);
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const { tenant, accountPath } = await makeTenant(page, testInfo, orgId, {
      digit: 4,
      name: "Yosef Hartono",
      rent: true,
    });
    const sent: PaymentInput[] = [];
    page.on("request", (request) => {
      if (isPaymentPost(request)) {
        sent.push(request.postDataJSON() as PaymentInput);
      }
    });
    await openTenantPage(page, orgId, tenant);
    const panel = await openPanel(page);
    await panel.getByLabel("Jumlah diterima").fill("300000");

    // Two presses in the same task, before React can render the disabled button.
    const answer = page.waitForResponse((response) =>
      isPaymentPost(response.request()),
    );
    await panel
      .getByRole("button", { name: "Simpan pembayaran" })
      .evaluate((button: HTMLButtonElement) => {
        button.click();
        button.click();
      });
    await answer;
    await expect(
      panel.getByRole("heading", { name: "Pembayaran tercatat" }),
    ).toBeVisible();

    expect(sent.length).toBeGreaterThanOrEqual(1);
    expect(new Set(sent.map((body) => body.requestId)).size).toBe(1);
    const account = await call<TenantAccount>(page, "GET", accountPath, 200);
    expect(account.payments).toHaveLength(1);

    await panel.getByRole("button", { name: "Selesai" }).click();
    await expect(
      page
        .getByRole("region", { name: REGION })
        .getByRole("list", { name: "Daftar pembayaran" })
        .locator(":scope > li"),
    ).toHaveCount(1);
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees no Catat pembayaran on a tenant with invoices", async ({
    page,
  }) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, ANGGREK);
    const { items } = await call<TenantList>(
      page,
      "GET",
      `/orgs/${orgId}/tenants?q=${encodeURIComponent("Nadia Putri")}`,
      200,
    );
    const nadia = items.find((each) => each.name === "Nadia Putri");
    if (nadia === undefined) throw new Error("Nadia Putri is not seeded");
    await page.goto(`/o/${orgId}/tenants/${nadia.id}`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Nadia Putri" }),
    ).toBeVisible();
    await expect(page.getByRole("region", { name: REGION })).toBeVisible();

    await expect(page.getByRole("button", { name: PANEL })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Ubah sewa" })).toHaveCount(
      0,
    );
    expect(outsideRequests).toEqual([]);
  });
});
