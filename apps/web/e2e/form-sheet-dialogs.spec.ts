import {
  expect,
  test,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import type {
  Lease,
  Tenant,
  TenantAccount,
  Unit,
} from "../src/lib/api/generated/model";
import { formatMonthYear, jakartaToday } from "../src/lib/format/date";
import {
  animationsSettled,
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { expectTouchTarget } from "./support/payment-page";
import { projectDigit, projectWord } from "./support/projects";
import { findOrCreateProperty } from "./support/property-fixtures";
import { isPhone, ORG_HOME_URL } from "./support/shell";
import { HENDRA } from "./support/staff";

// The billing forms on the form sheet (board HP-Lembar): "Koreksi nominal" and "Catat janji" on
// Detail penyewa are a sheet from the bottom on the phone, with Batal and the save button pinned
// in view, that asks before a typed form closes unsaved; on iPad and desktop they stay the
// centred dialog, at most 560px wide, they always were. Against the real API and its worker:
// each test makes a tenant of its own who owes one invoice (a kos unit rented by the month from
// the 1st). Nothing is saved; every form is closed unsaved.

const GRIYA = "Griya Sejahtera";
const REGION = "Uang penyewa";
const PRICE = 1_050_000;
const QUESTION = "Tutup tanpa menyimpan?";

test.use({ storageState: HENDRA.storageState });

function runId(): string {
  const run = process.env.E2E_RUN_ID;
  if (run === undefined || !/^\d{4}$/.test(run)) {
    throw new Error("E2E_RUN_ID is set by playwright.config.ts");
  }
  return run;
}

// A name no other run, project or retry uses.
function uniqueName(testInfo: TestInfo, base: string): string {
  const project = projectWord(testInfo);
  const retry = testInfo.retry === 0 ? "" : ` r${testInfo.retry}`;
  return `${base} ${runId()} ${project}${retry}`;
}

// A tenant number no other run, project, test or retry uses; 0815-6 is used by no other spec.
function tenantNumber(testInfo: TestInfo, testDigit: number): string {
  const run = runId();
  const project = projectDigit(testInfo);
  return `0815-6${run.slice(0, 3)}-${run.slice(3)}${project}${testDigit}${testInfo.retry}`;
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

// Opens Detail penyewa of a tenant of the test's own who owes one invoice, and returns the
// "Koreksi" button of that invoice.
async function openTenantWhoOwes(
  page: Page,
  testInfo: TestInfo,
  testDigit: number,
  tenantBase: string,
): Promise<{ correct: Locator; correctTitle: string }> {
  await page.goto("/organizations");
  if (HENDRA.organizations.length > 1) {
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name: GRIYA })
      .click();
  }
  await expect(page).toHaveURL(ORG_HOME_URL);
  const orgId = /\/o\/([0-9a-f-]{36})/.exec(page.url())?.[1];
  if (orgId === undefined) throw new Error("not on an organization's page");

  const property = await findOrCreateProperty(page, orgId, testInfo, {
    base: "Lembar",
    kind: "kos",
  });
  const unit = await call<Unit>(
    page,
    "POST",
    `/orgs/${orgId}/properties/${property.id}/units`,
    201,
    {
      label: uniqueName(testInfo, `Kamar Lembar ${testDigit}`),
      terms: [{ months: 1, price: PRICE }],
    },
  );
  const tenant = await call<Tenant>(
    page,
    "POST",
    `/orgs/${orgId}/tenants`,
    201,
    {
      name: uniqueName(testInfo, tenantBase),
      phone: tenantNumber(testInfo, testDigit),
      note: "",
    },
  );
  await call<Lease>(page, "POST", `/orgs/${orgId}/leases`, 201, {
    tenantId: tenant.id,
    unitId: unit.id,
    startDate: `${jakartaToday(new Date()).slice(0, 7)}-01`,
    termMonths: 1,
    depositAmount: PRICE,
  });

  // The worker invoices the lease within seconds.
  const accountPath = `/orgs/${orgId}/tenants/${tenant.id}/account`;
  let account = await call<TenantAccount>(page, "GET", accountPath, 200);
  await expect
    .poll(
      async () => {
        account = await call<TenantAccount>(page, "GET", accountPath, 200);
        return account.invoices.length;
      },
      { timeout: 20_000 },
    )
    .toBeGreaterThanOrEqual(1);
  const [invoice] = account.invoices;
  if (invoice === undefined) throw new Error("no invoice");

  await page.goto(`/o/${orgId}/tenants/${tenant.id}`);
  await expect(
    page.getByRole("heading", { level: 1, name: tenant.name }),
  ).toBeVisible();
  const region = page.getByRole("region", { name: REGION });
  await expect(region).toBeVisible();
  const correctTitle = `Koreksi tagihan ${formatMonthYear(invoice.period)}`;
  return {
    correct: region.getByRole("button", { name: correctTitle }),
    correctTitle,
  };
}

function viewportOf(page: Page) {
  const viewport = page.viewportSize();
  if (viewport === null) throw new Error("The projects set a viewport.");
  return viewport;
}

async function boxOf(locator: Locator) {
  const box = await locator.boundingBox();
  if (box === null) throw new Error("The element is not visible.");
  return box;
}

// Opens a dialog by its button and waits for its enter animation to end.
async function openDialog(page: Page, opener: Locator, name: string) {
  await opener.click();
  const dialog = page.getByRole("dialog", { name });
  await expect(dialog).toBeVisible();
  await animationsSettled(page);
  return dialog;
}

// The phone's bottom sheet, or the centred dialog of iPad and desktop, with its footer in view.
async function expectLayout(
  page: Page,
  testInfo: TestInfo,
  dialog: Locator,
  save: string,
) {
  const viewport = viewportOf(page);
  const box = await boxOf(dialog);
  const handle = dialog.locator('[data-slot="form-sheet-handle"]');
  const enter = await dialog.evaluate((el) => {
    const style = getComputedStyle(el);
    return {
      x: style.getPropertyValue("--tw-enter-translate-x").trim(),
      y: style.getPropertyValue("--tw-enter-translate-y").trim(),
      scale: style.getPropertyValue("--tw-enter-scale").trim(),
      radius: style.borderTopLeftRadius,
    };
  });

  if (isPhone(testInfo)) {
    // From the bottom edge up to 8% below the top, the full width, with its grab handle,
    // sliding up from the bottom.
    expect(Math.abs(box.y + box.height - viewport.height)).toBeLessThanOrEqual(
      1,
    );
    expect(Math.abs(box.y - viewport.height * 0.08)).toBeLessThanOrEqual(2);
    expect(box.x).toBe(0);
    expect(box.width).toBe(viewport.width);
    expect(enter.radius).toBe("20px");
    expect(enter.y).toBe("100%");
    await expect(handle).toBeVisible();
  } else {
    // As before: centred, at most 560px wide, rounded, zooming in rather than sliding.
    expect(box.width).toBeLessThanOrEqual(560);
    expect(
      Math.abs(box.x + box.width / 2 - viewport.width / 2),
    ).toBeLessThanOrEqual(1);
    expect(
      Math.abs(box.y + box.height / 2 - viewport.height / 2),
    ).toBeLessThanOrEqual(1);
    expect(box.height).toBeLessThanOrEqual(viewport.height - 32);
    expect(enter.radius).toBe("16px");
    expect(enter).toMatchObject({ x: "0", y: "0" });
    expect(Number(enter.scale)).toBe(0.95);
    await expect(handle).toBeHidden();
  }

  // Batal and the save button in view without scrolling, with 44px targets.
  for (const name of ["Batal", save]) {
    const button = dialog.getByRole("button", { name, exact: true });
    await expect(button).toBeInViewport({ ratio: 1 });
    await expectTouchTarget(button);
  }
}

test("Koreksi and Catat janji open as a bottom sheet on the phone and as the centred dialog on iPad and desktop", async ({
  page,
}, testInfo) => {
  test.setTimeout(90_000);
  const outsideRequests = trackOutsideRequests(page);
  const { correct, correctTitle } = await openTenantWhoOwes(
    page,
    testInfo,
    1,
    "Ratna Kusumawati",
  );

  const correction = await openDialog(page, correct, correctTitle);
  await expectLayout(page, testInfo, correction, "Simpan koreksi");
  // "Nominal baru" takes the focus, as before.
  await expect(
    correction.getByRole("textbox", { name: "Nominal baru" }),
  ).toBeFocused();
  await attachViewportScreenshot(page, testInfo, "form-sheet-correction");
  // Untouched: it closes at once, on every size.
  await correction.getByRole("button", { name: "Batal", exact: true }).click();
  await expect(correction).toHaveCount(0);
  await expect(correct).toBeFocused();

  const record = page.getByRole("button", { name: "Catat janji" });
  const promise = await openDialog(page, record, "Catat janji");
  await expectLayout(page, testInfo, promise, "Simpan janji");
  await attachViewportScreenshot(page, testInfo, "form-sheet-promise");
  await page.keyboard.press("Escape");
  await expect(promise).toHaveCount(0);
  await expect(record).toBeFocused();
  expect(outsideRequests).toEqual([]);
});

test("a typed correction asks before Batal closes it, on the phone only", async ({
  page,
}, testInfo) => {
  test.setTimeout(90_000);
  const { correct, correctTitle } = await openTenantWhoOwes(
    page,
    testInfo,
    2,
    "Bambang Sutrisno",
  );
  const question = page.getByRole("alertdialog", { name: QUESTION });
  const reasonText = uniqueName(testInfo, "Potongan kran rusak");

  const dialog = await openDialog(page, correct, correctTitle);
  const reason = dialog.getByRole("textbox", { name: "Alasan (wajib diisi)" });
  const cancel = dialog.getByRole("button", { name: "Batal", exact: true });
  await reason.fill(reasonText);
  await cancel.click();

  if (!isPhone(testInfo)) {
    // iPad and desktop close as before, without asking.
    await expect(dialog).toHaveCount(0);
    await expect(question).toHaveCount(0);
    await expect(correct).toBeFocused();
    return;
  }

  await expect(question).toBeVisible();
  await attachViewportScreenshot(
    page,
    testInfo,
    "form-sheet-correction-question",
  );
  await question.getByRole("button", { name: "Lanjut mengisi" }).click();
  await expect(question).toHaveCount(0);
  await expect(reason).toHaveValue(reasonText);

  await cancel.click();
  await question.getByRole("button", { name: "Ya, tutup" }).click();
  await expect(question).toHaveCount(0);
  await expect(dialog).toHaveCount(0);
  await expect(correct).toBeFocused();
});
