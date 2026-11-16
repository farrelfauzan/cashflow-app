import {
  expect,
  request as playwrightRequest,
  test,
  type APIRequestContext,
  type Locator,
  type Page,
  type Request,
  type Route,
  type TestInfo,
} from "@playwright/test";

import { RECEIVE_CASH_BUTTON_TEXT } from "../src/components/client/field-cash/utils";
import {
  FIELD_PROOF_BUTTON_TEXT,
  FIELD_PROOF_EXPLAIN,
  FIELD_PROOF_FAILED_TITLE,
  FIELD_PROOF_HINT,
  FIELD_PROOF_NETWORK_TEXT,
  FIELD_PROOF_PICK_NOTE,
  FIELD_PROOF_SENT_DESCRIPTION,
  FIELD_PROOF_SENT_TITLE,
  FIELD_PROOF_PICK_TEXT,
  FIELD_PROOF_TITLE,
} from "../src/components/client/field-proof/utils";
import { LIST_LABEL } from "../src/components/client/field-today/utils";
import { PROOF_PREVIEW_ALT } from "../src/components/client/payment-page/utils";
import type {
  CheckQueueItem,
  CheckQueuePage,
  FieldTodayItem,
  FieldTodayList,
  OrgMeResponse,
  UploadInput,
} from "../src/lib/api/generated/model";
import {
  animationsSettled,
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { call } from "./support/payment-page";
import { clearTenantProofs, transferPhoto } from "./support/proof-fixtures";
import {
  minBodyFontPx,
  projectName,
  type ProjectName,
} from "./support/projects";
import { FIELD_TODAY_URL } from "./support/shell";
import { DIMAS, FITRI, GALIH, LINA, type SeededStaff } from "./support/staff";
import { expectToast } from "./support/toast";

// "Bukti transfer dari penyewa" (board HP-Bukti-Karyawan, decision 49), against the real API with
// object storage (RustFS) and the fake upload scanner: the karyawan photographs a transfer a
// tenant shows them and sends it from Tagihan hari ini (createUpload transfer_proof, the PUT to
// storage, completeUpload). completeUpload puts it in the admin's Antrean cek with the karyawan as
// sentBy; the karyawan cannot read the queue, so Pak Dimas (admin of Kos Bulanan Sejahtera)
// checks it through his own API session, and takes the test's proofs off the queue again.
//
// Each size project signs in as its own karyawan and uses the tenant field-promise.spec.ts makes
// promises for. A proof never changes money, so sharing that tenant is safe; field-cash.spec.ts
// never pays them off (it takes small amounts from the desktop one, Irfan Hakim, so nothing here
// compares his balance). The desktop karyawan's only other tenant who owes is
// check-queue-proofs.spec.ts's, whose proofs that spec clears, so it cannot be used. The tests of one project share that tenant's proofs, so they run in order.

const MIN_TOUCH_PX = 44;

type Sender = {
  staff: SeededStaff;
  tenant: string;
};

const SENDERS: Record<ProjectName, Sender> = {
  ipad: { staff: GALIH, tenant: "Kurniawan Adi" },
  phone: { staff: LINA, tenant: "Sinta Maharani" },
  desktop: { staff: FITRI, tenant: "Irfan Hakim" },
};

const CREATE_UPLOAD_PATH = /^\/api\/orgs\/[0-9a-f-]{36}\/uploads$/;
const COMPLETE_UPLOAD_PATH =
  /^\/api\/orgs\/[0-9a-f-]{36}\/uploads\/([0-9a-f-]{36})\/complete$/;

function senderOf(testInfo: TestInfo): Sender {
  return SENDERS[projectName(testInfo)];
}

function orgIdOf(page: Page): string {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

async function openFieldToday(page: Page): Promise<string> {
  await page.goto("/organizations");
  await expect(page).toHaveURL(FIELD_TODAY_URL);
  await expect(page.getByRole("list", { name: LIST_LABEL })).toBeVisible();
  return orgIdOf(page);
}

async function fieldItem(
  page: Page,
  orgId: string,
  tenantName: string,
): Promise<FieldTodayItem> {
  const today = await call<FieldTodayList>(
    page.request,
    "GET",
    `/orgs/${orgId}/field/today?pageSize=100`,
    200,
  );
  const item = today.items.find((row) => row.tenantName === tenantName);
  if (item === undefined) throw new Error(`${tenantName} is not on the list`);
  return item;
}

function rowOf(page: Page, tenantName: string): Locator {
  return page
    .getByRole("list", { name: LIST_LABEL })
    .getByRole("listitem")
    .filter({ hasText: tenantName });
}

// Pak Dimas's own API session (the admin of the whole organization), for the queue.
async function adminApi(testInfo: TestInfo): Promise<APIRequestContext> {
  return playwrightRequest.newContext({
    baseURL: testInfo.project.use.baseURL,
    storageState: DIMAS.storageState,
  });
}

// The tenant's transfer proofs waiting in Antrean cek.
async function tenantProofs(
  admin: APIRequestContext,
  orgId: string,
  tenantId: string,
): Promise<CheckQueueItem[]> {
  const page = await call<CheckQueuePage>(
    admin,
    "GET",
    `/orgs/${orgId}/check-queue?kind=transfer_proof&pageSize=100`,
    200,
  );
  return page.items.filter(
    (item) => item.transferProof?.tenant.id === tenantId,
  );
}

async function openSheet(page: Page, tenantName: string): Promise<Locator> {
  await rowOf(page, tenantName)
    .getByRole("button", { name: FIELD_PROOF_BUTTON_TEXT })
    .click();
  const sheet = page.getByRole("dialog", { name: FIELD_PROOF_TITLE });
  await expect(sheet).toBeVisible();
  return sheet;
}

function parts(sheet: Locator) {
  return {
    send: sheet.getByRole("button", { name: /^(Kirim bukti|Mengirim…)$/ }),
    cancel: sheet.getByRole("button", { name: "Batal", exact: true }),
    other: sheet.getByRole("button", { name: "Pilih foto lain" }),
    close: sheet.getByRole("button", { name: "Tutup" }),
    retry: sheet.getByRole("button", { name: "Coba lagi" }),
    input: sheet.getByLabel(FIELD_PROOF_PICK_TEXT, { exact: true }),
    preview: sheet.getByRole("img", { name: PROOF_PREVIEW_ALT }),
  };
}

async function choosePhoto(page: Page, sheet: Locator, text: string) {
  const buffer = await transferPhoto(page, text);
  await parts(sheet).input.setInputFiles({
    name: "bukti.jpg",
    mimeType: "image/jpeg",
    buffer,
  });
  await expect(parts(sheet).preview).toBeVisible();
}

async function height(locator: Locator): Promise<number> {
  await animationsSettled(locator);
  const box = await locator.boundingBox();
  if (box === null) throw new Error("not visible");
  return box.height;
}

function isPost(request: Request, path: RegExp): boolean {
  return (
    request.method() === "POST" && path.test(new URL(request.url()).pathname)
  );
}

// Counts createUpload and completeUpload calls (with the uploads completed).
function trackUploads(page: Page) {
  const created: UploadInput[] = [];
  const completed: string[] = [];
  page.on("request", (request) => {
    if (isPost(request, CREATE_UPLOAD_PATH)) {
      created.push(request.postDataJSON() as UploadInput);
    }
    const match = COMPLETE_UPLOAD_PATH.exec(new URL(request.url()).pathname);
    if (request.method() === "POST" && match?.[1] !== undefined) {
      completed.push(match[1]);
    }
  });
  return { created, completed };
}

// Routes the storage host's PUT only (any URL whose origin is not the app's); everything else
// falls through.
async function routeStorage(
  page: Page,
  handler: (route: Route) => Promise<void>,
): Promise<() => Promise<void>> {
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
    storageState: async ({}, provide, testInfo) => {
      await provide(senderOf(testInfo).staff.storageState);
    },
  });

  let admin: APIRequestContext;
  let cleanup: { orgId: string; tenantId: string } | null = null;

  test.beforeEach(async ({}, testInfo) => {
    admin = await adminApi(testInfo);
  });

  test.afterEach(async () => {
    // The test's proofs leave the queue, so other runs and check-queue-proofs.spec.ts stay clean.
    if (cleanup !== null) {
      await clearTenantProofs(admin, cleanup.orgId, cleanup.tenantId);
    }
    cleanup = null;
    await admin.dispose();
  });

  test("sends the tenant's transfer proof for the admin to check", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const { staff, tenant } = senderOf(testInfo);
    const orgId = await openFieldToday(page);
    const me = await call<OrgMeResponse>(
      page.request,
      "GET",
      `/orgs/${orgId}/me`,
      200,
    );
    expect(
      me.capabilities.find((each) => each.action === "payment:proof")?.access,
    ).toBe("conditional");
    const before = await fieldItem(page, orgId, tenant);
    expect(before.outstanding).toBeGreaterThan(0);
    await clearTenantProofs(admin, orgId, before.tenantId);
    cleanup = { orgId, tenantId: before.tenantId };
    const uploads = trackUploads(page);

    // A text button under the row's buttons, never a primary.
    const row = rowOf(page, tenant);
    const rowText = await row.innerText();
    const button = row.getByRole("button", { name: FIELD_PROOF_BUTTON_TEXT });
    const receive = row.getByRole("button", { name: RECEIVE_CASH_BUTTON_TEXT });
    await button.scrollIntoViewIfNeeded();
    expect(await height(button)).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
    const buttonBox = await button.boundingBox();
    const receiveBox = await receive.boundingBox();
    expect(buttonBox?.y ?? 0).toBeGreaterThan(
      (receiveBox?.y ?? 0) + (receiveBox?.height ?? 0),
    );
    expect(
      await button.evaluate((el) => el.scrollWidth <= el.clientWidth),
    ).toBe(true);
    await attachViewportScreenshot(page, testInfo, "field-proof-row");

    // No photo yet: Kirim bukti waits for one.
    const sheet = await openSheet(page, tenant);
    const { send, cancel, other, close, preview } = parts(sheet);
    await expect(sheet).toContainText(`${tenant} · `);
    await expect(sheet.getByText(FIELD_PROOF_EXPLAIN)).toBeVisible();
    await expect(sheet.getByText(FIELD_PROOF_HINT)).toBeVisible();
    const pick = sheet.getByRole("button", {
      name: `${FIELD_PROOF_PICK_TEXT} ${FIELD_PROOF_PICK_NOTE}`,
    });
    await expect(pick).toBeVisible();
    await expect(send).toHaveText("Kirim bukti");
    await expect(send).toBeDisabled();
    await expect(cancel).toBeVisible();
    for (const control of [pick, cancel, send, close]) {
      expect(await height(control)).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
    }
    expect(
      await sheet
        .getByText(FIELD_PROOF_EXPLAIN)
        .evaluate((el) => parseFloat(getComputedStyle(el).fontSize)),
    ).toBeGreaterThanOrEqual(minBodyFontPx(testInfo));
    await attachViewportScreenshot(page, testInfo, "field-proof-sheet");

    // A chosen photo is shown, not sent.
    await choosePhoto(page, sheet, "Rp1.050.000");
    await expect(cancel).toHaveCount(0);
    await expect(other).toBeVisible();
    await expect(send).toBeEnabled();
    expect(uploads.created).toEqual([]);
    await attachViewportScreenshot(page, testInfo, "field-proof-preview");

    // The PUT is held: the percent shows and nothing closes the sheet.
    let release = () => {};
    const held = new Promise<void>((done) => {
      release = done;
    });
    let reachedPut = () => {};
    const putReached = new Promise<void>((done) => {
      reachedPut = done;
    });
    const unroute = await routeStorage(page, async (route) => {
      reachedPut();
      await held;
      await route.fallback();
    });
    await send.click();
    await putReached;
    await expect(send).toHaveText("Mengirim…");
    await expect(send).toBeDisabled();
    await expect(
      sheet.getByRole("progressbar", { name: /^Mengirim…( \d+%)?$/ }),
    ).toBeVisible();
    await expect(
      sheet.getByRole("paragraph").filter({ hasText: /^Mengirim…( \d+%)?$/ }),
    ).toBeVisible();
    await expect(other).toBeDisabled();
    await expect(close).toBeDisabled();
    await page.keyboard.press("Escape");
    await expect(sheet).toBeVisible();
    await attachViewportScreenshot(page, testInfo, "field-proof-sending");

    const completed = page.waitForResponse((response) =>
      isPost(response.request(), COMPLETE_UPLOAD_PATH),
    );
    release();
    expect((await completed).status()).toBe(200);
    await expect(sheet).toHaveCount(0);
    await expectToast(
      page,
      FIELD_PROOF_SENT_TITLE,
      "create",
      FIELD_PROOF_SENT_DESCRIPTION,
    );
    await unroute();
    await attachViewportScreenshot(page, testInfo, "field-proof-sent");

    // One upload for the tenant, completed once.
    expect(uploads.created).toEqual([
      expect.objectContaining({
        purpose: "transfer_proof",
        tenantId: before.tenantId,
        contentType: "image/jpeg",
      }),
    ]);
    expect(uploads.completed).toHaveLength(1);

    // The row does not change (no "Bukti dikirim" marker, the same amounts): the money is the
    // admin's to record. Compared on screen, since field-cash.spec.ts takes small cash amounts
    // from the desktop tenant at the same time.
    expect(await row.innerText()).toBe(rowText);
    await expect(
      row.getByRole("button", { name: FIELD_PROOF_BUTTON_TEXT }),
    ).toBeVisible();

    // The admin's queue lists it, sent by this karyawan.
    const proofs = await tenantProofs(admin, orgId, before.tenantId);
    expect(proofs).toHaveLength(1);
    expect(proofs[0]?.transferProof?.uploadId).toBe(uploads.completed[0]);
    expect(proofs[0]?.transferProof?.sentBy?.name).toBe(staff.name);
    await expect(preview).toHaveCount(0);
    expect(outsideRequests).toEqual([]);
  });

  test("sends once after the signal drops on the PUT", async ({
    page,
  }, testInfo) => {
    const { tenant } = senderOf(testInfo);
    const orgId = await openFieldToday(page);
    const item = await fieldItem(page, orgId, tenant);
    await clearTenantProofs(admin, orgId, item.tenantId);
    cleanup = { orgId, tenantId: item.tenantId };
    const uploads = trackUploads(page);

    const sheet = await openSheet(page, tenant);
    const { send, other, retry, close, preview } = parts(sheet);
    await choosePhoto(page, sheet, "Rp500.000");

    // The first PUT loses its connection; the next goes through.
    let puts = 0;
    const unroute = await routeStorage(page, async (route) => {
      puts += 1;
      if (puts === 1) {
        await route.abort("internetdisconnected");
        return;
      }
      await route.fallback();
    });
    await send.click();
    const failed = sheet.getByRole("alert").filter({
      hasText: FIELD_PROOF_FAILED_TITLE,
    });
    await expect(failed).toBeVisible();
    await expect(failed).toContainText(FIELD_PROOF_NETWORK_TEXT);
    await expect(preview).toBeVisible();
    await expect(send).toHaveText("Kirim bukti");
    await expect(send).toBeDisabled();
    await expect(other).toBeEnabled();
    await expect(close).toBeEnabled();
    expect(await height(retry)).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
    expect(uploads.completed).toEqual([]);
    await attachViewportScreenshot(page, testInfo, "field-proof-failed");

    // Coba lagi sends the same photo as a new upload; the first was never completed.
    const completed = page.waitForResponse((response) =>
      isPost(response.request(), COMPLETE_UPLOAD_PATH),
    );
    await retry.click();
    expect((await completed).status()).toBe(200);
    await expect(sheet).toHaveCount(0);
    await expectToast(
      page,
      FIELD_PROOF_SENT_TITLE,
      "create",
      FIELD_PROOF_SENT_DESCRIPTION,
    );
    await unroute();

    expect(puts).toBe(2);
    expect(uploads.created).toHaveLength(2);
    expect(uploads.created[1]).toEqual(uploads.created[0]);
    expect(uploads.completed).toHaveLength(1);
    const proofs = await tenantProofs(admin, orgId, item.tenantId);
    expect(proofs).toHaveLength(1);
    expect(proofs[0]?.transferProof?.uploadId).toBe(uploads.completed[0]);
  });

  test("completes the same upload again when only its answer was lost", async ({
    page,
  }, testInfo) => {
    const { tenant } = senderOf(testInfo);
    const orgId = await openFieldToday(page);
    const item = await fieldItem(page, orgId, tenant);
    await clearTenantProofs(admin, orgId, item.tenantId);
    cleanup = { orgId, tenantId: item.tenantId };
    const uploads = trackUploads(page);

    const sheet = await openSheet(page, tenant);
    const { send } = parts(sheet);
    await choosePhoto(page, sheet, "Rp750.000");

    // completeUpload reaches the API, which records the proof; its answer never arrives.
    let firstStatus = 0;
    const matches = (url: URL) => COMPLETE_UPLOAD_PATH.test(url.pathname);
    await page.route(matches, async (route) => {
      const response = await route.fetch();
      firstStatus = response.status();
      await route.abort("internetdisconnected");
    });
    await send.click();
    await expect(sheet.getByText(FIELD_PROOF_FAILED_TITLE)).toBeVisible();
    expect(firstStatus).toBe(200);
    await page.unroute(matches);

    // Unsure: another photo would make a second proof, so it cannot be chosen. Closing and opening
    // again brings back this same attempt, not a fresh sheet.
    const { other, close } = parts(sheet);
    await expect(other).toBeDisabled();
    await close.click();
    await expect(sheet).toHaveCount(0);
    const reopened = await openSheet(page, tenant);
    await expect(reopened.getByText(FIELD_PROOF_FAILED_TITLE)).toBeVisible();
    await expect(parts(reopened).other).toBeDisabled();

    const completed = page.waitForResponse((response) =>
      isPost(response.request(), COMPLETE_UPLOAD_PATH),
    );
    await parts(reopened).retry.click();
    expect((await completed).status()).toBe(200);
    await expect(reopened).toHaveCount(0);
    await expectToast(
      page,
      FIELD_PROOF_SENT_TITLE,
      "create",
      FIELD_PROOF_SENT_DESCRIPTION,
    );

    // One upload, completed twice: still one proof.
    expect(uploads.created).toHaveLength(1);
    expect(uploads.completed).toHaveLength(2);
    expect(uploads.completed[1]).toBe(uploads.completed[0]);
    const proofs = await tenantProofs(admin, orgId, item.tenantId);
    expect(proofs).toHaveLength(1);
    expect(proofs[0]?.transferProof?.uploadId).toBe(uploads.completed[0]);
  });
});
