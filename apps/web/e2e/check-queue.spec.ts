import {
  expect,
  test,
  type APIRequestContext,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import {
  DEPOSIT_GAP_NOTE_LABEL,
  DEPOSIT_GAP_SUBMIT,
  DEPOSIT_GAP_TITLE,
  EMPTY_TEXT,
  FILTERS_LABEL,
  FORBIDDEN_TEXT,
  LIST_LABEL,
  NOTE_EMPTY_MESSAGE,
  PHONE_APPLY,
  PHONE_BACK,
  PHONE_REASON_LABEL,
  PHONE_REJECT,
  PHONE_REJECT_SUBMIT,
  PHONE_REJECT_TITLE,
  PHONE_TITLE,
  REASON_EMPTY_MESSAGE,
  SUBTITLE,
} from "../src/components/client/check-queue/utils";
import type {
  CashDeposit,
  CashDepositInput,
  CashPaymentInput,
  CashPaymentResult,
  CashTotal,
  CheckQueuePage,
  CheckResolution,
  FieldVisitList,
  OrgMeResponse,
  Tenant,
  TenantList,
  VisitOutcome,
  VisitOutcomeInput,
} from "../src/lib/api/generated/model";
import { formatPhone } from "../src/lib/format/phone";
import { formatRupiah } from "../src/lib/format/rupiah";
import {
  attachFullPageScreenshot,
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { minBodyFontPx, projectName } from "./support/projects";
import { ORG_HOME_URL } from "./support/shell";
import { DIMAS, HENDRA, KURNIA, SARI, type SeededStaff } from "./support/staff";
import { expectToast } from "./support/toast";

// "Antrean cek" (boards W21), against the real API. Pak Dimas is Admin of the whole of Kos Bulanan
// Sejahtera (payment:check for the whole organization), so he answers the items. Mas Kurnia is a
// Karyawan there, collector of Qori Amalia and Rizki Ananda; only this spec records his cash, his
// deposits and his visit outcomes. Pak Hendra is Supervisor of Kost Putri Anggrek (cash:read, no
// payment:check), Mbak Sari an Admin limited to Kos Melati in Griya Sejahtera (neither for the
// whole organization).
//
// Items are created through the API: a deposit gap (Mas Kurnia deposits, Pak Dimas counts it
// short) and new numbers (Mas Kurnia's number_changed outcomes). Only the iPad project creates
// and answers them, so the projects never race one karyawan's cash or one tenant's number. An
// infected upload and a QRIS mismatch cannot be made on this stack (no uploads, and the AutoGoPay
// stand-in pays at the QR's amount); their rows and drawers are covered by the unit tests.

const BULANAN = "Kos Bulanan Sejahtera";
const ANGGREK = "Kost Putri Anggrek";
const GRIYA = "Griya Sejahtera";
const CASH_TENANT = "Qori Amalia";
const PHONE_TENANT = "Rizki Ananda";
const OTHER_TENANT = "Qori Amalia";
const TITLE = "Antrean cek";
const QUEUE_URL = /\/o\/[0-9a-f-]{36}\/billing\/check-queue(\?.*)?$/;
const ACTION_BUTTONS =
  /^(Tutup selisih|Tandai sudah ditangani|Tandai sudah dicek|Pakai nomor baru|Jangan pakai)$/;

function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

function orgIdOf(page: Page): string {
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");
  return orgId;
}

async function openOrganization(page: Page, staff: SeededStaff, name: string) {
  await page.goto("/organizations");
  if (staff.organizations.length > 1) {
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name })
      .click();
  }
  await expect(page).toHaveURL(ORG_HOME_URL);
}

// Setup through the API, as a signed-in member.
async function call<T>(
  request: APIRequestContext,
  method: "GET" | "POST",
  path: string,
  want: number,
  data?: object,
): Promise<T> {
  const response = await request.fetch(`/api${path}`, { method, data });
  expect(response.status(), await response.text()).toBe(want);
  return (await response.json()) as T;
}

async function access(page: Page, orgId: string, action: string) {
  const me = await call<OrgMeResponse>(
    page.request,
    "GET",
    `/orgs/${orgId}/me`,
    200,
  );
  return me.capabilities.find((capability) => capability.action === action)
    ?.access;
}

async function tenantNamed(
  request: APIRequestContext,
  orgId: string,
  name: string,
): Promise<Tenant> {
  const tenants = await call<TenantList>(
    request,
    "GET",
    `/orgs/${orgId}/tenants?q=${encodeURIComponent(name)}`,
    200,
  );
  const tenant = tenants.items.find((t) => t.name === name);
  if (tenant === undefined) throw new Error(`${name} not seeded`);
  return tenant;
}

async function fontSize(target: Locator): Promise<number> {
  return target
    .first()
    .evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
}

async function expectTall(target: Locator, min: number) {
  const box = await target.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(min);
}

function filters(page: Page) {
  return page.getByRole("group", { name: FILTERS_LABEL });
}

function rows(page: Page) {
  return page.getByRole("list", { name: LIST_LABEL }).getByRole("listitem");
}

async function expectScreen(page: Page, testInfo: TestInfo) {
  await expect(page).toHaveURL(QUEUE_URL);
  await expect(
    page.getByRole("heading", { level: 1, name: TITLE }),
  ).toBeVisible();
  await expect(page.getByText(SUBTITLE)).toBeVisible();
  await expect(
    page
      .getByRole("navigation", { name: "Lokasi halaman" })
      .getByRole("link", { name: "Penagihan" }),
  ).toBeVisible();
  // The side menu (iPad and desktop) keeps Penagihan active.
  if (projectName(testInfo) !== "phone") {
    await expect(
      page
        .getByRole("navigation")
        .getByRole("link", { name: "Penagihan", exact: true })
        .and(page.locator("[aria-current=page]")),
    ).toHaveCount(1);
  }
  await expect(filters(page)).toBeVisible();
  await expect(
    page
      .getByRole("list", { name: LIST_LABEL })
      .or(page.getByText(EMPTY_TEXT))
      .first(),
  ).toBeVisible();
}

function checkQueueLink(page: Page) {
  return page.getByRole("main").getByRole("link", { name: /^Antrean cek/ });
}

test.describe("Admin of the whole organization", () => {
  test.use({ storageState: DIMAS.storageState });

  test("lists the queue and filters it by kind", async ({ page }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    await openOrganization(page, DIMAS, BULANAN);
    const orgId = orgIdOf(page);
    expect(await access(page, orgId, "payment:check")).toBe("allow");

    // Penagihan links to the queue with the number waiting (other specs may change it).
    await page.goto(`/o/${orgId}/billing`);
    await expect(checkQueueLink(page)).toHaveAccessibleName(
      /^Antrean cek(, \d+ menunggu)?$/,
    );
    await expectTall(checkQueueLink(page), 44);
    await checkQueueLink(page).click();
    await expect(page).toHaveURL(/\/billing\/check-queue$/);
    await expectScreen(page, testInfo);
    const queue = await call<CheckQueuePage>(
      page.request,
      "GET",
      `/orgs/${orgId}/check-queue?pageSize=1`,
      200,
    );

    // "Semua" is chosen and counts every waiting item.
    const all = filters(page).getByRole("button", { name: /^Semua/ });
    await expect(all).toHaveAttribute("aria-pressed", "true");
    await expect(all).toHaveText(new RegExp(`^Semua\\s*${queue.waiting}$`));
    await expectTall(all, 44);

    const gaps = filters(page).getByRole("button", {
      name: /^Selisih setoran/,
    });
    await expect(gaps).toHaveText(
      new RegExp(`^Selisih setoran\\s*${queue.counts.deposit_gap}$`),
    );
    await gaps.click();
    await expect(page).toHaveURL(/\?kind=deposit_gap$/);
    await expect(gaps).toHaveAttribute("aria-pressed", "true");
    await expect(all).toHaveAttribute("aria-pressed", "false");
    if (queue.counts.deposit_gap === 0) {
      await expect(page.getByText(EMPTY_TEXT)).toBeVisible();
    } else {
      await expect(rows(page).first()).toContainText("Selisih setoran");
      await expect(rows(page).filter({ hasText: "Nomor baru" })).toHaveCount(0);
      await expectTall(rows(page).first(), 76);
    }

    await all.click();
    await expect(page).toHaveURL(/\/billing\/check-queue$/);
    await expect(page.getByText(FORBIDDEN_TEXT)).toHaveCount(0);
    expect(
      await fontSize(page.getByText(SUBTITLE, { exact: true })),
    ).toBeGreaterThanOrEqual(minBodyFontPx(testInfo));
    await attachFullPageScreenshot(page, testInfo, "check-queue");
    expect(outsideRequests).toEqual([]);
  });

  test("closes a deposit gap with a note", async ({
    page,
    playwright,
    baseURL,
  }, testInfo) => {
    test.skip(
      projectName(testInfo) !== "ipad",
      "One karyawan's cash is shared by every project; only the iPad deposits and counts it.",
    );
    const outsideRequests = trackOutsideRequests(page);
    await openOrganization(page, DIMAS, BULANAN);
    const orgId = orgIdOf(page);

    // Mas Kurnia takes cash from a tenant he collects from and hands it all to the office.
    const karyawan = await playwright.request.newContext({
      baseURL,
      storageState: KURNIA.storageState,
    });
    let deposit: CashDeposit;
    try {
      const tenant = await tenantNamed(karyawan, orgId, CASH_TENANT);
      const payment: CashPaymentInput = {
        requestId: crypto.randomUUID(),
        amount: Number(`2${runId()}10`),
      };
      await call<CashPaymentResult>(
        karyawan,
        "POST",
        `/orgs/${orgId}/tenants/${tenant.id}/cash-payments`,
        201,
        payment,
      );
      const cash = await call<CashTotal>(
        karyawan,
        "GET",
        `/orgs/${orgId}/field/cash`,
        200,
      );
      const input: CashDepositInput = {
        requestId: crypto.randomUUID(),
        expectedAmount: cash.amount,
        expectedCount: cash.count,
        amount: cash.amount,
      };
      deposit = await call<CashDeposit>(
        karyawan,
        "POST",
        `/orgs/${orgId}/field/cash-deposits`,
        201,
        input,
      );
    } finally {
      await karyawan.dispose();
    }

    // Pak Dimas counts it short by an amount of this run, so the row is this run's.
    const short = Number(`5${runId()}`);
    await call<CashDeposit>(
      page.request,
      "POST",
      `/orgs/${orgId}/cash-deposits/${deposit.id}/confirmation`,
      201,
      { receivedAmount: deposit.expectedAmount - short },
    );
    const shortText = `Kurang ${formatRupiah(short)}`;

    await page.goto(`/o/${orgId}/billing/check-queue?kind=deposit_gap`);
    await expectScreen(page, testInfo);
    const row = rows(page)
      .filter({ hasText: KURNIA.name })
      .filter({ hasText: shortText });
    await expect(row).toHaveCount(1);
    await expect(row).toContainText(`${deposit.paymentCount} kwitansi`);
    await expect(row).toContainText(
      `Total kwitansi ${formatRupiah(deposit.expectedAmount)}`,
    );
    await expect(row).toContainText(/Masuk \S+ \d+ \w+, \d\d\.\d\d/);
    await expectTall(row.getByRole("button"), 76);
    expect(
      await fontSize(row.getByText(KURNIA.name, { exact: false })),
    ).toBeGreaterThanOrEqual(minBodyFontPx(testInfo));
    await row.getByRole("button").click();

    const sheet = page.getByRole("dialog", { name: DEPOSIT_GAP_TITLE });
    await expect(sheet).toBeVisible();
    await expect(sheet).toContainText(
      `${KURNIA.name} · dihitung ${DIMAS.name}, `,
    );
    await expect(sheet).toContainText(formatRupiah(deposit.expectedAmount));
    await expect(sheet).toContainText(
      formatRupiah(deposit.expectedAmount - short),
    );
    await expect(sheet).toContainText(shortText);
    await expect(sheet).toContainText("Tanyakan ke Mas Kurnia.");
    await expect(
      sheet.getByRole("link", { name: "Lihat setoran ini" }),
    ).toHaveAttribute("href", `/o/${orgId}/billing/deposits?tab=confirmed`);

    // The note is required.
    const submit = sheet.getByRole("button", { name: DEPOSIT_GAP_SUBMIT });
    await expectTall(submit, 44);
    await submit.click();
    await expect(sheet.getByText(NOTE_EMPTY_MESSAGE)).toBeVisible();
    await attachViewportScreenshot(page, testInfo, "check-queue-deposit-gap");

    const note = `Mas Kurnia menyerahkan sisanya, run ${runId()}`;
    await sheet.getByLabel(DEPOSIT_GAP_NOTE_LABEL).fill(`  ${note}  `);
    await expect(sheet.getByText(NOTE_EMPTY_MESSAGE)).toHaveCount(0);
    const answer = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname.endsWith(
          `/check-queue/deposit_gap/${deposit.id}/dismiss`,
        ),
    );
    await submit.click();
    const response = await answer;
    expect(response.status(), await response.text()).toBe(200);
    expect(response.request().postDataJSON()).toEqual({ note });
    const resolution = (await response.json()) as CheckResolution;
    expect(resolution.outcome).toBe("dismissed");

    await expectToast(page, "Selisih ditutup", "create");
    await expect(sheet).toHaveCount(0);
    await expect(row).toHaveCount(0);

    // The deposit shows the closure, and a second answer is refused.
    const confirmed = await call<{ items: CashDeposit[] }>(
      page.request,
      "GET",
      `/orgs/${orgId}/cash-deposits?status=confirmed&pageSize=100`,
      200,
    );
    expect(
      confirmed.items.find((item) => item.id === deposit.id)?.gapClosure,
    ).toBeDefined();
    const again = await page.request.post(
      `/api/orgs/${orgId}/check-queue/deposit_gap/${deposit.id}/dismiss`,
      { data: { note: "lagi" } },
    );
    expect(again.status()).toBe(409);
    expect(await again.json()).toMatchObject({
      error: { code: "check_item_resolved" },
    });
    expect(outsideRequests).toEqual([]);
  });

  test("applies one new number and refuses another", async ({
    page,
    playwright,
    baseURL,
  }, testInfo) => {
    test.skip(
      projectName(testInfo) !== "ipad",
      "One tenant's number is shared by every project; only the iPad changes it.",
    );
    const outsideRequests = trackOutsideRequests(page);
    await openOrganization(page, DIMAS, BULANAN);
    const orgId = orgIdOf(page);
    const tenant = await tenantNamed(page.request, orgId, PHONE_TENANT);
    const other = await tenantNamed(page.request, orgId, OTHER_TENANT);

    // Earlier runs may have left numbers for this tenant waiting: take them off first, so the
    // rows on screen are this test's two.
    const waiting = await call<CheckQueuePage>(
      page.request,
      "GET",
      `/orgs/${orgId}/check-queue?kind=phone_changed&pageSize=100`,
      200,
    );
    for (const left of waiting.items) {
      if (left.phoneChanged?.tenant.id !== tenant.id) continue;
      await call<CheckResolution>(
        page.request,
        "POST",
        `/orgs/${orgId}/check-queue/phone_changed/${left.id}/dismiss`,
        200,
        { note: "Sisa uji sebelumnya" },
      );
    }

    // Mas Kurnia brings back two numbers: one of this run, then one another tenant already has.
    const newPhone = `08197${runId()}01`;
    const takenPhone = other.phone;
    const karyawan = await playwright.request.newContext({
      baseURL,
      storageState: KURNIA.storageState,
    });
    let visits: VisitOutcome[];
    try {
      const visitList = await call<FieldVisitList>(
        karyawan,
        "GET",
        `/orgs/${orgId}/field/visits?pageSize=100`,
        200,
      );
      // The e2e setup runs today's daily jobs for Kos Bulanan Sejahtera, which open the visits
      // (.github/workflows/web.yml, "Migrate and seed").
      expect(
        visitList.items.some((visit) => visit.tenantId === tenant.id),
        `${PHONE_TENANT} has no open visit: run "admin run-day -day <today> -org <Kos Bulanan Sejahtera>" after seeding.`,
      ).toBe(true);
      visits = [];
      for (const [phone, note] of [
        [newPhone, "Nomor lama sudah hilang"],
        [takenPhone, "Katanya pakai nomor kakaknya"],
      ] as const) {
        const input: VisitOutcomeInput = {
          requestId: crypto.randomUUID(),
          outcome: "number_changed",
          newPhone: phone,
          note,
        };
        visits.push(
          await call<VisitOutcome>(
            karyawan,
            "POST",
            `/orgs/${orgId}/field/tenants/${tenant.id}/visit-outcomes`,
            201,
            input,
          ),
        );
      }
    } finally {
      await karyawan.dispose();
    }
    const [applied, refused] = visits;
    if (applied === undefined || refused === undefined) throw new Error("no");

    await page.goto(`/o/${orgId}/billing/check-queue?kind=phone_changed`);
    await expectScreen(page, testInfo);
    const tenantRows = rows(page).filter({ hasText: PHONE_TENANT });
    await expect(tenantRows).toHaveCount(2);
    await expect(tenantRows.first()).toContainText(
      `Dicatat ${KURNIA.name} saat kunjungan`,
    );
    await expect(tenantRows.first()).toContainText("Perlu diterapkan");

    // The older one first: the run's own number.
    await tenantRows.first().getByRole("button").click();
    const sheet = page.getByRole("dialog", { name: PHONE_TITLE });
    await expect(sheet).toBeVisible();
    await expect(sheet).toContainText(formatPhone(tenant.phone));
    await expect(sheet).toContainText(formatPhone(applied.newPhone ?? ""));
    await expect(sheet).toContainText(`dari kunjungan ${KURNIA.name}`);
    await expect(sheet).toContainText(`Catatan ${KURNIA.name} · `);
    await expect(sheet).toContainText("“Nomor lama sudah hilang”");
    await attachViewportScreenshot(page, testInfo, "check-queue-phone");
    const applyAnswer = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname.endsWith(
          `/check-queue/phone_changed/${applied.id}/apply`,
        ),
    );
    await sheet.getByRole("button", { name: PHONE_APPLY }).click();
    expect((await applyAnswer).status()).toBe(200);
    await expectToast(page, `Nomor ${PHONE_TENANT} diganti`, "create");
    await expect(sheet).toHaveCount(0);
    await expect(tenantRows).toHaveCount(1);
    const changed = await call<Tenant>(
      page.request,
      "GET",
      `/orgs/${orgId}/tenants/${tenant.id}`,
      200,
    );
    expect(changed.phone).toBe(applied.newPhone);

    // The other number belongs to another tenant: the API refuses it under the number.
    await tenantRows.first().getByRole("button").click();
    const second = page.getByRole("dialog", { name: PHONE_TITLE });
    await expect(second).toContainText(formatPhone(refused.newPhone ?? ""));
    await second.getByRole("button", { name: PHONE_APPLY }).click();
    await expect(
      second.getByText("Nomor ini sudah dipakai penyewa lain."),
    ).toBeVisible();

    // "Jangan pakai" turns the drawer into the reason form; "Kembali" goes back.
    await second.getByRole("button", { name: PHONE_REJECT }).click();
    const reject = page.getByRole("dialog", { name: PHONE_REJECT_TITLE });
    await expect(reject).toBeVisible();
    await expect(reject).toContainText(
      `Nomor ${PHONE_TENANT} tetap ${formatPhone(changed.phone)}. Item ini hilang dari antrean.`,
    );
    await reject.getByRole("button", { name: PHONE_BACK }).click();
    await expect(page.getByRole("dialog", { name: PHONE_TITLE })).toBeVisible();
    await page
      .getByRole("dialog", { name: PHONE_TITLE })
      .getByRole("button", { name: PHONE_REJECT })
      .click();
    const submit = reject.getByRole("button", { name: PHONE_REJECT_SUBMIT });
    await submit.click();
    await expect(reject.getByText(REASON_EMPTY_MESSAGE)).toBeVisible();
    await reject
      .getByLabel(PHONE_REASON_LABEL)
      .fill("Nomor milik penyewa lain");
    const rejectAnswer = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname.endsWith(
          `/check-queue/phone_changed/${refused.id}/dismiss`,
        ),
    );
    await submit.click();
    expect((await rejectAnswer).status()).toBe(200);
    await expectToast(page, "Nomor baru tidak dipakai", "neutral");
    await expect(reject).toHaveCount(0);
    await expect(tenantRows).toHaveCount(0);
    const kept = await call<Tenant>(
      page.request,
      "GET",
      `/orgs/${orgId}/tenants/${tenant.id}`,
      200,
    );
    expect(kept.phone).toBe(applied.newPhone);
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("Supervisor", () => {
  test.use({ storageState: HENDRA.storageState });

  test("reads the queue without answering it", async ({ page }, testInfo) => {
    await openOrganization(page, HENDRA, ANGGREK);
    const orgId = orgIdOf(page);
    expect(await access(page, orgId, "cash:read")).toBe("allow");
    expect(await access(page, orgId, "payment:check")).not.toBe("allow");

    await page.goto(`/o/${orgId}/billing`);
    await checkQueueLink(page).click();
    await expect(page).toHaveURL(/\/billing\/check-queue$/);
    await expectScreen(page, testInfo);
    await expect(page.getByText(FORBIDDEN_TEXT)).toHaveCount(0);
    await expect(
      filters(page).getByRole("button", { name: /^Semua/ }),
    ).toHaveAttribute("aria-pressed", "true");
    await expect(
      page.getByRole("button", { name: ACTION_BUTTONS }),
    ).toHaveCount(0);

    // A row that opens shows its drawer with "Tutup" only: no note, no action.
    const openable = rows(page).getByRole("button");
    if ((await openable.count()) > 0) {
      await openable.first().click();
      const sheet = page.getByRole("dialog");
      await expect(sheet).toBeVisible();
      await expect(sheet.getByRole("textbox")).toHaveCount(0);
      await expect(
        sheet.getByRole("button", { name: ACTION_BUTTONS }),
      ).toHaveCount(0);
      await sheet.getByRole("button", { name: "Tutup" }).last().click();
      await expect(sheet).toHaveCount(0);
    }
  });
});

test.describe("Admin limited to one property", () => {
  test.use({ storageState: SARI.storageState });

  test("has no access to Antrean cek", async ({ page }) => {
    await openOrganization(page, SARI, GRIYA);
    const orgId = orgIdOf(page);
    expect(await access(page, orgId, "payment:check")).not.toBe("allow");
    expect(await access(page, orgId, "cash:read")).not.toBe("allow");

    // Penagihan offers no link.
    await page.goto(`/o/${orgId}/billing`);
    await expect(
      page.getByRole("heading", { level: 1, name: "Penagihan" }),
    ).toBeVisible();
    await expect(checkQueueLink(page)).toHaveCount(0);

    // A typed address: the screen says so and offers nothing; the API agrees.
    await page.goto(`/o/${orgId}/billing/check-queue`);
    await expect(
      page.getByRole("heading", { level: 1, name: TITLE }),
    ).toBeVisible();
    await expect(page.getByText(FORBIDDEN_TEXT)).toBeVisible();
    await expect(filters(page)).toHaveCount(0);
    const answer = await page.request.get(`/api/orgs/${orgId}/check-queue`);
    expect(answer.status()).toBe(403);
  });
});
