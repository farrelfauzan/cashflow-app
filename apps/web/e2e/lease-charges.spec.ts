import {
  expect,
  test,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import type {
  ChargeType,
  Lease,
  Tenant,
  Unit,
} from "../src/lib/api/generated/model";
import { jakartaToday } from "../src/lib/format/date";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { findOrCreateProperty } from "./support/property-fixtures";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA, SARI, type SeededStaff } from "./support/staff";

// "Biaya tambahan" on a lease's card: the lease's fixed monthly extras, and "Jenis biaya baru",
// against the real API (seed-demo data). Pak Hendra is Admin of the whole of Griya Sejahtera and
// Supervisor in Kost Putri Anggrek; Mbak Sari is Admin limited to Kos Melati. Charge types belong
// to the whole organization, so each test creates the types it needs with names no other run,
// project, test or retry uses, finds its rows by those names, and never asserts the whole list.
// Each test changes only its own unit, tenant and lease, in a property reused across runs
// (support/property-fixtures.ts). Property names start with "Wisma" so they sort after every
// seeded property and never push one off the first page of the property list that
// properties.spec.ts reads.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const PANEL = "Biaya tambahan";
const SAVE = "Simpan biaya tambahan";
const ADD_TYPE = "Tambah jenis biaya baru";
const CREATE_TYPE = "Tambah jenis biaya";
const INTRO =
  "Biaya yang jumlahnya sama setiap bulan. Ditambahkan ke tagihan di atas sewa, mulai tagihan berikutnya. Hanya angka.";
const METERED_NOTE =
  "Listrik dan air yang dicatat dari meteran tidak diatur di sini. Jumlahnya dicatat tiap bulan.";
const BAD_AMOUNT = "Isi jumlahnya dengan angka, paling sedikit Rp1.";
const TAKEN_NAME = "Nama biaya ini sudah dipakai.";

// Minimum body text per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };

function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

// A name no other run, project, test or retry uses.
function uniqueName(testInfo: TestInfo, base: string): string {
  const project = testInfo.project.name === "phone" ? "Dua" : "Satu";
  const retry = testInfo.retry === 0 ? "" : ` r${testInfo.retry}`;
  return `${base} ${runId()} ${project}${retry}`;
}

// A tenant number no other run, project, test or retry uses; 0813-6 keeps it apart from the
// numbers of lease-changes.spec.ts (0813-7) and tenant-forms.spec.ts (0813-9).
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = testInfo.project.name === "phone" ? "2" : "1";
  return `0813-6${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
}

function orgIdOf(page: Page): string {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

// Opens the organization's tenants page: straight in for one organization, by name for several.
async function openTenants(page: Page, staff: SeededStaff, name: string) {
  await page.goto("/organizations");
  if (staff.organizations.length > 1) {
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name })
      .click();
  }
  await expect(page).toHaveURL(ORG_HOME_URL);
  await page.goto(`${page.url()}/tenants`);
  await expect(
    page.getByRole("heading", { level: 1, name: "Penyewa" }),
  ).toBeVisible();
  const list = page.getByRole("list", { name: "Daftar penyewa" });
  await expect(list.getByRole("listitem").first()).toBeVisible();
  return list;
}

// Text on the current page only. The app keeps a page it navigated away from in the DOM, hidden.
function visibleText(page: Page, text: string | RegExp): Locator {
  return page.getByText(text).filter({ visible: true });
}

async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

// Setup through the API, as the signed-in member (the page's cookies).
async function call<T>(
  page: Page,
  path: string,
  want: number,
  data: object,
): Promise<T> {
  const response = await page.request.fetch(`/api${path}`, {
    method: "POST",
    data,
  });
  expect(response.status(), await response.text()).toBe(want);
  return (await response.json()) as T;
}

async function createChargeType(
  page: Page,
  orgId: string,
  name: string,
): Promise<ChargeType> {
  return call<ChargeType>(page, `/orgs/${orgId}/charge-types`, 201, {
    name,
    metered: false,
  });
}

// Opens Pak Hendra's Griya Sejahtera and, on its tenant's page, a lease of the test's own that
// started today: a reused property ("Wisma Tetap ...", found or made once), and a unit, tenant and
// lease new this run (the unit's name is unique per run, as the property keeps every run's units).
async function openOwnLease(
  page: Page,
  testInfo: TestInfo,
  names: { property: string; tenant: string; testDigit: number },
) {
  await openTenants(page, HENDRA, GRIYA);
  const orgId = orgIdOf(page);
  const property = await findOrCreateProperty(page, orgId, testInfo, {
    base: names.property,
    kind: "kos",
  });
  const unit = await call<Unit>(
    page,
    `/orgs/${orgId}/properties/${property.id}/units`,
    201,
    {
      label: uniqueName(testInfo, "Kamar 1"),
      terms: [{ months: 1, price: 1_250_000 }],
    },
  );
  const tenant = await call<Tenant>(page, `/orgs/${orgId}/tenants`, 201, {
    name: uniqueName(testInfo, names.tenant),
    phone: tenantNumber(testInfo, names.testDigit),
    note: "",
  });
  await call<Lease>(page, `/orgs/${orgId}/leases`, 201, {
    tenantId: tenant.id,
    unitId: unit.id,
    startDate: jakartaToday(new Date()),
    termMonths: 1,
    depositAmount: 2_500_000,
  });
  return { orgId, property, tenant, unit };
}

async function openTenantPage(page: Page, orgId: string, tenant: Tenant) {
  await page.goto(`/o/${orgId}/tenants/${tenant.id}`);
  await expect(
    page.getByRole("heading", { level: 1, name: tenant.name }),
  ).toBeVisible();
}

function chargeBox(panel: Locator, name: string): Locator {
  return panel.getByRole("checkbox", { name, exact: true });
}

function chargeAmount(panel: Locator, name: string): Locator {
  return panel.getByRole("textbox", { name: `Jumlah ${name}`, exact: true });
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sets two charges, then removes one", async ({ page }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const { orgId, property, tenant, unit } = await openOwnLease(
      page,
      testInfo,
      {
        property: "Biaya",
        tenant: "Wulan Anggraini",
        testDigit: 1,
      },
    );
    const parkir = uniqueName(testInfo, "Parkir Uji");
    const sampah = uniqueName(testInfo, "Sampah Uji");
    await createChargeType(page, orgId, parkir);
    await createChargeType(page, orgId, sampah);
    await openTenantPage(page, orgId, tenant);
    const card = page.getByRole("region", {
      name: `${unit.label} · ${property.name}`,
    });
    await expect(card.getByText("Biaya tambahan tetap")).toHaveCount(0);

    const open = card.getByRole("button", { name: PANEL, exact: true });
    await expectTouchTarget(open);
    await open.click();
    const panel = page.getByRole("dialog", { name: PANEL });
    await expect(panel).toContainText(
      `${tenant.name} · ${unit.label} · ${property.name}`,
    );
    await expect(panel).toContainText(INTRO);
    await expect(panel).toContainText(METERED_NOTE);
    await expect(chargeBox(panel, parkir)).not.toBeChecked();
    await expect(chargeBox(panel, sampah)).not.toBeChecked();

    // Typing an amount checks its row; the field groups the digits.
    await chargeAmount(panel, parkir).fill("15000");
    await expect(chargeAmount(panel, parkir)).toHaveValue("15.000");
    await expect(chargeBox(panel, parkir)).toBeChecked();
    await chargeBox(panel, sampah).click();
    await chargeAmount(panel, sampah).fill("20000");

    const min = MIN_BODY_FONT_PX[testInfo.project.name] ?? 16;
    expect(await fontSize(panel.getByText(INTRO))).toBeGreaterThanOrEqual(min);
    expect(
      await fontSize(panel.getByText(parkir, { exact: true })),
    ).toBeGreaterThanOrEqual(min);
    await expectTouchTarget(panel.locator("label").filter({ hasText: parkir }));
    await expectTouchTarget(chargeAmount(panel, parkir));
    await expectTouchTarget(panel.getByRole("button", { name: SAVE }));
    await expectTouchTarget(
      panel.getByRole("button", { name: ADD_TYPE, exact: true }),
    );
    await attachViewportScreenshot(page, testInfo, "lease-charges");

    await panel.getByRole("button", { name: SAVE }).click();
    await expect(panel).toHaveCount(0);
    await expect(
      visibleText(page, `Biaya tambahan ${unit.label} disimpan.`),
    ).toBeVisible();
    await expect(card.getByText("Biaya tambahan tetap")).toBeVisible();
    await expect(card).toContainText(`${parkir} Rp15.000`);
    await expect(card).toContainText(`${sampah} Rp20.000`);

    // Opened again: both prefilled. Unchecking keeps the text but leaves the row out.
    await open.click();
    await expect(panel).toBeVisible();
    await expect(chargeBox(panel, parkir)).toBeChecked();
    await expect(chargeAmount(panel, parkir)).toHaveValue("15.000");
    await expect(chargeBox(panel, sampah)).toBeChecked();
    await expect(chargeAmount(panel, sampah)).toHaveValue("20.000");
    await chargeBox(panel, sampah).click();
    await expect(chargeBox(panel, sampah)).not.toBeChecked();
    await expect(chargeAmount(panel, sampah)).toHaveValue("20.000");
    await panel.getByRole("button", { name: SAVE }).click();
    await expect(panel).toHaveCount(0);
    await expect(card).toContainText(`${parkir} Rp15.000`);
    await expect(card).not.toContainText(sampah);

    expect(outsideRequests).toEqual([]);
  });

  test("adds a new charge type from the panel", async ({ page }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const { orgId, property, tenant, unit } = await openOwnLease(
      page,
      testInfo,
      {
        property: "Jenis",
        tenant: "Wiwik Handayani",
        testDigit: 2,
      },
    );
    const taken = uniqueName(testInfo, "Keamanan Uji");
    await createChargeType(page, orgId, taken);
    await openTenantPage(page, orgId, tenant);
    const card = page.getByRole("region", {
      name: `${unit.label} · ${property.name}`,
    });
    await card.getByRole("button", { name: PANEL, exact: true }).click();
    const panel = page.getByRole("dialog", { name: PANEL });
    await expect(chargeBox(panel, taken)).toBeVisible();

    await panel.getByRole("button", { name: ADD_TYPE, exact: true }).click();
    const box = panel.getByRole("group", { name: "Jenis biaya baru" });
    await expect(box).toContainText(
      "Jenis biaya baru dipakai untuk semua properti.",
    );
    await expect(
      panel.getByRole("button", { name: ADD_TYPE, exact: true }),
    ).toHaveCount(0);
    const name = box.getByLabel("Nama biaya");
    await expect(name).toBeFocused();
    await expect(name).toHaveAttribute("placeholder", "Contoh: Air galon");

    // An empty name is refused before anything is sent.
    const create = box.getByRole("button", { name: CREATE_TYPE, exact: true });
    await expectTouchTarget(create);
    await create.click();
    await expect(box.getByText("Isi nama biaya.")).toBeVisible();
    await expect(name).toBeFocused();

    // A name another type has: the API's message under "Nama biaya".
    await name.fill(taken.toUpperCase());
    await create.click();
    await expect(box.getByText(TAKEN_NAME)).toBeVisible();
    await expect(name).toHaveAttribute("aria-invalid", "true");
    await expect(name).toBeFocused();
    await expect(panel).toBeVisible();

    // Enter in the name adds the type and does not save the panel.
    const galon = uniqueName(testInfo, "Air Galon Uji");
    await name.fill(galon);
    await attachViewportScreenshot(page, testInfo, "lease-charge-type");
    await name.press("Enter");
    await expect(box).toHaveCount(0);
    await expect(panel).toBeVisible();
    await expect(chargeBox(panel, galon)).toBeChecked();
    await expect(chargeAmount(panel, galon)).toBeFocused();
    await page.keyboard.type("12000");
    await expect(chargeAmount(panel, galon)).toHaveValue("12.000");

    // A metered type gets no row, only a short line.
    const meter = uniqueName(testInfo, "Gas Uji");
    await panel.getByRole("button", { name: ADD_TYPE, exact: true }).click();
    await box.getByLabel("Nama biaya").fill(meter);
    await box
      .getByRole("checkbox", {
        name: "Jumlahnya berbeda tiap bulan (dicatat dari meteran)",
      })
      .click();
    await box.getByRole("button", { name: CREATE_TYPE, exact: true }).click();
    await expect(box).toHaveCount(0);
    await expect(panel).toContainText(
      `Jenis biaya ${meter} ditambahkan. Biaya dari meteran dicatat tiap bulan, jadi tidak diatur di sini.`,
    );
    await expect(chargeBox(panel, meter)).toHaveCount(0);

    await panel.getByRole("button", { name: SAVE }).click();
    await expect(panel).toHaveCount(0);
    await expect(card).toContainText(`${galon} Rp12.000`);

    expect(outsideRequests).toEqual([]);
  });

  test("refuses a checked row without an amount", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const { orgId, property, tenant, unit } = await openOwnLease(
      page,
      testInfo,
      {
        property: "Jumlah",
        tenant: "Wiryo Kusumo",
        testDigit: 3,
      },
    );
    const kebersihan = uniqueName(testInfo, "Kebersihan Uji");
    await createChargeType(page, orgId, kebersihan);
    await openTenantPage(page, orgId, tenant);
    const card = page.getByRole("region", {
      name: `${unit.label} · ${property.name}`,
    });
    await card.getByRole("button", { name: PANEL, exact: true }).click();
    const panel = page.getByRole("dialog", { name: PANEL });

    await chargeBox(panel, kebersihan).click();
    await panel.getByRole("button", { name: SAVE }).click();
    await expect(panel.getByText(BAD_AMOUNT)).toBeVisible();
    await expect(chargeAmount(panel, kebersihan)).toBeFocused();
    await expect(chargeAmount(panel, kebersihan)).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    await expect(panel).toBeVisible();

    await panel.getByRole("button", { name: "Batal" }).click();
    await expect(panel).toHaveCount(0);
    await expect(card.getByText("Biaya tambahan tetap")).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Admin limited to one property", () => {
  test.use({ storageState: SARI.storageState });

  test("sets charges on her property's tenant but adds no type", async ({
    page,
  }) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openTenants(page, SARI, GRIYA);
    await list
      .getByRole("listitem")
      .filter({ hasText: "Budi Santoso" })
      .getByRole("link")
      .click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Budi Santoso" }),
    ).toBeVisible();
    const card = page.getByRole("region", { name: "Kamar 2 · Kos Melati" });
    const open = card.getByRole("button", { name: PANEL, exact: true });
    await expectTouchTarget(open);
    await open.click();
    const panel = page.getByRole("dialog", { name: PANEL });
    await expect(panel).toContainText(INTRO);
    await expect(panel.getByRole("checkbox").first()).toBeVisible();
    await expect(panel.getByRole("button", { name: ADD_TYPE })).toHaveCount(0);
    await panel.getByRole("button", { name: "Batal" }).click();
    await expect(panel).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees no Biaya tambahan button", async ({ page }) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openTenants(page, HENDRA, ANGGREK);
    await list
      .getByRole("listitem")
      .filter({ hasText: "Nadia Putri" })
      .getByRole("link")
      .click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Nadia Putri" }),
    ).toBeVisible();
    await expect(page.getByRole("region").first()).toBeVisible();
    await expect(page.getByRole("button", { name: PANEL })).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });
});
