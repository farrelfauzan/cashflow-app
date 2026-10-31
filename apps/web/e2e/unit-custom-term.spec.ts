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
  Tenant,
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

// A unit's one custom term (decision 33): "Lainnya" in "Masa sewa dan harga" with "Jumlah bulan"
// and a price, which then behaves like the standard terms in the unit's form, "Riwayat harga",
// "Ubah harga" and the lease's "Masa sewa". Against the real API (seed-demo data). Pak Hendra is
// Admin of the whole of Griya Sejahtera and Supervisor in Kost Putri Anggrek. Each test makes its
// own property, unit and tenant through the API, with names and numbers no other run, project or
// retry uses (numbers 0813-3..., which no other spec uses). Property names start with "Wisma" and
// tenant names with W, so they sort after every seeded property and tenant.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const TERMS = "Masa sewa dan harga";

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

// A tenant number no other spec, run, project, test or retry uses (0813-3...).
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = testInfo.project.name === "phone" ? "2" : "1";
  return `0813-3${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
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

function isUnitWrite(request: Request): boolean {
  return (
    request.method() === "PUT" &&
    new URL(request.url()).pathname.includes("/units/")
  );
}

function termCheckbox(panel: Locator, name: string): Locator {
  return panel.getByRole("checkbox", { name, exact: true });
}

function historyRow(panel: Locator, title: string): Locator {
  return panel
    .getByRole("list", { name: "Riwayat harga" })
    .getByRole("listitem")
    .filter({ hasText: title });
}

async function choose(
  page: Page,
  scope: Locator,
  field: string,
  option: string,
) {
  await scope.getByRole("combobox", { name: field }).click();
  await page.getByRole("option", { name: option, exact: true }).click();
}

// The options a select offers, in order, while it is open; it is closed again afterwards.
async function optionsOf(
  page: Page,
  scope: Locator,
  field: string,
): Promise<string[]> {
  await scope.getByRole("combobox", { name: field }).click();
  const options = page.getByRole("option");
  await expect(options.first()).toBeVisible();
  const texts = (await options.allTextContents()).map((text) => text.trim());
  await page.keyboard.press("Escape");
  await expect(options).toHaveCount(0);
  return texts;
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("adds a 4-month term with its price, which then reads like a standard term", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const property = await createProperty(page, orgId, testInfo, {
      property: "Wisma Uji Lainnya",
      kind: "kos",
    });
    await createUnit(page, orgId, property, "Kamar 1", [
      { months: 1, price: 1_250_000 },
    ]);
    await openPropertyPage(page, orgId, property);

    let panel = await openUnitForm(page, "Kamar 1");
    const lainnya = termCheckbox(panel, "Lainnya");
    await expect(lainnya).not.toBeChecked();
    await expect(panel.getByLabel("Jumlah bulan")).toHaveCount(0);
    const lainnyaLabel = panel
      .locator("label")
      .filter({ hasText: /^Lainnya$/ });
    await expectTouchTarget(lainnyaLabel);
    expect(await fontSize(lainnyaLabel)).toBeGreaterThanOrEqual(
      MIN_BODY_FONT_PX[testInfo.project.name] ?? 16,
    );

    await lainnya.click();
    const months = panel.getByLabel("Jumlah bulan");
    await expect(months).toBeVisible();
    await expect(months).toHaveAttribute("inputmode", "numeric");
    // The range, as a hint the field is described by; the API still judges it.
    await expect(months).toHaveAccessibleDescription(
      "2 sampai 60 bulan, selain 3, 6 dan 12.",
    );
    await expect(
      panel.getByLabel("Harga per periode", { exact: true }),
    ).toBeVisible();
    // Digits only.
    await months.fill("4 bln");
    await expect(months).toHaveValue("4");
    const price = panel.getByLabel("Harga per 4 bulan", { exact: true });
    await expect(price).toBeVisible();
    await price.fill("4800000");
    await expect(price).toHaveValue("4.800.000");
    await expectTouchTarget(months);
    await price.scrollIntoViewIfNeeded();
    await attachViewportScreenshot(page, testInfo, "unit-custom-term-form");

    const saved = page.waitForRequest(isUnitWrite);
    await panel.getByRole("button", { name: "Simpan unit" }).click();
    expect((await saved).postDataJSON()).toEqual({
      label: "Kamar 1",
      terms: [{ months: 1 }, { months: 4, price: 4_800_000 }],
    });
    await expect(panel).toHaveCount(0);
    await expect(page.getByText("Kamar 1 sudah disimpan.")).toBeVisible();

    // Opened again: the row is the unit's 4-month term, priced, with "Ubah harga".
    panel = await openUnitForm(page, "Kamar 1");
    await expect(termCheckbox(panel, "Per 4 bulan")).toBeChecked();
    await expect(termCheckbox(panel, "Lainnya")).toHaveCount(0);
    await expect(panel.getByLabel("Jumlah bulan")).toHaveCount(0);
    await expect(panel.getByText("Rp4.800.000", { exact: true })).toBeVisible();
    await expect(
      panel.getByRole("button", { name: "Ubah harga per 4 bulan" }),
    ).toBeVisible();
    const current = historyRow(panel, "Per 4 bulan · Rp4.800.000");
    await expect(current).toContainText("Berlaku sekarang");
    await expect(current).toContainText(
      `Mulai ${formatDate(jakartaToday(new Date()))} · dicatat ${HENDRA.name}`,
    );
    await current.scrollIntoViewIfNeeded();
    await attachViewportScreenshot(page, testInfo, "unit-custom-term-saved");

    // "Ubah harga" offers it among the unit's terms.
    await panel.getByRole("button", { name: "Ubah harga per 4 bulan" }).click();
    const pricing = page.getByRole("dialog", { name: "Ubah harga" });
    await expect(
      pricing.getByRole("combobox", { name: "Masa sewa" }),
    ).toHaveText("Per 4 bulan");
    await expect(pricing).toContainText("Harga sekarang: Rp4.800.000");
    expect(await optionsOf(page, pricing, "Masa sewa")).toEqual([
      "Per bulan",
      "Per 4 bulan",
    ]);
    await pricing.getByRole("button", { name: "Kembali" }).click();

    // Unticked, the unit stops offering it; ticked again, it is "Lainnya" with its months typed in.
    panel = page.getByRole("dialog", { name: "Ubah unit" });
    await termCheckbox(panel, "Per 4 bulan").click();
    await expect(
      panel.getByText(
        "Sewa yang sudah memakai masa sewa ini tetap berjalan seperti biasa.",
      ),
    ).toBeVisible();
    await termCheckbox(panel, "Per 4 bulan").click();
    await expect(termCheckbox(panel, "Lainnya")).toBeChecked();
    await expect(panel.getByLabel("Jumlah bulan")).toHaveValue("4");
    await expect(
      panel.getByText(
        "Kalau dikosongkan, harga terakhir Rp4.800.000 berlaku lagi.",
      ),
    ).toBeVisible();
    await expect(
      panel.getByText(
        "Sewa yang sudah memakai masa sewa ini tetap berjalan seperti biasa.",
      ),
    ).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });

  test("rents a unit on its 4-month term", async ({ page }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const property = await createProperty(page, orgId, testInfo, {
      property: "Wisma Uji Lainnya Sewa",
      kind: "kos",
    });
    const unit = await createUnit(page, orgId, property, "Kamar 2", [
      { months: 1, price: 1_250_000 },
      { months: 4, price: 4_800_000 },
    ]);
    const tenant = await post<Tenant>(page, `/orgs/${orgId}/tenants`, {
      name: uniqueName(testInfo, "Wulan Empat Bulan"),
      phone: tenantNumber(testInfo, 1),
      note: "",
    });

    await page.goto(`/o/${orgId}/tenants/${tenant.id}`);
    await expect(
      page.getByRole("heading", { level: 1, name: tenant.name }),
    ).toBeVisible();
    await page
      .getByRole("region", { name: "Belum ada sewa" })
      .getByRole("button", { name: "Sewakan unit" })
      .click();
    const lease = page.getByRole("dialog", { name: "Sewakan unit" });
    await choose(page, lease, "Properti", property.name);
    await choose(page, lease, "Unit", unit.label);

    const term = lease.getByRole("combobox", { name: "Masa sewa" });
    expect(await optionsOf(page, lease, "Masa sewa")).toEqual([
      "Per bulan · Rp1.250.000",
      "Per 4 bulan · Rp4.800.000",
    ]);
    await term.click();
    await expect(page.getByRole("option").first()).toBeVisible();
    await attachViewportScreenshot(page, testInfo, "lease-custom-term-picker");
    await page
      .getByRole("option", { name: "Per 4 bulan · Rp4.800.000", exact: true })
      .click();
    await expect(term).toHaveText("Per 4 bulan · Rp4.800.000");

    const start = jakartaToday(new Date());
    await lease.getByLabel("Mulai sewa").fill(start);
    const saved = page.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname.endsWith("/leases"),
    );
    await lease.getByRole("button", { name: "Simpan sewa" }).click();
    expect((await saved).postDataJSON()).toMatchObject({
      tenantId: tenant.id,
      unitId: unit.id,
      startDate: start,
      termMonths: 4,
    });
    await expect(lease).toHaveCount(0);

    const card = page.getByRole("region", {
      name: `${unit.label} · ${property.name}`,
    });
    await expect(card).toContainText("Sewa per 4 bulan");
    await expect(card).toContainText("Rp4.800.000");

    expect(outsideRequests).toEqual([]);
  });

  test("shows the API's refusal of 61 months under Masa sewa dan harga", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const property = await createProperty(page, orgId, testInfo, {
      property: "Wisma Uji Lainnya Panjang",
      kind: "kos",
    });
    await createUnit(page, orgId, property, "Kamar 3", [
      { months: 1, price: 1_250_000 },
    ]);
    await openPropertyPage(page, orgId, property);

    const panel = await openUnitForm(page, "Kamar 3");
    await termCheckbox(panel, "Lainnya").click();
    await panel.getByLabel("Jumlah bulan").fill("61");
    // Not checked in the browser: the API judges the range.
    const refused = page.waitForResponse((response) =>
      isUnitWrite(response.request()),
    );
    await panel.getByRole("button", { name: "Simpan unit" }).click();
    const response = await refused;
    expect(response.status()).toBe(400);
    const body = (await response.json()) as {
      error: { code: string; message: string; field?: string };
    };
    expect(body.error.code).toBe("invalid_input");
    expect(body.error.field).toBe("terms");
    expect(body.error.message).toBe("Jangka sewa harus 1 sampai 60 bulan.");

    const terms = panel.getByRole("group", { name: TERMS });
    await expect(terms.getByText(body.error.message)).toBeVisible();
    await expect(panel).toBeVisible();
    // The terms' first checkbox takes the focus.
    await expect(termCheckbox(panel, "Per bulan")).toBeFocused();
    await terms.getByText(body.error.message).scrollIntoViewIfNeeded();
    await attachViewportScreenshot(page, testInfo, "unit-custom-term-bad-n");

    // Changing the custom row clears the API's error.
    await panel.getByLabel("Jumlah bulan").fill("60");
    await expect(terms.getByText(body.error.message)).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });

  test("refuses 6 months as Lainnya before sending anything", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const property = await createProperty(page, orgId, testInfo, {
      property: "Wisma Uji Lainnya Enam",
      kind: "kos",
    });
    await createUnit(page, orgId, property, "Kamar 4", [
      { months: 1, price: 1_250_000 },
    ]);
    const unitWrites: string[] = [];
    page.on("request", (request) => {
      if (isUnitWrite(request)) unitWrites.push(request.url());
    });
    await openPropertyPage(page, orgId, property);

    const panel = await openUnitForm(page, "Kamar 4");
    await termCheckbox(panel, "Lainnya").click();
    const months = panel.getByLabel("Jumlah bulan");
    // Empty first, then a standard term.
    await panel.getByRole("button", { name: "Simpan unit" }).click();
    await expect(panel.getByText("Isi jumlah bulan.")).toBeVisible();
    await expect(months).toBeFocused();

    await months.fill("6");
    await expect(panel.getByText("Isi jumlah bulan.")).toHaveCount(0);
    await panel.getByRole("button", { name: "Simpan unit" }).click();
    await expect(
      panel.getByText(
        "Untuk 1, 3, 6 atau 12 bulan, centang pilihannya di atas.",
      ),
    ).toBeVisible();
    await expect(months).toHaveAttribute("aria-invalid", "true");
    await expect(months).toBeFocused();
    expect(unitWrites).toEqual([]);

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
