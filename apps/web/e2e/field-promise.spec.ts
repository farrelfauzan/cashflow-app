import {
  expect,
  test,
  type Locator,
  type Page,
  type Request,
  type TestInfo,
} from "@playwright/test";

import {
  FIELD_NEXT_DUE_LABEL,
  FIELD_OWN_DATE_LABEL,
  FIELD_PROMISE_AMOUNT_LABEL,
  FIELD_PROMISE_DATE_LABEL,
  FIELD_PROMISE_INTRO,
  FIELD_PROMISE_LOADING_LABEL,
  RECORD_FIELD_PROMISE_TEXT,
} from "../src/components/client/field-promise/utils";
import { LIST_LABEL } from "../src/components/client/field-today/utils";
import type {
  FieldPromiseOffer,
  FieldTodayItem,
  FieldTodayList,
  MakeTenantPromiseRequest,
  OrgMeResponse,
} from "../src/lib/api/generated/model";
import { formatDate, formatWeekdayDayMonth } from "../src/lib/format/date";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  dateControl,
  expectDate,
  expectDateLimits,
  expectDayRefused,
  isCalendarField,
  setDate,
  shiftDay,
} from "./support/date-field";
import {
  animationsSettled,
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import {
  minBodyFontPx,
  projectName,
  type ProjectName,
} from "./support/projects";
import { FIELD_TODAY_URL } from "./support/shell";
import { FITRI, GALIH, LINA, type SeededStaff } from "./support/staff";
import { expectToast } from "./support/toast";

// "Catat janji bayar" (board HP-Tagihan, column 4), against the real API: the karyawan records a
// tenant's payment promise from Tagihan hari ini. The sheet reads getFieldPromiseOffer when it
// opens and saves with makeFieldPromise, recorded as the karyawan's.
//
// Each size project signs in as its own karyawan of Kos Bulanan Sejahtera (e2e/support/staff.ts)
// and uses a tenant who owes for months. A promise never changes money, so the tenants
// field-cash.spec.ts takes small amounts from can be used here too, but never the one it pays off.
// The tests of one project share that tenant's active promise, so they run in order; every run
// after the first finds the promise of the run before and replaces it.

const MIN_TOUCH_PX = 44;

type Promiser = {
  staff: SeededStaff;
  tenant: string;
};

const PROMISERS: Record<ProjectName, Promiser> = {
  ipad: { staff: GALIH, tenant: "Kurniawan Adi" },
  phone: { staff: LINA, tenant: "Sinta Maharani" },
  desktop: { staff: FITRI, tenant: "Irfan Hakim" },
};

const PROMISE_PATH =
  /^\/api\/orgs\/[0-9a-f-]{36}\/field\/tenants\/[0-9a-f-]{36}\/promises$/;

function promiserOf(testInfo: TestInfo): Promiser {
  return PROMISERS[projectName(testInfo)];
}

function orgIdOf(page: Page): string {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

async function call<T>(page: Page, path: string): Promise<T> {
  const response = await page.request.get(`/api${path}`);
  expect(response.status(), await response.text()).toBe(200);
  return (await response.json()) as T;
}

async function fieldItem(
  page: Page,
  orgId: string,
  tenantName: string,
): Promise<FieldTodayItem> {
  const today = await call<FieldTodayList>(
    page,
    `/orgs/${orgId}/field/today?pageSize=100`,
  );
  const item = today.items.find((row) => row.tenantName === tenantName);
  if (item === undefined) throw new Error(`${tenantName} is not on the list`);
  return item;
}

function offerOf(page: Page, orgId: string, tenantId: string) {
  return call<FieldPromiseOffer>(
    page,
    `/orgs/${orgId}/field/tenants/${tenantId}/promises`,
  );
}

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

function isPromiseRequest(request: Request, method: "GET" | "POST"): boolean {
  return (
    request.method() === method &&
    PROMISE_PATH.test(new URL(request.url()).pathname)
  );
}

async function openSheet(page: Page, tenantName: string): Promise<Locator> {
  await rowOf(page, tenantName)
    .getByRole("button", { name: RECORD_FIELD_PROMISE_TEXT })
    .click();
  const sheet = page.getByRole("dialog", { name: RECORD_FIELD_PROMISE_TEXT });
  await expect(sheet).toBeVisible();
  // The form shows once this opening's offer has arrived.
  await expect(sheet.getByRole("button", { name: "Batal" })).toBeVisible();
  return sheet;
}

function parts(sheet: Locator) {
  return {
    ownDate: sheet.getByRole("radio", {
      name: new RegExp(`^${FIELD_OWN_DATE_LABEL}`),
    }),
    nextDue: sheet.getByRole("radio", {
      name: new RegExp(`^${FIELD_NEXT_DUE_LABEL}`),
    }),
    amount: sheet.getByRole("textbox", { name: FIELD_PROMISE_AMOUNT_LABEL }),
    save: sheet.getByRole("button", {
      name: /^(Simpan janji|Simpan perubahan|Menyimpan…)$/,
    }),
    cancel: sheet.getByRole("button", { name: "Batal" }),
    close: sheet.getByRole("button", { name: "Tutup" }),
  };
}

async function height(locator: Locator): Promise<number> {
  await animationsSettled(locator);
  const box = await locator.boundingBox();
  if (box === null) throw new Error("not visible");
  return box.height;
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

function replacesText(offer: FieldPromiseOffer): string | null {
  const active = offer.activePromise;
  if (active === null) return null;
  const amount =
    active.promisedAmount === null
      ? "semua sisa tagihan"
      : formatRupiah(active.promisedAmount);
  return `Janji aktif ${formatWeekdayDayMonth(active.promisedDate)}, ${amount} akan diganti.`;
}

test.describe("Karyawan", () => {
  test.describe.configure({ mode: "default" });
  test.use({
    storageState: async ({}, provide, testInfo) => {
      await provide(promiserOf(testInfo).staff.storageState);
    },
  });

  test("opens Catat janji bayar with the API's offer", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const { tenant } = promiserOf(testInfo);
    const orgId = await openFieldToday(page);
    const me = await call<OrgMeResponse>(page, `/orgs/${orgId}/me`);
    expect(
      me.capabilities.find((each) => each.action === "promise:create")?.access,
    ).toBe("conditional");
    const item = await fieldItem(page, orgId, tenant);
    expect(item.outstanding).toBeGreaterThan(0);
    const offer = await offerOf(page, orgId, item.tenantId);
    const dates = offer.promiseDates;
    if (dates === null) throw new Error(`${tenant} cannot promise now`);

    // The secondary button beside the primary "Terima tunai".
    const row = rowOf(page, tenant);
    const record = row.getByRole("button", { name: RECORD_FIELD_PROMISE_TEXT });
    const receive = row.getByRole("button", { name: "Terima tunai" });
    expect(await height(record)).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
    const recordBox = await record.boundingBox();
    const receiveBox = await receive.boundingBox();
    expect(recordBox?.x ?? 0).toBeGreaterThan(receiveBox?.x ?? 0);
    expect(Math.abs((recordBox?.y ?? 0) - (receiveBox?.y ?? 1))).toBeLessThan(
      2,
    );

    // Both fit side by side, the label on one line.
    expect(
      await record.evaluate((el) => el.scrollWidth <= el.clientWidth),
    ).toBe(true);
    await record.scrollIntoViewIfNeeded();
    await attachViewportScreenshot(page, testInfo, "field-promise-row");

    // The offer is read when the sheet opens: a skeleton until it arrives.
    let release = () => {};
    const held = new Promise<void>((done) => {
      release = done;
    });
    await page.route(
      (url) => PROMISE_PATH.test(url.pathname),
      async (route) => {
        if (route.request().method() === "GET") await held;
        await route.fallback();
      },
    );
    const read = page.waitForRequest((request) =>
      isPromiseRequest(request, "GET"),
    );
    await record.click();
    await read;
    const sheet = page.getByRole("dialog", { name: RECORD_FIELD_PROMISE_TEXT });
    await expect(
      sheet.getByRole("status", { name: FIELD_PROMISE_LOADING_LABEL }),
    ).toBeVisible();
    release();
    await expect(parts(sheet).cancel).toBeVisible();
    await page.unrouteAll();

    await expect(sheet).toContainText(`${tenant} · `);
    await expect(sheet.getByText(FIELD_PROMISE_INTRO)).toBeVisible();
    const replaces = replacesText(offer);
    if (replaces === null) {
      await expect(sheet).not.toContainText("akan diganti");
    } else {
      await expect(sheet.getByText(replaces)).toBeVisible();
    }
    const { ownDate, nextDue, amount, save, cancel, close } = parts(sheet);
    await expect(ownDate).toBeVisible();
    if (offer.nextInvoiceDueDate === null) {
      await expect(nextDue).toHaveCount(0);
    } else {
      await expect(nextDue).toBeVisible();
      await expect(nextDue).toHaveAccessibleName(
        new RegExp(formatDate(offer.nextInvoiceDueDate)),
      );
    }
    // "Kosong = semua sisa" names what the row says is left.
    await expect(amount).toHaveAccessibleDescription(
      `Kosong = semua sisa, ${formatRupiah(item.outstanding)}.`,
    );
    await expect(sheet.getByText("Catatan (tidak wajib)")).toHaveCount(0);

    await ownDate.click();
    await expectDateLimits(
      sheet,
      FIELD_PROMISE_DATE_LABEL,
      dates.earliest,
      dates.latest,
    );
    for (const control of [close, cancel, save]) {
      expect(await height(control)).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
    }
    expect(
      await fontSize(sheet.getByText(FIELD_OWN_DATE_LABEL, { exact: true })),
    ).toBeGreaterThanOrEqual(minBodyFontPx(testInfo));
    await attachViewportScreenshot(page, testInfo, "field-promise-sheet");

    await cancel.click();
    await expect(sheet).toHaveCount(0);
    expect(outsideRequests).toEqual([]);
  });

  test("records a promise for a date, for all that is left", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const { tenant } = promiserOf(testInfo);
    const orgId = await openFieldToday(page);
    const item = await fieldItem(page, orgId, tenant);
    const before = await offerOf(page, orgId, item.tenantId);
    const dates = before.promiseDates;
    if (dates === null) throw new Error(`${tenant} cannot promise now`);
    const promisedDate = dates.earliest;

    const sheet = await openSheet(page, tenant);
    const { ownDate, amount, save } = parts(sheet);
    await ownDate.click();
    await setDate(sheet, FIELD_PROMISE_DATE_LABEL, promisedDate);
    await amount.fill("");
    const posted = page.waitForRequest((request) =>
      isPromiseRequest(request, "POST"),
    );
    const answered = page.waitForResponse((response) =>
      isPromiseRequest(response.request(), "POST"),
    );
    await save.click();
    const body = (await posted).postDataJSON() as MakeTenantPromiseRequest;
    expect(body).toEqual({ kind: "specific_date", promisedDate });
    expect((await answered).status()).toBe(200);
    await expect(sheet).toHaveCount(0);
    if (before.activePromise === null) {
      await expectToast(page, "Janji bayar dicatat", "create", tenant);
    } else {
      await expectToast(page, "Janji bayar diubah", "caution", tenant);
    }

    // The API's own record: the tenant's active promise, made by the karyawan.
    const after = await offerOf(page, orgId, item.tenantId);
    expect(after.activePromise).toMatchObject({
      kind: "specific_date",
      promisedDate,
      promisedAmount: null,
      source: "karyawan",
    });
    expect(outsideRequests).toEqual([]);
  });

  test("opening again says the active promise is replaced", async ({
    page,
  }, testInfo) => {
    const { tenant } = promiserOf(testInfo);
    const orgId = await openFieldToday(page);
    const item = await fieldItem(page, orgId, tenant);
    const offer = await offerOf(page, orgId, item.tenantId);
    const active = offer.activePromise;
    if (active === null) throw new Error("the test before records a promise");
    expect(active.source).toBe("karyawan");

    const sheet = await openSheet(page, tenant);
    const replaces = replacesText(offer);
    if (replaces === null) throw new Error("unreachable");
    await expect(sheet.getByText(replaces)).toBeVisible();
    const { ownDate, amount, save, cancel } = parts(sheet);
    await expect(save).toHaveText("Simpan perubahan");
    await expect(ownDate).toBeChecked();
    await expectDate(sheet, FIELD_PROMISE_DATE_LABEL, active.promisedDate);
    await expect(amount).toHaveValue("");
    await attachViewportScreenshot(page, testInfo, "field-promise-replace");
    await cancel.click();
    await expect(sheet).toHaveCount(0);
  });

  test("refuses a date outside what the API offers", async ({
    page,
  }, testInfo) => {
    const { tenant } = promiserOf(testInfo);
    const orgId = await openFieldToday(page);
    const item = await fieldItem(page, orgId, tenant);
    const offer = await offerOf(page, orgId, item.tenantId);
    const dates = offer.promiseDates;
    if (dates === null) throw new Error(`${tenant} cannot promise now`);
    const tooLate = shiftDay(dates.latest, 1);
    const posts: string[] = [];
    page.on("request", (request) => {
      if (isPromiseRequest(request, "POST")) posts.push(request.url());
    });

    const sheet = await openSheet(page, tenant);
    const { ownDate, save } = parts(sheet);
    await ownDate.click();
    const control = dateControl(sheet, FIELD_PROMISE_DATE_LABEL);
    if (await isCalendarField(control)) {
      // iPad and desktop: the calendar offers no day after the last one.
      await expectDayRefused(sheet, FIELD_PROMISE_DATE_LABEL, tooLate);
    } else {
      // The phone's own date input takes any day; the form refuses it before anything is sent,
      // as the admin's does, under the date.
      await control.fill(tooLate);
      await save.click();
      const message = `Pilih tanggal dari ${formatDate(dates.earliest)} sampai ${formatDate(dates.latest)}.`;
      await expect(control).toHaveAccessibleDescription(
        new RegExp(message.replace(/[.]/g, "\\.")),
      );
      await expect(sheet.getByText(message)).toBeVisible();
      await attachViewportScreenshot(page, testInfo, "field-promise-range");
    }
    expect(posts).toEqual([]);
    await parts(sheet).close.click();
  });
});
