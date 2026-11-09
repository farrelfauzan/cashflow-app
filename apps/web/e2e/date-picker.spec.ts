import { expect, test, type Locator, type Page } from "@playwright/test";

import type { Lease, Tenant, Unit } from "../src/lib/api/generated/model";
import { formatFullDate, jakartaToday } from "../src/lib/format/date";
import {
  calendarDay,
  calendarPopover,
  dateControl,
  expectDate,
  expectDateLimits,
  isCalendarField,
  setDate,
  shiftDay,
  showMonthOf,
} from "./support/date-field";
import {
  attachViewportScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import {
  call,
  expectTouchTarget,
  GRIYA,
  makeLink,
  orgIdByName,
  readPage,
  rentSinceLastMonth,
  runId,
  staffApi,
  tenantClientAddress,
  tenantNumber,
  thisMonthInvoiceId,
  trackTokenLeaks,
  uniqueName,
} from "./support/payment-page";
import { projectDigit } from "./support/projects";
import { findOrCreateProperty } from "./support/property-fixtures";
import { HENDRA } from "./support/staff";

// Date fields (components/client/form/date-field.tsx), against the real API (seed-demo data):
// the shadcn calendar in a popover on iPad, the device's own date input on a phone, chosen by
// width. A staff drawer ("Mulai sewa" in "Ubah sewa") and the tenant payment page ("Tanggal
// pelunasan"). This spec's tenants: numbers 0815-4…, which no other spec uses; rooms
// "Kamar Kalender"; payment page client addresses 198.22.x.x.

const PHONE_PREFIX = "0815-4";

// A calendar date `months` months from today in Jakarta, on `day` (at most 28, so no month end
// moves it).
function dayOfMonth(months: number, day: number): string {
  const [year = 0, month = 1] = jakartaToday(new Date()).split("-").map(Number);
  return new Date(Date.UTC(year, month - 1 + months, day))
    .toISOString()
    .slice(0, 10);
}

// Whether the popover lies wholly inside the viewport (not clipped by the screen's edge).
async function expectInsideViewport(page: Page, popover: Locator) {
  const box = await popover.boundingBox();
  const viewport = page.viewportSize();
  if (box === null || viewport === null) throw new Error("nothing to measure");
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.y).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(viewport.width);
  expect(box.y + box.height).toBeLessThanOrEqual(viewport.height);
}

// Whether the element is what a tap at its center reaches (not covered by the drawer).
async function expectOnTop(locator: Locator) {
  const onTop = await locator.evaluate((el) => {
    const rect = el.getBoundingClientRect();
    const hit = document.elementFromPoint(
      rect.x + rect.width / 2,
      rect.y + rect.height / 2,
    );
    return hit !== null && (hit === el || el.contains(hit));
  });
  expect(onTop).toBe(true);
}

// The same-origin scripts the page has loaded so far, by URL (all /_next/static paths; the
// payment link's token is never in one).
async function loadedScripts(page: Page): Promise<string[]> {
  return page.evaluate(() =>
    performance
      .getEntriesByType("resource")
      .map((entry) => entry.name)
      .filter((name) => {
        const url = new URL(name);
        return url.origin === location.origin && url.pathname.endsWith(".js");
      }),
  );
}

// Whether any loaded script carries react-day-picker. Its class names are built at run time
// ("rdp-" + "month_grid"), so the bundle holds the bare "month_grid" of its UI enum.
async function calendarCodeLoaded(page: Page): Promise<boolean> {
  const scripts = await loadedScripts(page);
  expect(scripts.length).toBeGreaterThan(0);
  for (const script of scripts) {
    const response = await page.request.get(script);
    expect(response.ok()).toBe(true);
    if ((await response.text()).includes("month_grid")) return true;
  }
  return false;
}

test.describe("A lease date in a staff drawer", () => {
  test.use({ storageState: HENDRA.storageState });

  test("Mulai sewa in Ubah sewa: the calendar on iPad, the device's input on a phone", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);
    // A booking (starts in two months, no invoice yet), so its start date can still change.
    const start = dayOfMonth(2, 10);
    const chosen = dayOfMonth(3, 15);
    const today = jakartaToday(new Date());

    const orgId = await orgIdByName(page.request, GRIYA);
    const property = await findOrCreateProperty(page, orgId, testInfo, {
      base: "Kalender",
      kind: "kos",
    });
    const unit = await call<Unit>(
      page.request,
      "POST",
      `/orgs/${orgId}/properties/${property.id}/units`,
      201,
      {
        label: uniqueName(testInfo, "Kamar Kalender 1"),
        terms: [{ months: 1, price: 1_250_000 }],
      },
    );
    const tenant = await call<Tenant>(
      page.request,
      "POST",
      `/orgs/${orgId}/tenants`,
      201,
      {
        name: uniqueName(testInfo, "Wening Kalender"),
        phone: tenantNumber(testInfo, PHONE_PREFIX, 1),
        note: "",
      },
    );
    const lease = await call<Lease>(
      page.request,
      "POST",
      `/orgs/${orgId}/leases`,
      201,
      {
        tenantId: tenant.id,
        unitId: unit.id,
        startDate: start,
        termMonths: 1,
        depositAmount: 0,
      },
    );

    await page.goto(`/o/${orgId}/tenants/${tenant.id}`);
    await expect(
      page.getByRole("heading", { level: 1, name: tenant.name }),
    ).toBeVisible();
    const edit = page.getByRole("button", { name: "Ubah sewa" });
    await edit.click();
    const form = page.getByRole("dialog", { name: "Ubah sewa" });
    await expect(form).toBeVisible();
    const control = dateControl(form, "Mulai sewa");
    await expectDate(form, "Mulai sewa", start);
    await expectTouchTarget(control);

    if (await isCalendarField(control)) {
      // The trigger: a button named by the label and the chosen day, showing the day.
      await expect(control).toHaveText(formatFullDate(start));
      await expect(control).toHaveAccessibleName(
        `Mulai sewa ${formatFullDate(start)}`,
      );
      await control.click();
      const popover = calendarPopover(page);
      await expect(popover).toBeVisible();
      await expectInsideViewport(page, popover);

      // Indonesian months and weekdays, weeks from Monday.
      const months = popover.getByRole("combobox", { name: "Pilih bulan" });
      const years = popover.getByRole("combobox", { name: "Pilih tahun" });
      await expect(months.locator('option[value="0"]')).toHaveText("Januari");
      await expect(months.locator('option[value="7"]')).toHaveText("Agustus");
      await expect(months.locator('option[value="11"]')).toHaveText("Desember");
      const weekdays = popover.locator("thead th");
      await expect(weekdays).toHaveCount(7);
      await expect(weekdays.first()).toHaveText("Sen");
      await expect(weekdays.last()).toHaveText("Min");

      // The chosen day is the one the calendar opens on, and says so.
      await expect(
        popover.getByRole("button", {
          name: `${formatFullDate(start)}, dipilih`,
          exact: true,
        }),
      ).toBeVisible();
      // 44px days and lists, and nothing of it under the drawer.
      const startDay = calendarDay(popover, start);
      await expectTouchTarget(startDay);
      const startBox = await startDay.boundingBox();
      expect(startBox?.width ?? 0).toBeGreaterThanOrEqual(44);
      await expectTouchTarget(months);
      await expectTouchTarget(years);
      const todayButton = popover.getByRole("button", {
        name: "Hari ini",
        exact: true,
      });
      await expectTouchTarget(todayButton);
      await expectOnTop(startDay);
      await expectOnTop(todayButton);
      // On the chosen day's month, as the admin first sees it.
      await attachViewportScreenshot(page, testInfo, "date-picker-open");

      // Today, in Jakarta, carries ", hari ini".
      await showMonthOf(popover, today);
      await expect(
        popover.getByRole("button", {
          name: `${formatFullDate(today)}, hari ini`,
          exact: true,
        }),
      ).toBeVisible();

      // Escape closes the calendar only, without a change, and the trigger has the focus;
      // a second Escape closes the drawer.
      await page.keyboard.press("Escape");
      await expect(popover).toHaveCount(0);
      await expect(form).toBeVisible();
      await expect(control).toBeFocused();
      await expect(control).toHaveText(formatFullDate(start));
      await page.keyboard.press("Escape");
      await expect(form).toHaveCount(0);

      // "Hari ini" picks today in Jakarta, closes the calendar and gives the focus back.
      await edit.click();
      await expect(form).toBeVisible();
      await control.click();
      await expect(popover).toBeVisible();
      await todayButton.click();
      await expect(popover).toHaveCount(0);
      await expect(control).toHaveText(formatFullDate(today));
      await expect(control).toBeFocused();

      // A month and year from the lists, then a day: the trigger shows it, focus is back.
      await control.click();
      await expect(popover).toBeVisible();
      await showMonthOf(popover, chosen);
      await calendarDay(popover, chosen).click();
      await expect(popover).toHaveCount(0);
      await expect(control).toHaveText(formatFullDate(chosen));
      await expect(control).toHaveAccessibleName(
        `Mulai sewa ${formatFullDate(chosen)}`,
      );
      await expect(control).toBeFocused();
    } else {
      // The phone: the device's own date input, and no calendar anywhere.
      await expect(control).toHaveAttribute("type", "date");
      await expect(calendarPopover(page)).toHaveCount(0);
      await expect(page.locator(".rdp-root, [data-slot=calendar]")).toHaveCount(
        0,
      );
      await setDate(form, "Mulai sewa", chosen);
      await expectDate(form, "Mulai sewa", chosen);
      await attachViewportScreenshot(page, testInfo, "date-picker-native");
    }

    // Saved: the API gets the chosen day, and keeps it.
    const saved = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        new URL(response.url()).pathname.endsWith(`/leases/${lease.id}`),
    );
    await form.getByRole("button", { name: "Simpan sewa" }).click();
    const response = await saved;
    expect(response.status()).toBe(200);
    expect(response.request().postDataJSON()).toMatchObject({
      startDate: chosen,
    });
    expect(((await response.json()) as Lease).startDate).toBe(chosen);
    await expect(form).toHaveCount(0);

    await edit.click();
    await expect(form).toBeVisible();
    await expectDate(form, "Mulai sewa", chosen);

    expect(outsideRequests).toEqual([]);
  });
});

test.describe("The tenant promise date on the payment page", () => {
  test.use({
    storageState: { cookies: [], origins: [] },
    extraHTTPHeaders: async ({}, provide, testInfo) => {
      await provide({ "X-Forwarded-For": tenantClientAddress(testInfo, 22) });
    },
  });

  test("Tanggal pelunasan offers only the API's days, and a phone loads no calendar code", async ({
    page,
  }, testInfo) => {
    const api = await staffApi(HENDRA, testInfo);
    const run = Number(runId()) % 1000;
    const rented = await rentSinceLastMonth(api, testInfo, {
      digit: 2,
      phonePrefix: PHONE_PREFIX,
      price: 1_100_000 + run * 100 + Number(projectDigit(testInfo)) * 10,
      unitBase: "Kamar Kalender",
    });
    const { path, token } = await makeLink(
      api,
      rented.orgId,
      thisMonthInvoiceId(rented.account),
    );
    await api.dispose();
    const expected = await readPage(page, path);
    const dates = expected.promiseDates;
    if (dates === null) throw new Error("a tenant who owes has promise dates");

    const outside = trackOutsideRequests(page);
    const leaks = trackTokenLeaks(page, path, token);
    await page.goto(path);
    await page
      .getByRole("radiogroup", { name: "Mau bayar berapa?" })
      .getByRole("radio", { name: /^Belum bisa bayar sekarang/ })
      .check();
    await page.getByRole("button", { name: "Lanjut", exact: true }).click();
    await expect(
      page.getByRole("heading", { level: 1, name: "Kapan sisanya dilunasi?" }),
    ).toBeVisible();
    await page.getByRole("radio", { name: /^Pilih tanggal sendiri/ }).check();

    const label = "Tanggal pelunasan";
    const control = dateControl(page, label);
    const calendar = await isCalendarField(control);
    // Before the earliest and after the latest day: disabled on the calendar, the native
    // input's min and max on a phone.
    await expectDateLimits(page, label, dates.earliest, dates.latest);
    const promised = shiftDay(dates.earliest, 7);

    if (calendar) {
      // Today comes before the earliest day, so "Hari ini" cannot pick it.
      await control.click();
      const popover = calendarPopover(page);
      await expect(popover).toBeVisible();
      await expectInsideViewport(page, popover);
      await attachViewportScreenshot(
        page,
        testInfo,
        "date-picker-promise-open",
      );
      await expect(
        popover.getByRole("button", { name: "Hari ini", exact: true }),
      ).toBeDisabled();
      await page.keyboard.press("Escape");
      await expect(popover).toHaveCount(0);
      await expect(control).toBeFocused();
      // The check below finds the calendar's code here, so it can tell.
      expect(await calendarCodeLoaded(page)).toBe(true);
    } else {
      await expect(control).toHaveAttribute("type", "date");
      expect(await calendarCodeLoaded(page)).toBe(false);
    }

    await setDate(page, label, promised);
    await expectDate(page, label, promised);
    await attachViewportScreenshot(page, testInfo, "date-picker-promise");
    const sent = page.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname.endsWith("/promise"),
    );
    await page
      .getByRole("button", { name: "Simpan janji", exact: true })
      .click();
    expect((await sent).postDataJSON()).toMatchObject({
      kind: "specific_date",
      promisedDate: promised,
    });
    await expect(
      page.getByRole("heading", { level: 1, name: "Janji tercatat" }),
    ).toBeVisible();
    const after = await readPage(page, path);
    expect(after.promise?.promisedDate).toBe(promised);
    if (!calendar) expect(await calendarCodeLoaded(page)).toBe(false);

    expect(outside).toEqual([]);
    expect(leaks).toEqual([]);
  });
});
