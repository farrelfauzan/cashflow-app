import {
  expect,
  test,
  type Locator,
  type Page,
  type Response,
  type TestInfo,
} from "@playwright/test";

import {
  currentPeriod,
  FORBIDDEN_TEXT,
  LIST_LABEL,
  monthLabel,
  previousMonth,
  summaryTiles,
} from "../src/components/client/invoice/utils";
import type {
  InvoiceList,
  Lease,
  OrgMeResponse,
  Tenant,
  TenantAccount,
  Unit,
} from "../src/lib/api/generated/model";
import {
  formatDate,
  formatDayMonth,
  jakartaToday,
} from "../src/lib/format/date";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  attachFullPageScreenshot,
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { findOrCreateProperty } from "./support/property-fixtures";
import { rangeText } from "../src/components/client/list/utils";
import { menuSections, ORG_HOME_URL, openMenu } from "./support/shell";
import { HENDRA, RUDI, type SeededStaff } from "./support/staff";

// Penagihan, the month's invoices, against the real API and its worker. The admin test makes its
// own data (a kos unit, a tenant, a lease starting on the 1st of this month, a partial cash
// payment) and then reads the screen; other specs write money to the same organization at the
// same time, so every figure is read from the listInvoices answer the page itself received, never
// from a number read earlier. The local seed may hold overdue invoices that CI's does not, so no
// test needs arrears to exist. Pak Hendra is Admin of the whole of Griya Sejahtera and Supervisor
// in Kost Putri Anggrek; Mas Rudi is Karyawan in Kost Putri Anggrek.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const SEARCH = "Cari penyewa atau unit…";

// Minimum body text per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };

function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

// A name no other run, project or retry uses.
function uniqueName(testInfo: TestInfo, base: string): string {
  const project = testInfo.project.name === "phone" ? "Dua" : "Satu";
  const retry = testInfo.retry === 0 ? "" : ` r${testInfo.retry}`;
  return `${base} ${runId()} ${project}${retry}`;
}

// A tenant number no other run, project or retry uses; 0814-2 is used by no other spec.
function tenantNumber(testInfo: TestInfo): string {
  const run = runId();
  const project = testInfo.project.name === "phone" ? "2" : "1";
  return `0814-2${run.slice(0, 3)}-${run.slice(3)}${project}1${testInfo.retry}`;
}

function orgIdOf(page: Page): string {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

async function openOrganization(page: Page, staff: SeededStaff, name: string) {
  await page.goto("/organizations");
  if (staff.organizations.length > 1) {
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name })
      .click();
  }
  await expect(page).toHaveURL(ORG_HOME_URL);
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

// The next answer to listInvoices whose query matches, started before the action that causes it.
function nextList(
  page: Page,
  matches: (params: URLSearchParams) => boolean,
): Promise<Response> {
  return page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      response.request().method() === "GET" &&
      /\/api\/orgs\/[0-9a-f-]{36}\/invoices$/.test(url.pathname) &&
      matches(url.searchParams)
    );
  });
}

async function listOf(response: Promise<Response>): Promise<InvoiceList> {
  const answer = await response;
  expect(answer.status(), await answer.text()).toBe(200);
  return (await answer.json()) as InvoiceList;
}

// Follows "Penagihan" in the menu, as a person would, and returns the first month's answer.
async function openBilling(
  page: Page,
  testInfo: TestInfo,
): Promise<InvoiceList> {
  const menu = await openMenu(page, testInfo);
  const first = nextList(
    page,
    (params) =>
      !params.has("q") && !params.has("status") && !params.has("period"),
  );
  await menuSections(menu).filter({ hasText: "Penagihan" }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Penagihan" }),
  ).toBeVisible();
  return listOf(first);
}

function invoiceList(page: Page): Locator {
  return page.getByRole("list", { name: LIST_LABEL });
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

async function choose(page: Page, select: string, option: string) {
  await page.getByRole("combobox", { name: select, exact: true }).click();
  await page.getByRole("option", { name: option, exact: true }).click();
}

// The three tiles carry what the summary says (the board's "Terkumpul", "Sisa belum dibayar" and
// "Tunggakan tertua").
async function expectTiles(
  page: Page,
  list: InvoiceList,
  period: string,
  thisMonth: string,
) {
  const tiles = summaryTiles(list.summary, period, thisMonth);
  expect(tiles.map((tile) => tile.label)).toEqual([
    "Terkumpul",
    "Sisa belum dibayar",
    "Tunggakan tertua",
  ]);
  for (const tile of tiles) {
    const group = page.getByRole("group", { name: tile.label, exact: true });
    await expect(group).toContainText(tile.value);
    for (const note of tile.notes) await expect(group).toContainText(note);
  }
  await expect(
    page.getByRole("group", { name: "Terkumpul", exact: true }),
  ).toContainText(formatRupiah(list.summary.paid));
  await expect(
    page.getByRole("group", { name: "Sisa belum dibayar", exact: true }),
  ).toContainText(formatRupiah(list.summary.outstanding));
  await expect(
    page.getByRole("group", { name: "Tunggakan tertua", exact: true }),
  ).toContainText(
    list.summary.oldestOverdue === undefined
      ? "Tidak ada tunggakan"
      : `Terlambat ${list.summary.oldestOverdue.ageDays} hari`,
  );
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("reads the month, filters it, steps through months and opens a tenant", async ({
    page,
  }, testInfo) => {
    test.setTimeout(120_000);
    const outsideRequests = trackOutsideRequests(page);
    await openOrganization(page, HENDRA, GRIYA);
    const orgId = orgIdOf(page);
    const today = jakartaToday(new Date());
    const thisMonth = currentPeriod(today);
    const minFont = MIN_BODY_FONT_PX[testInfo.project.name] ?? 16;

    // A kos unit of the test's own, and a lease starting on the 1st of this month: its first
    // period is this month, and the worker invoices it within seconds.
    const property = await findOrCreateProperty(page, orgId, testInfo, {
      base: "Penagihan",
      kind: "kos",
    });
    const unit = await call<Unit>(
      page,
      "POST",
      `/orgs/${orgId}/properties/${property.id}/units`,
      201,
      {
        label: uniqueName(testInfo, "Kamar Penagihan"),
        terms: [{ months: 1, price: 1_100_000 }],
      },
    );
    const tenant = await call<Tenant>(
      page,
      "POST",
      `/orgs/${orgId}/tenants`,
      201,
      {
        name: uniqueName(testInfo, "Sulastri Purnamasari"),
        phone: tenantNumber(testInfo),
        note: "",
      },
    );
    await call<Lease>(page, "POST", `/orgs/${orgId}/leases`, 201, {
      tenantId: tenant.id,
      unitId: unit.id,
      startDate: `${thisMonth}-01`,
      termMonths: 1,
      depositAmount: 1_100_000,
    });
    const accountPath = `/orgs/${orgId}/tenants/${tenant.id}/account`;
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
    const [made] = account.invoices;
    if (made === undefined) throw new Error("no invoice");
    await call(page, "POST", `/orgs/${orgId}/payments`, 201, {
      requestId: crypto.randomUUID(),
      tenantId: tenant.id,
      amount: 400_000,
      method: "cash",
      receivedOn: today,
      note: "Titip ke Mas Dimas",
    });
    // The rest is promised for the first day a promise may name, as the admin's "Catat janji"
    // would record it: the row says "Janji <date>".
    const promised = await call<TenantAccount>(page, "GET", accountPath, 200);
    const promiseDay = promised.promiseDates?.earliest;
    if (promiseDay === undefined)
      throw new Error("a tenant who owes can promise");
    await call(
      page,
      "POST",
      `/orgs/${orgId}/tenants/${tenant.id}/promises`,
      200,
      {
        kind: "specific_date",
        promisedDate: promiseDay,
      },
    );

    // From the menu, as an admin opens it. The month defaults to this one and stays out of the
    // URL.
    const month = await openBilling(page, testInfo);
    await expect(page).toHaveURL(/\/billing$/);
    await expect(
      page.getByText(monthLabel(thisMonth), { exact: true }),
    ).toBeVisible();
    await expectTiles(page, month, thisMonth, thisMonth);
    await expect(
      page.getByText(rangeText(month.page, month.pageSize, month.total) ?? ""),
    ).toBeVisible();
    await expect(invoiceList(page).getByRole("listitem")).toHaveCount(
      month.items.length,
    );
    await attachFullPageScreenshot(page, testInfo, "billing");

    // Search the test's own tenant: one invoice, partly paid.
    const searched = nextList(
      page,
      (params) => params.get("q") === tenant.name,
    );
    await page.getByRole("searchbox", { name: SEARCH }).fill(tenant.name);
    const found = await listOf(searched);
    expect(found.total).toBe(1);
    const item = found.items[0];
    if (item === undefined) throw new Error("the search found no invoice");
    expect(item.tenantId).toBe(tenant.id);
    expect(item.status).toBe("partial");
    await expect(page).toHaveURL(/[?&]q=/);

    const rows = invoiceList(page).getByRole("listitem");
    await expect(rows).toHaveCount(1);
    const row = rows.first();
    await expect(row).toContainText("Sebagian");
    expect(item.promise?.promisedDate).toBe(promiseDay);
    await expect(row).toContainText(`Janji ${formatDayMonth(promiseDay)}`);
    await row.scrollIntoViewIfNeeded();
    await attachViewportScreenshot(page, testInfo, "billing-promise");
    await expect(row).toContainText(item.unitLabel);
    await expect(row).toContainText(item.propertyName);
    await expect(row).toContainText(formatDate(item.dueDate));
    await expect(row).toContainText(formatRupiah(item.billed));
    await expect(row).toContainText(formatRupiah(item.paid));
    await expect(row).toContainText(formatRupiah(item.outstanding));
    await expect(page.getByText("Menampilkan 1–1 dari 1")).toBeVisible();

    // Reads: body text and the 44px link, whose box does not stretch the row.
    const link = row.getByRole("link", { name: tenant.name });
    expect((await link.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(await fontSize(row.getByText("Sebagian"))).toBeGreaterThanOrEqual(
      minFont,
    );
    expect(
      await fontSize(row.getByText(formatRupiah(item.outstanding)).first()),
    ).toBeGreaterThanOrEqual(minFont);
    expect(
      await fontSize(page.getByText("Menampilkan 1–1 dari 1")),
    ).toBeGreaterThanOrEqual(minFont);

    // The rest is paid by the API: "Lunas" now shows the invoice, and only paid ones.
    await call(page, "POST", `/orgs/${orgId}/payments`, 201, {
      requestId: crypto.randomUUID(),
      tenantId: tenant.id,
      amount: item.outstanding,
      method: "cash",
      receivedOn: today,
    });
    const paidAnswer = nextList(
      page,
      (params) =>
        params.get("status") === "paid" && params.get("q") === tenant.name,
    );
    await choose(page, "Status", "Lunas");
    const paid = await listOf(paidAnswer);
    await expect(page).toHaveURL(/[?&]status=paid(&|$)/);
    expect(paid.items.map((each) => each.id)).toContain(item.id);
    await expect(rows).toHaveCount(paid.items.length);
    for (const each of await rows.all()) {
      await expect(each).toContainText("Lunas");
      await expect(each).not.toContainText("Sebagian");
    }

    // "Terlambat" is debt of every month: no month, so no stepper, and none is sent.
    const overdueAnswer = nextList(
      page,
      (params) => params.get("status") === "overdue",
    );
    await choose(page, "Status", "Terlambat");
    const overdue = await overdueAnswer;
    expect(new URL(overdue.url()).searchParams.has("period")).toBe(false);
    await expect(page).toHaveURL(/[?&]status=overdue(&|$)/);
    await expect(page.getByText("Semua tunggakan, semua bulan")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Bulan sebelumnya" }),
    ).toHaveCount(0);
    // Our tenant owes nothing and is not overdue, so the search matches nothing.
    await expect(page.getByText("Tidak ada tagihan yang cocok.")).toBeVisible();

    // "Hapus filter" takes the search and the status off; the stepper is back.
    // (The default month is already cached, so this one makes no request.)
    await page.getByRole("button", { name: "Hapus filter" }).click();
    await expect(page).not.toHaveURL(/[?&](q|status)=/);
    await expect(
      page.getByRole("button", { name: "Bulan sebelumnya" }),
    ).toBeVisible();

    // The month stepper: this month has no "next"; "previous" moves ?period back one month.
    const previous = previousMonth(thisMonth);
    const next = page.getByRole("button", { name: "Bulan berikutnya" });
    await expect(next).toBeDisabled();
    const back = page.getByRole("button", { name: "Bulan sebelumnya" });
    expect((await back.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect((await back.boundingBox())?.width ?? 0).toBeGreaterThanOrEqual(44);
    const earlierAnswer = nextList(
      page,
      (params) => params.get("period") === previous,
    );
    await back.click();
    const earlier = await listOf(earlierAnswer);
    await expect(page).toHaveURL(new RegExp(`[?&]period=${previous}(&|$)`));
    await expect(
      page.getByText(monthLabel(previous), { exact: true }),
    ).toBeVisible();
    await expectTiles(page, earlier, previous, thisMonth);
    await expect(next).toBeEnabled();
    expect((await next.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);

    await next.click();
    await expect(page).not.toHaveURL(/period=/);
    await expect(
      page.getByText(monthLabel(thisMonth), { exact: true }),
    ).toBeVisible();
    await expect(next).toBeDisabled();

    // The name opens Detail penyewa.
    await page.getByRole("searchbox", { name: SEARCH }).fill(tenant.name);
    await expect(rows).toHaveCount(1);
    await rows.first().getByRole("link", { name: tenant.name }).click();
    await expect(page).toHaveURL(new RegExp(`/tenants/${tenant.id}$`));
    await expect(
      page.getByRole("heading", { level: 1, name: tenant.name }),
    ).toBeVisible();
    expect(outsideRequests).toEqual([]);
  });

  test("a hand-edited month or filter in the URL falls back to the defaults", async ({
    page,
  }) => {
    const outsideRequests = trackOutsideRequests(page);
    await openOrganization(page, HENDRA, GRIYA);
    const orgId = orgIdOf(page);
    const answer = nextList(
      page,
      (params) => !params.has("period") && !params.has("status"),
    );
    await page.goto(
      `/o/${orgId}/billing?period=oktober&status=late&sort=amount&propertyId=x`,
    );
    const list = await listOf(answer);
    await expect(
      page.getByRole("heading", { level: 1, name: "Penagihan" }),
    ).toBeVisible();
    await expect(
      page.getByText(monthLabel(currentPeriod(jakartaToday(new Date()))), {
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("group", { name: "Terkumpul", exact: true }),
    ).toContainText(formatRupiah(list.summary.paid));
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("reads the month with nothing to change", async ({ page }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await openOrganization(page, HENDRA, ANGGREK);
    const thisMonth = currentPeriod(jakartaToday(new Date()));
    const month = await openBilling(page, testInfo);

    await expectTiles(page, month, thisMonth, thisMonth);
    if (month.total === 0) {
      await expect(
        page.getByText("Belum ada tagihan bulan ini.").first(),
      ).toBeVisible();
    } else {
      await expect(invoiceList(page).getByRole("listitem")).toHaveCount(
        month.items.length,
      );
      await expect(
        page.getByText(
          rangeText(month.page, month.pageSize, month.total) ?? "",
        ),
      ).toBeVisible();
    }
    // Only the month's steppers: nothing to record, pay or correct.
    await expect(
      page.getByRole("button", {
        name: /Catat|Bayar|Koreksi|Kirim|Aksi|Tambah/,
      }),
    ).toHaveCount(0);
    await page.evaluate(() => window.scrollTo(0, 0));
    await attachFullPageScreenshot(page, testInfo, "billing-supervisor");
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Karyawan", () => {
  test.use({ storageState: RUDI.storageState });

  test("reads the invoices of their own tenants", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await openOrganization(page, RUDI, ANGGREK);
    const orgId = orgIdOf(page);
    // The built-in Karyawan role reads invoices on condition (their own tenants): the menu shows
    // Penagihan for "conditional" access, and the API answers with only what they may see.
    const me = await call<OrgMeResponse>(page, "GET", `/orgs/${orgId}/me`, 200);
    expect(
      me.capabilities.find((capability) => capability.action === "invoice:read")
        ?.access,
    ).toBe("conditional");

    const menu = await openMenu(page, testInfo);
    const answer = nextList(page, () => true);
    await menuSections(menu).filter({ hasText: "Penagihan" }).click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Penagihan" }),
    ).toBeVisible();
    const response = await answer;
    expect(response.status()).toBe(200);
    await expect(page.getByText(FORBIDDEN_TEXT)).toHaveCount(0);
    await expect(
      page.getByRole("group", { name: "Terkumpul", exact: true }),
    ).toBeVisible();
    expect(outsideRequests).toEqual([]);
  });
});
