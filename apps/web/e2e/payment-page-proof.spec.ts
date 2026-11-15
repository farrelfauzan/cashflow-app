import {
  expect,
  test,
  type APIRequestContext,
  type Page,
  type Request,
  type TestInfo,
} from "@playwright/test";

import type { PaymentPage, PaymentProof } from "../src/lib/api/generated/model";
import { formatTimestampShortWeekdayDayMonthTime } from "../src/lib/format/date";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import {
  expectTouchTarget,
  makeLink,
  payAll,
  readPage,
  rentSinceLastMonth,
  staffApi,
  tenantClientAddress,
  thisMonthInvoiceId,
  trackTokenLeaks,
  type RentedTenant,
} from "./support/payment-page";
import { waitForCleanProof } from "./support/proof-fixtures";
import { minBodyFontPx } from "./support/projects";
import { HENDRA } from "./support/staff";

// "Kirim bukti transfer" on the tenant payment page (board "Kirim bukti transfer", Bayar-Bukti),
// against the real API with object storage and the fake upload scanner (UPLOAD_SCANNER=fake, see
// e2e/support/proof-fixtures.ts). The tenant has no account; Pak Hendra, Admin of Griya Sejahtera
// (bank account and QRIS from seed-demo), rents each test its own tenant and room, and finds the
// proof in Antrean cek. This spec's tenants: numbers 0815-7…, rooms "Kamar Bukti".

const SENT_TEXT = "Bukti terkirim. Admin akan mengeceknya.";
const LIST_TITLE = "Bukti transfer terkirim";
const LIST_NOTE =
  "Admin mencocokkan dengan mutasi rekening, lalu mencatat pembayarannya. Tagihan berubah setelah dicatat.";
const TYPE_REFUSED = "Hanya foto JPEG, PNG atau WebP yang bisa dikirim.";

// The tenant: no cookies, no storage, and a client address of the test's own.
test.use({
  storageState: { cookies: [], origins: [] },
  extraHTTPHeaders: async ({}, provide, testInfo) => {
    await provide({ "X-Forwarded-For": tenantClientAddress(testInfo, 24) });
  },
});

type ProofFixture = {
  api: APIRequestContext;
  rented: RentedTenant;
  path: string;
  expected: PaymentPage;
  outside: string[];
  leaks: string[];
};

// A tenant of the test's own opens their link and goes to the Transfer bank tab.
async function openTransferTab(
  page: Page,
  testInfo: TestInfo,
  digit: number,
): Promise<ProofFixture> {
  const api = await staffApi(HENDRA, testInfo);
  const rented = await rentSinceLastMonth(api, testInfo, {
    digit,
    phonePrefix: "0815-7",
    price: 1_150_000,
    unitBase: "Kamar Bukti",
  });
  const { path, token } = await makeLink(
    api,
    rented.orgId,
    thisMonthInvoiceId(rented.account),
  );
  const expected = await readPage(page, path);
  expect(expected.proofsWaiting).toEqual([]);

  const outside = trackOutsideRequests(page);
  const leaks = trackTokenLeaks(page, path, token);
  await page.goto(path);
  await payAll(page, expected.outstanding);
  const transferTab = page
    .getByRole("tablist", { name: "Cara bayar" })
    .getByRole("tab", { name: "Transfer bank" });
  await transferTab.click();
  await expect(transferTab).toHaveAttribute("aria-selected", "true");
  return { api, rented, path, expected, outside, leaks };
}

// A photo of a bank app's "Transfer berhasil" screen, drawn by the browser as a PNG (no file is
// committed). At 2400×3600 it is above the page's 2000px limit, so the page shrinks it and sends
// a JPEG.
async function largeTransferPhoto(page: Page, amount: string): Promise<Buffer> {
  const dataUrl = await page.evaluate((text) => {
    const canvas = document.createElement("canvas");
    canvas.width = 2400;
    canvas.height = 3600;
    const ctx = canvas.getContext("2d");
    if (ctx === null) throw new Error("no 2d canvas");
    ctx.fillStyle = "#f2f5f7";
    ctx.fillRect(0, 0, 2400, 3600);
    ctx.fillStyle = "#0b5aa8";
    ctx.fillRect(0, 0, 2400, 500);
    ctx.fillStyle = "#ffffff";
    ctx.font = "bold 150px sans-serif";
    ctx.fillText("m-Transfer", 160, 320);
    ctx.fillStyle = "#1e7d32";
    ctx.font = "bold 170px sans-serif";
    ctx.fillText("Transfer berhasil", 160, 1000);
    ctx.fillStyle = "#1e2a2b";
    ctx.font = "bold 220px sans-serif";
    ctx.fillText(text, 160, 1400);
    ctx.font = "120px sans-serif";
    ctx.fillText("Ke: BCA 0123 456 789", 160, 1800);
    ctx.fillText("a.n. Ani Rahmawati", 160, 2000);
    return canvas.toDataURL("image/png");
  }, amount);
  const base64 = dataUrl.split(",")[1];
  if (base64 === undefined) throw new Error("the canvas gave no PNG");
  return Buffer.from(base64, "base64");
}

function isProofPost(request: Request, path: string): boolean {
  return (
    request.method() === "POST" &&
    new URL(request.url()).pathname === `/api${path}/proof`
  );
}

async function fontSize(page: Page, text: string): Promise<number> {
  return page
    .getByText(text, { exact: true })
    .first()
    .evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

test("Kirim bukti transfer sends a shrunk photo through our API; it waits in the list and in Antrean cek", async ({
  page,
}, testInfo) => {
  const fixture = await openTransferTab(page, testInfo, 1);
  const { path, rented } = fixture;

  // Board 1: a plain button under the instructions; copying the account stays the primary.
  const transfer = page.getByRole("region", { name: "Transfer bank" });
  const open = transfer.getByRole("button", { name: "Kirim bukti transfer" });
  await expect(open).toBeVisible();
  await expectTouchTarget(open);
  await expect(
    page.getByRole("button", { name: "Salin nomor rekening" }),
  ).toBeVisible();
  await expect(page.getByRole("region", { name: LIST_TITLE })).toHaveCount(0);
  await open.scrollIntoViewIfNeeded();
  await attachViewportScreenshot(page, testInfo, "proof-transfer-tab");

  // The button opens the phone's camera or gallery; the chosen photo shows in the sheet.
  const photo = await largeTransferPhoto(
    page,
    formatRupiah(fixture.expected.outstanding),
  );
  const chooser = page.waitForEvent("filechooser");
  await open.click();
  const fileChooser = await chooser;
  expect(fileChooser.isMultiple()).toBe(false);
  await fileChooser.setFiles({
    name: "bukti-transfer.png",
    mimeType: "image/png",
    buffer: photo,
  });

  const sheet = page.getByRole("dialog", { name: "Kirim bukti transfer" });
  await expect(sheet).toBeVisible();
  await expect(
    sheet.getByText("Pastikan jumlah, tanggal dan rekening tujuan terbaca."),
  ).toBeVisible();
  const preview = sheet.getByRole("img", {
    name: "Foto bukti transfer yang dipilih",
  });
  await expect(preview).toBeVisible();
  await expect
    .poll(() => preview.evaluate((img: HTMLImageElement) => img.naturalWidth))
    .toBeGreaterThan(0);
  const other = sheet.getByRole("button", { name: "Pilih foto lain" });
  const send = sheet.getByRole("button", { name: "Kirim foto" });
  await expectTouchTarget(other);
  await expectTouchTarget(send);
  await attachViewportScreenshot(page, testInfo, "proof-sheet");

  // Sending: held at our own API until the sheet is looked at, so the sending state shows. The
  // sheet does not close meanwhile.
  let release: () => void = () => undefined;
  const held = new Promise<void>((done) => {
    release = done;
  });
  await page.route(`**/api${path}/proof`, async (route) => {
    await held;
    await route.continue();
  });
  const posted = page.waitForRequest((request) => isProofPost(request, path));
  const answered = page.waitForResponse((response) =>
    isProofPost(response.request(), path),
  );
  await send.click();
  const request = await posted;
  await expect(sheet.getByRole("button", { name: "Mengirim…" })).toBeDisabled();
  await expect(sheet.getByText(/^Mengirim… \d{1,3}%$/)).toBeVisible();
  await expect(sheet.getByRole("progressbar")).toBeVisible();
  await expect(other).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(sheet).toBeVisible();
  await attachViewportScreenshot(page, testInfo, "proof-sending");

  // The page sent a JPEG it shrank itself, as one multipart part named file.
  const body = request.postDataBuffer()?.toString("latin1") ?? "";
  expect(body).toContain('name="file"; filename="bukti-transfer.jpg"');
  expect(body).toContain("Content-Type: image/jpeg");
  expect(request.postDataBuffer()?.length ?? 0).toBeLessThan(photo.length);

  release();
  const response = await answered;
  expect(response.status()).toBe(201);
  const proof = (await response.json()) as PaymentProof;
  await page.unroute(`**/api${path}/proof`);

  // Board 3: the sheet closes, the line says it was received, and the API's list shows it.
  await expect(sheet).toHaveCount(0);
  // A screen reader hears it from a live region; the eye reads the banner above the list.
  await expect(
    page.locator("[aria-live=polite]").filter({ hasText: SENT_TEXT }),
  ).toHaveCount(1);
  await expect(page.getByText(SENT_TEXT).last()).toBeVisible();
  const list = page.getByRole("region", { name: LIST_TITLE });
  const sentLine = `Dikirim ${formatTimestampShortWeekdayDayMonthTime(proof.createdAt)}`;
  const row = list.getByRole("listitem");
  await expect(row).toHaveCount(1);
  await expect(row).toContainText(sentLine);
  await expect(row).toContainText("Menunggu dicek admin");
  await expect(list.getByText(LIST_NOTE)).toBeVisible();
  await expect(list.getByRole("img")).toHaveCount(0);
  expect(await fontSize(page, "Menunggu dicek admin")).toBeGreaterThanOrEqual(
    minBodyFontPx(testInfo),
  );
  await page.evaluate(() => window.scrollTo(0, 0));
  await attachViewportScreenshot(page, testInfo, "proof-sent");

  // The bill is unchanged: sending a proof records no payment.
  const after = await readPage(page, path);
  expect(after.outstanding).toBe(fixture.expected.outstanding);
  expect(after.proofsWaiting.map((each) => each.id)).toEqual([proof.id]);

  // Opening the link again later still lists it (the line is for this visit only).
  await page.reload();
  const again = page.getByRole("region", { name: LIST_TITLE });
  await expect(again.getByRole("listitem")).toHaveCount(1);
  await expect(again.getByRole("listitem")).toContainText(sentLine);
  await expect(page.getByText(SENT_TEXT)).toHaveCount(0);

  // The admin finds it in Antrean cek, as the tenant's proof.
  const item = await waitForCleanProof(fixture.api, rented.orgId, proof.id);
  expect(item.transferProof?.tenant.id).toBe(rented.tenant.id);
  await fixture.api.dispose();

  expect(fixture.outside).toEqual([]);
  expect(fixture.leaks).toEqual([]);
});

test("a file that is not a photo gets the API's message, and a dropped connection resends the same photo", async ({
  page,
}, testInfo) => {
  const fixture = await openTransferTab(page, testInfo, 2);
  const { path } = fixture;
  const transfer = page.getByRole("region", { name: "Transfer bank" });
  await expect(
    transfer.getByRole("button", { name: "Kirim bukti transfer" }),
  ).toBeVisible();
  // The hidden input behind the button (the file chooser is tested in the first test).
  const input = page.locator('input[type="file"]');

  // A text file named like a photo: the browser cannot show it, the API refuses it by content.
  await input.setInputFiles({
    name: "bukti.jpg",
    mimeType: "image/jpeg",
    buffer: Buffer.from("ini bukan foto, hanya teks"),
  });
  const sheet = page.getByRole("dialog", { name: "Kirim bukti transfer" });
  await expect(
    sheet.getByText("Foto ini tidak bisa ditampilkan di sini."),
  ).toBeVisible();
  const send = sheet.getByRole("button", { name: "Kirim foto" });
  const refused = page.waitForResponse((response) =>
    isProofPost(response.request(), path),
  );
  await send.click();
  expect((await refused).status()).toBe(400);
  await expect(sheet.getByRole("alert")).toHaveText(TYPE_REFUSED);
  await expect(send).toBeDisabled();
  await attachViewportScreenshot(page, testInfo, "proof-refused");

  // "Pilih foto lain" takes a real photo; the message goes.
  const chooser = page.waitForEvent("filechooser");
  await sheet.getByRole("button", { name: "Pilih foto lain" }).click();
  await (
    await chooser
  ).setFiles({
    name: "bukti.png",
    mimeType: "image/png",
    buffer: await largeTransferPhoto(
      page,
      formatRupiah(fixture.expected.outstanding),
    ),
  });
  await expect(
    sheet.getByRole("img", { name: "Foto bukti transfer yang dipilih" }),
  ).toBeVisible();
  await expect(sheet.getByRole("alert")).toHaveCount(0);

  // The signal drops: "Gagal terkirim." and "Coba lagi" sends the same photo.
  await page.route(`**/api${path}/proof`, (route) =>
    route.abort("internetdisconnected"),
  );
  await send.click();
  await expect(sheet.getByText("Gagal terkirim.")).toBeVisible();
  const retry = sheet.getByRole("button", { name: "Coba lagi" });
  await expect(retry).toBeEnabled();
  await page.unroute(`**/api${path}/proof`);
  const answered = page.waitForResponse((response) =>
    isProofPost(response.request(), path),
  );
  await retry.click();
  const response = await answered;
  expect(response.status()).toBe(201);
  const proof = (await response.json()) as PaymentProof;

  await expect(sheet).toHaveCount(0);
  await expect(
    page.locator("[aria-live=polite]").filter({ hasText: SENT_TEXT }),
  ).toHaveCount(1);
  await expect(
    page.getByRole("region", { name: LIST_TITLE }).getByRole("listitem"),
  ).toHaveCount(1);
  // Only the photo was received; the refused file left nothing behind.
  const after = await readPage(page, path);
  expect(after.proofsWaiting.map((each) => each.id)).toEqual([proof.id]);
  await fixture.api.dispose();

  expect(fixture.outside).toEqual([]);
  expect(fixture.leaks).toEqual([]);
});
