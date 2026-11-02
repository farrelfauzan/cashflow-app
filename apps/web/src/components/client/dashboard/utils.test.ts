import { describe, expect, it } from "vitest";

import type {
  Dashboard,
  DashboardCorrection,
  TenantArrears,
} from "@/lib/api/generated/model";

import {
  arrearsPlace,
  arrearsRows,
  arrearsTotal,
  collectedPercent,
  correctionRows,
  greeting,
  monthMoney,
  moreArrearsText,
} from "./utils";

const HENDRA = { memberId: "m-1", name: "Pak Hendra Gunawan" };

function dashboard(overrides: Partial<Dashboard> = {}): Dashboard {
  return {
    period: "2026-11",
    billed: 6_240_000,
    collected: 4_820_000,
    outstanding: 1_420_000,
    invoiceCount: 8,
    tenantsOutstanding: 3,
    overdue: 2_350_000,
    tenantsInArrears: 2,
    arrears: [],
    recentCorrections: [],
    ...overrides,
  };
}

function arrears(overrides: Partial<TenantArrears> = {}): TenantArrears {
  return {
    tenantId: "t-1",
    tenantName: "Bu Wati Lestari",
    units: [{ propertyName: "Kos Melati", unitLabel: "Kamar 11" }],
    overdue: 1_050_000,
    oldestUnpaidDueDate: "2026-10-26",
    ageDays: 7,
    ...overrides,
  };
}

function correction(
  overrides: Partial<DashboardCorrection> = {},
): DashboardCorrection {
  return {
    id: "c-1",
    tenantId: "t-1",
    tenantName: "Dimas Pratama",
    amountDelta: -200_000,
    reason: "Listrik salah catat",
    createdBy: HENDRA,
    createdAt: "2026-11-01T18:30:00Z",
    ...overrides,
  };
}

describe("greeting", () => {
  it.each([
    { hour: 4, want: "Selamat pagi" },
    { hour: 10, want: "Selamat pagi" },
    { hour: 11, want: "Selamat siang" },
    { hour: 14, want: "Selamat siang" },
    { hour: 15, want: "Selamat sore" },
    { hour: 17, want: "Selamat sore" },
    { hour: 18, want: "Selamat malam" },
    { hour: 0, want: "Selamat malam" },
    { hour: 3, want: "Selamat malam" },
  ])("says $want at $hour", ({ hour, want }) => {
    expect(greeting(hour)).toBe(want);
  });
});

describe("collectedPercent", () => {
  it.each([
    { collected: 4_820_000, billed: 6_240_000, want: 77 },
    { collected: 0, billed: 1_000_000, want: 0 },
    { collected: 999_999, billed: 1_000_000, want: 99 },
    { collected: 1_000_000, billed: 1_000_000, want: 100 },
    { collected: 1_200_000, billed: 1_000_000, want: 100 },
    { collected: 0, billed: 0, want: null },
  ])("draws $collected of $billed as $want", ({ collected, billed, want }) => {
    expect(collectedPercent(collected, billed)).toBe(want);
  });
});

describe("monthMoney", () => {
  it("names the month the API answered for and what is still owed", () => {
    expect(monthMoney(dashboard())).toEqual({
      title: "Tagihan bulan November",
      collected: "Rp4.820.000",
      percent: 77,
      summary: {
        kind: "owing",
        billed: "Rp6.240.000",
        outstanding: "Rp1.420.000",
        tenants: 3,
      },
    });
  });

  it("says everything is paid when nothing is outstanding", () => {
    const view = monthMoney(
      dashboard({
        collected: 6_240_000,
        outstanding: 0,
        tenantsOutstanding: 0,
      }),
    );
    expect(view.summary).toEqual({ kind: "paid", billed: "Rp6.240.000" });
    expect(view.percent).toBe(100);
  });

  it("has no bar and says so when the month has no invoice", () => {
    const view = monthMoney(
      dashboard({
        period: "2027-01",
        billed: 0,
        collected: 0,
        outstanding: 0,
        invoiceCount: 0,
        tenantsOutstanding: 0,
      }),
    );
    expect(view.title).toBe("Tagihan bulan Januari");
    expect(view.percent).toBeNull();
    expect(view.summary).toEqual({ kind: "none" });
  });
});

describe("arrearsTotal", () => {
  it("shows the overdue total in the warning tone", () => {
    expect(arrearsTotal(dashboard())).toEqual({
      value: "Rp2.350.000",
      note: "2 penyewa terlambat bayar, semua bulan",
      tone: "warning",
    });
  });

  it("says there is no debt", () => {
    expect(
      arrearsTotal(dashboard({ overdue: 0, tenantsInArrears: 0 })),
    ).toEqual({ value: "Rp0", note: "Tidak ada tunggakan", tone: "default" });
  });
});

describe("arrearsPlace", () => {
  it("names one unit with its property", () => {
    expect(arrearsPlace(arrears())).toBe("Kamar 11, Kos Melati");
  });

  it("counts the other units of a debt over several", () => {
    expect(
      arrearsPlace(
        arrears({
          units: [
            { propertyName: "Kos Melati", unitLabel: "Kamar 11" },
            { propertyName: "Ruko Sentosa", unitLabel: "Ruko 2" },
            { propertyName: "Ruko Sentosa", unitLabel: "Ruko 3" },
          ],
        }),
      ),
    ).toBe("Kamar 11, Kos Melati +2 unit lain");
  });

  it("is empty without units", () => {
    expect(arrearsPlace(arrears({ units: [] }))).toBe("");
  });
});

describe("arrearsRows", () => {
  it("keeps the API's order and formats each tenant's debt", () => {
    const rows = arrearsRows(
      dashboard({
        arrears: [
          arrears({
            tenantId: "t-2",
            tenantName: "Pak Hendra Gunawan",
            units: [{ propertyName: "Kontrakan Mawar", unitLabel: "No. 4" }],
            overdue: 1_450_000,
            ageDays: 12,
          }),
          arrears(),
        ],
      }),
    );
    expect(rows).toEqual([
      {
        tenantId: "t-2",
        name: "Pak Hendra Gunawan",
        place: "No. 4, Kontrakan Mawar",
        amount: "Rp1.450.000",
        age: "Terlambat 12 hari",
      },
      {
        tenantId: "t-1",
        name: "Bu Wati Lestari",
        place: "Kamar 11, Kos Melati",
        amount: "Rp1.050.000",
        age: "Terlambat 7 hari",
      },
    ]);
  });
});

describe("moreArrearsText", () => {
  it("counts the tenants the list leaves out", () => {
    expect(
      moreArrearsText(
        dashboard({ tenantsInArrears: 13, arrears: Array(10).fill(arrears()) }),
      ),
    ).toBe("dan 3 penyewa lain");
  });

  it("is null when every tenant is listed", () => {
    expect(
      moreArrearsText(dashboard({ tenantsInArrears: 1, arrears: [arrears()] })),
    ).toBeNull();
  });
});

describe("correctionRows", () => {
  it("names an invoice correction's month and unit, on the Jakarta day it was made", () => {
    const [row] = correctionRows(
      dashboard({
        recentCorrections: [
          correction({
            invoiceId: "i-1",
            period: "2026-11-15",
            unit: { propertyName: "Kos Melati", unitLabel: "Kamar 3" },
          }),
        ],
      }),
    );
    expect(row).toEqual({
      id: "c-1",
      title: "2 Nov 2026 · Dimas Pratama",
      subject: "Tagihan November 2026 · Kamar 3, Kos Melati",
      amount: "−Rp200.000",
      reason: "Listrik salah catat",
      by: "oleh Pak Hendra Gunawan",
    });
  });

  it("names a payment correction's received day", () => {
    const [row] = correctionRows(
      dashboard({
        recentCorrections: [
          correction({
            paymentId: "p-1",
            receivedOn: "2026-10-25",
            amountDelta: 50_000,
          }),
        ],
      }),
    );
    expect(row?.subject).toBe("Pembayaran 25 Okt 2026");
    expect(row?.amount).toBe("+Rp50.000");
  });
});
