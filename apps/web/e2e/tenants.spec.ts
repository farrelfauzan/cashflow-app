import {
  expect,
  test,
  type Locator,
  type Page,
  type PlaywrightWorkerArgs,
  type TestInfo,
} from "@playwright/test";

import type { TenantList } from "../src/lib/api/generated/model";
import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA, RUDI, SARI, type SeededStaff } from "./support/staff";

// "Penyewa" and "Penyewa dan sewanya": the list with its search and filters, and one tenant's
// leases, read-only, against the real API (seed-demo data). Pak Hendra is Admin of the whole of
// Griya Sejahtera and Supervisor in Kost Putri Anggrek; Mbak Sari is Admin limited to Kos Melati;
// Mas Rudi is a Karyawan in Kost Putri Anggrek. The screens change nothing, and neither do the
// tests.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const UNKNOWN_TENANT_ID = "00000000-0000-4000-8000-000000000000";

// Minimum body text per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };

function minFont(testInfo: TestInfo): number {
  return MIN_BODY_FONT_PX[testInfo.project.name] ?? 16;
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

// Text on the current page only. The app keeps a page it navigated away from in the DOM, hidden,
// so a text that is on both pages would match twice.
function visibleText(page: Page, text: string | RegExp): Locator {
  return page.getByText(text).filter({ visible: true });
}

// The tenants seed-demo puts in Griya Sejahtera. Other specs add tenants of their own (with names
// that sort after these), so counts look at the seeded rows only.
const GRIYA_SEEDED =
  /Rina Wulandari|Budi Santoso|Citra Lestari|Dewi Anggraini|Eko Prasetyo|Toko Sumber Rejeki|Fajar Nugroho/;

function seededRows(list: Locator): Locator {
  return list.getByRole("listitem").filter({ hasText: GRIYA_SEEDED });
}

function tenantRow(list: Locator, name: string): Locator {
  return list.getByRole("listitem").filter({ hasText: name });
}

async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

async function choose(page: Page, filter: string, option: string) {
  await page.getByRole("combobox", { name: filter }).click();
  await page.getByRole("option", { name: option, exact: true }).click();
}

// Opens one tenant's page from the list.
async function openTenant(page: Page, list: Locator, name: string) {
  await tenantRow(list, name).getByRole("link").click();
  await expect(page).toHaveURL(/\/tenants\/[0-9a-f-]{36}$/);
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
}

async function backToList(page: Page) {
  await page.getByRole("main").getByRole("link", { name: "Penyewa" }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Penyewa" }),
  ).toBeVisible();
  const list = page.getByRole("list", { name: "Daftar penyewa" });
  await expect(list.getByRole("listitem").first()).toBeVisible();
  return list;
}

// The id of a tenant, as an admin of the whole organization sees it, to open the page of a tenant
// another member cannot see.
async function tenantIdOf(
  playwright: PlaywrightWorkerArgs["playwright"],
  baseURL: string | undefined,
  orgUrl: string,
  name: string,
): Promise<string> {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(orgUrl)?.[1];
  if (baseURL === undefined || orgId === undefined) {
    throw new Error("baseURL and the organization's address are needed.");
  }
  const api = await playwright.request.newContext({
    baseURL,
    storageState: HENDRA.storageState,
  });
  try {
    const response = await api.get(`/api/orgs/${orgId}/tenants`);
    expect(response.ok()).toBe(true);
    const { items: tenants } = (await response.json()) as TenantList;
    const found = tenants.find((tenant) => tenant.name === name);
    if (found === undefined) throw new Error(`${name} is not seeded.`);
    return found.id;
  } finally {
    await api.dispose();
  }
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("lists the tenants with unit, collector and status, and flags the one without a collector", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openTenants(page, HENDRA, GRIYA);

    await expect(visibleText(page, "Ibu Lestari Handayani")).toHaveCount(0);
    await expect(visibleText(page, "Ayu Lestari")).toHaveCount(0);
    await expect(
      visibleText(page, "Semua penyewa Griya Sejahtera dan sewanya."),
    ).toBeVisible();
    // Adding tenants is e2e/tenant-forms.spec.ts, importing them e2e/tenant-import.spec.ts.
    await expect(
      page.getByRole("button", { name: "Tambah penyewa" }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Impor dari spreadsheet" }),
    ).toHaveAttribute("href", /\/o\/[0-9a-f-]{36}\/tenants\/import$/);

    await expect(seededRows(list)).toHaveCount(7);
    // Rina has two leases; the newest active one comes first, and it has no collector.
    const rina = tenantRow(list, "Rina Wulandari");
    await expect(rina).toContainText("0813-0001-0001");
    await expect(rina).toContainText("Rumah B");
    await expect(rina).toContainText("Kontrakan Mawar");
    await expect(rina).toContainText("+1 sewa lain");
    await expect(rina).toContainText(/tidak ada/i);
    await expect(rina).toContainText("Aktif");
    const budi = tenantRow(list, "Budi Santoso");
    await expect(budi).toContainText("Kamar 2");
    await expect(budi).toContainText("Mas Dimas Saputra");
    await expect(budi).not.toContainText("sewa lain");
    await expect(tenantRow(list, "Citra Lestari")).toContainText(
      "Mas Joko Prasetyo",
    );
    const ruko = tenantRow(list, "Toko Sumber Rejeki (Pak Agus)");
    await expect(ruko).toContainText("Ruko 1");
    await expect(ruko).toContainText("Ruko Godean");
    const dewi = tenantRow(list, "Dewi Anggraini");
    await expect(dewi).toContainText("Sudah keluar");
    await expect(dewi).toContainText("keluar 30 Sep");
    await expect(dewi).toContainText(/tidak ada/i);
    // A lease booked for next month starts on its first day.
    const fajar = tenantRow(list, "Fajar Nugroho");
    await expect(fajar).toContainText("Belum mulai");
    await expect(fajar).toContainText(/mulai 1 \p{L}{3}/u);
    // Mas Agus left; he is still the collector of Eko's lease.
    const eko = tenantRow(list, "Eko Prasetyo");
    await expect(eko).toContainText("Kontrakan Mawar");
    await expect(eko).toContainText("Mas Agus Setiawan");
    await expect(eko).toContainText("Belum punya penagih");
    await expect(
      list.getByText("Belum punya penagih", { exact: true }),
    ).toHaveCount(1);

    const banner = page.getByRole("status").filter({
      hasText: "1 penyewa belum punya penagih",
    });
    await expect(banner).toBeVisible();
    await expect(banner).toContainText("Penagih mereka sudah dinonaktifkan.");
    const show = banner.getByRole("button", {
      name: "Tampilkan penyewa yang belum punya penagih",
    });

    // Text sizes and touch targets.
    const min = minFont(testInfo);
    expect(
      await fontSize(rina.getByText("Rina Wulandari")),
    ).toBeGreaterThanOrEqual(min);
    expect(
      await fontSize(rina.getByText("0813-0001-0001")),
    ).toBeGreaterThanOrEqual(min);
    expect(
      await fontSize(budi.getByText("Mas Dimas Saputra")),
    ).toBeGreaterThanOrEqual(min);
    expect(await fontSize(rina.getByText("Aktif"))).toBeGreaterThanOrEqual(min);
    expect(
      await fontSize(banner.getByText("1 penyewa")),
    ).toBeGreaterThanOrEqual(min);
    await expectTouchTarget(show);
    await expectTouchTarget(page.getByLabel("Cari nama, nomor, atau unit"));
    await expectTouchTarget(
      page.getByRole("combobox", { name: "Filter properti" }),
    );
    await expectTouchTarget(
      page.getByRole("combobox", { name: "Filter status" }),
    );
    await expectTouchTarget(rina.getByRole("link"));
    await attachFullPageScreenshot(page, testInfo, "tenants-list");

    // "Tampilkan" narrows the list to the flagged tenants, and the chip takes that off.
    await show.click();
    await expect(list.getByRole("listitem")).toHaveCount(1);
    await expect(tenantRow(list, "Eko Prasetyo")).toBeVisible();
    const chip = page.getByRole("button", {
      name: "Hanya yang belum punya penagih, hapus penyaring",
    });
    await expect(chip).toBeVisible();
    await expectTouchTarget(chip);
    await attachFullPageScreenshot(page, testInfo, "tenants-list-flagged");
    await chip.click();
    await expect(chip).toHaveCount(0);
    await expect(seededRows(list)).toHaveCount(7);

    expect(outsideRequests).toEqual([]);
  });

  test("searches by name, unit and phone digits, and filters by property and status", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openTenants(page, HENDRA, GRIYA);
    const search = page.getByLabel("Cari nama, nomor, atau unit");
    const rows = list.getByRole("listitem");

    await search.fill("prasetyo");
    await expect(rows).toHaveCount(1);
    await expect(tenantRow(list, "Eko Prasetyo")).toBeVisible();
    await search.fill("RUKO 1");
    await expect(rows).toHaveCount(1);
    await expect(tenantRow(list, "Toko Sumber Rejeki")).toBeVisible();
    // The same number, typed the way people type it.
    for (const typed of [
      "0813 0001 0004",
      "0813-0001-0004",
      "+62 813 0001 0004",
    ]) {
      await search.fill(typed);
      await expect(rows).toHaveCount(1);
      await expect(tenantRow(list, "Dewi Anggraini")).toBeVisible();
    }
    await search.fill("");
    await expect(seededRows(list)).toHaveCount(7);

    // A tenant is in a property through any of their leases.
    await choose(page, "Filter properti", "Kontrakan Mawar");
    await expect(rows).toHaveCount(2);
    await expect(tenantRow(list, "Rina Wulandari")).toBeVisible();
    await expect(tenantRow(list, "Eko Prasetyo")).toBeVisible();
    await choose(page, "Filter properti", "Kos Melati");
    await expect(rows).toHaveCount(5);
    await choose(page, "Filter status", "Sudah keluar");
    await expect(rows).toHaveCount(1);
    await expect(tenantRow(list, "Dewi Anggraini")).toBeVisible();
    await choose(page, "Filter status", "Belum mulai");
    await expect(rows).toHaveCount(1);
    await expect(tenantRow(list, "Fajar Nugroho")).toBeVisible();
    await choose(page, "Filter status", "Aktif");
    await expect(rows).toHaveCount(3);
    await search.fill("citra");
    await expect(rows).toHaveCount(1);

    // Nobody is left: say so, and "Hapus pencarian" takes everything off.
    await choose(page, "Filter status", "Tanpa sewa");
    await expect(
      page.getByRole("heading", { name: "Tidak ada penyewa yang cocok" }),
    ).toBeVisible();
    await expect(
      visibleText(
        page,
        "Coba kata lain, atau pilih Semua properti dan Semua status.",
      ),
    ).toBeVisible();
    await expect(list).toHaveCount(0);
    await attachFullPageScreenshot(page, testInfo, "tenants-list-no-results");
    const reset = page.getByRole("button", { name: "Hapus pencarian" });
    await expectTouchTarget(reset);
    await reset.click();
    await expect(seededRows(list)).toHaveCount(7);
    await expect(search).toHaveValue("");
    await expect(
      page.getByRole("combobox", { name: "Filter properti" }),
    ).toHaveText("Semua properti");
    await expect(
      page.getByRole("combobox", { name: "Filter status" }),
    ).toHaveText("Semua status");

    await search.fill("tegalrejo");
    await expect(
      page.getByRole("heading", { name: "Tidak ada penyewa yang cocok" }),
    ).toBeVisible();

    expect(outsideRequests).toEqual([]);
  });

  test("opens a tenant and shows each lease with its terms, collector and dates", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    let list = await openTenants(page, HENDRA, GRIYA);

    // Rina has two leases, and the first has a fixed extra charge.
    await openTenant(page, list, "Rina Wulandari");
    await expect(visibleText(page, "0813-0001-0001")).toBeVisible();
    await expect(visibleText(page, /WhatsApp aktif/)).toHaveCount(0);
    const first = page.getByRole("region", { name: /Kamar 1 · Kos Melati/ });
    await expect(first).toBeVisible();
    await expect(first).toContainText("Aktif");
    await expect(first.getByText("Sewa per bulan")).toBeVisible();
    await expect(first).toContainText("Rp850.000");
    await expect(first.getByText("Uang jaminan")).toBeVisible();
    await expect(first).toContainText("Mas Dimas Saputra");
    await expect(first).not.toContainText("Nonaktif, belum diganti");
    await expect(first.getByText("Mulai sewa")).toBeVisible();
    await expect(first).toContainText("1 Jul 2026");
    await expect(first.getByText("Mulai tagih dari")).toBeVisible();
    // The label comes before the card's "Rencana keluar" button.
    await expect(first.getByText("Rencana keluar").first()).toBeVisible();
    await expect(first).toContainText("Belum diatur");
    await expect(first.getByText("Biaya tambahan tetap")).toBeVisible();
    await expect(first).toContainText("Wi-Fi Rp50.000");
    await expect(first).toContainText(
      /Dimasukkan ke aplikasi \d{1,2} \p{L}{3} \d{4}\./u,
    );
    const second = page.getByRole("region", {
      name: /Rumah B · Kontrakan Mawar/,
    });
    await expect(second).toBeVisible();
    await expect(second).toContainText("Rp1.500.000");
    await expect(second).toContainText("Tidak ada");
    await expect(second.getByText("Biaya tambahan tetap")).toHaveCount(0);
    // Pak Hendra may change both active leases (lease-changes.spec.ts and lease-charges.spec.ts
    // cover what the buttons do); invoices are not on this page yet.
    for (const card of [first, second]) {
      await expect(
        card.getByRole("button", { name: "Biaya tambahan", exact: true }),
      ).toBeVisible();
      await expect(
        card.getByRole("button", { name: "Rencana keluar" }),
      ).toBeVisible();
      await expect(
        card.getByRole("button", { name: "Catat pindah keluar" }),
      ).toBeVisible();
    }
    await expect(page.getByRole("button", { name: "Ubah sewa" })).toHaveCount(
      2,
    );
    await expect(
      page.getByRole("button", { name: "Biaya tambahan", exact: true }),
    ).toHaveCount(2);
    await expect(
      page.getByRole("button", { name: /Lihat tagihan/ }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("link", { name: /Biaya tambahan|Lihat tagihan/ }),
    ).toHaveCount(0);

    const min = minFont(testInfo);
    expect(
      await fontSize(first.getByText("Rp850.000").first()),
    ).toBeGreaterThanOrEqual(min);
    expect(
      await fontSize(first.getByText("Sewa per bulan")),
    ).toBeGreaterThanOrEqual(min);
    expect(
      await fontSize(first.getByText("Wi-Fi Rp50.000")),
    ).toBeGreaterThanOrEqual(min);
    expect(
      await fontSize(first.getByText(/Dimasukkan ke aplikasi/)),
    ).toBeGreaterThanOrEqual(min);
    await expectTouchTarget(
      page.getByRole("main").getByRole("link", { name: "Penyewa" }),
    );
    await attachFullPageScreenshot(page, testInfo, "tenant-detail");

    // Eko: Mas Agus is disabled and still the collector.
    list = await backToList(page);
    await openTenant(page, list, "Eko Prasetyo");
    const lease = page.getByRole("region", {
      name: /Rumah A · Kontrakan Mawar/,
    });
    await expect(lease).toContainText("Mas Agus Setiawan");
    await expect(lease).toContainText("Nonaktif, belum diganti");
    await expect(lease).toContainText("Rp1.500.000");
    await expect(lease).toContainText("1 Jun 2026");
    await expect(lease.getByText("Biaya tambahan tetap")).toHaveCount(0);
    expect(
      await fontSize(lease.getByText("Nonaktif, belum diganti")),
    ).toBeGreaterThanOrEqual(min);
    await attachFullPageScreenshot(page, testInfo, "tenant-detail-flagged");

    // An ended lease shows when the tenant left, and a booked one has not started.
    list = await backToList(page);
    await openTenant(page, list, "Dewi Anggraini");
    const ended = page.getByRole("region", { name: /Kamar 4 · Kos Melati/ });
    await expect(ended).toContainText("Sudah keluar");
    await expect(ended.getByText("Keluar", { exact: true })).toBeVisible();
    await expect(ended).toContainText("30 Sep 2026");
    await expect(ended.getByText("Rencana keluar")).toHaveCount(0);
    list = await backToList(page);
    await openTenant(page, list, "Fajar Nugroho");
    await expect(
      page.getByRole("region", { name: /Kamar 5 · Kos Melati/ }),
    ).toContainText("Belum mulai");

    expect(outsideRequests).toEqual([]);
  });

  test("says so when the tenant is not found", async ({ page }) => {
    const outsideRequests = trackOutsideRequests(page);
    await openTenants(page, HENDRA, GRIYA);
    const home = page.url().replace(/\/tenants$/, "");
    await page.goto(`${home}/tenants/${UNKNOWN_TENANT_ID}`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Penyewa tidak ditemukan" }),
    ).toBeVisible();
    await page.getByRole("link", { name: "Kembali ke daftar penyewa" }).click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Penyewa" }),
    ).toBeVisible();
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees the list and a tenant without anything to change", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openTenants(page, HENDRA, ANGGREK);

    await expect(list.getByRole("listitem")).toHaveCount(3);
    await expect(visibleText(page, "Rina Wulandari")).toHaveCount(1);
    await expect(tenantRow(list, "Ayu Lestari")).toContainText(
      "Mas Rudi Hartanto",
    );
    await expect(tenantRow(list, "Nadia Putri")).toContainText(/tidak ada/i);
    await expect(tenantRow(list, "Rina Wulandari")).toContainText("Kamar 3");
    await expect(tenantRow(list, "Rina Wulandari")).not.toContainText(
      "sewa lain",
    );
    await expect(visibleText(page, /penyewa belum punya penagih/)).toHaveCount(
      0,
    );
    await attachFullPageScreenshot(page, testInfo, "tenants-list-supervisor");

    await openTenant(page, list, "Ayu Lestari");
    await expect(
      page.getByRole("region", { name: /Kamar 1 · Kos Putri Anggrek/ }),
    ).toContainText("Rp750.000");
    await expect(
      page.getByRole("button", { name: /Ubah|Catat|Rencana|Biaya/ }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("link", { name: /Ubah|Catat|Biaya/ }),
    ).toHaveCount(0);
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Admin limited to one property", () => {
  test.use({ storageState: SARI.storageState });

  test("sees only the tenants of her property, and only the leases there", async ({
    page,
    playwright,
    baseURL,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openTenants(page, SARI, GRIYA);
    const rows = list.getByRole("listitem");

    // Dewi moved out: a member limited to a property sees only tenants with an active lease.
    await expect(rows).toHaveCount(4);
    for (const name of [
      "Rina Wulandari",
      "Budi Santoso",
      "Citra Lestari",
      "Fajar Nugroho",
    ]) {
      await expect(tenantRow(list, name)).toBeVisible();
    }
    await expect(visibleText(page, "Dewi Anggraini")).toHaveCount(0);
    await expect(visibleText(page, "Eko Prasetyo")).toHaveCount(0);
    await expect(
      visibleText(page, "Toko Sumber Rejeki (Pak Agus)"),
    ).toHaveCount(0);
    // Rina's second lease is in a property she cannot see.
    await expect(tenantRow(list, "Rina Wulandari")).not.toContainText(
      "sewa lain",
    );
    await expect(tenantRow(list, "Rina Wulandari")).not.toContainText(
      "Kontrakan Mawar",
    );
    await page.getByRole("combobox", { name: "Filter properti" }).click();
    await expect(page.getByRole("option")).toHaveText([
      "Semua properti",
      "Kos Melati",
    ]);
    await page.keyboard.press("Escape");
    await attachFullPageScreenshot(page, testInfo, "tenants-list-limited");

    await openTenant(page, list, "Rina Wulandari");
    await expect(page.getByRole("region")).toHaveCount(1);
    await expect(
      page.getByRole("region", { name: /Kamar 1 · Kos Melati/ }),
    ).toBeVisible();
    await expect(visibleText(page, /Rumah B/)).toHaveCount(0);

    // A tenant whose leases are all in other properties is not found.
    const ekoId = await tenantIdOf(
      playwright,
      baseURL,
      page.url(),
      "Eko Prasetyo",
    );
    await page.goto(
      page.url().replace(/\/tenants\/[0-9a-f-]{36}$/, `/tenants/${ekoId}`),
    );
    await expect(
      page.getByRole("heading", { level: 1, name: "Penyewa tidak ditemukan" }),
    ).toBeVisible();
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Karyawan", () => {
  test.use({ storageState: RUDI.storageState });

  test("sees only the tenants assigned to him", async ({ page }) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openTenants(page, RUDI, ANGGREK);

    await expect(list.getByRole("listitem")).toHaveCount(1);
    await expect(tenantRow(list, "Ayu Lestari")).toContainText(
      "Mas Rudi Hartanto",
    );
    await expect(visibleText(page, "Nadia Putri")).toHaveCount(0);
    await expect(visibleText(page, /penyewa belum punya penagih/)).toHaveCount(
      0,
    );

    await openTenant(page, list, "Ayu Lestari");
    await expect(page.getByRole("region")).toHaveCount(1);
    expect(outsideRequests).toEqual([]);
  });
});
