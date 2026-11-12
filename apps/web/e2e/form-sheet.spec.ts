import { expect, test, type Locator, type Page } from "@playwright/test";

import {
  clearRecordedAnimations,
  recordAnimations,
  recordedAnimations,
} from "./support/drawer-motion";
import { openMembers } from "./support/members";
import {
  animationsSettled,
  attachViewportScreenshot,
} from "./support/page-checks";
import { expectTouchTarget } from "./support/payment-page";
import { isPhone } from "./support/shell";
import { HENDRA } from "./support/staff";

// The create and edit form sheet (board HP-Lembar), on "Tambah anggota" of Anggota tim: a sheet
// from the bottom on the phone, with a grab handle and swipe down to close, that asks before a
// form with unsaved changes closes; the right-hand drawer, unchanged, on iPad and desktop.
// Against the real API; nothing is sent, every sheet is closed without saving.

const GRIYA = "Griya Sejahtera";
const OPENER = "Tambah anggota";
const QUESTION = "Tutup tanpa menyimpan?";
// The board's typed name, on no seeded member; never saved.
const TYPED_NAME = "Mas Rizal Fauzi";

test.use({ storageState: HENDRA.storageState });

function viewportOf(page: Page) {
  const viewport = page.viewportSize();
  if (viewport === null) throw new Error("The projects set a viewport.");
  return viewport;
}

async function boxOf(locator: Locator) {
  const box = await locator.boundingBox();
  if (box === null) throw new Error("The element is not visible.");
  return box;
}

async function openSheet(page: Page) {
  await page.getByRole("button", { name: OPENER }).click();
  const sheet = page.getByRole("dialog", { name: OPENER });
  await expect(sheet).toBeVisible();
  await animationsSettled(page);
  return sheet;
}

// Presses the mouse at `from`, moves it down by `by` pixels, holds still long enough that the
// release is not a flick, then releases. `whileHeld` runs before the release.
async function dragDown(
  page: Page,
  from: { x: number; y: number },
  by: number,
  whileHeld?: () => Promise<void>,
) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x, from.y + by, { steps: 8 });
  // Past the 100 ms over which the release speed is measured.
  await page.waitForTimeout(250);
  await whileHeld?.();
  await page.mouse.up();
}

async function handleCenter(sheet: Locator) {
  const box = await boxOf(sheet.locator('[data-slot="form-sheet-handle"]'));
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

function transformOf(sheet: Locator) {
  return sheet.evaluate((el) => getComputedStyle(el).transform);
}

test("opens as a bottom sheet on the phone and as the right-hand drawer on iPad and desktop", async ({
  page,
}, testInfo) => {
  await openMembers(page, GRIYA);
  const sheet = await openSheet(page);
  const viewport = viewportOf(page);
  const box = await boxOf(sheet);
  const handle = sheet.locator('[data-slot="form-sheet-handle"]');

  if (isPhone(testInfo)) {
    // From the bottom edge up to 8% below the top (64px on a 390×844 phone, as on the board).
    expect(Math.abs(box.y + box.height - viewport.height)).toBeLessThanOrEqual(
      1,
    );
    expect(Math.abs(box.y - viewport.height * 0.08)).toBeLessThanOrEqual(2);
    expect(box.x).toBe(0);
    expect(box.width).toBe(viewport.width);
    expect(
      await sheet.evaluate((el) => getComputedStyle(el).borderTopLeftRadius),
    ).toBe("20px");

    // The grab handle: a 40×5 pill, centered, 8px below the top.
    await expect(handle).toBeVisible();
    const pill = await boxOf(handle.locator("span"));
    expect(pill.width).toBe(40);
    expect(pill.height).toBe(5);
    expect(Math.abs(pill.y - box.y - 8)).toBeLessThanOrEqual(0.5);
    expect(
      Math.abs(pill.x + pill.width / 2 - (box.x + box.width / 2)),
    ).toBeLessThanOrEqual(0.5);
    // A finger on the handle or the header drags the sheet; the browser does not pan instead.
    for (const area of [handle, sheet.locator('[data-slot="sheet-header"]')]) {
      expect(
        await area.evaluate((el) => getComputedStyle(el).touchAction),
      ).toBe("none");
    }
  } else {
    // The drawer as before: on the right, full height, 470px wide, no handle.
    expect(Math.abs(box.x + box.width - viewport.width)).toBeLessThanOrEqual(
      0.5,
    );
    expect(box.width).toBe(470);
    expect(box.y).toBe(0);
    expect(box.height).toBe(viewport.height);
    await expect(handle).toBeHidden();
  }

  // The footer is pinned in view without scrolling, with 44px targets.
  for (const name of ["Batal", "Tambah dan kirim undangan"]) {
    const button = sheet.getByRole("button", { name, exact: true });
    await expect(button).toBeInViewport({ ratio: 1 });
    await expectTouchTarget(button);
  }
  // It slid in from the bottom on the phone, from the right on iPad and desktop.
  const enter = await sheet.evaluate((el) => {
    const style = getComputedStyle(el);
    return {
      x: style.getPropertyValue("--tw-enter-translate-x").trim(),
      y: style.getPropertyValue("--tw-enter-translate-y").trim(),
    };
  });
  expect(enter).toEqual(
    isPhone(testInfo) ? { x: "0", y: "100%" } : { x: "100%", y: "0" },
  );
  await attachViewportScreenshot(page, testInfo, "form-sheet-open");

  await page.keyboard.press("Escape");
  await expect(sheet).toHaveCount(0);
});

test("asks before a form with unsaved changes closes, on the phone only", async ({
  page,
}, testInfo) => {
  await openMembers(page, GRIYA);
  const opener = page.getByRole("button", { name: OPENER });
  const question = page.getByRole("alertdialog", { name: QUESTION });

  // An untouched form closes at once.
  let sheet = await openSheet(page);
  await page.keyboard.press("Escape");
  await expect(sheet).toHaveCount(0);
  await expect(opener).toBeFocused();

  // Every way of closing: Escape, the X and Batal.
  const closers: readonly {
    name: string;
    close: (sheet: Locator) => Promise<void>;
  }[] = [
    { name: "Escape", close: () => page.keyboard.press("Escape") },
    {
      name: "the X",
      close: (sheet) =>
        sheet.getByRole("button", { name: "Tutup", exact: true }).click(),
    },
    {
      name: "Batal",
      close: (sheet) =>
        sheet.getByRole("button", { name: "Batal", exact: true }).click(),
    },
  ];

  if (!isPhone(testInfo)) {
    // iPad and desktop close as before, without asking.
    for (const closer of closers) {
      sheet = await openSheet(page);
      await sheet.getByLabel("Nama").fill(TYPED_NAME);
      await closer.close(sheet);
      await expect(sheet, closer.name).toHaveCount(0);
      await expect(question).toHaveCount(0);
      await expect(opener).toBeFocused();
    }
    return;
  }

  sheet = await openSheet(page);
  const name = sheet.getByLabel("Nama");
  await name.fill(TYPED_NAME);
  for (const closer of closers) {
    await closer.close(sheet);
    await expect(question, closer.name).toBeVisible();
    await expect(question).toContainText(
      "Yang sudah diisi di formulir ini akan hilang.",
    );
    // Focus starts on the safe choice.
    await expect(
      question.getByRole("button", { name: "Lanjut mengisi" }),
    ).toBeFocused();
    if (closer.name === "Escape") {
      await attachViewportScreenshot(page, testInfo, "form-sheet-question");
    }
    await question.getByRole("button", { name: "Lanjut mengisi" }).click();
    await expect(question).toHaveCount(0);
    // Back in the form, with what was typed, and focus inside the sheet.
    await expect(name).toHaveValue(TYPED_NAME);
    expect(
      await sheet.evaluate((el) => el.contains(document.activeElement)),
      `focus is in the sheet after ${closer.name}`,
    ).toBe(true);
  }

  // "Ya, tutup" closes it, and focus goes back to the opener.
  await sheet.getByRole("button", { name: "Batal", exact: true }).click();
  await question.getByRole("button", { name: "Ya, tutup" }).click();
  await expect(question).toHaveCount(0);
  await expect(sheet).toHaveCount(0);
  await expect(opener).toBeFocused();

  // The next sheet starts empty and clean.
  sheet = await openSheet(page);
  await expect(sheet.getByLabel("Nama")).toHaveValue("");
  await page.keyboard.press("Escape");
  await expect(sheet).toHaveCount(0);
});

test.describe("Swipe down on the phone", () => {
  // The phone project is the only mobile one.
  test.skip(
    ({ isMobile }) => !isMobile,
    "Only the phone's sheet has a handle.",
  );

  test("closes past a quarter of the sheet and snaps back short of it", async ({
    page,
  }) => {
    await openMembers(page, GRIYA);
    const opener = page.getByRole("button", { name: OPENER });
    const sheet = await openSheet(page);
    const height = (await boxOf(sheet)).height;

    // 10%: the sheet follows the pointer, then goes back up.
    await dragDown(page, await handleCenter(sheet), height * 0.1, async () => {
      expect(await transformOf(sheet)).not.toBe("none");
    });
    await expect(sheet).toBeVisible();
    await expect.poll(() => transformOf(sheet)).toBe("none");

    // A drag in the form's body never moves the sheet, so scrolling a form never closes it.
    const hint = await boxOf(sheet.getByText("Kami kirim undangan"));
    await dragDown(page, { x: hint.x + 20, y: hint.y + 5 }, height * 0.4);
    await expect(sheet).toBeVisible();
    expect(await transformOf(sheet)).toBe("none");

    // 40%: closed, and focus goes back to the opener.
    await dragDown(page, await handleCenter(sheet), height * 0.4);
    await expect(sheet).toHaveCount(0);
    await expect(opener).toBeFocused();
  });

  test("asks first when the form has unsaved changes", async ({ page }) => {
    await openMembers(page, GRIYA);
    const sheet = await openSheet(page);
    const height = (await boxOf(sheet)).height;
    await sheet.getByLabel("Nama").fill(TYPED_NAME);

    await dragDown(page, await handleCenter(sheet), height * 0.4);
    const question = page.getByRole("alertdialog", { name: QUESTION });
    await expect(question).toBeVisible();
    await question.getByRole("button", { name: "Lanjut mengisi" }).click();
    await expect(question).toHaveCount(0);
    await expect(sheet.getByLabel("Nama")).toHaveValue(TYPED_NAME);
    await expect.poll(() => transformOf(sheet)).toBe("none");

    await dragDown(page, await handleCenter(sheet), height * 0.4);
    await question.getByRole("button", { name: "Ya, tutup" }).click();
    await expect(sheet).toHaveCount(0);
    await expect(page.getByRole("button", { name: OPENER })).toBeFocused();
  });

  test("with reduced motion a swipe closes the sheet without an exit animation", async ({
    page,
  }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await openMembers(page, GRIYA);
    const sheet = await openSheet(page);
    const height = (await boxOf(sheet)).height;
    await recordAnimations(page);
    await clearRecordedAnimations(page);

    await dragDown(page, await handleCenter(sheet), height * 0.4);
    await expect(sheet).toHaveCount(0);
    expect(await recordedAnimations(page)).not.toContainEqual({
      slot: "sheet-content",
      name: "exit",
    });
  });
});
