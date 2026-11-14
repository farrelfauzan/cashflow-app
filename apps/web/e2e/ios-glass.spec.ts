import { expect, test, type Locator, type Page } from "@playwright/test";

import {
  GLASS_BLUR,
  glassLook,
  openIosGate,
  reduceTransparency,
} from "./support/ios-glass";
import {
  animationsSettled,
  attachViewportScreenshot,
} from "./support/page-checks";
import { isPhone, ORG_HOME_URL } from "./support/shell";
import { HENDRA } from "./support/staff";
import { expectToast } from "./support/toast";

// The iOS glass on the floating chrome (T-097e, boards iOS-Aturan, iOS-HP-Tab, iOS-HP-Lembar and
// iOS-iPad): the tab bar, top bar, side menu and toasts (e1), the form sheets, the iPad drawer and
// Lainnya (e2). It
// is gated in CSS on -webkit-touch-callout, which only WebKit on iOS and iPadOS knows: Chromium is
// what Android and desktop see, and with the gate opened in its stylesheets (openIosGate) it is
// what an iPhone or iPad sees. Whether the blur looks right is checked on a real device.

test.use({ storageState: HENDRA.storageState });

async function openGriya(page: Page) {
  await page.goto("/organizations");
  await page
    .getByRole("list", { name: "Daftar usaha" })
    .getByRole("link", { name: "Griya Sejahtera" })
    .click();
  await expect(page).toHaveURL(ORG_HOME_URL);
}

function tabBar(page: Page): Locator {
  return page.locator("nav[data-glass]");
}

function topBar(page: Page): Locator {
  return page.locator("header[data-glass]");
}

function sideMenu(page: Page): Locator {
  return page.locator("#org-sidebar");
}

// The alpha of a computed color ("rgba(…, 0.8)", "color(srgb … / 0.8)"); 1 when it has none.
function alphaOf(color: string): number {
  const match =
    /^rgba\([^)]*,\s*([\d.]+)\)$/.exec(color) ?? /\/\s*([\d.]+)\)$/.exec(color);
  return match === null ? 1 : Number(match[1]);
}

async function box(element: Locator) {
  const found = await element.boundingBox();
  if (found === null) throw new Error("element has no box");
  return found;
}

// The card the form sits on inside a sheet: the theme's paper, never see-through.
const CARD = "rgb(251, 250, 247)";

// Opens "Ubah properti" for Kos Melati (a form sheet: the bottom sheet on the phone, the drawer
// on iPad and desktop), once it has slid in. The page loads again, so the iOS gate is opened
// again when asked.
async function openEditKosMelati(page: Page, ios: boolean): Promise<Locator> {
  await page.goto(`${page.url().replace(/\/properties.*$/, "")}/properties`);
  if (ios) await openIosGate(page);
  await page.getByRole("button", { name: "Ubah Kos Melati" }).first().click();
  const sheet = page.getByRole("dialog", { name: "Ubah properti" });
  await expect(sheet).toBeVisible();
  await animationsSettled(sheet);
  return sheet;
}

// Opens Lainnya on the phone, once it has slid in.
async function openLainnya(page: Page): Promise<Locator> {
  await page.getByRole("button", { name: "Lainnya" }).click();
  const sheet = page.getByRole("dialog", { name: "Lainnya" });
  await expect(sheet).toBeVisible();
  await animationsSettled(sheet);
  return sheet;
}

// How far below a sheet's outer top edge its grab handle starts.
async function handleOffset(sheet: Locator): Promise<number> {
  const handle = sheet.locator("span.h-\\[5px\\]").first();
  return (await box(handle)).y - (await box(sheet)).y;
}

// Saves Kos Melati unchanged, for a real "Properti disimpan" toast. The page loads again, so the
// iOS gate is opened again when asked.
async function saveKosMelati(page: Page, ios: boolean): Promise<Locator> {
  await page.goto(`${page.url().replace(/\/properties.*$/, "")}/properties`);
  if (ios) await openIosGate(page);
  await page.getByRole("button", { name: "Ubah Kos Melati" }).first().click();
  const edit = page.getByRole("dialog", { name: "Ubah properti" });
  await edit.getByRole("button", { name: "Simpan properti" }).click();
  await expect(edit).toHaveCount(0);
  return expectToast(page, "Properti disimpan", "create", "Kos Melati");
}

test("Android and desktop keep the solid chrome along the edges", async ({
  page,
}, testInfo) => {
  await openGriya(page);
  const viewport = page.viewportSize();
  if (viewport === null) throw new Error("no viewport");

  if (isPhone(testInfo)) {
    const bar = await box(tabBar(page));
    expect(bar.x).toBe(0);
    expect(bar.width).toBe(viewport.width);
    expect(bar.y + bar.height).toBe(viewport.height);
    expect(await glassLook(tabBar(page))).toMatchObject({
      backdropFilter: "none",
      borderRadius: "0px",
    });
    expect(alphaOf((await glassLook(tabBar(page))).backgroundColor)).toBe(1);
    expect((await box(topBar(page))).x).toBe(0);
    expect((await glassLook(topBar(page))).backdropFilter).toBe("none");
  } else {
    const menu = await box(sideMenu(page));
    expect(menu.x).toBe(0);
    expect(menu.y).toBe(0);
    expect(await glassLook(sideMenu(page))).toMatchObject({
      backdropFilter: "none",
      borderRadius: "0px",
    });
  }

  // The form sheet: along the bottom on the phone, the drawer along the right from md up.
  const sheet = await openEditKosMelati(page, false);
  const sheetBox = await box(sheet);
  if (isPhone(testInfo)) {
    expect(sheetBox.x).toBe(0);
    expect(sheetBox.y + sheetBox.height).toBe(viewport.height);
  } else {
    expect(sheetBox.x + sheetBox.width).toBe(viewport.width);
    expect(sheetBox.y).toBe(0);
  }
  expect((await glassLook(sheet)).backdropFilter).toBe("none");
  expect(alphaOf((await glassLook(sheet)).backgroundColor)).toBe(1);
  await page.keyboard.press("Escape");
  await expect(sheet).toHaveCount(0);

  if (isPhone(testInfo)) {
    const lainnya = await openLainnya(page);
    expect((await box(lainnya)).x).toBe(0);
    expect((await glassLook(lainnya)).backdropFilter).toBe("none");
    await page.keyboard.press("Escape");
    await expect(lainnya).toHaveCount(0);
  }

  const shown = await saveKosMelati(page, false);
  const look = await glassLook(shown);
  expect(look.backdropFilter).toBe("none");
  expect(alphaOf(look.backgroundColor)).toBe(1);
});

test.describe("on iOS and iPadOS", () => {
  test.beforeEach(({}, testInfo) => {
    test.skip(
      testInfo.project.name === "desktop",
      "macOS Safari has no -webkit-touch-callout; the desktop stays solid (test above)",
    );
  });

  test("the tab bar, top bar, side menu and toasts float as glass", async ({
    page,
  }, testInfo) => {
    await openGriya(page);
    expect(await openIosGate(page)).toBeGreaterThanOrEqual(2);
    const viewport = page.viewportSize();
    if (viewport === null) throw new Error("no viewport");

    if (isPhone(testInfo)) {
      // The tab bar: 12px from the sides, 24px above the bottom, fully round, cream glass.
      const bar = await box(tabBar(page));
      expect(bar.x).toBe(12);
      expect(bar.width).toBe(viewport.width - 24);
      expect(viewport.height - (bar.y + bar.height)).toBe(24);
      const barLook = await glassLook(tabBar(page));
      expect(barLook.backdropFilter).toBe(GLASS_BLUR);
      expect(barLook.borderRadius).toBe("30px");
      expect(alphaOf(barLook.backgroundColor)).toBeCloseTo(0.8);
      // Every tab still a 44px target, 56px tall on the board.
      for (const tab of await tabBar(page).locator("a, button").all()) {
        expect((await box(tab)).height).toBeGreaterThanOrEqual(56);
      }

      // The top bar: 8px from the edges, round, cream glass.
      const header = await box(topBar(page));
      expect(header.x).toBe(8);
      expect(header.y).toBe(8);
      const headerLook = await glassLook(topBar(page));
      expect(headerLook.backdropFilter).toBe(GLASS_BLUR);
      expect(headerLook.borderRadius).toBe("18px");

      // Scrolled to the end, the page's last line clears the floating bar.
      await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
      const main = page.getByRole("main");
      const lastLine = await main.evaluate((el) => {
        const children = Array.from(el.querySelectorAll("*"));
        return Math.max(
          ...children.map((child) => child.getBoundingClientRect().bottom),
        );
      });
      const barAfterScroll = await box(tabBar(page));
      expect(lastLine).toBeLessThanOrEqual(barAfterScroll.y);
    } else {
      // The iPad side menu: a panel 12px from the edges, round, cream glass.
      const menu = await box(sideMenu(page));
      expect(menu.x).toBe(12);
      expect(menu.y).toBe(12);
      expect(menu.height).toBe(viewport.height - 24);
      const menuLook = await glassLook(sideMenu(page));
      expect(menuLook.backdropFilter).toBe(GLASS_BLUR);
      expect(menuLook.borderRadius).toBe("24px");
      expect(alphaOf(menuLook.backgroundColor)).toBeCloseTo(0.8);
    }
    await attachViewportScreenshot(page, testInfo, "ios-glass-chrome");

    // A toast keeps its color by meaning, 90% for green, over the blur.
    const shown = await saveKosMelati(page, true);
    const look = await glassLook(shown);
    expect(look.backdropFilter).toBe(GLASS_BLUR);
    expect(look.borderRadius).toBe("18px");
    expect(alphaOf(look.backgroundColor)).toBeCloseTo(0.9);
    await attachViewportScreenshot(page, testInfo, "ios-glass-toast");
  });

  test("the form sheet, the iPad drawer and Lainnya float as glass, the form on a card", async ({
    page,
  }, testInfo) => {
    await openGriya(page);
    const viewport = page.viewportSize();
    if (viewport === null) throw new Error("no viewport");

    const sheet = await openEditKosMelati(page, true);
    const sheetBox = await box(sheet);
    const sheetLook = await glassLook(sheet);
    expect(sheetLook.backdropFilter).toBe(GLASS_BLUR);
    expect(alphaOf(sheetLook.backgroundColor)).toBeCloseTo(0.8);
    const body = sheet.locator('[data-slot="form-sheet-body"]');
    const bodyLook = await glassLook(body);
    expect(bodyLook.backgroundColor).toBe(CARD);
    const footer = sheet.locator('[data-slot="form-sheet-footer"]');

    if (isPhone(testInfo)) {
      // 8px from the sides and the bottom, round, the form on a 24px card 8px in.
      expect(sheetBox.x).toBe(8);
      expect(sheetBox.width).toBe(viewport.width - 16);
      expect(viewport.height - (sheetBox.y + sheetBox.height)).toBe(8);
      expect(sheetLook.borderRadius).toBe("32px");
      expect(bodyLook.borderRadius).toBe("24px");
      expect((await box(body)).x).toBe(sheetBox.x + 1 + 8);
      expect(await handleOffset(sheet)).toBe(8);
    } else {
      // 12px from the top, right and bottom, round, the form on a 20px card.
      expect(sheetBox.x + sheetBox.width).toBe(viewport.width - 12);
      expect(sheetBox.y).toBe(12);
      expect(sheetBox.height).toBe(viewport.height - 24);
      expect(sheetLook.borderRadius).toBe("28px");
      expect(bodyLook.borderRadius).toBe("20px");
    }
    // The buttons stay pinned in view, inside the floating panel.
    await expect(
      footer.getByRole("button", { name: "Simpan properti" }),
    ).toBeInViewport();
    const footerBox = await box(footer);
    expect(footerBox.y + footerBox.height).toBeLessThanOrEqual(
      sheetBox.y + sheetBox.height,
    );
    await attachViewportScreenshot(page, testInfo, "ios-glass-sheet");

    if (isPhone(testInfo)) {
      // Something typed still asks before the sheet closes (T-097c).
      await sheet.getByLabel("Nama properti").fill("Kos Melati Baru");
      await page.keyboard.press("Escape");
      const question = page.getByRole("alertdialog", {
        name: "Tutup tanpa menyimpan?",
      });
      await expect(question).toBeVisible();
      await question.getByRole("button", { name: "Ya, tutup" }).click();
      await expect(sheet).toHaveCount(0);

      // Lainnya floats the same way, its handle where the form sheet's is.
      const lainnya = await openLainnya(page);
      const lainnyaBox = await box(lainnya);
      expect(lainnyaBox.x).toBe(8);
      expect(viewport.height - (lainnyaBox.y + lainnyaBox.height)).toBe(8);
      const lainnyaLook = await glassLook(lainnya);
      expect(lainnyaLook.backdropFilter).toBe(GLASS_BLUR);
      expect(lainnyaLook.borderRadius).toBe("32px");
      expect(await handleOffset(lainnya)).toBe(8);
      await attachViewportScreenshot(page, testInfo, "ios-glass-lainnya");
    } else {
      // From md up a "sheet" (the billing forms' centred dialog) stays solid; a "drawer" is glass.
      const probe = await page.evaluate(() => {
        const look = (kind: string) => {
          const element = document.createElement("div");
          element.dataset.glass = kind;
          document.body.append(element);
          const filter = getComputedStyle(element).backdropFilter;
          element.remove();
          return filter;
        };
        return { sheet: look("sheet"), drawer: look("drawer") };
      });
      expect(probe).toEqual({ sheet: "none", drawer: GLASS_BLUR });
    }
  });

  test("Increase Contrast and Reduce Transparency turn the glass solid, still floating", async ({
    page,
  }, testInfo) => {
    const chrome = isPhone(testInfo) ? tabBar(page) : sideMenu(page);
    const floatsSolid = async () => {
      expect((await box(chrome)).x).toBe(12);
      const look = await glassLook(chrome);
      expect(look.backdropFilter).toBe("none");
      expect(alphaOf(look.backgroundColor)).toBe(1);
    };

    await page.emulateMedia({ contrast: "more" });
    await openGriya(page);
    await openIosGate(page);
    await floatsSolid();
    const shown = await saveKosMelati(page, true);
    expect((await glassLook(shown)).backdropFilter).toBe("none");
    expect(alphaOf((await glassLook(shown)).backgroundColor)).toBe(1);
    await attachViewportScreenshot(page, testInfo, "ios-solid-contrast");

    // The form sheet too: solid, still floating, the form still on its card, over the usual
    // stronger dim.
    const sheet = await openEditKosMelati(page, true);
    const dim = await page
      .locator('[data-slot="sheet-overlay"]')
      .evaluate((element) => getComputedStyle(element).backgroundColor);
    expect(alphaOf(dim)).toBeCloseTo(0.5);
    const sheetBox = await box(sheet);
    if (isPhone(testInfo)) expect(sheetBox.x).toBe(8);
    else expect(sheetBox.y).toBe(12);
    const sheetLook = await glassLook(sheet);
    expect(sheetLook.backdropFilter).toBe("none");
    expect(alphaOf(sheetLook.backgroundColor)).toBe(1);
    expect(
      (await glassLook(sheet.locator('[data-slot="form-sheet-body"]')))
        .backgroundColor,
    ).toBe(CARD);
    await attachViewportScreenshot(page, testInfo, "ios-solid-contrast-sheet");
    await page.keyboard.press("Escape");

    await page.emulateMedia({ contrast: "no-preference" });
    await reduceTransparency(page);
    await openGriya(page);
    await openIosGate(page);
    await floatsSolid();
  });
});
