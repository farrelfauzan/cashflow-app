import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/errors";
import type {
  InvoiceListItem,
  InvoiceListSummary,
} from "@/lib/api/generated/model";

import {
  CLEAR_FILTERS,
  currentPeriod,
  INVOICE_SORT_OPTIONS,
  invoiceChips,
  invoiceRow,
  isFiltering,
  isForbidden,
  isLatestMonth,
  listPeriod,
  monthLabel,
  nextMonth,
  noInvoicesText,
  paidPercent,
  periodParam,
  previousMonth,
  readInvoiceFilters,
  readPeriod,
  shownPeriod,
  summaryTiles,
  unitText,
} from "./utils";
import { readSort } from "@/components/client/list/utils";

const PROPERTY = "44444444-4444-4444-8444-444444444444";

const params = (query: string) => new URLSearchParams(query);

const item = (over: Partial<InvoiceListItem> = {}): InvoiceListItem => ({
  id: "inv-1",
  tenantId: "ten-1",
  tenantName: "Rina Wulandari",
  leaseId: "lease-1",
  propertyId: PROPERTY,
  propertyName: "Kos Melati",
  unitLabel: "Kamar 3",
  period: "2026-11-10",
  periodEnd: "2026-12-09",
  dueDate: "2026-11-10",
  issuedOn: "2026-11-01",
  amount: 1_500_000,
  billed: 1_500_000,
  paid: 500_000,
  outstanding: 1_000_000,
  status: "partial",
  overdueDays: 0,
  pastPlannedEnd: false,
  lines: [],
  ...over,
});

const summary = (
  over: Partial<InvoiceListSummary> = {},
): InvoiceListSummary => ({
  billed: 10_000_000,
  paid: 4_500_000,
  outstanding: 5_500_000,
  tenantsOutstanding: 4,
  ...over,
});

describe("month math", () => {
  it.each([
    ["2026-11", "2026-10"],
    ["2026-01", "2025-12"],
    ["2026-03", "2026-02"],
    ["2000-01", "1999-12"],
  ])("goes back from %s to %s", (period, want) => {
    expect(previousMonth(period)).toBe(want);
  });

  it.each([
    ["2026-10", "2026-11"],
    ["2026-12", "2027-01"],
    ["2026-01", "2026-02"],
  ])("goes on from %s to %s", (period, want) => {
    expect(nextMonth(period)).toBe(want);
  });

  it("refuses a month that is not YYYY-MM", () => {
    expect(() => previousMonth("2026-13")).toThrow(RangeError);
    expect(() => nextMonth("November")).toThrow(RangeError);
  });

  it("labels a month in Indonesian with its year", () => {
    expect(monthLabel("2026-11")).toBe("November 2026");
    expect(monthLabel("2027-01")).toBe("Januari 2027");
  });

  it("takes this month from the Jakarta day", () => {
    expect(currentPeriod("2026-11-02")).toBe("2026-11");
  });

  it("shows the URL's month, else this month", () => {
    expect(shownPeriod("2026-09", "2026-11")).toBe("2026-09");
    expect(shownPeriod(undefined, "2026-11")).toBe("2026-11");
  });

  it("stops 'Bulan berikutnya' at this month", () => {
    expect(isLatestMonth("2026-11", "2026-11")).toBe(true);
    expect(isLatestMonth("2027-02", "2026-11")).toBe(true);
    expect(isLatestMonth("2026-10", "2026-11")).toBe(false);
  });

  it("keeps this month out of the URL", () => {
    expect(periodParam("2026-11", "2026-11")).toBeNull();
    expect(periodParam("2026-10", "2026-11")).toBe("2026-10");
  });
});

describe("readPeriod", () => {
  it.each([
    ["period=2026-10", "2026-10"],
    ["", undefined],
    ["period=2026-13", undefined],
    ["period=2026-00", undefined],
    ["period=2026-1", undefined],
    ["period=202610", undefined],
    ["period=abc", undefined],
    ["period=2026-10-01", undefined],
  ])("reads %j as %j", (query, want) => {
    expect(readPeriod(params(query))).toBe(want);
  });
});

describe("readInvoiceFilters", () => {
  it("reads every filter", () => {
    expect(
      readInvoiceFilters(
        params(
          `period=2026-10&status=partial&propertyId=${PROPERTY}&q=%20rina%20`,
        ),
      ),
    ).toEqual({
      period: "2026-10",
      status: "partial",
      propertyId: PROPERTY,
      q: "rina",
    });
  });

  it("drops values the API would refuse", () => {
    expect(
      readInvoiceFilters(
        params("period=oct&status=late&propertyId=nope&q=%20"),
      ),
    ).toEqual({
      period: undefined,
      status: undefined,
      propertyId: undefined,
      q: undefined,
    });
  });

  it("accepts every status the API lists", () => {
    for (const status of [
      "open",
      "partial",
      "paid",
      "cancelled",
      "unpaid",
      "overdue",
    ]) {
      expect(readInvoiceFilters(params(`status=${status}`)).status).toBe(
        status,
      );
    }
  });

  it("falls back to the default sort for an unknown one", () => {
    expect(readSort(params("sort=amount"), INVOICE_SORT_OPTIONS)).toBe(
      "dueDate",
    );
    expect(readSort(params("sort=-outstanding"), INVOICE_SORT_OPTIONS)).toBe(
      "-outstanding",
    );
    expect(readSort(params("sort=tenantName"), INVOICE_SORT_OPTIONS)).toBe(
      "tenantName",
    );
    expect(readSort(params("sort=-dueDate"), INVOICE_SORT_OPTIONS)).toBe(
      "-dueDate",
    );
  });
});

describe("listPeriod", () => {
  it("sends the month in the URL", () => {
    expect(
      listPeriod(readInvoiceFilters(params("period=2026-10&status=paid"))),
    ).toBe("2026-10");
  });

  it("sends nothing when the month is not set", () => {
    expect(listPeriod(readInvoiceFilters(params("")))).toBeUndefined();
    expect(
      listPeriod(readInvoiceFilters(params("period=bad"))),
    ).toBeUndefined();
  });

  it("sends no month for debt of every month", () => {
    expect(
      listPeriod(readInvoiceFilters(params("period=2026-10&status=overdue"))),
    ).toBeUndefined();
  });
});

describe("isFiltering", () => {
  it("is false for a month alone and true for a search or a filter", () => {
    expect(isFiltering(readInvoiceFilters(params("period=2026-10")))).toBe(
      false,
    );
    expect(isFiltering(readInvoiceFilters(params("q=rina")))).toBe(true);
    expect(isFiltering(readInvoiceFilters(params("status=paid")))).toBe(true);
    expect(
      isFiltering(readInvoiceFilters(params(`propertyId=${PROPERTY}`))),
    ).toBe(true);
  });

  it("clears the search, the property and the status, not the month", () => {
    expect(Object.keys(CLEAR_FILTERS).sort()).toEqual([
      "propertyId",
      "q",
      "status",
    ]);
  });
});

describe("texts", () => {
  it("says 'bulan ini' for this month and names another", () => {
    expect(noInvoicesText("2026-11", "2026-11")).toBe(
      "Belum ada tagihan bulan ini.",
    );
    expect(noInvoicesText("2026-10", "2026-11")).toBe(
      "Belum ada tagihan di Oktober 2026.",
    );
  });

  it("joins the unit and the property", () => {
    expect(unitText({ unit: "Kamar 3", property: "Kos Melati" })).toBe(
      "Kamar 3 · Kos Melati",
    );
  });

  it("knows a 403 from other errors", () => {
    const error = (status: number) =>
      new ApiError({ status, code: "x", message: "m" });
    expect(isForbidden(error(403))).toBe(true);
    expect(isForbidden(error(500))).toBe(false);
    expect(isForbidden(new Error("x"))).toBe(false);
    expect(isForbidden(null)).toBe(false);
  });
});

describe("paidPercent", () => {
  it.each([
    [4_500_000, 10_000_000, 45],
    [999, 1000, 99],
    [1, 3, 33],
    [0, 1000, 0],
    [1000, 1000, 100],
    [2000, 1000, 100],
    [-5, 1000, 0],
    [500, 0, 0],
  ])("%d of %d is %d%%", (paid, billed, want) => {
    expect(paidPercent(paid, billed)).toBe(want);
  });
});

describe("summaryTiles", () => {
  const OLDEST = {
    tenantId: "ten-9",
    tenantName: "Bu Ani Rahmawati",
    propertyName: "Kontrakan Mawar",
    unitLabel: "Rumah 2",
    oldestUnpaidDueDate: "2026-09-10",
    ageDays: 53,
  };

  it("reads the three tiles from the summary", () => {
    const tiles = summaryTiles(
      summary({ oldestOverdue: OLDEST }),
      "2026-11",
      "2026-11",
    );
    expect(tiles).toEqual([
      {
        label: "Terkumpul",
        value: "Rp4.500.000",
        notes: ["45% dari Rp10.000.000"],
        tone: "default",
      },
      {
        label: "Sisa belum dibayar",
        value: "Rp5.500.000",
        notes: ["4 penyewa"],
        tone: "default",
      },
      {
        label: "Tunggakan tertua",
        value: "Terlambat 53 hari",
        notes: ["Bu Ani Rahmawati · Rumah 2, Kontrakan Mawar"],
        tone: "warning",
      },
    ]);
  });

  it("says there is no debt when there is no oldest overdue", () => {
    const [, , oldest] = summaryTiles(summary(), "2026-11", "2026-11");
    expect(oldest).toEqual({
      label: "Tunggakan tertua",
      value: "Tidak ada",
      notes: ["Tidak ada tunggakan"],
      tone: "default",
    });
  });

  it("says there is no invoice yet when nothing was billed", () => {
    const [collected, owed] = summaryTiles(
      summary({ billed: 0, paid: 0, outstanding: 0, tenantsOutstanding: 0 }),
      "2026-11",
      "2026-11",
    );
    expect(collected?.value).toBe("Rp0");
    expect(collected?.notes).toEqual(["Belum ada tagihan bulan ini."]);
    expect(owed?.notes).toEqual(["0 penyewa"]);
  });

  it("names the month when it is not this one", () => {
    const [collected] = summaryTiles(
      summary({ billed: 0, paid: 0, outstanding: 0, tenantsOutstanding: 0 }),
      "2026-10",
      "2026-11",
    );
    expect(collected?.notes).toEqual(["Belum ada tagihan di Oktober 2026."]);
  });

  it("says everything is paid when nothing is owed", () => {
    const [collected, owed] = summaryTiles(
      summary({ paid: 10_000_000, outstanding: 0, tenantsOutstanding: 0 }),
      "2026-11",
      "2026-11",
    );
    expect(collected?.notes).toEqual(["100% dari Rp10.000.000"]);
    expect(owed?.notes).toEqual(["Semua sudah lunas"]);
  });
});

describe("invoiceChips", () => {
  const cases: {
    name: string;
    over: Partial<InvoiceListItem>;
    want: { label: string; tone: string }[];
  }[] = [
    {
      name: "paid",
      over: { status: "paid", paid: 1_500_000, outstanding: 0 },
      want: [{ label: "Lunas", tone: "active" }],
    },
    {
      name: "cancelled",
      over: { status: "cancelled", billed: 0, outstanding: 0 },
      want: [{ label: "Dibatalkan", tone: "ended" }],
    },
    {
      name: "open and overdue",
      over: { status: "open", paid: 0, overdueDays: 12 },
      want: [{ label: "Terlambat 12 hari", tone: "warning" }],
    },
    {
      name: "partial and overdue",
      over: { status: "partial", overdueDays: 1 },
      want: [{ label: "Terlambat 1 hari", tone: "warning" }],
    },
    {
      name: "partial, not due yet",
      over: { status: "partial", overdueDays: 0 },
      want: [{ label: "Sebagian", tone: "neutral" }],
    },
    {
      name: "open, not due yet",
      over: { status: "open", paid: 0, overdueDays: 0 },
      want: [{ label: "Belum jatuh tempo", tone: "upcoming" }],
    },
    {
      name: "paid, past planned end",
      over: { status: "paid", pastPlannedEnd: true },
      want: [
        { label: "Lunas", tone: "active" },
        { label: "Lewat rencana keluar", tone: "warning" },
      ],
    },
    {
      name: "overdue, past planned end",
      over: { status: "open", overdueDays: 3, pastPlannedEnd: true },
      want: [
        { label: "Terlambat 3 hari", tone: "warning" },
        { label: "Lewat rencana keluar", tone: "warning" },
      ],
    },
    {
      name: "paid with overdueDays that is not trusted",
      over: { status: "paid", overdueDays: 5 },
      want: [{ label: "Lunas", tone: "active" }],
    },
  ];

  it.each(cases)("$name", ({ over, want }) => {
    expect(invoiceChips(item(over))).toEqual(want);
  });
});

describe("invoiceRow", () => {
  it("formats the figures the API sent", () => {
    expect(invoiceRow(item())).toEqual({
      id: "inv-1",
      tenantId: "ten-1",
      tenantName: "Rina Wulandari",
      unit: "Kamar 3",
      property: "Kos Melati",
      dueDate: "10 Nov 2026",
      billed: "Rp1.500.000",
      paid: "Rp500.000",
      outstanding: "Rp1.000.000",
      chips: [{ label: "Sebagian", tone: "neutral" }],
    });
  });
});
