import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from "@playwright/test";

import {
  CONFIRMED_CAPTION,
  CONFIRMED_LIST_LABEL,
  CONFIRMED_TAB_TEXT,
  FORBIDDEN_TEXT,
  PENDING_LIST_LABEL,
  PENDING_TAB_TEXT,
  RECEIVED_HINT,
  RECEIVED_LABEL,
  SAVE_LABEL,
  SHEET_TITLE,
  SUBTITLE,
} from "../src/components/client/deposit-count/utils";
import type {
  CashDeposit,
  CashDepositConfirmationInput,
  CashDepositInput,
  CashPaymentInput,
  CashPaymentResult,
  CashTotal,
  OrgMeResponse,
  TenantList,
} from "../src/lib/api/generated/model";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  attachFullPageScreenshot,
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { minBodyFontPx, projectDigit, projectName } from "./support/projects";
import { ORG_HOME_URL } from "./support/shell";
import { DIMAS, HENDRA, JUWITA, SARI, type SeededStaff } from "./support/staff";
import { expectToast } from "./support/toast";

// "Setoran tunai" (board W20b), against the real API. Pak Dimas is Admin of the whole of Kos
// Bulanan Sejahtera (payment:check for the whole organization), so he counts deposits. Mbak
// Juwita is a Karyawan there, collector of Oky Firmansyah; no other spec records her cash. Pak
// Hendra is Supervisor of Kost Putri Anggrek (cash:read, no payment:check), and Mbak Sari an
// Admin limited to Kos Melati in Griya Sejahtera (neither for the whole organization).
//
// A karyawan's cash is one total, and a deposit takes all of it, so only the iPad (the admin's
// device) records, deposits and counts; the other sizes check the screen without changing it.

const BULANAN = "Kos Bulanan Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const GRIYA = "Griya Sejahtera";
const COLLECTED_TENANT = "Oky Firmansyah";
const TITLE = "Setoran tunai";
const DEPOSITS_URL = /\/o\/[0-9a-f-]{36}\/billing\/deposits$/;

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

// Setup through the API, as a signed-in member.
async function call<T>(
  request: APIRequestContext,
  method: "GET" | "POST",
  path: string,
  want: number,
  data?: object,
): Promise<T> {
  const response = await request.fetch(`/api${path}`, { method, data });
  expect(response.status(), await response.text()).toBe(want);
  return (await response.json()) as T;
}

async function access(page: Page, orgId: string, action: string) {
  const me = await call<OrgMeResponse>(
    page.request,
    "GET",
    `/orgs/${orgId}/me`,
    200,
  );
  return me.capabilities.find((capability) => capability.action === action)
    ?.access;
}

// Penagihan's link to the screen, in the page (not the menus).
function depositLink(page: Page) {
  return page.getByRole("main").getByRole("link", { name: /^Setoran tunai/ });
}

function countButtons(page: Page) {
  return page.getByRole("button", { name: /^Hitung setoran / });
}

async function fontSize(page: Page, text: string): Promise<number> {
  return page
    .getByText(text, { exact: true })
    .first()
    .evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

async function expectScreen(page: Page) {
  await expect(page).toHaveURL(DEPOSITS_URL);
  await expect(
    page.getByRole("heading", { level: 1, name: TITLE }),
  ).toBeVisible();
  await expect(page.getByText(SUBTITLE)).toBeVisible();
  await expect(
    page
      .getByRole("navigation", { name: "Lokasi halaman" })
      .getByRole("link", { name: "Penagihan" }),
  ).toBeVisible();
  await expect(
    page.getByRole("tab", { name: /^Menunggu dihitung/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("tab", { name: CONFIRMED_TAB_TEXT }),
  ).toBeVisible();
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: DIMAS.storageState });

  test("opens Setoran tunai from Penagihan", async ({ page }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await openOrganization(page, DIMAS, BULANAN);
    const orgId = orgIdOf(page);
    expect(await access(page, orgId, "payment:check")).toBe("allow");

    await page.goto(`/o/${orgId}/billing`);
    const link = depositLink(page);
    await expect(link).toBeVisible();
    const box = await link.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    await attachFullPageScreenshot(page, testInfo, "billing-deposit-link");
    await link.click();
    await expectScreen(page);

    // The side menu (iPad and desktop) keeps Penagihan active.
    if (projectName(testInfo) !== "phone") {
      await expect(
        page
          .getByRole("navigation")
          .getByRole("link", { name: "Penagihan", exact: true })
          .and(page.locator("[aria-current=page]")),
      ).toHaveCount(1);
    }

    // Either tab shows its list or its empty line, never an error.
    for (const tab of [PENDING_TAB_TEXT, CONFIRMED_TAB_TEXT]) {
      await page.getByRole("tab", { name: new RegExp(`^${tab}`) }).click();
      await expect(
        page
          .getByRole("list", {
            name:
              tab === PENDING_TAB_TEXT
                ? PENDING_LIST_LABEL
                : CONFIRMED_LIST_LABEL,
          })
          .or(page.getByText(/^Belum ada setoran yang/))
          .first(),
      ).toBeVisible();
    }
    await expect(page).toHaveURL(/tab=confirmed$/);
    const tabBox = await page
      .getByRole("tab", { name: CONFIRMED_TAB_TEXT })
      .boundingBox();
    expect(tabBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    await expect(page.getByText(FORBIDDEN_TEXT)).toHaveCount(0);
    expect(await fontSize(page, SUBTITLE)).toBeGreaterThanOrEqual(
      minBodyFontPx(testInfo),
    );
    await attachFullPageScreenshot(page, testInfo, "admin-deposits");
    expect(outsideRequests).toEqual([]);
  });

  test("counts a deposit short", async ({
    page,
    playwright,
    baseURL,
  }, testInfo) => {
    test.skip(
      projectName(testInfo) !== "ipad",
      "One karyawan's cash is shared by every project; only the iPad deposits and counts it.",
    );
    const outsideRequests = trackOutsideRequests(page);
    await openOrganization(page, DIMAS, BULANAN);
    const orgId = orgIdOf(page);

    // Mbak Juwita takes cash from a tenant she collects from and hands it all to the office.
    const karyawan = await playwright.request.newContext({
      baseURL,
      storageState: JUWITA.storageState,
    });
    let deposit: CashDeposit;
    try {
      const tenants = await call<TenantList>(
        karyawan,
        "GET",
        `/orgs/${orgId}/tenants?q=${encodeURIComponent(COLLECTED_TENANT)}`,
        200,
      );
      const tenant = tenants.items.find((t) => t.name === COLLECTED_TENANT);
      if (tenant === undefined)
        throw new Error(`${COLLECTED_TENANT} not seeded`);
      const payment: CashPaymentInput = {
        requestId: crypto.randomUUID(),
        amount: Number(`1${runId()}${projectDigit(testInfo)}0`),
      };
      await call<CashPaymentResult>(
        karyawan,
        "POST",
        `/orgs/${orgId}/tenants/${tenant.id}/cash-payments`,
        201,
        payment,
      );
      const cash = await call<CashTotal>(
        karyawan,
        "GET",
        `/orgs/${orgId}/field/cash`,
        200,
      );
      const input: CashDepositInput = {
        requestId: crypto.randomUUID(),
        expectedAmount: cash.amount,
        expectedCount: cash.count,
        amount: cash.amount,
      };
      deposit = await call<CashDeposit>(
        karyawan,
        "POST",
        `/orgs/${orgId}/field/cash-deposits`,
        201,
        input,
      );
    } finally {
      await karyawan.dispose();
    }
    const expected = formatRupiah(deposit.expectedAmount);
    // Short by an amount of this run, so the toast and the row are this run's.
    const short = Number(`5${runId()}`);
    const received = deposit.expectedAmount - short;

    await page.goto(`/o/${orgId}/billing`);
    await expect(depositLink(page)).toHaveAccessibleName(
      /^Setoran tunai, \d+ menunggu dihitung$/,
    );
    await depositLink(page).click();
    await expectScreen(page);

    const row = page
      .getByRole("list", { name: PENDING_LIST_LABEL })
      .getByRole("listitem")
      .filter({ hasText: JUWITA.name })
      .filter({ hasText: expected });
    await expect(row).toHaveCount(1);
    await expect(row).toContainText(String(deposit.paymentCount));
    const count = row.getByRole("button", {
      name: `Hitung setoran ${JUWITA.name}`,
    });
    const countBox = await count.boundingBox();
    expect(countBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(await fontSize(page, JUWITA.name)).toBeGreaterThanOrEqual(
      minBodyFontPx(testInfo),
    );
    await count.click();

    const sheet = page.getByRole("dialog", { name: SHEET_TITLE });
    await expect(sheet).toBeVisible();
    await expect(sheet).toContainText(`${JUWITA.name} · disetor `);
    await expect(sheet).toContainText("Kwitansi dalam setoran ini");
    await expect(
      sheet.getByRole("list", { name: "Kwitansi dalam setoran ini" }),
    ).toContainText(COLLECTED_TENANT);
    // Recorded through the API with no photo.
    await expect(sheet).toContainText("Foto belum ada");
    const field = sheet.getByLabel(RECEIVED_LABEL);
    // The admin types their own count: the field opens empty.
    await expect(field).toHaveValue("");
    await expect(sheet.getByText(RECEIVED_HINT)).toBeVisible();

    await field.fill(String(deposit.expectedAmount));
    await expect(sheet.getByText("Cocok dengan total kwitansi.")).toBeVisible();
    await field.fill(String(received));
    await expect(
      sheet.getByText(
        `Kurang ${formatRupiah(short)} dari total kwitansi. Selisihnya masuk Antrean cek. Tagihan penyewa tidak berubah.`,
      ),
    ).toBeVisible();
    await attachViewportScreenshot(page, testInfo, "admin-deposit-count");

    const answer = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname.endsWith(
          `/cash-deposits/${deposit.id}/confirmation`,
        ),
    );
    await sheet.getByRole("button", { name: SAVE_LABEL }).click();
    const response = await answer;
    expect(response.status(), await response.text()).toBe(201);
    const body = response
      .request()
      .postDataJSON() as CashDepositConfirmationInput;
    // An integer, and no note when none was typed.
    expect(body).toEqual({ receivedAmount: received });
    const counted = (await response.json()) as CashDeposit;
    expect(counted.status).toBe("gap");
    expect(counted.confirmation?.gap).toBe(-short);

    await expectToast(
      page,
      "Hitungan disimpan",
      "caution",
      `Selisih ${formatRupiah(short)} masuk Antrean cek.`,
    );
    await expect(sheet).toHaveCount(0);
    await expect(row).toHaveCount(0);

    await page.getByRole("tab", { name: CONFIRMED_TAB_TEXT }).click();
    const done = page
      .getByRole("list", { name: CONFIRMED_LIST_LABEL })
      .getByRole("listitem")
      .filter({ hasText: JUWITA.name })
      .filter({ hasText: `Kurang ${formatRupiah(short)}` });
    await expect(done).toHaveCount(1);
    await expect(done).toContainText(expected);
    await expect(done).toContainText(formatRupiah(received));
    await expect(done).toContainText("Di Antrean cek");
    await expect(done).toContainText(DIMAS.name);
    await expect(page.getByText(CONFIRMED_CAPTION)).toBeVisible();

    // Counting it again is refused: the API keeps the first count.
    const again = await page.request.fetch(
      `/api/orgs/${orgId}/cash-deposits/${deposit.id}/confirmation`,
      { method: "POST", data: { receivedAmount: received + 1 } },
    );
    expect(again.status()).toBe(409);
    await attachFullPageScreenshot(page, testInfo, "admin-deposit-counted");
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("reads the deposits without counting them", async ({ page }) => {
    await openOrganization(page, HENDRA, ANGGREK);
    const orgId = orgIdOf(page);
    expect(await access(page, orgId, "cash:read")).toBe("allow");
    expect(await access(page, orgId, "payment:check")).not.toBe("allow");

    await page.goto(`/o/${orgId}/billing`);
    await depositLink(page).click();
    await expectScreen(page);
    await expect(page.getByText(FORBIDDEN_TEXT)).toHaveCount(0);
    await expect(
      page
        .getByRole("list", { name: PENDING_LIST_LABEL })
        .or(page.getByText(/^Belum ada setoran yang/))
        .first(),
    ).toBeVisible();
    await expect(countButtons(page)).toHaveCount(0);
    await expect(page.getByText("Dihitung admin lain")).toHaveCount(0);

    await page.getByRole("tab", { name: CONFIRMED_TAB_TEXT }).click();
    await expect(
      page
        .getByRole("list", { name: CONFIRMED_LIST_LABEL })
        .or(page.getByText(/^Belum ada setoran yang/))
        .first(),
    ).toBeVisible();
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });
});

test.describe("Admin limited to one property", () => {
  test.use({ storageState: SARI.storageState });

  test("has no Setoran tunai", async ({ page }) => {
    await openOrganization(page, SARI, GRIYA);
    const orgId = orgIdOf(page);
    expect(await access(page, orgId, "payment:check")).not.toBe("allow");
    expect(await access(page, orgId, "cash:read")).not.toBe("allow");

    await page.goto(`/o/${orgId}/billing`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Penagihan" }),
    ).toBeVisible();
    await expect(depositLink(page)).toHaveCount(0);

    // A typed address: the screen says so and offers nothing; the API agrees.
    await page.goto(`/o/${orgId}/billing/deposits`);
    await expect(
      page.getByRole("heading", { level: 1, name: TITLE }),
    ).toBeVisible();
    await expect(page.getByText(FORBIDDEN_TEXT)).toBeVisible();
    await expect(page.getByRole("tab")).toHaveCount(0);
    const answer = await page.request.get(
      `/api/orgs/${orgId}/cash-deposits?status=pending`,
    );
    expect(answer.status()).toBe(403);
  });
});
