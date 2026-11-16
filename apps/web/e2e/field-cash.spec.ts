import {
  expect,
  test,
  type Locator,
  type Page,
  type Request,
  type Route,
  type TestInfo,
} from "@playwright/test";

import { invoiceName } from "../src/components/client/billing/utils";
import {
  CLOSE_LABEL,
  PHOTO_FAILED_TITLE,
  PHOTO_NETWORK_TEXT,
  PHOTO_PROGRESS_LABEL,
  PHOTO_REFUSED_TITLE,
  RECEIVE_CASH_BUTTON_TEXT,
  RECEIVE_CASH_TITLE,
  UNKNOWN_SAVE_TEXT,
  WAITING_PHOTO_TEXT,
} from "../src/components/client/field-cash/utils";
import { UNSAVED_CHANGES_TITLE } from "../src/components/client/form-sheet/utils";
import {
  LIST_LABEL,
  owedText,
} from "../src/components/client/field-today/utils";
import type {
  CashPaymentInput,
  CashPaymentResult,
  CashTotal,
  FieldTodayItem,
  FieldTodayList,
  OrgMeResponse,
  TenantList,
  UndepositedCashList,
  Upload,
} from "../src/lib/api/generated/model";
import { formatRupiah } from "../src/lib/format/rupiah";
import { cashPhoto } from "./support/cash-photo";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { transferPhoto } from "./support/proof-fixtures";
import { projectName, type ProjectName } from "./support/projects";
import { FIELD_TODAY_URL, ORG_LANDING_URL } from "./support/shell";
import { FITRI, GALIH, HENDRA, LINA, type SeededStaff } from "./support/staff";

// "Terima tunai" (board HP-Tagihan, columns 2 and 3; photo states on board HP-Foto), against the
// real API with object storage (RustFS) and the fake upload scanner. Cash is never recorded
// without its receipt photo (decision 50): the photo goes to storage first, and Simpan sends its
// upload id.
//
// The cash a karyawan holds is one total per member, and cash-deposit.spec.ts deposits all of
// Mas Rudi's on the phone. So this spec never uses Mas Rudi: each size project records cash as
// its own karyawan of Kos Bulanan Sejahtera (e2e/support/staff.ts), from tenants no other spec
// touches, and can check his held cash exactly. The tests of one project change that one total,
// so they run one after another (mode "default"); the projects still run side by side.

type Collector = {
  staff: SeededStaff;
  // Owes for many months: every test takes a small part, so many runs a day fit.
  tenant: string;
  // Paid in full by the "Lunas" test (once a day; later runs find the row already Lunas).
  paidTenant: string;
};

const COLLECTORS: Record<ProjectName, Collector> = {
  ipad: {
    staff: GALIH,
    tenant: "Indah Kusuma Wardani",
    paidTenant: "Joni Hermawan",
  },
  phone: {
    staff: LINA,
    tenant: "Warung Makan Bu Tini",
    paidTenant: "Tegar Wijaya",
  },
  desktop: {
    staff: FITRI,
    tenant: "Irfan Hakim",
    paidTenant: "Hadi Susanto",
  },
};

const GRIYA = "Griya Sejahtera";
const CASH_PAYMENTS_PATH =
  /^\/api\/orgs\/[0-9a-f-]{36}\/tenants\/[0-9a-f-]{36}\/cash-payments$/;
const UPLOAD_COMPLETE_PATH =
  /^\/api\/orgs\/[0-9a-f-]{36}\/uploads\/[0-9a-f-]{36}\/complete$/;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

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

// A partial amount of this run, one per test (slot 1 to 3): Rp10.000 to Rp39.999, so a tenant
// who owes a few months of rent pays only a small part each run and lasts for dozens of runs,
// and every payment of a run is told apart by its amount.
function amountOf(slot: 1 | 2 | 3): number {
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
  if (item === undefined) throw new Error(`${tenantName} is not on the list`);
  return item;
}

async function heldCash(page: Page, orgId: string): Promise<CashTotal> {
  return call<CashTotal>(page, "GET", `/orgs/${orgId}/field/cash`, 200);
}

// The karyawan's undeposited cash payments from the tenant at this amount.
async function heldPayments(
  page: Page,
  orgId: string,
  tenantName: string,
  amount: number,
) {
  const held = await call<UndepositedCashList>(
    page,
    "GET",
    `/orgs/${orgId}/field/cash/payments?pageSize=100`,
    200,
  );
  return held.items.filter(
    (item) => item.tenantName === tenantName && item.amount === amount,
  );
}

// Tagihan hari ini, where his one organization opens.
async function openFieldToday(page: Page): Promise<string> {
  await page.goto("/organizations");
  await expect(page).toHaveURL(FIELD_TODAY_URL);
  await expect(page.getByRole("list", { name: LIST_LABEL })).toBeVisible();
  return orgIdOf(page);
}

function rowOf(page: Page, tenantName: string): Locator {
  return page
    .getByRole("list", { name: LIST_LABEL })
    .getByRole("listitem")
    .filter({ hasText: tenantName });
}

async function openSheet(page: Page, tenantName: string): Promise<Locator> {
  await rowOf(page, tenantName)
    .getByRole("button", { name: RECEIVE_CASH_BUTTON_TEXT })
    .click();
  const sheet = page.getByRole("dialog", { name: RECEIVE_CASH_TITLE });
  await expect(sheet).toBeVisible();
  return sheet;
}

// The sheet's one primary: "Simpan", "Simpan Rp…", "Menunggu foto…" or "Menyimpan…".
function primary(sheet: Locator): Locator {
  return sheet.getByRole("button", {
    name: /^(Simpan( Rp[\d.]+)?|Menunggu foto…|Menyimpan…)$/,
  });
}

function photoInput(sheet: Locator): Locator {
  return sheet.getByLabel("Ambil foto kwitansi");
}

async function choosePhoto(page: Page, sheet: Locator, text: string) {
  const buffer = await transferPhoto(page, text);
  await photoInput(sheet).setInputFiles({
    name: "kwitansi.jpg",
    mimeType: "image/jpeg",
    buffer,
  });
}

function isCashPost(request: Request): boolean {
  return (
    request.method() === "POST" &&
    CASH_PAYMENTS_PATH.test(new URL(request.url()).pathname)
  );
}

// Routes the storage host only (any URL whose origin is not the app's); everything else
// falls through.
async function routeStorage(
  page: Page,
  handler: (route: Route) => Promise<void>,
) {
  const appOrigin = new URL(page.url()).origin;
  const matches = (url: URL) => url.origin !== appOrigin;
  await page.route(matches, async (route) => {
    if (route.request().method() !== "PUT") {
      await route.fallback();
      return;
    }
    await handler(route);
  });
  return () => page.unroute(matches);
}

test.describe("Karyawan", () => {
  test.describe.configure({ mode: "default" });
  test.use({
    // Each size project signs in as its own karyawan.
    storageState: async ({}, provide, testInfo) => {
      await provide(collectorOf(testInfo).staff.storageState);
    },
  });

  test("records part of what is owed with its receipt photo", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const { tenant } = collectorOf(testInfo);
    const orgId = await openFieldToday(page);
    const me = await call<OrgMeResponse>(page, "GET", `/orgs/${orgId}/me`, 200);
    expect(
      me.capabilities.find((each) => each.action === "cash:record")?.access,
    ).toBe("conditional");

    const before = await fieldItem(page, orgId, tenant);
    const amount = amountOf(1);
    expect(amount, `${tenant} must owe more than one run takes`).toBeLessThan(
      before.outstanding,
    );

    const sheet = await openSheet(page, tenant);
    await expect(sheet).toContainText(formatRupiah(before.outstanding));
    // No photo: Simpan cannot be pressed (decision 50).
    await expect(primary(sheet)).toHaveText("Simpan");
    await expect(primary(sheet)).toBeDisabled();

    await sheet.getByLabel("Uang yang diterima").fill(String(amount));
    await expect(primary(sheet)).toHaveText(`Simpan ${formatRupiah(amount)}`);
    await expect(primary(sheet)).toBeDisabled();

    // The PUT is held, so the photo's sending state can be seen.
    let releasePut = () => {};
    const putHeld = new Promise<void>((done) => {
      releasePut = done;
    });
    const unrouteStorage = await routeStorage(page, async (route) => {
      await putHeld;
      await route.continue();
    });
    const completed = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        UPLOAD_COMPLETE_PATH.test(new URL(response.url()).pathname),
    );
    await choosePhoto(page, sheet, formatRupiah(amount));
    await expect(
      sheet.getByRole("progressbar", { name: PHOTO_PROGRESS_LABEL }),
    ).toBeVisible();
    await expect(primary(sheet)).toHaveText(WAITING_PHOTO_TEXT);
    await expect(primary(sheet)).toBeDisabled();
    releasePut();

    const completeResponse = await completed;
    expect(completeResponse.status()).toBe(200);
    const upload = (await completeResponse.json()) as Upload;
    await unrouteStorage();
    await expect(
      sheet.getByRole("status").filter({ hasText: "Foto terkirim." }),
    ).toHaveCount(1);
    await expect(primary(sheet)).toHaveText(`Simpan ${formatRupiah(amount)}`);
    await expect(primary(sheet)).toBeEnabled();
    const box = await primary(sheet).boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    await attachViewportScreenshot(page, testInfo, "field-cash-photo-sent");

    const posted = page.waitForRequest(isCashPost);
    const answered = page.waitForResponse((response) =>
      isCashPost(response.request()),
    );
    await primary(sheet).click();
    const body = (await posted).postDataJSON() as CashPaymentInput;
    expect(body.amount).toBe(amount);
    expect(body.photoUploadId).toBe(upload.id);
    expect(body.requestId).toMatch(UUID);
    const response = await answered;
    expect(response.status()).toBe(201);
    const result = (await response.json()) as CashPaymentResult;

    // "Tersimpan": every figure is the API's answer.
    await expect(
      sheet.getByText(`${formatRupiah(result.payment.amount)} tercatat`),
    ).toBeVisible();
    await expect(sheet.getByText("Dipakai untuk")).toBeVisible();
    const paid = sheet
      .getByRole("list", { name: "Tagihan yang dibayar" })
      .getByRole("listitem");
    expect(result.paid.length).toBeGreaterThan(0);
    await expect(paid).toHaveCount(result.paid.length);
    for (const [index, invoice] of result.paid.entries()) {
      await expect(paid.nth(index)).toContainText(invoiceName(invoice));
      await expect(paid.nth(index)).toContainText(formatRupiah(invoice.amount));
    }
    const remaining = sheet
      .locator("dl > div")
      .filter({ hasText: "Sisa yang belum dibayar" });
    await expect(remaining.locator("dd")).toHaveText(
      formatRupiah(result.outstanding),
    );
    expect(result.outstanding).toBe(before.outstanding - amount);
    await expect(sheet.getByText("Foto kwitansi tersimpan")).toBeVisible();
    await attachViewportScreenshot(page, testInfo, "field-cash-recorded");

    await sheet.getByRole("button", { name: "Selesai" }).click();
    await expect(sheet).toBeHidden();

    // The row and the cash card show the API's numbers after the payment.
    const after = await fieldItem(page, orgId, tenant);
    expect(after.outstanding).toBe(result.outstanding);
    const row = rowOf(page, tenant);
    await expect(row.getByText(owedText(after), { exact: true })).toBeVisible();
    await expect(row).toContainText(
      `Tunai hari ini ${formatRupiah(after.cashToday.amount)}`,
    );
    const cash = await heldCash(page, orgId);
    const card = page.getByRole("region", { name: "Tunai di tangan" });
    await expect(card).toContainText(formatRupiah(cash.amount));
    await expect(card).toContainText(`${cash.count} kwitansi`);
    expect(outsideRequests).toEqual([]);
  });

  test("never saves while the photo is still being sent", async ({
    page,
  }, testInfo) => {
    const { tenant } = collectorOf(testInfo);
    const orgId = await openFieldToday(page);
    const cashBefore = await heldCash(page, orgId);
    const cashPosts: string[] = [];
    page.on("request", (request) => {
      if (isCashPost(request)) cashPosts.push(request.url());
    });

    const sheet = await openSheet(page, tenant);
    await sheet.getByLabel("Uang yang diterima").fill("50000");
    let releasePut = () => {};
    const putHeld = new Promise<void>((done) => {
      releasePut = done;
    });
    const unrouteStorage = await routeStorage(page, async (route) => {
      await putHeld;
      await route.continue();
    });
    await choosePhoto(page, sheet, "Rp50.000");
    await expect(
      sheet.getByRole("progressbar", { name: PHOTO_PROGRESS_LABEL }),
    ).toBeVisible();
    await expect(primary(sheet)).toHaveText(WAITING_PHOTO_TEXT);
    await expect(primary(sheet)).toBeDisabled();
    // Enter in the amount submits the form: it must not record anything either.
    await sheet.getByLabel("Uang yang diterima").press("Enter");
    await expect(primary(sheet)).toBeDisabled();

    releasePut();
    await expect(primary(sheet)).toHaveText("Simpan Rp50.000");
    await expect(primary(sheet)).toBeEnabled();
    await unrouteStorage();
    // Nothing was sent while waiting; the sheet is left unsaved.
    expect(cashPosts).toEqual([]);
    expect(await heldCash(page, orgId)).toEqual(cashBefore);
  });

  test("offers only Coba lagi when the signal drops during the photo", async ({
    page,
  }, testInfo) => {
    const { tenant } = collectorOf(testInfo);
    const orgId = await openFieldToday(page);
    const amount = amountOf(2);
    // Counted before and after, so a rerun in the same minute (same amount) still checks one.
    const heldBefore = (await heldPayments(page, orgId, tenant, amount)).length;
    const sheet = await openSheet(page, tenant);
    await sheet.getByLabel("Uang yang diterima").fill(String(amount));

    const unrouteStorage = await routeStorage(page, (route) =>
      route.abort("internetdisconnected"),
    );
    await choosePhoto(page, sheet, formatRupiah(amount));
    await expect(
      sheet.getByText(PHOTO_FAILED_TITLE, { exact: true }),
    ).toBeVisible();
    await expect(
      sheet.getByText(PHOTO_NETWORK_TEXT, { exact: true }),
    ).toBeVisible();
    await expect(primary(sheet)).toBeDisabled();
    await expect(
      sheet.getByRole("button", { name: "Pilih foto lain" }),
    ).toBeVisible();
    await unrouteStorage();

    await sheet.getByRole("button", { name: "Coba lagi" }).click();
    await expect(
      sheet.getByRole("status").filter({ hasText: "Foto terkirim." }),
    ).toHaveCount(1);
    await expect(primary(sheet)).toBeEnabled();

    const answered = page.waitForResponse((response) =>
      isCashPost(response.request()),
    );
    await primary(sheet).click();
    expect((await answered).status()).toBe(201);
    await expect(
      sheet.getByText(`${formatRupiah(amount)} tercatat`),
    ).toBeVisible();
    await sheet.getByRole("button", { name: "Selesai" }).click();
    await expect(sheet).toBeHidden();
    expect(await heldPayments(page, orgId, tenant, amount)).toHaveLength(
      heldBefore + 1,
    );
  });

  test("shows the API's refusal of a photo it cannot take", async ({
    page,
  }, testInfo) => {
    // The browser makes a chosen photo ready (lib/photo) and sends one it cannot decode as it
    // is, so the API judges it. An iPhone HEIC on a browser without HEIC support is that case:
    // createUpload refuses its type, and only another photo helps.
    const { tenant } = collectorOf(testInfo);
    await openFieldToday(page);
    const sheet = await openSheet(page, tenant);
    await sheet.getByLabel("Uang yang diterima").fill("75000");
    const refused = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        /\/uploads$/.test(new URL(response.url()).pathname),
    );
    await photoInput(sheet).setInputFiles({
      name: "IMG_0012.HEIC",
      mimeType: "image/heic",
      buffer: Buffer.concat([
        Buffer.from([0, 0, 0, 24]),
        Buffer.from("ftypheic", "latin1"),
        Buffer.alloc(64),
      ]),
    });
    const response = await refused;
    expect(response.status()).toBe(400);
    const { error } = (await response.json()) as {
      error: { code: string; message: string };
    };
    expect(error.code).toBe("upload_type_not_allowed");
    await expect(sheet.getByText(PHOTO_REFUSED_TITLE)).toBeVisible();
    await expect(sheet.getByText(error.message)).toBeVisible();
    await expect(sheet.getByRole("button", { name: "Coba lagi" })).toHaveCount(
      0,
    );
    await expect(primary(sheet)).toBeDisabled();

    // "Pilih foto lain" opens the same picker; a real JPEG fixes it.
    const chooser = page.waitForEvent("filechooser");
    await sheet.getByRole("button", { name: "Pilih foto lain" }).click();
    await (
      await chooser
    ).setFiles({
      name: "kwitansi.jpg",
      mimeType: "image/jpeg",
      buffer: await transferPhoto(page, "Rp75.000"),
    });
    await expect(
      sheet.getByRole("status").filter({ hasText: "Foto terkirim." }),
    ).toHaveCount(1);
    await expect(sheet.getByText(error.message)).toHaveCount(0);
    await expect(primary(sheet)).toHaveText("Simpan Rp75.000");
    await expect(primary(sheet)).toBeEnabled();
  });

  test("records once when the answer to Simpan is lost", async ({
    page,
  }, testInfo) => {
    const { tenant } = collectorOf(testInfo);
    const orgId = await openFieldToday(page);
    const amount = amountOf(3);
    // Counted before and after, so a rerun in the same minute (same amount) still checks one.
    const heldBefore = (await heldPayments(page, orgId, tenant, amount)).length;
    const cashBefore = await heldCash(page, orgId);
    const sheet = await openSheet(page, tenant);
    await sheet.getByLabel("Uang yang diterima").fill(String(amount));
    await choosePhoto(page, sheet, formatRupiah(amount));
    await expect(primary(sheet)).toBeEnabled();

    // The request reaches the API and records the cash; its answer never reaches the phone.
    let firstStatus = 0;
    const matches = (url: URL) => CASH_PAYMENTS_PATH.test(url.pathname);
    await page.route(matches, async (route) => {
      if (route.request().method() !== "POST") {
        await route.fallback();
        return;
      }
      const response = await route.fetch();
      firstStatus = response.status();
      await route.abort("internetdisconnected");
    });
    const firstPost = page.waitForRequest(isCashPost);
    await primary(sheet).click();
    const first = (await firstPost).postDataJSON() as CashPaymentInput;
    await expect(sheet.getByText(UNKNOWN_SAVE_TEXT)).toBeVisible();
    expect(firstStatus).toBe(201);
    await page.unroute(matches);

    // Amount and photo stay as sent; only Simpan again.
    await expect(sheet.getByLabel("Uang yang diterima")).toBeDisabled();
    await expect(sheet.getByLabel("Uang yang diterima")).toHaveValue(
      formatRupiah(amount).replace(/^Rp/, ""),
    );
    await expect(
      sheet.getByRole("button", { name: "Ganti foto kwitansi" }),
    ).toHaveCount(0);
    await expect(primary(sheet)).toHaveText(`Simpan ${formatRupiah(amount)}`);
    await expect(primary(sheet)).toBeEnabled();

    // He closes the unsure sheet, and the list's reload fails too (still no signal), so the row
    // still offers Terima tunai. Pressing it again brings back the same opening, still locked:
    // a fresh one would get a new requestId and record the same cash twice.
    const listPath = (url: URL) => /\/field\/today$/.test(url.pathname);
    await page.route(listPath, (route) => route.abort("internetdisconnected"));
    await sheet.getByRole("button", { name: CLOSE_LABEL }).first().click();
    // The opening is kept, so even on the phone closing it does not ask to discard.
    await expect(
      page.getByRole("alertdialog", { name: UNSAVED_CHANGES_TITLE }),
    ).toHaveCount(0);
    await expect(sheet).toBeHidden();
    await page.unroute(listPath);
    const reopened = await openSheet(page, tenant);
    await expect(reopened.getByText(UNKNOWN_SAVE_TEXT)).toBeVisible();
    await expect(reopened.getByLabel("Uang yang diterima")).toBeDisabled();
    await expect(reopened.getByLabel("Uang yang diterima")).toHaveValue(
      formatRupiah(amount).replace(/^Rp/, ""),
    );
    await expect(primary(reopened)).toBeEnabled();

    const secondPost = page.waitForRequest(isCashPost);
    const answered = page.waitForResponse((response) =>
      isCashPost(response.request()),
    );
    await primary(sheet).click();
    expect((await secondPost).postDataJSON()).toEqual(first);
    const response = await answered;
    // Already recorded under this requestId: nothing recorded twice.
    expect(response.status()).toBe(200);
    await expect(
      sheet.getByText(`${formatRupiah(amount)} tercatat`),
    ).toBeVisible();
    await sheet.getByRole("button", { name: "Selesai" }).click();
    await expect(sheet).toBeHidden();

    expect(await heldPayments(page, orgId, tenant, amount)).toHaveLength(
      heldBefore + 1,
    );
    const cashAfter = await heldCash(page, orgId);
    expect(cashAfter.count).toBe(cashBefore.count + 1);
    expect(cashAfter.amount).toBe(cashBefore.amount + amount);
  });

  test("offers no Terima tunai once a tenant is paid in full", async ({
    page,
  }, testInfo) => {
    const { paidTenant } = collectorOf(testInfo);
    const orgId = await openFieldToday(page);
    let item = await fieldItem(page, orgId, paidTenant);
    if (item.outstanding > 0) {
      // Paid in full through the API, with its photo; a later run today finds it Lunas.
      const tenants = await call<TenantList>(
        page,
        "GET",
        `/orgs/${orgId}/tenants?q=${encodeURIComponent(paidTenant)}`,
        200,
      );
      const tenant = tenants.items.find((each) => each.name === paidTenant);
      if (tenant === undefined) throw new Error(`${paidTenant} not seeded`);
      const input: CashPaymentInput = {
        requestId: crypto.randomUUID(),
        amount: item.outstanding,
        photoUploadId: await cashPhoto(page, page.request, orgId, tenant.id),
      };
      await call<CashPaymentResult>(
        page,
        "POST",
        `/orgs/${orgId}/tenants/${tenant.id}/cash-payments`,
        201,
        input,
      );
      item = await fieldItem(page, orgId, paidTenant);
      await page.reload();
    }
    expect(item.reason).toBe("paid_today");
    expect(item.outstanding).toBe(0);
    const row = rowOf(page, paidTenant);
    await expect(row.getByText("Lunas", { exact: true })).toBeVisible();
    await expect(
      row.getByRole("button", { name: RECEIVE_CASH_BUTTON_TEXT }),
    ).toHaveCount(0);
  });
});

test.describe("Admin without cash:record", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees no Terima tunai", async ({ page }) => {
    await page.goto("/organizations");
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name: GRIYA })
      .click();
    await expect(page).toHaveURL(ORG_LANDING_URL);
    const orgId = orgIdOf(page);
    const me = await call<OrgMeResponse>(page, "GET", `/orgs/${orgId}/me`, 200);
    expect(
      me.capabilities.find((each) => each.action === "cash:record")?.access ??
        "deny",
    ).toBe("deny");

    // A typed address of Tagihan hari ini: no way to record cash on it. (The seed has no admin
    // who collects, so his list is empty; canRecordCash's unit tests cover the check itself.)
    await page.goto(`/o/${orgId}/field`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Tagihan hari ini" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: RECEIVE_CASH_BUTTON_TEXT }),
    ).toHaveCount(0);
  });
});
