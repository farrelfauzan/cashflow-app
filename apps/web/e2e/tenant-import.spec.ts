import { readFile } from "node:fs/promises";

import {
  expect,
  test,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import { formatDate, jakartaToday } from "../src/lib/format/date";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA, SARI, type SeededStaff } from "./support/staff";

// "Impor penyewa", against the real API (seed-demo data). Pak Hendra is Admin of the whole of
// Griya Sejahtera and Supervisor in Kost Putri Anggrek; Mbak Sari is Admin limited to Kos Melati.
// Only one test saves anything through the import: a new property "Wisma Impor ..." (sorting
// after every seeded property) with two tenants whose names start with W and Y (sorting after
// every seeded tenant) and numbers 0813-5..., which no other spec uses. The term test makes its own
// property "Wisma Impor Jangka ..." and its unit through the API. Every name and number is unique
// to the run, project and retry. The file's contents are never logged.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const IMPORT_PATH = "/imports/tenants";
const ROW_LIST = "Baris berkas";
const TOO_LARGE_MESSAGE = "Berkas terlalu besar. Paling besar 1 MB.";
// The API's warning for a file without the mulai_tagih column
// (services/api/internal/tenantimport/parse.go).
const NO_BILL_FROM_WARNING =
  "Tidak ada kolom mulai_tagih: setiap sewa ditagih sejak tanggal mulai.";
const HEADER = [
  "properti",
  "jenis_properti",
  "kamar",
  "nama_penyewa",
  "no_wa",
  "sewa",
  "tanggal_mulai",
  "mulai_tagih",
];

// Minimum body text per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };

function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

function suffix(testInfo: TestInfo): string {
  const project = testInfo.project.name === "phone" ? "Dua" : "Satu";
  const retry = testInfo.retry === 0 ? "" : ` r${testInfo.retry}`;
  return `${runId()} ${project}${retry}`;
}

// A name no other run, project, test or retry uses.
function uniqueName(testInfo: TestInfo, base: string): string {
  return `${base} ${suffix(testInfo)}`;
}

// A tenant number no other spec, run, project, row or retry uses (0813-5...).
function tenantNumber(testInfo: TestInfo, rowDigit: number): string {
  const run = runId();
  const project = testInfo.project.name === "phone" ? "2" : "1";
  return `0813-5${run.slice(0, 3)}-${run.slice(3)}${project}${rowDigit}${testInfo.retry}`;
}

// A calendar date `months` months from today in Jakarta, on the 10th.
function tenthOfMonth(months: number): string {
  const [year = 0, month = 1] = jakartaToday(new Date()).split("-").map(Number);
  return new Date(Date.UTC(year, month - 1 + months, 10))
    .toISOString()
    .slice(0, 10);
}

// 2026-10-10 as people type it: 10/10/2026.
function dayFirst(date: string): string {
  const [year, month, day] = date.split("-");
  return `${day}/${month}/${year}`;
}

// A CSV as Excel writes it in an Indonesian locale: semicolons and CRLF.
function csv(header: readonly string[], rows: readonly string[][]): Buffer {
  return Buffer.from(
    [header, ...rows].map((cells) => cells.join(";")).join("\r\n") + "\r\n",
    "utf8",
  );
}

async function chooseFile(page: Page, name: string, buffer: Buffer) {
  await page
    .locator('input[type="file"]')
    .setInputFiles({ name, mimeType: "text/csv", buffer });
}

// Every request the page makes to the import endpoint, to count them.
function trackImportRequests(page: Page): string[] {
  const requests: string[] = [];
  page.on("request", (request) => {
    if (new URL(request.url()).pathname.endsWith(IMPORT_PATH)) {
      requests.push(request.method());
    }
  });
  return requests;
}

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
  await expect(
    page
      .getByRole("list", { name: "Daftar penyewa" })
      .getByRole("listitem")
      .first(),
  ).toBeVisible();
}

// From the tenants page header, as an admin would.
async function openImport(page: Page) {
  await openTenants(page, HENDRA, GRIYA);
  await page.getByRole("link", { name: "Impor dari spreadsheet" }).click();
  await expect(page).toHaveURL(/\/o\/[0-9a-f-]{36}\/tenants\/import$/);
  await expect(
    page.getByRole("heading", { level: 1, name: "Impor penyewa" }),
  ).toBeVisible();
}

async function check(page: Page) {
  await page.getByRole("button", { name: "Periksa berkas" }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Periksa berkas impor" }),
  ).toBeVisible();
}

function importRow(page: Page, line: number): Locator {
  return page
    .getByRole("list", { name: ROW_LIST })
    .getByRole("listitem")
    .filter({ hasText: new RegExp(`Baris ${line}(?!\\d)`) });
}

async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("downloads the template with its byte order mark", async ({ page }) => {
    const outsideRequests = trackOutsideRequests(page);
    await openImport(page);

    const link = page.getByRole("link", { name: "Unduh template CSV" });
    await expectTouchTarget(link);
    const [download] = await Promise.all([
      page.waitForEvent("download"),
      link.click(),
    ]);
    expect(download.suggestedFilename()).toBe("template-impor-penyewa.csv");
    const bytes = await readFile(await download.path());
    expect([...bytes.subarray(0, 3)]).toEqual([0xef, 0xbb, 0xbf]);
    const header = bytes.subarray(3).toString("utf8").split(/\r?\n/)[0] ?? "";
    expect(header.split(/[;,]/)).toContain("nama_penyewa");

    expect(outsideRequests).toEqual([]);
  });

  test("lists a file's problems by row in Indonesian columns and keeps it from being saved", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await openImport(page);

    const property = uniqueName(testInfo, "Wisma Impor Galat");
    await chooseFile(
      page,
      "penyewa-salah.csv",
      csv(HEADER, [
        [
          property,
          "kos",
          "Kamar 1",
          uniqueName(testInfo, "Wening Astuti"),
          tenantNumber(testInfo, 3),
          "1.250.000",
          tenthOfMonth(1),
          "",
        ],
        [
          property,
          "kos",
          "Kamar 2",
          uniqueName(testInfo, "Yudha Saputra"),
          "12345",
          "1.250.000",
          tenthOfMonth(1),
          "",
        ],
        [
          property,
          "kos",
          "Kamar 3",
          uniqueName(testInfo, "Yuliana Sari"),
          tenantNumber(testInfo, 4),
          "1.250.000",
          "31/13/2026",
          "",
        ],
      ]),
    );
    await check(page);
    await expect(
      page.getByText("penyewa-salah.csv · belum ada yang disimpan."),
    ).toBeVisible();

    await expect(importRow(page, 2)).toContainText("Siap");
    const badPhone = importRow(page, 3);
    await expect(badPhone).toContainText("Perlu diperbaiki");
    await expect(badPhone).toContainText("Kamar 2");
    await expect(badPhone).toContainText(property);
    await expect(badPhone).toContainText("Kolom no_wa:");
    const badDate = importRow(page, 4);
    await expect(badDate).toContainText("Perlu diperbaiki");
    await expect(badDate).toContainText("Kolom tanggal_mulai:");
    // The terms could not be read, so no rent or start date is shown.
    await expect(badDate).not.toContainText("Mulai ");

    const save = page.getByRole("button", { name: "Simpan 1 baris" });
    await expect(save).toBeDisabled();
    await expect(
      page.getByText(
        "Simpan baru bisa kalau semua baris benar. Tidak ada yang tersimpan sebagian.",
      ),
    ).toBeVisible();

    const all = page.getByRole("button", { name: "Semua 3" });
    const errors = page.getByRole("button", { name: "Perlu diperbaiki 2" });
    await expect(all).toHaveAttribute("aria-pressed", "true");
    await expectTouchTarget(errors);
    await errors.click();
    await expect(errors).toHaveAttribute("aria-pressed", "true");
    const rows = page
      .getByRole("list", { name: ROW_LIST })
      .getByRole("listitem");
    await expect(rows).toHaveCount(2);
    await expect(importRow(page, 2)).toHaveCount(0);
    expect(
      await fontSize(badPhone.getByText("Kamar 2")),
    ).toBeGreaterThanOrEqual(MIN_BODY_FONT_PX[testInfo.project.name] ?? 16);
    await attachViewportScreenshot(page, testInfo, "import-check-errors");

    const fixed = page.getByRole("button", {
      name: "Pilih berkas yang sudah diperbaiki",
    });
    await expectTouchTarget(fixed);
    await fixed.click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Impor penyewa" }),
    ).toBeVisible();
    await expect(page.getByText("Belum ada berkas dipilih")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Periksa berkas" }),
    ).toBeDisabled();

    expect(outsideRequests).toEqual([]);
  });

  test("checks a clean file, saves it, and the tenants appear in the list", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const importRequests = trackImportRequests(page);
    await openImport(page);
    await expect(
      page.getByRole("button", { name: "Periksa berkas" }),
    ).toBeDisabled();

    const property = uniqueName(testInfo, "Wisma Impor");
    const first = uniqueName(testInfo, "Wiwik Pratiwi");
    const second = uniqueName(testInfo, "Yoga Permana");
    const firstStart = tenthOfMonth(-2);
    const firstBill = tenthOfMonth(-1);
    const secondStart = tenthOfMonth(1);
    const fileName = `penyewa-${runId()}.csv`;
    await chooseFile(
      page,
      fileName,
      csv(HEADER, [
        [
          property,
          "kos",
          "Kamar A1",
          first,
          tenantNumber(testInfo, 1),
          "Rp1.250.000",
          dayFirst(firstStart),
          dayFirst(firstBill),
        ],
        [
          property,
          "kos",
          "Kamar A2",
          second,
          tenantNumber(testInfo, 2),
          "1350000",
          secondStart,
          "",
        ],
      ]),
    );
    await expect(page.getByText(fileName)).toBeVisible();
    await expect(page.getByText(/^\d+ KB$/)).toBeVisible();
    const checkButton = page.getByRole("button", { name: "Periksa berkas" });
    await expect(checkButton).toBeEnabled();
    await expectTouchTarget(checkButton);
    await expectTouchTarget(page.getByRole("button", { name: "Ganti berkas" }));
    await attachViewportScreenshot(page, testInfo, "import-choose");

    await check(page);
    const one = importRow(page, 2);
    await expect(one).toContainText("Siap");
    await expect(one).toContainText(first);
    await expect(one).toContainText(
      `Kamar A1 · ${property} · Per bulan · Rp1.250.000`,
    );
    await expect(one).toContainText(
      `Mulai ${formatDate(firstStart)}, tagih dari ${formatDate(firstBill)}`,
    );
    for (const chip of [
      "Properti baru",
      "Unit baru",
      "Penyewa baru",
      "Sewa baru",
    ]) {
      await expect(one).toContainText(chip);
    }
    const two = importRow(page, 3);
    await expect(two).toContainText("Siap");
    await expect(two).toContainText(
      `Kamar A2 · ${property} · Per bulan · Rp1.350.000`,
    );
    await expect(two).toContainText(`Mulai ${formatDate(secondStart)}`);
    await expect(two).not.toContainText("tagih dari");
    // The second row finds the property the first one creates.
    await expect(two).not.toContainText("Properti baru");
    await expect(
      page.getByText("Juga akan dibuat: 1 properti, 2 unit, 2 penyewa."),
    ).toBeVisible();
    await expect(page.getByText("Peringatan untuk seluruh berkas")).toHaveCount(
      0,
    );
    await expect(
      page.getByRole("button", { name: "Perlu diperbaiki 0" }),
    ).toHaveCount(0);
    await attachViewportScreenshot(page, testInfo, "import-check-ready");

    const save = page.getByRole("button", { name: "Simpan 2 baris" });
    await expect(save).toBeEnabled();
    await expectTouchTarget(save);
    await save.click();

    const done = page.getByRole("status").filter({
      hasText: "Impor selesai: 2 baris disimpan",
    });
    await expect(done).toBeVisible();
    await expect(done).toContainText(`${fileName} sudah masuk ke ${GRIYA}.`);
    const created = page.getByRole("region", { name: "Yang dibuat" });
    for (const [label, count] of [
      ["Properti baru", "1"],
      ["Unit baru", "2"],
      ["Penyewa baru", "2"],
      ["Sewa baru", "2"],
      ["Sudah ada, tidak diubah", "0"],
    ] as const) {
      const term = created.getByText(label, { exact: true });
      await expect(term).toBeVisible();
      // The term and its count share one line.
      await expect(term.locator("..")).toHaveText(`${label}${count}`);
    }
    // The dry run and the commit, each once.
    expect(importRequests).toEqual(["POST", "POST"]);
    await attachViewportScreenshot(page, testInfo, "import-done");

    const toList = page.getByRole("link", { name: "Lihat daftar penyewa" });
    await expectTouchTarget(toList);
    await expectTouchTarget(
      page.getByRole("button", { name: "Impor berkas lain" }),
    );
    await toList.click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Penyewa" }),
    ).toBeVisible();
    const list = page.getByRole("list", { name: "Daftar penyewa" });
    const search = page.getByLabel("Cari nama, nomor, atau unit");
    await search.fill(first);
    await expect(
      list.getByRole("listitem").filter({ hasText: first }),
    ).toContainText(property);
    await search.fill(second);
    await expect(
      list.getByRole("listitem").filter({ hasText: second }),
    ).toContainText("Kamar A2");

    expect(outsideRequests).toEqual([]);
  });

  test("names a term the unit does not offer under jangka_bulan", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await openImport(page);
    const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1] ?? "";

    // The format card names the new column and what an empty cell means.
    await expect(
      page.getByText(
        "jangka_bulan: 1, 3, 6 atau 12. Kosongkan untuk 1 bulan, atau satu-satunya jangka yang ditawarkan unit.",
      ),
    ).toBeVisible();

    // A kos unit of the test's own that is rented by the month only.
    const property = uniqueName(testInfo, "Wisma Impor Jangka");
    const created = await page.request.post(`/api/orgs/${orgId}/properties`, {
      data: { name: property, kind: "kos", address: "" },
    });
    expect(created.status(), await created.text()).toBe(201);
    const { id: propertyId } = (await created.json()) as { id: string };
    const unit = await page.request.post(
      `/api/orgs/${orgId}/properties/${propertyId}/units`,
      { data: { label: "Kamar 1", terms: [{ months: 1, price: 1_250_000 }] } },
    );
    expect(unit.status(), await unit.text()).toBe(201);

    const header = [
      ...HEADER.slice(0, HEADER.indexOf("sewa") + 1),
      "jangka_bulan",
      ...HEADER.slice(HEADER.indexOf("sewa") + 1),
    ];
    await chooseFile(
      page,
      "penyewa-jangka.csv",
      csv(header, [
        [
          property,
          "kos",
          "Kamar 1",
          uniqueName(testInfo, "Wahyuni Jangka"),
          tenantNumber(testInfo, 7),
          "",
          "3",
          tenthOfMonth(1),
          "",
        ],
      ]),
    );
    await check(page);
    const row = importRow(page, 2);
    await expect(row).toContainText("Perlu diperbaiki");
    await expect(row).toContainText(
      "Kolom jangka_bulan: Unit ini tidak menawarkan jangka sewa 3 bulan.",
    );
    await expect(
      page.getByRole("button", { name: "Simpan 0 baris" }),
    ).toBeDisabled();

    expect(outsideRequests).toEqual([]);
  });

  test("refuses a file over 1 MB before sending it", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const importRequests = trackImportRequests(page);
    await openImport(page);

    await chooseFile(page, "besar.csv", Buffer.alloc(1_048_577, "a"));
    await expect(
      page.getByRole("alert").filter({ hasText: TOO_LARGE_MESSAGE }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Periksa berkas" }),
    ).toHaveCount(0);
    const other = page.getByRole("button", { name: "Pilih berkas lain" });
    await expect(other).toBeVisible();
    await expectTouchTarget(other);
    expect(importRequests).toEqual([]);

    // Another file clears it.
    await chooseFile(
      page,
      "kecil.csv",
      csv(HEADER, [
        [
          uniqueName(testInfo, "Wisma Impor Kecil"),
          "kos",
          "Kamar 1",
          uniqueName(testInfo, "Wulan Kecil"),
          tenantNumber(testInfo, 6),
          "1250000",
          tenthOfMonth(1),
          "",
        ],
      ]),
    );
    await expect(page.getByText(TOO_LARGE_MESSAGE)).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Periksa berkas" }),
    ).toBeEnabled();
    expect(importRequests).toEqual([]);

    expect(outsideRequests).toEqual([]);
  });

  test("shows the API's warning for a file without mulai_tagih, and a file it cannot read", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await openImport(page);

    const header = HEADER.filter((column) => column !== "mulai_tagih");
    await chooseFile(
      page,
      "tanpa-mulai-tagih.csv",
      csv(header, [
        [
          uniqueName(testInfo, "Wisma Impor Catatan"),
          "kos",
          "Kamar 1",
          uniqueName(testInfo, "Wulan Catatan"),
          tenantNumber(testInfo, 5),
          "1250000",
          tenthOfMonth(1),
        ],
      ]),
    );
    await check(page);
    const warnings = page.getByRole("region", {
      name: "Peringatan untuk seluruh berkas",
    });
    await expect(warnings).toContainText(NO_BILL_FROM_WARNING);

    // Back to step 1, with a file missing a required column: the API cannot read it.
    await page.getByRole("button", { name: "Pilih berkas lain" }).click();
    await chooseFile(
      page,
      "tanpa-no-wa.csv",
      csv(
        ["properti", "kamar", "nama_penyewa", "sewa", "tanggal_mulai"],
        [["A", "B", "C", "1", tenthOfMonth(1)]],
      ),
    );
    await page.getByRole("button", { name: "Periksa berkas" }).click();
    const unreadable = page
      .getByRole("alert")
      .filter({ hasText: "Berkas tidak bisa dibaca" });
    await expect(unreadable).toBeVisible();
    await expect(unreadable).toContainText(/\S+\./);
    await expect(
      page.getByRole("button", { name: "Pilih berkas lain" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Periksa berkas" }),
    ).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });

  test("the Supervisor of another organization sees no import", async ({
    page,
  }) => {
    const outsideRequests = trackOutsideRequests(page);
    await openTenants(page, HENDRA, ANGGREK);
    await expect(
      page.getByRole("link", { name: "Impor dari spreadsheet" }),
    ).toHaveCount(0);

    const importRequests = trackImportRequests(page);
    await page.goto(`${page.url()}/import`);
    await expect(
      page.getByText("Anda tidak punya akses untuk mengimpor penyewa."),
    ).toBeVisible();
    await expect(page.locator('input[type="file"]')).toHaveCount(0);
    expect(importRequests).toEqual([]);

    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Admin limited to one property", () => {
  test.use({ storageState: SARI.storageState });

  test("does not see the import, and its address shows a notice", async ({
    page,
  }) => {
    const outsideRequests = trackOutsideRequests(page);
    await openTenants(page, SARI, GRIYA);
    await expect(
      page.getByRole("link", { name: "Impor dari spreadsheet" }),
    ).toHaveCount(0);

    const importRequests = trackImportRequests(page);
    const templateRequests: string[] = [];
    page.on("request", (request) => {
      if (request.url().includes("/imports/"))
        templateRequests.push(request.url());
    });
    await page.goto(`${page.url()}/import`);
    await expect(
      page.getByText("Anda tidak punya akses untuk mengimpor penyewa."),
    ).toBeVisible();
    const back = page.getByRole("main").getByRole("link", { name: "Penyewa" });
    await expect(back).toBeVisible();
    await expectTouchTarget(back);
    await expect(
      page.getByRole("button", { name: "Periksa berkas" }),
    ).toHaveCount(0);
    expect(importRequests).toEqual([]);
    expect(templateRequests).toEqual([]);

    expect(outsideRequests).toEqual([]);
  });
});
