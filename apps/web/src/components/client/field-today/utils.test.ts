import { describe, expect, it } from "vitest";

import type {
  FieldCashToday,
  FieldTodayItem,
  ReceiptPhotoStatus,
} from "@/lib/api/generated/model";

import {
  cashLine,
  fieldTodayRow,
  moreRowsText,
  owedText,
  photoChip,
  placeText,
  reasonChip,
} from "./utils";

const NO_CASH: FieldCashToday = { amount: 0, count: 0, payments: [] };

function cashToday(
  amount: number,
  statuses: readonly ReceiptPhotoStatus[],
): FieldCashToday {
  return {
    amount,
    count: statuses.length,
    payments: statuses.map((status, index) => ({
      paymentId: `p-${index}`,
      amount: amount / statuses.length,
      recordedAt: "2026-11-16T03:00:00Z",
      photo: { status },
    })),
  };
}

const WATI: FieldTodayItem = {
  tenantId: "t-1",
  tenantName: "Bu Wati Handayani",
  units: [
    {
      unitLabel: "Kamar 11",
      propertyName: "Kos Melati",
      propertyAddress: "Jl. Melati No. 12, Bekasi",
    },
  ],
  reason: "overdue",
  daysOverdue: 7,
  oldestUnpaidDueDate: "2026-11-09",
  outstanding: 1_050_000,
  overdue: 1_050_000,
  cashToday: NO_CASH,
};

describe("placeText", () => {
  it.each([
    { name: "one unit", units: WATI.units, want: "Kamar 11 · Kos Melati" },
    {
      name: "two units",
      units: [
        ...WATI.units,
        {
          unitLabel: "Ruko 1",
          propertyName: "Ruko Seturan",
          propertyAddress: "",
        },
      ],
      want: "Kamar 11 · Kos Melati, Ruko 1 · Ruko Seturan",
    },
    { name: "no unit", units: [], want: "" },
  ])("$name", ({ units, want }) => {
    expect(placeText(units)).toBe(want);
  });
});

describe("owedText", () => {
  it.each([
    {
      name: "the overdue part after what is owed",
      outstanding: 1_050_000,
      overdue: 1_050_000,
      want: "Sisa Rp1.050.000 · telat Rp1.050.000",
    },
    {
      name: "only what is owed when nothing is overdue",
      outstanding: 1_050_000,
      overdue: 0,
      want: "Sisa Rp1.050.000",
    },
    {
      name: "a part overdue",
      outstanding: 2_100_000,
      overdue: 50_000,
      want: "Sisa Rp2.100.000 · telat Rp50.000",
    },
    {
      name: "Lunas when nothing is owed",
      outstanding: 0,
      overdue: 0,
      want: "Lunas",
    },
  ])("$name", ({ outstanding, overdue, want }) => {
    expect(owedText({ outstanding, overdue })).toBe(want);
  });
});

describe("reasonChip", () => {
  it.each([
    {
      name: "overdue with its days, in the warning tone",
      item: { reason: "overdue", daysOverdue: 7 },
      want: { label: "Telat 7 hari", tone: "warning" },
    },
    {
      name: "overdue on the first day",
      item: { reason: "overdue", daysOverdue: 1 },
      want: { label: "Telat 1 hari", tone: "warning" },
    },
    {
      name: "due today",
      item: { reason: "due_today" },
      want: { label: "Jatuh tempo hari ini", tone: "neutral" },
    },
    {
      name: "due tomorrow",
      item: { reason: "due_in_days", daysUntilDue: 1 },
      want: { label: "Jatuh tempo besok", tone: "neutral" },
    },
    {
      name: "due the day after tomorrow",
      item: { reason: "due_in_days", daysUntilDue: 2 },
      want: { label: "Jatuh tempo lusa", tone: "neutral" },
    },
    {
      name: "paid in cash today, in the success tone",
      item: { reason: "paid_today" },
      want: { label: "Tunai diterima", tone: "active" },
    },
  ] as const)("$name", ({ item, want }) => {
    expect(reasonChip(item)).toEqual(want);
  });
});

describe("photoChip", () => {
  it.each([
    { statuses: ["clean"], want: "Foto tersimpan" },
    { statuses: ["pending"], want: "Foto sedang diperiksa" },
    { statuses: ["missing"], want: "Foto belum ada" },
    { statuses: ["rejected"], want: "Foto ditolak" },
    { statuses: ["infected"], want: "Foto ditolak" },
    { statuses: ["clean", "pending"], want: "Foto sedang diperiksa" },
    { statuses: ["clean", "rejected", "pending"], want: "Foto ditolak" },
  ] as const)("$statuses says $want", ({ statuses, want }) => {
    expect(photoChip(statuses).label).toBe(want);
  });

  it("warns about a refused photo", () => {
    expect(photoChip(["rejected"]).tone).toBe("warning");
    expect(photoChip(["clean"]).tone).toBe("active");
  });
});

describe("cashLine", () => {
  it("is null without cash today", () => {
    expect(cashLine(NO_CASH)).toBeNull();
  });

  it("names today's cash and its photo", () => {
    expect(cashLine(cashToday(950_000, ["clean"]))).toEqual({
      text: "Tunai hari ini Rp950.000",
      photo: { label: "Foto tersimpan", tone: "active" },
    });
  });
});

describe("fieldTodayRow", () => {
  it("shows an overdue tenant as on the board", () => {
    expect(fieldTodayRow(WATI)).toEqual({
      tenantId: "t-1",
      tenantName: "Bu Wati Handayani",
      place: "Kamar 11 · Kos Melati",
      owed: "Sisa Rp1.050.000 · telat Rp1.050.000",
      reason: { label: "Telat 7 hari", tone: "warning" },
      cash: null,
    });
  });

  it("shows a tenant who paid in full in cash today", () => {
    const row = fieldTodayRow({
      ...WATI,
      reason: "paid_today",
      daysOverdue: undefined,
      oldestUnpaidDueDate: undefined,
      outstanding: 0,
      overdue: 0,
      cashToday: cashToday(950_000, ["clean"]),
    });
    expect(row.owed).toBe("Lunas");
    expect(row.reason.label).toBe("Tunai diterima");
    expect(row.cash?.text).toBe("Tunai hari ini Rp950.000");
  });
});

describe("moreRowsText", () => {
  it.each([
    { total: 3, shown: 3, want: null },
    { total: 103, shown: 100, want: "dan 3 penyewa lainnya" },
  ])("$total of $shown", ({ total, shown, want }) => {
    expect(moreRowsText(total, shown)).toBe(want);
  });
});
