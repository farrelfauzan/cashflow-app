import {
  expect,
  test,
  type Locator,
  type Page,
  type Request,
  type TestInfo,
} from "@playwright/test";

import {
  FIELD_PROMISE_DATE_LABEL,
  RECORD_FIELD_PROMISE_TEXT,
} from "../src/components/client/field-promise/utils";
import { RECEIVE_CASH_TITLE } from "../src/components/client/field-cash/utils";
import {
  UNSAVED_CHANGES_DISCARD,
  UNSAVED_CHANGES_TITLE,
} from "../src/components/client/form-sheet/utils";
import {
  CHANGE_PROMISE_TEXT,
  DIRECTIONS_TEXT,
  DONE_LIST_LABEL,
  LIST_LABEL,
  NOTE_LABEL,
  NO_CASH_TEXT,
  PHONE_LABEL,
  RECEIVE_CASH_TEXT,
  RECORD_PROMISE_TEXT,
  RECORD_TEXT,
  SAVED_TOAST_TITLE,
  SHEET_TITLE,
  TITLE,
  UNKNOWN_SAVE_TEXT,
  cashChoices,
  directionsUrl,
  doneChip,
  fieldVisitRow,
  notHomeLine,
} from "../src/components/client/field-visits/utils";
import type {
  CashPaymentInput,
  CashPaymentResult,
  CheckQueuePage,
  FieldPromiseOffer,
  FieldVisit,
  FieldVisitList,
  OrgMeResponse,
  VisitOutcome,
  VisitOutcomeInput,
} from "../src/lib/api/generated/model";
import { cashPhoto } from "./support/cash-photo";
import { setDate } from "./support/date-field";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import {
  minBodyFontPx,
  projectDigit,
  projectName,
  type ProjectName,
} from "./support/projects";
import {
  FIELD_TODAY_URL,
  FIELD_VISITS_URL,
  openSection,
} from "./support/shell";
import { DIMAS, FITRI, GALIH, LINA, type SeededStaff } from "./support/staff";
import { expectToast } from "./support/toast";

// "Kunjungan hari ini" and "Catat hasil kunjungan" (board HP-Kunjungan), against the real API.
// The e2e setup runs today's daily jobs for Kos Bulanan Sejahtera ("admin run-day"), which open
// the visits of tenants a week past due. Each size project signs in as its own karyawan there
// (e2e/support/staff.ts), who collects from three tenants; the tests pick from the visits the API
// lists for him, never from a fixed name, and record one outcome per test.
//
// An outcome closes a visit for the day (paid and promise close the overdue and broken-promise
// visits, a new number the unreachable one), so a second run on the same database finds fewer
// open visits: the tests that need one say so. Not home keeps a visit open, so it is used for the
// tests that must not close one.
//
// Field cash and promises are shared with field-cash.spec.ts and field-promise.spec.ts, which use
// the same karyawan: the tests prefer a tenant whose cash is already recorded today, and keep
// away from the tenant field-promise.spec.ts changes the promise of.

type Visitor = {
  staff: SeededStaff;
  // field-promise.spec.ts makes this tenant's promise on every run; a promise outcome here would
  // race it.
  promiseTenant: string;
};

const VISITORS: Record<ProjectName, Visitor> = {
  ipad: { staff: GALIH, promiseTenant: "Kurniawan Adi" },
  phone: { staff: LINA, promiseTenant: "Sinta Maharani" },
  desktop: { staff: FITRI, promiseTenant: "Irfan Hakim" },
};

const MIN_TOUCH_PX = 44;
const OUTCOME_PATH =
  /^\/api\/orgs\/[0-9a-f-]{36}\/field\/tenants\/[0-9a-f-]{36}\/visit-outcomes$/;
const SEED_HINT =
  'Run "admin run-day -day <today> -org <Kos Bulanan Sejahtera>" on a freshly seeded database (.github/workflows/web.yml, "Migrate and seed").';

function visitorOf(testInfo: TestInfo): Visitor {
  return VISITORS[projectName(testInfo)];
}

function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

function orgIdOf(page: Page): string {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

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

function visitsOf(page: Page, orgId: string): Promise<FieldVisitList> {
  return call<FieldVisitList>(
    page,
    "GET",
    `/orgs/${orgId}/field/visits?pageSize=100`,
    200,
  );
}

async function visitOf(
  page: Page,
  orgId: string,
  tenantId: string,
): Promise<FieldVisit> {
  const visit = (await visitsOf(page, orgId)).items.find(
    (item) => item.tenantId === tenantId,
  );
  if (visit === undefined) throw new Error(`${tenantId} left the visit list`);
  return visit;
}

// An open visit of the karyawan's: one `prefer` accepts that is not in `avoid` first, then any
// not in `avoid`, and only then one of `avoid` (another spec may change it at the same time).
async function openVisit(
  page: Page,
  orgId: string,
  avoid: readonly string[],
  prefer: (visit: FieldVisit) => boolean = () => true,
): Promise<FieldVisit> {
  const open = (await visitsOf(page, orgId)).items.filter(
    (item) => item.status === "open",
  );
  const free = open.filter((item) => !avoid.includes(item.tenantName));
  const visit = free.find(prefer) ?? free[0] ?? open.find(prefer) ?? open[0];
  expect(visit, `No open visit left for this test. ${SEED_HINT}`).toBeDefined();
  if (visit === undefined) throw new Error("no open visit");
  return visit;
}

// Signs in lands on Tagihan hari ini; Kunjungan hari ini is the next tab or menu item.
async function openVisits(page: Page, testInfo: TestInfo): Promise<string> {
  await page.goto("/organizations");
  await expect(page).toHaveURL(FIELD_TODAY_URL);
  await openSection(page, testInfo, TITLE);
  await expect(page).toHaveURL(FIELD_VISITS_URL);
  await expect(
    page.getByRole("heading", { level: 1, name: TITLE }),
  ).toBeVisible();
  return orgIdOf(page);
}

function openRows(page: Page): Locator {
  return page.getByRole("list", { name: LIST_LABEL }).getByRole("listitem");
}

function doneRows(page: Page): Locator {
  return page
    .getByRole("list", { name: DONE_LIST_LABEL })
    .getByRole("listitem");
}

function openRowOf(page: Page, tenantName: string): Locator {
  return openRows(page).filter({ hasText: tenantName });
}

async function openSheet(page: Page, tenantName: string): Promise<Locator> {
  await openRowOf(page, tenantName)
    .getByRole("button", { name: RECORD_TEXT })
    .click();
  const sheet = page.getByRole("dialog", { name: SHEET_TITLE });
  await expect(sheet).toBeVisible();
  return sheet;
}

function save(sheet: Locator): Locator {
  return sheet.getByRole("button", { name: /^(Simpan hasil|Menyimpan…)$/ });
}

function outcomeRadio(sheet: Locator, label: string): Locator {
  return sheet.getByRole("radio", { name: new RegExp(`^${label}`) });
}

function isOutcomePost(request: Request): boolean {
  return (
    request.method() === "POST" &&
    OUTCOME_PATH.test(new URL(request.url()).pathname)
  );
}

async function fontSize(target: Locator): Promise<number> {
  return target.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

async function expectTouchTarget(target: Locator) {
  const box = await target.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
}

// Saves the sheet and waits for the API's answer and the toast.
async function saveOutcome(
  page: Page,
  sheet: Locator,
  tenantName: string,
): Promise<{ body: VisitOutcomeInput; answer: VisitOutcome }> {
  const posted = page.waitForRequest(isOutcomePost);
  const answered = page.waitForResponse((response) =>
    isOutcomePost(response.request()),
  );
  await save(sheet).click();
  const body = (await posted).postDataJSON() as VisitOutcomeInput;
  const response = await answered;
  expect(response.status(), await response.text()).toBe(201);
  await expect(sheet).toHaveCount(0);
  await expectToast(page, SAVED_TOAST_TITLE, "create", tenantName);
  return { body, answer: (await response.json()) as VisitOutcome };
}

// The row after an outcome, as the API now lists it: still open (with what that shows) or under
// "Sudah dicatat hari ini" with its chip.
async function expectRowAsListed(page: Page, orgId: string, tenantId: string) {
  const visit = await visitOf(page, orgId, tenantId);
  const view = fieldVisitRow(visit);
  if (visit.status === "open") {
    await expect(openRowOf(page, visit.tenantName)).toHaveCount(1);
    await expect(
      doneRows(page).filter({ hasText: visit.tenantName }),
    ).toHaveCount(0);
    return visit;
  }
  const row = doneRows(page).filter({ hasText: visit.tenantName });
  await expect(row).toHaveCount(1);
  await expect(openRowOf(page, visit.tenantName)).toHaveCount(0);
  if (view.doneChip !== null)
    await expect(row).toContainText(view.doneChip.label);
  if (view.doneLine !== null) await expect(row).toContainText(view.doneLine);
  return visit;
}

test.describe("Karyawan", () => {
  // One karyawan's visits per project: the tests close them one by one, in this order.
  test.describe.configure({ mode: "default" });
  test.use({
    storageState: async ({}, provide, testInfo) => {
      await provide(visitorOf(testInfo).staff.storageState);
    },
  });

  test("lists the open visits as the API orders them, then those recorded today", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openVisits(page, testInfo);
    const me = await call<OrgMeResponse>(page, "GET", `/orgs/${orgId}/me`, 200);
    const access = (action: string) =>
      me.capabilities.find((capability) => capability.action === action)
        ?.access;
    expect(access("visit:record")).toBe("conditional");

    const list = await visitsOf(page, orgId);
    const open = list.items.filter((item) => item.status === "open");
    const done = list.items.filter((item) => item.status === "done");
    expect(open.length, `No open visit. ${SEED_HINT}`).toBeGreaterThan(0);

    // The API's order, open first; each row with its chips, what is owed and its buttons.
    await expect(openRows(page)).toHaveCount(open.length);
    await expect(doneRows(page)).toHaveCount(done.length);
    for (const [index, visit] of open.entries()) {
      const view = fieldVisitRow(visit);
      const row = openRows(page).nth(index);
      await expect(row).toContainText(visit.tenantName);
      await expect(row).toContainText(view.place);
      for (const reason of view.reasons) {
        await expect(
          row.getByText(reason.label, { exact: true }),
        ).toBeVisible();
      }
      await expect(row.getByText(view.owed, { exact: true })).toBeVisible();
      if (view.notHomeLine !== null) {
        await expect(row).toContainText(view.notHomeLine);
      }
      const record = row.getByRole("button", { name: RECORD_TEXT });
      await expectTouchTarget(record);
      const directions = row.getByRole("link", { name: DIRECTIONS_TEXT });
      const url = directionsUrl(visit.units);
      if (url === null) {
        await expect(directions).toHaveCount(0);
      } else {
        // A link only: nothing is loaded from the maps site.
        await expect(directions).toHaveAttribute("href", url);
        await expect(directions).toHaveAttribute("target", "_blank");
        await expect(directions).toHaveAttribute("rel", "noopener noreferrer");
        await expectTouchTarget(directions);
      }
    }
    for (const [index, visit] of done.entries()) {
      const chip = doneChip(visit.lastOutcome);
      if (chip !== null) {
        await expect(doneRows(page).nth(index)).toContainText(chip.label);
      }
    }

    // Readable sizes, as on Tagihan hari ini.
    const first = openRows(page).first();
    const min = minBodyFontPx(testInfo);
    const firstView = fieldVisitRow(open[0] as FieldVisit);
    expect(
      await fontSize(first.getByText(firstView.tenantName, { exact: true })),
    ).toBeGreaterThanOrEqual(min);
    expect(
      await fontSize(first.getByText(firstView.owed, { exact: true })),
    ).toBeGreaterThanOrEqual(min);
    const firstChip = firstView.reasons[0];
    if (firstChip !== undefined) {
      expect(
        await fontSize(first.getByText(firstChip.label, { exact: true })),
      ).toBeGreaterThanOrEqual(min);
    }
    await attachViewportScreenshot(page, testInfo, "field-visits");
    expect(outsideRequests).toEqual([]);
  });

  test("Tidak bertemu keeps the visit open, and says so on the row", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openVisits(page, testInfo);
    const visit = await openVisit(page, orgId, []);
    const sheet = await openSheet(page, visit.tenantName);
    await expect(sheet).toContainText(fieldVisitRow(visit).owed);
    await expectTouchTarget(outcomeRadio(sheet, "Tidak bertemu").locator(".."));
    await outcomeRadio(sheet, "Tidak bertemu").click();
    await sheet
      .getByRole("textbox", { name: new RegExp(`^${NOTE_LABEL}`) })
      .fill(`Pintu dikunci, uji ${runId()}`);
    await attachViewportScreenshot(page, testInfo, "field-visits-sheet");
    const { body, answer } = await saveOutcome(page, sheet, visit.tenantName);
    expect(body).toEqual({
      requestId: expect.stringMatching(/^[0-9a-f-]{36}$/),
      outcome: "not_home",
      note: `Pintu dikunci, uji ${runId()}`,
    });
    expect(answer.resolved).toEqual([]);

    const after = await visitOf(page, orgId, visit.tenantId);
    expect(after.status).toBe("open");
    expect(after.lastOutcome?.outcome).toBe("not_home");
    const line = notHomeLine(after.lastOutcome);
    if (line === null) throw new Error("no not_home line");
    await expect(openRowOf(page, visit.tenantName)).toContainText(line);
    expect(outsideRequests).toEqual([]);
  });

  test("records once when the answer to Simpan hasil is lost", async ({
    page,
  }, testInfo) => {
    const orgId = await openVisits(page, testInfo);
    const visit = await openVisit(page, orgId, []);
    const sheet = await openSheet(page, visit.tenantName);
    await outcomeRadio(sheet, "Tidak bertemu").click();

    // The request reaches the API and records the outcome; its answer never reaches the phone.
    let first: VisitOutcome | null = null;
    const matches = (url: URL) => OUTCOME_PATH.test(url.pathname);
    await page.route(matches, async (route) => {
      const response = await route.fetch();
      expect(response.status()).toBe(201);
      first = (await response.json()) as VisitOutcome;
      await route.abort("internetdisconnected");
    });
    const firstPost = page.waitForRequest(isOutcomePost);
    await save(sheet).click();
    const sent = (await firstPost).postDataJSON() as VisitOutcomeInput;
    await expect(sheet.getByText(UNKNOWN_SAVE_TEXT)).toBeVisible();
    await page.unroute(matches);
    await expect(outcomeRadio(sheet, "Tidak bertemu")).toBeDisabled();
    await expect(save(sheet)).toBeEnabled();

    // Closing and pressing Catat hasil again brings back the same opening, still locked.
    await sheet.getByRole("button", { name: "Tutup" }).click();
    if (projectName(testInfo) === "phone") {
      await page
        .getByRole("alertdialog", { name: UNSAVED_CHANGES_TITLE })
        .getByRole("button", { name: UNSAVED_CHANGES_DISCARD })
        .click();
    }
    await expect(sheet).toBeHidden();
    const reopened = await openSheet(page, visit.tenantName);
    await expect(reopened.getByText(UNKNOWN_SAVE_TEXT)).toBeVisible();
    await expect(outcomeRadio(reopened, "Tidak bertemu")).toBeChecked();

    const again = page.waitForRequest(isOutcomePost);
    const answered = page.waitForResponse((response) =>
      isOutcomePost(response.request()),
    );
    await save(reopened).click();
    expect((await again).postDataJSON()).toEqual(sent);
    const response = await answered;
    // Already recorded under this requestId: nothing recorded twice.
    expect(response.status()).toBe(200);
    const answer = (await response.json()) as VisitOutcome;
    expect(first).not.toBeNull();
    expect(answer.id).toBe((first as VisitOutcome | null)?.id);
    await expect(reopened).toHaveCount(0);
    await expectToast(page, SAVED_TOAST_TITLE, "create", visit.tenantName);
  });

  test("Nomor WA berubah sends the number to the admin's Antrean cek", async ({
    page,
    playwright,
    baseURL,
  }, testInfo) => {
    const orgId = await openVisits(page, testInfo);
    const visit = await openVisit(page, orgId, []);
    const sheet = await openSheet(page, visit.tenantName);
    await outcomeRadio(sheet, "Nomor WA berubah").click();
    const phone = sheet.getByRole("textbox", { name: PHONE_LABEL });
    await expect(phone).toHaveAttribute("type", "tel");

    // Not a number: the API's own message, under the field.
    await phone.fill("12");
    const refused = page.waitForResponse((response) =>
      isOutcomePost(response.request()),
    );
    await save(sheet).click();
    const refusal = await refused;
    expect(refusal.status()).toBe(400);
    const { error } = (await refusal.json()) as {
      error: { code: string; message: string; field: string };
    };
    expect(error).toMatchObject({ code: "invalid_phone", field: "newPhone" });
    await expect(sheet.getByText(error.message)).toBeVisible();
    await expect(phone).toBeFocused();

    const newPhone = `08196${runId()}0${projectDigit(testInfo)}`;
    await phone.fill(newPhone);
    const { body, answer } = await saveOutcome(page, sheet, visit.tenantName);
    expect(body).toMatchObject({ outcome: "number_changed", newPhone });
    const after = await expectRowAsListed(page, orgId, visit.tenantId);
    // It closes only an unreachable-number visit; any other open visit stays open.
    const onlyUnreachable = visit.reasons.every(
      (reason) => reason.code === "undeliverable",
    );
    expect(after.status).toBe(onlyUnreachable ? "done" : "open");

    // The admin finds the new number waiting, then takes it off so later runs start clean.
    const admin = await playwright.request.newContext({
      baseURL,
      storageState: DIMAS.storageState,
    });
    try {
      const queue = await admin.get(
        `/api/orgs/${orgId}/check-queue?kind=phone_changed&pageSize=100`,
      );
      expect(queue.status()).toBe(200);
      const waiting = ((await queue.json()) as CheckQueuePage).items.find(
        (item) =>
          item.phoneChanged?.tenant.id === visit.tenantId &&
          item.phoneChanged.newPhone === answer.newPhone,
      );
      expect(waiting?.phoneChanged?.recordedBy.name).toBe(
        visitorOf(testInfo).staff.name,
      );
      if (waiting !== undefined) {
        const dismissed = await admin.post(
          `/api/orgs/${orgId}/check-queue/phone_changed/${waiting.id}/dismiss`,
          { data: { note: "Sisa uji Kunjungan hari ini" } },
        );
        expect(dismissed.status()).toBe(200);
      }
    } finally {
      await admin.dispose();
    }
  });

  test("Sudah bayar hari ini names the cash taken today and closes the visit", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openVisits(page, testInfo);
    const { promiseTenant } = visitorOf(testInfo);
    let visit = await openVisit(
      page,
      orgId,
      [promiseTenant],
      (item) => item.unlinkedCashToday.length > 0,
    );
    if (visit.unlinkedCashToday.length === 0) {
      // No cash from this tenant today yet: taken through the API with its photo, as Terima tunai
      // does.
      const input: CashPaymentInput = {
        requestId: crypto.randomUUID(),
        amount: Number(`4${runId()}`),
        photoUploadId: await cashPhoto(
          page,
          page.request,
          orgId,
          visit.tenantId,
        ),
      };
      await call<CashPaymentResult>(
        page,
        "POST",
        `/orgs/${orgId}/tenants/${visit.tenantId}/cash-payments`,
        201,
        input,
      );
      visit = await visitOf(page, orgId, visit.tenantId);
      await page.reload();
    }
    const newest = visit.unlinkedCashToday.at(-1);
    if (newest === undefined) throw new Error("no cash to name");

    const sheet = await openSheet(page, visit.tenantName);
    // Cash taken today: the sheet opens on Sudah bayar, with the newest cash chosen.
    await expect(outcomeRadio(sheet, "Sudah bayar hari ini")).toBeChecked();
    const [label] = cashChoices(visit.unlinkedCashToday).map(
      (choice) => choice.label,
    );
    if (label === undefined) throw new Error("no choice");
    await expect(sheet.getByRole("radio", { name: label })).toBeChecked();
    const { body, answer } = await saveOutcome(page, sheet, visit.tenantName);
    expect(body).toMatchObject({
      outcome: "paid",
      paymentId: newest.paymentId,
    });
    expect(answer.paymentId).toBe(newest.paymentId);

    const after = await expectRowAsListed(page, orgId, visit.tenantId);
    if (visit.reasons.every((reason) => reason.code !== "undeliverable")) {
      expect(after.status).toBe("done");
    }
    expect(outsideRequests).toEqual([]);
  });

  test("Janji bayar baru opens Catat janji bayar over the sheet, then names the promise", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openVisits(page, testInfo);
    const { promiseTenant } = visitorOf(testInfo);
    const visit = await openVisit(page, orgId, [promiseTenant]);
    const sheet = await openSheet(page, visit.tenantName);

    // Without cash today, Sudah bayar says why and offers Terima tunai, which opens over this
    // sheet; Batal brings back the sheet as it was.
    if (visit.unlinkedCashToday.length === 0) {
      await outcomeRadio(sheet, "Sudah bayar hari ini").click();
      await expect(sheet.getByText(NO_CASH_TEXT)).toBeVisible();
      await expect(save(sheet)).toBeDisabled();
      await sheet.getByRole("button", { name: RECEIVE_CASH_TEXT }).click();
      const cash = page.getByRole("dialog", { name: RECEIVE_CASH_TITLE });
      await expect(cash).toBeVisible();
      await cash.getByRole("button", { name: "Batal" }).click();
      await expect(cash).toHaveCount(0);
      await expect(sheet).toBeVisible();
      await expect(outcomeRadio(sheet, "Sudah bayar hari ini")).toBeChecked();
    }

    await outcomeRadio(sheet, "Janji bayar baru").click();
    const opener = sheet.getByRole("button", {
      name:
        visit.unlinkedPromiseToday === null
          ? RECORD_PROMISE_TEXT
          : CHANGE_PROMISE_TEXT,
    });
    if (visit.unlinkedPromiseToday === null) {
      await expect(save(sheet)).toBeDisabled();
    }
    const offer = await call<FieldPromiseOffer>(
      page,
      "GET",
      `/orgs/${orgId}/field/tenants/${visit.tenantId}/promises`,
      200,
    );
    const dates = offer.promiseDates;
    if (dates === null)
      throw new Error(`${visit.tenantName} cannot promise now`);
    await opener.click();
    const promise = page.getByRole("dialog", {
      name: RECORD_FIELD_PROMISE_TEXT,
    });
    await expect(promise).toBeVisible();
    await expect(promise.getByRole("button", { name: "Batal" })).toBeVisible();
    await promise.getByRole("radio", { name: /^Pilih tanggal/ }).click();
    await setDate(promise, FIELD_PROMISE_DATE_LABEL, dates.earliest);
    await promise
      .getByRole("button", { name: /^(Simpan janji|Simpan perubahan)$/ })
      .click();
    await expect(promise).toHaveCount(0);

    // Back on Catat hasil, still on Janji bayar baru, with the promise just made.
    await expect(sheet).toBeVisible();
    await expect(outcomeRadio(sheet, "Janji bayar baru")).toBeChecked();
    const made = (await visitOf(page, orgId, visit.tenantId))
      .unlinkedPromiseToday;
    if (made === null) throw new Error("the promise is not offered");
    expect(made.promisedDate).toBe(dates.earliest);
    await expect(
      sheet.getByRole("button", { name: CHANGE_PROMISE_TEXT }),
    ).toBeVisible();
    await expect(save(sheet)).toBeEnabled();

    const { body } = await saveOutcome(page, sheet, visit.tenantName);
    expect(body).toMatchObject({
      outcome: "promise",
      promiseVersionId: made.promiseVersionId,
    });
    const after = await expectRowAsListed(page, orgId, visit.tenantId);
    if (visit.reasons.every((reason) => reason.code !== "undeliverable")) {
      expect(after.status).toBe("done");
    }
    expect(outsideRequests).toEqual([]);
  });
});
