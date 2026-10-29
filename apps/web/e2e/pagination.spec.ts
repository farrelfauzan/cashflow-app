import { expect, test, type Locator, type Page } from "@playwright/test";

import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { ORG_HOME_URL } from "./support/shell";
import { DIMAS, LESTARI } from "./support/staff";

// The paginated lists against the real API, on Kos Bulanan Sejahtera (seed-demo's volume
// organization): 27 members, 25 properties, 30 units in Kos Sejahtera Utama and 45 tenants, of
// which 5 are booked ahead and 3 have a collector who left (Gita Savitri, Nanda Ardiansyah, Heri
// Setiawan). The page, page size, search and filters live in the URL, so reload and back keep
// them. Pak Dimas is Admin of the whole organization; Mbak Lestari is Admin limited to
// Kontrakan Seturan. The tests change nothing.

const KOS_UTAMA = "Kos Sejahtera Utama";

// Opens a screen of the organization by its path under /o/<id>.
async function openScreen(page: Page, segment: string, heading: string) {
  await page.goto("/organizations");
  await expect(page).toHaveURL(ORG_HOME_URL);
  await page.goto(`${page.url()}/${segment}`);
  await expect(
    page.getByRole("heading", { level: 1, name: heading }),
  ).toBeVisible();
}

// The page bar of the current screen ("Halaman").
function pageBar(page: Page): Locator {
  return page
    .getByRole("navigation", { name: "Halaman" })
    .filter({ visible: true });
}

// "Menampilkan 21–40 dari 45" in the page bar.
async function expectRange(
  page: Page,
  from: number,
  to: number,
  total: number,
) {
  await expect(pageBar(page)).toContainText(
    `Menampilkan ${from}–${to} dari ${total}`,
  );
}

function step(page: Page, name: "Sebelumnya" | "Berikutnya"): Locator {
  return pageBar(page).getByRole("link", { name });
}

// The rows of a list on the current page: the table on iPad, the cards on the phone.
function rows(page: Page, listName: string): Locator {
  return page
    .getByRole("list", { name: listName })
    .filter({ visible: true })
    .getByRole("listitem");
}

async function chooseOption(page: Page, label: string, option: string) {
  await page
    .getByRole("combobox", { name: label })
    .filter({ visible: true })
    .click();
  await page.getByRole("option", { name: option }).click();
}

function searchField(page: Page): Locator {
  return page
    .getByRole("searchbox", { name: "Cari nama, nomor, atau unit" })
    .filter({ visible: true });
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: DIMAS.storageState });

  test("pages through the tenants, and reload and back keep the page", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await openScreen(page, "tenants", "Penyewa");
    await expectRange(page, 1, 20, 45);
    await expect(rows(page, "Daftar penyewa")).toHaveCount(20);
    await expect(
      pageBar(page).getByRole("button", { name: "Sebelumnya" }),
    ).toBeDisabled();

    await step(page, "Berikutnya").click();
    await expect(page).toHaveURL(/[?&]page=2(&|$)/);
    await expectRange(page, 21, 40, 45);

    await page.reload();
    await expectRange(page, 21, 40, 45);
    await attachFullPageScreenshot(page, testInfo, "tenants-page-2");
    const barShot = testInfo.outputPath(
      `pagination-bar-${testInfo.project.name}.png`,
    );
    await pageBar(page).screenshot({ path: barShot });
    await testInfo.attach("pagination-bar", {
      path: barShot,
      contentType: "image/png",
    });

    await step(page, "Berikutnya").click();
    await expectRange(page, 41, 45, 45);
    await expect(rows(page, "Daftar penyewa")).toHaveCount(5);
    await expect(
      pageBar(page).getByRole("button", { name: "Berikutnya" }),
    ).toBeDisabled();

    await page.goBack();
    await expectRange(page, 21, 40, 45);
    expect(outsideRequests).toEqual([]);
  });

  test("a bigger page size shows every tenant on one page", async ({
    page,
  }) => {
    await openScreen(page, "tenants", "Penyewa");
    await step(page, "Berikutnya").click();
    await expectRange(page, 21, 40, 45);

    await chooseOption(page, "Baris per halaman", "50");
    await expect(page).toHaveURL(/[?&]pageSize=50(&|$)/);
    await expect(page).not.toHaveURL(/[?&]page=/);
    await expectRange(page, 1, 45, 45);
    await expect(rows(page, "Daftar penyewa")).toHaveCount(45);
  });

  test("a page past the end goes to the last page", async ({ page }) => {
    await openScreen(page, "tenants", "Penyewa");
    await page.goto(`${page.url()}?page=9`);
    await expect(page).toHaveURL(/[?&]page=3(&|$)/);
    await expectRange(page, 41, 45, 45);
  });

  test("searches by name and by WhatsApp number on the server, from page 1", async ({
    page,
  }) => {
    await openScreen(page, "tenants", "Penyewa");
    await step(page, "Berikutnya").click();
    await expectRange(page, 21, 40, 45);

    await searchField(page).fill("gita");
    await expect(page).toHaveURL(/[?&]q=gita(&|$)/);
    await expect(page).not.toHaveURL(/[?&]page=/);
    await expect(rows(page, "Daftar penyewa")).toHaveCount(1);
    await expect(rows(page, "Daftar penyewa")).toContainText("Gita Savitri");

    // Nanda Ardiansyah's number, typed in the international form.
    await searchField(page).fill("+62 813-0009-0014");
    await expect(rows(page, "Daftar penyewa")).toHaveCount(1);
    await expect(rows(page, "Daftar penyewa")).toContainText(
      "Nanda Ardiansyah",
    );

    await page.reload();
    await expect(searchField(page)).toHaveValue("+62 813-0009-0014");
    await expect(rows(page, "Daftar penyewa")).toContainText(
      "Nanda Ardiansyah",
    );
  });

  test("filters by status and by property, and resets to every tenant", async ({
    page,
  }) => {
    await openScreen(page, "tenants", "Penyewa");
    await chooseOption(page, "Filter status", "Belum mulai");
    await expect(page).toHaveURL(/[?&]status=upcoming(&|$)/);
    await expectRange(page, 1, 5, 5);
    await expect(rows(page, "Daftar penyewa")).toHaveCount(5);

    // Mulyadi is booked into Rumah 8 of Kontrakan Seturan from June 2027.
    await chooseOption(page, "Filter properti", "Kontrakan Seturan");
    await expect(page).toHaveURL(/[?&]propertyId=/);
    await expect(rows(page, "Daftar penyewa")).toHaveCount(1);
    await expect(rows(page, "Daftar penyewa")).toContainText("Mulyadi");

    await searchField(page).fill("gita");
    await expect(
      page.getByText("Tidak ada penyewa yang cocok").filter({ visible: true }),
    ).toBeVisible();

    await page
      .getByRole("button", { name: "Hapus pencarian" })
      .filter({ visible: true })
      .click();
    await expect(page).not.toHaveURL(/[?&](status|propertyId|q)=/);
    await expect(searchField(page)).toHaveValue("");
    await expectRange(page, 1, 20, 45);
  });

  test("the banner shows the tenants without a collector", async ({ page }) => {
    await openScreen(page, "tenants", "Penyewa");
    await expect(
      page.getByText("3 penyewa belum punya penagih").filter({ visible: true }),
    ).toBeVisible();
    await page
      .getByRole("button", {
        name: "Tampilkan penyewa yang belum punya penagih",
      })
      .click();
    await expect(page).toHaveURL(/[?&]collectorDisabled=true(&|$)/);
    const flagged = rows(page, "Daftar penyewa");
    await expect(flagged).toHaveCount(3);
    for (const name of ["Gita Savitri", "Nanda Ardiansyah", "Heri Setiawan"]) {
      await expect(flagged.filter({ hasText: name })).toHaveCount(1);
    }

    await page
      .getByRole("button", {
        name: "Hanya yang belum punya penagih, hapus penyaring",
      })
      .click();
    await expect(page).not.toHaveURL(/collectorDisabled/);
    await expectRange(page, 1, 20, 45);
  });

  test("pages through the members, the properties and one property's units", async ({
    page,
  }, testInfo) => {
    await openScreen(page, "members", "Anggota tim");
    await expectRange(page, 1, 20, 27);
    await step(page, "Berikutnya").click();
    await expectRange(page, 21, 27, 27);
    await expect(rows(page, "Daftar anggota")).toHaveCount(7);
    await attachFullPageScreenshot(page, testInfo, "members-page-2");

    await openScreen(page, "properties", "Properti");
    await expectRange(page, 1, 20, 25);
    await step(page, "Berikutnya").click();
    await expectRange(page, 21, 25, 25);
    await attachFullPageScreenshot(page, testInfo, "properties-page-2");

    await page.goBack();
    await expectRange(page, 1, 20, 25);
    await page
      .getByRole("list", { name: "Daftar properti" })
      .filter({ visible: true })
      .getByRole("link", { name: `Buka unit ${KOS_UTAMA}` })
      .click();
    await expect(
      page.getByRole("heading", { level: 1, name: KOS_UTAMA }),
    ).toBeVisible();
    await expectRange(page, 1, 20, 30);
    await step(page, "Berikutnya").click();
    await expectRange(page, 21, 30, 30);
    await attachFullPageScreenshot(page, testInfo, "units-page-2");
  });
});

test.describe("Admin limited to one property", () => {
  test.use({ storageState: LESTARI.storageState });

  test("pages and totals count only the tenants she may see", async ({
    page,
  }) => {
    await openScreen(page, "tenants", "Penyewa");
    await expectRange(page, 1, 8, 8);
    await expect(rows(page, "Daftar penyewa")).toHaveCount(8);
    await expect(pageBar(page).getByRole("link")).toHaveCount(0);

    // Gita lives in Kos Sejahtera Utama, outside Mbak Lestari's property.
    await searchField(page).fill("gita");
    await expect(
      page.getByText("Tidak ada penyewa yang cocok").filter({ visible: true }),
    ).toBeVisible();
  });
});
