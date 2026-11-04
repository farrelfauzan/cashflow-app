import {
  expect,
  test,
  type Locator,
  type Page,
  type Request,
  type TestInfo,
} from "@playwright/test";

import type {
  ChargeType,
  ChargeTypeList,
  Lease,
  MeteredCharge,
  MeteredChargeInput,
  MeteredChargeList,
  Tenant,
  TenantList,
  Unit,
} from "../src/lib/api/generated/model";
import { formatMonthYear, jakartaToday } from "../src/lib/format/date";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { findOrCreateProperty } from "./support/property-fixtures";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA } from "./support/staff";

// "Catat meteran" (W14) on Detail penyewa, against the real API and its worker. Each test makes
// its own kos unit, tenant and lease, started on the 1st of last month in Jakarta, expecting
// Listrik and Air from a meter. Every amount and date checked is read from the API, never
// hard-coded. Pak Hendra is Admin of the whole of Griya Sejahtera (charge:record) and Supervisor
// in Kost Putri Anggrek (no charge:record).

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const PANEL = "Catat meteran";
const REGION = "Uang penyewa";

// Minimum body text per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };

function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

// A name no other run, project or retry uses.
function uniqueName(testInfo: TestInfo, base: string): string {
  const project = testInfo.project.name === "phone" ? "Dua" : "Satu";
  const retry = testInfo.retry === 0 ? "" : ` r${testInfo.retry}`;
  return `${base} ${runId()} ${project}${retry}`;
}

// A tenant number no other run, project, test or retry uses; 0814-5 is used by no other spec.
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = testInfo.project.name === "phone" ? "2" : "1";
  return `0814-5${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
}

// Months in Jakarta, YYYY-MM.
function monthFrom(offset: number): string {
  const [year = 0, month = 1] = jakartaToday(new Date()).split("-").map(Number);
  return new Date(Date.UTC(year, month - 1 + offset, 1))
    .toISOString()
    .slice(0, 7);
}

const monthText = (month: string) => formatMonthYear(`${month}-01`);

async function call<T>(
  page: Page,
  method: "GET" | "POST" | "PUT",
  path: string,
  want: number,
  data?: object,
): Promise<T> {
  const response = await page.request.fetch(`/api${path}`, { method, data });
  expect(response.status(), await response.text()).toBe(want);
  return (await response.json()) as T;
}

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

// The organization's metered type of this name, made when the organization lacks it.
async function meteredType(
  page: Page,
  orgId: string,
  name: string,
): Promise<ChargeType> {
  const find = async () =>
    (
      await call<ChargeTypeList>(
        page,
        "GET",
        `/orgs/${orgId}/charge-types`,
        200,
      )
    ).chargeTypes.find(
      (type) => type.metered && type.name.toLowerCase() === name.toLowerCase(),
    );
  const existing = await find();
  if (existing !== undefined) return existing;
  const response = await page.request.post(`/api/orgs/${orgId}/charge-types`, {
    data: { name, metered: true },
  });
  if (response.status() === 201) return (await response.json()) as ChargeType;
  const raced = await find();
  if (raced === undefined) throw new Error(`no metered type ${name}`);
  return raced;
}

type Fixture = {
  tenant: Tenant;
  lease: Lease;
  listrik: ChargeType;
  air: ChargeType;
  metersPath: string;
};

// A tenant renting a kos unit from the 1st of last month, expecting Listrik and Air. By default
// it waits until the worker has made the lease's invoices, so an amount recorded afterwards goes
// on a later one. With `billNextMonth` the lease is billed only from next month, so nothing is
// invoiced yet.
async function makeLease(
  page: Page,
  testInfo: TestInfo,
  orgId: string,
  options: { digit: number; name: string; billNextMonth?: boolean },
): Promise<Fixture> {
  const [listrik, air] = [
    await meteredType(page, orgId, "Listrik"),
    await meteredType(page, orgId, "Air"),
  ];
  const tenant = await call<Tenant>(
    page,
    "POST",
    `/orgs/${orgId}/tenants`,
    201,
    {
      name: uniqueName(testInfo, options.name),
      phone: tenantNumber(testInfo, options.digit),
      note: "",
    },
  );
  const property = await findOrCreateProperty(page, orgId, testInfo, {
    base: "Catat Meteran",
    kind: "kos",
  });
  const unit = await call<Unit>(
    page,
    "POST",
    `/orgs/${orgId}/properties/${property.id}/units`,
    201,
    {
      label: uniqueName(testInfo, `Kamar Catat ${options.digit}`),
      terms: [{ months: 1, price: 1_000_000 }],
    },
  );
  const startDate = `${monthFrom(-1)}-01`;
  const created = await call<Lease>(
    page,
    "POST",
    `/orgs/${orgId}/leases`,
    201,
    {
      tenantId: tenant.id,
      unitId: unit.id,
      startDate,
      termMonths: 1,
      depositAmount: 0,
      ...(options.billNextMonth === true
        ? { billedFrom: `${monthFrom(1)}-01` }
        : {}),
    },
  );
  const lease = await call<Lease>(
    page,
    "PUT",
    `/orgs/${orgId}/leases/${created.id}/charges`,
    200,
    { charges: [], meteredChargeTypeIds: [listrik.id, air.id] },
  );
  const metersPath = `/orgs/${orgId}/leases/${lease.id}/metered-charges`;
  if (options.billNextMonth !== true) {
    // The next invoice is this month's until the worker has made it.
    await expect
      .poll(
        async () => {
          const { nextInvoice } = await call<MeteredChargeList>(
            page,
            "GET",
            metersPath,
            200,
          );
          return (nextInvoice?.period ?? "") > `${monthFrom(0)}-01`;
        },
        { timeout: 20_000 },
      )
      .toBe(true);
  }
  return { tenant, lease, listrik, air, metersPath };
}

async function openTenantPage(page: Page, orgId: string, tenant: Tenant) {
  await page.goto(`/o/${orgId}/tenants/${tenant.id}`);
  await expect(
    page.getByRole("heading", { level: 1, name: tenant.name }),
  ).toBeVisible();
  await expect(page.getByRole("region", { name: REGION })).toBeVisible();
}

async function openPanel(page: Page): Promise<Locator> {
  await page.getByRole("button", { name: PANEL }).click();
  const panel = page.getByRole("dialog", { name: PANEL });
  await expect(panel).toBeVisible();
  return panel;
}

function panelParts(panel: Locator) {
  return {
    month: panel.getByRole("combobox", { name: "Bulan pemakaian" }),
    start: panel.getByRole("textbox", { name: "Awal", exact: true }),
    end: panel.getByRole("textbox", { name: "Akhir", exact: true }),
    amount: panel.getByRole("textbox", { name: "Jumlah tagihan" }),
    recorded: panel.getByRole("list", { name: "Sudah dicatat" }),
  };
}

function isMeterPut(request: Request): boolean {
  return (
    request.method() === "PUT" &&
    /\/api\/orgs\/[^/]+\/leases\/[^/]+\/metered-charges\/[^/]+\/[^/]+$/.test(
      request.url(),
    )
  );
}

async function expectTouchTarget(locator: Locator, min = 44) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(min);
}

async function fontSize(locator: Locator): Promise<number> {
  return locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("records Listrik for last month and stays open for the next reading", async ({
    page,
  }, testInfo) => {
    test.setTimeout(90_000);
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const { tenant, lease, listrik, metersPath } = await makeLease(
      page,
      testInfo,
      orgId,
      { digit: 1, name: "Wahyu Kurniawan" },
    );
    const before = await call<MeteredChargeList>(page, "GET", metersPath, 200);
    const next = before.nextInvoice;
    if (next === undefined) throw new Error("no next invoice");
    await openTenantPage(page, orgId, tenant);

    // The card names the metered types the lease expects, in the API's order.
    await expect(
      page.getByText(
        `${lease.meteredChargeTypes.map((type) => type.name).join(", ")} dari meteran`,
      ),
    ).toBeVisible();
    // A secondary action: "Catat pembayaran" stays the page's one primary button.
    const open = page.getByRole("button", { name: PANEL });
    await expectTouchTarget(open);
    const panel = await openPanel(page);
    await expect(panel).toContainText(
      `${tenant.name} · ${lease.unitLabel} · ${lease.propertyName}`,
    );
    const parts = panelParts(panel);
    const lastMonth = monthFrom(-1);
    // Defaults: last month (the lease's start month), the first metered type.
    await expect(parts.month).toContainText(monthText(lastMonth));
    await expect(panel.getByText("Biasanya bulan lalu.")).toBeVisible();
    const [firstType] = lease.meteredChargeTypes;
    if (firstType === undefined) throw new Error("no metered type");
    await expect(
      panel.getByRole("radio", { name: firstType.name }),
    ).toBeChecked();
    await panel.getByRole("radio", { name: "Listrik" }).click();
    await expect(panel.getByRole("radio", { name: "Listrik" })).toBeChecked();
    await expect(panel).toContainText(
      `Masuk ke tagihan berikutnya yang terbit setelah ini dicatat: Tagihan ${formatMonthYear(next.period)}`,
    );
    await expect(panel).toContainText("Belum ada meteran yang dicatat.");

    await parts.start.fill("1200");
    await parts.end.fill("1350");
    await parts.amount.fill("123000");
    await expect(parts.amount).toHaveValue("123.000");

    const minFont = MIN_BODY_FONT_PX[testInfo.project.name] ?? 16;
    expect(
      await fontSize(
        panel.getByText(
          "Hanya untuk catatan. Yang ditagihkan adalah jumlah di bawah.",
        ),
      ),
    ).toBeGreaterThanOrEqual(minFont);
    expect(
      await fontSize(panel.getByText(/Masuk ke tagihan berikutnya/)),
    ).toBeGreaterThanOrEqual(minFont);
    for (const name of ["Listrik", "Air"]) {
      await expectTouchTarget(
        panel.locator("label").filter({ hasText: name }),
        56,
      );
    }
    await expectTouchTarget(parts.month);
    await expectTouchTarget(parts.start);
    await expectTouchTarget(panel.getByRole("button", { name: "Batal" }));
    await expectTouchTarget(
      panel.getByRole("button", { name: "Simpan meteran" }),
    );
    await panel.locator("form").evaluate((form) => form.scrollTo(0, 0));
    await attachViewportScreenshot(page, testInfo, "meter-form");

    const answer = page.waitForResponse((response) =>
      isMeterPut(response.request()),
    );
    await panel.getByRole("button", { name: "Simpan meteran" }).click();
    const response = await answer;
    expect(response.status()).toBe(200);
    expect(new URL(response.url()).pathname).toBe(
      `/api/orgs/${orgId}/leases/${lease.id}/metered-charges/${listrik.id}/${lastMonth}`,
    );
    expect(response.request().postDataJSON() as MeteredChargeInput).toEqual({
      amount: 123_000,
      meterStart: 1200,
      meterEnd: 1350,
    });
    const saved = (await response.json()) as MeteredCharge;
    expect(saved.billed).toBeUndefined();

    // The panel stays open, says what was saved and where it goes, and clears the fields.
    await expect(panel).toContainText(
      `Listrik ${monthText(lastMonth)} ${formatRupiah(saved.amount)} tersimpan. Masuk Tagihan ${formatMonthYear(next.period)}.`,
    );
    await expect(parts.amount).toHaveValue("");
    await expect(parts.start).toHaveValue("");
    await expect(parts.end).toHaveValue("");
    const rows = parts.recorded.getByRole("listitem");
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toContainText(
      `Listrik · ${monthText(lastMonth)} · ${formatRupiah(saved.amount)}`,
    );
    await expect(rows.first()).toContainText("Belum ditagihkan");
    await expectTouchTarget(
      panel.getByRole("button", {
        name: `Ubah Listrik ${monthText(lastMonth)}`,
      }),
    );
    await expectTouchTarget(panel.getByRole("button", { name: "Selesai" }));
    await attachViewportScreenshot(page, testInfo, "meter-saved");

    await panel.getByRole("button", { name: "Selesai" }).click();
    await expect(panel).toHaveCount(0);
    expect(outsideRequests).toEqual([]);
  });

  test("replaces an amount recorded but not billed", async ({
    page,
  }, testInfo) => {
    test.setTimeout(90_000);
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const { tenant, listrik, metersPath } = await makeLease(
      page,
      testInfo,
      orgId,
      { digit: 2, name: "Yuliana Hapsari" },
    );
    const lastMonth = monthFrom(-1);
    await call<MeteredCharge>(
      page,
      "PUT",
      `${metersPath}/${listrik.id}/${lastMonth}`,
      200,
      { amount: 100_000, meterStart: 40, meterEnd: 90 },
    );
    await openTenantPage(page, orgId, tenant);
    const panel = await openPanel(page);
    const parts = panelParts(panel);
    await panel.getByRole("radio", { name: "Listrik" }).click();

    const warning = `Listrik ${monthText(lastMonth)} sudah dicatat Rp100.000 oleh ${HENDRA.name}. Menyimpan akan mengganti jumlahnya.`;
    await expect(panel).toContainText(warning);
    await expect(parts.amount).toHaveValue("100.000");
    await expect(parts.start).toHaveValue("40");
    await expect(parts.end).toHaveValue("90");
    await expect(
      panel.getByRole("button", { name: "Simpan perubahan" }),
    ).toBeVisible();

    // Air has nothing yet: no warning, empty fields, "Simpan meteran".
    await panel.getByRole("radio", { name: "Air" }).click();
    await expect(panel).not.toContainText(warning);
    await expect(parts.amount).toHaveValue("");
    await expect(
      panel.getByRole("button", { name: "Simpan meteran" }),
    ).toBeVisible();

    // "Ubah" on the Listrik row goes back to it, prefilled.
    await panel
      .getByRole("button", { name: `Ubah Listrik ${monthText(lastMonth)}` })
      .click();
    await expect(panel.getByRole("radio", { name: "Listrik" })).toBeChecked();
    await expect(parts.amount).toHaveValue("100.000");

    await parts.amount.fill("150000");
    const answer = page.waitForResponse((response) =>
      isMeterPut(response.request()),
    );
    await panel.getByRole("button", { name: "Simpan perubahan" }).click();
    expect((await answer).status()).toBe(200);
    await expect(panel).toContainText(
      `Listrik ${monthText(lastMonth)} Rp150.000 tersimpan.`,
    );
    const rows = parts.recorded.getByRole("listitem");
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toContainText("Rp150.000");

    const after = await call<MeteredChargeList>(page, "GET", metersPath, 200);
    expect(after.items).toHaveLength(1);
    expect(after.items[0]?.amount).toBe(150_000);
    expect(outsideRequests).toEqual([]);
  });

  test("stops an empty amount, zero and an end below the start before sending", async ({
    page,
  }, testInfo) => {
    test.setTimeout(90_000);
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const { tenant } = await makeLease(page, testInfo, orgId, {
      digit: 3,
      name: "Zulkifli Hasan",
    });
    const puts: string[] = [];
    page.on("request", (request) => {
      if (isMeterPut(request)) puts.push(request.url());
    });
    await openTenantPage(page, orgId, tenant);
    const panel = await openPanel(page);
    const parts = panelParts(panel);
    const save = panel.getByRole("button", { name: "Simpan meteran" });

    await save.click();
    await expect(panel.getByText("Isi jumlah tagihan.")).toBeVisible();
    await expect(parts.amount).toBeFocused();

    await parts.amount.fill("0");
    await expect(panel.getByText("Isi jumlah tagihan.")).toHaveCount(0);
    await save.click();
    await expect(panel.getByText("Jumlah harus lebih dari Rp0.")).toBeVisible();

    await parts.amount.fill("50000");
    // Letters never reach the meter numbers.
    await parts.start.fill("2a00");
    await expect(parts.start).toHaveValue("200");
    await parts.end.fill("150");
    await save.click();
    await expect(
      panel.getByText("Angka akhir tidak boleh lebih kecil dari angka awal."),
    ).toBeVisible();
    await expect(parts.end).toHaveAttribute("aria-invalid", "true");

    expect(puts).toEqual([]);
    await panel.getByRole("button", { name: "Batal" }).click();
    await expect(panel).toHaveCount(0);
    expect(puts).toEqual([]);
    expect(outsideRequests).toEqual([]);
  });

  test("shows a billed amount locked, with the invoice to correct", async ({
    page,
  }, testInfo) => {
    test.setTimeout(120_000);
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, GRIYA);
    const { tenant, lease, listrik, metersPath } = await makeLease(
      page,
      testInfo,
      orgId,
      { digit: 4, name: "Xaverius Gunadi", billNextMonth: true },
    );
    const lastMonth = monthFrom(-1);
    await call<MeteredCharge>(
      page,
      "PUT",
      `${metersPath}/${listrik.id}/${lastMonth}`,
      200,
      { amount: 87_500, meterStart: 100, meterEnd: 180 },
    );
    // Billing from the start date again makes the worker invoice the past periods now, and the
    // first invoice made bills the amount recorded before it.
    await call<Lease>(page, "PUT", `/orgs/${orgId}/leases/${lease.id}`, 200, {
      startDate: lease.startDate,
      depositAmount: 0,
      billedFrom: lease.startDate,
    });
    let billed: MeteredCharge | undefined;
    await expect
      .poll(
        async () => {
          const list = await call<MeteredChargeList>(
            page,
            "GET",
            metersPath,
            200,
          );
          billed = list.items.find((each) => each.billed !== undefined);
          return billed !== undefined;
        },
        { timeout: 30_000 },
      )
      .toBe(true);
    const invoicePeriod = billed?.billed?.period;
    if (invoicePeriod === undefined) throw new Error("not billed");
    const invoiceMonth = formatMonthYear(invoicePeriod);

    await openTenantPage(page, orgId, tenant);
    const panel = await openPanel(page);
    // Last month's Listrik is billed: choosing it shows the locked view instead of the form.
    await panel.getByRole("radio", { name: "Listrik" }).click();
    await expect(
      panel.getByRole("heading", { name: `Listrik · ${monthText(lastMonth)}` }),
    ).toBeVisible();
    await expect(panel).toContainText("Sudah ditagihkan, terkunci");
    await expect(panel).toContainText(`Jumlah${formatRupiah(87_500)}`);
    await expect(panel).toContainText("Angka awal100");
    await expect(panel).toContainText("Angka akhir180");
    await expect(panel).toContainText(`Dicatat${HENDRA.name} · `);
    await expect(panel).toContainText(
      `Jumlah ini sudah masuk Tagihan ${invoiceMonth} dan tidak bisa diubah lagi. Kalau salah, buat koreksi pada tagihan itu dengan alasannya.`,
    );
    await expect(
      panel.getByRole("textbox", { name: "Jumlah tagihan" }),
    ).toHaveCount(0);
    const correct = panel.getByRole("button", {
      name: `Koreksi tagihan ${invoiceMonth}`,
    });
    await expectTouchTarget(correct);
    await expectTouchTarget(panel.getByRole("button", { name: "Kembali" }));
    await attachViewportScreenshot(page, testInfo, "meter-locked");

    // "Kembali" goes back to the form, which says the month is locked; the row opens it again.
    await panel.getByRole("button", { name: "Kembali" }).click();
    const parts = panelParts(panel);
    await expect(parts.amount).toBeVisible();
    await expect(panel).toContainText(
      `Listrik ${monthText(lastMonth)} sudah ditagihkan dan terkunci.`,
    );
    const row = parts.recorded.getByRole("listitem").first();
    await expect(row).toContainText("Sudah ditagihkan, terkunci");
    await row.getByRole("button").click();
    await expect(correct).toBeVisible();

    // "Koreksi tagihan …" closes the panel and opens the correction dialog for that invoice.
    await correct.click();
    await expect(page.getByRole("dialog", { name: PANEL })).toHaveCount(0);
    const dialog = page.getByRole("dialog", {
      name: `Koreksi tagihan ${invoiceMonth}`,
    });
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("Nominal sekarang");
    await expect(
      dialog.getByRole("textbox", { name: "Nominal baru" }),
    ).toBeFocused();
    await dialog.getByRole("button", { name: "Batal" }).click();
    await expect(dialog).toHaveCount(0);
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees no Catat meteran", async ({ page }) => {
    const outsideRequests = trackOutsideRequests(page);
    const orgId = await openOrganization(page, ANGGREK);
    const { items } = await call<TenantList>(
      page,
      "GET",
      `/orgs/${orgId}/tenants?q=${encodeURIComponent("Nadia Putri")}`,
      200,
    );
    const nadia = items.find((each) => each.name === "Nadia Putri");
    if (nadia === undefined) throw new Error("Nadia Putri is not seeded");
    await page.goto(`/o/${orgId}/tenants/${nadia.id}`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Nadia Putri" }),
    ).toBeVisible();
    await expect(page.getByRole("region", { name: REGION })).toBeVisible();
    await expect(page.getByRole("button", { name: PANEL })).toHaveCount(0);
    expect(outsideRequests).toEqual([]);
  });
});
