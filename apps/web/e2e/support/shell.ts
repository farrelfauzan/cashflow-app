import {
  expect,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";
import { projectName } from "./projects";

// The app shell from a test's point of view: on iPad and desktop the side menu is always open;
// on the phone the sections are a tab bar at the bottom (board HP-Tab) and the rest (Anggota
// tim, Ganti usaha, Telegram, Keluar) is in the sheet behind its "Lainnya" tab.

export const ORG_HOME_URL = /\/o\/[0-9a-f-]{36}$/;

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
// Beranda, Penyewa, Penagihan and Properti in that order, and Anggota tim moves to "Lainnya".
const PHONE_TABS = ["Beranda", "Penyewa", "Penagihan", "Properti"];
export function expectedSections(
  testInfo: TestInfo,
  sideMenu: readonly string[],
): string[] {
  if (!isPhone(testInfo)) return [...sideMenu];
  return PHONE_TABS.filter((tab) => sideMenu.includes(tab));
}

// Follows a section of the menu, as a person would: a link in the side menu, a tab on the phone,
// or on the phone "Anggota tim" from the "Lainnya" sheet.
export async function openSection(
  page: Page,
  testInfo: TestInfo,
  section: string,
) {
  const onPhoneInSheet = isPhone(testInfo) && !PHONE_TABS.includes(section);
  const menu = onPhoneInSheet
    ? await openAccountMenu(page, testInfo)
    : await openMenu(page, testInfo);
  const links = onPhoneInSheet ? menu.getByRole("link") : menuSections(menu);
  await links.filter({ hasText: section }).click();
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
