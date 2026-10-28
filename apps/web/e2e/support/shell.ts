import {
  expect,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

// The app shell from a test's point of view: on iPad the side menu is always open; on the
// phone it opens from "Buka menu" in a sheet.

export const ORG_HOME_URL = /\/o\/[0-9a-f-]{36}$/;

export function isPhone(testInfo: TestInfo): boolean {
  return testInfo.project.name === "phone";
}

// Opens the phone's menu sheet (a no-op on iPad) and returns the menu.
export async function openMenu(page: Page, testInfo: TestInfo) {
  if (isPhone(testInfo)) {
    await page.getByRole("button", { name: "Buka menu" }).click();
    await expect(page.getByRole("dialog")).toBeVisible();
  }
  const menu = page.getByRole("navigation", { name: "Menu utama" });
  await expect(menu).toBeVisible();
  return menu;
}

// The menu's section links, in order, from an open menu.
export function menuSections(menu: Locator) {
  return menu.getByRole("list").getByRole("link");
}
