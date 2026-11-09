import { expect, type Locator, type Page } from "@playwright/test";

import { formatFullDate } from "../../src/lib/format/date";

// Date fields (components/client/form/date-field.tsx): the device's own date input on a phone,
// the calendar popover on iPad and desktop. These helpers work with either, so a spec states
// the day it wants and runs at every size.

// The field's control: the native input or the calendar's trigger, both named by the label.
export function dateControl(scope: Page | Locator, label: string): Locator {
  return scope
    .getByLabel(label, { exact: false })
    .and(scope.locator("input[type=date], button[aria-haspopup=dialog]"));
}

// Whether the field is the calendar popover (iPad, desktop) rather than the native input.
// Waits for the visible control first: while the page hydrates, a wide screen still has the
// hidden native input, which the calendar's trigger replaces.
export async function isCalendarField(control: Locator): Promise<boolean> {
  await expect(control).toBeVisible();
  return control.evaluate((el) => el.tagName === "BUTTON");
}

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// The open calendar popover.
export function calendarPopover(page: Page): Locator {
  return page.getByRole("dialog", { name: "Pilih tanggal" });
}

// The calendar's button for a day ("15 November 2026", with ", hari ini" or ", dipilih").
export function calendarDay(popover: Locator, day: string): Locator {
  return popover.getByRole("button", {
    name: new RegExp(`^${escapeRegExp(formatFullDate(day))}(,|$)`),
  });
}

// Opens the calendar on `day`'s month through the month and year lists.
export async function showMonthOf(
  popover: Locator,
  day: string,
): Promise<void> {
  const [year, month] = day.split("-").map(Number);
  await popover
    .getByRole("combobox", { name: "Pilih tahun" })
    .selectOption(String(year));
  await popover
    .getByRole("combobox", { name: "Pilih bulan" })
    .selectOption(String((month ?? 1) - 1));
}

// Sets the field to `day` (YYYY-MM-DD) the way a user would at this size: types it into the
// native input, or opens the calendar, moves to its month and presses the day.
export async function setDate(
  scope: Page | Locator,
  label: string,
  day: string,
): Promise<void> {
  const control = dateControl(scope, label);
  if (!(await isCalendarField(control))) {
    await control.fill(day);
    return;
  }
  await control.click();
  const popover = calendarPopover(control.page());
  await expect(popover).toBeVisible();
  await showMonthOf(popover, day);
  await calendarDay(popover, day).click();
  await expect(popover).toHaveCount(0);
}

// Empties the field. Only the native input can be emptied (the calendar has no "no date"), so
// specs that check an empty date's error do it on the phone or before choosing one.
export async function clearDate(
  scope: Page | Locator,
  label: string,
): Promise<void> {
  const control = dateControl(scope, label);
  if (await isCalendarField(control)) {
    throw new Error(
      `"${label}" is a calendar field here and cannot be emptied`,
    );
  }
  await control.fill("");
}

// The field shows `day` (YYYY-MM-DD, "" for none).
export async function expectDate(
  scope: Page | Locator,
  label: string,
  day: string,
): Promise<void> {
  const control = dateControl(scope, label);
  if (!(await isCalendarField(control))) {
    await expect(control).toHaveValue(day);
    return;
  }
  await expect(control).toHaveText(
    day === "" ? "Pilih tanggal" : formatFullDate(day),
  );
}

// `day` (YYYY-MM-DD) moved by `days` days.
export function shiftDay(day: string, days: number): string {
  const [year = 0, month = 1, date = 1] = day.split("-").map(Number);
  return new Date(Date.UTC(year, month - 1, date + days))
    .toISOString()
    .slice(0, 10);
}

// Moves the open calendar to `day`'s month when its year and month lists offer it; false when
// they do not (the month is outside the field's limits, so none of its days can be chosen).
async function showMonthIfOffered(
  popover: Locator,
  day: string,
): Promise<boolean> {
  const [year = 0, month = 1] = day.split("-").map(Number);
  const years = popover.getByRole("combobox", { name: "Pilih tahun" });
  const yearOption = years.locator(`option[value="${year}"]`);
  if ((await yearOption.count()) === 0 || (await yearOption.isDisabled())) {
    return false;
  }
  await years.selectOption(String(year));
  const months = popover.getByRole("combobox", { name: "Pilih bulan" });
  const monthOption = months.locator(`option[value="${month - 1}"]`);
  if ((await monthOption.count()) === 0 || (await monthOption.isDisabled())) {
    return false;
  }
  await months.selectOption(String(month - 1));
  return true;
}

// Opens the calendar, runs `check` on it, and closes it with Escape, which leaves the form open.
async function withOpenCalendar(
  control: Locator,
  check: (popover: Locator) => Promise<void>,
): Promise<void> {
  await control.click();
  const popover = calendarPopover(control.page());
  await expect(popover).toBeVisible();
  await check(popover);
  await control.page().keyboard.press("Escape");
  await expect(popover).toHaveCount(0);
}

// On the calendar (iPad, desktop): `day` cannot be chosen, because it is disabled in its month
// or its month is not offered at all. Leaves the field as it was.
export async function expectDayRefused(
  scope: Page | Locator,
  label: string,
  day: string,
): Promise<void> {
  const control = dateControl(scope, label);
  if (!(await isCalendarField(control))) {
    throw new Error(`"${label}" is a native date input here`);
  }
  await withOpenCalendar(control, async (popover) => {
    if (await showMonthIfOffered(popover, day)) {
      await expect(calendarDay(popover, day)).toBeDisabled();
    }
  });
}

// The field offers exactly the days from `min` to `max`: the native input's min and max, or on
// the calendar the days just outside disabled and the limits themselves enabled.
export async function expectDateLimits(
  scope: Page | Locator,
  label: string,
  min: string,
  max: string,
): Promise<void> {
  const control = dateControl(scope, label);
  if (!(await isCalendarField(control))) {
    await expect(control).toHaveAttribute("min", min);
    await expect(control).toHaveAttribute("max", max);
    return;
  }
  await withOpenCalendar(control, async (popover) => {
    for (const [day, offered] of [
      [shiftDay(min, -1), false],
      [min, true],
      [max, true],
      [shiftDay(max, 1), false],
    ] as const) {
      if (!(await showMonthIfOffered(popover, day))) {
        expect(offered, `${day}'s month is offered`).toBe(false);
        continue;
      }
      const button = calendarDay(popover, day);
      if (offered) await expect(button).toBeEnabled();
      else await expect(button).toBeDisabled();
    }
  });
}
