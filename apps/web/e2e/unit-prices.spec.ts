import {
  expect,
  test,
  type Locator,
  type Page,
  type Request,
  type TestInfo,
} from "@playwright/test";

import type {
  Property,
  PropertyKind,
  Unit,
  UnitTermInput,
} from "../src/lib/api/generated/model";
import { formatDate, jakartaToday } from "../src/lib/format/date";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA } from "./support/staff";

// A unit's terms and prices in "Tambah unit" / "Ubah unit", "Ubah harga" from a date, and
// "Riwayat harga" (decision 32: prices are an append-only history per unit and term), against the
// real API (seed-demo data). Pak Hendra is Admin of the whole of Griya Sejahtera and Supervisor in
// Kost Putri Anggrek. Each test makes its own property and unit through the API, with names no
// other run, project or retry uses; property names start with "Wisma" so they sort after every
// seeded property.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";

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

// A calendar date `days` days from today in Jakarta.
function daysFromToday(days: number): string {
  const [year = 0, month = 1, day = 1] = jakartaToday(new Date())
    .split("-")
    .map(Number);
  return new Date(Date.UTC(year, month - 1, day + days))
    .toISOString()
    .slice(0, 10);
}

async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

// Opens an organization (Pak Hendra belongs to two) and returns its id.
async function openOrganization(page: Page, name: string): Promise<string> {
  await page.goto("/organizations");
  await page
    .getByRole("list", { name: "Daftar usaha" })
    .getByRole("link", { name })
    .click();
  await expect(page).toHaveURL(ORG_HOME_URL);
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

// Setup through the API, as the signed-in member (the page's cookies).
async function post<T>(page: Page, path: string, data: object): Promise<T> {
  const response = await page.request.post(`/api${path}`, { data });
  expect(response.status(), await response.text()).toBe(201);
  return (await response.json()) as T;
}

// A property of the test's own, with one unit offering the given terms when any are given.
async function createProperty(
  page: Page,
  orgId: string,
  testInfo: TestInfo,
  fixture: { property: string; kind: PropertyKind },
): Promise<Property> {
  return post<Property>(page, `/orgs/${orgId}/properties`, {
    name: uniqueName(testInfo, fixture.property),
    kind: fixture.kind,
    address: "",
  });
}

async function createUnit(
  page: Page,
  orgId: string,
  property: Property,
  label: string,
  terms: UnitTermInput[],
): Promise<Unit> {
  return post<Unit>(page, `/orgs/${orgId}/properties/${property.id}/units`, {
    label,
    terms,
  });
}

// The property's page, its units listed.
async function openPropertyPage(page: Page, orgId: string, property: Property) {
  await page.goto(`/o/${orgId}/properties/${property.id}`);
  await expect(
    page.getByRole("heading", { level: 1, name: property.name }),
  ).toBeVisible();
}

// "Ubah" on a unit, then the panel.
async function openUnitForm(page: Page, label: string): Promise<Locator> {
  await page
    .getByRole("button", { name: `Ubah ${label}`, exact: true })
    .click();
  const panel = page.getByRole("dialog", { name: "Ubah unit" });
  await expect(panel).toBeVisible();
  return panel;
}

// Collects the requests whose path ends with `suffix`, as "METHOD path".
function watchRequests(page: Page, test: (request: Request) => boolean) {
  const seen: string[] = [];
  page.on("request", (request) => {
    if (test(request)) {
      seen.push(`${request.method()} ${new URL(request.url()).pathname}`);
    }
  });
  return seen;
}

function termCheckbox(panel: Locator, name: string): Locator {
  return panel.getByRole("checkbox", { name, exact: true });
}

function priceHistory(panel: Locator): Locator {
  return panel.getByRole("list", { name: "Riwayat harga" });
}

function historyRow(panel: Locator, title: string): Locator {
  return priceHistory(panel).getByRole("listitem").filter({ hasText: title });
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("a new unit in a ruko property starts with only Per tahun checked", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const property = await createProperty(page, orgId, testInfo, {
      property: "Wisma Uji Ruko Baru",
      kind: "ruko",
    });
    await openPropertyPage(page, orgId, property);

    await page.getByRole("button", { name: "Tambah unit" }).click();
    const panel = page.getByRole("dialog", { name: "Tambah unit" });
    await expect(
      panel.getByRole("group", { name: "Masa sewa dan harga" }),
    ).toBeVisible();
    await expect(termCheckbox(panel, "Per bulan")).not.toBeChecked();
    await expect(termCheckbox(panel, "Per 3 bulan")).not.toBeChecked();
    await expect(termCheckbox(panel, "Per 6 bulan")).not.toBeChecked();
    await expect(termCheckbox(panel, "Per tahun")).toBeChecked();
    // Only the checked term asks for a price, and it may stay empty.
    const price = panel.getByLabel("Harga per tahun", { exact: true });
    await expect(price).toBeVisible();
    await expect(
      panel.getByLabel("Harga per bulan", { exact: true }),
    ).toHaveCount(0);
    await expect(
      panel.getByText("Boleh dikosongkan. Tanpa harga", { exact: false }),
    ).toBeVisible();
    // No history for a unit that does not exist yet.
    await expect(panel.getByText("Riwayat harga")).toHaveCount(0);
    await expectTouchTarget(
      panel.locator("label").filter({ hasText: /^Per tahun$/ }),
    );
    expect(
      await fontSize(panel.locator("label").filter({ hasText: /^Per tahun$/ })),
    ).toBeGreaterThanOrEqual(MIN_BODY_FONT_PX[testInfo.project.name] ?? 16);

    await panel.getByLabel("Nama unit").fill("Ruko A1");
    await price.fill("50000000");
    await expect(price).toHaveValue("50.000.000");
    await attachViewportScreenshot(page, testInfo, "unit-form-ruko-add");

    const saved = page.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname.endsWith("/units"),
    );
    await panel.getByRole("button", { name: "Simpan unit" }).click();
    expect((await saved).postDataJSON()).toEqual({
      label: "Ruko A1",
      terms: [{ months: 12, price: 50_000_000 }],
    });
    await expect(panel).toHaveCount(0);
    await expect(
      page
        .getByRole("list", { name: "Daftar unit" })
        .filter({ visible: true })
        .getByRole("listitem")
        .filter({ hasText: "Ruko A1" }),
    ).toContainText("Rp50.000.000 per tahun");

    expect(outsideRequests).toEqual([]);
  });

  test("offers a kos unit by the year with a price, which the history shows in force", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const property = await createProperty(page, orgId, testInfo, {
      property: "Wisma Uji Harga Tahunan",
      kind: "kos",
    });
    await createUnit(page, orgId, property, "Kamar 1", [
      { months: 1, price: 1_250_000 },
    ]);
    await openPropertyPage(page, orgId, property);

    let panel = await openUnitForm(page, "Kamar 1");
    // The monthly price is in force: shown, not typed.
    await expect(termCheckbox(panel, "Per bulan")).toBeChecked();
    await expect(
      panel.getByLabel("Harga per bulan", { exact: true }),
    ).toHaveCount(0);
    await expect(
      panel.getByRole("button", { name: "Ubah harga per bulan" }),
    ).toBeVisible();
    await expect(historyRow(panel, "Per bulan · Rp1.250.000")).toContainText(
      "Berlaku sekarang",
    );

    await termCheckbox(panel, "Per tahun").click();
    await panel.getByLabel("Harga per tahun", { exact: true }).fill("15000000");
    const saved = page.waitForRequest(
      (request) =>
        request.method() === "PUT" &&
        new URL(request.url()).pathname.includes("/units/"),
    );
    await panel.getByRole("button", { name: "Simpan unit" }).click();
    expect((await saved).postDataJSON()).toEqual({
      label: "Kamar 1",
      terms: [{ months: 1 }, { months: 12, price: 15_000_000 }],
    });
    await expect(panel).toHaveCount(0);
    await expect(page.getByText("Kamar 1 sudah disimpan.")).toBeVisible();

    panel = await openUnitForm(page, "Kamar 1");
    await expect(termCheckbox(panel, "Per tahun")).toBeChecked();
    await expect(
      panel.getByRole("button", { name: "Ubah harga per tahun" }),
    ).toBeVisible();
    const yearly = historyRow(panel, "Per tahun · Rp15.000.000");
    await expect(yearly).toContainText("Berlaku sekarang");
    await expect(yearly).toContainText(
      `Mulai ${formatDate(jakartaToday(new Date()))} · dicatat ${HENDRA.name}`,
    );
    // The monthly price is unchanged and still in force.
    await expect(historyRow(panel, "Per bulan · Rp1.250.000")).toContainText(
      "Berlaku sekarang",
    );
    await expect(priceHistory(panel).getByRole("listitem")).toHaveCount(2);
    expect(
      await fontSize(yearly.getByText("Per tahun · Rp15.000.000")),
    ).toBeGreaterThanOrEqual(MIN_BODY_FONT_PX[testInfo.project.name] ?? 16);
    // The history sits under the terms; bring it into the capture.
    await priceHistory(panel).scrollIntoViewIfNeeded();
    await attachViewportScreenshot(page, testInfo, "unit-form-terms");

    expect(outsideRequests).toEqual([]);
  });

  test("Ubah harga from a later date adds a price still to come", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const property = await createProperty(page, orgId, testInfo, {
      property: "Wisma Uji Harga Baru",
      kind: "kos",
    });
    const unit = await createUnit(page, orgId, property, "Kamar 2", [
      { months: 1, price: 1_250_000 },
      { months: 12, price: 15_000_000 },
    ]);
    await openPropertyPage(page, orgId, property);

    const panel = await openUnitForm(page, "Kamar 2");
    await panel.getByRole("button", { name: "Ubah harga per tahun" }).click();
    const pricing = page.getByRole("dialog", { name: "Ubah harga" });
    await expect(pricing).toContainText(`Kamar 2 · ${property.name}`);
    const term = pricing.getByRole("combobox", { name: "Masa sewa" });
    await expect(term).toHaveText("Per tahun");
    await expect(pricing).toContainText("Harga sekarang: Rp15.000.000");
    await expectTouchTarget(term);

    const from = daysFromToday(60);
    await pricing.getByLabel("Harga baru").fill("16000000");
    await pricing.getByLabel("Berlaku mulai").fill(from);
    await attachViewportScreenshot(page, testInfo, "unit-price-change");

    const saved = page.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname.endsWith(`/units/${unit.id}/prices`),
    );
    await pricing.getByRole("button", { name: "Simpan harga" }).click();
    expect((await saved).postDataJSON()).toEqual({
      months: 12,
      amount: 16_000_000,
      effectiveFrom: from,
    });

    const back = page.getByRole("dialog", { name: "Ubah unit" });
    await expect(back.getByRole("status")).toHaveText(
      `Harga per tahun dicatat, berlaku mulai ${formatDate(from)}.`,
    );
    // The term keeps today's price and says what comes next.
    await expect(back).toContainText(`Mulai ${formatDate(from)}: Rp16.000.000`);
    const upcoming = historyRow(back, "Per tahun · Rp16.000.000");
    await expect(upcoming).toContainText("Akan berlaku");
    await expect(historyRow(back, "Per tahun · Rp15.000.000")).toContainText(
      "Berlaku sekarang",
    );
    await upcoming.scrollIntoViewIfNeeded();
    await attachViewportScreenshot(page, testInfo, "unit-price-history-after");

    expect(outsideRequests).toEqual([]);
  });

  test("Ubah harga refuses a date before today before sending, and Kembali keeps the unit's form", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const property = await createProperty(page, orgId, testInfo, {
      property: "Wisma Uji Harga Mundur",
      kind: "kos",
    });
    await createUnit(page, orgId, property, "Kamar 3", [
      { months: 1, price: 1_250_000 },
    ]);
    const priceRequests = watchRequests(
      page,
      (request) =>
        new URL(request.url()).pathname.endsWith("/prices") &&
        request.method() === "POST",
    );
    await openPropertyPage(page, orgId, property);

    const panel = await openUnitForm(page, "Kamar 3");
    await panel.getByLabel("Nama unit").fill("Kamar 3B");
    await panel.getByRole("button", { name: "Ubah harga per bulan" }).click();
    const pricing = page.getByRole("dialog", { name: "Ubah harga" });
    await expect(pricing.getByLabel("Berlaku mulai")).toHaveValue(
      jakartaToday(new Date()),
    );
    await pricing.getByLabel("Harga baru").fill("1300000");
    await pricing.getByLabel("Berlaku mulai").fill(daysFromToday(-1));
    await pricing.getByRole("button", { name: "Simpan harga" }).click();
    await expect(
      pricing.getByText("Tanggal tidak boleh sebelum hari ini."),
    ).toBeVisible();
    await expect(pricing.getByLabel("Berlaku mulai")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    await expect(pricing.getByLabel("Berlaku mulai")).toBeFocused();
    expect(priceRequests).toEqual([]);

    await pricing.getByRole("button", { name: "Kembali" }).click();
    const back = page.getByRole("dialog", { name: "Ubah unit" });
    await expect(back.getByLabel("Nama unit")).toHaveValue("Kamar 3B");
    expect(priceRequests).toEqual([]);

    expect(outsideRequests).toEqual([]);
  });

  test("refuses a unit with no term before sending anything", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const property = await createProperty(page, orgId, testInfo, {
      property: "Wisma Uji Tanpa Masa",
      kind: "kontrakan",
    });
    await createUnit(page, orgId, property, "Rumah 1", [
      { months: 1, price: 2_000_000 },
    ]);
    const unitWrites = watchRequests(
      page,
      (request) =>
        request.method() === "PUT" &&
        new URL(request.url()).pathname.includes("/units/"),
    );
    await openPropertyPage(page, orgId, property);

    const panel = await openUnitForm(page, "Rumah 1");
    await termCheckbox(panel, "Per bulan").click();
    await expect(
      panel.getByText(
        "Sewa yang sudah memakai masa sewa ini tetap berjalan seperti biasa.",
      ),
    ).toBeVisible();
    await panel.getByRole("button", { name: "Simpan unit" }).click();
    await expect(
      panel.getByText("Pilih minimal satu masa sewa."),
    ).toBeVisible();
    await expect(termCheckbox(panel, "Per bulan")).toBeFocused();
    expect(unitWrites).toEqual([]);
    await expect(panel).toBeVisible();

    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees no Ubah on units", async ({ page }) => {
    const outsideRequests = trackOutsideRequests(page);
    await openOrganization(page, ANGGREK);
    await page.goto(`${page.url()}/properties`);
    await page
      .getByRole("link", { name: "Buka unit Kos Putri Anggrek" })
      .click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Kos Putri Anggrek" }),
    ).toBeVisible();
    await expect(
      page
        .getByRole("list", { name: "Daftar unit" })
        .filter({ visible: true })
        .getByRole("listitem")
        .first(),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: /^Ubah/ })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Tambah unit" })).toHaveCount(
      0,
    );

    expect(outsideRequests).toEqual([]);
  });
});
