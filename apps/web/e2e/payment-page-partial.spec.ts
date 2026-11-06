import {
  expect,
  request as playwrightRequest,
  test,
  type APIRequestContext,
  type Page,
  type TestInfo,
} from "@playwright/test";

import type { PaymentPage } from "../src/lib/api/generated/model";
import { formatMonthYear } from "../src/lib/format/date";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import {
  expectTouchTarget,
  makeLink,
  projectDigit,
  readPage,
  rentSinceLastMonth,
  runId,
  staffApi,
  tenantClientAddress,
  thisMonthInvoiceId,
  trackTokenLeaks,
} from "./support/payment-page";
import { HENDRA } from "./support/staff";

// "Bayar sebagian" on the tenant payment page /t/[token] (W16 part 1, before promises): the
// tenant types an amount, getPaymentPreview says what it pays oldest first, and one QR is made
// for it. Against the real API, its worker and the AutoGoPay stand-in, like
// payment-page-qris.spec.ts. This spec's tenants: numbers 0815-2…, rooms "Kamar Sebagian".

const STAND_IN = process.env.AUTOGOPAY_CONTROL_URL ?? "http://127.0.0.1:8091";
const REMINDER_TEXT =
  "Kami kirim pengingat sehari sebelum tanggal ini. Pengingat tunggakan berhenti sampai tanggal ini; tagihan baru tetap dikirim.";
const LIMIT_TEXT = "QRIS paling banyak Rp10.000.000 sekali bayar.";

test.use({
  storageState: { cookies: [], origins: [] },
  extraHTTPHeaders: async ({}, provide, testInfo) => {
    await provide({ "X-Forwarded-For": tenantClientAddress(testInfo, 20) });
  },
});

// A monthly price no other test, project or recent run uses, so the QR amounts are the test's
// own on the shared stand-in. Rent is billed for last month and this month, so the total is
// twice the price.
function uniquePrice(testInfo: TestInfo, digit: number): number {
  const run = Number(runId()) % 1000;
  return 1_200_000 + run * 100 + Number(projectDigit(testInfo)) * 10 + digit;
}

type PartialFixture = {
  path: string;
  accountPath: string;
  page: PaymentPage;
  price: number;
  outside: string[];
  leaks: string[];
};

async function openPartialPage(
  page: Page,
  testInfo: TestInfo,
  digit: number,
): Promise<PartialFixture> {
  const price = uniquePrice(testInfo, digit);
  const api = await staffApi(HENDRA, testInfo);
  const rented = await rentSinceLastMonth(api, testInfo, {
    digit,
    phonePrefix: "0815-2",
    price,
    unitBase: "Kamar Sebagian",
  });
  const { path, token } = await makeLink(
    api,
    rented.orgId,
    thisMonthInvoiceId(rented.account),
  );
  await api.dispose();

  const expected = await readPage(page, path);
  expect(expected.outstanding).toBe(price * 2);
  expect(expected.openInvoices).toHaveLength(2);

  const outside = trackOutsideRequests(page);
  const leaks = trackTokenLeaks(page, path, token);
  await page.goto(path);
  return {
    path,
    accountPath: rented.accountPath,
    page: expected,
    price,
    outside,
    leaks,
  };
}

function finish(fixture: PartialFixture) {
  expect(fixture.outside).toEqual([]);
  expect(fixture.leaks).toEqual([]);
}

function monthOf(fixture: PartialFixture, index: number): string {
  const invoice = fixture.page.openInvoices[index];
  if (invoice === undefined) throw new Error("no such open invoice");
  return formatMonthYear(invoice.period);
}

// Bayar-Pilih: one of the radios, then its "Lanjut".
async function choose(page: Page, label: RegExp) {
  const radio = page
    .getByRole("radiogroup", { name: "Mau bayar berapa?" })
    .getByRole("radio", { name: label });
  await radio.check();
  const next = page.getByRole("button", { name: "Lanjut", exact: true });
  await expectTouchTarget(next);
  await next.click();
}

async function openEntry(page: Page) {
  await choose(page, /^Bayar sebagian/);
  await expect(
    page.getByRole("heading", { level: 1, name: "Bayar sebagian" }),
  ).toBeVisible();
}

function amountField(page: Page) {
  return page.getByLabel("Nominal yang dibayar sekarang");
}

test("Bayar sebagian previews an amount, makes its QR, and a paid QR leaves the rest", async ({
  page,
}, testInfo) => {
  const fixture = await openPartialPage(page, testInfo, 1);
  const { price } = fixture;
  await openEntry(page);

  await expect(
    page.getByText(`Total tagihan ${formatRupiah(price * 2)}`),
  ).toBeVisible();
  // Quick amounts: the oldest invoice (two are open), then the round amounts below the total.
  const oldestChip = page.getByRole("button", {
    name: new RegExp(`${monthOf(fixture, 0)} · ${formatRupiah(price)}$`),
  });
  await expect(oldestChip).toBeVisible();
  for (const round of [250_000, 500_000, 1_000_000]) {
    await expect(
      page.getByRole("button", { name: formatRupiah(round), exact: true }),
    ).toBeVisible();
  }

  // A chip fills the field and shows pressed; the preview pays the oldest invoice in full.
  await oldestChip.click();
  await expect(oldestChip).toHaveAttribute("aria-pressed", "true");
  const preview = page.getByRole("region", { name: "Uang ini dipakai untuk" });
  await expect(preview.getByRole("listitem")).toHaveCount(1);
  await expect(preview.getByRole("listitem")).toContainText("Lunas");

  // Typed: the oldest paid in full, Rp100.000 on this month, the rest still owed.
  const amount = price + 100_000;
  await amountField(page).fill(String(amount));
  await expect(oldestChip).toHaveAttribute("aria-pressed", "false");
  const rows = preview.getByRole("listitem");
  await expect(rows).toHaveCount(2);
  await expect(rows.nth(0)).toContainText(monthOf(fixture, 0));
  await expect(rows.nth(0)).toContainText("Lunas");
  await expect(rows.nth(0)).toContainText(formatRupiah(price));
  await expect(rows.nth(1)).toContainText(monthOf(fixture, 1));
  await expect(rows.nth(1)).toContainText(
    `sisa ${formatRupiah(price - 100_000)}`,
  );
  await expect(rows.nth(1)).toContainText(formatRupiah(100_000));
  await expect(preview).toContainText(
    `Sisa tagihan setelah ini ${formatRupiah(price - 100_000)}`,
  );

  // Something stays owed, so the next step is when it is paid.
  const rest = formatRupiah(price - 100_000);
  await expect(
    page.getByText(`Berikutnya: pilih kapan sisa ${rest} dilunasi.`),
  ).toBeVisible();
  const next = page.getByRole("button", {
    name: "Lanjut pilih tanggal pelunasan",
  });
  await expect(next).toBeEnabled();
  await expectTouchTarget(next);
  await expectTouchTarget(page.getByRole("button", { name: "Kembali" }));
  await attachViewportScreenshot(page, testInfo, "payment-partial-entry");

  // Janji pelunasan: the rest by the next invoice's due date, the default when offered.
  await next.click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Kapan sisanya dilunasi?" }),
  ).toBeVisible();
  await expect(
    page.getByText(`Sisa setelah bayar hari ini: ${rest}`),
  ).toBeVisible();
  await expect(
    page.getByRole("radio", { name: /^Bareng tagihan/ }),
  ).toBeChecked();
  await expect(
    page.getByText(`Kosongkan untuk semua sisa, ${rest}.`),
  ).toBeVisible();
  await attachViewportScreenshot(page, testInfo, "payment-promise-form");
  await page
    .getByRole("button", {
      name: `Simpan janji, lalu bayar ${formatRupiah(amount)}`,
    })
    .click();

  // The QR is for the chosen amount, under "Jumlah" and the promise for the rest.
  const qr = page.getByRole("img", {
    name: `Kode QRIS ${formatRupiah(amount)}`,
  });
  await expect(qr).toBeVisible();
  await expect(
    page.getByText(
      new RegExp(`^Sisa ${rest.replace(/\./g, "\\.")} dijanjikan .+\\.$`),
    ),
  ).toBeVisible();
  // A QR is live: the amount cannot change under it.
  await expect(page.getByRole("button", { name: "Ubah nominal" })).toHaveCount(
    0,
  );
  await expect
    .poll(() => qr.evaluate((image: HTMLImageElement) => image.naturalWidth))
    .toBeGreaterThan(0);
  await attachViewportScreenshot(page, testInfo, "payment-partial-qr");

  // Paid on the stand-in: Janji tercatat, with the payment, what is left and the promise.
  const standIn = await playwrightRequest.newContext({ baseURL: STAND_IN });
  const transactionId = await findQr(standIn, amount);
  const paid = await standIn.post("/_control/pay", {
    data: { transactionId },
  });
  expect(paid.ok(), await paid.text()).toBe(true);
  await standIn.dispose();
  await page.getByRole("button", { name: "Saya sudah bayar" }).click();

  await expect(
    page.getByRole("heading", { level: 1, name: "Janji tercatat" }),
  ).toBeVisible();
  await expect(
    page.getByText(
      `Pembayaran ${formatRupiah(amount)} sudah diterima. Terima kasih, ${fixture.page.tenantName}.`,
    ),
  ).toBeVisible();
  await expect(page.getByText(`Sisa tagihan ${rest}`)).toBeVisible();
  await expect(page.getByText("Semua sisa tagihan")).toBeVisible();
  await expect(page.getByText(REMINDER_TEXT)).toBeVisible();
  await expect(page.getByText(/kwitansi/i)).toHaveCount(0);
  await expectTouchTarget(page.getByRole("button", { name: "Ubah janji" }));
  await attachViewportScreenshot(page, testInfo, "payment-promise-recorded");
  finish(fixture);
});

test("Bayar sebagian refuses an amount above the total or one QR's most, and Kembali goes back", async ({
  page,
}, testInfo) => {
  const fixture = await openPartialPage(page, testInfo, 2);
  await openEntry(page);
  const field = amountField(page);

  // The whole total leaves nothing to promise: straight to its QR.
  await field.fill(String(fixture.page.outstanding));
  await expect(
    page.getByRole("button", {
      name: `Bayar QRIS ${formatRupiah(fixture.page.outstanding)}`,
    }),
  ).toBeEnabled();

  // Above what is owed: the API's message under the field, no preview, no QR.
  await field.fill(String(fixture.price * 2 + 1));
  await expect(field).toHaveAttribute("aria-invalid", "true");
  await expect(
    page.getByRole("region", { name: "Uang ini dipakai untuk" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: /^(Bayar QRIS|Lanjut pilih)/ }),
  ).toBeDisabled();
  await attachViewportScreenshot(page, testInfo, "payment-partial-above-total");

  // Above Rp10.000.000: refused before asking the API.
  await field.fill("10000001");
  await expect(page.getByText(LIMIT_TEXT)).toBeVisible();
  await expect(
    page.getByRole("button", { name: /^(Bayar QRIS|Lanjut pilih)/ }),
  ).toBeDisabled();

  // Empty: nothing to say, nothing to pay.
  await field.fill("");
  await expect(page.getByText(LIMIT_TEXT)).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Bayar QRIS" })).toBeDisabled();

  // Kembali: the choice again, still on "Bayar sebagian".
  await page.getByRole("button", { name: "Kembali" }).click();
  await expect(
    page.getByRole("radio", { name: /^Bayar sebagian/ }),
  ).toBeChecked();
  finish(fixture);
});

test("Belum bisa bayar sekarang records a promise by a chosen date, and Ubah janji changes it", async ({
  page,
}, testInfo) => {
  const fixture = await openPartialPage(page, testInfo, 3);
  const dates = fixture.page.promiseDates;
  if (dates === null) throw new Error("a tenant who owes has promise dates");
  const total = formatRupiah(fixture.page.outstanding);
  await choose(page, /^Belum bisa bayar sekarang/);

  await expect(
    page.getByRole("heading", { level: 1, name: "Kapan sisanya dilunasi?" }),
  ).toBeVisible();
  await expect(page.getByText(`Total tagihan ${total}`)).toBeVisible();
  await page.getByRole("radio", { name: /^Pilih tanggal sendiri/ }).check();
  const save = page.getByRole("button", { name: "Simpan janji", exact: true });

  // Checked before sending: a date is needed, and the amount is at most the rest.
  await save.click();
  await expect(page.getByText("Pilih tanggal pelunasan.")).toBeVisible();
  const date = page.getByLabel("Tanggal pelunasan");
  await date.fill(dates.earliest);
  const amount = page.getByLabel("Nominal janji (boleh dikosongkan)");
  await amount.fill(String(fixture.page.outstanding + 1));
  await save.click();
  await expect(
    page.getByText(`Nominal janji paling banyak ${total}.`),
  ).toBeVisible();
  await amount.fill("500000");
  await attachViewportScreenshot(page, testInfo, "payment-promise-own-date");
  await save.click();

  await expect(
    page.getByRole("heading", { level: 1, name: "Janji tercatat" }),
  ).toBeVisible();
  await expect(page.getByText(/sudah diterima/)).toHaveCount(0);
  await expect(page.getByText(formatRupiah(500_000))).toBeVisible();
  await expect(page.getByText(REMINDER_TEXT)).toBeVisible();

  // Opened again (a reminder's link): the promise above the choice, with Ubah janji.
  await page.reload();
  await expect(
    page.getByText(
      new RegExp(
        `^Janji pelunasan .+ · ${formatRupiah(500_000).replace(/\./g, "\\.")}$`,
      ),
    ),
  ).toBeVisible();
  await page.getByRole("button", { name: "Ubah janji" }).click();
  await expect(page.getByLabel("Tanggal pelunasan")).toHaveValue(
    dates.earliest,
  );
  await page.getByRole("radio", { name: /^Bareng tagihan/ }).check();
  await page.getByLabel("Nominal janji (boleh dikosongkan)").fill("");
  await page.getByRole("button", { name: "Simpan perubahan" }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Janji tercatat" }),
  ).toBeVisible();
  await expect(page.getByText("Semua sisa tagihan")).toBeVisible();
  finish(fixture);
});

// The stand-in's newest pending QR of this amount (the amount is the test's own).
async function findQr(
  standIn: APIRequestContext,
  amount: number,
): Promise<string> {
  let id: string | undefined;
  await expect
    .poll(async () => {
      const response = await standIn.get("/_control/transactions");
      const { transactions } = (await response.json()) as {
        transactions: {
          transactionId: string;
          amount: number;
          status: string;
        }[];
      };
      id = transactions.find(
        (qr) => qr.amount === amount && qr.status === "pending",
      )?.transactionId;
      return id !== undefined;
    })
    .toBe(true);
  if (id === undefined) throw new Error("unreachable");
  return id;
}
