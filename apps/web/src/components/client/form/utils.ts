// The look of the side-panel forms (properties, units, tenants, leases), shared so every form
// reads the same.

// A text field: 52px tall, 17px text on iPad and 16px on phone.
export const INPUT_CLASS =
  "h-13 rounded-xl border-[1.5px] bg-card px-3.5 text-base aria-invalid:border-2 md:text-[17px]";

export const TEXTAREA_CLASS =
  "min-h-28 rounded-xl border-[1.5px] bg-card px-3.5 py-3 text-base aria-invalid:border-2 md:text-[17px]";

// A select's trigger, as tall as a text field.
export const SELECT_TRIGGER_CLASS =
  "w-full rounded-xl border-[1.5px] bg-card px-3.5 text-base aria-invalid:border-2 data-[size=default]:h-13 md:text-[17px]";

// A select's choice, a 44px touch target.
export const SELECT_ITEM_CLASS = "min-h-11 text-base";

export const FIELD_LABEL_CLASS =
  "text-[15px] leading-normal font-semibold md:text-base";

export const FIELD_HINT_CLASS =
  "text-sm leading-[1.45] text-muted-foreground md:text-base";

// The closed trigger of a searchable select: the same box as a select's trigger (a button, since
// the popover's trigger is not Radix's select trigger), 52px tall, with the placeholder muted.
export const SEARCH_SELECT_TRIGGER_CLASS =
  "flex h-13 w-full items-center justify-between gap-2 rounded-xl border-[1.5px] border-input bg-card px-3.5 text-left text-base whitespace-nowrap shadow-xs transition-[color,box-shadow] outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-2 aria-invalid:border-destructive aria-invalid:ring-destructive/20 data-[placeholder]:text-muted-foreground md:text-[17px] [&_svg]:pointer-events-none [&_svg]:shrink-0";

// The open list: as wide as the trigger, and not taller than a few options, so it scrolls.
export const SEARCH_SELECT_CONTENT_CLASS =
  "w-(--radix-popover-trigger-width) overflow-hidden p-0";

// The list's box takes the height of its content; the search box's wrapper (shadcn's, 36px by
// default) and the box itself are 48px tall, with 16px text.
export const SEARCH_SELECT_COMMAND_CLASS =
  "h-auto **:data-[slot=command-input-wrapper]:h-12";
export const SEARCH_SELECT_INPUT_CLASS = "h-12 text-base";

// A choice of the open list, a 44px touch target with the check at the end.
export const SEARCH_SELECT_ITEM_CLASS =
  "min-h-11 justify-between px-3 py-2 text-base";

// The text of the list when it has no choices or is loading, and the note under it.
export const SEARCH_SELECT_MESSAGE_CLASS =
  "px-3 py-4 text-center text-base text-muted-foreground";

export const SEARCH_SELECT_NOTE_CLASS =
  "border-t px-3 py-3 text-sm leading-[1.45] text-muted-foreground md:text-base";

// What the trigger shows: the option's label, else the label remembered for the value, else
// nothing (the placeholder).
export function searchSelectShownLabel(
  value: string,
  options: readonly { value: string; label: string }[],
  selectedLabel: string | undefined,
): string | undefined {
  if (value === "") return undefined;
  return (
    options.find((option) => option.value === value)?.label ?? selectedLabel
  );
}

// ---------- Date fields ----------

// The width at which date fields switch from the device's own date input (phones) to the
// calendar popover (iPad and desktop): Tailwind's `md`, the phone layout's edge.
export const WIDE_LAYOUT_QUERY = "(min-width: 48rem)";

export const DATE_PLACEHOLDER = "Pilih tanggal";
export const DATE_TODAY_LABEL = "Hari ini";
export const DATE_PICKER_LABEL = "Pilih tanggal";
export const DATE_MONTH_LABEL = "Pilih bulan";
export const DATE_YEAR_LABEL = "Pilih tahun";
export const DATE_PREVIOUS_MONTH_LABEL = "Bulan sebelumnya";
export const DATE_NEXT_MONTH_LABEL = "Bulan berikutnya";

// The calendar offers this many years around today when the field sets no limit: back far
// enough for a lease entered late (the tenant import), forward for a planned end.
export const DATE_YEARS_BACK = 10;
export const DATE_YEARS_AHEAD = 5;

const CALENDAR_DAY = /^(\d{4})-(\d{2})-(\d{2})$/;

// A form's calendar date (`YYYY-MM-DD`) as the Date the calendar widget works with: midnight of
// that day on the device's clock, built from its parts. Never `new Date("YYYY-MM-DD")`, which
// reads it as UTC and shows the day before in any timezone west of Greenwich. Undefined for ""
// or anything that is not a real day.
export function calendarDayToLocalDate(day: string): Date | undefined {
  const match = CALENDAR_DAY.exec(day);
  if (!match) return undefined;
  const [year, month, date] = [
    Number(match[1]),
    Number(match[2]),
    Number(match[3]),
  ];
  const local = new Date(2000, 0, 1);
  local.setFullYear(year, month - 1, date);
  local.setHours(0, 0, 0, 0);
  if (
    local.getFullYear() !== year ||
    local.getMonth() !== month - 1 ||
    local.getDate() !== date
  ) {
    return undefined;
  }
  return local;
}

// The day the calendar widget picked, back to `YYYY-MM-DD`, from the device-clock parts it was
// built from (so the device's timezone never moves it).
export function localDateToCalendarDay(date: Date): string {
  const year = String(date.getFullYear()).padStart(4, "0");
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

// Whether a day is inside the field's limits; `YYYY-MM-DD` strings compare as dates.
export function isDayWithin(
  day: string,
  min: string | undefined,
  max: string | undefined,
): boolean {
  return (min === undefined || day >= min) && (max === undefined || day <= max);
}

// The first and last month the calendar's month and year lists offer: the field's limits when
// it has them, else DATE_YEARS_BACK and DATE_YEARS_AHEAD around today (Jakarta), always
// including the chosen day.
export function datePickerMonths(
  today: string,
  value: string,
  min: string | undefined,
  max: string | undefined,
): { start: Date; end: Date } {
  const year = Number(today.slice(0, 4));
  const chosen = calendarDayToLocalDate(value);
  const lower = calendarDayToLocalDate(min ?? "");
  const upper = calendarDayToLocalDate(max ?? "");
  let start = lower ?? new Date(year - DATE_YEARS_BACK, 0, 1);
  let end = upper ?? new Date(year + DATE_YEARS_AHEAD, 11, 1);
  if (chosen !== undefined && chosen < start) start = chosen;
  if (chosen !== undefined && chosen > end) end = chosen;
  return {
    start: new Date(start.getFullYear(), start.getMonth(), 1),
    end: new Date(end.getFullYear(), end.getMonth(), 1),
  };
}

// The month the calendar opens on: the chosen day's, else today's, moved inside the limits.
export function datePickerOpenMonth(
  today: string,
  value: string,
  min: string | undefined,
  max: string | undefined,
): Date {
  let day = calendarDayToLocalDate(value) !== undefined ? value : today;
  if (min !== undefined && day < min) day = min;
  if (max !== undefined && day > max) day = max;
  const date = calendarDayToLocalDate(day) ?? new Date();
  return new Date(date.getFullYear(), date.getMonth(), 1);
}

// Calendar labels, Bahasa Indonesia. The Dates here are the widget's device-clock days
// (calendarDayToLocalDate), so they are formatted on the device's clock too.
const monthNameFormat = new Intl.DateTimeFormat("id-ID", { month: "long" });
const weekdayNameFormat = new Intl.DateTimeFormat("id-ID", {
  weekday: "short",
});

// "November".
export function calendarMonthName(date: Date): string {
  return monthNameFormat.format(date);
}

// "Sen", "Sel", "Rab", "Kam", "Jum", "Sab", "Min".
export function calendarWeekdayName(date: Date): string {
  return weekdayNameFormat.format(date);
}

// A day's accessible name: "15 November 2026", with ", hari ini" and ", dipilih" when they apply.
export function calendarDayLabel(
  date: Date,
  state: { today: boolean; selected: boolean },
): string {
  const full = `${date.getDate()} ${calendarMonthName(date)} ${date.getFullYear()}`;
  return `${full}${state.today ? ", hari ini" : ""}${state.selected ? ", dipilih" : ""}`;
}

// The closed calendar field: the same 52px box as a select's trigger, with the placeholder muted.
export const DATE_TRIGGER_CLASS = SEARCH_SELECT_TRIGGER_CLASS;
