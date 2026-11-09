import {
  expect,
  test,
  type Locator,
  type Page,
  type Request,
  type TestInfo,
} from "@playwright/test";

import type {
  Lease,
  MakeTenantPromiseRequest,
  PaymentResult,
  Tenant,
  TenantAccount,
  TenantList,
  TenantPromiseList,
  Unit,
} from "../src/lib/api/generated/model";
import {
  formatTimestampDayMonth,
  formatWeekdayDayMonth,
  jakartaToday,
} from "../src/lib/format/date";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  animationsSettled,
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import {
  dateControl,
  expectDate,
  expectDateLimits,
  setDate,
} from "./support/date-field";
import { minBodyFontPx, projectDigit, projectWord } from "./support/projects";
import { findOrCreateProperty } from "./support/property-fixtures";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA, type SeededStaff } from "./support/staff";

// The tenant's payment promise on Detail penyewa ("Janji aktif", "Riwayat janji", "Catat janji"),
// against the real API and its worker. Each admin test makes money of its own: a kos unit
// ("Kamar Janji"), a tenant and a lease that started on the 1st of this month, which the worker
// invoices within seconds. Every amount and date checked is read from what the API returns,
// never hard-coded. Pak Hendra is Admin of the whole of Griya Sejahtera and Supervisor in Kost
// Putri Anggrek, where he may read promises but not record them.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const REGION = "Uang penyewa";
const PRICE = 1_150_000;

const MIN_TOUCH_PX = 44;

function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

// A name no other run, project or retry uses.
function uniqueName(testInfo: TestInfo, base: string): string {
  const project = projectWord(testInfo);
  const retry = testInfo.retry === 0 ? "" : ` r${testInfo.retry}`;
  return `${base} ${runId()} ${project}${retry}`;
}

// A tenant number no other run, project, test or retry uses; 0815-3 is used by no other spec.
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = projectDigit(testInfo);
  return `0815-3${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
}

// The first day of this month in Jakarta: a lease from it is invoiced at once.
function firstOfThisMonth(): string {
  return `${jakartaToday(new Date()).slice(0, 7)}-01`;
}

function orgIdOf(page: Page): string {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

// Opens the organization's home: straight in for one organization, by name for several.
async function openOrganization(page: Page, staff: SeededStaff, name: string) {
  await page.goto("/organizations");
  if (staff.organizations.length > 1) {
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name })
      .click();
  }
  await expect(page).toHaveURL(ORG_HOME_URL);
  return orgIdOf(page);
}

// Setup through the API, as the signed-in member (the page's cookies).
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

type Fixture = {
  orgId: string;
  tenant: Tenant;
  accountPath: string;
  account: TenantAccount;
};

// A tenant of the test's own who owes one invoice: a kos unit rented by the month from the 1st.
async function tenantWhoOwes(
  page: Page,
  testInfo: TestInfo,
  testDigit: number,
  tenantBase: string,
): Promise<Fixture> {
  const orgId = await openOrganization(page, HENDRA, GRIYA);
  const property = await findOrCreateProperty(page, orgId, testInfo, {
    base: "Janji",
    kind: "kos",
  });
  const unit = await call<Unit>(
    page,
    "POST",
    `/orgs/${orgId}/properties/${property.id}/units`,
    201,
    {
      label: uniqueName(testInfo, `Kamar Janji ${testDigit}`),
      terms: [{ months: 1, price: PRICE }],
    },
  );
  const tenant = await call<Tenant>(
    page,
    "POST",
    `/orgs/${orgId}/tenants`,
    201,
    {
      name: uniqueName(testInfo, tenantBase),
      phone: tenantNumber(testInfo, testDigit),
      note: "",
    },
  );
  await call<Lease>(page, "POST", `/orgs/${orgId}/leases`, 201, {
    tenantId: tenant.id,
    unitId: unit.id,
    startDate: firstOfThisMonth(),
    termMonths: 1,
    depositAmount: PRICE,
  });
  const accountPath = `/orgs/${orgId}/tenants/${tenant.id}/account`;

  // The worker invoices the lease within seconds.
  let account = await call<TenantAccount>(page, "GET", accountPath, 200);
  await expect
    .poll(
      async () => {
        account = await call<TenantAccount>(page, "GET", accountPath, 200);
        return account.invoices.length;
      },
      { timeout: 20_000 },
    )
    .toBeGreaterThanOrEqual(1);
  return { orgId, tenant, accountPath, account };
}

async function openTenantPage(
  page: Page,
  orgId: string,
  tenant: Pick<Tenant, "id" | "name">,
) {
  await page.goto(`/o/${orgId}/tenants/${tenant.id}`);
  await expect(
    page.getByRole("heading", { level: 1, name: tenant.name }),
  ).toBeVisible();
  const region = page.getByRole("region", { name: REGION });
  await expect(region).toBeVisible();
  return region;
}

function isPromisePost(request: Request): boolean {
  return (
    request.method() === "POST" &&
    /\/api\/orgs\/[0-9a-f-]{36}\/tenants\/[0-9a-f-]{36}\/promises$/.test(
      request.url(),
    )
  );
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

async function height(locator: Locator): Promise<number> {
  await animationsSettled(locator);
  const box = await locator.boundingBox();
  if (box === null) throw new Error("not visible");
  return box.height;
}

// The "Riwayat janji" card: its heading and what follows it.
function historyCard(region: Locator): Locator {
  return region.locator("section").filter({
    has: region
      .page()
      .getByRole("heading", { level: 2, name: "Riwayat janji" }),
  });
}

// The dialog's parts, by what a person reads.
function dialogParts(dialog: Locator) {
  return {
    nextInvoice: dialog.getByRole("radio", {
      name: /Bareng tagihan berikutnya/,
    }),
    ownDate: dialog.getByRole("radio", { name: /Pilih tanggal sendiri/ }),
    date: dateControl(dialog, "Tanggal pelunasan"),
    amount: dialog.getByRole("textbox", {
      name: "Nominal janji (boleh dikosongkan)",
    }),
    note: dialog.getByRole("textbox", { name: "Catatan (tidak wajib)" }),
    save: dialog.getByRole("button", {
      name: /^(Simpan janji|Simpan perubahan|Menyimpan…)$/,
    }),
    cancel: dialog.getByRole("button", { name: "Batal" }),
    close: dialog.getByRole("button", { name: "Tutup" }),
  };
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("records a promise for a date, then changes it", async ({
    page,
  }, testInfo) => {
    test.setTimeout(120_000);
    const outsideRequests = trackOutsideRequests(page);
    const posts: string[] = [];
    page.on("request", (request) => {
      if (isPromisePost(request)) posts.push(request.url());
    });
    const { orgId, tenant, accountPath, account } = await tenantWhoOwes(
      page,
      testInfo,
      1,
      "Wahyuni Setyaningsih",
    );
    const dates = account.promiseDates;
    if (dates === null) throw new Error("the tenant owes but has no dates");
    expect(account.activePromise).toBeNull();
    const promisedDate = dates.earliest;
    const day = formatWeekdayDayMonth(promisedDate);
    const note = uniqueName(testInfo, "Gajian tanggal 10");

    // No promise yet.
    const region = await openTenantPage(page, orgId, tenant);
    const tile = region.getByRole("group", { name: "Janji aktif" });
    await expect(tile).toContainText("—");
    await expect(tile).toContainText("Tidak ada janji aktif");
    const history = historyCard(region);
    await expect(history).toContainText("Belum ada janji.");
    await page.evaluate(() => window.scrollTo(0, 0));
    await attachViewportScreenshot(page, testInfo, "tenant-promise-tile");

    // "Catat janji", an outline button in the header beside the primary "Catat pembayaran".
    const record = page.getByRole("button", { name: "Catat janji" });
    expect(await height(record)).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
    await record.click();
    const dialog = page.getByRole("dialog", { name: "Catat janji" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText(tenant.name, { exact: true })).toBeVisible();
    await expect(dialog).not.toContainText("akan diganti");
    const parts = dialogParts(dialog);
    await expect(parts.save).toHaveText("Simpan janji");
    await expect(parts.amount).toHaveAccessibleDescription(
      `Kosongkan untuk semua sisa, ${formatRupiah(account.outstanding)}.`,
    );
    for (const control of [parts.close, parts.cancel, parts.save]) {
      expect(await height(control)).toBeGreaterThanOrEqual(MIN_TOUCH_PX);
    }
    const minFont = minBodyFontPx(testInfo);
    expect(
      await fontSize(dialog.getByText("Pilih tanggal sendiri")),
    ).toBeGreaterThanOrEqual(minFont);

    // "Pilih tanggal sendiri" without a date: refused before anything is sent.
    await parts.ownDate.click();
    await expect(parts.date).toBeVisible();
    await expectDateLimits(
      dialog,
      "Tanggal pelunasan",
      dates.earliest,
      dates.latest,
    );
    await parts.save.click();
    await expect(dialog.getByText("Pilih tanggal pelunasan.")).toBeVisible();
    expect(posts).toEqual([]);

    await setDate(dialog, "Tanggal pelunasan", promisedDate);
    await parts.note.fill(note);
    await attachViewportScreenshot(page, testInfo, "tenant-promise-form");
    const sent = page.waitForRequest(isPromisePost);
    await parts.save.click();
    const body = (await sent).postDataJSON() as MakeTenantPromiseRequest;
    expect(body).toEqual({ kind: "specific_date", promisedDate, note });

    await expect(dialog).toHaveCount(0);
    await expect(page.getByText("Janji tercatat.")).toBeVisible();
    await expect(tile).toContainText(day);
    await expect(tile).toContainText(
      `Semua sisa tagihan · dicatat ${HENDRA.name}`,
    );
    await expect(tile).toContainText(note);
    await tile.scrollIntoViewIfNeeded();
    await attachViewportScreenshot(page, testInfo, "tenant-promise-active");
    const list = history.getByRole("list", { name: "Daftar janji" });
    const rows = list.getByRole("listitem");
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toContainText(`${day} · Semua sisa tagihan`);
    await expect(rows.first()).toContainText(`dicatat ${HENDRA.name}`);
    await expect(rows.first()).toContainText("Aktif");
    await expect(rows.first()).not.toContainText("diubah");

    // "Catat janji" again: filled from the active promise, which saving replaces.
    await record.click();
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText(
      `Janji aktif ${day}, semua sisa tagihan akan diganti.`,
    );
    await expect(parts.save).toHaveText("Simpan perubahan");
    await expect(parts.ownDate).toBeChecked();
    await expectDate(dialog, "Tanggal pelunasan", promisedDate);
    await expect(parts.amount).toHaveValue("");
    await attachViewportScreenshot(page, testInfo, "tenant-promise-replace");

    // Part of what is owed, never more than that.
    const promisedAmount = Math.min(100_000, account.outstanding);
    await parts.amount.fill(String(promisedAmount));
    const edited = page.waitForRequest(isPromisePost);
    await parts.save.click();
    const editBody = (await edited).postDataJSON() as MakeTenantPromiseRequest;
    expect(editBody).toEqual({
      kind: "specific_date",
      promisedDate,
      promisedAmount,
    });
    expect(editBody).not.toHaveProperty("payingNow");
    await expect(dialog).toHaveCount(0);

    // The API's own record: one promise with two versions.
    const promises = await call<TenantPromiseList>(
      page,
      "GET",
      `/orgs/${orgId}/tenants/${tenant.id}/promises`,
      200,
    );
    expect(promises.items).toHaveLength(1);
    const versions = promises.items[0]?.versions ?? [];
    expect(versions).toHaveLength(2);
    const newest = versions.at(-1);
    if (newest === undefined) throw new Error("no version");
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toContainText(
      `${day} · ${formatRupiah(promisedAmount)}`,
    );
    await expect(rows.first()).toContainText(
      `dicatat ${HENDRA.name} ${formatTimestampDayMonth(newest.createdAt)} · diubah 1×`,
    );
    await expect(tile).toContainText(
      `${formatRupiah(promisedAmount)} · dicatat ${HENDRA.name}`,
    );
    const after = await call<TenantAccount>(page, "GET", accountPath, 200);
    expect(after.activePromise?.version).toBe(2);
    expect(
      await fontSize(rows.first().getByText("diubah 1×", { exact: false })),
    ).toBeGreaterThanOrEqual(minFont);

    await history.scrollIntoViewIfNeeded();
    await attachViewportScreenshot(page, testInfo, "tenant-promise-history");
    expect(outsideRequests).toEqual([]);
  });

  test("offers no Catat janji for a tenant who owes nothing", async ({
    page,
  }, testInfo) => {
    test.setTimeout(90_000);
    const outsideRequests = trackOutsideRequests(page);
    const { orgId, tenant, accountPath, account } = await tenantWhoOwes(
      page,
      testInfo,
      2,
      "Sekar Ayuningtyas",
    );
    await call<PaymentResult>(page, "POST", `/orgs/${orgId}/payments`, 201, {
      requestId: crypto.randomUUID(),
      tenantId: tenant.id,
      amount: account.outstanding,
      method: "cash",
      receivedOn: jakartaToday(new Date()),
      note: "",
    });
    const paid = await call<TenantAccount>(page, "GET", accountPath, 200);
    expect(paid.outstanding).toBe(0);
    expect(paid.promiseDates).toBeNull();

    const region = await openTenantPage(page, orgId, tenant);
    await expect(
      region.getByRole("group", { name: "Janji aktif" }),
    ).toContainText("Tidak ada janji aktif");
    await expect(historyCard(region)).toContainText("Belum ada janji.");
    // The header's other actions show, so the page has loaded what it decides from.
    await expect(
      page.getByRole("button", { name: "Catat pembayaran" }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Catat janji" })).toHaveCount(
      0,
    );
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees the promise tile and history but cannot record one", async ({
    page,
  }) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, HENDRA, ANGGREK);
    const { items } = await call<TenantList>(
      page,
      "GET",
      `/orgs/${orgId}/tenants?pageSize=100`,
      200,
    );
    if (items.length === 0)
      throw new Error("Kost Putri Anggrek has no tenants");
    // A tenant who owes (seed-demo bills Kost Putri Anggrek's leases and pays only part), so the
    // only thing that hides "Catat janji" is the Supervisor's missing promise:create.
    let chosen: (typeof items)[number] | undefined;
    for (const each of items) {
      const account = await call<TenantAccount>(
        page,
        "GET",
        `/orgs/${orgId}/tenants/${each.id}/account`,
        200,
      );
      if (account.promiseDates !== null) {
        chosen = each;
        break;
      }
    }
    if (chosen === undefined) {
      throw new Error("Kost Putri Anggrek has no tenant who owes");
    }

    const region = await openTenantPage(page, orgId, chosen);
    const tile = region.getByRole("group", { name: "Janji aktif" });
    await expect(tile).toBeVisible();
    await expect(
      region.getByRole("heading", { level: 2, name: "Riwayat janji" }),
    ).toBeVisible();
    await expect(historyCard(region)).not.toContainText("Memuat janji…");
    await expect(page.getByRole("button", { name: "Catat janji" })).toHaveCount(
      0,
    );
    await expect(region.getByRole("button")).toHaveCount(0);
    expect(outsideRequests).toEqual([]);
  });
});
