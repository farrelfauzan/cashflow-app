import {
  expect,
  test,
  type Locator,
  type Page,
  type Request,
  type TestInfo,
} from "@playwright/test";

import { RECEIPTS_LABEL } from "../src/components/client/deposit/utils";
import { LIST_LABEL } from "../src/components/client/field-today/utils";
import {
  RETAKE_EXPLAIN,
  RETAKE_TITLE,
  RETAKE_UNKNOWN_TEXT,
} from "../src/components/client/receipt-retake/utils";
import type {
  AttachCashPaymentPhotoInput,
  CashPaymentInput,
  CashPaymentResult,
  FieldCashPayment,
  FieldTodayItem,
  FieldTodayList,
  ReceiptPhoto,
  UndepositedCashList,
  Upload,
} from "../src/lib/api/generated/model";
import { formatRupiah } from "../src/lib/format/rupiah";
import { cashPhoto } from "./support/cash-photo";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { infectedPhoto, transferPhoto } from "./support/proof-fixtures";
import {
  minBodyFontPx,
  projectName,
  type ProjectName,
} from "./support/projects";
import { FIELD_TODAY_URL } from "./support/shell";
import { BAGUS, MIRA, NURUL, type SeededStaff } from "./support/staff";
import { expectToast } from "./support/toast";

// "Ambil ulang" (decision 46.10, board HP-Foto, "Setelah disimpan"): the karyawan retakes the
// receipt photo of a cash payment whose photo the scan refused after it was saved. Against the
// real API with object storage (RustFS) and the fake upload scanner (UPLOAD_SCANNER=fake), which
// marks any file holding the EICAR test string infected: a test records cash through the API with
// an infected copy of a real JPEG, waits until listFieldToday reports that payment's photo
// infected, then retakes it in the UI with a clean photo.
//
// Each size project signs in as its own karyawan of Kos Kunjungan Asri (e2e/support/staff.ts),
// whose cash no other spec records, so nothing here moves the totals the Kos Bulanan Sejahtera
// field specs check. A refused photo is retaken on the karyawan's first tenant; the clean photo is
// recorded on the second. The tests of one project run in order.

type Collector = {
  staff: SeededStaff;
  // Gets the payments whose photo is refused and retaken.
  refusedTenant: string;
  // Gets the payment with a clean photo.
  cleanTenant: string;
};

const COLLECTORS: Record<ProjectName, Collector> = {
  ipad: {
    staff: NURUL,
    refusedTenant: "Nico Pranata",
    cleanTenant: "Satria Nugraha",
  },
  phone: {
    staff: BAGUS,
    refusedTenant: "Galuh Kartiko",
    cleanTenant: "Zaskia Ardiningsih",
  },
  desktop: {
    staff: MIRA,
    refusedTenant: "Ilham Sasongko",
    cleanTenant: "Tiara Anindita",
  },
};

const MIN_TOUCH_PX = 44;
const ATTACH_PATH =
  /^\/api\/orgs\/[0-9a-f-]{36}\/cash-payments\/[0-9a-f-]{36}\/photo$/;
const UPLOAD_COMPLETE_PATH =
  /^\/api\/orgs\/[0-9a-f-]{36}\/uploads\/[0-9a-f-]{36}\/complete$/;
const STORAGE_HINT =
  "The e2e job must start RustFS and give the API and the worker STORAGE_* and UPLOAD_SCANNER=fake (.github/workflows/web.yml).";
const SEED_HINT =
  "seed-demo must include Kos Kunjungan Asri (Kos Asri Kaliurang, Rp800.000 a month, every tenant overdue). Cash recorded here accumulates across runs on one database, so a tenant is paid off after many runs: seed the database again.";

function collectorOf(testInfo: TestInfo): Collector {
  return COLLECTORS[projectName(testInfo)];
}

function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

// Rp1.000 to Rp49.999, one slot per test (1 to 4) and unique per run: the seeded tenants owe
// months of rent, so many runs fit before one is paid off, and every payment of a run is told
// apart by its amount.
function amountOf(slot: 1 | 2 | 3 | 4): number {
  return Number(`${slot}${runId()}`);
}

function orgIdOf(page: Page): string {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

// Setup and checks through the API, as the signed-in member (the page's cookies).
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

async function fieldItem(
  page: Page,
  orgId: string,
  tenantName: string,
): Promise<FieldTodayItem> {
  const today = await call<FieldTodayList>(
    page,
    "GET",
    `/orgs/${orgId}/field/today?pageSize=100`,
    200,
  );
  const item = today.items.find((row) => row.tenantName === tenantName);
  if (item === undefined) {
    throw new Error(`${tenantName} is not on the list. ${SEED_HINT}`);
  }
  return item;
}

async function fieldPayment(
  page: Page,
  orgId: string,
  tenantName: string,
  paymentId: string,
): Promise<FieldCashPayment | undefined> {
  const item = await fieldItem(page, orgId, tenantName);
  return item.cashToday.payments.find((each) => each.paymentId === paymentId);
}

async function openFieldToday(page: Page): Promise<string> {
  await page.goto("/organizations");
  await expect(page).toHaveURL(FIELD_TODAY_URL);
  await expect(page.getByRole("list", { name: LIST_LABEL })).toBeVisible();
  return orgIdOf(page);
}

// Records cash with a photo of `bytes` through the API. A photo the scan refused before the cash
// was saved is refused with it (409 receipt_photo_upload_refused): an infected photo is then
// sent again with a new upload, so the cash is recorded while its scan is still pending.
async function recordCash(
  page: Page,
  orgId: string,
  tenantId: string,
  amount: number,
  bytes: Buffer,
): Promise<CashPaymentResult> {
  for (let attempt = 0; attempt < 3; attempt += 1) {
    const input: CashPaymentInput = {
      requestId: crypto.randomUUID(),
      amount,
      photoUploadId: await cashPhoto(
        page,
        page.request,
        orgId,
        tenantId,
        bytes,
      ),
    };
    const response = await page.request.post(
      `/api/orgs/${orgId}/tenants/${tenantId}/cash-payments`,
      { data: input },
    );
    const text = await response.text();
    if (
      response.status() === 409 &&
      text.includes("receipt_photo_upload_refused")
    ) {
      continue;
    }
    expect(response.status(), text).toBe(201);
    return JSON.parse(text) as CashPaymentResult;
  }
  throw new Error(
    "The scan refused the photo before the cash could be saved, three times",
  );
}

// Cash from the tenant whose photo the scan refused: recorded with an infected photo, then
// waited for until Tagihan hari ini reports it infected.
async function refusedPayment(
  page: Page,
  orgId: string,
  tenantName: string,
  amount: number,
): Promise<CashPaymentResult> {
  const item = await fieldItem(page, orgId, tenantName);
  expect(amount, `${tenantName} owes too little. ${SEED_HINT}`).toBeLessThan(
    item.outstanding,
  );
  const photo = infectedPhoto(await transferPhoto(page, formatRupiah(amount)));
  const result = await recordCash(page, orgId, item.tenantId, amount, photo);
  await expect
    .poll(
      async () =>
        (await fieldPayment(page, orgId, tenantName, result.payment.id))?.photo
          .status,
      {
        message: `The infected photo was not refused. ${STORAGE_HINT}`,
        timeout: 30_000,
      },
    )
    .toBe("infected");
  return result;
}

function rowOf(page: Page, tenantName: string): Locator {
  return page
    .getByRole("list", { name: LIST_LABEL })
    .getByRole("listitem")
    .filter({ hasText: tenantName });
}

// The payment's "Ambil ulang", named after its amount ("Ambil ulang foto kwitansi Tunai
// Rp45.123 · 10.42").
function retakeButton(scope: Locator | Page, amount: number): Locator {
  const rupiah = formatRupiah(amount).replace(/\./g, "\\.");
  return scope.getByRole("button", {
    name: new RegExp(`^Ambil ulang foto kwitansi Tunai ${rupiah} · `),
  });
}

function primary(sheet: Locator): Locator {
  return sheet.getByRole("button", {
    name: /^(Simpan foto|Menunggu foto…|Menyimpan…)$/,
  });
}

async function chooseCleanPhoto(page: Page, sheet: Locator, text: string) {
  await sheet.getByLabel("Ambil foto kwitansi").setInputFiles({
    name: "kwitansi.jpg",
    mimeType: "image/jpeg",
    buffer: await transferPhoto(page, text),
  });
}

function isAttach(request: Request): boolean {
  return (
    request.method() === "PUT" &&
    ATTACH_PATH.test(new URL(request.url()).pathname)
  );
}

async function fontSizeOf(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

// Opens the sheet from `button`, sends a clean photo and returns the upload the API completed.
async function openAndSendPhoto(
  page: Page,
  button: Locator,
  amount: number,
): Promise<{ sheet: Locator; upload: Upload }> {
  await button.click();
  const sheet = page.getByRole("dialog", { name: RETAKE_TITLE });
  await expect(sheet).toBeVisible();
  await expect(sheet).toContainText(`Tunai ${formatRupiah(amount)}`);
  await expect(sheet.getByText(RETAKE_EXPLAIN)).toBeVisible();
  await expect(primary(sheet)).toHaveText("Simpan foto");
  await expect(primary(sheet)).toBeDisabled();

  const completed = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      UPLOAD_COMPLETE_PATH.test(new URL(response.url()).pathname),
  );
  await chooseCleanPhoto(page, sheet, formatRupiah(amount));
  const completeResponse = await completed;
  expect(completeResponse.status()).toBe(200);
  const upload = (await completeResponse.json()) as Upload;
  await expect(
    sheet.getByRole("status").filter({ hasText: "Foto terkirim." }),
  ).toHaveCount(1);
  await expect(primary(sheet)).toBeEnabled();
  return { sheet, upload };
}

test.describe("Karyawan", () => {
  test.describe.configure({ mode: "default" });
  test.use({
    storageState: async ({}, provide, testInfo) => {
      await provide(collectorOf(testInfo).staff.storageState);
    },
  });

  test("retakes a refused photo from Tagihan hari ini", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const { refusedTenant: tenant } = collectorOf(testInfo);
    const orgId = await openFieldToday(page);
    const amount = amountOf(1);
    const recorded = await refusedPayment(page, orgId, tenant, amount);
    await page.reload();

    const row = rowOf(page, tenant);
    await expect(row.getByText("Foto ditolak", { exact: true })).toBeVisible();
    const button = retakeButton(row, amount);
    await expect(button).toHaveCount(1);
    await expect(button).toHaveText("Ambil ulang");
    const box = await button.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
    expect(await fontSizeOf(button)).toBeGreaterThanOrEqual(
      minBodyFontPx(testInfo),
    );
    const line = row.getByText(
      new RegExp(`^Tunai ${formatRupiah(amount).replace(/\./g, "\\.")} · `),
    );
    expect(await fontSizeOf(line)).toBeGreaterThanOrEqual(
      minBodyFontPx(testInfo),
    );

    const { sheet, upload } = await openAndSendPhoto(page, button, amount);
    await expect(sheet).toContainText(tenant);
    expect(
      await fontSizeOf(sheet.getByText(RETAKE_EXPLAIN)),
    ).toBeGreaterThanOrEqual(minBodyFontPx(testInfo));
    const primaryBox = await primary(sheet).boundingBox();
    expect(primaryBox?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
    await attachViewportScreenshot(page, testInfo, "receipt-retake-sent");

    const posted = page.waitForRequest(isAttach);
    const answered = page.waitForResponse((response) =>
      isAttach(response.request()),
    );
    await primary(sheet).click();
    const body = (await posted).postDataJSON() as AttachCashPaymentPhotoInput;
    expect(body).toEqual({ uploadId: upload.id });
    const response = await answered;
    expect(response.status()).toBe(200);
    const photo = (await response.json()) as ReceiptPhoto;
    expect(photo.uploadId).toBe(upload.id);
    expect(["pending", "clean"]).toContain(photo.status);

    await expect(sheet).toBeHidden();
    await expectToast(page, "Foto kwitansi diganti", "create", tenant);

    // The API now reports the new photo, and the row no longer offers to retake it.
    const after = await fieldPayment(page, orgId, tenant, recorded.payment.id);
    expect(after?.photo.uploadId).toBe(upload.id);
    expect(["pending", "clean"]).toContain(after?.photo.status);
    await expect(retakeButton(row, amount)).toHaveCount(0);
    expect(outsideRequests).toEqual([]);
  });

  test("offers no Ambil ulang on a pending or clean photo", async ({
    page,
  }, testInfo) => {
    const { cleanTenant: tenant } = collectorOf(testInfo);
    const orgId = await openFieldToday(page);
    const amount = amountOf(2);
    const item = await fieldItem(page, orgId, tenant);
    const result = await recordCash(
      page,
      orgId,
      item.tenantId,
      amount,
      await transferPhoto(page, formatRupiah(amount)),
    );
    await page.reload();
    const row = rowOf(page, tenant);
    await expect(row).toBeVisible();
    // Pending or already clean: either way, nothing to retake.
    await expect(retakeButton(row, amount)).toHaveCount(0);

    await expect
      .poll(
        async () =>
          (await fieldPayment(page, orgId, tenant, result.payment.id))?.photo
            .status,
        {
          message: `The photo was not scanned. ${STORAGE_HINT}`,
          timeout: 30_000,
        },
      )
      .toBe("clean");
    await page.reload();
    await expect(rowOf(page, tenant)).toBeVisible();
    await expect(retakeButton(rowOf(page, tenant), amount)).toHaveCount(0);
  });

  test("attaches one photo when the answer to Simpan foto is lost", async ({
    page,
  }, testInfo) => {
    const { refusedTenant: tenant } = collectorOf(testInfo);
    const orgId = await openFieldToday(page);
    const amount = amountOf(3);
    const recorded = await refusedPayment(page, orgId, tenant, amount);
    await page.reload();

    const { sheet, upload } = await openAndSendPhoto(
      page,
      retakeButton(rowOf(page, tenant), amount),
      amount,
    );

    // The request reaches the API and attaches the photo; its answer never reaches the phone.
    let firstStatus = 0;
    const matches = (url: URL) => ATTACH_PATH.test(url.pathname);
    await page.route(matches, async (route) => {
      if (route.request().method() !== "PUT") {
        await route.fallback();
        return;
      }
      const response = await route.fetch();
      firstStatus = response.status();
      await route.abort("internetdisconnected");
    });
    const firstPut = page.waitForRequest(isAttach);
    await primary(sheet).click();
    const first = (
      await firstPut
    ).postDataJSON() as AttachCashPaymentPhotoInput;
    await expect(sheet.getByText(RETAKE_UNKNOWN_TEXT)).toBeVisible();
    expect(firstStatus).toBe(200);
    await page.unroute(matches);

    // The photo stays as sent; only Simpan foto again.
    await expect(
      sheet.getByRole("button", { name: "Ganti foto kwitansi" }),
    ).toHaveCount(0);
    await expect(primary(sheet)).toHaveText("Simpan foto");
    await expect(primary(sheet)).toBeEnabled();

    const secondPut = page.waitForRequest(isAttach);
    const answered = page.waitForResponse((response) =>
      isAttach(response.request()),
    );
    await primary(sheet).click();
    expect((await secondPut).postDataJSON()).toEqual(first);
    expect(first).toEqual({ uploadId: upload.id });
    const response = await answered;
    // The same upload again changes nothing.
    expect(response.status()).toBe(200);
    expect(((await response.json()) as ReceiptPhoto).uploadId).toBe(upload.id);
    await expect(sheet).toBeHidden();
    await expectToast(page, "Foto kwitansi diganti", "create", tenant);

    const after = await fieldPayment(page, orgId, tenant, recorded.payment.id);
    expect(after?.photo.uploadId).toBe(upload.id);
    await expect(retakeButton(rowOf(page, tenant), amount)).toHaveCount(0);
  });

  test("retakes a refused photo from Setor tunai", async ({
    page,
  }, testInfo) => {
    // Mas Bagus on the karyawan's own device; the flow is the same sheet at every size.
    test.skip(projectName(testInfo) !== "phone", "Setor tunai on the phone");
    const { refusedTenant: tenant } = collectorOf(testInfo);
    const orgId = await openFieldToday(page);
    const amount = amountOf(4);
    const recorded = await refusedPayment(page, orgId, tenant, amount);

    await page.goto(`/o/${orgId}/field/deposit`);
    const receipts = page.getByRole("list", { name: RECEIPTS_LABEL });
    await expect(receipts).toBeVisible();
    const receipt = receipts
      .getByRole("listitem")
      .filter({ has: retakeButton(page, amount) });
    await expect(receipt).toHaveCount(1);
    await expect(
      receipt.getByText("Foto ditolak", { exact: true }),
    ).toBeVisible();
    const button = retakeButton(receipt, amount);
    const box = await button.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_PX);

    const { sheet, upload } = await openAndSendPhoto(page, button, amount);
    const answered = page.waitForResponse((response) =>
      isAttach(response.request()),
    );
    await primary(sheet).click();
    expect((await answered).status()).toBe(200);
    await expect(sheet).toBeHidden();
    await expectToast(page, "Foto kwitansi diganti", "create", tenant);
    await expect(retakeButton(receipts, amount)).toHaveCount(0);

    const held = await call<UndepositedCashList>(
      page,
      "GET",
      `/orgs/${orgId}/field/cash/payments?pageSize=100`,
      200,
    );
    const payment = held.items.find(
      (each) => each.paymentId === recorded.payment.id,
    );
    expect(payment?.photo.uploadId).toBe(upload.id);
    expect(["pending", "clean"]).toContain(payment?.photo.status);
  });
});
