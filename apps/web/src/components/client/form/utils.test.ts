import { afterEach, describe, expect, it } from "vitest";

import {
  calendarDayLabel,
  calendarDayToLocalDate,
  calendarMonthName,
  calendarWeekdayName,
  datePickerMonths,
  datePickerOpenMonth,
  isDayWithin,
  localDateToCalendarDay,
  searchSelectShownLabel,
} from "./utils";

describe("searchSelectShownLabel", () => {
  const options = [
    { value: "p1", label: "Kos Melati" },
    { value: "p2", label: "Kos Anggrek" },
  ];

  it.each([
    {
      name: "nothing chosen",
      value: "",
      remembered: "Kos Mawar",
      want: undefined,
    },
    {
      name: "a choice in the list",
      value: "p2",
      remembered: "Kos Mawar",
      want: "Kos Anggrek",
    },
    {
      name: "a choice the search moved out of the list",
      value: "p9",
      remembered: "Kos Mawar",
      want: "Kos Mawar",
    },
    {
      name: "a choice nobody remembers",
      value: "p9",
      remembered: undefined,
      want: undefined,
    },
  ])("shows $name", ({ value, remembered, want }) => {
    expect(searchSelectShownLabel(value, options, remembered)).toBe(want);
  });
});

describe("calendarDayToLocalDate and localDateToCalendarDay", () => {
  const zones = [
    "Asia/Jakarta",
    "UTC",
    "America/Los_Angeles",
    "Pacific/Kiritimati",
    "Pacific/Pago_Pago",
  ];
  const days = [
    "2026-11-15",
    "2026-01-31",
    "2026-02-28",
    "2028-02-29",
    "2026-12-31",
    "2027-01-01",
    "2026-03-29",
  ];
  const originalZone = process.env.TZ;
  afterEach(() => {
    process.env.TZ = originalZone;
  });

  it.each(zones)("keeps every day the same on a device in %s", (zone) => {
    process.env.TZ = zone;
    for (const day of days) {
      const date = calendarDayToLocalDate(day);
      expect(date?.getHours()).toBe(0);
      expect(localDateToCalendarDay(date as Date)).toBe(day);
    }
  });

  it.each([
    { name: "an empty value", input: "" },
    { name: "a day that does not exist", input: "2026-02-30" },
    { name: "29 February outside a leap year", input: "2027-02-29" },
    { name: "31 April", input: "2026-04-31" },
    { name: "a timestamp", input: "2026-11-15T00:00:00Z" },
    { name: "a day-first date", input: "15-11-2026" },
  ])("gives undefined for $name", ({ input }) => {
    expect(calendarDayToLocalDate(input)).toBeUndefined();
  });

  it("keeps years before 100 as typed", () => {
    expect(
      localDateToCalendarDay(calendarDayToLocalDate("0099-05-01") as Date),
    ).toBe("0099-05-01");
  });
});

describe("isDayWithin", () => {
  it.each([
    { day: "2026-11-09", min: undefined, max: undefined, want: true },
    { day: "2026-11-09", min: "2026-11-09", max: "2026-11-09", want: true },
    { day: "2026-11-08", min: "2026-11-09", max: undefined, want: false },
    { day: "2026-11-10", min: undefined, max: "2026-11-09", want: false },
    { day: "2027-01-01", min: "2026-12-31", max: "2027-01-09", want: true },
  ])("$day in [$min, $max] is $want", ({ day, min, max, want }) => {
    expect(isDayWithin(day, min, max)).toBe(want);
  });
});

describe("datePickerMonths", () => {
  const month = (date: Date) => localDateToCalendarDay(date);

  it("offers ten years back and five ahead without limits", () => {
    const { start, end } = datePickerMonths(
      "2026-11-09",
      "",
      undefined,
      undefined,
    );
    expect(month(start)).toBe("2016-01-01");
    expect(month(end)).toBe("2031-12-01");
  });

  it("follows the field's limits", () => {
    const { start, end } = datePickerMonths(
      "2026-11-09",
      "",
      "2026-11-10",
      "2027-01-09",
    );
    expect(month(start)).toBe("2026-11-01");
    expect(month(end)).toBe("2027-01-01");
  });

  it("still reaches a chosen day outside the default years", () => {
    const { start } = datePickerMonths(
      "2026-11-09",
      "2012-03-15",
      undefined,
      undefined,
    );
    expect(month(start)).toBe("2012-03-01");
  });
});

describe("datePickerOpenMonth", () => {
  const month = (date: Date) => localDateToCalendarDay(date);

  it.each([
    {
      name: "the chosen day's month",
      value: "2026-01-31",
      min: undefined,
      max: undefined,
      want: "2026-01-01",
    },
    {
      name: "today's month when empty",
      value: "",
      min: undefined,
      max: undefined,
      want: "2026-11-01",
    },
    {
      name: "the first allowed month when today is before it",
      value: "",
      min: "2026-12-02",
      max: undefined,
      want: "2026-12-01",
    },
    {
      name: "the last allowed month when today is after it",
      value: "",
      min: undefined,
      max: "2026-10-31",
      want: "2026-10-01",
    },
  ])("opens on $name", ({ value, min, max, want }) => {
    expect(month(datePickerOpenMonth("2026-11-09", value, min, max))).toBe(
      want,
    );
  });
});

describe("calendar labels", () => {
  it("names months and weekdays in Bahasa Indonesia, weeks from Monday", () => {
    const monday = calendarDayToLocalDate("2026-11-09") as Date;
    const week = Array.from({ length: 7 }, (_, i) =>
      calendarWeekdayName(new Date(2026, 10, 9 + i)),
    );
    expect(calendarMonthName(monday)).toBe("November");
    expect(week).toEqual(["Sen", "Sel", "Rab", "Kam", "Jum", "Sab", "Min"]);
  });

  it("labels a day for screen readers", () => {
    const day = calendarDayToLocalDate("2026-11-15") as Date;
    expect(calendarDayLabel(day, { today: false, selected: false })).toBe(
      "15 November 2026",
    );
    expect(calendarDayLabel(day, { today: true, selected: true })).toBe(
      "15 November 2026, hari ini, dipilih",
    );
  });
});
