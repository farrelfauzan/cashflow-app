import { describe, expect, it } from "vitest";

import type { Unit } from "@/lib/api/generated/model";

import type { LeaseFormValues } from "./type";
import {
  addDays,
  anniversary,
  BAD_DEPOSIT_MESSAGE,
  BAD_RENT_MESSAGE,
  billAllHint,
  billedFromOptions,
  billingExplanation,
  checkLeaseForm,
  defaultBilledFrom,
  firstAnniversaryAfter,
  firstLeaseError,
  freeUnits,
  isAnniversary,
  isCalendarDate,
  isLeaseFormField,
  leaseFormSubtitle,
  NO_BILLED_FROM_MESSAGE,
  NO_COLLECTOR,
  NO_PROPERTY_MESSAGE,
  NO_START_MESSAGE,
  NO_UNIT_MESSAGE,
  periodStartOn,
  rentSuggestion,
  rentTextFor,
  startDateHint,
  startTiming,
} from "./utils";

const TODAY = "2026-10-28";

// The same cases as the API's TestAnniversary (services/api/internal/tenant/anniversary_test.go),
// so the two rules cannot drift apart.
describe("anniversary", () => {
  it.each([
    {
      name: "n=0 is the start date",
      start: "2026-10-15",
      n: 0,
      want: "2026-10-15",
    },
    {
      name: "n=0 on a 31st is the start date",
      start: "2027-01-31",
      n: 0,
      want: "2027-01-31",
    },
    {
      name: "the next month, same day",
      start: "2026-10-15",
      n: 1,
      want: "2026-11-15",
    },
    {
      name: "across the new year",
      start: "2026-12-15",
      n: 2,
      want: "2027-02-15",
    },
    {
      name: "a whole year later",
      start: "2026-10-15",
      n: 12,
      want: "2027-10-15",
    },
    {
      name: "31 Jan clamps to 28 Feb",
      start: "2027-01-31",
      n: 1,
      want: "2027-02-28",
    },
    {
      name: "31 Jan in a leap year clamps to 29 Feb",
      start: "2028-01-31",
      n: 1,
      want: "2028-02-29",
    },
    {
      name: "30 Jan clamps to 28 Feb",
      start: "2027-01-30",
      n: 1,
      want: "2027-02-28",
    },
    {
      name: "29 Jan clamps to 28 Feb",
      start: "2027-01-29",
      n: 1,
      want: "2027-02-28",
    },
    {
      name: "29 Jan in a leap year is 29 Feb",
      start: "2028-01-29",
      n: 1,
      want: "2028-02-29",
    },
    {
      name: "counted from the start, not the previous anniversary",
      start: "2027-01-31",
      n: 2,
      want: "2027-03-31",
    },
    {
      name: "31 Aug clamps to 30 Sep",
      start: "2026-08-31",
      n: 1,
      want: "2026-09-30",
    },
    {
      name: "29 Feb start in a common year is 28 Feb",
      start: "2028-02-29",
      n: 12,
      want: "2029-02-28",
    },
    {
      name: "29 Feb start in the next leap year is 29 Feb",
      start: "2028-02-29",
      n: 48,
      want: "2032-02-29",
    },
  ])("$name", ({ start, n, want }) => {
    expect(anniversary(start, n)).toBe(want);
  });

  it("walks a 31st start through a whole year", () => {
    const want = [
      "2027-01-31",
      "2027-02-28",
      "2027-03-31",
      "2027-04-30",
      "2027-05-31",
      "2027-06-30",
      "2027-07-31",
      "2027-08-31",
      "2027-09-30",
      "2027-10-31",
      "2027-11-30",
      "2027-12-31",
      "2028-01-31",
    ];
    want.forEach((date, n) => expect(anniversary("2027-01-31", n)).toBe(date));
  });
});

// TestIsAnniversary.
describe("isAnniversary", () => {
  it.each([
    {
      name: "the start date itself",
      start: "2026-09-15",
      date: "2026-09-15",
      want: true,
    },
    {
      name: "the next month",
      start: "2026-09-15",
      date: "2026-10-15",
      want: true,
    },
    {
      name: "a year later",
      start: "2026-09-15",
      date: "2027-09-15",
      want: true,
    },
    {
      name: "the day before an anniversary",
      start: "2026-09-15",
      date: "2026-10-14",
      want: false,
    },
    {
      name: "the day after an anniversary",
      start: "2026-09-15",
      date: "2026-10-16",
      want: false,
    },
    {
      name: "another day of the month",
      start: "2026-09-15",
      date: "2026-10-20",
      want: false,
    },
    {
      name: "before the start, even on the same day of an earlier month",
      start: "2026-09-15",
      date: "2026-08-15",
      want: false,
    },
    {
      name: "the day before the start",
      start: "2026-09-15",
      date: "2026-09-14",
      want: false,
    },
    {
      name: "a month-end start: 28 Feb is its anniversary",
      start: "2027-01-31",
      date: "2027-02-28",
      want: true,
    },
    {
      name: "a month-end start: 15 Feb is not",
      start: "2027-01-31",
      date: "2027-02-15",
      want: false,
    },
    {
      name: "a month-end start: 27 Feb is not",
      start: "2027-01-31",
      date: "2027-02-27",
      want: false,
    },
    {
      name: "a month-end start: 31 Mar is back on the 31st",
      start: "2027-01-31",
      date: "2027-03-31",
      want: true,
    },
    {
      name: "a month-end start: 30 Mar is not",
      start: "2027-01-31",
      date: "2027-03-30",
      want: false,
    },
    {
      name: "a month-end start: 30 Apr is its anniversary",
      start: "2027-01-31",
      date: "2027-04-30",
      want: true,
    },
    {
      name: "a leap year: 29 Feb is an anniversary of 31 Jan",
      start: "2028-01-31",
      date: "2028-02-29",
      want: true,
    },
    {
      name: "a leap year: 28 Feb is not",
      start: "2028-01-31",
      date: "2028-02-28",
      want: false,
    },
  ])("$name", ({ start, date, want }) => {
    expect(isAnniversary(start, date)).toBe(want);
  });
});

// TestFirstAnniversaryAfter.
describe("firstAnniversaryAfter", () => {
  it.each([
    {
      name: "d before the start is the start",
      start: "2026-12-01",
      date: "2026-11-02",
      want: "2026-12-01",
    },
    {
      name: "d the day before the start is the start",
      start: "2026-12-01",
      date: "2026-11-30",
      want: "2026-12-01",
    },
    {
      name: "d on the start is the next anniversary",
      start: "2026-12-01",
      date: "2026-12-01",
      want: "2027-01-01",
    },
    {
      name: "d on an anniversary is the next one",
      start: "2026-09-15",
      date: "2026-10-15",
      want: "2026-11-15",
    },
    {
      name: "d between two anniversaries",
      start: "2026-09-15",
      date: "2026-11-02",
      want: "2026-11-15",
    },
    {
      name: "d the day before an anniversary",
      start: "2026-09-15",
      date: "2026-10-14",
      want: "2026-10-15",
    },
    {
      name: "d the day after an anniversary",
      start: "2026-09-15",
      date: "2026-10-16",
      want: "2026-11-15",
    },
    {
      name: "across the new year",
      start: "2026-11-20",
      date: "2026-12-25",
      want: "2027-01-20",
    },
    {
      name: "a month-end start: d on 31 Jan",
      start: "2027-01-31",
      date: "2027-01-31",
      want: "2027-02-28",
    },
    {
      name: "a month-end start: d on 15 Feb",
      start: "2027-01-31",
      date: "2027-02-15",
      want: "2027-02-28",
    },
    {
      name: "a month-end start: d on 28 Feb",
      start: "2027-01-31",
      date: "2027-02-28",
      want: "2027-03-31",
    },
    {
      name: "a month-end start: d on 1 Mar",
      start: "2027-01-31",
      date: "2027-03-01",
      want: "2027-03-31",
    },
    {
      name: "a month-end start: d on 31 Mar",
      start: "2027-01-31",
      date: "2027-03-31",
      want: "2027-04-30",
    },
    {
      name: "a leap year: d on 29 Feb",
      start: "2028-01-31",
      date: "2028-02-29",
      want: "2028-03-31",
    },
    {
      name: "a leap year: d on 28 Feb",
      start: "2028-01-31",
      date: "2028-02-28",
      want: "2028-02-29",
    },
  ])("$name", ({ start, date, want }) => {
    const got = firstAnniversaryAfter(start, date);
    expect(got).toBe(want);
    if (date >= start) expect(got > date).toBe(true);
    expect(isAnniversary(start, got)).toBe(true);
  });
});

describe("periodStartOn", () => {
  it.each([
    {
      name: "on the start date",
      start: "2026-09-15",
      date: "2026-09-15",
      want: "2026-09-15",
    },
    {
      name: "in the first period",
      start: "2026-09-15",
      date: "2026-10-14",
      want: "2026-09-15",
    },
    {
      name: "on an anniversary",
      start: "2026-09-15",
      date: "2026-10-15",
      want: "2026-10-15",
    },
    {
      name: "between anniversaries",
      start: "2026-09-15",
      date: "2026-10-28",
      want: "2026-10-15",
    },
    {
      name: "before a month-end anniversary",
      start: "2026-08-31",
      date: "2026-10-28",
      want: "2026-09-30",
    },
    {
      name: "on a clamped month end",
      start: "2027-01-31",
      date: "2027-02-28",
      want: "2027-02-28",
    },
  ])("$name", ({ start, date, want }) => {
    expect(periodStartOn(start, date)).toBe(want);
  });
});

describe("dates", () => {
  it.each([
    { date: "2026-10-28", days: 7, want: "2026-11-04" },
    { date: "2026-12-28", days: 7, want: "2027-01-04" },
    { date: "2028-02-27", days: 2, want: "2028-02-29" },
  ])("$date plus $days is $want", ({ date, days, want }) => {
    expect(addDays(date, days)).toBe(want);
  });

  it.each([
    { date: "2026-10-28", want: true },
    { date: "2028-02-29", want: true },
    { date: "2027-02-29", want: false },
    { date: "2026-13-01", want: false },
    { date: "", want: false },
    { date: "28/10/2026", want: false },
  ])("'$date' is a calendar date: $want", ({ date, want }) => {
    expect(isCalendarDate(date)).toBe(want);
  });

  it.each([
    { start: "2026-09-15", want: "past" },
    { start: TODAY, want: "today" },
    { start: "2026-12-15", want: "future" },
  ])("$start is $want", ({ start, want }) => {
    expect(startTiming(start, TODAY)).toBe(want);
  });
});

describe("billedFromOptions", () => {
  const values = (start: string, today: string) =>
    billedFromOptions(start, today).map((option) => option.value);

  it("offers the later anniversaries up to the first one after today (the board's case)", () => {
    expect(billedFromOptions("2026-09-15", TODAY)).toEqual([
      { value: "2026-10-15", label: "15 Okt 2026" },
      { value: "2026-11-15", label: "15 Nov 2026" },
    ]);
  });

  it("offers only the next anniversary while the lease is in its first period", () => {
    expect(values("2026-10-15", TODAY)).toEqual(["2026-11-15"]);
  });

  it("follows the month ends of a 31st start", () => {
    expect(values("2026-08-31", TODAY)).toEqual(["2026-09-30", "2026-10-31"]);
    expect(values("2027-01-31", "2027-03-15")).toEqual([
      "2027-02-28",
      "2027-03-31",
    ]);
  });

  it("follows a leap February", () => {
    expect(values("2028-01-31", "2028-03-01")).toEqual([
      "2028-02-29",
      "2028-03-31",
    ]);
  });

  it("stops at the API's latest allowed date when today is an anniversary", () => {
    expect(values("2026-08-28", TODAY)).toEqual([
      "2026-09-28",
      "2026-10-28",
      "2026-11-28",
    ]);
  });

  it("offers every option as an anniversary the API accepts", () => {
    for (const option of billedFromOptions("2025-12-31", TODAY)) {
      expect(isAnniversary("2025-12-31", option.value)).toBe(true);
      expect(option.value <= firstAnniversaryAfter("2025-12-31", TODAY)).toBe(
        true,
      );
    }
  });
});

describe("defaultBilledFrom", () => {
  it("starts on the period running today", () => {
    expect(
      defaultBilledFrom(billedFromOptions("2026-09-15", TODAY), TODAY),
    ).toBe("2026-10-15");
  });

  it("starts on the next anniversary in the first period", () => {
    expect(
      defaultBilledFrom(billedFromOptions("2026-10-15", TODAY), TODAY),
    ).toBe("2026-11-15");
  });

  it("is empty without options", () => {
    expect(defaultBilledFrom([], TODAY)).toBe("");
  });
});

describe("billingExplanation", () => {
  it("bills every period since the start: past periods today, the current one in 7 days (CLAUDE.md's example)", () => {
    expect(billingExplanation("2026-09-15", "2026-09-15", TODAY)).toBe(
      "Tanggal mulai ini sudah lewat, jadi setiap bulan sejak 15 Sep 2026 ikut ditagih. Bulan yang sudah lewat jatuh tempo hari ini (28 Okt); bulan berjalan jatuh tempo 4 Nov. Kalau pemilik sudah menerima uang sewa bulan-bulan itu langsung, pilih bulan mulai tagih yang lebih baru.",
    );
  });

  it("bills from the current period only", () => {
    expect(billingExplanation("2026-09-15", "2026-10-15", TODAY)).toBe(
      "Tanggal mulai ini sudah lewat, jadi bulan berjalan sejak 15 Okt 2026 ikut ditagih, jatuh tempo 4 Nov.",
    );
  });

  it("a lease in its first period bills that period, due in 7 days", () => {
    expect(billingExplanation("2026-10-15", "2026-10-15", TODAY)).toBe(
      "Tanggal mulai ini sudah lewat, jadi bulan berjalan sejak 15 Okt 2026 ikut ditagih, jatuh tempo 4 Nov.",
    );
  });

  it("billing from the next anniversary leaves the owner every period so far", () => {
    expect(billingExplanation("2026-09-15", "2026-11-15", TODAY)).toBe(
      "Bulan sebelum 15 Nov 2026 tidak ditagih. Tagihan pertama untuk bulan mulai 15 Nov 2026, jatuh tempo 15 Nov.",
    );
  });

  it("finds the current period of a 31st start at a month end", () => {
    expect(billingExplanation("2026-08-31", "2026-09-30", TODAY)).toBe(
      "Tanggal mulai ini sudah lewat, jadi bulan berjalan sejak 30 Sep 2026 ikut ditagih, jatuh tempo 4 Nov.",
    );
    expect(billingExplanation("2026-08-31", "2026-08-31", TODAY)).toContain(
      "setiap bulan sejak 31 Agu 2026 ikut ditagih",
    );
  });

  it("crosses the new year for the current period's due date", () => {
    expect(
      billingExplanation("2026-11-20", "2026-11-20", "2026-12-28"),
    ).toContain("bulan berjalan jatuh tempo 4 Jan");
  });
});

describe("copy", () => {
  it("names the start date in the first radio's line", () => {
    expect(billAllHint("2026-09-15")).toBe(
      "Semua bulan sejak 15 Sep 2026 dibuatkan tagihan.",
    );
  });

  it.each([
    {
      start: "2026-09-15",
      want: "Juga jadi tanggal tagihan tiap bulan. Mulai tanggal 15, jatuh tempo tiap tanggal 15. Tanggal 29 sampai 31 jatuh di hari terakhir bulan yang lebih pendek.",
    },
    {
      start: "2026-08-03",
      want: "Juga jadi tanggal tagihan tiap bulan. Mulai tanggal 3, jatuh tempo tiap tanggal 3. Tanggal 29 sampai 31 jatuh di hari terakhir bulan yang lebih pendek.",
    },
    {
      start: "",
      want: "Juga jadi tanggal tagihan tiap bulan. Tanggal 29 sampai 31 jatuh di hari terakhir bulan yang lebih pendek.",
    },
  ])("the start hint for '$start'", ({ start, want }) => {
    expect(startDateHint(start)).toBe(want);
  });

  it("names the tenant and their number in the subtitle", () => {
    expect(leaseFormSubtitle("Bu Dewi Lestari", "+6281234567899")).toBe(
      "Bu Dewi Lestari · 0812-3456-7899",
    );
  });
});

describe("units and rent", () => {
  const unit = (over: Partial<Unit>): Unit => ({
    id: "u",
    propertyId: "p",
    label: "Kamar 1",
    occupied: false,
    createdAt: "2026-10-28T03:00:00Z",
    updatedAt: "2026-10-28T03:00:00Z",
    ...over,
  });

  it("keeps only the free units, in the API's order", () => {
    expect(
      freeUnits([
        unit({ id: "a" }),
        unit({ id: "b", occupied: true }),
        unit({ id: "c" }),
      ]).map((u) => u.id),
    ).toEqual(["a", "c"]);
  });

  it("suggests the unit's usual rent", () => {
    expect(rentSuggestion(unit({ defaultRent: 1_250_000 }))).toBe(
      "Usulan dari sewa biasa unit: Rp1.250.000.",
    );
    expect(rentTextFor(unit({ defaultRent: 1_250_000 }))).toBe("1.250.000");
  });

  it("suggests nothing without a usual rent", () => {
    expect(rentSuggestion(unit({}))).toBeNull();
    expect(rentSuggestion(undefined)).toBeNull();
    expect(rentTextFor(unit({}))).toBe("");
  });
});

describe("checkLeaseForm", () => {
  const values = (over: Partial<LeaseFormValues> = {}): LeaseFormValues => ({
    tenantId: "t",
    unitId: "u",
    startDate: "2026-11-01",
    billing: "start",
    billedFrom: "",
    rent: "1.250.000",
    deposit: "",
    collector: NO_COLLECTOR,
    ...over,
  });

  it("sends integers and leaves out what is not given", () => {
    expect(checkLeaseForm(values(), "p", TODAY)).toEqual({
      ok: true,
      input: {
        tenantId: "t",
        unitId: "u",
        startDate: "2026-11-01",
        rentAmount: 1_250_000,
      },
    });
  });

  it("sends a deposit of 0 typed as asked", () => {
    const checked = checkLeaseForm(values({ deposit: "0" }), "p", TODAY);
    expect(checked.ok && checked.input.depositAmount).toBe(0);
  });

  it("refuses a rent of 0", () => {
    expect(checkLeaseForm(values({ rent: "0" }), "p", TODAY)).toEqual({
      ok: false,
      errors: { rentAmount: BAD_RENT_MESSAGE },
    });
  });

  it("sends the deposit and the collector when given", () => {
    expect(
      checkLeaseForm(
        values({ deposit: "2.500.000", collector: "member-1" }),
        "p",
        TODAY,
      ),
    ).toEqual({
      ok: true,
      input: {
        tenantId: "t",
        unitId: "u",
        startDate: "2026-11-01",
        rentAmount: 1_250_000,
        depositAmount: 2_500_000,
        collectorMemberId: "member-1",
      },
    });
  });

  it("sends billedFrom for a past start with the second choice", () => {
    const checked = checkLeaseForm(
      values({
        startDate: "2026-09-15",
        billing: "pick",
        billedFrom: "2026-10-15",
      }),
      "p",
      TODAY,
    );
    expect(checked.ok && checked.input.billedFrom).toBe("2026-10-15");
  });

  it.each([
    {
      name: "the first choice",
      over: {
        startDate: "2026-09-15",
        billing: "start" as const,
        billedFrom: "2026-10-15",
      },
    },
    {
      name: "a start today",
      over: {
        startDate: TODAY,
        billing: "pick" as const,
        billedFrom: "2026-11-28",
      },
    },
    {
      name: "a booking",
      over: {
        startDate: "2026-12-01",
        billing: "pick" as const,
        billedFrom: "2027-01-01",
      },
    },
  ])("sends no billedFrom with $name", ({ over }) => {
    const checked = checkLeaseForm(values(over), "p", TODAY);
    expect(checked.ok).toBe(true);
    expect(checked.ok && "billedFrom" in checked.input).toBe(false);
  });

  it("refuses a billedFrom the API would refuse", () => {
    expect(
      checkLeaseForm(
        values({
          startDate: "2026-09-15",
          billing: "pick",
          billedFrom: "2026-12-15",
        }),
        "p",
        TODAY,
      ),
    ).toEqual({ ok: false, errors: { billedFrom: NO_BILLED_FROM_MESSAGE } });
  });

  it("finds every missing field", () => {
    expect(
      checkLeaseForm(
        values({ unitId: "", startDate: "", rent: "" }),
        "",
        TODAY,
      ),
    ).toEqual({
      ok: false,
      errors: {
        propertyId: NO_PROPERTY_MESSAGE,
        unitId: NO_UNIT_MESSAGE,
        startDate: NO_START_MESSAGE,
        rentAmount: BAD_RENT_MESSAGE,
      },
    });
  });

  it.each(["0", "abc", "1.5"])("refuses the rent '%s'", (rent) => {
    expect(checkLeaseForm(values({ rent }), "p", TODAY)).toEqual({
      ok: false,
      errors: { rentAmount: BAD_RENT_MESSAGE },
    });
  });

  it("refuses a deposit that is not a whole amount", () => {
    expect(checkLeaseForm(values({ deposit: "1,5" }), "p", TODAY)).toEqual({
      ok: false,
      errors: { depositAmount: BAD_DEPOSIT_MESSAGE },
    });
  });
});

describe("form fields", () => {
  it("focuses the first bad field in the form's order", () => {
    expect(
      firstLeaseError({
        rentAmount: "x",
        startDate: "y",
        collectorMemberId: "z",
      }),
    ).toBe("startDate");
    expect(firstLeaseError({})).toBeNull();
  });

  it.each([
    { field: "unitId", want: true },
    { field: "billedFrom", want: true },
    { field: "tenantId", want: false },
    { field: null, want: false },
  ])("$field is a field of the form: $want", ({ field, want }) => {
    expect(isLeaseFormField(field)).toBe(want);
  });
});
