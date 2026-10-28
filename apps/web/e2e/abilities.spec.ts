import { expect, test, type Page, type TestInfo } from "@playwright/test";

import { trackOutsideRequests } from "./support/page-checks";
import { menuSections, ORG_HOME_URL, openMenu } from "./support/shell";
import { HENDRA, RUDI, SARI, type SeededStaff } from "./support/staff";

// The controls each role sees come from one CASL ability per organization, built from getOrgMe's
// capabilities (src/lib/capabilities.ts) and kept by the shell. These cases move between screens
// inside the app, as people do, so the ability carries over between pages; switching
// organizations must start a new one. The other specs check each screen in depth.

const ADD_MEMBER = "Tambah anggota";
const ADD_PROPERTY = "Tambah properti";
const ADD_UNIT = "Tambah unit";
const ONLY_ASSIGNED_NOTE =
  "Anda hanya melihat properti yang ditugaskan kepada Anda.";

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

// Follows a menu section, as a person would, and waits for its heading.
async function goTo(page: Page, testInfo: TestInfo, section: string) {
  const menu = await openMenu(page, testInfo);
  await menuSections(menu).filter({ hasText: section }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: section }),
  ).toBeVisible();
}

// A button on the current page only: the app keeps a page it left in the DOM, hidden.
function button(page: Page, name: string) {
  return page.getByRole("button", { name }).filter({ visible: true });
}

async function switchOrganization(
  page: Page,
  testInfo: TestInfo,
  name: string,
) {
  const before = page.url();
  const menu = await openMenu(page, testInfo);
  await menu.getByRole("combobox", { name: "Ganti usaha" }).click();
  await page.getByRole("option").filter({ hasText: name }).click();
  await expect(page).not.toHaveURL(before);
  await expect(page).toHaveURL(ORG_HOME_URL);
}

async function openKosMelati(page: Page) {
  await page
    .getByRole("list", { name: "Daftar properti" })
    .getByRole("link", { name: "Buka unit Kos Melati" })
    .click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Kos Melati" }),
  ).toBeVisible();
}

test.describe("Admin, then Supervisor after switching organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees the Admin's controls, none as Supervisor, and the Admin's again after switching back", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);

    // Admin of the whole of Griya Sejahtera.
    await openOrganization(page, HENDRA, "Griya Sejahtera");
    await goTo(page, testInfo, "Anggota tim");
    await expect(button(page, ADD_MEMBER)).toBeVisible();
    await goTo(page, testInfo, "Properti");
    await expect(button(page, ADD_PROPERTY)).toBeVisible();
    await openKosMelati(page);
    await expect(button(page, ADD_UNIT)).toBeVisible();

    // Supervisor in Kost Putri Anggrek: reads everything, changes nothing.
    await switchOrganization(page, testInfo, "Kost Putri Anggrek");
    await goTo(page, testInfo, "Anggota tim");
    await expect(
      page
        .getByRole("list", { name: "Daftar anggota" })
        .filter({ visible: true }),
    ).toBeVisible();
    await expect(button(page, ADD_MEMBER)).toHaveCount(0);
    await goTo(page, testInfo, "Properti");
    await expect(
      page
        .getByRole("list", { name: "Daftar properti" })
        .getByRole("listitem")
        .first(),
    ).toBeVisible();
    await expect(button(page, ADD_PROPERTY)).toHaveCount(0);

    // Back in Griya Sejahtera, the Admin's controls return.
    await switchOrganization(page, testInfo, "Griya Sejahtera");
    await goTo(page, testInfo, "Anggota tim");
    await expect(button(page, ADD_MEMBER)).toBeVisible();
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Admin limited to one property", () => {
  test.use({ storageState: SARI.storageState });

  test("changes her property and its units, but adds no property and sees no members", async ({
    page,
  }, testInfo) => {
    await openOrganization(page, SARI, "Griya Sejahtera");
    const sections = menuSections(await openMenu(page, testInfo));
    await expect(sections.filter({ hasText: "Anggota tim" })).toHaveCount(0);
    await sections.filter({ hasText: "Properti" }).click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Properti" }),
    ).toBeVisible();
    await expect(
      page.getByText(ONLY_ASSIGNED_NOTE).filter({ visible: true }),
    ).toBeVisible();
    await expect(button(page, ADD_PROPERTY)).toHaveCount(0);
    await openKosMelati(page);
    await expect(button(page, "Ubah properti")).toBeVisible();
    await expect(button(page, ADD_UNIT)).toBeVisible();
  });
});

test.describe("Karyawan", () => {
  test.use({ storageState: RUDI.storageState });

  test("gets no add control on the screens his menu leaves out", async ({
    page,
  }) => {
    await openOrganization(page, RUDI, "Kost Putri Anggrek");
    const home = page.url();
    // Typed addresses: the menu does not offer these screens, and the API refuses their lists.
    await page.goto(`${home}/members`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Anggota tim" }),
    ).toBeVisible();
    await expect(button(page, ADD_MEMBER)).toHaveCount(0);
    await page.goto(`${home}/properties`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Properti" }),
    ).toBeVisible();
    await expect(button(page, ADD_PROPERTY)).toHaveCount(0);
  });
});
