import {
  expect,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";
import { projectName } from "./projects";

// The app shell from a test's point of view: on iPad and desktop the side menu is always open;
// on the phone up to four sections are a tab bar at the bottom (boards HP-Tab and
// HP-Tab-Karyawan) and the rest (Anggota tim, Ganti usaha, Telegram, Keluar) is in the sheet
// behind its "Lainnya" tab.

export const ORG_HOME_URL = /\/o\/[0-9a-f-]{36}$/;

// Where opening an organization lands: Beranda for a member who may read reports, Tagihan hari
// ini for a karyawan (the home page sends a member without reports to their first section).
export const ORG_LANDING_URL = /\/o\/[0-9a-f-]{36}(\/field)?$/;
export const FIELD_TODAY_URL = /\/o\/[0-9a-f-]{36}\/field$/;

// The organization's home (/o/<id>) of any page inside it.
export function orgHomeOf(url: string): string {
  const home = /^(.*\/o\/[0-9a-f-]{36})(?:[/?#]|$)/.exec(url)?.[1];
  if (home === undefined) throw new Error(`not an organization's page: ${url}`);
  return home;
}

// Beranda's heading greets by the hour in Jakarta ("Selamat pagi").
export const HOME_HEADING = /^Selamat (pagi|siang|sore|malam)$/;

export function isPhone(testInfo: TestInfo): boolean {
  return projectName(testInfo) === "phone";
}

// The menu with the sections: the side menu, or the phone's tab bar. Nothing to open on either.
export async function openMenu(page: Page, testInfo: TestInfo) {
  const menu = isPhone(testInfo)
    ? page.getByRole("navigation", { name: "Menu utama" }).filter({
        has: page.getByRole("button", { name: "Lainnya" }),
      })
    : page.getByRole("navigation", { name: "Menu utama" });
  await expect(menu).toBeVisible();
  return menu;
}

// The menu's section links, in order, from an open menu.
export function menuSections(menu: Locator) {
  return menu.getByRole("list").getByRole("link");
}

// The sections the menu shows at this size, from the side menu's list: the phone's tab bar has
// at most four, in this order (field work first), under their short names; the rest moves to
// "Lainnya".
const PHONE_TAB_ORDER = [
  "Tagihan hari ini",
  "Setor tunai",
  "Beranda",
  "Penyewa",
  "Penagihan",
  "Properti",
];
const TAB_LABELS: Readonly<Record<string, string>> = {
  "Tagihan hari ini": "Tagihan",
  "Setor tunai": "Setor",
};
export function tabLabel(section: string): string {
  return TAB_LABELS[section] ?? section;
}
export function expectedSections(
  testInfo: TestInfo,
  sideMenu: readonly string[],
): string[] {
  if (!isPhone(testInfo)) return [...sideMenu];
  return PHONE_TAB_ORDER.filter((tab) => sideMenu.includes(tab))
    .slice(0, 4)
    .map(tabLabel);
}

// Follows a section of the menu, as a person would: a link in the side menu, a tab on the phone
// (under its short name), or on the phone a section that is not a tab from the "Lainnya" sheet.
export async function openSection(
  page: Page,
  testInfo: TestInfo,
  section: string,
) {
  const menu = await openMenu(page, testInfo);
  if (!isPhone(testInfo)) {
    await menuSections(menu).filter({ hasText: section }).click();
    return;
  }
  const tab = menuSections(menu).filter({
    hasText: new RegExp(`^${tabLabel(section)}$`),
  });
  if ((await tab.count()) > 0) {
    await tab.click();
    return;
  }
  const sheet = await openAccountMenu(page, testInfo);
  await sheet.getByRole("link").filter({ hasText: section }).click();
}

// Pages outside the staff shell (login, the tenant's payment page) have no menu and no tab bar.
export async function expectNoMenu(page: Page) {
  await expect(
    page.getByRole("navigation", { name: "Menu utama" }),
  ).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Lainnya" })).toHaveCount(0);
}

// Where "Ganti usaha", "Telegram" and "Keluar" are: the side menu, or on the phone the "Lainnya"
// sheet, which this opens.
export async function openAccountMenu(page: Page, testInfo: TestInfo) {
  if (!isPhone(testInfo)) return openMenu(page, testInfo);
  await page.getByRole("button", { name: "Lainnya" }).click();
  const sheet = page.getByRole("dialog", { name: "Lainnya" });
  await expect(sheet).toBeVisible();
  return sheet;
}
