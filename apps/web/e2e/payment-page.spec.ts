import { randomBytes } from "node:crypto";

import { expect, test, type Locator, type Page } from "@playwright/test";

import type { InvoiceList } from "../src/lib/api/generated/model";
import { formatMonthYear } from "../src/lib/format/date";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  attachFullPageScreenshot,
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import {
  BULANAN,
  GRIYA,
  GRIYA_BANK,
  call,
  expectTouchTarget,
  firstOfMonth,
  makeLink,
  payAll,
  orgIdByName,
  readPage,
  recordCash,
  rentSinceLastMonth,
  tenantClientAddress,
  staffApi,
  thisMonthInvoiceId,
  trackTokenLeaks,
  type RentOptions,
} from "./support/payment-page";
import { DIMAS, HENDRA } from "./support/staff";

// The tenant payment page /t/[token] (W15), read-only states, against the real API and its
// worker. The tenant has no account: the page opens in a browser with no session at all, and
// staff set up its data through the API with their own sessions. Pak Hendra is Admin of the
// whole of Griya Sejahtera, which seed-demo gives a bank account and QRIS (a fake AutoGoPay
// key); Pak Dimas is Admin of Kos Bulanan Sejahtera, which has neither. Every amount the page
// must show is read from the API. The QRIS states are in payment-page-qris.spec.ts.

// Minimum body text per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };

// This spec's tenants: tenant numbers 0814-8…, rooms "Kamar Bayar".
function rent(digit: number): RentOptions {
  return {
    digit,
    phonePrefix: "0814-8",
    price: 1_100_000,
    unitBase: "Kamar Bayar",
  };
}

// The tenant: no cookies, no storage, and a client address of the test's own (see
// tenantClientAddress).
test.use({
  storageState: { cookies: [], origins: [] },
  // Playwright reads the fixtures a fixture needs from this pattern; this one needs none.
  extraHTTPHeaders: async ({}, provide, testInfo) => {
    await provide({ "X-Forwarded-For": tenantClientAddress(testInfo, 18) });
  },
});

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
  const rented = await rentSinceLastMonth(api, testInfo, rent(1));
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

  // Bayar-Pilih: pay everything, a part, or not yet, with "Bayar semua" picked and one primary.
  const choices = page.getByRole("radiogroup", { name: "Mau bayar berapa?" });
  await expect(choices.getByRole("radio")).toHaveCount(3);
  await expect(
    choices.getByRole("radio", { name: /^Bayar semua/ }),
  ).toBeChecked();
  await expect(
    choices.getByRole("radio", { name: /^Bayar sebagian/ }),
  ).toBeVisible();
  await expect(
    choices.getByRole("radio", { name: /^Belum bisa bayar sekarang/ }),
  ).toBeVisible();
  await attachViewportScreenshot(page, testInfo, "payment-choice");
  await payAll(page, expected.outstanding);

  // Griya Sejahtera has QRIS: two tabs, QRIS first, with "Bayar QRIS" for the total as its
  // one primary action. Bank transfer is the other tab.
  const tabs = page.getByRole("tablist", { name: "Cara bayar" });
  await expect(tabs).toBeVisible();
  const qrisTab = tabs.getByRole("tab", { name: "QRIS", exact: true });
  const transferTab = tabs.getByRole("tab", { name: "Transfer bank" });
  await expect(qrisTab).toHaveAttribute("aria-selected", "true");
  await expect(transferTab).toHaveAttribute("aria-selected", "false");
  await expectTouchTarget(qrisTab);
  await expectTouchTarget(transferTab);
  const payQris = page.getByRole("button", {
    name: `Bayar QRIS ${formatRupiah(expected.outstanding)}`,
  });
  await expect(payQris).toBeVisible();
  // "Bayar QRIS" is the one primary; "Kembali" goes back to the choice.
  await expect(page.getByRole("button", { name: "Kembali" })).toBeVisible();
  await expect(page.getByRole("button")).toHaveCount(2);
  await expect(page.getByRole("region", { name: "Transfer bank" })).toHaveCount(
    0,
  );

  await transferTab.click();
  await expect(transferTab).toHaveAttribute("aria-selected", "true");
  await expect(payQris).toHaveCount(0);

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

  // The one primary action, and "Kembali".
  const copy = page.getByRole("button", { name: "Salin nomor rekening" });
  await expect(page.getByRole("button")).toHaveCount(2);
  await expectTouchTarget(copy);
  await copy.click();
  await expect(transfer.getByRole("status")).toHaveText(
    "Nomor rekening disalin",
  );
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
  const rented = await rentSinceLastMonth(api, testInfo, rent(2));
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
  await payAll(page, expected.outstanding);
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
  // Nothing to press but "Kembali" to the choice.
  await expect(page.getByRole("button")).toHaveCount(1);
  // No QRIS set up: no tabs and no QRIS at all.
  await expect(page.getByRole("tablist")).toHaveCount(0);
  await expect(page.getByText("QRIS")).toHaveCount(0);
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
