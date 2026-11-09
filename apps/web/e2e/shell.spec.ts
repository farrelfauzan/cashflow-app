import {
  expect,
  test,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import {
  animationsSettled,
  attachFullPageScreenshot,
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { expectTouchTarget } from "./support/payment-page";
import { minBodyFontPx } from "./support/projects";
import {
  expectedSections,
  HOME_HEADING,
  isPhone,
  menuSections,
  ORG_HOME_URL,
  openAccountMenu,
  openMenu,
  openSection,
} from "./support/shell";
import { DIMAS, HENDRA, RUDI, SARI, type SeededStaff } from "./support/staff";

// The app shell inside an organization: the menu shows what the member's capabilities from the
// API allow (never their role name), marks the current page, and is the way to switch
// organization and sign out. On iPad and desktop it is the side menu; on the phone it is a tab bar
// fixed to the bottom (Beranda, Penyewa, Penagihan, Properti) and a "Lainnya" tab that opens a
// sheet with the rest (boards HP-Tab and HP-Lainnya).

// The smallest label on the phone's tab bar (the approved board: 13px).
const TAB_LABEL_MIN_PX = 13;
const SIDE_MENU_ADMIN = [
  "Beranda",
  "Properti",
  "Penyewa",
  "Penagihan",
  "Anggota tim",
];

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

function lainnyaButton(page: Page): Locator {
  return page.getByRole("button", { name: "Lainnya" });
}

// The same button by CSS: while the sheet is open the page behind it is hidden from the
// accessibility tree, so a role query cannot find it.
function lainnyaTab(page: Page): Locator {
  return page.locator('nav[aria-label="Menu utama"] button', {
    hasText: "Lainnya",
  });
}

// The phone's tab bar is on screen, flush with the bottom of the viewport, with one even column
// per tab (the sections the member may open, then "Lainnya"), each at least 44px tall and with a
// label of at least 13px.
async function expectTabBar(
  page: Page,
  testInfo: TestInfo,
  sections: readonly string[],
) {
  const bar = await openMenu(page, testInfo);
  await expect(bar).toBeInViewport();
  const box = await bar.boundingBox();
  const viewport = page.viewportSize();
  expect(viewport).not.toBeNull();
  expect(
    Math.abs((box?.y ?? 0) + (box?.height ?? 0) - (viewport?.height ?? 0)),
  ).toBeLessThanOrEqual(1);
  expect(box?.width ?? 0).toBeCloseTo(viewport?.width ?? 0, 0);

  const tabs = bar.getByRole("listitem").locator("a, button");
  await expect(tabs).toHaveCount(sections.length + 1);
  await expect(menuSections(bar)).toHaveText([...sections]);
  const widths: number[] = [];
  for (const tab of await tabs.all()) {
    await expectTouchTarget(tab);
    const size = await tab.evaluate((el) =>
      parseFloat(getComputedStyle(el).fontSize),
    );
    expect(size).toBeGreaterThanOrEqual(TAB_LABEL_MIN_PX);
    widths.push((await tab.boundingBox())?.width ?? 0);
  }
  // Even columns: the same width, up to rounding.
  expect(Math.max(...widths) - Math.min(...widths)).toBeLessThanOrEqual(1);
  return bar;
}

// Which tab links are current: their names.
async function currentTabs(bar: Locator): Promise<string[]> {
  return bar
    .locator('a[aria-current="page"]')
    .evaluateAll((links) => links.map((link) => link.textContent ?? ""));
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
      page.getByRole("heading", { level: 1, name: HOME_HEADING }),
    ).toBeVisible();
    await attachFullPageScreenshot(page, testInfo, "shell-home");

    const sections = menuSections(await openMenu(page, testInfo));
    await expect(sections).toHaveText(
      expectedSections(testInfo, SIDE_MENU_ADMIN),
    );
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
      isPhone(testInfo) ? TAB_LABEL_MIN_PX : minBodyFontPx(testInfo),
    );
    await attachFullPageScreenshot(page, testInfo, "shell-menu");

    await openSection(page, testInfo, "Anggota tim");
    await expect(page).toHaveURL(`${home}/members`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Anggota tim" }),
    ).toBeVisible();
    const menu = await openMenu(page, testInfo);
    if (isPhone(testInfo)) {
      // Following a link closes the sheet, and Anggota tim is no tab: none is current, and
      // "Lainnya" looks current instead (bold, like a current tab).
      await expect(page.getByRole("dialog")).toHaveCount(0);
      await expect(menu.locator('a[aria-current="page"]')).toHaveCount(0);
      await expect(lainnyaButton(page)).toHaveCSS("font-weight", "700");
      await expect(menu.getByRole("link", { name: "Beranda" })).toHaveCSS(
        "font-weight",
        "600",
      );
    } else {
      await expect(
        menu.getByRole("link", { name: "Anggota tim" }),
      ).toHaveAttribute("aria-current", "page");
      await expect(
        menu.getByRole("link", { name: "Beranda" }),
      ).not.toHaveAttribute("aria-current");
    }
    expect(outsideRequests).toEqual([]);
  });

  test("keeps the phone's tab bar at the bottom of every page and marks the current tab", async ({
    page,
  }, testInfo) => {
    test.skip(!isPhone(testInfo), "the tab bar is the phone's");
    await openOrganization(page, HENDRA, "Griya Sejahtera");
    await expect(
      page.getByRole("heading", { level: 1, name: HOME_HEADING }),
    ).toBeVisible();
    // In the tab bar's order, which is not the side menu's.
    const tabs = ["Beranda", "Penyewa", "Penagihan", "Properti"];
    let bar = await expectTabBar(page, testInfo, tabs);
    expect(await currentTabs(bar)).toEqual(["Beranda"]);

    for (const tab of tabs.slice(1)) {
      await bar.getByRole("link", { name: tab }).click();
      await expect(
        page.getByRole("heading", { level: 1, name: tab }),
      ).toBeVisible();
      bar = await expectTabBar(page, testInfo, tabs);
      expect(await currentTabs(bar)).toEqual([tab]);
      await expect(lainnyaButton(page)).toHaveCSS("font-weight", "600");
      if (tab === "Penyewa") {
        await attachViewportScreenshot(page, testInfo, "phone-tab-bar");
      }
    }

    // A page inside a section keeps its tab current: Kos Melati is under Properti.
    await page
      .getByRole("list", { name: "Daftar properti" })
      .getByRole("link", { name: "Buka unit Kos Melati" })
      .click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Kos Melati" }),
    ).toBeVisible();
    bar = await expectTabBar(page, testInfo, tabs);
    expect(await currentTabs(bar)).toEqual(["Properti"]);

    // Anggota tim, from "Lainnya": the bar is there, no tab link is current.
    await openSection(page, testInfo, "Anggota tim");
    await expect(
      page.getByRole("heading", { level: 1, name: "Anggota tim" }),
    ).toBeVisible();
    bar = await expectTabBar(page, testInfo, tabs);
    expect(await currentTabs(bar)).toEqual([]);
  });

  test("opens Lainnya as a sheet with who is signed in, closes it by following a link or with Escape", async ({
    page,
  }, testInfo) => {
    test.skip(!isPhone(testInfo), "the sheet is the phone's");
    await openOrganization(page, HENDRA, "Griya Sejahtera");
    const home = page.url();

    const sheet = await openAccountMenu(page, testInfo);
    await expect(sheet.getByText(HENDRA.name)).toBeVisible();
    await expect(
      sheet.getByText("Admin di Griya Sejahtera", { exact: true }),
    ).toBeVisible();
    const members = sheet.getByRole("link", { name: "Anggota tim" });
    await expectTouchTarget(members);
    const logout = sheet.getByRole("button", { name: "Keluar" });
    await expectTouchTarget(logout);
    // Links and buttons line up: every row's icon starts at the same x.
    const iconX = async (row: typeof members) =>
      (await row.locator("svg").first().boundingBox())?.x ?? -1;
    await animationsSettled(sheet);
    expect(await iconX(logout)).toBeCloseTo(await iconX(members), 0);
    // The sheet comes up from the bottom, and the button looks current while it is open.
    await expect(lainnyaTab(page)).toHaveCSS("font-weight", "700");
    await animationsSettled(sheet);
    const sheetBox = await sheet.boundingBox();
    const viewport = page.viewportSize();
    expect(
      Math.abs(
        (sheetBox?.y ?? 0) + (sheetBox?.height ?? 0) - (viewport?.height ?? 0),
      ),
    ).toBeLessThanOrEqual(1);
    await attachViewportScreenshot(page, testInfo, "phone-more-sheet");

    // Escape closes it and focus goes back to "Lainnya".
    await page.keyboard.press("Escape");
    await expect(sheet).toBeHidden();
    await expect(lainnyaButton(page)).toBeFocused();
    await expect(lainnyaButton(page)).toHaveCSS("font-weight", "600");

    // Following a link closes it and lands on the page.
    const again = await openAccountMenu(page, testInfo);
    await again.getByRole("link", { name: "Anggota tim" }).click();
    await expect(page).toHaveURL(`${home}/members`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Anggota tim" }),
    ).toBeVisible();
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });

  test("keeps the side menu on iPad and desktop, with no tab bar", async ({
    page,
  }, testInfo) => {
    test.skip(isPhone(testInfo), "the phone has the tab bar");
    await openOrganization(page, HENDRA, "Griya Sejahtera");
    await expect(page.locator("#org-sidebar")).toBeVisible();
    const menu = await openMenu(page, testInfo);
    await expect(menuSections(menu)).toHaveText(SIDE_MENU_ADMIN);
    // The phone's tab bar is hidden, so it is not in the accessibility tree either: one menu.
    await expect(
      page.getByRole("navigation", { name: "Menu utama" }),
    ).toHaveCount(1);
    await expect(lainnyaButton(page)).toHaveCount(0);
    await expect(page.locator("nav.fixed.bottom-0")).toBeHidden();
  });
});

test.describe("Admin of a long list", () => {
  test.use({ storageState: DIMAS.storageState });

  test("ends the page above the phone's tab bar", async ({
    page,
  }, testInfo) => {
    test.skip(!isPhone(testInfo), "the tab bar is the phone's");
    await openOrganization(page, DIMAS, "Kos Bulanan Sejahtera");
    const bar = await openMenu(page, testInfo);
    await bar.getByRole("link", { name: "Penyewa" }).click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Penyewa" }),
    ).toBeVisible();
    const rows = page
      .getByRole("list", { name: "Daftar penyewa" })
      .filter({ visible: true })
      .getByRole("listitem");
    await expect(rows.first()).toBeVisible();
    expect(await rows.count()).toBeGreaterThan(5);

    await page.evaluate(() =>
      window.scrollTo(0, document.documentElement.scrollHeight),
    );
    const pageBar = page
      .getByRole("navigation", { name: "Halaman" })
      .filter({ visible: true });
    const barTop = (await bar.boundingBox())?.y ?? 0;
    const lastRow = await rows.last().boundingBox();
    expect((lastRow?.y ?? 0) + (lastRow?.height ?? 0)).toBeLessThan(barTop);
    // The page bar under the list is the last thing on the page.
    await expect(pageBar).toBeInViewport({ ratio: 1 });
    const pager = await pageBar.boundingBox();
    expect((pager?.y ?? 0) + (pager?.height ?? 0)).toBeLessThan(barTop);
  });
});

test.describe("Karyawan", () => {
  test.use({ storageState: RUDI.storageState });

  test("sees only the sections his capabilities allow", async ({
    page,
  }, testInfo) => {
    await openOrganization(page, RUDI, "Kost Putri Anggrek");
    const menu = await openMenu(page, testInfo);
    if (isPhone(testInfo)) {
      // Three tabs and "Lainnya": no Properti tab, and no Anggota tim in the sheet.
      await expectTabBar(page, testInfo, ["Beranda", "Penyewa", "Penagihan"]);
      const sheet = await openAccountMenu(page, testInfo);
      await expect(sheet.getByText(RUDI.name)).toBeVisible();
      await expect(
        sheet.getByText("Karyawan di Kost Putri Anggrek", { exact: true }),
      ).toBeVisible();
      await expect(
        sheet.getByRole("link", { name: "Anggota tim" }),
      ).toHaveCount(0);
      // One organization: no "Ganti usaha".
      await expect(
        sheet.getByRole("link", { name: "Ganti usaha" }),
      ).toHaveCount(0);
      await page.keyboard.press("Escape");
    } else {
      await expect(menu.getByText("Karyawan", { exact: true })).toBeVisible();
    }
    await expect(menuSections(menu)).toHaveText([
      "Beranda",
      "Penyewa",
      "Penagihan",
    ]);
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
    // She may not read members: no Anggota tim anywhere, in the side menu or in "Lainnya".
    await expect(sections.filter({ hasText: "Anggota tim" })).toHaveCount(0);
    if (isPhone(testInfo)) {
      const sheet = await openAccountMenu(page, testInfo);
      await expect(
        sheet.getByRole("link", { name: "Anggota tim" }),
      ).toHaveCount(0);
      await expect(sheet.getByRole("button", { name: "Keluar" })).toBeVisible();
    }
  });
});
