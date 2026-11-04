import { expect, test, type Page } from "@playwright/test";

import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { isPhone, menuSections, ORG_HOME_URL, openMenu } from "./support/shell";
import { HENDRA } from "./support/staff";

// The side menu is found by its id (#org-sidebar, the toggle's aria-controls), since a page may
// have other complementary regions.
//
// The iPad side menu collapses to a rail of icons and expands again; the choice is a plain
// cookie (cf_sidebar), so it survives navigation and reloads and the server draws the right
// width on the first paint. The phone keeps its menu sheet, whatever the cookie says. Each test
// starts from Pak Hendra's saved session, so its choice never reaches another test.

const WIDE_PX = 228;
const RAIL_PX = 76;

async function openGriya(page: Page) {
  await page.goto("/organizations");
  await page
    .getByRole("list", { name: "Daftar usaha" })
    .getByRole("link", { name: "Griya Sejahtera" })
    .click();
  await expect(page).toHaveURL(ORG_HOME_URL);
}

async function asideWidth(page: Page): Promise<number> {
  const box = await page.locator("#org-sidebar").boundingBox();
  return box?.width ?? 0;
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("collapses the side menu to its rail, keeps it across pages and reloads, and expands it", async ({
    page,
  }, testInfo) => {
    test.skip(
      isPhone(testInfo),
      "the side menu is the iPad's; see the phone test",
    );
    const outsideRequests = trackOutsideRequests(page);
    await openGriya(page);
    const home = page.url();
    const aside = page.locator("#org-sidebar");
    const menu = page.getByRole("navigation", { name: "Menu utama" });

    // Wide by default, with the labels.
    const collapse = page.getByRole("button", { name: "Ciutkan menu" });
    await expect(collapse).toHaveAttribute("aria-expanded", "true");
    expect((await collapse.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(
      44,
    );
    expect(await asideWidth(page)).toBe(WIDE_PX);
    await expect(menu.getByText("Penyewa", { exact: true })).toBeVisible();
    await attachViewportScreenshot(page, testInfo, "sidebar-wide");

    // Collapsed: a rail of icons, the links still named and the current page still marked,
    // "Ganti usaha" an icon button, and more room for the page.
    const mainBefore = (await page.getByRole("main").boundingBox())?.width ?? 0;
    await collapse.click();
    const expand = page.getByRole("button", { name: "Lebarkan menu" });
    await expect(expand).toHaveAttribute("aria-expanded", "false");
    await expect.poll(() => asideWidth(page)).toBe(RAIL_PX);
    await expect(aside).toHaveAttribute("data-sidebar", "collapsed");
    const sections = menuSections(menu);
    await expect(sections).toHaveCount(5);
    await expect(
      menu.getByText("Penyewa", { exact: true }),
    ).not.toBeInViewport();
    await expect(sections.filter({ hasText: "Beranda" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    for (const link of await sections.all()) {
      const box = await link.boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
      expect(box?.width ?? 0).toBeGreaterThanOrEqual(44);
    }
    const switcher = aside.getByRole("combobox", { name: "Ganti usaha" });
    await expect(switcher).toBeVisible();
    expect((await switcher.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(
      44,
    );
    await expect(aside.getByRole("button", { name: "Keluar" })).toBeVisible();
    expect(
      (await page.getByRole("main").boundingBox())?.width ?? 0,
    ).toBeGreaterThan(mainBefore);
    await attachViewportScreenshot(page, testInfo, "sidebar-collapsed");

    // The rail's switcher still opens the organizations.
    await switcher.click();
    await expect(
      page.getByRole("option", { name: "Kost Putri Anggrek" }),
    ).toBeVisible();
    await attachViewportScreenshot(page, testInfo, "sidebar-collapsed-switch");
    await page.keyboard.press("Escape");

    // A link from the rail opens its page and the rail stays.
    await sections.filter({ hasText: "Penyewa" }).click();
    await expect(page).toHaveURL(`${home}/tenants`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Penyewa" }),
    ).toBeVisible();
    await expect(sections.filter({ hasText: "Penyewa" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    await expect(expand).toBeVisible();
    expect(await asideWidth(page)).toBe(RAIL_PX);
    await attachViewportScreenshot(page, testInfo, "sidebar-collapsed-tenants");

    // The server draws it collapsed: the page's own HTML has the rail, so a reload never
    // starts wide.
    const html = await (await page.request.get(page.url())).text();
    expect(html).toContain('data-sidebar="collapsed"');
    await page.reload();
    await expect(
      page.getByRole("heading", { level: 1, name: "Penyewa" }),
    ).toBeVisible();
    await expect(expand).toBeVisible();
    expect(await asideWidth(page)).toBe(RAIL_PX);

    // Expanded again, and that is remembered too.
    await expand.click();
    await expect(collapse).toHaveAttribute("aria-expanded", "true");
    await expect.poll(() => asideWidth(page)).toBe(WIDE_PX);
    await expect(menu.getByText("Penyewa", { exact: true })).toBeVisible();
    await page.reload();
    await expect(collapse).toBeVisible();
    expect(await asideWidth(page)).toBe(WIDE_PX);
    const cookies = await page.context().cookies();
    expect(
      cookies.find((cookie) => cookie.name === "cf_sidebar"),
    ).toBeUndefined();
    expect(outsideRequests).toEqual([]);
  });

  test("keeps the phone's menu sheet as it was, even with the rail chosen on iPad", async ({
    page,
  }, testInfo) => {
    test.skip(!isPhone(testInfo), "the phone's menu sheet");
    await page.context().addCookies([
      {
        name: "cf_sidebar",
        value: "collapsed",
        url: testInfo.project.use.baseURL ?? "http://localhost:3000",
      },
    ]);
    await openGriya(page);
    await expect(page.locator("#org-sidebar")).toBeHidden();
    await expect(
      page.getByRole("button", { name: "Ciutkan menu" }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Lebarkan menu" }),
    ).toHaveCount(0);

    const menu = await openMenu(page, testInfo);
    await expect(menuSections(menu)).toHaveText([
      "Beranda",
      "Properti",
      "Penyewa",
      "Penagihan",
      "Anggota tim",
    ]);
    await expect(menu.getByText("Penyewa", { exact: true })).toBeVisible();
    await expect(menu.getByText("Ganti usaha", { exact: true })).toBeVisible();
    await expect(menu.getByRole("button", { name: "Keluar" })).toBeVisible();
    await attachViewportScreenshot(page, testInfo, "sidebar-phone-menu");
  });
});
