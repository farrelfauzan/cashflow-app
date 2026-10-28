// Date display. Every time is shown in Asia/Jakarta whatever the device's timezone.
// Month names come from Intl (id-ID); the separators are assembled here so the output is
// the same in Node and every browser, whatever ICU version each one ships.

const JAKARTA = "Asia/Jakarta";

// A calendar date (`format: date`, e.g. a due date) has no time and no zone. It is read as
// midnight UTC and formatted in UTC, so it never moves a day in any timezone.
const calendarDateParts = new Intl.DateTimeFormat("id-ID", {
  timeZone: "UTC",
  day: "numeric",
  month: "short",
  year: "numeric",
});

const calendarDayMonthParts = new Intl.DateTimeFormat("id-ID", {
  timeZone: "UTC",
  day: "numeric",
  month: "short",
});

const jakartaDayParts = new Intl.DateTimeFormat("en-CA", {
  timeZone: JAKARTA,
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});

const jakartaDateTimeParts = new Intl.DateTimeFormat("id-ID", {
  timeZone: JAKARTA,
  day: "numeric",
  month: "short",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hourCycle: "h23",
});

const jakartaDateParts = new Intl.DateTimeFormat("id-ID", {
  timeZone: JAKARTA,
  day: "numeric",
  month: "short",
  year: "numeric",
});

const CALENDAR_DATE = /^(\d{4})-(\d{2})-(\d{2})$/;
// RFC 3339 with an explicit zone. A timestamp without one would be read in the device's
// timezone, so it is rejected instead of guessed.
const TIMESTAMP_WITH_ZONE =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:\d{2})$/i;

function part(
  parts: Intl.DateTimeFormatPart[],
  type: Intl.DateTimeFormatPartTypes,
): string {
  const value = parts.find((p) => p.type === type)?.value;
  if (value === undefined) {
    throw new Error(`Intl did not return a ${type} part`);
  }
  return value;
}

// Reads `YYYY-MM-DD` as midnight UTC, so formatting it in UTC never moves the day.
function calendarDateToUtc(date: string, caller: string): Date {
  const match = CALENDAR_DATE.exec(date);
  if (!match) {
    throw new RangeError(
      `${caller} expects YYYY-MM-DD, got ${JSON.stringify(date)}`,
    );
  }
  const [year, month, day] = [
    Number(match[1]),
    Number(match[2]),
    Number(match[3]),
  ];
  const utc = new Date(Date.UTC(year, month - 1, day));
  if (
    utc.getUTCFullYear() !== year ||
    utc.getUTCMonth() !== month - 1 ||
    utc.getUTCDate() !== day
  ) {
    throw new RangeError(`${caller} got a date that does not exist: ${date}`);
  }
  return utc;
}

/**
 * Formats an API calendar date (`"2026-10-25"`) as `"25 Okt 2026"`.
 *
 * @throws RangeError when the value is not a real `YYYY-MM-DD` date.
 */
export function formatDate(date: string): string {
  const parts = calendarDateParts.formatToParts(
    calendarDateToUtc(date, "formatDate"),
  );
  return `${part(parts, "day")} ${part(parts, "month")} ${part(parts, "year")}`;
}

/**
 * Formats an API calendar date without its year, for short notes: `"2026-12-05"` gives
 * `"5 Des"`.
 *
 * @throws RangeError when the value is not a real `YYYY-MM-DD` date.
 */
export function formatDayMonth(date: string): string {
  const parts = calendarDayMonthParts.formatToParts(
    calendarDateToUtc(date, "formatDayMonth"),
  );
  return `${part(parts, "day")} ${part(parts, "month")}`;
}

/**
 * The calendar day it is in Jakarta at this instant, as `YYYY-MM-DD`:
 * `2026-10-27T18:30:00Z` gives `"2026-10-28"`. Dates from the API compare with it as strings.
 *
 * @throws RangeError when `now` is an invalid Date.
 */
export function jakartaToday(now: Date): string {
  if (Number.isNaN(now.getTime())) {
    throw new RangeError("jakartaToday got an invalid date");
  }
  const parts = jakartaDayParts.formatToParts(now);
  return `${part(parts, "year")}-${part(parts, "month")}-${part(parts, "day")}`;
}

/**
 * Formats an API timestamp (`format: date-time`) in Jakarta time:
 * `"2026-10-25T14:05:00Z"` gives `"25 Okt 2026, 21.05"`.
 *
 * @throws RangeError when the value is not a valid timestamp with a zone.
 */
export function formatDateTime(timestamp: string | Date): string {
  const parts = jakartaDateTimeParts.formatToParts(
    toInstant(timestamp, "formatDateTime"),
  );
  const date = `${part(parts, "day")} ${part(parts, "month")} ${part(parts, "year")}`;
  return `${date}, ${part(parts, "hour")}.${part(parts, "minute")}`;
}

/**
 * Formats the Jakarta calendar day of an API timestamp (`format: date-time`), without the
 * time: `"2026-10-27T18:30:00Z"` gives `"28 Okt 2026"`.
 *
 * @throws RangeError when the value is not a valid timestamp with a zone.
 */
export function formatTimestampDate(timestamp: string | Date): string {
  const parts = jakartaDateParts.formatToParts(
    toInstant(timestamp, "formatTimestampDate"),
  );
  return `${part(parts, "day")} ${part(parts, "month")} ${part(parts, "year")}`;
}

function toInstant(timestamp: string | Date, caller: string): Date {
  if (typeof timestamp === "string" && !TIMESTAMP_WITH_ZONE.test(timestamp)) {
    throw new RangeError(
      `${caller} expects an RFC 3339 timestamp with a zone, got ${JSON.stringify(timestamp)}`,
    );
  }
  const instant =
    typeof timestamp === "string" ? new Date(timestamp) : timestamp;
  if (Number.isNaN(instant.getTime())) {
    throw new RangeError(`${caller} got an invalid date`);
  }
  return instant;
}
