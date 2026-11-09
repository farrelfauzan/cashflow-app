import {
  expect,
  test,
  type APIRequestContext,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import type {
  Lease,
  Property,
  PropertyList,
  Tenant,
  TenantList,
  Unit,
} from "../src/lib/api/generated/model";
import { formatDate, jakartaToday } from "../src/lib/format/date";
import {
  animationsSettled,
  attachFullPageScreenshot,
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { setDate } from "./support/date-field";
import { minBodyFontPx, projectDigit, projectWord } from "./support/projects";
import { findOrCreateProperty } from "./support/property-fixtures";
import { chooseProperty } from "./support/property-picker";
import { ORG_HOME_URL } from "./support/shell";
import { expectToast, toast } from "./support/toast";
import { DIMAS, HENDRA, SARI, type SeededStaff } from "./support/staff";

// "Tambah penyewa", "Ubah penyewa" and "Sewakan unit", against the real API (seed-demo data). Pak
// Hendra is Admin of the whole of Griya Sejahtera and Supervisor in Kost Putri Anggrek; Mbak Sari
// is Admin limited to Kos Melati. Tests change only rows they create themselves: their own
// unit and tenants (in a property reused across runs, except where a test needs one with no other
// unit), with names and numbers no other run, project or retry uses. Tenant
// names start with W or Y, so they sort after every seeded tenant and never push one off the
// first page of the list.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const TAKEN_MESSAGE = "Nomor ini sudah dipakai penyewa lain.";
const OCCUPIED_MESSAGE = "Unit ini masih disewa. Akhiri sewa yang lama dulu.";
const NO_FREE_UNIT_TEXT = "Semua unit di properti ini sudah terisi.";

function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

// A name no other run, project, test or retry uses.
function uniqueName(testInfo: TestInfo, base: string): string {
  const project = projectWord(testInfo);
  const retry = testInfo.retry === 0 ? "" : ` r${testInfo.retry}`;
  return `${base} ${runId()} ${project}${retry}`;
}

// A tenant number no other run, project, test or retry uses (tenant numbers are unique per
// organization).
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = projectDigit(testInfo);
  return `0813-9${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
}

// A calendar date `months` months from today in Jakarta, on the 10th (every month has one, so no
// month end can move it).
function tenthOfMonth(months: number): string {
  const [year = 0, month = 1] = jakartaToday(new Date()).split("-").map(Number);
  return new Date(Date.UTC(year, month - 1 + months, 10))
    .toISOString()
    .slice(0, 10);
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

async function openTenantPage(page: Page, orgId: string, tenant: Tenant) {
  await page.goto(`/o/${orgId}/tenants/${tenant.id}`);
  await expect(
    page.getByRole("heading", { level: 1, name: tenant.name }),
  ).toBeVisible();
}

// Text on the current page only. The app keeps a page it navigated away from in the DOM, hidden.
function visibleText(page: Page, text: string | RegExp): Locator {
  return page.getByText(text).filter({ visible: true });
}

async function expectTouchTarget(locator: Locator) {
  await animationsSettled(locator);
  const box = await locator.boundingBox();
  // Rounded to a hundredth of a pixel: an animated popover can leave a 43.99998px box.
  expect(Math.round((box?.height ?? 0) * 100) / 100).toBeGreaterThanOrEqual(44);
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
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

// Setup through the API, as the signed-in member (the page's cookies).
async function post<T>(
  api: APIRequestContext,
  path: string,
  data: object,
): Promise<T> {
  const response = await api.post(`/api${path}`, { data });
  expect(response.status(), await response.text()).toBe(201);
  return (await response.json()) as T;
}

// A new free unit whose usual rent is Rp1.250.000, named uniquely per run. It goes in a property
// reused across runs ("Wisma Tetap ...", support/property-fixtures.ts), or, when the test needs
// the property to hold no other unit (`own`), in a property made for this run ("Wisma Uji ...").
async function createUnit(
  page: Page,
  orgId: string,
  testInfo: TestInfo,
  propertyBase: string,
  own = false,
) {
  const property = own
    ? await post<Property>(page.request, `/orgs/${orgId}/properties`, {
        name: uniqueName(testInfo, `Wisma Uji ${propertyBase}`),
        kind: "kos",
        address: "",
      })
    : await findOrCreateProperty(page, orgId, testInfo, {
        base: propertyBase,
        kind: "kos",
      });
  const unit = await post<Unit>(
    page.request,
    `/orgs/${orgId}/properties/${property.id}/units`,
    {
      label: uniqueName(testInfo, "Kamar 1"),
      terms: [{ months: 1, price: 1_250_000 }],
    },
  );
  return { property, unit };
}

async function createTenant(
  page: Page,
  orgId: string,
  name: string,
  phone: string,
): Promise<Tenant> {
  return post<Tenant>(page.request, `/orgs/${orgId}/tenants`, {
    name,
    phone,
    note: "",
  });
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("adds a tenant, shows a taken number under the field, and rents them a unit with a later billing start", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openTenants(page, HENDRA, GRIYA);
    const orgId = orgIdOf(page);
    const { property, unit } = await createUnit(page, orgId, testInfo, "Sewa");
    const name = uniqueName(testInfo, "Wulan Sekarsari");
    const phone = tenantNumber(testInfo, 1);

    const add = page.getByRole("button", { name: "Tambah penyewa" });
    await expectTouchTarget(add);
    await add.click();
    const form = page.getByRole("dialog", { name: "Tambah penyewa" });
    await expect(form).toContainText(GRIYA);

    // Nothing typed: both fields say so, and the name has the focus.
    await form.getByRole("button", { name: "Simpan penyewa" }).click();
    await expect(form.getByText("Isi nama penyewa.")).toBeVisible();
    await expect(form.getByText("Isi nomor WhatsApp.")).toBeVisible();
    await expect(form.getByLabel("Nama penyewa")).toBeFocused();

    // Typing the name clears only its own message.
    await form.getByLabel("Nama penyewa").fill(name);
    await expect(form.getByText("Isi nama penyewa.")).toHaveCount(0);
    await expect(form.getByText("Isi nomor WhatsApp.")).toBeVisible();

    // A number another tenant has: the API's message under the number.
    await form.getByLabel("Nomor WhatsApp").fill("0813-0001-0002");
    await form
      .getByLabel("Catatan (boleh dikosongkan)")
      .fill("Kerja di toko, bayar lewat anaknya");
    await form.getByRole("button", { name: "Simpan penyewa" }).click();
    await expect(form.getByText(TAKEN_MESSAGE)).toBeVisible();
    // Refused: no toast, the message stays in the form.
    await expect(toast(page, "Penyewa ditambahkan")).toHaveCount(0);
    await expect(form.getByLabel("Nomor WhatsApp")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    await expect(form.getByLabel("Nomor WhatsApp")).toBeFocused();
    const min = minBodyFontPx(testInfo);
    expect(
      await fontSize(form.getByText(/Tulis seperti biasa/)),
    ).toBeGreaterThanOrEqual(min);
    await expectTouchTarget(form.getByLabel("Nama penyewa"));
    await attachFullPageScreenshot(page, testInfo, "tenant-form");

    // Changing the number clears the API's message.
    await form.getByLabel("Nomor WhatsApp").fill(phone);
    await expect(form.getByText(TAKEN_MESSAGE)).toHaveCount(0);
    await form.getByRole("button", { name: "Simpan penyewa" }).click();
    const added = page.getByRole("dialog", { name: "Penyewa ditambahkan" });
    // A green toast once the API answered, next to the panel's own "Penyewa ditambahkan".
    await expectToast(page, "Penyewa ditambahkan", "create", name);
    // The toast's close button works over the open panel and leaves the panel open.
    await toast(page, "Penyewa ditambahkan")
      .getByRole("button", { name: "Tutup notifikasi" })
      .click();
    await expect(toast(page, "Penyewa ditambahkan")).toHaveCount(0);
    await expect(added).toBeVisible();
    await expect(added).toContainText(
      `${name} belum punya sewa. Sewakan unit sekarang supaya tagihannya bisa dibuat.`,
    );
    await expect(added).toContainText(
      "Kalau belum siap, penyewa tetap tersimpan",
    );
    await expectTouchTarget(added.getByRole("button", { name: "Nanti saja" }));
    await attachFullPageScreenshot(page, testInfo, "tenant-added");

    // "Sewakan unit" goes on to the lease form in the same panel.
    await added.getByRole("button", { name: "Sewakan unit" }).click();
    const lease = page.getByRole("dialog", { name: "Sewakan unit" });
    await expect(lease).toContainText(`${name} · ${phone}`);
    await chooseProperty(page, lease, property.name);
    await choose(page, lease, "Unit", unit.label);
    // An amount takes digits only and shows them grouped; the API gets an integer.
    const deposit = lease.getByLabel("Uang jaminan");
    await deposit.fill("Rp 1x250.000,5");
    await expect(deposit).toHaveValue("12.500.005");
    await deposit.fill("1250000");
    await expect(deposit).toHaveValue("1.250.000");
    // Backspace right after a dot removes the digit before it, and the caret stays where the
    // admin is typing.
    await deposit.click();
    await expect(deposit).toBeFocused();
    await deposit.evaluate((input: HTMLInputElement) =>
      input.setSelectionRange(2, 2),
    );
    expect(
      await deposit.evaluate((input: HTMLInputElement) => input.selectionStart),
    ).toBe(2);
    await page.keyboard.press("Backspace");
    await expect(deposit).toHaveValue("250.000");
    await page.keyboard.press("1");
    await expect(deposit).toHaveValue("1.250.000");
    expect(
      await deposit.evaluate((input: HTMLInputElement) => input.selectionStart),
    ).toBe(1);
    await expectTouchTarget(deposit);
    await deposit.fill("2500000");
    await expect(deposit).toHaveValue("2.500.000");
    await expect(lease.getByRole("combobox", { name: "Penagih" })).toHaveText(
      "Tidak ada",
    );

    // Started two months ago: "Mulai tagih dari" offers to skip the first period.
    const start = tenthOfMonth(-2);
    const billedFrom = tenthOfMonth(-1);
    await setDate(lease, "Mulai sewa", start);
    await expect(lease).toContainText(
      "Mulai tanggal 10, jatuh tempo tiap tanggal 10.",
    );
    const billing = lease.getByRole("group", { name: "Mulai tagih dari" });
    await expect(billing).toBeVisible();
    await expect(billing).toContainText(
      `Semua bulan sejak ${formatDate(start)} dibuatkan tagihan.`,
    );
    await expect(lease).toContainText(
      `jadi setiap bulan sejak ${formatDate(start)} ikut ditagih`,
    );
    await billing
      .getByRole("radio", { name: /Mulai dari bulan yang saya pilih/ })
      .check();
    await choose(page, billing, "Bulan mulai tagih", formatDate(billedFrom));
    await expect(lease).toContainText(formatDate(billedFrom));
    await expect(lease).not.toContainText("sebagai pesanan");
    await expectTouchTarget(lease.getByRole("button", { name: "Simpan sewa" }));
    await attachFullPageScreenshot(page, testInfo, "lease-form-billing");

    await lease.getByRole("button", { name: "Simpan sewa" }).click();
    await expectToast(
      page,
      "Sewa dibuat",
      "create",
      `${unit.label} · ${property.name}`,
    );
    await expect(lease).toHaveCount(0);
    await expect(
      visibleText(
        page,
        `${unit.label} · ${property.name} sudah disewakan kepada ${name}.`,
      ),
    ).toBeVisible();

    // The new lease is on the tenant's page, with the later billing start.
    await page.getByLabel("Cari nama, nomor, atau unit").fill(name);
    const row = list.getByRole("listitem").filter({ hasText: name });
    await expect(row).toContainText(unit.label);
    await row.getByRole("link").click();
    await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
    const card = page.getByRole("region", {
      name: `${unit.label} · ${property.name}`,
    });
    await expect(card).toContainText("Aktif");
    await expect(card).toContainText("Rp1.250.000");
    await expect(card).toContainText("Rp2.500.000");
    await expect(card).toContainText(formatDate(start));
    await expect(card).toContainText(formatDate(billedFrom));
    await expect(card).toContainText("dianggap sudah ditagih sendiri");
    // The tenant has a running lease now: no "Sewakan unit".
    await expect(
      page.getByRole("button", { name: "Sewakan unit" }),
    ).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });

  test("shows the API's refusal under Unit when the unit was taken meanwhile", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await openTenants(page, HENDRA, GRIYA);
    const orgId = orgIdOf(page);
    const { property, unit } = await createUnit(
      page,
      orgId,
      testInfo,
      "Bentrok",
      true,
    );
    const tenant = await createTenant(
      page,
      orgId,
      uniqueName(testInfo, "Yusuf Hidayat"),
      tenantNumber(testInfo, 2),
    );
    const other = await createTenant(
      page,
      orgId,
      uniqueName(testInfo, "Yanti Kusuma"),
      tenantNumber(testInfo, 3),
    );

    await openTenantPage(page, orgId, tenant);
    const none = page.getByRole("region", { name: "Belum ada sewa" });
    await expect(none).toContainText(
      `${tenant.name} belum menyewa unit apa pun. Sewakan satu unit supaya tagihan bisa dibuat.`,
    );
    const rent = none.getByRole("button", { name: "Sewakan unit" });
    await expectTouchTarget(rent);
    await rent.click();
    const lease = page.getByRole("dialog", { name: "Sewakan unit" });
    // Nothing chosen yet: each field says what it misses, and the first one takes the focus.
    await lease.getByRole("button", { name: "Simpan sewa" }).click();
    await expect(lease.getByText("Pilih properti.")).toBeVisible();
    await expect(lease.getByText("Isi tanggal mulai sewa.")).toBeVisible();
    await expect(
      lease.getByRole("combobox", { name: "Properti" }),
    ).toBeFocused();
    // Choosing the property clears its own message, not the start date's.
    await chooseProperty(page, lease, property.name);
    await expect(lease.getByText("Pilih properti.")).toHaveCount(0);
    await expect(lease.getByText("Isi tanggal mulai sewa.")).toBeVisible();
    await choose(page, lease, "Unit", unit.label);

    // A booking: the board's note, no "Mulai tagih dari".
    await setDate(lease, "Mulai sewa", tenthOfMonth(2));
    await expect(lease).toContainText("dihitung sebagai pesanan");
    await expect(
      lease.getByRole("group", { name: "Mulai tagih dari" }),
    ).toHaveCount(0);
    // Today: neither.
    const today = jakartaToday(new Date());
    await setDate(lease, "Mulai sewa", today);
    await expect(lease).not.toContainText("dihitung sebagai pesanan");
    await expect(
      lease.getByRole("group", { name: "Mulai tagih dari" }),
    ).toHaveCount(0);

    // Someone else rents the unit before this form is saved.
    await post<Lease>(page.request, `/orgs/${orgId}/leases`, {
      tenantId: other.id,
      unitId: unit.id,
      startDate: today,
      termMonths: 1,
    });
    await lease.getByRole("button", { name: "Simpan sewa" }).click();
    await expect(lease.getByText(OCCUPIED_MESSAGE)).toBeVisible();
    const unitSelect = lease.getByRole("combobox", { name: "Unit" });
    await expect(unitSelect).toHaveAttribute("aria-invalid", "true");
    // The API names the unit: its select takes the focus.
    await expect(unitSelect).toBeFocused();
    // The unit list reloads, so the taken unit is no longer offered.
    await expect(unitSelect).toHaveText("Pilih unit");
    await expect(lease).toContainText(NO_FREE_UNIT_TEXT);
    await lease.getByRole("button", { name: "Batal" }).click();
    await expect(lease).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });

  test("changes a tenant's name and note", async ({ page }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await openTenants(page, HENDRA, GRIYA);
    const orgId = orgIdOf(page);
    const phone = tenantNumber(testInfo, 4);
    const tenant = await createTenant(
      page,
      orgId,
      uniqueName(testInfo, "Wahyu Saputra"),
      phone,
    );
    const renamed = uniqueName(testInfo, "Wahyu Saputro");

    await openTenantPage(page, orgId, tenant);
    const edit = page.getByRole("button", { name: "Ubah penyewa" });
    await expectTouchTarget(edit);
    await edit.click();
    const form = page.getByRole("dialog", { name: "Ubah penyewa" });
    await expect(form.getByLabel("Nama penyewa")).toHaveValue(tenant.name);
    await expect(form.getByLabel("Nomor WhatsApp")).toHaveValue(phone);

    await form.getByLabel("Nama penyewa").fill(" ");
    await form.getByRole("button", { name: "Simpan penyewa" }).click();
    await expect(form.getByText("Isi nama penyewa.")).toBeVisible();

    await form.getByLabel("Nama penyewa").fill(renamed);
    await form
      .getByLabel("Catatan (boleh dikosongkan)")
      .fill("Pindahan dari Kontrakan Mawar");
    await form.getByRole("button", { name: "Simpan penyewa" }).click();
    await expectToast(page, "Data penyewa disimpan", "create", renamed);
    await expect(form).toHaveCount(0);
    await expect(
      page.getByRole("heading", { level: 1, name: renamed }),
    ).toBeVisible();
    await expect(visibleText(page, `${renamed} sudah disimpan.`)).toBeVisible();

    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Admin limited to one property", () => {
  test.use({ storageState: SARI.storageState });

  test("cannot add a tenant, but may change one of her property's tenants", async ({
    page,
  }) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openTenants(page, SARI, GRIYA);
    await expect(
      page.getByRole("button", { name: "Tambah penyewa" }),
    ).toHaveCount(0);

    await list
      .getByRole("listitem")
      .filter({ hasText: "Budi Santoso" })
      .getByRole("link")
      .click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Budi Santoso" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Ubah penyewa" }),
    ).toBeVisible();
    // Budi lives in Kamar 2: no "Sewakan unit" for a tenant with a running lease.
    await expect(
      page.getByRole("button", { name: "Sewakan unit" }),
    ).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });
});

// Kos Bulanan Sejahtera is seeded with 25 properties, more than the 20 the property picker of
// "Sewakan unit" lists: a property past the first page is found by name. The test only opens the
// panel; it changes nothing.
test.describe("Admin of an organization with many properties", () => {
  test.use({ storageState: DIMAS.storageState });

  test("finds a property past the first page by name in the lease form", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await openTenants(page, DIMAS, "Kos Bulanan Sejahtera");
    const orgId = orgIdOf(page);

    // The names of every property in name order, as the picker's list is ordered.
    const propertiesResponse = await page.request.get(
      `/api/orgs/${orgId}/properties`,
      { params: { pageSize: 100 } },
    );
    expect(propertiesResponse.status()).toBe(200);
    const properties = (await propertiesResponse.json()) as PropertyList;
    expect(properties.total).toBeGreaterThan(20);
    const firstPage = properties.items.slice(0, 20).map((p) => p.name);
    const last = properties.items[properties.items.length - 1];
    if (last === undefined) throw new Error("no property in the organization");
    expect(firstPage).not.toContain(last.name);

    // A tenant the form can open for: one without a lease.
    const tenantsResponse = await page.request.get(
      `/api/orgs/${orgId}/tenants`,
      { params: { status: "none", pageSize: 1 } },
    );
    expect(tenantsResponse.status()).toBe(200);
    const tenant = ((await tenantsResponse.json()) as TenantList).items[0];
    if (tenant === undefined) throw new Error("no tenant without a lease");

    await openTenantPage(page, orgId, tenant);
    await page.getByRole("button", { name: "Sewakan unit" }).first().click();
    const lease = page.getByRole("dialog", { name: "Sewakan unit" });
    const picker = lease.getByRole("combobox", { name: "Properti" });
    await expect(picker).toContainText("Pilih properti");
    await expect(lease.getByRole("combobox", { name: "Unit" })).toBeDisabled();

    await picker.click();
    await expect(picker).toHaveAttribute("aria-expanded", "true");
    // The search box and the list carry their own names, and the trigger controls the popup.
    const search = page.getByRole("combobox", { name: "Cari nama properti" });
    await expect(
      page.getByRole("listbox", { name: "Daftar properti" }),
    ).toBeVisible();
    const popupId = await picker.getAttribute("aria-controls");
    expect(popupId).not.toBeNull();
    await expect(
      page
        .locator(`[id="${popupId}"]`)
        .getByRole("listbox", { name: "Daftar properti" }),
    ).toBeVisible();
    await expect(search).toBeFocused();
    await expectTouchTarget(search);
    await expect(page.getByRole("option")).toHaveCount(20);
    await expect(page.getByRole("option").first()).toHaveText(
      firstPage[0] ?? "",
    );
    await expect(
      page.getByText(
        `Menampilkan 20 dari ${properties.total} properti. Ketik nama untuk mencari yang lain.`,
      ),
    ).toBeVisible();
    await expectTouchTarget(page.getByRole("option").first());
    // The list scrolls with the wheel inside the side panel.
    const list = page.getByRole("listbox");
    await list.hover();
    await page.mouse.wheel(0, 400);
    await expect
      .poll(() => list.evaluate((el) => el.scrollTop))
      .toBeGreaterThan(0);
    await list.evaluate((el) => el.scrollTo(0, 0));
    await expect(page.getByRole("option", { name: last.name })).toHaveCount(0);
    await attachViewportScreenshot(page, testInfo, "property-picker");

    // Typing sends one search to the API and shows the property that was past the page.
    const searched = page.waitForRequest((request) => {
      const url = new URL(request.url());
      return (
        url.pathname === `/api/orgs/${orgId}/properties` &&
        url.searchParams.get("q") === last.name
      );
    });
    await search.fill(last.name);
    await searched;
    await expect(page.getByRole("option", { name: last.name })).toBeVisible();
    await expect(
      page.getByText(`Menampilkan 20 dari ${properties.total}`),
    ).toHaveCount(0);
    // One match is not picked for the member: the auto-choice is for an organization with one
    // property, not for a search with one result.
    await expect(page.getByRole("option")).toHaveCount(1);
    await expect(picker).toContainText("Pilih properti");
    await expect(lease.getByRole("combobox", { name: "Unit" })).toBeDisabled();

    // A name nobody has says so.
    await search.fill("Properti yang tidak ada");
    await expect(
      page.getByText("Tidak ada properti dengan nama itu."),
    ).toBeVisible();
    // A failed search says why in the list, not that no property has the name.
    const unavailable = "Layanan sedang tidak tersedia. Coba lagi sebentar.";
    await page.route(`**/api/orgs/${orgId}/properties?*q=*`, (route) =>
      route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({
          error: { code: "unavailable", message: unavailable },
        }),
      }),
    );
    await search.fill("Kos yang gagal dicari");
    await expect(
      page.getByRole("listbox", { name: "Daftar properti" }),
    ).toContainText(unavailable);
    await expect(
      page.getByText("Tidak ada properti dengan nama itu."),
    ).toHaveCount(0);
    await page.unrouteAll();
    await search.fill(last.name);

    // Picking it closes the list, keeps its name on the trigger and opens the units.
    await page.getByRole("option", { name: last.name, exact: true }).click();
    await expect(search).toHaveCount(0);
    await expect(picker).toBeFocused();
    await expect(picker).toHaveText(last.name);
    await expect(lease.getByRole("combobox", { name: "Unit" })).toBeEnabled();

    // Opening it again starts from the full page, with the pick checked.
    await picker.click();
    await expect(search).toHaveValue("");
    await expect(page.getByRole("option")).toHaveCount(20);
    // Escape closes only the list, not the panel.
    await page.keyboard.press("Escape");
    await expect(search).toHaveCount(0);
    await expect(lease).toBeVisible();
    await expect(picker).toHaveText(last.name);

    await lease.getByRole("button", { name: "Batal" }).click();
    await expect(lease).toHaveCount(0);
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees no control that adds or changes a tenant or a lease", async ({
    page,
  }) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openTenants(page, HENDRA, ANGGREK);
    await expect(
      page.getByRole("button", { name: "Tambah penyewa" }),
    ).toHaveCount(0);

    await list
      .getByRole("listitem")
      .filter({ hasText: "Nadia Putri" })
      .getByRole("link")
      .click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Nadia Putri" }),
    ).toBeVisible();
    await expect(page.getByRole("region").first()).toBeVisible();
    await expect(
      page.getByRole("button", { name: /Ubah penyewa|Sewakan unit/ }),
    ).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });
});
