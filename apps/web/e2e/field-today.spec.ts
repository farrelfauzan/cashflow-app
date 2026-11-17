import { expect, test, type Locator, type Page } from "@playwright/test";

import {
  CASH_CARD_LABEL,
  DEPOSIT_LINK_TEXT,
  EMPTY_TEXT,
  LIST_LABEL,
  owedText,
  placeText,
  reasonChip,
  TITLE,
} from "../src/components/client/field-today/utils";
import type {
  CashTotal,
  FieldTodayList,
  OrgMeResponse,
} from "../src/lib/api/generated/model";
import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { minBodyFontPx } from "./support/projects";
import {
  FIELD_TODAY_URL,
  HOME_HEADING,
  isPhone,
  menuSections,
  openAccountMenu,
  openMenu,
  ORG_HOME_URL,
  orgHomeOf,
} from "./support/shell";
import { HENDRA, RUDI, UDIN } from "./support/staff";

// "Tagihan hari ini" (board HP-Tagihan, column 1) and the karyawan's menu (board
// HP-Tab-Karyawan), against the real API. Mas Rudi is the seeded Karyawan in Kost Putri Anggrek,
// collector of Ayu Lestari, who still owes on an invoice past its due date (the seed's partial
// payer). Mas Udin collects from nobody. Pak Hendra is an admin (Griya Sejahtera) and Supervisor
// (Kost Putri Anggrek): report:read, no field work.
//
// Nothing here records cash or promises: the cash a karyawan holds is shared by every project
// running in parallel (cash-deposit.spec.ts records and deposits Mas Rudi's on the phone), so the
// cash card is checked for its shape, and the rows against the API's answer at the same moment.

const ANGGREK = "Kost Putri Anggrek";
const COLLECTED_TENANT = "Ayu Lestari";
const OWED_LINE = /^Sisa Rp[\d.]+ · telat Rp[\d.]+$/;

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

async function access(page: Page, orgId: string, action: string) {
  const me = await call<OrgMeResponse>(page, `/orgs/${orgId}/me`);
  return me.capabilities.find((capability) => capability.action === action)
    ?.access;
}

async function fontSize(target: Locator): Promise<number> {
  return target.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

function heading(page: Page) {
  return page.getByRole("heading", { level: 1, name: TITLE });
}

test.describe("Karyawan", () => {
  test.use({ storageState: RUDI.storageState });

  test("lands on Tagihan hari ini, with field work first and no Beranda", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    // One organization: the picker goes straight in, and without report:read the organization
    // opens on his first section.
    await page.goto("/organizations");
    await expect(page).toHaveURL(FIELD_TODAY_URL);
    await expect(heading(page)).toBeVisible();
    const orgId = orgIdOf(page);
    expect(await access(page, orgId, "report:read")).toBe("deny");
    expect(await access(page, orgId, "cash:record")).toBe("conditional");

    const menu = await openMenu(page, testInfo);
    if (isPhone(testInfo)) {
      await expect(menuSections(menu)).toHaveText([
        "Tagihan",
        "Kunjungan",
        "Setor",
        "Penyewa",
      ]);
      await expect(page.getByRole("button", { name: "Lainnya" })).toBeVisible();
      const sheet = await openAccountMenu(page, testInfo);
      await expect(sheet.getByRole("link", { name: "Beranda" })).toHaveCount(0);
      await expect(
        sheet.getByRole("link", { name: "Penagihan" }),
      ).toBeVisible();
      await page.keyboard.press("Escape");
    } else {
      await expect(menuSections(menu)).toHaveText([
        "Tagihan hari ini",
        "Kunjungan hari ini",
        "Setor tunai",
        "Penyewa",
        "Penagihan",
      ]);
    }
    await expect(
      menuSections(menu).filter({ hasText: /^Tagihan( hari ini)?$/ }),
    ).toHaveAttribute("aria-current", "page");
    await expect(menuSections(menu).filter({ hasText: "Beranda" })).toHaveCount(
      0,
    );

    // A typed address of the home goes the same way.
    await page.goto(orgHomeOf(page.url()));
    await expect(page).toHaveURL(FIELD_TODAY_URL);
    await expect(heading(page)).toBeVisible();
    await expect(
      page.getByRole("heading", { level: 1, name: HOME_HEADING }),
    ).toHaveCount(0);
    expect(outsideRequests).toEqual([]);
  });

  test("lists the tenants he collects from, with what they owe", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await page.goto("/organizations");
    await expect(page).toHaveURL(FIELD_TODAY_URL);
    const orgId = orgIdOf(page);

    const list = page.getByRole("list", { name: LIST_LABEL });
    await expect(list).toBeVisible();
    const today = await call<FieldTodayList>(
      page,
      `/orgs/${orgId}/field/today?pageSize=100`,
    );
    const item = today.items.find((row) => row.tenantName === COLLECTED_TENANT);
    if (item === undefined) {
      throw new Error(`${COLLECTED_TENANT} is not on Mas Rudi's list`);
    }
    // The seed leaves her owing past a due date.
    expect(item.reason).toBe("overdue");
    await expect(list.getByRole("listitem")).toHaveCount(today.items.length);

    const row = list
      .getByRole("listitem")
      .filter({ hasText: COLLECTED_TENANT });
    await expect(row).toHaveCount(1);
    await expect(row).toContainText(placeText(item.units));
    await expect(row).toContainText(reasonChip(item).label);
    // What she owes as the API counts it. The phone's cash-deposit spec may take cash from her
    // at the same time, so the amounts are matched by their shape; the unit tests pin the
    // wording (owedText).
    expect(owedText(item)).toMatch(OWED_LINE);
    await expect(row.getByText(OWED_LINE)).toBeVisible();

    // His own cash in hand above the list, with the way to hand it over while he holds any.
    const card = page.getByRole("region", { name: "Tunai di tangan" });
    await expect(card).toContainText(CASH_CARD_LABEL);
    await expect(card).toContainText(/Rp[\d.]+/);
    await expect(card).toContainText(/\d+ kwitansi/);

    // Readable sizes: the name is body text, the chip and the owed line at least the minimum.
    const min = minBodyFontPx(testInfo);
    expect(
      await fontSize(row.getByText(COLLECTED_TENANT, { exact: true })),
    ).toBeGreaterThanOrEqual(min);
    expect(
      await fontSize(row.getByText(reasonChip(item).label, { exact: true })),
    ).toBeGreaterThanOrEqual(min);
    expect(await fontSize(row.getByText(OWED_LINE))).toBeGreaterThanOrEqual(
      min,
    );
    await attachFullPageScreenshot(page, testInfo, "field-today");

    const cash = await call<CashTotal>(page, `/orgs/${orgId}/field/cash`);
    if (cash.count > 0) {
      const link = card.getByRole("link", { name: DEPOSIT_LINK_TEXT });
      await expect(link).toBeVisible();
      const box = await link.boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
      await link.click();
      await expect(page).toHaveURL(/\/field\/deposit$/);
      await expect(
        page.getByRole("heading", { level: 1, name: "Setor tunai" }),
      ).toBeVisible();
    }
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Karyawan who collects from nobody", () => {
  test.use({ storageState: UDIN.storageState });

  test("sees a calm empty list and no cash to hand over", async ({
    page,
  }, testInfo) => {
    // Mas Udin collects in no spec: the real API answers no tenant and no cash.
    await page.goto("/organizations");
    await expect(page).toHaveURL(FIELD_TODAY_URL);
    const orgId = orgIdOf(page);
    const today = await call<FieldTodayList>(
      page,
      `/orgs/${orgId}/field/today`,
    );
    expect(today.total).toBe(0);

    await expect(page.getByText(EMPTY_TEXT)).toBeVisible();
    expect(await fontSize(page.getByText(EMPTY_TEXT))).toBeGreaterThanOrEqual(
      minBodyFontPx(testInfo),
    );
    const card = page.getByRole("region", { name: "Tunai di tangan" });
    await expect(card).toContainText("Rp0");
    await expect(card).toContainText("0 kwitansi");
    await expect(
      card.getByRole("link", { name: DEPOSIT_LINK_TEXT }),
    ).toHaveCount(0);
    await attachFullPageScreenshot(page, testInfo, "field-today-empty");
  });
});

test.describe("Admin and Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("keep Beranda first and have no Tagihan hari ini", async ({
    page,
  }, testInfo) => {
    await page.goto("/organizations");
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name: ANGGREK })
      .click();
    await expect(page).toHaveURL(ORG_HOME_URL);
    await expect(
      page.getByRole("heading", { level: 1, name: HOME_HEADING }),
    ).toBeVisible();
    const sections = menuSections(await openMenu(page, testInfo));
    await expect(sections.first()).toHaveText("Beranda");
    await expect(sections.filter({ hasText: /^Tagihan/ })).toHaveCount(0);
    await expect(sections.filter({ hasText: /^Setor/ })).toHaveCount(0);
  });
});
