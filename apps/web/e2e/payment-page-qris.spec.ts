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
  PaymentPage,
  TenantAccount,
} from "../src/lib/api/generated/model";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import {
  GRIYA_BANK,
  call,
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
  type RentedTenant,
} from "./support/payment-page";
import { HENDRA } from "./support/staff";

// The QRIS states of the tenant payment page /t/[token] (W15 part 2), against the real API, its
// worker and the AutoGoPay stand-in (services/api/cmd/fakeautogopay, `make fake-autogopay`),
// which the API and the worker reach through AUTOGOPAY_BASE_URL. seed-demo gives Griya
// Sejahtera a fake AutoGoPay key, so its pages offer QRIS.
//
// The stand-in is shared by both projects, which run at the same time, and by every earlier run
// while it stays up, so it is never reset: each test finds its own QRs by an amount no other
// test, project or recent run uses, made after the test started.
//
// getPaymentStatus never asks AutoGoPay (the webhook is not built yet): a QR paid or expired on
// the stand-in reaches the page through "Saya sudah bayar" (checkPaymentQris), which asks it.

const STAND_IN = process.env.AUTOGOPAY_CONTROL_URL ?? "http://127.0.0.1:8091";

const PENDING_TEXT =
  "Menunggu pembayaran. Halaman ini berubah sendiri jadi Lunas.";
const EXPIRED_TEXT =
  "QR hanya berlaku 15 menit dan sudah habis. Tagihan tidak berubah, dan QR lama sudah dibatalkan.";
const EXPIRED_NOTE = "Belum ada pembayaran yang masuk dari QR ini.";
const UNAVAILABLE_TEXT =
  "QRIS sedang gangguan. Pakai transfer bank dulu; tagihan tetap berjalan seperti biasa.";
const GUIDE_TITLE = "Cara bayar pakai QR yang diunduh";
const GUIDE_STEPS = [
  'Ketuk "Unduh QR". Bisa juga ambil tangkapan layar, asal seluruh kotak QR ikut.',
  "Buka aplikasi bank atau e-wallet, lalu pilih menu QRIS atau Scan.",
  'Ketuk ikon galeri atau "Upload QR", lalu pilih gambar QR tadi.',
  "Pastikan jumlahnya sama dengan di halaman ini, lalu bayar dengan PIN Anda.",
  'Kembali ke halaman ini. Kalau belum Lunas, ketuk "Saya sudah bayar".',
];
const GUIDE_FALLBACK =
  "Tidak ada pilihan galeri? Scan QR ini dengan kamera dari HP lain, atau bayar lewat Transfer bank.";
const LIMIT_NOTE =
  "QRIS paling banyak Rp10.000.000 sekali bayar. Sisanya bayar dengan QR berikutnya atau transfer bank.";

// The tenant: no cookies, no storage, and a client address of the test's own (see
// tenantClientAddress).
test.use({
  storageState: { cookies: [], origins: [] },
  // Playwright reads the fixtures a fixture needs from this pattern; this one needs none.
  extraHTTPHeaders: async ({}, provide, testInfo) => {
    await provide({ "X-Forwarded-For": tenantClientAddress(testInfo, 19) });
  },
});

// A monthly price no other test, project or run in the last 1000 minutes uses, so the tenant's
// total, and so its QR's amount, is the test's own.
function uniquePrice(testInfo: TestInfo, digit: number, base: number): number {
  const run = Number(runId()) % 1000;
  return base + run * 100 + Number(projectDigit(testInfo)) * 10 + digit;
}

type StandInQr = {
  transactionId: string;
  amount: number;
  status: "pending" | "paid" | "expired" | "cancelled";
  createdAt: string;
  expiresAt: string;
};

type QrisFixture = {
  rented: RentedTenant;
  path: string;
  token: string;
  expected: PaymentPage;
  outside: string[];
  leaks: string[];
  standIn: APIRequestContext;
  // When the test started, minus a margin: the stand-in reports times to the second.
  since: number;
};

// A Griya tenant of the test's own, a link to their invoice, and the page open on it with the
// outside-request and token trackers on.
async function openQrisPage(
  page: Page,
  testInfo: TestInfo,
  digit: number,
  base = 1_100_000,
): Promise<QrisFixture> {
  const since = Date.now() - 2_000;
  const api = await staffApi(HENDRA, testInfo);
  const rented = await rentSinceLastMonth(api, testInfo, {
    digit,
    phonePrefix: "0815-1",
    price: uniquePrice(testInfo, digit, base),
    unitBase: "Kamar QRIS",
  });
  const { path, token } = await makeLink(
    api,
    rented.orgId,
    thisMonthInvoiceId(rented.account),
  );
  await api.dispose();

  const expected = await readPage(page, path);
  expect(expected.paid).toBe(false);
  expect(expected.qrisAvailable).toBe(true);

  const outside = trackOutsideRequests(page);
  const leaks = trackTokenLeaks(page, path, token);
  await page.goto(path);
  await expect(page.getByRole("tablist", { name: "Cara bayar" })).toBeVisible();

  const standIn = await playwrightRequest.newContext({ baseURL: STAND_IN });
  return { rented, path, token, expected, outside, leaks, standIn, since };
}

async function finish(fixture: QrisFixture): Promise<void> {
  await fixture.standIn.dispose();
  expect(fixture.outside).toEqual([]);
  expect(fixture.leaks).toEqual([]);
}

// The test's QRs on the stand-in, newest first.
async function qrsFor(fixture: QrisFixture): Promise<StandInQr[]> {
  const response = await fixture.standIn.get("/_control/transactions");
  expect(response.status()).toBe(200);
  const { transactions } = (await response.json()) as {
    transactions: StandInQr[];
  };
  return transactions.filter(
    (qr) =>
      qr.amount === fixture.expected.outstanding &&
      Date.parse(qr.createdAt) >= fixture.since,
  );
}

async function control(
  fixture: QrisFixture,
  path: string,
  data: object,
): Promise<void> {
  const response = await fixture.standIn.post(path, { data });
  expect(response.ok(), await response.text()).toBe(true);
}

function payButton(page: Page, fixture: QrisFixture) {
  return page.getByRole("button", {
    name: `Bayar QRIS ${formatRupiah(fixture.expected.outstanding)}`,
  });
}

function qrImage(page: Page, fixture: QrisFixture) {
  return page.getByRole("img", {
    name: `Kode QRIS ${formatRupiah(fixture.expected.outstanding)}`,
  });
}

// The QR image is on screen and its PNG loaded.
async function expectQrImageLoaded(page: Page, fixture: QrisFixture) {
  await expectImageLoaded(qrImage(page, fixture));
}

async function expectImageLoaded(image: Locator) {
  await expect(image).toBeVisible();
  await expect
    .poll(() =>
      image.evaluate((element) =>
        element instanceof HTMLImageElement && element.complete
          ? element.naturalWidth
          : 0,
      ),
    )
    .toBeGreaterThan(0);
}

// Presses "Bayar QRIS" and waits for the live QR. Returns the stand-in's QR for it.
async function makeQr(page: Page, fixture: QrisFixture): Promise<StandInQr> {
  await payButton(page, fixture).click();
  await expectQrImageLoaded(page, fixture);
  let qrs: StandInQr[] = [];
  await expect
    .poll(async () => {
      qrs = await qrsFor(fixture);
      return qrs.length;
    })
    .toBe(1);
  const [qr] = qrs;
  if (qr === undefined) throw new Error("unreachable");
  expect(qr.status).toBe("pending");
  return qr;
}

test("Bayar QRIS shows a live QR to scan or download", async ({
  page,
}, testInfo) => {
  const fixture = await openQrisPage(page, testInfo, 1);
  const pay = payButton(page, fixture);
  await expect(pay).toBeVisible();
  await expectTouchTarget(pay);

  await makeQr(page, fixture);

  await expect(page.getByText(/^Berlaku \d+:\d\d lagi$/)).toBeVisible();
  await expect(page.getByRole("status")).toHaveText(PENDING_TEXT);
  await expect(
    page.getByText(
      "Bayar dari HP ini? Unduh QR, lalu pilih dari galeri di aplikasi bank atau e-wallet.",
    ),
  ).toBeVisible();

  // "Unduh QR": the same-origin image, saved under a name without the token.
  const download = page.getByRole("link", { name: "Unduh QR" });
  await expect(download).toBeVisible();
  await expectTouchTarget(download);
  const name = await download.getAttribute("download");
  expect(name).toMatch(/^qris-[a-z0-9-]+\.png$/);
  expect(name).not.toContain(fixture.token);
  const href = await download.getAttribute("href");
  if (href === null) throw new Error("Unduh QR has no href");
  const url = new URL(href, page.url());
  expect(url.origin).toBe(new URL(page.url()).origin);
  expect(url.pathname).toBe(`/api${fixture.path}/qris.png`);
  expect(url.search).toMatch(/^\?v=\d+$/);

  // More than 2 minutes left: "Saya sudah bayar", and no "Buat QR baru" yet.
  const paidClaim = page.getByRole("button", { name: "Saya sudah bayar" });
  await expect(paidClaim).toBeVisible();
  await expectTouchTarget(paidClaim);
  await expect(page.getByRole("button", { name: "Buat QR baru" })).toHaveCount(
    0,
  );
  // "Saya sudah bayar" and the closed guide's toggle are the only buttons.
  await expect(page.getByRole("button")).toHaveCount(2);

  await attachViewportScreenshot(page, testInfo, "payment-qris-live");

  // "Cara bayar pakai QR yang diunduh": closed at first, so "Unduh QR" stays the one primary
  // action; open, the steps, the per-app notes and the way out when an app has no gallery.
  const guide = page.getByRole("button", { name: GUIDE_TITLE });
  await expect(guide).toHaveAttribute("aria-expanded", "false");
  await expectTouchTarget(guide);
  await expect(page.getByText(GUIDE_FALLBACK)).toHaveCount(0);
  await guide.evaluate((element) => element.scrollIntoView({ block: "end" }));
  await attachViewportScreenshot(page, testInfo, "payment-qris-guide-closed");
  await guide.click();
  await expect(guide).toHaveAttribute("aria-expanded", "true");
  await expect(
    page
      .getByRole("list")
      .filter({ hasText: "Unduh QR" })
      .getByRole("listitem"),
  ).toHaveText(GUIDE_STEPS);
  for (const app of ["myBCA", "Livin' by Mandiri", "GoPay", "Aplikasi lain"]) {
    await expect(page.getByRole("term").filter({ hasText: app })).toBeVisible();
  }
  await expect(page.getByText(GUIDE_FALLBACK)).toBeVisible();
  await guide.evaluate((element) => element.scrollIntoView({ block: "start" }));
  await attachViewportScreenshot(page, testInfo, "payment-qris-guide");

  await guide.click();
  await expect(page.getByText(GUIDE_FALLBACK)).toHaveCount(0);
  await finish(fixture);
});

test("a QR paid on AutoGoPay turns the page Lunas after Saya sudah bayar", async ({
  page,
}, testInfo) => {
  const fixture = await openQrisPage(page, testInfo, 2);
  const qr = await makeQr(page, fixture);

  await control(fixture, "/_control/pay", { transactionId: qr.transactionId });
  await page.getByRole("button", { name: "Saya sudah bayar" }).click();

  const paid = page.getByRole("region", { name: "Lunas" });
  await expect(paid.getByText("Lunas", { exact: true })).toBeVisible();
  await expect(
    paid.getByText("Terima kasih, semua tagihan sudah lunas."),
  ).toBeVisible();
  await expect(page.getByRole("tablist")).toHaveCount(0);

  const api = await staffApi(HENDRA, testInfo);
  const account = await call<TenantAccount>(
    api,
    "GET",
    fixture.rented.accountPath,
    200,
  );
  await api.dispose();
  expect(account.outstanding).toBe(0);
  await finish(fixture);
});

test("an expired QR shows QR kedaluwarsa, and Buat QR baru makes a new one", async ({
  page,
}, testInfo) => {
  const fixture = await openQrisPage(page, testInfo, 3);
  const first = await makeQr(page, fixture);

  await control(fixture, "/_control/expire", {
    transactionId: first.transactionId,
  });
  await page.getByRole("button", { name: "Saya sudah bayar" }).click();

  await expect(page.getByText(EXPIRED_TEXT)).toBeVisible();
  await expect(page.getByText(EXPIRED_NOTE)).toBeVisible();
  await expect(page.getByRole("status")).toHaveText("QR kedaluwarsa");
  await expect(qrImage(page, fixture)).toHaveCount(0);
  const again = page.getByRole("button", { name: "Buat QR baru" });
  await expect(again).toBeVisible();
  await expectTouchTarget(again);
  // The one primary action: no other button.
  await expect(page.getByRole("button")).toHaveCount(1);
  await attachViewportScreenshot(page, testInfo, "payment-qris-expired");

  await again.click();
  await expectQrImageLoaded(page, fixture);
  await expect(page.getByRole("status")).toHaveText(PENDING_TEXT);
  let qrs: StandInQr[] = [];
  await expect
    .poll(async () => {
      qrs = await qrsFor(fixture);
      return qrs.length;
    })
    .toBe(2);
  const [newest] = qrs;
  expect(newest?.transactionId).not.toBe(first.transactionId);
  expect(newest?.status).toBe("pending");
  await finish(fixture);
});

test("Saya sudah bayar without a payment shows Sedang dicek, also after a reload", async ({
  page,
}, testInfo) => {
  const fixture = await openQrisPage(page, testInfo, 4);
  await makeQr(page, fixture);

  await page.getByRole("button", { name: "Saya sudah bayar" }).click();

  async function expectChecking() {
    await expect(
      page.getByRole("heading", {
        level: 2,
        name: "Pembayaran sedang dicek",
      }),
    ).toBeVisible();
    await expect(
      page.getByText(
        "Konfirmasi dari bank belum masuk. Biasanya selesai kurang dari 30 menit, dan halaman ini berubah sendiri jadi Lunas.",
      ),
    ).toBeVisible();
    await expect(
      page.getByText("Jangan bayar ulang.", { exact: true }),
    ).toBeVisible();
    const details = page.getByRole("region", { name: "Rincian" });
    await expect(details).toContainText(
      `Jumlah${formatRupiah(fixture.expected.outstanding)}`,
    );
    await expect(details).toContainText(
      /Dicek sejak\d{1,2} \S+ \d{4}, \d{2}\.\d{2}/,
    );
    await expect(details).toContainText("MetodeQRIS");
    // Nothing to do but wait: no tabs and no button.
    await expect(page.getByRole("tablist")).toHaveCount(0);
    await expect(page.getByRole("button")).toHaveCount(0);
  }

  await expectChecking();
  await attachViewportScreenshot(page, testInfo, "payment-qris-checking");

  // A new visit resumes it.
  await page.reload();
  await expectChecking();
  await finish(fixture);
});

test("when QRIS fails, the page switches to bank transfer with a warning", async ({
  page,
}, testInfo) => {
  const fixture = await openQrisPage(page, testInfo, 5);
  await control(fixture, "/_control/fail-next", {
    method: "generate",
    amount: fixture.expected.outstanding,
  });

  await payButton(page, fixture).click();

  // Filtered: Next.js's route announcer is an alert region too.
  await expect(
    page.getByRole("alert").filter({ hasText: UNAVAILABLE_TEXT }),
  ).toBeVisible();
  await expect(
    page.getByRole("tab", { name: "Transfer bank" }),
  ).toHaveAttribute("aria-selected", "true");
  const transfer = page.getByRole("region", { name: "Transfer bank" });
  await expect(
    transfer.getByText(GRIYA_BANK.accountNumber, { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Salin nomor rekening" }),
  ).toBeVisible();
  await expect(page.getByRole("button")).toHaveCount(1);
  expect(await qrsFor(fixture)).toEqual([]);
  await attachViewportScreenshot(page, testInfo, "payment-qris-unavailable");

  // Back on the QRIS tab, "Bayar QRIS" is there again.
  await page.getByRole("tab", { name: "QRIS", exact: true }).click();
  await expect(payButton(page, fixture)).toBeVisible();
  await finish(fixture);
});

test("a reload resumes the live QR without making a new one", async ({
  page,
}, testInfo) => {
  const fixture = await openQrisPage(page, testInfo, 6);
  const qr = await makeQr(page, fixture);

  const made: string[] = [];
  page.on("request", (request) => {
    if (
      request.method() === "POST" &&
      new URL(request.url()).pathname.endsWith("/qris")
    ) {
      made.push(request.url());
    }
  });
  await page.reload();

  await expectQrImageLoaded(page, fixture);
  await expect(page.getByRole("status")).toHaveText(PENDING_TEXT);
  await expect(page.getByText(/^Berlaku \d+:\d\d lagi$/)).toBeVisible();
  expect(made).toEqual([]);
  const qrs = await qrsFor(fixture);
  expect(qrs.map((each) => each.transactionId)).toEqual([qr.transactionId]);
  await finish(fixture);
});

test("an expired QR from an earlier visit does not show on a new one", async ({
  page,
}, testInfo) => {
  const fixture = await openQrisPage(page, testInfo, 7);
  const qr = await makeQr(page, fixture);
  await control(fixture, "/_control/expire", {
    transactionId: qr.transactionId,
  });
  // The API learns of it when it asks AutoGoPay: "Saya sudah bayar".
  await page.getByRole("button", { name: "Saya sudah bayar" }).click();
  await expect(page.getByText(EXPIRED_TEXT)).toBeVisible();

  await page.goto(fixture.path);
  await expect(payButton(page, fixture)).toBeVisible();
  await expect(page.getByText(EXPIRED_TEXT)).toHaveCount(0);
  await expect(page.getByText(EXPIRED_NOTE)).toHaveCount(0);
  await expect(qrImage(page, fixture)).toHaveCount(0);
  await finish(fixture);
});

test("above Rp10.000.000 one QR asks for Rp10.000.000 and says the rest comes later", async ({
  page,
}, testInfo) => {
  // Two months of a room above Rp5.000.000.
  const fixture = await openQrisPage(page, testInfo, 8, 5_100_000);
  expect(fixture.expected.outstanding).toBeGreaterThan(10_000_000);

  await expect(page.getByText(LIMIT_NOTE)).toBeVisible();
  const start = page.getByRole("button", { name: "Bayar QRIS Rp10.000.000" });
  await expectTouchTarget(start);
  await start.click();

  // The stand-in tells QRs apart only by amount, and both projects make a Rp10.000.000 QR here,
  // so this test reads the page, not the stand-in, and leaves the QR unpaid.
  await expectImageLoaded(
    page.getByRole("img", { name: "Kode QRIS Rp10.000.000" }),
  );
  await expect(page.getByRole("link", { name: "Unduh QR" })).toBeVisible();
  await expect(page.getByText(LIMIT_NOTE)).toBeVisible();
  // The summary still shows the whole total the API gave.
  await expect(
    page.getByText(formatRupiah(fixture.expected.outstanding)).first(),
  ).toBeVisible();
  await attachViewportScreenshot(page, testInfo, "payment-qris-above-limit");
  await finish(fixture);
});
