import { expect, test, type Page } from "@playwright/test";

import { openPropertiesPage } from "./support/drawer-motion";
import { animationsSettled } from "./support/page-checks";
import { isPhone, ORG_HOME_URL } from "./support/shell";
import { HENDRA } from "./support/staff";

// The tenant and property panels on the form sheet (board HP-Lembar), on the phone: a form left
// as it opened closes at once, one with something typed asks "Tutup tanpa menyimpan?" first.
// Against the real API; nothing is sent, every sheet is closed without saving.

const GRIYA = "Griya Sejahtera";
const QUESTION = "Tutup tanpa menyimpan?";

test.use({ storageState: HENDRA.storageState });

async function openTenantsPage(page: Page) {
  await page.goto("/organizations");
  await page
    .getByRole("list", { name: "Daftar usaha" })
    .getByRole("link", { name: GRIYA })
    .click();
  await expect(page).toHaveURL(ORG_HOME_URL);
  await page.goto(`${page.url()}/tenants`);
  await expect(
    page.getByRole("heading", { level: 1, name: "Penyewa" }),
  ).toBeVisible();
}

const PANELS = [
  {
    opener: "Tambah properti",
    field: "Nama properti",
    typed: "Kos Uji Kenanga",
    open: (page: Page) => openPropertiesPage(page, HENDRA.organizations, GRIYA),
  },
  {
    opener: "Tambah penyewa",
    field: "Nama penyewa",
    typed: "Bu Wulan Kusumawati",
    open: openTenantsPage,
  },
] as const;

for (const panel of PANELS) {
  test(`${panel.opener}: closes at once untouched, asks first once something is typed`, async ({
    page,
  }, testInfo) => {
    test.skip(!isPhone(testInfo), "The question is asked on the phone only.");
    await panel.open(page);

    const openSheet = async () => {
      await page.getByRole("button", { name: panel.opener }).click();
      const sheet = page.getByRole("dialog", { name: panel.opener });
      await expect(sheet).toBeVisible();
      await animationsSettled(page);
      return sheet;
    };
    const question = page.getByRole("alertdialog", { name: QUESTION });

    // Untouched: Batal closes without a question.
    let sheet = await openSheet();
    await sheet.getByRole("button", { name: "Batal" }).click();
    await expect(sheet).toBeHidden();
    await expect(question).toBeHidden();

    // Typed: Escape asks; "Lanjut mengisi" keeps the sheet and what was typed.
    sheet = await openSheet();
    const field = sheet.getByLabel(panel.field, { exact: true });
    await field.fill(panel.typed);
    await page.keyboard.press("Escape");
    await expect(question).toBeVisible();
    await question.getByRole("button", { name: "Lanjut mengisi" }).click();
    await expect(question).toBeHidden();
    await expect(sheet).toBeVisible();
    await expect(field).toHaveValue(panel.typed);

    // "Ya, tutup" closes it, and the next opening starts empty.
    await sheet.getByRole("button", { name: "Batal" }).click();
    await question.getByRole("button", { name: "Ya, tutup" }).click();
    await expect(sheet).toBeHidden();
    sheet = await openSheet();
    await expect(sheet.getByLabel(panel.field, { exact: true })).toHaveValue(
      "",
    );
    await sheet.getByRole("button", { name: "Batal" }).click();
    await expect(sheet).toBeHidden();
  });
}
