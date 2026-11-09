import { expect, test, type Locator, type Page } from "@playwright/test";

import {
  animationsSettled,
  attachViewportScreenshot,
} from "./support/page-checks";
import {
  clearRecordedAnimations,
  openPropertiesPage,
  recordAnimations,
  recordedAnimations,
} from "./support/drawer-motion";
import { HENDRA } from "./support/staff";

// The side drawer of the create and edit forms slides in from the right and out again, its
// backdrop fades, and it honors "reduce motion". The header's back link has no underline on
// hover. Against the real API (seed-demo data). Nothing is saved: every drawer is closed
// without sending, so the seeded Kos Melati stays as it is.

const GRIYA = "Griya Sejahtera";

// The slide must be done within this, so a second click or Escape is never held up by an exit
// animation (the exit lasts 200 ms).
const CLOSE_WITHIN_MS = 1_000;

type Opener = {
  label: string;
  dialog: string;
  button: (page: Page) => Locator;
};

// A create form and an edit form, both on the properties page.
const OPENERS: readonly Opener[] = [
  {
    label: "create",
    dialog: "Tambah properti",
    button: (page) => page.getByRole("button", { name: "Tambah properti" }),
  },
  {
    label: "edit",
    dialog: "Ubah properti",
    button: (page) => page.getByRole("button", { name: "Ubah Kos Melati" }),
  },
];

async function expectClosed(dialog: Locator) {
  await expect(dialog).toHaveCount(0, { timeout: CLOSE_WITHIN_MS });
}

test.describe("Side drawer motion", () => {
  test.use({ storageState: HENDRA.storageState });

  for (const opener of OPENERS) {
    test(`${opener.label} form slides in and out, and focus returns to the opener`, async ({
      page,
    }, testInfo) => {
      await openPropertiesPage(page, HENDRA.organizations, GRIYA);
      const button = opener.button(page);
      await expect(button).toBeVisible();
      await recordAnimations(page);

      // Open: the panel slides in ("enter" keyframes) and the backdrop fades in.
      await button.click();
      const dialog = page.getByRole("dialog", { name: opener.dialog });
      await expect(dialog).toBeVisible();
      await expect
        .poll(() => recordedAnimations(page))
        .toEqual(
          expect.arrayContaining([
            { slot: "sheet-content", name: "enter" },
            { slot: "sheet-overlay", name: "enter" },
          ]),
        );
      await animationsSettled(page);
      await attachViewportScreenshot(
        page,
        testInfo,
        `drawer-${opener.label}-open`,
      );
      // Focus is inside the drawer, and Tab never leaves it.
      await expect(dialog.getByLabel("Nama properti")).toBeFocused();
      for (let i = 0; i < 12; i += 1) {
        await page.keyboard.press("Tab");
        const inside = await dialog.evaluate((el) =>
          el.contains(document.activeElement),
        );
        expect(inside, "focus stays in the drawer").toBe(true);
      }

      // Escape closes it with the slide-out, and focus goes back to the opener.
      await clearRecordedAnimations(page);
      await page.keyboard.press("Escape");
      await expect
        .poll(() => recordedAnimations(page))
        .toEqual(
          expect.arrayContaining([{ slot: "sheet-content", name: "exit" }]),
        );
      await expectClosed(dialog);
      await expect(button).toBeFocused();

      // The close button does the same.
      await button.click();
      await expect(dialog).toBeVisible();
      await dialog.getByRole("button", { name: "Tutup" }).click();
      await expectClosed(dialog);
      await expect(button).toBeFocused();

      // The backdrop closes it on iPad. On the phone the drawer covers the whole screen, so
      // there is no backdrop to tap.
      if (testInfo.project.name === "ipad") {
        await button.click();
        await expect(dialog).toBeVisible();
        await page.locator("[data-slot=sheet-overlay]").click({
          position: { x: 20, y: 400 },
        });
        await expectClosed(dialog);
        await expect(button).toBeFocused();
      }
    });
  }

  test("opening the drawer does not move the page behind it", async ({
    page,
  }) => {
    await openPropertiesPage(page, HENDRA.organizations, GRIYA);
    // By tag: while the drawer is open the page behind is hidden from the accessibility tree.
    const heading = page.locator("main h1", { hasText: "Properti" });
    const before = await heading.boundingBox();
    await page.getByRole("button", { name: "Tambah properti" }).click();
    const dialog = page.getByRole("dialog", { name: "Tambah properti" });
    await expect(dialog).toBeVisible();
    // Past the slide, so the scrollbar lock has been applied.
    await expect
      .poll(() =>
        dialog.evaluate((el) => getComputedStyle(el).transform === "none"),
      )
      .toBe(true);
    expect(await heading.boundingBox()).toEqual(before);
    await page.keyboard.press("Escape");
    await expectClosed(dialog);
    expect(await heading.boundingBox()).toEqual(before);
  });

  test("with reduced motion the panel fades without sliding", async ({
    page,
  }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await openPropertiesPage(page, HENDRA.organizations, GRIYA);
    await recordAnimations(page);

    await page.getByRole("button", { name: "Tambah properti" }).click();
    const dialog = page.getByRole("dialog", { name: "Tambah properti" });
    await expect(dialog).toBeVisible();
    // It fades in (board UI-Laci: "kurangi gerakan = memudar saja"), from where it stands.
    await expect
      .poll(() => recordedAnimations(page))
      .toEqual(
        expect.arrayContaining([{ slot: "sheet-content", name: "enter" }]),
      );
    const motion = await dialog.evaluate((el) => {
      const style = getComputedStyle(el);
      return {
        enterX: style.getPropertyValue("--tw-enter-translate-x").trim(),
        exitX: style.getPropertyValue("--tw-exit-translate-x").trim(),
        duration: style.animationDuration,
      };
    });
    expect(motion).toEqual({ enterX: "0", exitX: "0", duration: "0.15s" });

    await page.keyboard.press("Escape");
    await expectClosed(dialog);
  });
});

test.describe("Header back link", () => {
  test.use({ storageState: HENDRA.storageState });

  test("has no underline on hover, and keeps a visible focus ring", async ({
    page,
  }, testInfo) => {
    await openPropertiesPage(page, HENDRA.organizations, GRIYA);
    await page
      .getByRole("link", { name: "Buka unit Kos Melati" })
      .first()
      .click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Kos Melati" }),
    ).toBeVisible();

    const back = page
      .getByRole("main")
      .getByRole("link", { name: "Properti", exact: true });
    await expect(back).toBeVisible();
    await back.hover();
    expect(
      await back.evaluate((el) => getComputedStyle(el).textDecorationLine),
    ).toBe("none");
    // A hover cue that is not an underline: a tinted background.
    // Hover styles apply only where the device can hover (a mouse or trackpad); the touch
    // projects never show the tint.
    if (await page.evaluate(() => matchMedia("(hover: hover)").matches)) {
      await expect
        .poll(() => back.evaluate((el) => getComputedStyle(el).backgroundColor))
        .not.toBe("rgba(0, 0, 0, 0)");
    }

    // Keyboard focus shows a ring.
    await page.mouse.move(0, 0);
    await back.focus();
    await page.keyboard.press("Shift+Tab");
    await page.keyboard.press("Tab");
    await expect(back).toBeFocused();
    await attachViewportScreenshot(page, testInfo, "back-link-focus");
    expect(
      await back.evaluate((el) => getComputedStyle(el).boxShadow),
    ).not.toBe("none");
  });
});
