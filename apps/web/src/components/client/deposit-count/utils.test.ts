import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/errors";
import type {
  CashDeposit,
  Capability,
  HeldCashPayment,
} from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import {
  canCountDeposits,
  canReadDeposits,
  confirmErrorKind,
  confirmRequestBody,
  COUNT_FORM_DEFAULTS,
  countAction,
  countedChip,
  countedRowViews,
  countFormSchema,
  countHint,
  countSavedToast,
  countSheetView,
  depositLinkLabel,
  depositReceiptView,
  differenceText,
  EMPTY_RECEIVED_MESSAGE,
  gapNote,
  NOTE_TOO_LONG_MESSAGE,
  pendingRowView,
  photoText,
  readDepositTab,
  RECEIVED_DIGITS_MESSAGE,
  RECEIVED_TOO_LARGE_MESSAGE,
  tabParam,
  toDepositTab,
} from "./utils";

function deposit(overrides: Partial<CashDeposit>): CashDeposit {
  return {
    id: "d-1",
    staffUserId: "u-karyawan",
    staffName: "Mas Dimas Saputra",
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
    confirmedBy: "u-admin",
    confirmedByName: "Pak Hendra Gunawan",
    // Sabtu 14 Nov 2026, 17.02 in Jakarta.
    confirmedAt: "2026-11-14T10:02:00Z",
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

describe("canCountDeposits and canReadDeposits", () => {
  function ability(capabilities: Capability[]) {
    return createAppAbility(capabilities);
  }

  it.each([
    {
      name: "an admin of the whole organization",
      capabilities: [{ action: "payment:check", access: "allow" }],
      count: true,
      read: true,
    },
    {
      name: "an admin limited to one property",
      capabilities: [{ action: "payment:check", access: "conditional" }],
      count: false,
      read: false,
    },
    {
      name: "the Supervisor",
      capabilities: [
        { action: "payment:check", access: "deny" },
        { action: "cash:read", access: "allow" },
      ],
      count: false,
      read: true,
    },
    {
      name: "a member who reads cash for some properties only",
      capabilities: [{ action: "cash:read", access: "conditional" }],
      count: false,
      read: false,
    },
    {
      name: "a member with nothing",
      capabilities: [],
      count: false,
      read: false,
    },
  ] as const)("is right for $name", ({ capabilities, count, read }) => {
    const granted = ability([...capabilities]);
    expect(canCountDeposits(granted)).toBe(count);
    expect(canReadDeposits(granted)).toBe(read);
  });
});

describe("countAction", () => {
  it.each([
    {
      name: "someone else's deposit",
      userId: "u-admin",
      canCount: true,
      want: "count",
    },
    {
      name: "one's own deposit",
      userId: "u-karyawan",
      canCount: true,
      want: "own",
    },
    { name: "a reader", userId: "u-admin", canCount: false, want: "none" },
    {
      name: "a reader's own deposit",
      userId: "u-karyawan",
      canCount: false,
      want: "none",
    },
  ] as const)("offers $want on $name", ({ userId, canCount, want }) => {
    expect(countAction(deposit({}), userId, canCount)).toBe(want);
  });
});

describe("the tab in the URL", () => {
  it.each([
    { query: "", want: "pending" },
    { query: "tab=confirmed", want: "confirmed" },
    { query: "tab=other", want: "pending" },
  ] as const)("reads '$query' as $want", ({ query, want }) => {
    expect(readDepositTab(new URLSearchParams(query))).toBe(want);
  });

  it("keeps the default tab out of the URL", () => {
    expect(tabParam("pending")).toBeNull();
    expect(tabParam("confirmed")).toBe("confirmed");
    expect(toDepositTab("confirmed")).toBe("confirmed");
    expect(toDepositTab("x")).toBe("pending");
  });
});

describe("differenceText", () => {
  it.each([
    { amount: 2_450_000, expected: 2_500_000, want: "kurang Rp50.000" },
    { amount: 2_550_000, expected: 2_500_000, want: "lebih Rp50.000" },
    { amount: 2_500_000, expected: 2_500_000, want: null },
  ])("says $want", ({ amount, expected, want }) => {
    expect(differenceText(amount, expected)).toBe(want);
  });
});

describe("countHint", () => {
  const expected = 2_500_000;

  it.each([
    { name: "an empty field", text: "", want: null },
    { name: "letters", text: "2,5jt", want: null },
    {
      name: "the receipts' total",
      text: "2.500.000",
      want: { tone: "match", text: "Cocok dengan total kwitansi." },
    },
    {
      name: "less",
      text: "2.450.000",
      want: {
        tone: "gap",
        text: "Kurang Rp50.000 dari total kwitansi. Selisihnya masuk Antrean cek. Tagihan penyewa tidak berubah.",
      },
    },
    {
      name: "more",
      text: "2550000",
      want: {
        tone: "gap",
        text: "Lebih Rp50.000 dari total kwitansi. Selisihnya masuk Antrean cek. Tagihan penyewa tidak berubah.",
      },
    },
    {
      name: "zero",
      text: "0",
      want: {
        tone: "gap",
        text: "Kurang Rp2.500.000 dari total kwitansi. Selisihnya masuk Antrean cek. Tagihan penyewa tidak berubah.",
      },
    },
  ])("for $name", ({ text, want }) => {
    expect(countHint(text, expected)).toEqual(want);
  });

  it("compares with the receipts' total, not with what the karyawan said", () => {
    // The karyawan said Rp2.450.000; the admin counted Rp2.500.000, the receipts' total.
    const handedLess = deposit({ amount: 2_450_000 });
    expect(countHint("2.500.000", handedLess.expectedAmount)?.tone).toBe(
      "match",
    );
  });
});

describe("photoText", () => {
  it.each([
    { status: "clean", want: "Foto tersimpan" },
    { status: "pending", want: "Foto sedang diperiksa" },
    { status: "missing", want: "Foto belum ada" },
    { status: "rejected", want: "Foto ditolak" },
    { status: "infected", want: "Foto ditolak" },
  ] as const)("shows a $status photo as $want", ({ status, want }) => {
    expect(photoText(status)).toBe(want);
  });
});

describe("pendingRowView", () => {
  it("shows the karyawan, when, the receipts and both amounts", () => {
    expect(pendingRowView(deposit({}), "count")).toEqual({
      id: "d-1",
      staffName: "Mas Dimas Saputra",
      recordedLine: "Disetor Sabtu 14 Nov, 16.20",
      paymentCount: 3,
      expected: "Rp2.500.000",
      handed: "Rp2.500.000",
      handedNote: null,
      action: "count",
    });
  });

  it("notes how what was handed over differs", () => {
    expect(
      pendingRowView(deposit({ amount: 2_450_000 }), "own").handedNote,
    ).toBe("kurang Rp50.000");
  });
});

describe("countedChip and gapNote", () => {
  it.each([
    { gap: 0, label: "Cocok", tone: "active" },
    { gap: -50_000, label: "Kurang Rp50.000", tone: "warning" },
    { gap: 20_000, label: "Lebih Rp20.000", tone: "warning" },
  ] as const)("labels $gap as $label", ({ gap, label, tone }) => {
    expect(countedChip(gap)).toEqual({ label, tone });
  });

  it("says where a gap is", () => {
    expect(gapNote(0, undefined)).toBeNull();
    expect(gapNote(-50_000, undefined)).toBe("Di Antrean cek");
    expect(
      gapNote(-50_000, {
        closedAt: "2026-11-15T01:00:00Z",
        closedBy: { staffUserId: "u-admin", name: "Pak Hendra Gunawan" },
        note: "Uang ditemukan di laci.",
      }),
    ).toBe("Selisih ditutup");
  });
});

describe("countedRowViews", () => {
  it("shows a counted deposit with its result and who counted it", () => {
    expect(
      countedRowViews([
        deposit({
          status: "gap",
          confirmation: confirmation(2_450_000, -50_000),
        }),
      ]),
    ).toEqual([
      {
        id: "d-1",
        staffName: "Mas Dimas Saputra",
        recordedLine: "Disetor Sab 14 Nov, 16.20",
        expected: "Rp2.500.000",
        receipts: "3 kwitansi",
        received: "Rp2.450.000",
        result: { label: "Kurang Rp50.000", tone: "warning" },
        resultNote: "Di Antrean cek",
        countedBy: "Pak Hendra Gunawan",
        countedAt: "Sab 14 Nov, 17.02",
      },
    ]);
  });

  it("leaves out a deposit without its count", () => {
    expect(countedRowViews([deposit({ status: "matched" })])).toEqual([]);
  });
});

describe("countSheetView and depositReceiptView", () => {
  it("shows the drawer's figures", () => {
    expect(countSheetView(deposit({ photo: { status: "pending" } }))).toEqual({
      subtitle: "Mas Dimas Saputra · disetor Sabtu 14 Nov, 16.20",
      expected: "Rp2.500.000",
      receipts: "3 kwitansi",
      handed: "Rp2.500.000",
      photo: "Foto sedang diperiksa",
    });
  });

  it("shows a receipt with its units, short time in Jakarta and photo", () => {
    expect(depositReceiptView(payment({}))).toEqual({
      paymentId: "p-1",
      tenantName: "Bu Wati Handayani",
      detail: "Rumah A1 · Jum 13 Nov, 10.15",
      amount: "Rp1.050.000",
      photo: "Foto tersimpan",
    });
    expect(depositReceiptView(payment({ units: [] })).detail).toBe(
      "Jum 13 Nov, 10.15",
    );
  });
});

describe("countFormSchema and confirmRequestBody", () => {
  it("opens empty", () => {
    expect(COUNT_FORM_DEFAULTS).toEqual({ receivedAmount: "", note: "" });
  });

  it.each([
    { name: "grouped digits", input: "2.450.000", want: 2_450_000 },
    { name: "zero, which the API allows", input: "0", want: 0 },
  ])("turns $name into integer rupiah", ({ input, want }) => {
    expect(
      countFormSchema.parse({ receivedAmount: input, note: "" }).receivedAmount,
    ).toBe(want);
  });

  it.each([
    { name: "an empty field", input: "", message: EMPTY_RECEIVED_MESSAGE },
    { name: "letters", input: "2,5jt", message: RECEIVED_DIGITS_MESSAGE },
    {
      name: "more than the API allows",
      input: "1000000000001",
      message: RECEIVED_TOO_LARGE_MESSAGE,
    },
  ])("refuses $name", ({ input, message }) => {
    const result = countFormSchema.safeParse({
      receivedAmount: input,
      note: "",
    });
    expect(result.success).toBe(false);
    expect(result.error?.issues[0]?.message).toBe(message);
  });

  it("refuses a note over 500 letters", () => {
    const result = countFormSchema.safeParse({
      receivedAmount: "1",
      note: "a".repeat(501),
    });
    expect(result.error?.issues[0]?.message).toBe(NOTE_TOO_LONG_MESSAGE);
  });

  it("sends the note only when there is one, trimmed", () => {
    const blank = countFormSchema.parse({
      receivedAmount: "2.450.000",
      note: "   ",
    });
    expect(confirmRequestBody(blank)).toEqual({ receivedAmount: 2_450_000 });
    const noted = countFormSchema.parse({
      receivedAmount: "2.450.000",
      note: "  Uang kurang selembar. ",
    });
    expect(confirmRequestBody(noted)).toEqual({
      receivedAmount: 2_450_000,
      note: "Uang kurang selembar.",
    });
  });
});

describe("countSavedToast", () => {
  it("says it matched", () => {
    expect(
      countSavedToast(
        deposit({
          status: "matched",
          confirmation: confirmation(2_500_000, 0),
        }),
      ),
    ).toEqual({
      tone: "create",
      title: "Hitungan disimpan",
      description: "Cocok dengan total kwitansi.",
    });
  });

  it("names the gap the API saved", () => {
    expect(
      countSavedToast(
        deposit({
          status: "gap",
          confirmation: confirmation(2_450_000, -50_000),
        }),
      ),
    ).toEqual({
      tone: "caution",
      title: "Hitungan disimpan",
      description: "Selisih Rp50.000 masuk Antrean cek.",
    });
  });
});

describe("confirmErrorKind", () => {
  function apiError(status: number, code: string) {
    return new ApiError({ status, code, message: "Pesan dari API." });
  }

  it.each([
    {
      error: apiError(409, "deposit_already_confirmed"),
      want: "already_confirmed",
    },
    { error: apiError(403, "forbidden"), want: "other" },
    { error: apiError(400, "invalid_input"), want: "other" },
    { error: new Error("boom"), want: "other" },
  ])("classifies $want", ({ error, want }) => {
    expect(confirmErrorKind(error)).toBe(want);
  });
});

describe("depositLinkLabel", () => {
  it("names the deposits waiting, when there are any", () => {
    expect(depositLinkLabel(0)).toBe("Setoran tunai");
    expect(depositLinkLabel(2)).toBe("Setoran tunai, 2 menunggu dihitung");
  });
});
