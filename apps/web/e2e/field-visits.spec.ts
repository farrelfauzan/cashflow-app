import {
  expect,
  test,
  type Locator,
  type Page,
  type Request,
  type TestInfo,
} from "@playwright/test";

import {
  RECEIVE_CASH_TITLE,
  UNKNOWN_SAVE_TEXT as UNKNOWN_CASH_SAVE_TEXT,
} from "../src/components/client/field-cash/utils";
import {
  FIELD_PROMISE_DATE_LABEL,
  RECORD_FIELD_PROMISE_TEXT,
} from "../src/components/client/field-promise/utils";
import { UNSAVED_CHANGES_TITLE } from "../src/components/client/form-sheet/utils";
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
  reasonChip,
} from "../src/components/client/field-visits/utils";
import type {
  CashPaymentResult,
  CheckQueuePage,
  FieldPromiseOffer,
  FieldVisit,
  FieldVisitList,
  OrgMeResponse,
  VisitOutcome,
  VisitOutcomeInput,
} from "../src/lib/api/generated/model";
import { formatRupiah } from "../src/lib/format/rupiah";
import { setDate } from "./support/date-field";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { transferPhoto } from "./support/proof-fixtures";
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
import {
  ARIF,
  BAMBANG,
  SRI,
  TEGUH,
  VISIT_ORG,
  type SeededStaff,
} from "./support/staff";
import { expectToast } from "./support/toast";

// "Kunjungan hari ini" and "Catat hasil kunjungan" (board HP-Kunjungan), against the real API, in
// Kos Kunjungan Asri: an organization seeded for this spec alone, so no other spec's cash or
// promises meet its visits. Its tenants are about two invoices past due, so each has an open
// visit once "admin run-day" has run for the organization. Each size project signs in as its own
// karyawan there (e2e/support/staff.ts), who collects from four tenants, in the seed's order:
//
//   1. overdue: "Tidak bertemu", then the lost answer (Tidak bertemu leaves the visit open)
//   2. overdue: "Nomor WA berubah", which Pak Bambang (Admin) finds in Antrean cek
//   3. overdue and two broken promises: "Janji bayar baru" through Catat janji bayar
//   4. overdue: "Sudah bayar hari ini", with the cash taken through Terima tunai from the sheet
//
// So the tests never compete for a tenant. Paid and promise close a visit for the day, so a second
// run on the same database finds tenants 3 and 4 done: reset the database (or run the day again
// for a new day) before running it again.

type Visitor = {
  staff: SeededStaff;
  // The four tenants above, in that order.
  notHome: string;
  newNumber: string;
  promise: string;
  paid: string;
};

const VISITORS: Record<ProjectName, Visitor> = {
  ipad: {
    staff: ARIF,
    notHome: "Ahmad Fauzi",
    newNumber: "Bayu Saputra",
    promise: "Citra Ayu Lestari",
    paid: "Danu Prakoso",
  },
  phone: {
    staff: SRI,
    notHome: "Eko Nugroho",
    newNumber: "Fajar Ramadhan",
    promise: "Gilang Permana",
    paid: "Hesti Wulansari",
  },
  desktop: {
    staff: TEGUH,
    notHome: "Imam Santoso",
    newNumber: "Joko Susilo",
    promise: "Kiki Amelia",
    paid: "Lutfi Hakim",
  },
};

const MIN_TOUCH_PX = 44;
const OUTCOME_PATH =
  /^\/api\/orgs\/[0-9a-f-]{36}\/field\/tenants\/[0-9a-f-]{36}\/visit-outcomes$/;
const CASH_PAYMENTS_PATH =
  /^\/api\/orgs\/[0-9a-f-]{36}\/tenants\/[0-9a-f-]{36}\/cash-payments$/;
const SEED_HINT = `Seed ${VISIT_ORG} and run "admin run-day -day <today> -org <${VISIT_ORG}'s id>" (.github/workflows/web.yml, "Migrate and seed"); a second run on the same database finds the paid and promise visits already done.`;

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

async function call<T>(page: Page, path: string): Promise<T> {
  const response = await page.request.get(`/api${path}`);
  expect(response.status(), await response.text()).toBe(200);
  return (await response.json()) as T;
}

function visitsOf(page: Page, orgId: string): Promise<FieldVisitList> {
  return call<FieldVisitList>(page, `/orgs/${orgId}/field/visits?pageSize=100`);
}

// The tenant's row as the API lists it now.
async function visitNamed(
  page: Page,
  orgId: string,
  tenantName: string,
): Promise<FieldVisit> {
  const visit = (await visitsOf(page, orgId)).items.find(
    (item) => item.tenantName === tenantName,
  );
  expect(
    visit,
    `${tenantName} is not on the visit list. ${SEED_HINT}`,
  ).toBeDefined();
  if (visit === undefined) throw new Error(`${tenantName} not listed`);
  return visit;
}

// The tenant's row, which this test needs still open.
async function openVisitNamed(
  page: Page,
  orgId: string,
  tenantName: string,
): Promise<FieldVisit> {
  const visit = await visitNamed(page, orgId, tenantName);
  expect(
    visit.status,
    `${tenantName}'s visit is already done. ${SEED_HINT}`,
  ).toBe("open");
  return visit;
}

// The karyawan has one organization: signing in lands on Tagihan hari ini, and Kunjungan hari
// ini is the next tab or menu item.
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

function isCashPost(request: Request): boolean {
  return (
    request.method() === "POST" &&
    CASH_PAYMENTS_PATH.test(new URL(request.url()).pathname)
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

// The row after an outcome, as the API now lists it: still open, or under "Sudah dicatat hari
// ini" with its chip and time.
async function expectRowAsListed(
  page: Page,
  orgId: string,
  tenantName: string,
): Promise<FieldVisit> {
  const visit = await visitNamed(page, orgId, tenantName);
  const done = doneRows(page).filter({ hasText: tenantName });
  if (visit.status === "open") {
    await expect(openRowOf(page, tenantName)).toHaveCount(1);
    await expect(done).toHaveCount(0);
    return visit;
  }
  const view = fieldVisitRow(visit);
  await expect(done).toHaveCount(1);
  await expect(openRowOf(page, tenantName)).toHaveCount(0);
  if (view.doneChip !== null) {
    await expect(done).toContainText(view.doneChip.label);
  }
  if (view.doneLine !== null) await expect(done).toContainText(view.doneLine);
  return visit;
}

test.describe("Karyawan", () => {
  // One karyawan's visits per project; the list test reads them before the others change them.
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
    const me = await call<OrgMeResponse>(page, `/orgs/${orgId}/me`);
    expect(
      me.capabilities.find((capability) => capability.action === "visit:record")
        ?.access,
    ).toBe("conditional");

    const list = await visitsOf(page, orgId);
    const visitor = visitorOf(testInfo);
    // His four tenants, and no one else's.
    expect(list.items.map((item) => item.tenantName).sort(), SEED_HINT).toEqual(
      [
        visitor.notHome,
        visitor.newNumber,
        visitor.promise,
        visitor.paid,
      ].sort(),
    );
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
      await expectTouchTarget(row.getByRole("button", { name: RECORD_TEXT }));
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
    const firstVisit = open[0];
    if (firstVisit === undefined) throw new Error("no open visit");
    const first = openRows(page).first();
    const firstView = fieldVisitRow(firstVisit);
    const min = minBodyFontPx(testInfo);
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
    const { notHome } = visitorOf(testInfo);
    const orgId = await openVisits(page, testInfo);
    const visit = await openVisitNamed(page, orgId, notHome);
    const sheet = await openSheet(page, notHome);
    await expect(sheet).toContainText(fieldVisitRow(visit).owed);
    const radio = outcomeRadio(sheet, "Tidak bertemu");
    await expectTouchTarget(radio.locator(".."));
    await radio.click();
    const note = `Pintu dikunci, uji ${runId()}`;
    await sheet
      .getByRole("textbox", { name: new RegExp(`^${NOTE_LABEL}`) })
      .fill(note);
    await attachViewportScreenshot(page, testInfo, "field-visits-sheet");
    const { body, answer } = await saveOutcome(page, sheet, notHome);
    expect(body).toEqual({
      requestId: expect.stringMatching(/^[0-9a-f-]{36}$/),
      outcome: "not_home",
      note,
    });
    expect(answer.resolved).toEqual([]);

    const after = await expectRowAsListed(page, orgId, notHome);
    expect(after.status).toBe("open");
    expect(after.lastOutcome?.outcome).toBe("not_home");
    const line = notHomeLine(after.lastOutcome);
    if (line === null) throw new Error("no not_home line");
    await expect(openRowOf(page, notHome)).toContainText(line);
    expect(outsideRequests).toEqual([]);
  });

  test("records once when the answer to Simpan hasil is lost", async ({
    page,
  }, testInfo) => {
    // Tidak bertemu left this tenant's visit open, so another one can be recorded.
    const { notHome } = visitorOf(testInfo);
    const orgId = await openVisits(page, testInfo);
    await openVisitNamed(page, orgId, notHome);
    const sheet = await openSheet(page, notHome);
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
    // The opening is kept, so even on the phone closing it does not ask to discard.
    await sheet.getByRole("button", { name: "Tutup" }).click();
    await expect(
      page.getByRole("alertdialog", { name: UNSAVED_CHANGES_TITLE }),
    ).toHaveCount(0);
    await expect(sheet).toBeHidden();
    const reopened = await openSheet(page, notHome);
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
    await expectToast(page, SAVED_TOAST_TITLE, "create", notHome);
  });

  test("Nomor WA berubah sends the number to the admin's Antrean cek", async ({
    page,
    playwright,
    baseURL,
  }, testInfo) => {
    const { newNumber, staff } = visitorOf(testInfo);
    const orgId = await openVisits(page, testInfo);
    const visit = await openVisitNamed(page, orgId, newNumber);
    const sheet = await openSheet(page, newNumber);
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

    const typed = `08196${runId()}0${projectDigit(testInfo)}`;
    await phone.fill(typed);
    const { body, answer } = await saveOutcome(page, sheet, newNumber);
    expect(body).toMatchObject({ outcome: "number_changed", newPhone: typed });
    // It closes only an unreachable-number visit: an overdue one stays open.
    const after = await expectRowAsListed(page, orgId, newNumber);
    const onlyUnreachable = visit.reasons.every(
      (reason) => reason.code === "undeliverable",
    );
    expect(after.status).toBe(onlyUnreachable ? "done" : "open");

    // Pak Bambang (Admin) finds the new number waiting, then takes it off so later runs start
    // clean.
    const admin = await playwright.request.newContext({
      baseURL,
      storageState: BAMBANG.storageState,
    });
    try {
      const queue = await admin.get(
        `/api/orgs/${orgId}/check-queue?kind=phone_changed&pageSize=100`,
      );
      expect(queue.status(), await queue.text()).toBe(200);
      const waiting = ((await queue.json()) as CheckQueuePage).items.find(
        (item) =>
          item.phoneChanged?.tenant.id === visit.tenantId &&
          item.phoneChanged.newPhone === answer.newPhone,
      );
      expect(waiting?.phoneChanged?.recordedBy.name).toBe(staff.name);
      if (waiting !== undefined) {
        const dismissed = await admin.post(
          `/api/orgs/${orgId}/check-queue/phone_changed/${waiting.id}/dismiss`,
          { data: { note: "Sisa uji Kunjungan hari ini" } },
        );
        expect(dismissed.status(), await dismissed.text()).toBe(200);
      }
    } finally {
      await admin.dispose();
    }
  });

  test("Janji bayar baru opens Catat janji bayar over the sheet, then names the promise", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const { promise: tenant } = visitorOf(testInfo);
    const orgId = await openVisits(page, testInfo);
    const visit = await openVisitNamed(page, orgId, tenant);

    // The seed broke two of this tenant's promises: the row shows both reasons.
    expect(visit.reasons.map((reason) => reason.code).sort()).toEqual([
      "broken_promises",
      "overdue",
    ]);
    expect(
      visit.reasons.find((reason) => reason.code === "broken_promises")
        ?.brokenPromises,
    ).toBe(2);
    const row = openRowOf(page, tenant);
    for (const reason of visit.reasons) {
      await expect(
        row.getByText(reasonChip(reason).label, { exact: true }),
      ).toBeVisible();
    }
    await expect(
      row.getByText("Janji lewat 2×", { exact: true }),
    ).toBeVisible();

    const offer = await call<FieldPromiseOffer>(
      page,
      `/orgs/${orgId}/field/tenants/${visit.tenantId}/promises`,
    );
    const dates = offer.promiseDates;
    if (dates === null) throw new Error(`${tenant} cannot promise now`);

    const sheet = await openSheet(page, tenant);
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
    const made = (await visitNamed(page, orgId, tenant)).unlinkedPromiseToday;
    if (made === null) throw new Error("the promise is not offered");
    expect(made.promisedDate).toBe(dates.earliest);
    await expect(
      sheet.getByRole("button", { name: CHANGE_PROMISE_TEXT }),
    ).toBeVisible();
    await expect(save(sheet)).toBeEnabled();

    const { body, answer } = await saveOutcome(page, sheet, tenant);
    expect(body).toMatchObject({
      outcome: "promise",
      promiseVersionId: made.promiseVersionId,
    });
    expect([...answer.resolved].sort()).toEqual(["broken_promises", "overdue"]);
    const after = await expectRowAsListed(page, orgId, tenant);
    expect(after.status).toBe("done");
    expect(outsideRequests).toEqual([]);
  });

  test("Sudah bayar hari ini takes the cash through Terima tunai, then names it", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const { paid: tenant } = visitorOf(testInfo);
    const orgId = await openVisits(page, testInfo);
    const visit = await openVisitNamed(page, orgId, tenant);
    const sheet = await openSheet(page, tenant);

    let paymentId: string;
    // What this run received through Terima tunai; null when an earlier run took the cash.
    let receivedAmount: number | null = null;
    if (visit.unlinkedCashToday.length === 0) {
      // No cash yet: Sudah bayar says why and offers Terima tunai, which opens over this sheet.
      await outcomeRadio(sheet, "Sudah bayar hari ini").click();
      await expect(sheet.getByText(NO_CASH_TEXT)).toBeVisible();
      await expect(save(sheet)).toBeDisabled();
      await sheet.getByRole("button", { name: RECEIVE_CASH_TEXT }).click();
      const cash = page.getByRole("dialog", { name: RECEIVE_CASH_TITLE });
      await expect(cash).toBeVisible();

      // A small part of what is owed, with its receipt photo.
      const amount = Number(`5${runId()}`);
      receivedAmount = amount;
      await cash.getByLabel("Uang yang diterima").fill(String(amount));
      await cash.getByLabel("Ambil foto kwitansi").setInputFiles({
        name: "kwitansi.jpg",
        mimeType: "image/jpeg",
        buffer: await transferPhoto(page, formatRupiah(amount)),
      });
      const simpan = cash.getByRole("button", {
        name: `Simpan ${formatRupiah(amount)}`,
      });
      await expect(simpan).toBeEnabled();
      const recorded = page.waitForResponse((response) =>
        isCashPost(response.request()),
      );
      await simpan.click();
      const response = await recorded;
      expect(response.status(), await response.text()).toBe(201);
      paymentId = ((await response.json()) as CashPaymentResult).payment.id;
      await expect(cash.getByText(UNKNOWN_CASH_SAVE_TEXT)).toHaveCount(0);
      await expect(
        cash.getByText(`${formatRupiah(amount)} tercatat`),
      ).toBeVisible();
      await cash.getByRole("button", { name: "Selesai" }).click();
      await expect(cash).toHaveCount(0);
    } else {
      // A run before this one took the cash and stopped: the newest is named.
      const newest = visit.unlinkedCashToday.at(-1);
      if (newest === undefined) throw new Error("no cash");
      paymentId = newest.paymentId;
    }

    // Back on Catat hasil, still on Sudah bayar, the new cash listed and chosen.
    await expect(sheet).toBeVisible();
    await expect(outcomeRadio(sheet, "Sudah bayar hari ini")).toBeChecked();
    const listed = await visitNamed(page, orgId, tenant);
    expect(listed.unlinkedCashToday.at(-1)?.paymentId).toBe(paymentId);
    const [label] = cashChoices(listed.unlinkedCashToday).map(
      (choice) => choice.label,
    );
    if (label === undefined) throw new Error("no choice");
    await expect(sheet.getByRole("radio", { name: label })).toBeChecked();
    await expect(save(sheet)).toBeEnabled();

    const { body, answer } = await saveOutcome(page, sheet, tenant);
    expect(body).toMatchObject({ outcome: "paid", paymentId });
    expect(answer.paymentId).toBe(paymentId);
    const after = await expectRowAsListed(page, orgId, tenant);
    expect(after.status).toBe("done");
    // The done row names the cash it counts, as the board says: "Rp950.000 tunai · 09.15".
    if (receivedAmount !== null) {
      await expect(
        doneRows(page).filter({ hasText: tenant }),
        "the done row names the received amount",
      ).toContainText(`${formatRupiah(receivedAmount)} tunai`);
    }
    expect(outsideRequests).toEqual([]);
  });
});
