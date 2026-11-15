import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/errors";
import type {
  CashDeposit,
  Capability,
  HeldCashPayment,
} from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import {
  AMOUNT_DIGITS_MESSAGE,
  AMOUNT_TOO_LARGE_MESSAGE,
  AMOUNT_ZERO_MESSAGE,
  amountFieldText,
  canDepositCash,
  cashSinceLine,
  depositErrorKind,
  depositFormDefaults,
  depositFormSchema,
  depositRequestBody,
  EMPTY_AMOUNT_MESSAGE,
  hasGap,
  heldReceiptView,
  isForbidden,
  moreReceiptsText,
  pastDepositViews,
  pendingDeposits,
  pendingDepositView,
  photoChip,
  receiptCountText,
  resultChip,
  submitLabel,
} from "./utils";

function deposit(overrides: Partial<CashDeposit>): CashDeposit {
  return {
    id: "d-1",
    staffUserId: "u-1",
    staffName: "Mas Rudi Hartanto",
    amount: 2_500_000,
    expectedAmount: 2_500_000,
    paymentCount: 3,
    photo: { status: "missing" },
    // Sabtu 14 Nov 2026, 16.20 in Jakarta.
    recordedAt: "2026-11-14T09:20:00Z",
    status: "pending",
    ...overrides,
  };
}

function confirmation(
  receivedAmount: number,
  gap: number,
): NonNullable<CashDeposit["confirmation"]> {
  return {
    receivedAmount,
    gap,
    confirmedBy: "u-2",
    confirmedByName: "Pak Budi Hartono",
    confirmedAt: "2026-11-14T10:00:00Z",
  };
}

function payment(overrides: Partial<HeldCashPayment>): HeldCashPayment {
  return {
    paymentId: "p-1",
    tenantId: "t-1",
    tenantName: "Bu Wati Handayani",
    units: [
      {
        unitLabel: "Rumah A1",
        propertyName: "Kontrakan Seturan",
        propertyAddress: "",
      },
    ],
    amount: 1_050_000,
    // Jumat 13 Nov 2026, 10.15 in Jakarta.
    recordedAt: "2026-11-13T03:15:00Z",
    photo: { status: "clean", uploadId: "up-1" },
    ...overrides,
  };
}

describe("depositFormSchema", () => {
  it.each([
    { name: "a plain number", input: "2500000", want: 2_500_000 },
    { name: "grouped digits", input: "2.500.000", want: 2_500_000 },
    { name: "Rp1", input: "1", want: 1 },
  ])("turns $name into integer rupiah", ({ input, want }) => {
    expect(depositFormSchema.parse({ amount: input })).toEqual({
      amount: want,
    });
  });

  it.each([
    { name: "an empty field", input: "", message: EMPTY_AMOUNT_MESSAGE },
    { name: "spaces only", input: "  ", message: EMPTY_AMOUNT_MESSAGE },
    { name: "letters", input: "2,5jt", message: AMOUNT_DIGITS_MESSAGE },
    { name: "zero", input: "0", message: AMOUNT_ZERO_MESSAGE },
    {
      name: "more than the API allows",
      input: "1000000000001",
      message: AMOUNT_TOO_LARGE_MESSAGE,
    },
  ])("refuses $name", ({ input, message }) => {
    const result = depositFormSchema.safeParse({ amount: input });
    expect(result.success).toBe(false);
    expect(result.error?.issues[0]?.message).toBe(message);
  });
});

describe("depositFormDefaults and amountFieldText", () => {
  it("starts at the total held, grouped", () => {
    expect(depositFormDefaults({ amount: 2_500_000, count: 3 })).toEqual({
      amount: "2.500.000",
    });
  });

  it.each([
    { amount: 0, want: "0" },
    { amount: 950, want: "950" },
    { amount: 1_250_000, want: "1.250.000" },
  ])("shows $amount as $want", ({ amount, want }) => {
    expect(amountFieldText(amount)).toBe(want);
  });
});

describe("depositRequestBody", () => {
  it("sends the total and count as shown, the typed amount and the requestId", () => {
    expect(
      depositRequestBody(
        { amount: 2_450_000 },
        { amount: 2_500_000, count: 3 },
        "req-1",
      ),
    ).toEqual({
      requestId: "req-1",
      expectedAmount: 2_500_000,
      expectedCount: 3,
      amount: 2_450_000,
    });
  });
});

describe("submitLabel", () => {
  it.each([
    { text: "2.500.000", want: "Setor Rp2.500.000" },
    { text: "2500000", want: "Setor Rp2.500.000" },
    { text: "", want: "Setor" },
    { text: "abc", want: "Setor" },
  ])("labels $text as $want", ({ text, want }) => {
    expect(submitLabel(text)).toBe(want);
  });
});

describe("cashSinceLine", () => {
  it("names the day of the newest deposit", () => {
    expect(cashSinceLine({ recordedAt: "2026-11-12T08:00:00Z" })).toBe(
      "Tunai di tangan sejak setoran Kamis, 12 Nov",
    );
  });

  it("reads the day in Jakarta", () => {
    expect(cashSinceLine({ recordedAt: "2026-11-11T18:00:00Z" })).toBe(
      "Tunai di tangan sejak setoran Kamis, 12 Nov",
    );
  });

  it("says only Tunai di tangan before the first deposit", () => {
    expect(cashSinceLine(undefined)).toBe("Tunai di tangan");
  });
});

describe("moreReceiptsText and receiptCountText", () => {
  it.each([
    { total: 3, shown: 3, want: null },
    { total: 103, shown: 100, want: "dan 3 kwitansi lainnya" },
    { total: 0, shown: 0, want: null },
  ])(
    "says $want for $total held and $shown shown",
    ({ total, shown, want }) => {
      expect(moreReceiptsText(total, shown)).toBe(want);
    },
  );

  it("counts receipts", () => {
    expect(receiptCountText(1)).toBe("1 kwitansi");
    expect(receiptCountText(4)).toBe("4 kwitansi");
  });
});

describe("photoChip", () => {
  it.each([
    { status: "clean", label: "Foto tersimpan", tone: "active" },
    { status: "pending", label: "Foto sedang diperiksa", tone: "none" },
    { status: "missing", label: "Foto belum ada", tone: "warning" },
    { status: "rejected", label: "Foto ditolak, ambil ulang", tone: "warning" },
    { status: "infected", label: "Foto ditolak, ambil ulang", tone: "warning" },
  ] as const)("shows a $status photo as $label", ({ status, label, tone }) => {
    expect(photoChip(status)).toEqual({ label, tone });
  });
});

describe("heldReceiptView", () => {
  it("shows the tenant, unit, time in Jakarta, amount and photo", () => {
    expect(heldReceiptView(payment({}))).toEqual({
      paymentId: "p-1",
      tenantName: "Bu Wati Handayani",
      detail: "Rumah A1 · Jumat 13 Nov, 10.15",
      amount: "Rp1.050.000",
      photo: { label: "Foto tersimpan", tone: "active" },
    });
  });

  it("joins several units", () => {
    const view = heldReceiptView(
      payment({
        units: [
          { unitLabel: "Kamar 02", propertyName: "Kos", propertyAddress: "" },
          { unitLabel: "Kamar 03", propertyName: "Kos", propertyAddress: "" },
        ],
      }),
    );
    expect(view.detail).toBe("Kamar 02, Kamar 03 · Jumat 13 Nov, 10.15");
  });

  it("shows only the time when the tenant has no unit", () => {
    expect(heldReceiptView(payment({ units: [] })).detail).toBe(
      "Jumat 13 Nov, 10.15",
    );
  });
});

describe("pending deposits", () => {
  it("keeps only the pending ones, in the API's order", () => {
    const items = [
      deposit({ id: "a" }),
      deposit({ id: "b", status: "matched" }),
      deposit({ id: "c" }),
    ];
    expect(pendingDeposits(items).map((item) => item.id)).toEqual(["a", "c"]);
  });

  it("shows the amount, when and how many receipts", () => {
    expect(pendingDepositView(deposit({}))).toEqual({
      id: "d-1",
      amount: "Rp2.500.000",
      detail: "Disetor Sabtu 14 Nov, 16.20 · 3 kwitansi",
    });
  });
});

describe("resultChip", () => {
  it.each([
    { status: "matched", gap: 0, label: "Cocok", tone: "active" },
    { status: "gap", gap: -50_000, label: "Selisih Rp50.000", tone: "warning" },
    { status: "gap", gap: 20_000, label: "Selisih Rp20.000", tone: "warning" },
  ] as const)(
    "labels $status $gap as $label",
    ({ status, gap, label, tone }) => {
      expect(resultChip(status, gap)).toEqual({ label, tone });
    },
  );
});

describe("pastDepositViews", () => {
  it("shows a matched deposit with who counted it", () => {
    const views = pastDepositViews([
      deposit({
        id: "m",
        amount: 3_800_000,
        paymentCount: 4,
        recordedAt: "2026-11-12T08:00:00Z",
        status: "matched",
        confirmation: confirmation(3_800_000, 0),
      }),
    ]);
    expect(views).toEqual([
      {
        id: "m",
        date: "Kamis, 12 Nov",
        detail: "Dihitung Pak Budi Hartono · 4 kwitansi",
        amount: "Rp3.800.000",
        chip: { label: "Cocok", tone: "active" },
      },
    ]);
    expect(hasGap(views)).toBe(false);
  });

  it("shows a gap with what was handed over and what was counted", () => {
    const views = pastDepositViews([
      deposit({
        id: "g",
        amount: 2_000_000,
        expectedAmount: 2_000_000,
        recordedAt: "2026-11-09T08:00:00Z",
        status: "gap",
        confirmation: confirmation(1_950_000, -50_000),
      }),
    ]);
    expect(views).toEqual([
      {
        id: "g",
        date: "Senin, 9 Nov",
        detail: "Disetor Rp2.000.000 · dihitung Rp1.950.000",
        amount: "Rp1.950.000",
        chip: { label: "Selisih Rp50.000", tone: "warning" },
      },
    ]);
    expect(hasGap(views)).toBe(true);
  });

  it("leaves out pending deposits and any without a count", () => {
    expect(
      pastDepositViews([
        deposit({ id: "p" }),
        deposit({ id: "x", status: "matched" }),
      ]),
    ).toEqual([]);
  });
});

describe("depositErrorKind and isForbidden", () => {
  function apiError(status: number, code: string) {
    return new ApiError({ status, code, message: "Pesan dari API." });
  }

  it.each([
    { error: apiError(409, "deposit_cash_changed"), want: "cash_changed" },
    { error: apiError(409, "nothing_to_deposit"), want: "nothing_to_deposit" },
    { error: apiError(409, "request_id_reused"), want: "request_reused" },
    { error: apiError(400, "invalid_input"), want: "other" },
    { error: new Error("boom"), want: "other" },
  ])("classifies $want", ({ error, want }) => {
    expect(depositErrorKind(error)).toBe(want);
  });

  it("knows a 403", () => {
    expect(isForbidden(apiError(403, "forbidden"))).toBe(true);
    expect(isForbidden(apiError(404, "not_found"))).toBe(false);
    expect(isForbidden(null)).toBe(false);
  });
});

describe("canDepositCash", () => {
  function ability(access: Capability["access"]) {
    return createAppAbility([{ action: "cash:deposit", access }]);
  }

  it.each([
    { access: "allow", want: true },
    { access: "conditional", want: true },
    { access: "deny", want: false },
  ] as const)("is $want for $access", ({ access, want }) => {
    expect(canDepositCash(ability(access))).toBe(want);
  });

  it("is false without the action", () => {
    expect(canDepositCash(createAppAbility([]))).toBe(false);
  });
});
