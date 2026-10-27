import { describe, expect, it } from "vitest";

import { formatDate, formatDateTime } from "./date";

// Jakarta is UTC+7 all year, so getTimezoneOffset() there is -420. vitest.config.mts runs the
// tests in America/New_York; if that ever stops working, this fails instead of the date
// tests passing only because the machine happens to be in Jakarta.
describe("test process timezone", () => {
  it("is not Jakarta's", () => {
    expect(process.env.TZ).toBe("America/New_York");
    const jakartaOffset = -7 * 60;
    for (const probe of [
      "2026-01-15T05:00:00Z",
      "2026-07-15T05:00:00Z",
      "2026-10-25T23:30:00Z",
    ]) {
      expect(new Date(probe).getTimezoneOffset()).not.toBe(jakartaOffset);
    }
    expect(Intl.DateTimeFormat().resolvedOptions().timeZone).toBe(
      "America/New_York",
    );
  });
});

describe("formatDate", () => {
  it.each([
    { input: "2026-10-25", want: "25 Okt 2026" },
    { input: "2026-01-01", want: "1 Jan 2026" },
    { input: "2026-12-31", want: "31 Des 2026" },
    { input: "2026-05-05", want: "5 Mei 2026" },
    { input: "2026-08-17", want: "17 Agu 2026" },
    { input: "2028-02-29", want: "29 Feb 2028" },
  ])("formats $input as $want without shifting the day", ({ input, want }) => {
    expect(formatDate(input)).toBe(want);
  });

  it.each([
    { name: "an empty string", input: "" },
    { name: "a timestamp", input: "2026-10-25T00:00:00Z" },
    { name: "a day-first date", input: "25-10-2026" },
    { name: "a date that does not exist", input: "2026-02-30" },
    { name: "month 13", input: "2026-13-01" },
  ])("throws on $name", ({ input }) => {
    expect(() => formatDate(input)).toThrow(RangeError);
  });
});

describe("formatDateTime", () => {
  it.each([
    {
      name: "an evening in Jakarta",
      input: "2026-10-25T14:05:00Z",
      want: "25 Okt 2026, 21.05",
    },
    {
      name: "a UTC evening that is already the next day in Jakarta",
      input: "2026-10-25T17:30:00Z",
      want: "26 Okt 2026, 00.30",
    },
    {
      name: "the last minute before Jakarta midnight",
      input: "2026-10-25T16:59:00Z",
      want: "25 Okt 2026, 23.59",
    },
    {
      name: "a timestamp with a Jakarta offset",
      input: "2026-10-25T21:05:00+07:00",
      want: "25 Okt 2026, 21.05",
    },
    {
      name: "a timestamp with fractional seconds",
      input: "2026-12-31T17:00:00.123456Z",
      want: "1 Jan 2027, 00.00",
    },
  ])("formats $name", ({ input, want }) => {
    expect(formatDateTime(input)).toBe(want);
  });

  it("accepts a Date", () => {
    expect(formatDateTime(new Date(Date.UTC(2026, 9, 25, 17, 30)))).toBe(
      "26 Okt 2026, 00.30",
    );
  });

  it.each([
    { name: "a timestamp without a zone", input: "2026-10-25T17:30:00" },
    { name: "a calendar date", input: "2026-10-25" },
    { name: "garbage", input: "kemarin" },
    { name: "an impossible time", input: "2026-10-25T25:61:00Z" },
  ])("throws on $name", ({ input }) => {
    expect(() => formatDateTime(input)).toThrow(RangeError);
  });

  it("throws on an invalid Date", () => {
    expect(() => formatDateTime(new Date(Number.NaN))).toThrow(RangeError);
  });
});
