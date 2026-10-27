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

const jakartaDateTimeParts = new Intl.DateTimeFormat("id-ID", {
  timeZone: JAKARTA,
  day: "numeric",
  month: "short",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hourCycle: "h23",
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

/**
 * Formats an API calendar date (`"2026-10-25"`) as `"25 Okt 2026"`.
 *
 * @throws RangeError when the value is not a real `YYYY-MM-DD` date.
 */
export function formatDate(date: string): string {
  const match = CALENDAR_DATE.exec(date);
  if (!match) {
    throw new RangeError(
      `formatDate expects YYYY-MM-DD, got ${JSON.stringify(date)}`,
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
    throw new RangeError(`formatDate got a date that does not exist: ${date}`);
  }
  const parts = calendarDateParts.formatToParts(utc);
  return `${part(parts, "day")} ${part(parts, "month")} ${part(parts, "year")}`;
}

/**
 * Formats an API timestamp (`format: date-time`) in Jakarta time:
 * `"2026-10-25T14:05:00Z"` gives `"25 Okt 2026, 21.05"`.
 *
 * @throws RangeError when the value is not a valid timestamp with a zone.
 */
export function formatDateTime(timestamp: string | Date): string {
  if (typeof timestamp === "string" && !TIMESTAMP_WITH_ZONE.test(timestamp)) {
    throw new RangeError(
      `formatDateTime expects an RFC 3339 timestamp with a zone, got ${JSON.stringify(timestamp)}`,
    );
  }
  const instant =
    typeof timestamp === "string" ? new Date(timestamp) : timestamp;
  if (Number.isNaN(instant.getTime())) {
    throw new RangeError("formatDateTime got an invalid date");
  }
  const parts = jakartaDateTimeParts.formatToParts(instant);
  const date = `${part(parts, "day")} ${part(parts, "month")} ${part(parts, "year")}`;
  return `${date}, ${part(parts, "hour")}.${part(parts, "minute")}`;
}
