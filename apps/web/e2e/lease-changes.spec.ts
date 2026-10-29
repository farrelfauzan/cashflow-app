import {
  expect,
  test,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import type {
  CollectorCandidateList,
  Lease,
  Property,
  Tenant,
  Unit,
} from "../src/lib/api/generated/model";
import { formatDate, jakartaToday } from "../src/lib/format/date";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { ORG_HOME_URL } from "./support/shell";
import { HENDRA, SARI, type SeededStaff } from "./support/staff";

// "Ubah sewa", "Rencana keluar", "Catat pindah keluar" and "Batalkan sewa" on a lease's card,
// against the real API (seed-demo data). Pak Hendra is Admin of the whole of Griya Sejahtera and
// Supervisor in Kost Putri Anggrek; Mbak Sari is Admin limited to Kos Melati. Each test changes
// only rows it creates itself: its own property, unit, tenant and lease, with names and numbers no
// other run, project, test or retry uses. Tenant names start with W or Y, so they sort after every
// seeded tenant.

const GRIYA = "Griya Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const UNIT = "Kamar 1";
const LEASE_ENDED_MESSAGE = "Sewa ini sudah berakhir dan tidak bisa diubah.";
const LEASE_BUTTONS =
  /^(Ubah sewa|Rencana keluar|Catat pindah keluar|Batalkan sewa)$/;

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

// A tenant number no other run, project, test or retry uses; 0813-7 keeps it apart from the
// numbers of tenant-forms.spec.ts (0813-9).
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = testInfo.project.name === "phone" ? "2" : "1";
  return `0813-7${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
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
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
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
async function call<T>(
  page: Page,
  method: "GET" | "POST",
  path: string,
  want: number,
  data?: object,
): Promise<T> {
  const response = await page.request.fetch(`/api${path}`, { method, data });
  expect(response.status(), await response.text()).toBe(want);
  return (await response.json()) as T;
}

// A property and unit of the test's own, a tenant and their lease starting on `start`.
async function createLease(
  page: Page,
  orgId: string,
  testInfo: TestInfo,
  names: { property: string; tenant: string; testDigit: number },
  start: string,
) {
  const property = await call<Property>(
    page,
    "POST",
    `/orgs/${orgId}/properties`,
    201,
    { name: uniqueName(testInfo, names.property), kind: "kos", address: "" },
  );
  const unit = await call<Unit>(
    page,
    "POST",
    `/orgs/${orgId}/properties/${property.id}/units`,
    201,
    { label: UNIT, terms: [{ months: 1, price: 1_250_000 }] },
  );
  const tenant = await call<Tenant>(
    page,
    "POST",
    `/orgs/${orgId}/tenants`,
    201,
    {
      name: uniqueName(testInfo, names.tenant),
      phone: tenantNumber(testInfo, names.testDigit),
      note: "",
    },
  );
  const lease = await call<Lease>(page, "POST", `/orgs/${orgId}/leases`, 201, {
    tenantId: tenant.id,
    unitId: unit.id,
    startDate: start,
    termMonths: 1,
    depositAmount: 2_500_000,
  });
  return { property, tenant, lease };
}

// Opens Pak Hendra's Griya Sejahtera and a lease of the test's own on its tenant's page.
async function openOwnLease(
  page: Page,
  testInfo: TestInfo,
  names: { property: string; tenant: string; testDigit: number },
  start: string,
) {
  await openTenants(page, HENDRA, GRIYA);
  const orgId = orgIdOf(page);
  const created = await createLease(page, orgId, testInfo, names, start);
  await openTenantPage(page, orgId, created.tenant);
  const card = page.getByRole("region", {
    name: `${UNIT} · ${created.property.name}`,
  });
  await expect(card).toBeVisible();
  return { orgId, card, ...created };
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: HENDRA.storageState });

  test("changes a lease's deposit and collector", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const { orgId, card, property, tenant } = await openOwnLease(
      page,
      testInfo,
      { property: "Kos Uji Ubah", tenant: "Wening Pratiwi", testDigit: 1 },
      jakartaToday(new Date()),
    );
    const { candidates } = await call<CollectorCandidateList>(
      page,
      "GET",
      `/orgs/${orgId}/collector-candidates`,
      200,
    );
    const collector = candidates[0]?.name;
    if (collector === undefined) throw new Error("Griya has no candidates");

    const edit = page.getByRole("button", { name: "Ubah sewa" });
    await expectTouchTarget(edit);
    await expectTouchTarget(
      card.getByRole("button", { name: "Rencana keluar" }),
    );
    await expectTouchTarget(
      card.getByRole("button", { name: "Catat pindah keluar" }),
    );
    await expect(
      card.getByRole("button", { name: "Batalkan sewa" }),
    ).toHaveCount(0);
    await edit.click();

    const form = page.getByRole("dialog", { name: "Ubah sewa" });
    await expect(form).toContainText(
      `${tenant.name} · ${UNIT} · ${property.name}`,
    );
    await expect(form).toContainText(`Unit: ${UNIT} · ${property.name}`);
    await expect(form).toContainText(
      "Unit dan penyewa tidak bisa diganti. Untuk pindah unit, catat pindah keluar, lalu buat sewa baru.",
    );
    await expect(form.getByLabel("Uang jaminan")).toHaveValue("2.500.000");
    await expect(form.getByRole("combobox", { name: "Penagih" })).toHaveText(
      "Tidak ada",
    );
    // A lease entered on its start date: no "Mulai tagih dari".
    await expect(
      form.getByRole("group", { name: "Mulai tagih dari" }),
    ).toHaveCount(0);

    await form.getByLabel("Uang jaminan").fill("3000000");
    await expect(form.getByLabel("Uang jaminan")).toHaveValue("3.000.000");
    await choose(page, form, "Penagih", collector);
    const min = MIN_BODY_FONT_PX[testInfo.project.name] ?? 16;
    expect(
      await fontSize(form.getByText("Isi 0 kalau tidak ada.")),
    ).toBeGreaterThanOrEqual(min);
    await expectTouchTarget(form.getByRole("button", { name: "Simpan sewa" }));
    await attachViewportScreenshot(page, testInfo, "lease-edit");

    await form.getByRole("button", { name: "Simpan sewa" }).click();
    await expect(form).toHaveCount(0);
    await expect(visibleText(page, `Sewa ${UNIT} diperbarui.`)).toBeVisible();
    await expect(card).toContainText("Rp3.000.000");
    await expect(card).toContainText(collector);

    expect(outsideRequests).toEqual([]);
  });

  test("sets a planned end, then clears it", async ({ page }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const { card, property, tenant } = await openOwnLease(
      page,
      testInfo,
      { property: "Kos Uji Rencana", tenant: "Wahyuni Lestari", testDigit: 2 },
      tenthOfMonth(-1),
    );
    await expect(card).toContainText("Belum diatur");

    await card.getByRole("button", { name: "Rencana keluar" }).click();
    const form = page.getByRole("dialog", { name: "Rencana keluar" });
    await expect(form).toContainText(
      `${tenant.name} · ${UNIT} · ${property.name}`,
    );
    const date = form.getByLabel("Hari terakhir penyewa tinggal");
    // The suggestion: the day before the next period starts (the 9th, the lease starts on a 10th).
    await expect(date).toHaveValue(/^\d{4}-\d{2}-09$/);
    await expect(form).toContainText("Usulan: ");
    await expect(form).toContainText("Yang terjadi setelah ini");
    await expect(
      form.getByRole("button", { name: "Hapus rencana keluar" }),
    ).toHaveCount(0);

    // A day before today: the form says so before anything is sent.
    await date.fill(tenthOfMonth(-1));
    await form.getByRole("button", { name: "Simpan rencana keluar" }).click();
    await expect(
      form.getByText("Rencana keluar tidak boleh sebelum hari ini."),
    ).toBeVisible();
    await expect(date).toBeFocused();

    const planned = tenthOfMonth(1);
    await date.fill(planned);
    await form.getByRole("button", { name: "Simpan rencana keluar" }).click();
    await expect(form).toHaveCount(0);
    await expect(
      visibleText(page, `Rencana keluar ${UNIT}: ${formatDate(planned)}.`),
    ).toBeVisible();
    await expect(card).toContainText(formatDate(planned));

    // Opened again: the current date, and the way to clear it.
    await card.getByRole("button", { name: "Rencana keluar" }).click();
    await expect(form).toBeVisible();
    await expect(date).toHaveValue(planned);
    await expect(form).toContainText(
      `Rencana keluar saat ini: ${formatDate(planned)}. Menghapusnya mengembalikan tagihan yang tadi dibatalkan.`,
    );
    const clear = form.getByRole("button", { name: "Hapus rencana keluar" });
    await expectTouchTarget(clear);
    await attachViewportScreenshot(page, testInfo, "lease-planned-end");
    await clear.click();
    await expect(form).toHaveCount(0);
    await expect(
      visibleText(page, `Rencana keluar ${UNIT} dihapus.`),
    ).toBeVisible();
    await expect(card).toContainText("Belum diatur");

    expect(outsideRequests).toEqual([]);
  });

  test("records a move-out, and the unit can be rented again", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const { card, lease } = await openOwnLease(
      page,
      testInfo,
      { property: "Kos Uji Pindah", tenant: "Yohana Sitompul", testDigit: 3 },
      tenthOfMonth(-1),
    );
    await expect(
      page.getByRole("button", { name: "Sewakan unit" }),
    ).toHaveCount(0);

    await card.getByRole("button", { name: "Catat pindah keluar" }).click();
    const form = page.getByRole("dialog", { name: "Catat pindah keluar" });
    await expect(form.getByLabel("Hari terakhir menyewa")).toHaveValue(
      jakartaToday(new Date()),
    );
    await expect(form).toContainText(
      `Diisi hari ini. Tidak boleh sebelum tanggal mulai sewa (${formatDate(lease.startDate)}) dan tidak boleh setelah hari ini.`,
    );
    await expect(form).toContainText(
      "Unit langsung kosong dan bisa disewakan lagi.",
    );
    const save = form.getByRole("button", { name: "Catat pindah keluar" });
    await expectTouchTarget(save);
    await attachViewportScreenshot(page, testInfo, "lease-move-out");

    await save.click();
    await expect(form).toHaveCount(0);
    await expect(
      visibleText(page, `Pindah keluar dicatat. ${UNIT} sekarang kosong.`),
    ).toBeVisible();
    await expect(card).toContainText("Sudah keluar");
    await expect(card.getByRole("button")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Ubah sewa" })).toHaveCount(
      0,
    );
    await expect(
      page.getByRole("button", { name: "Sewakan unit" }),
    ).toBeVisible();

    expect(outsideRequests).toEqual([]);
  });

  test("cancels a booking", async ({ page }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const start = tenthOfMonth(2);
    const { card } = await openOwnLease(
      page,
      testInfo,
      { property: "Kos Uji Batal", tenant: "Yudha Prakoso", testDigit: 4 },
      start,
    );
    await expect(card).toContainText("Belum mulai");
    await expect(
      card.getByRole("button", { name: "Catat pindah keluar" }),
    ).toHaveCount(0);
    const cancel = card.getByRole("button", { name: "Batalkan sewa" });
    await expectTouchTarget(cancel);
    await cancel.click();

    const form = page.getByRole("dialog", { name: "Batalkan sewa" });
    await expect(form).toContainText(
      `Sewa ini belum dimulai (mulai ${formatDate(start)}). Kalau dibatalkan, unit langsung kosong lagi dan sewa ini tidak akan pernah ditagih.`,
    );
    await expect(form.getByLabel("Tanggal berakhir")).toHaveValue(
      formatDate(start),
    );
    await expect(form).toContainText(
      "Diisi otomatis sama dengan tanggal mulai.",
    );
    await expectTouchTarget(
      form.getByRole("button", { name: "Jangan batalkan" }),
    );
    // The user's choice: cancelling a booking looks destructive (the theme's destructive color).
    const confirm = form.getByRole("button", { name: "Batalkan sewa" });
    const colors = await confirm.evaluate((button) => {
      const probe = document.createElement("div");
      probe.className = "bg-destructive";
      document.body.append(probe);
      const want = getComputedStyle(probe).backgroundColor;
      probe.remove();
      return { got: getComputedStyle(button).backgroundColor, want };
    });
    expect(colors.got).toBe(colors.want);
    await attachViewportScreenshot(page, testInfo, "lease-cancel-booking");

    await confirm.click();
    await expect(form).toHaveCount(0);
    await expect(
      visibleText(page, `Sewa ${UNIT} dibatalkan. Unitnya kosong lagi.`),
    ).toBeVisible();
    await expect(card).toContainText("Sudah keluar");
    await expect(
      page.getByRole("button", { name: "Sewakan unit" }),
    ).toBeVisible();

    expect(outsideRequests).toEqual([]);
  });

  test("shows the API's refusal when the lease ended meanwhile", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    const today = jakartaToday(new Date());
    const { orgId, lease } = await openOwnLease(
      page,
      testInfo,
      { property: "Kos Uji Selesai", tenant: "Winarti Susanti", testDigit: 5 },
      today,
    );

    await page.getByRole("button", { name: "Ubah sewa" }).click();
    const form = page.getByRole("dialog", { name: "Ubah sewa" });
    await form.getByLabel("Uang jaminan").fill("3000000");

    // Someone else records the move-out before this form is saved.
    await call<Lease>(
      page,
      "POST",
      `/orgs/${orgId}/leases/${lease.id}/move-out`,
      200,
      { endDate: today },
    );
    await form.getByRole("button", { name: "Simpan sewa" }).click();
    await expect(form.getByRole("alert")).toContainText(LEASE_ENDED_MESSAGE);
    await form.getByRole("button", { name: "Batal" }).click();
    await expect(form).toHaveCount(0);

    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Admin limited to one property", () => {
  test.use({ storageState: SARI.storageState });

  test("sees the lease buttons on her property's tenant", async ({ page }) => {
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
    await expect(page.getByRole("button", { name: "Ubah sewa" })).toBeVisible();
    await expect(
      card.getByRole("button", { name: "Rencana keluar" }),
    ).toBeVisible();
    await expect(
      card.getByRole("button", { name: "Catat pindah keluar" }),
    ).toBeVisible();

    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("sees no button that changes a lease", async ({ page }) => {
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
    await expect(page.getByRole("button", { name: LEASE_BUTTONS })).toHaveCount(
      0,
    );

    expect(outsideRequests).toEqual([]);
  });
});
