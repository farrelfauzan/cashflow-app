import { expect, test, type Locator, type Page } from "@playwright/test";

import { attachFullPageScreenshot } from "./support/page-checks";
import { ORG_HOME_URL } from "./support/shell";
import { DIMAS } from "./support/staff";

// "Urutkan" on the four server-paginated lists, against the real API. Kos Bulanan Sejahtera
// (seed-demo's volume organization: 27 members, 25 properties, 30 units in Kos Sejahtera Utama,
// 45 tenants) is only read. "Terbaru/Terlama ditambahkan" are checked against the API's own
// answer for the same sort: which row is newest is the API's rule (its Go tests cover it), and
// adding rows here would spend the organization's hourly invitations and race other specs.

const KOS_UTAMA = "Kos Sejahtera Utama";
function orgIdOf(page: Page): string {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error(`no organization in ${page.url()}`);
  return orgId;
}

// Opens a screen of the organization the member lands in (or picks by name) by its path.
async function openScreen(
  page: Page,
  segment: string,
  heading: string,
  organization?: string,
) {
  await page.goto("/organizations");
  if (organization !== undefined) {
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name: organization })
      .click();
  }
  await expect(page).toHaveURL(ORG_HOME_URL);
  await page.goto(`${page.url()}/${segment}`);
  await expect(
    page.getByRole("heading", { level: 1, name: heading }),
  ).toBeVisible();
}

// The rows of a list on the current page: the table on iPad, the cards on the phone.
function rows(page: Page, listName: string): Locator {
  return page
    .getByRole("list", { name: listName })
    .filter({ visible: true })
    .getByRole("listitem");
}

function sortSelect(page: Page): Locator {
  return page
    .getByRole("combobox", { name: "Urutkan" })
    .filter({ visible: true });
}

async function chooseSort(page: Page, option: string) {
  await sortSelect(page).click();
  await page.getByRole("option", { name: option, exact: true }).click();
}

async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
}

// What one row of the list is called, read through the API in the default order (A–Z).
async function namesFromApi(
  page: Page,
  path: string,
  key: "name" | "label",
): Promise<string[]> {
  const response = await page.request.get(`/api${path}`);
  expect(response.ok(), await response.text()).toBe(true);
  const body = (await response.json()) as {
    items: Record<string, string>[];
  };
  return body.items.map((item) => item[key] ?? "");
}

// Z–A shows the last names first; the URL keeps the choice through a reload; the select is a
// 44px touch target.
async function expectReversedAndKept(
  page: Page,
  listName: string,
  names: string[],
  zToA: string,
  sortValue: string,
) {
  const reversed = [...names].reverse();
  await expect(rows(page, listName).first()).toContainText(names[0] ?? "");

  await chooseSort(page, zToA);
  await expect(page).toHaveURL(
    new RegExp(`[?&]sort=${encodeURIComponent(sortValue)}(&|$)`),
  );
  for (let i = 0; i < 3; i++) {
    await expect(rows(page, listName).nth(i)).toContainText(reversed[i] ?? "");
  }

  await page.reload();
  await expect(sortSelect(page)).toHaveText(zToA);
  for (let i = 0; i < 3; i++) {
    await expect(rows(page, listName).nth(i)).toContainText(reversed[i] ?? "");
  }
  await expectTouchTarget(sortSelect(page));
}

test.describe("Admin of the volume organization", () => {
  test.use({ storageState: DIMAS.storageState });

  test("sorts the tenants Z–A, from page 1, and a reload keeps it", async ({
    page,
  }, testInfo) => {
    await openScreen(page, "tenants", "Penyewa");
    const names = await namesFromApi(
      page,
      `/orgs/${orgIdOf(page)}/tenants?pageSize=100&sort=name`,
      "name",
    );
    expect(names).toHaveLength(45);

    // The page, the search and a filter stay out of the way of the sort: the page goes back to 1
    // and the other params stay.
    await page.goto(`${page.url()}?page=2&status=active`);
    await expect(rows(page, "Daftar penyewa").first()).toBeVisible();
    await chooseSort(page, "Nama Z–A");
    await expect(page).toHaveURL(/[?&]sort=-name(&|$)/);
    await expect(page).toHaveURL(/[?&]status=active(&|$)/);
    await expect(page).not.toHaveURL(/[?&]page=/);

    // Back to the default: it leaves the URL.
    await chooseSort(page, "Nama A–Z");
    await expect(page).not.toHaveURL(/[?&]sort=/);
    await expect(page).toHaveURL(/[?&]status=active(&|$)/);

    await page.goto(page.url().split("?")[0] ?? page.url());
    await expectReversedAndKept(
      page,
      "Daftar penyewa",
      names,
      "Nama Z–A",
      "-name",
    );
    await attachFullPageScreenshot(page, testInfo, "list-sort");
  });

  test("sorts the members Z–A and a reload keeps it", async ({ page }) => {
    await openScreen(page, "members", "Anggota tim");
    const names = await namesFromApi(
      page,
      `/orgs/${orgIdOf(page)}/members?pageSize=100&sort=name`,
      "name",
    );
    expect(names).toHaveLength(27);
    await expectReversedAndKept(
      page,
      "Daftar anggota",
      names,
      "Nama Z–A",
      "-name",
    );
  });

  test("sorts the properties Z–A and a reload keeps it", async ({ page }) => {
    await openScreen(page, "properties", "Properti");
    const names = await namesFromApi(
      page,
      `/orgs/${orgIdOf(page)}/properties?pageSize=100&sort=name`,
      "name",
    );
    expect(names).toHaveLength(25);
    await expectReversedAndKept(
      page,
      "Daftar properti",
      names,
      "Nama Z–A",
      "-name",
    );
  });

  test("sorts a property's units Z–A and a reload keeps it", async ({
    page,
  }) => {
    await openScreen(page, "properties", "Properti");
    await page
      .getByRole("list", { name: "Daftar properti" })
      .filter({ visible: true })
      .getByRole("link", { name: `Buka unit ${KOS_UTAMA}` })
      .click();
    await expect(
      page.getByRole("heading", { level: 1, name: KOS_UTAMA }),
    ).toBeVisible();
    await expect(rows(page, "Daftar unit").first()).toBeVisible();

    const propertyId = /\/properties\/([0-9a-f-]{36})/.exec(page.url())?.[1];
    const names = await namesFromApi(
      page,
      `/orgs/${orgIdOf(page)}/properties/${propertyId}/units?pageSize=100&sort=label`,
      "label",
    );
    expect(names).toHaveLength(30);
    await expectReversedAndKept(
      page,
      "Daftar unit",
      names,
      "Nama unit Z–A",
      "-label",
    );

    // The units have their own options: no "Terbaru" here, and the default is A–Z.
    await sortSelect(page).click();
    await expect(page.getByRole("option")).toHaveText([
      "Nama unit A–Z",
      "Nama unit Z–A",
    ]);
    await page.keyboard.press("Escape");
  });

  for (const list of [
    { segment: "tenants", heading: "Penyewa", name: "Daftar penyewa" },
    { segment: "members", heading: "Anggota tim", name: "Daftar anggota" },
  ]) {
    test(`orders the ${list.segment} by when they were added, as the API does`, async ({
      page,
    }) => {
      await openScreen(page, list.segment, list.heading);
      const path = `/orgs/${orgIdOf(page)}/${list.segment}`;
      for (const [option, sort] of [
        ["Terbaru ditambahkan", "-createdAt"],
        ["Terlama ditambahkan", "createdAt"],
      ] as const) {
        const names = await namesFromApi(page, `${path}?sort=${sort}`, "name");
        await chooseSort(page, option);
        await expect(page).toHaveURL(
          new RegExp(`[?&]sort=${encodeURIComponent(sort)}(&|$)`),
        );
        for (let i = 0; i < 3; i++) {
          await expect(rows(page, list.name).nth(i)).toContainText(
            names[i] ?? "",
          );
        }
      }
      await page.reload();
      await expect(sortSelect(page)).toHaveText("Terlama ditambahkan");
      await expectTouchTarget(sortSelect(page));
    });
  }

  test("an unknown sort in the URL counts as the default", async ({ page }) => {
    await openScreen(page, "tenants", "Penyewa");
    await page.goto(`${page.url()}?sort=email`);
    await expect(rows(page, "Daftar penyewa").first()).toBeVisible();
    await expect(sortSelect(page)).toHaveText("Nama A–Z");
  });
});
