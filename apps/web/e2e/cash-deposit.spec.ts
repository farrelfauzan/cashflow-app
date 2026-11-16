import { expect, test, type Page, type TestInfo } from "@playwright/test";

import {
  AMOUNT_LABEL,
  amountFieldText,
  EMPTY_TEXT,
  FOOTER_CAPTION,
  FORBIDDEN_TEXT,
  PENDING_BADGE,
  RECEIPTS_LABEL,
} from "../src/components/client/deposit/utils";
import type {
  CashDeposit,
  CashPaymentInput,
  CashPaymentResult,
  CashTotal,
  OrgMeResponse,
  TenantList,
  UndepositedCashList,
} from "../src/lib/api/generated/model";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { cashPhoto } from "./support/cash-photo";
import { minBodyFontPx, projectDigit, projectName } from "./support/projects";
import {
  isPhone,
  menuSections,
  openAccountMenu,
  openMenu,
  openSection,
  ORG_LANDING_URL,
} from "./support/shell";
import { HENDRA, RUDI, UDIN, type SeededStaff } from "./support/staff";
import { expectToast } from "./support/toast";

// "Setor tunai" (board HP-Setor), against the real API. Mas Rudi is the seeded Karyawan in Kost
// Putri Anggrek, collector of Ayu Lestari; his capabilities grant cash:deposit on his own cash.
// Pak Hendra is Admin of the whole of Griya Sejahtera, where Admin holds no cash:deposit.
//
// The cash a karyawan holds is one total per member, and a deposit takes all of it. The three
// size projects run in parallel as the same Mas Rudi, so only the phone (the karyawan's device)
// records cash and deposits it; the other sizes check the menu and the screen without changing
// what he holds.

const ANGGREK = "Kost Putri Anggrek";
const GRIYA = "Griya Sejahtera";
const BULANAN = "Kos Bulanan Sejahtera";
const COLLECTED_TENANT = "Ayu Lestari";
const SECTION = "Setor tunai";
const DEPOSIT_URL = /\/o\/[0-9a-f-]{36}\/field\/deposit$/;

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
  await expect(page).toHaveURL(ORG_LANDING_URL);
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

async function access(page: Page, orgId: string, action: string) {
  const me = await call<OrgMeResponse>(page, "GET", `/orgs/${orgId}/me`, 200);
  return me.capabilities.find((capability) => capability.action === action)
    ?.access;
}

// The deposit's one primary action, "Setor Rp…".
function depositButton(page: Page) {
  return page.getByRole("button", { name: /^Setor( Rp[\d.]+)?$/ });
}

async function fontSize(page: Page, text: string): Promise<number> {
  return page
    .getByText(text, { exact: true })
    .first()
    .evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

// The screen opened from the menu: on iPad and desktop the side menu, on the phone its "Setor" tab.
async function openDeposit(page: Page, testInfo: TestInfo) {
  await openSection(page, testInfo, SECTION);
  await expect(page).toHaveURL(DEPOSIT_URL);
  await expect(
    page.getByRole("heading", { level: 1, name: SECTION }),
  ).toBeVisible();
}

test.describe("Karyawan", () => {
  test.use({ storageState: RUDI.storageState });

  test("finds Setor tunai in the menu", async ({ page }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await openOrganization(page, RUDI, ANGGREK);
    const orgId = orgIdOf(page);
    // Granted on his own cash only: "conditional", which shows the item.
    expect(await access(page, orgId, "cash:deposit")).toBe("conditional");

    if (isPhone(testInfo)) {
      // A tab under its short name, field work first (board HP-Tab-Karyawan), so not in
      // "Lainnya".
      await expect(
        menuSections(await openMenu(page, testInfo)).filter({
          hasText: /^Setor$/,
        }),
      ).toHaveCount(1);
      const sheet = await openAccountMenu(page, testInfo);
      await expect(sheet.getByRole("link", { name: SECTION })).toHaveCount(0);
      await page.keyboard.press("Escape");
    } else {
      await expect(
        menuSections(await openMenu(page, testInfo)).filter({
          hasText: SECTION,
        }),
      ).toHaveCount(1);
    }

    await openDeposit(page, testInfo);
    // Whatever he holds now, the screen shows one of its states and no error.
    await expect(
      depositButton(page)
        .or(page.getByText(EMPTY_TEXT))
        .or(page.getByRole("region", { name: PENDING_BADGE }))
        .first(),
    ).toBeVisible();
    await expect(page.getByText(FORBIDDEN_TEXT)).toHaveCount(0);
    expect(outsideRequests).toEqual([]);
  });

  test("deposits the cash held", async ({ page }, testInfo) => {
    test.skip(
      projectName(testInfo) !== "phone",
      "One karyawan's cash is shared by every project; only the phone deposits it.",
    );
    const outsideRequests = trackOutsideRequests(page);
    await openOrganization(page, RUDI, ANGGREK);
    const orgId = orgIdOf(page);

    // A cash payment from a tenant he collects from, at an amount of this run and project.
    const tenants = await call<TenantList>(
      page,
      "GET",
      `/orgs/${orgId}/tenants?q=${encodeURIComponent(COLLECTED_TENANT)}`,
      200,
    );
    const tenant = tenants.items.find((t) => t.name === COLLECTED_TENANT);
    if (tenant === undefined) throw new Error(`${COLLECTED_TENANT} not seeded`);
    const amount = Number(`1${runId()}${projectDigit(testInfo)}0`);
    const input: CashPaymentInput = {
      requestId: crypto.randomUUID(),
      amount,
      photoUploadId: await cashPhoto(page, page.request, orgId, tenant.id),
    };
    const recorded = await call<CashPaymentResult>(
      page,
      "POST",
      `/orgs/${orgId}/tenants/${tenant.id}/cash-payments`,
      201,
      input,
    );
    const paymentId = recorded.payment.id;

    // What he holds now: this payment and any an earlier run left undeposited.
    const cash = await call<CashTotal>(
      page,
      "GET",
      `/orgs/${orgId}/field/cash`,
      200,
    );
    const held = await call<UndepositedCashList>(
      page,
      "GET",
      `/orgs/${orgId}/field/cash/payments?pageSize=100`,
      200,
    );
    expect(held.items.map((item) => item.paymentId)).toContain(paymentId);

    await openDeposit(page, testInfo);
    await expect(
      page.getByText(`${cash.count} kwitansi`).first(),
    ).toBeVisible();
    const receipts = page.getByRole("list", { name: RECEIPTS_LABEL });
    await expect(receipts.getByRole("listitem")).toHaveCount(
      Math.min(held.total, 100),
    );
    const row = receipts
      .getByRole("listitem")
      .filter({ hasText: COLLECTED_TENANT })
      .filter({ hasText: formatRupiah(amount) });
    await expect(row).toHaveCount(1);
    // Recorded just now with its photo (decision 50): still being scanned, or already clean.
    await expect(row).toContainText(/Foto (sedang diperiksa|tersimpan)/);
    await expect(row).not.toContainText("Foto belum ada");

    // The amount starts at the total, and the button names it.
    await expect(page.getByLabel(AMOUNT_LABEL)).toHaveValue(
      amountFieldText(cash.amount),
    );
    const button = depositButton(page);
    await expect(button).toHaveText(`Setor ${formatRupiah(cash.amount)}`);
    await expect(page.getByText(FOOTER_CAPTION)).toBeVisible();
    const box = await button.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(await fontSize(page, COLLECTED_TENANT)).toBeGreaterThanOrEqual(
      minBodyFontPx(testInfo),
    );
    await attachFullPageScreenshot(page, testInfo, "cash-deposit-form");

    const answer = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        /\/api\/orgs\/[0-9a-f-]{36}\/field\/cash-deposits$/.test(
          new URL(response.url()).pathname,
        ),
    );
    await button.click();
    const response = await answer;
    expect(response.status(), await response.text()).toBe(201);
    const deposit = (await response.json()) as CashDeposit;
    expect(deposit.amount).toBe(cash.amount);
    expect(deposit.paymentCount).toBe(cash.count);

    await expectToast(
      page,
      "Setoran dicatat",
      "create",
      "Admin akan menghitungnya.",
    );
    const pending = page
      .getByRole("region", { name: PENDING_BADGE })
      .filter({ hasText: formatRupiah(deposit.amount) })
      .filter({ hasText: `${deposit.paymentCount} kwitansi` });
    await expect(pending.first()).toBeVisible();
    // Nothing is held any more: no form, and no empty line while a deposit waits.
    await expect(depositButton(page)).toHaveCount(0);
    await expect(page.getByText(EMPTY_TEXT)).toHaveCount(0);
    await attachFullPageScreenshot(page, testInfo, "cash-deposit-pending");
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Karyawan with nothing to deposit", () => {
  test.use({ storageState: UDIN.storageState });

  test("shows a calm empty state", async ({ page }, testInfo) => {
    // Mas Udin collects in no spec, so the real API answers nothing held and no deposit.
    await openOrganization(page, UDIN, BULANAN);
    const orgId = orgIdOf(page);
    const cash = await call<CashTotal>(
      page,
      "GET",
      `/orgs/${orgId}/field/cash`,
      200,
    );
    expect(cash).toEqual({ amount: 0, count: 0 });

    await openDeposit(page, testInfo);
    await expect(page.getByText(EMPTY_TEXT)).toBeVisible();
    await expect(depositButton(page)).toHaveCount(0);
    expect(await fontSize(page, EMPTY_TEXT)).toBeGreaterThanOrEqual(
      minBodyFontPx(testInfo),
    );
  });
});

test.describe("Admin without cash:deposit", () => {
  test.use({ storageState: HENDRA.storageState });

  test("has no Setor tunai", async ({ page }, testInfo) => {
    await openOrganization(page, HENDRA, GRIYA);
    const orgId = orgIdOf(page);
    expect(await access(page, orgId, "cash:deposit")).not.toBe("allow");
    expect(await access(page, orgId, "cash:deposit")).not.toBe("conditional");

    await expect(
      menuSections(await openMenu(page, testInfo)).filter({ hasText: SECTION }),
    ).toHaveCount(0);
    if (isPhone(testInfo)) {
      const sheet = await openAccountMenu(page, testInfo);
      await expect(sheet.getByRole("link", { name: SECTION })).toHaveCount(0);
      await page.keyboard.press("Escape");
    }

    // A typed address: the screen says so and offers nothing.
    await page.goto(`/o/${orgId}/field/deposit`);
    await expect(
      page.getByRole("heading", { level: 1, name: SECTION }),
    ).toBeVisible();
    await expect(page.getByText(FORBIDDEN_TEXT)).toBeVisible();
    await expect(depositButton(page)).toHaveCount(0);
  });
});
