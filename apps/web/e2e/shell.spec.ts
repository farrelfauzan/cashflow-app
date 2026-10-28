import { expect, test, type Page } from "@playwright/test";

import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { isPhone, menuSections, ORG_HOME_URL, openMenu } from "./support/shell";
import { HENDRA, RUDI, SARI, type SeededStaff } from "./support/staff";

// The app shell inside an organization: the menu shows what the member's capabilities from the
// API allow (never their role name), marks the current page, and is the way to switch
// organization and sign out. On the phone it lives in a sheet behind "Buka menu".

// Minimum body text per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };

// Opens the member's organization: straight in for one organization, by name for several.
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

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees every section, follows a link and marks the current page", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await openOrganization(page, HENDRA, "Griya Sejahtera");
    const home = page.url();
    await expect(
      page.getByRole("heading", { level: 1, name: "Beranda" }),
    ).toBeVisible();
    await attachFullPageScreenshot(page, testInfo, "shell-home");

    const sections = menuSections(await openMenu(page, testInfo));
    await expect(sections).toHaveText([
      "Beranda",
      "Properti",
      "Penyewa",
      "Anggota tim",
    ]);
    await expect(sections.filter({ hasText: "Beranda" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    for (const link of await sections.all()) {
      const box = await link.boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    }
    const size = await sections
      .first()
      .evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
    expect(size).toBeGreaterThanOrEqual(
      MIN_BODY_FONT_PX[testInfo.project.name] ?? 16,
    );
    await attachFullPageScreenshot(page, testInfo, "shell-menu");

    await sections.filter({ hasText: "Anggota tim" }).click();
    await expect(page).toHaveURL(`${home}/members`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Anggota tim" }),
    ).toBeVisible();
    if (isPhone(testInfo)) {
      // Following a link closes the sheet.
      await expect(page.getByRole("dialog")).toHaveCount(0);
    }
    const menu = await openMenu(page, testInfo);
    await expect(
      menu.getByRole("link", { name: "Anggota tim" }),
    ).toHaveAttribute("aria-current", "page");
    await expect(
      menu.getByRole("link", { name: "Beranda" }),
    ).not.toHaveAttribute("aria-current");
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Karyawan", () => {
  test.use({ storageState: RUDI.storageState });

  test("sees only the sections his capabilities allow", async ({
    page,
  }, testInfo) => {
    await openOrganization(page, RUDI, "Kost Putri Anggrek");
    const menu = await openMenu(page, testInfo);
    await expect(menu.getByText("Karyawan", { exact: true })).toBeVisible();
    await expect(menuSections(menu)).toHaveText(["Beranda", "Penyewa"]);
    await attachFullPageScreenshot(page, testInfo, "shell-karyawan");
  });
});

test.describe("Admin limited to one property", () => {
  test.use({ storageState: SARI.storageState });

  test("sees the property sections", async ({ page }, testInfo) => {
    await openOrganization(page, SARI, "Griya Sejahtera");
    const sections = menuSections(await openMenu(page, testInfo));
    await expect(sections.filter({ hasText: "Properti" })).toHaveCount(1);
    await expect(sections.filter({ hasText: "Penyewa" })).toHaveCount(1);
  });
});
