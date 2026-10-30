import {
  expect,
  test,
  type Locator,
  type Page,
  type PlaywrightWorkerArgs,
  type TestInfo,
} from "@playwright/test";

import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA, SARI, type SeededStaff } from "./support/staff";

// "Properti": the list with occupancy, a property's units with their tenants, and adding and
// changing a property and a unit, against the real API (seed-demo data). Pak Hendra is Admin of
// the whole of Griya Sejahtera and Supervisor in Kost Putri Anggrek; Mbak Sari is Admin limited to
// Kos Melati. Tests change only properties they create themselves, never a seeded one.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";

// Minimum body text per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };

function minFont(testInfo: TestInfo): number {
  return MIN_BODY_FONT_PX[testInfo.project.name] ?? 16;
}

// A name no other run, project, test or retry uses: both projects run in parallel on one
// database, property names are unique per organization (ignoring case), and nothing deletes a
// property, so a second run on the same database needs new names.
function uniqueName(testInfo: TestInfo, base: string): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  const project = testInfo.project.name === "phone" ? "Dua" : "Satu";
  const retry = testInfo.retry === 0 ? "" : ` r${testInfo.retry}`;
  return `${base} ${run} ${project}${retry}`;
}

// Opens the organization's properties page: straight in for one organization, by name for
// several.
async function openProperties(page: Page, staff: SeededStaff, name: string) {
  await page.goto("/organizations");
  if (staff.organizations.length > 1) {
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name })
      .click();
  }
  await expect(page).toHaveURL(ORG_HOME_URL);
  await page.goto(`${page.url()}/properties`);
  await expect(
    page.getByRole("heading", { level: 1, name: "Properti" }),
  ).toBeVisible();
  const list = page.getByRole("list", { name: "Daftar properti" });
  await expect(list.getByRole("listitem").first()).toBeVisible();
  return list;
}

// Text on the current page only. The app keeps a page it navigated away from in the DOM, hidden,
// so a text that is on both pages would match twice.
function visibleText(page: Page, text: string): Locator {
  return page.getByText(text).filter({ visible: true });
}

function propertyRow(list: Locator, name: string): Locator {
  return list.getByRole("listitem").filter({ hasText: name });
}

function unitRow(list: Locator, label: string): Locator {
  return list.getByRole("listitem").filter({ hasText: label });
}

// A property's id, read through the API as Pak Hendra (Admin of the whole of Griya Sejahtera), for
// a test whose member may not list it. `home` is the organization's page URL.
async function propertyIdAsHendra(
  playwright: PlaywrightWorkerArgs["playwright"],
  testInfo: TestInfo,
  home: string,
  name: string,
): Promise<string> {
  const orgId = new URL(home).pathname.split("/")[2];
  const api = await playwright.request.newContext({
    baseURL: testInfo.project.use.baseURL,
    storageState: HENDRA.storageState,
  });
  const response = await api.get(`/api/orgs/${orgId}/properties?pageSize=100`);
  expect(response.ok()).toBe(true);
  const body = (await response.json()) as {
    items: { id: string; name: string }[];
  };
  await api.dispose();
  const id = body.items.find((property) => property.name === name)?.id;
  if (id === undefined) throw new Error(`${name} is not seeded`);
  return id;
}

// Opens a property's page from the list by its "Buka unit" link.
async function openUnits(page: Page, list: Locator, name: string) {
  await propertyRow(list, name)
    .getByRole("link", { name: `Buka unit ${name}` })
    .click();
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
  return page.getByRole("list", { name: "Daftar unit" });
}

async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

// Adds a property through "Tambah properti" and returns its row.
async function addProperty(
  page: Page,
  list: Locator,
  name: string,
  kind: string,
  address: string,
) {
  await page.getByRole("button", { name: "Tambah properti" }).click();
  const panel = page.getByRole("dialog", { name: "Tambah properti" });
  await panel.getByLabel("Nama properti").fill(name);
  await panel.getByRole("radio", { name: kind }).check();
  await panel.getByLabel("Alamat (boleh dikosongkan)").fill(address);
  await panel.getByRole("button", { name: "Simpan properti" }).click();
  await expect(panel).toHaveCount(0);
  const row = propertyRow(list, name);
  await expect(row).toBeVisible();
  return row;
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("lists the properties with occupancy and opens one to see its units", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openProperties(page, HENDRA, GRIYA);

    await expect(visibleText(page, ANGGREK)).toHaveCount(0);
    await expect(
      visibleText(
        page,
        "Anda hanya bisa melihat. Untuk mengubah, minta admin.",
      ),
    ).toHaveCount(0);
    await expect(
      visibleText(page, "Anda hanya melihat properti yang ditugaskan"),
    ).toHaveCount(0);

    const melati = propertyRow(list, "Kos Melati");
    await expect(melati).toContainText("Jl. Kaliurang Km 5 No. 12");
    await expect(melati).toContainText("Kos");
    await expect(melati).toContainText("6 unit");
    await expect(melati).toContainText("4 terisi, 2 kosong");
    const mawar = propertyRow(list, "Kontrakan Mawar");
    await expect(mawar).toContainText("2 unit");
    await expect(mawar).toContainText("2 terisi, 0 kosong");
    const ruko = propertyRow(list, "Ruko Godean");
    await expect(ruko).toContainText("2 unit");
    await expect(ruko).toContainText("1 terisi, 1 kosong");

    const add = page.getByRole("button", { name: "Tambah properti" });
    await expect(add).toBeVisible();
    await expectTouchTarget(add);
    await expectTouchTarget(
      melati.getByRole("link", { name: "Buka unit Kos Melati" }),
    );
    await expectTouchTarget(
      melati.getByRole("button", { name: "Ubah Kos Melati" }),
    );
    expect(
      await fontSize(melati.getByRole("link", { name: "Kos Melati" }).first()),
    ).toBeGreaterThanOrEqual(minFont(testInfo));
    expect(
      await fontSize(melati.getByText("4 terisi, 2 kosong")),
    ).toBeGreaterThanOrEqual(minFont(testInfo));
    await attachFullPageScreenshot(page, testInfo, "properties-list");

    // Open Kos Melati: its units, who is in them, and the usual rent.
    const units = await openUnits(page, list, "Kos Melati");
    await expect(page).toHaveURL(/\/properties\/[0-9a-f-]{36}$/);
    await expect(
      visibleText(
        page,
        "Jl. Kaliurang Km 5 No. 12, Sleman, Yogyakarta · Kos · 6 unit",
      ),
    ).toBeVisible();
    await expect(unitRow(units, "Kamar 1")).toContainText("Rina Wulandari");
    await expect(unitRow(units, "Kamar 2")).toContainText("Budi Santoso");
    await expect(unitRow(units, "Kamar 3")).toContainText("Citra Lestari");
    await expect(unitRow(units, "Kamar 4")).toContainText("Kosong");
    // A lease booked for next month already makes the unit occupied.
    await expect(unitRow(units, "Kamar 5")).toContainText("Fajar Nugroho");
    await expect(unitRow(units, "Kamar 6")).toContainText("Kosong");
    await expect(unitRow(units, "Kamar 1")).toContainText("Rp850.000");
    // Plain text for now: the tenant pages come later.
    await expect(
      unitRow(units, "Kamar 1").getByRole("link", { name: "Rina Wulandari" }),
    ).toHaveCount(0);

    const addUnit = page.getByRole("button", { name: "Tambah unit" });
    await expectTouchTarget(addUnit);
    await expectTouchTarget(
      page.getByRole("button", { name: "Ubah properti" }),
    );
    await expectTouchTarget(
      unitRow(units, "Kamar 1").getByRole("button", { name: "Ubah Kamar 1" }),
    );
    await expectTouchTarget(
      page.getByRole("main").getByRole("link", { name: "Properti" }),
    );
    expect(
      await fontSize(unitRow(units, "Kamar 1").getByText("Rina Wulandari")),
    ).toBeGreaterThanOrEqual(minFont(testInfo));
    await attachFullPageScreenshot(page, testInfo, "property-units");

    // The back link returns to the list.
    await page
      .getByRole("main")
      .getByRole("link", { name: "Properti" })
      .click();
    await expect(
      page
        .getByRole("list", { name: "Daftar properti" })
        .getByRole("listitem")
        .first(),
    ).toBeVisible();
    expect(outsideRequests).toEqual([]);
  });

  test("adds a property, checks the form, refuses a taken name and changes it", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const name = uniqueName(testInfo, "Kos Cempaka");
    const renamed = uniqueName(testInfo, "Kos Cempaka Indah");
    const list = await openProperties(page, HENDRA, GRIYA);

    // Empty fields are caught before anything is sent.
    await page.getByRole("button", { name: "Tambah properti" }).click();
    const panel = page.getByRole("dialog", { name: "Tambah properti" });
    await expect(panel).toBeVisible();
    await attachFullPageScreenshot(page, testInfo, "property-form");
    await expectTouchTarget(panel.locator("label", { hasText: "Kontrakan" }));
    await expectTouchTarget(panel.getByLabel("Nama properti"));
    await expectTouchTarget(
      panel.getByRole("button", { name: "Simpan properti" }),
    );
    await expect(panel.getByRole("radio", { name: "Kos" })).not.toBeChecked();
    await panel.getByRole("button", { name: "Simpan properti" }).click();
    await expect(panel.getByText("Isi nama properti.")).toBeVisible();
    await expect(panel.getByText("Pilih jenis properti.")).toBeVisible();
    await expect(panel.getByLabel("Nama properti")).toBeFocused();

    // A name another property already has comes back from the API under the name field.
    await panel.getByLabel("Nama properti").fill("kos melati");
    await panel.getByRole("radio", { name: "Kos" }).check();
    await panel.getByRole("button", { name: "Simpan properti" }).click();
    await expect(panel.getByLabel("Nama properti")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    await expect(panel.getByRole("alert")).toBeVisible();
    await expect(panel).toBeVisible();
    await panel.getByRole("button", { name: "Batal" }).click();
    await expect(panel).toHaveCount(0);

    // Add a real one.
    const row = await addProperty(
      page,
      list,
      name,
      "Kontrakan",
      "Jl. Cempaka No. 9, Bantul",
    );
    await expect(visibleText(page, `${name} sudah ditambahkan.`)).toBeVisible();
    await expect(row).toContainText("Jl. Cempaka No. 9, Bantul");
    await expect(row).toContainText("Kontrakan");
    await expect(row).toContainText("0 unit");
    await expect(row).toContainText("Belum ada unit");

    // Change its name, kind and address.
    await row.getByRole("button", { name: `Ubah ${name}` }).click();
    const edit = page.getByRole("dialog", { name: "Ubah properti" });
    await expect(edit.getByLabel("Nama properti")).toHaveValue(name);
    await expect(edit.getByRole("radio", { name: "Kontrakan" })).toBeChecked();
    await expect(edit.getByLabel("Alamat (boleh dikosongkan)")).toHaveValue(
      "Jl. Cempaka No. 9, Bantul",
    );
    await edit.getByLabel("Nama properti").fill(renamed);
    await edit.getByRole("radio", { name: "Ruko" }).check();
    await edit.getByLabel("Alamat (boleh dikosongkan)").fill("");
    await edit.getByRole("button", { name: "Simpan properti" }).click();
    await expect(edit).toHaveCount(0);
    await expect(visibleText(page, `${renamed} sudah disimpan.`)).toBeVisible();
    const changed = propertyRow(list, renamed);
    await expect(changed).toContainText("Ruko");
    await expect(changed).not.toContainText("Jl. Cempaka");
    await expect(propertyRow(list, name)).toHaveCount(0);

    // The property page shows the new summary, and its own "Ubah properti" opens the same form.
    await openUnits(page, list, renamed);
    await expect(visibleText(page, "Ruko · 0 unit")).toBeVisible();
    await expect(
      visibleText(page, "Belum ada unit di properti ini."),
    ).toBeVisible();
    await page.getByRole("button", { name: "Ubah properti" }).click();
    const again = page.getByRole("dialog", { name: "Ubah properti" });
    await expect(again.getByLabel("Nama properti")).toHaveValue(renamed);
    // Saved from its own page, the property's line under the name follows (getProperty).
    await again
      .getByLabel("Alamat (boleh dikosongkan)")
      .fill("Jl. Cempaka No. 11, Bantul");
    await again.getByRole("button", { name: "Simpan properti" }).click();
    await expect(again).toHaveCount(0);
    await expect(
      visibleText(page, "Jl. Cempaka No. 11, Bantul · Ruko · 0 unit"),
    ).toBeVisible();

    expect(outsideRequests).toEqual([]);
  });

  test("adds and changes units and refuses a taken name", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const name = uniqueName(testInfo, "Kos Dahlia");
    const list = await openProperties(page, HENDRA, GRIYA);
    await addProperty(page, list, name, "Kos", "Jl. Dahlia No. 3, Sleman");
    const units = await openUnits(page, list, name);
    await expect(
      visibleText(page, "Belum ada unit di properti ini."),
    ).toBeVisible();

    // Empty label is caught before anything is sent.
    await page.getByRole("button", { name: "Tambah unit" }).click();
    const panel = page.getByRole("dialog", { name: "Tambah unit" });
    await expect(panel).toContainText(name);
    await panel.getByRole("button", { name: "Simpan unit" }).click();
    await expect(panel.getByText("Isi nama unit.")).toBeVisible();
    await expect(panel.getByLabel("Nama unit")).toBeFocused();

    // Only the label goes: a new kos unit gets the monthly term, with no price yet.
    await panel.getByLabel("Nama unit").fill("Kamar 01");
    await attachFullPageScreenshot(page, testInfo, "unit-form");
    await expectTouchTarget(panel.getByLabel("Nama unit"));
    await expectTouchTarget(panel.getByRole("button", { name: "Simpan unit" }));
    const saved = page.waitForRequest(
      (request) =>
        request.method() === "POST" && request.url().includes("/units"),
    );
    await panel.getByRole("button", { name: "Simpan unit" }).click();
    expect((await saved).postDataJSON()).toEqual({ label: "Kamar 01" });
    await expect(panel).toHaveCount(0);
    await expect(
      visibleText(page, "Kamar 01 sudah ditambahkan."),
    ).toBeVisible();
    const first = unitRow(units, "Kamar 01");
    await expect(first).toContainText("Belum diisi");
    await expect(first).toContainText("Kosong");
    // The unit count under the property's name follows.
    await expect(visibleText(page, "Kos · 1 unit")).toBeVisible();

    // A second unit.
    await page.getByRole("button", { name: "Tambah unit" }).click();
    await panel.getByLabel("Nama unit").fill("Kamar 02");
    await panel.getByRole("button", { name: "Simpan unit" }).click();
    await expect(panel).toHaveCount(0);
    await expect(unitRow(units, "Kamar 02")).toContainText("Belum diisi");

    // The same label again, in other letters, comes back from the API under the label.
    await page.getByRole("button", { name: "Tambah unit" }).click();
    await panel.getByLabel("Nama unit").fill("kamar 01");
    await panel.getByRole("button", { name: "Simpan unit" }).click();
    await expect(panel.getByLabel("Nama unit")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    await expect(panel.getByRole("alert")).toBeVisible();
    await panel.getByRole("button", { name: "Batal" }).click();
    await expect(panel).toHaveCount(0);

    // Change the first unit's label; only the label is sent, so its terms stay.
    await first.getByRole("button", { name: "Ubah Kamar 01" }).click();
    const edit = page.getByRole("dialog", { name: "Ubah unit" });
    await expect(edit.getByLabel("Nama unit")).toHaveValue("Kamar 01");
    await edit.getByLabel("Nama unit").fill("Kamar 01A");
    const changed = page.waitForRequest(
      (request) =>
        request.method() === "PUT" && request.url().includes("/units/"),
    );
    await edit.getByRole("button", { name: "Simpan unit" }).click();
    expect((await changed).postDataJSON()).toEqual({ label: "Kamar 01A" });
    await expect(edit).toHaveCount(0);
    await expect(visibleText(page, "Kamar 01A sudah disimpan.")).toBeVisible();
    await expect(unitRow(units, "Kamar 01A")).toContainText("Belum diisi");

    // Back on the list, the property counts both units, none occupied.
    await page
      .getByRole("main")
      .getByRole("link", { name: "Properti" })
      .click();
    const row = propertyRow(
      page.getByRole("list", { name: "Daftar properti" }),
      name,
    );
    await expect(row).toContainText("2 unit");
    await expect(row).toContainText("0 terisi, 2 kosong");

    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees the properties and units read-only", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openProperties(page, HENDRA, ANGGREK);

    await expect(
      visibleText(
        page,
        "Anda hanya bisa melihat. Untuk mengubah, minta admin.",
      ),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Tambah properti" }),
    ).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^Ubah/ })).toHaveCount(0);
    const row = propertyRow(list, "Kos Putri Anggrek");
    await expect(row).toContainText("4 unit");
    await expect(row).toContainText("3 terisi, 1 kosong");

    const units = await openUnits(page, list, "Kos Putri Anggrek");
    await expect(unitRow(units, "Kamar 1")).toContainText("Ayu Lestari");
    await expect(unitRow(units, "Kamar 4")).toContainText("Kosong");
    await expect(page.getByRole("button", { name: "Tambah unit" })).toHaveCount(
      0,
    );
    await expect(page.getByRole("button", { name: /^Ubah/ })).toHaveCount(0);
    await expect(
      visibleText(
        page,
        "Anda hanya bisa melihat. Untuk mengubah, minta admin.",
      ),
    ).toBeVisible();
    await attachFullPageScreenshot(page, testInfo, "property-units-supervisor");

    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Admin limited to one property", () => {
  test.use({ storageState: SARI.storageState });

  test("sees only her property, cannot add one and cannot open another", async ({
    page,
    playwright,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const list = await openProperties(page, SARI, GRIYA);

    await expect(
      visibleText(
        page,
        "Anda hanya melihat properti yang ditugaskan kepada Anda.",
      ),
    ).toBeVisible();
    await expect(list.getByRole("listitem")).toHaveCount(1);
    await expect(propertyRow(list, "Kos Melati")).toBeVisible();
    await expect(visibleText(page, "Kontrakan Mawar")).toHaveCount(0);
    await expect(visibleText(page, "Ruko Godean")).toHaveCount(0);
    // Adding a property needs the whole organization, so there is no button, but she may change
    // her own property.
    await expect(
      page.getByRole("button", { name: "Tambah properti" }),
    ).toHaveCount(0);
    await expect(
      propertyRow(list, "Kos Melati").getByRole("button", {
        name: "Ubah Kos Melati",
      }),
    ).toBeVisible();
    await attachFullPageScreenshot(page, testInfo, "properties-list-limited");

    const units = await openUnits(page, list, "Kos Melati");
    await expect(unitRow(units, "Kamar 1")).toContainText("Rina Wulandari");
    await expect(
      page.getByRole("button", { name: "Tambah unit" }),
    ).toBeVisible();

    // A property that does not exist, and one that exists outside hers (getProperty answers
    // 404 and 403), both read "Properti tidak ditemukan".
    const home = page.url().replace(/\/properties\/[0-9a-f-]{36}$/, "");
    await page.goto(`${home}/properties/00000000-0000-4000-8000-000000000000`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Properti tidak ditemukan" }),
    ).toBeVisible();
    const mawarId = await propertyIdAsHendra(
      playwright,
      testInfo,
      home,
      "Kontrakan Mawar",
    );
    await page.goto(`${home}/properties/${mawarId}`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Properti tidak ditemukan" }),
    ).toBeVisible();
    await expect(visibleText(page, "Kontrakan Mawar")).toHaveCount(0);
    await page
      .getByRole("link", { name: "Kembali ke daftar properti" })
      .click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Properti" }),
    ).toBeVisible();

    expect(outsideRequests).toEqual([]);
  });
});
