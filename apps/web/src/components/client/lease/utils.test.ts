import { describe, expect, it } from "vitest";

import type { ChargeType, Lease, Unit } from "@/lib/api/generated/model";

import type { ChargeRow, LeaseFormValues, LeaseTermsValues } from "./type";
import {
  BAD_CHARGE_AMOUNT_MESSAGE,
  chargeAmountId,
  chargeAmountLabel,
  chargeApiFields,
  chargeEntriesFromLease,
  chargeRowForApiField,
  chargeRows,
  checkChargeTypeName,
  checkLeaseCharges,
  EMPTY_CHARGE_TYPE_NAME_MESSAGE,
  LONG_CHARGE_TYPE_NAME_MESSAGE,
  meteredChargeTypeAddedText,
  withChargeAmount,
  withChargeChecked,
  cancelBookingText,
  checkLeaseTerms,
  checkMoveOut,
  checkPlannedEnd,
  collectorOptions,
  currentPlannedEndText,
  editStartBox,
  END_AFTER_TODAY_MESSAGE,
  END_BEFORE_START_MESSAGE,
  keepsDisabledCollector,
  leaseTermsValues,
  leaseUnitText,
  moveOutHint,
  NO_DATE_MESSAGE,
  PLANNED_END_BEFORE_START_MESSAGE,
  PLANNED_END_BEFORE_TODAY_MESSAGE,
  plannedEndHint,
  plannedEndSuggestion,
  addDays,
  anniversary,
  BAD_DEPOSIT_MESSAGE,
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
  NO_TERM_MESSAGE,
  leaseTermFor,
  shortestTerm,
  TERM_PERIOD_WORDS,
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
    expect(billingExplanation("2026-09-15", "2026-09-15", TODAY, TODAY)).toBe(
      "Tanggal mulai ini sudah lewat, jadi setiap bulan sejak 15 Sep 2026 ikut ditagih. Bulan yang sudah lewat jatuh tempo hari ini (28 Okt); bulan berjalan jatuh tempo 4 Nov. Kalau pemilik sudah menerima uang sewa bulan-bulan itu langsung, pilih bulan mulai tagih yang lebih baru.",
    );
  });

  it("bills from the current period only", () => {
    expect(billingExplanation("2026-09-15", "2026-10-15", TODAY, TODAY)).toBe(
      "Tanggal mulai ini sudah lewat, jadi bulan berjalan sejak 15 Okt 2026 ikut ditagih, jatuh tempo 4 Nov.",
    );
  });

  it("a lease in its first period bills that period, due in 7 days", () => {
    expect(billingExplanation("2026-10-15", "2026-10-15", TODAY, TODAY)).toBe(
      "Tanggal mulai ini sudah lewat, jadi bulan berjalan sejak 15 Okt 2026 ikut ditagih, jatuh tempo 4 Nov.",
    );
  });

  it("billing from the next anniversary leaves the owner every period so far", () => {
    expect(billingExplanation("2026-09-15", "2026-11-15", TODAY, TODAY)).toBe(
      "Bulan sebelum 15 Nov 2026 tidak ditagih. Tagihan pertama untuk bulan mulai 15 Nov 2026, jatuh tempo 15 Nov.",
    );
  });

  it("finds the current period of a 31st start at a month end", () => {
    expect(billingExplanation("2026-08-31", "2026-09-30", TODAY, TODAY)).toBe(
      "Tanggal mulai ini sudah lewat, jadi bulan berjalan sejak 30 Sep 2026 ikut ditagih, jatuh tempo 4 Nov.",
    );
    expect(
      billingExplanation("2026-08-31", "2026-08-31", TODAY, TODAY),
    ).toContain("setiap bulan sejak 31 Agu 2026 ikut ditagih");
  });

  it("crosses the new year for the current period's due date", () => {
    expect(
      billingExplanation(
        "2026-11-20",
        "2026-11-20",
        "2026-12-28",
        "2026-12-28",
      ),
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
    terms: [{ months: 1, price: 1_250_000 }],
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
});

describe("shortestTerm", () => {
  it.each([
    { name: "a single term", months: [12], want: 12 },
    { name: "several terms, fewest months first", months: [1, 3, 12], want: 1 },
    { name: "terms in any order", months: [12, 6, 3], want: 3 },
    { name: "no term", months: [], want: null },
  ] as const)("takes $want from $name", ({ months, want }) => {
    expect(shortestTerm(months.map((m) => ({ months: m })))).toBe(want);
  });

  it("takes nothing from no list", () => {
    expect(shortestTerm(undefined)).toBeNull();
  });
});

describe("leaseTermFor", () => {
  it.each([
    {
      name: "the shortest priced term",
      terms: [
        { months: 12, price: 15_000_000 },
        { months: 1, price: 1_300_000 },
      ],
      want: 1,
    },
    {
      name: "a priced longer term over an unpriced shorter one",
      terms: [{ months: 1 }, { months: 12, price: 15_000_000 }],
      want: 12,
    },
    {
      name: "the shortest term when none is priced",
      terms: [{ months: 6 }, { months: 3 }],
      want: 3,
    },
    { name: "no term", terms: [], want: null },
  ] as const)("takes $want for $name", ({ terms, want }) => {
    expect(leaseTermFor({ terms: [...terms] })).toBe(want);
  });

  it("takes nothing when no unit is chosen", () => {
    expect(leaseTermFor(undefined)).toBeNull();
  });
});

describe("TERM_PERIOD_WORDS", () => {
  it("names one period of every term", () => {
    expect(TERM_PERIOD_WORDS).toEqual({
      1: "bulan",
      3: "3 bulan",
      6: "6 bulan",
      12: "tahun",
    });
  });
});

describe("checkLeaseForm", () => {
  const values = (over: Partial<LeaseFormValues> = {}): LeaseFormValues => ({
    tenantId: "t",
    unitId: "u",
    startDate: "2026-11-01",
    billing: "start",
    billedFrom: "",
    termMonths: 1,
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
        termMonths: 1,
      },
    });
  });

  it("sends a deposit of 0 typed as asked", () => {
    const checked = checkLeaseForm(values({ deposit: "0" }), "p", TODAY);
    expect(checked.ok && checked.input.depositAmount).toBe(0);
  });

  it("sends the unit's shortest term", () => {
    const checked = checkLeaseForm(values({ termMonths: 12 }), "p", TODAY);
    expect(checked.ok && checked.input.termMonths).toBe(12);
  });

  it("refuses a unit with no term under the unit", () => {
    expect(checkLeaseForm(values({ termMonths: null }), "p", TODAY)).toEqual({
      ok: false,
      errors: { unitId: NO_TERM_MESSAGE },
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
        termMonths: 1,
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
        values({ unitId: "", startDate: "", termMonths: null }),
        "",
        TODAY,
      ),
    ).toEqual({
      ok: false,
      errors: {
        propertyId: NO_PROPERTY_MESSAGE,
        unitId: NO_UNIT_MESSAGE,
        startDate: NO_START_MESSAGE,
      },
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
        depositAmount: "x",
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

describe("billingExplanation for a lease being changed", () => {
  it("names the day the lease was entered when it is not today", () => {
    expect(
      billingExplanation("2026-09-15", "2026-09-15", "2026-10-20", TODAY),
    ).toBe(
      "Tanggal mulai ini sudah lewat, jadi setiap bulan sejak 15 Sep 2026 ikut ditagih. Bulan yang sudah lewat jatuh tempo 20 Okt, hari sewa ini dimasukkan; bulan berjalan jatuh tempo 27 Okt. Kalau pemilik sudah menerima uang sewa bulan-bulan itu langsung, pilih bulan mulai tagih yang lebih baru.",
    );
  });

  it("counts the current period from the day it was entered", () => {
    expect(
      billingExplanation("2026-08-15", "2026-09-15", "2026-09-20", TODAY),
    ).toBe(
      "Tanggal mulai ini sudah lewat, jadi bulan berjalan sejak 15 Sep 2026 ikut ditagih, jatuh tempo 27 Sep.",
    );
    expect(
      billingExplanation("2026-08-15", "2026-10-15", "2026-09-20", TODAY),
    ).toBe(
      "Bulan sebelum 15 Okt 2026 tidak ditagih. Tagihan pertama untuk bulan mulai 15 Okt 2026, jatuh tempo 15 Okt.",
    );
  });
});

describe("billedFromOptions for a lease being changed", () => {
  it("stops at the first anniversary after the day it was entered, not today", () => {
    expect(
      billedFromOptions("2026-08-15", "2026-09-20").map((o) => o.value),
    ).toEqual(["2026-09-15", "2026-10-15"]);
  });
});

const leaseOf = (over: Partial<Lease> = {}): Lease => ({
  id: "11111111-1111-4111-8111-111111111111",
  tenantId: "22222222-2222-4222-8222-222222222222",
  unitId: "33333333-3333-4333-8333-333333333333",
  unitLabel: "Kamar 07",
  propertyId: "44444444-4444-4444-8444-444444444444",
  propertyName: "Kos Melati",
  startDate: "2026-09-15",
  enteredOn: TODAY,
  billedFrom: "2026-09-15",
  termMonths: 1,
  periodRent: 1_500_000,
  depositAmount: 3_000_000,
  status: "active",
  charges: [],
  createdAt: "2026-10-28T03:00:00Z",
  updatedAt: "2026-10-28T03:00:00Z",
  ...over,
});

describe("editStartBox", () => {
  it.each([
    {
      name: "a start before the day it was entered shows the billing choice",
      start: "2026-09-15",
      enteredOn: TODAY,
      want: "billing",
    },
    {
      name: "a start on the day it was entered shows nothing",
      start: "2026-10-20",
      enteredOn: "2026-10-20",
      want: "none",
    },
    {
      name: "a start after it was entered but before today shows nothing",
      start: "2026-10-22",
      enteredOn: "2026-10-20",
      want: "none",
    },
    {
      name: "a start today shows nothing",
      start: TODAY,
      enteredOn: "2026-10-20",
      want: "none",
    },
    {
      name: "a start after today shows the booking note",
      start: "2026-12-05",
      enteredOn: "2026-10-20",
      want: "booking",
    },
    {
      name: "an empty date shows nothing",
      start: "",
      enteredOn: TODAY,
      want: "none",
    },
  ])("$name", ({ start, enteredOn, want }) => {
    expect(editStartBox(start, enteredOn, TODAY)).toBe(want);
  });
});

describe("leaseTermsValues", () => {
  it("starts from the lease's own terms", () => {
    expect(
      leaseTermsValues(
        leaseOf({
          collector: { memberId: "m-1", name: "Mas Dimas", active: true },
        }),
      ),
    ).toEqual({
      startDate: "2026-09-15",
      billing: "start",
      billedFrom: "",
      deposit: "3.000.000",
      collector: "m-1",
    });
  });

  it("picks the later billing start the lease has, and no collector", () => {
    expect(
      leaseTermsValues(leaseOf({ billedFrom: "2026-10-15", depositAmount: 0 })),
    ).toMatchObject({
      billing: "pick",
      billedFrom: "2026-10-15",
      deposit: "0",
      collector: NO_COLLECTOR,
    });
  });
});

describe("collectors", () => {
  const candidates = [
    { memberId: "m-1", name: "Mas Dimas Saputra", karyawan: true },
    { memberId: "m-2", name: "Mbak Sari", karyawan: false },
  ];

  it("offers none, then the candidates", () => {
    expect(collectorOptions(candidates, undefined)).toEqual([
      { value: NO_COLLECTOR, label: "Tidak ada" },
      { value: "m-1", label: "Mas Dimas Saputra" },
      { value: "m-2", label: "Mbak Sari" },
    ]);
  });

  it("adds the lease's disabled collector, who is no longer a candidate", () => {
    expect(
      collectorOptions(candidates, {
        memberId: "m-9",
        name: "Mas Teguh Santoso",
      }).at(-1),
    ).toEqual({ value: "m-9", label: "Mas Teguh Santoso" });
  });

  it("does not repeat a collector who is a candidate", () => {
    expect(
      collectorOptions(candidates, { memberId: "m-1", name: "Mas Dimas" }),
    ).toHaveLength(3);
  });

  it.each([
    {
      name: "the disabled collector still chosen",
      selected: "m-9",
      current: { memberId: "m-9", active: false },
      want: true,
    },
    {
      name: "another collector chosen",
      selected: "m-1",
      current: { memberId: "m-9", active: false },
      want: false,
    },
    {
      name: "an active collector",
      selected: "m-1",
      current: { memberId: "m-1", active: true },
      want: false,
    },
    {
      name: "no collector on the lease",
      selected: NO_COLLECTOR,
      current: undefined,
      want: false,
    },
  ])("warns for $name: $want", ({ selected, current, want }) => {
    expect(keepsDisabledCollector(selected, current)).toBe(want);
  });
});

describe("checkLeaseTerms", () => {
  const values = (over: Partial<LeaseTermsValues> = {}): LeaseTermsValues => ({
    startDate: "2026-09-15",
    billing: "start",
    billedFrom: "",
    deposit: "3.000.000",
    collector: NO_COLLECTOR,
    ...over,
  });
  // Entered on its start date: "Mulai tagih dari" never shows for it.
  const onTime = leaseOf({ startDate: "2026-09-15", enteredOn: "2026-09-15" });
  // Entered late, on 28 Oct.
  const late = leaseOf({ startDate: "2026-09-15", enteredOn: TODAY });

  it("sends the whole terms, leaving billedFrom out for an unchanged start", () => {
    expect(checkLeaseTerms(values({ collector: "m-1" }), onTime)).toEqual({
      ok: true,
      input: {
        startDate: "2026-09-15",
        depositAmount: 3_000_000,
        collectorMemberId: "m-1",
      },
    });
  });

  it("sends a deposit of 0 for an empty field and no collector for none", () => {
    expect(checkLeaseTerms(values({ deposit: "" }), onTime)).toEqual({
      ok: true,
      input: {
        startDate: "2026-09-15",
        depositAmount: 0,
      },
    });
  });

  it("sends the new start date as billedFrom when the start changes and the block is hidden", () => {
    const checked = checkLeaseTerms(
      values({ startDate: "2026-09-20" }),
      onTime,
    );
    expect(checked.ok && checked.input.billedFrom).toBe("2026-09-20");
  });

  it("sends the start date for the first choice while the block shows", () => {
    const checked = checkLeaseTerms(values(), late);
    expect(checked.ok && checked.input.billedFrom).toBe("2026-09-15");
  });

  it("sends the chosen anniversary for the second choice", () => {
    const checked = checkLeaseTerms(
      values({ billing: "pick", billedFrom: "2026-10-15" }),
      late,
    );
    expect(checked.ok && checked.input.billedFrom).toBe("2026-10-15");
  });

  it("follows a changed start while the block shows", () => {
    const checked = checkLeaseTerms(
      values({ startDate: "2026-08-31", billing: "start" }),
      late,
    );
    expect(checked.ok && checked.input.billedFrom).toBe("2026-08-31");
  });

  it("refuses an anniversary the API would refuse", () => {
    expect(
      checkLeaseTerms(
        values({ billing: "pick", billedFrom: "2026-12-15" }),
        late,
      ),
    ).toEqual({ ok: false, errors: { billedFrom: NO_BILLED_FROM_MESSAGE } });
  });

  it("ignores the second choice once the block is hidden", () => {
    const checked = checkLeaseTerms(
      values({ startDate: TODAY, billing: "pick", billedFrom: "2026-10-15" }),
      late,
    );
    expect(checked.ok && checked.input.billedFrom).toBe(TODAY);
  });

  it("finds every bad field", () => {
    expect(
      checkLeaseTerms(values({ startDate: "", deposit: "1,5" }), onTime),
    ).toEqual({
      ok: false,
      errors: {
        startDate: NO_START_MESSAGE,
        depositAmount: BAD_DEPOSIT_MESSAGE,
      },
    });
  });
});

describe("plannedEndSuggestion", () => {
  it.each([
    {
      name: "the day before the next anniversary (the board's case)",
      start: "2026-09-15",
      today: TODAY,
      want: { date: "2026-11-14", nextStart: "2026-11-15" },
    },
    {
      name: "today, when tomorrow is an anniversary",
      start: "2026-09-29",
      today: TODAY,
      want: { date: TODAY, nextStart: "2026-10-29" },
    },
    {
      name: "a 31st start before a 30-day month",
      start: "2026-08-31",
      today: "2026-11-05",
      want: { date: "2026-11-29", nextStart: "2026-11-30" },
    },
    {
      name: "a 31st start before February",
      start: "2026-12-31",
      today: "2027-02-10",
      want: { date: "2027-02-27", nextStart: "2027-02-28" },
    },
    {
      name: "a 30th start in a leap February",
      start: "2027-12-30",
      today: "2028-02-03",
      want: { date: "2028-02-28", nextStart: "2028-02-29" },
    },
    {
      name: "a start today: the day before its second period",
      start: TODAY,
      today: TODAY,
      want: { date: "2026-11-27", nextStart: "2026-11-28" },
    },
    {
      name: "a booking: the day before its second period",
      start: "2026-12-05",
      today: TODAY,
      want: { date: "2027-01-04", nextStart: "2027-01-05" },
    },
  ])("$name", ({ start, today, want }) => {
    expect(plannedEndSuggestion(start, today)).toEqual(want);
  });

  it("says why in the hint", () => {
    expect(
      plannedEndHint({ date: "2026-11-14", nextStart: "2026-11-15" }),
    ).toBe(
      "Usulan: 14 Nov 2026, sehari sebelum bulan berikutnya (15 Nov) dimulai. Tanggal harus hari ini atau sesudahnya, dan tidak sebelum tanggal mulai sewa.",
    );
    expect(currentPlannedEndText("2026-11-14")).toBe(
      "Rencana keluar saat ini: 14 Nov 2026. Menghapusnya mengembalikan tagihan yang tadi dibatalkan.",
    );
  });
});

describe("checkPlannedEnd", () => {
  it.each([
    {
      date: "2026-11-14",
      start: "2026-09-15",
      want: { ok: true, date: "2026-11-14" },
    },
    { date: TODAY, start: "2026-09-15", want: { ok: true, date: TODAY } },
    {
      date: "",
      start: "2026-09-15",
      want: { ok: false, error: NO_DATE_MESSAGE },
    },
    {
      date: "2026-10-27",
      start: "2026-09-15",
      want: { ok: false, error: PLANNED_END_BEFORE_TODAY_MESSAGE },
    },
    {
      date: "2026-12-01",
      start: "2026-12-05",
      want: { ok: false, error: PLANNED_END_BEFORE_START_MESSAGE },
    },
    {
      date: "2026-12-05",
      start: "2026-12-05",
      want: { ok: true, date: "2026-12-05" },
    },
  ])("'$date' for a start on $start", ({ date, start, want }) => {
    expect(checkPlannedEnd(date, start, TODAY)).toEqual(want);
  });
});

describe("checkMoveOut", () => {
  it.each([
    { date: TODAY, want: { ok: true, date: TODAY } },
    { date: "2026-09-15", want: { ok: true, date: "2026-09-15" } },
    { date: "", want: { ok: false, error: NO_DATE_MESSAGE } },
    {
      date: "2026-09-14",
      want: { ok: false, error: END_BEFORE_START_MESSAGE },
    },
    {
      date: "2026-10-29",
      want: { ok: false, error: END_AFTER_TODAY_MESSAGE },
    },
  ])("'$date'", ({ date, want }) => {
    expect(checkMoveOut(date, "2026-09-15", TODAY)).toEqual(want);
  });

  it("names the start date in the hint", () => {
    expect(moveOutHint("2026-09-15")).toBe(
      "Diisi hari ini. Tidak boleh sebelum tanggal mulai sewa (15 Sep 2026) dan tidak boleh setelah hari ini.",
    );
  });
});

describe("lease copy", () => {
  it("names the unit and the booking's start", () => {
    expect(leaseUnitText(leaseOf())).toBe("Kamar 07 · Kos Melati");
    expect(cancelBookingText("2026-12-05")).toBe(
      "Sewa ini belum dimulai (mulai 5 Des 2026). Kalau dibatalkan, unit langsung kosong lagi dan sewa ini tidak akan pernah ditagih.",
    );
  });
});

describe("lease charges", () => {
  const type = (id: string, name: string, metered = false): ChargeType => ({
    id,
    name,
    metered,
    createdAt: "2026-10-28T03:00:00Z",
    updatedAt: "2026-10-28T03:00:00Z",
  });
  const TYPES = [
    type("t-air", "Air", true),
    type("t-clean", "Kebersihan"),
    type("t-park", "Parkir"),
    type("t-wifi", "Wi-Fi"),
  ];
  const row = (
    over: Partial<ChargeRow> & { chargeTypeId: string },
  ): ChargeRow => ({
    name: over.chargeTypeId,
    checked: true,
    amount: "",
    ...over,
  });

  it("prefills the lease's charges, checked, with grouped amounts", () => {
    expect(
      chargeEntriesFromLease([
        { chargeTypeId: "t-wifi", name: "Wi-Fi", amount: 50_000 },
        { chargeTypeId: "t-clean", name: "Kebersihan", amount: 1_250_000 },
      ]),
    ).toEqual({
      "t-wifi": { checked: true, amount: "50.000" },
      "t-clean": { checked: true, amount: "1.250.000" },
    });
    expect(chargeEntriesFromLease([])).toEqual({});
  });

  it("has one row per non-metered type, in the API's order", () => {
    const entries = chargeEntriesFromLease([
      { chargeTypeId: "t-wifi", name: "Wi-Fi", amount: 50_000 },
    ]);
    expect(chargeRows(TYPES, entries)).toEqual([
      {
        chargeTypeId: "t-clean",
        name: "Kebersihan",
        checked: false,
        amount: "",
      },
      { chargeTypeId: "t-park", name: "Parkir", checked: false, amount: "" },
      {
        chargeTypeId: "t-wifi",
        name: "Wi-Fi",
        checked: true,
        amount: "50.000",
      },
    ]);
    expect(chargeRows([type("t-air", "Air", true)], {})).toEqual([]);
    expect(chargeRows([], entries)).toEqual([]);
  });

  it("checks a row when an amount is typed into it", () => {
    const typed = withChargeAmount({}, "t-park", "10.000");
    expect(typed).toEqual({ "t-park": { checked: true, amount: "10.000" } });
    // Emptying the field leaves the checkbox as it is.
    expect(withChargeAmount(typed, "t-park", "")).toEqual({
      "t-park": { checked: true, amount: "" },
    });
    const unchecked = { "t-park": { checked: false, amount: "" } };
    expect(withChargeAmount(unchecked, "t-park", "")).toEqual(unchecked);
  });

  it("keeps the typed text when a row is unchecked", () => {
    const entries = { "t-wifi": { checked: true, amount: "50.000" } };
    const unchecked = withChargeChecked(entries, "t-wifi", false);
    expect(unchecked).toEqual({
      "t-wifi": { checked: false, amount: "50.000" },
    });
    expect(withChargeChecked(unchecked, "t-wifi", true)).toEqual(entries);
    expect(withChargeChecked({}, "t-new", true)).toEqual({
      "t-new": { checked: true, amount: "" },
    });
  });

  it("sends the checked rows in display order, as integers", () => {
    expect(
      checkLeaseCharges([
        row({ chargeTypeId: "t-clean", amount: "25.000" }),
        row({ chargeTypeId: "t-park", checked: false, amount: "10.000" }),
        row({ chargeTypeId: "t-wifi", amount: "1.250.000" }),
      ]),
    ).toEqual({
      ok: true,
      input: {
        charges: [
          { chargeTypeId: "t-clean", amount: 25_000 },
          { chargeTypeId: "t-wifi", amount: 1_250_000 },
        ],
      },
      sent: ["t-clean", "t-wifi"],
    });
  });

  it("sends an empty set when no row is checked, removing every extra", () => {
    expect(
      checkLeaseCharges([
        row({ chargeTypeId: "t-wifi", checked: false, amount: "50.000" }),
      ]),
    ).toEqual({ ok: true, input: { charges: [] }, sent: [] });
    expect(checkLeaseCharges([])).toEqual({
      ok: true,
      input: { charges: [] },
      sent: [],
    });
  });

  it.each([
    { name: "empty", amount: "" },
    { name: "zero", amount: "0" },
    { name: "spaces", amount: "  " },
  ])("refuses a checked row with an amount that is $name", ({ amount }) => {
    expect(
      checkLeaseCharges([
        row({ chargeTypeId: "t-clean", amount: "25.000" }),
        row({ chargeTypeId: "t-park", amount }),
        row({ chargeTypeId: "t-wifi", amount: "" }),
        row({ chargeTypeId: "t-x", checked: false, amount: "" }),
      ]),
    ).toEqual({
      ok: false,
      errors: {
        "t-park": BAD_CHARGE_AMOUNT_MESSAGE,
        "t-wifi": BAD_CHARGE_AMOUNT_MESSAGE,
      },
      first: "t-park",
    });
  });

  it("maps an API field on charges[i] back to the row sent at index i", () => {
    const sent = ["t-clean", "t-wifi"];
    expect(chargeApiFields(sent)).toEqual([
      "charges[0].amount",
      "charges[0].chargeTypeId",
      "charges[1].amount",
      "charges[1].chargeTypeId",
    ]);
    expect(chargeRowForApiField("charges[1].amount", sent)).toBe("t-wifi");
    expect(chargeRowForApiField("charges[0].chargeTypeId", sent)).toBe(
      "t-clean",
    );
    expect(chargeRowForApiField("charges[2].amount", sent)).toBeNull();
    expect(chargeRowForApiField("charges", sent)).toBeNull();
    expect(chargeRowForApiField("charges[0].name", sent)).toBeNull();
    expect(chargeRowForApiField(null, sent)).toBeNull();
  });

  it.each([
    {
      name: "a name",
      input: "Air galon",
      want: { ok: true, name: "Air galon" },
    },
    {
      name: "trimmed",
      input: "  Parkir  ",
      want: { ok: true, name: "Parkir" },
    },
    {
      name: "empty",
      input: "",
      want: { ok: false, error: EMPTY_CHARGE_TYPE_NAME_MESSAGE },
    },
    {
      name: "only spaces",
      input: "   ",
      want: { ok: false, error: EMPTY_CHARGE_TYPE_NAME_MESSAGE },
    },
    {
      name: "50 characters",
      input: "a".repeat(50),
      want: { ok: true, name: "a".repeat(50) },
    },
    {
      name: "51 characters",
      input: "a".repeat(51),
      want: { ok: false, error: LONG_CHARGE_TYPE_NAME_MESSAGE },
    },
  ])("checks a new type's name: $name", ({ input, want }) => {
    expect(checkChargeTypeName(input)).toEqual(want);
  });

  it("words the rows and the metered line", () => {
    expect(chargeAmountLabel("Wi-Fi")).toBe("Jumlah Wi-Fi");
    expect(chargeAmountId("f1", "t-wifi")).toBe("f1-t-wifi-amount");
    expect(meteredChargeTypeAddedText("Listrik")).toBe(
      "Jenis biaya Listrik ditambahkan. Biaya dari meteran dicatat tiap bulan, jadi tidak diatur di sini.",
    );
    expect(LONG_CHARGE_TYPE_NAME_MESSAGE).toBe(
      "Nama biaya paling panjang 50 huruf.",
    );
  });
});
