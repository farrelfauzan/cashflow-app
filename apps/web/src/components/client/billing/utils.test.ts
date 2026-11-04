import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/errors";
import type {
  AccountInvoice,
  AccountPayment,
  Capability,
  Correction,
  TenantAccount,
} from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import {
  accountTiles,
  canCreateCorrection,
  canReadAccount,
  correctionDialogView,
  correctionFormError,
  correctionFormSchema,
  correctionInput,
  correctionRows,
  correctionSavedNotice,
  CREDIT_NOTE,
  invoiceCorrectionSubtitle,
  invoiceCorrectionTitle,
  isInvoicesQuery,
  paymentCorrectionTitle,
  daysBetween,
  debtAgeText,
  invoiceRows,
  invoiceStatusView,
  isAccountForbidden,
  isOverdue,
  monthsBetween,
  NO_DEBT_TEXT,
  orderedInvoices,
  orderedPayments,
  overdueNote,
  PAYMENT_METHOD_LABELS,
  paymentCount,
  paymentRows,
  settlingPaymentId,
} from "./utils";

const TODAY = "2026-11-02";
const MINUS = "−";

const HENDRA = { memberId: "m-hendra", name: "Pak Hendra Gunawan" };
const SARI = { memberId: "m-sari", name: "Mbak Sari Wulandari" };

const invoice = (over: Partial<AccountInvoice> = {}): AccountInvoice => ({
  id: "inv-oct",
  kind: "regular",
  leaseId: "lease-1",
  unitLabel: "Kamar 3",
  propertyName: "Kos Melati",
  period: "2026-10-10",
  periodEnd: "2026-11-09",
  dueDate: "2026-10-10",
  issuedOn: "2026-10-10",
  amount: 1_500_000,
  billed: 1_500_000,
  paid: 0,
  outstanding: 1_500_000,
  status: "open",
  lines: [],
  corrections: [],
  ...over,
});

const payment = (over: Partial<AccountPayment> = {}): AccountPayment => ({
  id: "pay-1",
  amount: 1_000_000,
  net: 1_000_000,
  method: "cash",
  receivedOn: "2026-10-25",
  note: "",
  allocations: [],
  corrections: [],
  createdAt: "2026-10-25T03:00:00Z",
  ...over,
});

const correction = (over: Partial<Correction> = {}): Correction => ({
  id: "cor-1",
  amountDelta: -100_000,
  reason: "Potongan kran rusak",
  createdBy: HENDRA,
  createdAt: "2026-11-01T03:00:00Z",
  ...over,
});

const account = (over: Partial<TenantAccount> = {}): TenantAccount => ({
  tenantId: "tenant-1",
  outstanding: 0,
  overdue: 0,
  credit: 0,
  invoices: [],
  payments: [],
  ...over,
});

describe("daysBetween and monthsBetween", () => {
  it.each([
    { from: "2026-11-01", to: "2026-11-02", days: 1, months: 0 },
    { from: "2026-11-02", to: "2026-11-02", days: 0, months: 0 },
    { from: "2026-11-03", to: "2026-11-02", days: -1, months: -1 },
    { from: "2026-10-10", to: "2026-11-09", days: 30, months: 0 },
    { from: "2026-10-10", to: "2026-11-10", days: 31, months: 1 },
    { from: "2026-02-28", to: "2026-03-01", days: 1, months: 0 },
    { from: "2025-12-31", to: "2026-03-31", days: 90, months: 3 },
    { from: "2026-01-31", to: "2026-03-30", days: 58, months: 1 },
  ])("$from to $to", ({ from, to, days, months }) => {
    expect(daysBetween(from, to)).toBe(days);
    expect(monthsBetween(from, to)).toBe(months);
  });
});

describe("isOverdue", () => {
  it.each([
    {
      name: "open, due yesterday",
      over: { dueDate: "2026-11-01" },
      want: true,
    },
    {
      name: "partial, due yesterday",
      over: { dueDate: "2026-11-01", status: "partial" as const },
      want: true,
    },
    { name: "open, due today", over: { dueDate: TODAY }, want: false },
    {
      name: "open, due tomorrow",
      over: { dueDate: "2026-11-03" },
      want: false,
    },
    {
      name: "paid, due long ago",
      over: { dueDate: "2026-09-01", status: "paid" as const },
      want: false,
    },
    {
      name: "cancelled, due long ago",
      over: { dueDate: "2026-09-01", status: "cancelled" as const },
      want: false,
    },
  ])("$name", ({ over, want }) => {
    expect(isOverdue(invoice(over), TODAY)).toBe(want);
  });
});

describe("invoiceStatusView", () => {
  it.each([
    {
      name: "paid",
      over: { status: "paid" as const },
      label: "Lunas",
      tone: "active",
    },
    {
      name: "cancelled",
      over: { status: "cancelled" as const },
      label: "Dibatalkan",
      tone: "ended",
    },
    {
      name: "open, due today (the boundary)",
      over: { dueDate: TODAY },
      label: "Belum jatuh tempo",
      tone: "upcoming",
    },
    {
      name: "open, due later",
      over: { dueDate: "2026-12-01" },
      label: "Belum jatuh tempo",
      tone: "upcoming",
    },
    {
      name: "open, due yesterday",
      over: { dueDate: "2026-11-01" },
      label: "Terlambat 1 hari",
      tone: "warning",
    },
    {
      name: "partial, due today",
      over: { status: "partial" as const, dueDate: TODAY },
      label: "Sebagian",
      tone: "neutral",
    },
    {
      name: "partial, due 23 days ago",
      over: { status: "partial" as const, dueDate: "2026-10-10" },
      label: "Terlambat 23 hari",
      tone: "warning",
    },
  ])("$name", ({ over, label, tone }) => {
    expect(invoiceStatusView(invoice(over), TODAY)).toEqual({ label, tone });
  });
});

describe("overdueNote", () => {
  const sep = invoice({ id: "s", period: "2026-09-10", dueDate: "2026-09-10" });
  const oct = invoice({ id: "o", period: "2026-10-10", dueDate: "2026-10-10" });
  const aug = invoice({ id: "a", period: "2026-08-10", dueDate: "2026-08-10" });
  const notDue = invoice({
    id: "n",
    period: "2026-11-10",
    dueDate: "2026-11-10",
  });
  const paid = invoice({ id: "p", period: "2026-07-10", status: "paid" });

  it.each([
    { name: "none overdue", invoices: [paid, notDue], want: null },
    { name: "one month", invoices: [paid, oct, notDue], want: "sisa Oktober" },
    {
      name: "two months, oldest first",
      invoices: [sep, oct, notDue],
      want: "sisa September dan Oktober",
    },
    {
      name: "three or more",
      invoices: [aug, sep, oct],
      want: "sisa 3 tagihan",
    },
  ])("$name", ({ invoices, want }) => {
    expect(overdueNote(invoices, TODAY)).toBe(want);
  });
});

describe("debtAgeText", () => {
  it.each([
    { due: "2026-11-01", want: "1 hari" },
    { due: "2026-10-03", want: "30 hari" },
    { due: "2026-10-02", want: "1 bulan" },
    { due: "2026-08-15", want: "2 bulan" },
    // Never "0 hari": debt starts the day after the due date.
    { due: TODAY, want: "1 hari" },
  ])("due $due is $want", ({ due, want }) => {
    expect(debtAgeText(due, TODAY)).toBe(want);
  });
});

describe("accountTiles", () => {
  it("shows Rp0 and no debt for a tenant without invoices", () => {
    expect(accountTiles(account(), TODAY)).toEqual([
      {
        label: "Tunggakan",
        value: "Rp0",
        notes: [NO_DEBT_TEXT],
        tone: "default",
      },
      {
        label: "Tunggakan tertua",
        value: "—",
        notes: [NO_DEBT_TEXT],
        tone: "default",
      },
      {
        label: "Saldo lebih bayar",
        value: "Rp0",
        notes: [CREDIT_NOTE],
        tone: "default",
      },
    ]);
  });

  it("names the overdue months, the oldest one and what is owed in all", () => {
    const tiles = accountTiles(
      account({
        overdue: 1_250_000,
        outstanding: 2_750_000,
        oldestUnpaidDueDate: "2026-10-01",
        credit: 0,
        invoices: [
          invoice({ id: "a", period: "2026-09-01", status: "paid" }),
          invoice({
            id: "b",
            period: "2026-10-01",
            dueDate: "2026-10-01",
            status: "partial",
          }),
          invoice({ id: "c", period: "2026-11-01", dueDate: "2026-11-09" }),
        ],
      }),
      TODAY,
    );
    expect(tiles[0]).toEqual({
      label: "Tunggakan",
      value: "Rp1.250.000",
      notes: ["sisa Oktober", "Total belum dibayar Rp2.750.000"],
      tone: "warning",
    });
    expect(tiles[1]).toEqual({
      label: "Tunggakan tertua",
      value: "1 bulan",
      notes: ["Oktober 2026"],
      tone: "default",
    });
  });

  it("shows what is not yet due when nothing is overdue", () => {
    const tiles = accountTiles(
      account({
        outstanding: 1_300_000,
        oldestUnpaidDueDate: "2026-11-09",
        credit: 50_000,
        invoices: [invoice({ dueDate: "2026-11-09" })],
      }),
      TODAY,
    );
    expect(tiles[0]?.notes).toEqual([
      NO_DEBT_TEXT,
      "Total belum dibayar Rp1.300.000",
    ]);
    expect(tiles[1]?.value).toBe("—");
    expect(tiles[2]?.value).toBe("Rp50.000");
  });
});

describe("ordering", () => {
  it("lists invoices newest period first", () => {
    const ids = orderedInvoices([
      invoice({ id: "sep", period: "2026-09-10" }),
      invoice({ id: "oct", period: "2026-10-10" }),
      invoice({ id: "nov", period: "2026-11-10" }),
    ]).map((each) => each.id);
    expect(ids).toEqual(["nov", "oct", "sep"]);
  });

  it("lists payments newest received first, then the one recorded last", () => {
    const ids = orderedPayments([
      payment({ id: "old", receivedOn: "2026-10-01" }),
      payment({
        id: "same-day-early",
        receivedOn: "2026-10-25",
        createdAt: "2026-10-25T01:00:00Z",
      }),
      payment({
        id: "same-day-late",
        receivedOn: "2026-10-25",
        createdAt: "2026-10-25T09:00:00Z",
      }),
      payment({ id: "new", receivedOn: "2026-11-01" }),
    ]).map((each) => each.id);
    expect(ids).toEqual(["new", "same-day-late", "same-day-early", "old"]);
  });
});

describe("payment counts and the settling payment", () => {
  const paid = invoice({ id: "oct", status: "paid" });
  const first = payment({
    id: "p1",
    receivedOn: "2026-10-12",
    allocations: [{ invoiceId: "oct", amount: 500_000 }],
  });
  const second = payment({
    id: "p2",
    receivedOn: "2026-10-25",
    allocations: [
      { invoiceId: "oct", amount: 1_000_000 },
      { invoiceId: "nov", amount: 250_000 },
    ],
  });

  it("counts the payments that pay part of an invoice", () => {
    expect(paymentCount("oct", [first, second])).toBe(2);
    expect(paymentCount("nov", [first, second])).toBe(1);
    expect(paymentCount("dec", [first, second])).toBe(0);
  });

  it("is the newest payment for a paid invoice, whatever the API's order", () => {
    expect(settlingPaymentId(paid, [first, second])).toBe("p2");
    expect(settlingPaymentId(paid, [second, first])).toBe("p2");
  });

  it("is null for an invoice that is not paid", () => {
    expect(
      settlingPaymentId(invoice({ id: "nov", status: "partial" }), [second]),
    ).toBeNull();
  });
});

describe("invoiceRows", () => {
  it("shows each status with its details, newest first", () => {
    const rows = invoiceRows(
      account({
        invoices: [
          invoice({
            id: "oct",
            period: "2026-10-01",
            dueDate: "2026-10-01",
            status: "paid",
            paid: 1_500_000,
            outstanding: 0,
            lines: [
              { id: "l1", kind: "rent", label: "Sewa", amount: 1_382_000 },
              {
                id: "l2",
                kind: "charge",
                label: "Listrik September",
                amount: 118_000,
              },
            ],
          }),
          invoice({
            id: "nov",
            period: "2026-11-01",
            dueDate: "2026-11-01",
            status: "partial",
            paid: 250_000,
            outstanding: 1_250_000,
          }),
          invoice({
            id: "dec",
            period: "2026-12-01",
            dueDate: "2026-12-01",
            amount: 1_500_000,
            billed: 1_400_000,
            outstanding: 1_400_000,
          }),
        ],
        payments: [
          payment({ allocations: [{ invoiceId: "oct", amount: 1_500_000 }] }),
        ],
      }),
      TODAY,
    );
    expect(rows.map((row) => row.title)).toEqual([
      "Desember 2026",
      "November 2026",
      "Oktober 2026",
    ]);
    const [dec, nov, oct] = rows;
    expect(dec).toMatchObject({
      correctionTitle: "Koreksi tagihan Desember 2026",
      due: "jatuh tempo 1 Des",
      amount: "Rp1.400.000",
      correctedFrom: "dikoreksi dari Rp1.500.000",
      struck: false,
      status: { label: "Belum jatuh tempo" },
      details: ["Sisa Rp1.400.000"],
      place: null,
    });
    expect(nov).toMatchObject({
      status: { label: "Terlambat 1 hari", tone: "warning" },
      details: ["dibayar Rp250.000", "Sisa Rp1.250.000"],
      correctedFrom: null,
    });
    expect(oct).toMatchObject({
      status: { label: "Lunas" },
      details: ["1 pembayaran"],
      lines: [
        { id: "l1", label: "Sewa", amount: "Rp1.382.000" },
        { id: "l2", label: "Listrik September", amount: "Rp118.000" },
      ],
    });
  });

  it("strikes a cancelled invoice's first amount, with no corrected-from line", () => {
    const [row] = invoiceRows(
      account({
        invoices: [
          invoice({
            status: "cancelled",
            billed: 0,
            outstanding: 0,
            corrections: [correction({ amountDelta: -1_500_000 })],
          }),
        ],
      }),
      TODAY,
    );
    expect(row).toMatchObject({
      amount: "Rp1.500.000",
      struck: true,
      correctedFrom: null,
      status: { label: "Dibatalkan" },
      details: [],
    });
  });

  it("names the unit when the invoices come from several leases", () => {
    const rows = invoiceRows(
      account({
        invoices: [
          invoice({ id: "a", leaseId: "lease-1" }),
          invoice({
            id: "b",
            leaseId: "lease-2",
            unitLabel: "Ruko 2",
            propertyName: "Ruko Kaliurang",
          }),
        ],
      }),
      TODAY,
    );
    expect(rows.map((row) => row.place)).toEqual([
      "Ruko 2 · Ruko Kaliurang",
      "Kamar 3 · Kos Melati",
    ]);
  });
});

describe("paymentRows", () => {
  const oct = invoice({ id: "oct", period: "2026-10-01", status: "paid" });
  const nov = invoice({ id: "nov", period: "2026-11-01", status: "partial" });

  it("marks (lunas) only on the payment that settled the invoice", () => {
    const rows = paymentRows(
      account({
        invoices: [oct, nov],
        payments: [
          payment({
            id: "p1",
            receivedOn: "2026-10-12",
            method: "transfer",
            allocations: [{ invoiceId: "oct", amount: 750_000 }],
          }),
          payment({
            id: "p2",
            receivedOn: "2026-11-25",
            method: "qris",
            note: "Titip ke Mas Dimas",
            recordedBy: SARI,
            allocations: [
              { invoiceId: "oct", amount: 750_000 },
              { invoiceId: "nov", amount: 250_000 },
            ],
          }),
        ],
      }),
    );
    expect(rows).toEqual([
      {
        id: "p2",
        title: "25 Nov 2026 · QRIS",
        amount: "Rp1.000.000",
        correctedFrom: null,
        note: "Titip ke Mas Dimas",
        recordedBy: "dicatat Mbak Sari Wulandari",
        allocations: [
          { invoiceId: "oct", text: "→ Oktober Rp750.000 (lunas)" },
          { invoiceId: "nov", text: "→ November Rp250.000" },
        ],
        correctionTitle: "Koreksi pembayaran 25 Nov 2026",
      },
      {
        id: "p1",
        title: "12 Okt 2026 · Transfer",
        amount: "Rp1.000.000",
        correctedFrom: null,
        note: null,
        recordedBy: null,
        allocations: [{ invoiceId: "oct", text: "→ Oktober Rp750.000" }],
        correctionTitle: "Koreksi pembayaran 12 Okt 2026",
      },
    ]);
  });

  it("shows the net amount and what was first recorded after a correction", () => {
    const [row] = paymentRows(
      account({ payments: [payment({ amount: 1_000_000, net: 900_000 })] }),
    );
    expect(row?.amount).toBe("Rp900.000");
    expect(row?.correctedFrom).toBe("dikoreksi dari Rp1.000.000");
    expect(row?.allocations).toEqual([]);
  });
});

describe("PAYMENT_METHOD_LABELS", () => {
  it("names every method in Indonesian", () => {
    expect(PAYMENT_METHOD_LABELS).toEqual({
      cash: "Tunai",
      transfer: "Transfer",
      qris: "QRIS",
      va: "Virtual account",
    });
  });
});

describe("correctionRows", () => {
  it("is empty without corrections", () => {
    expect(correctionRows(account({ invoices: [invoice()] }))).toEqual([]);
  });

  it("gathers the corrections of invoices and payments, newest first", () => {
    const rows = correctionRows(
      account({
        invoices: [
          invoice({
            period: "2026-11-01",
            corrections: [
              correction({
                id: "c-inv",
                invoiceId: "inv-oct",
                amountDelta: -50_000,
                createdAt: "2026-11-01T18:00:00Z",
              }),
            ],
          }),
        ],
        payments: [
          payment({
            receivedOn: "2026-10-25",
            corrections: [
              correction({
                id: "c-pay",
                paymentId: "pay-1",
                amountDelta: 100_000,
                reason: "Salah ketik",
                createdBy: SARI,
                createdAt: "2026-10-30T02:00:00Z",
              }),
            ],
          }),
        ],
      }),
    );
    expect(rows).toEqual([
      {
        id: "c-inv",
        // 18:00 UTC is the next day in Jakarta.
        title: "2 Nov 2026 · Tagihan November 2026",
        amount: `${MINUS}Rp50.000`,
        reason: "Potongan kran rusak",
        by: "oleh Pak Hendra Gunawan",
      },
      {
        id: "c-pay",
        title: "30 Okt 2026 · Pembayaran 25 Okt 2026",
        amount: "+Rp100.000",
        reason: "Salah ketik",
        by: "oleh Mbak Sari Wulandari",
      },
    ]);
  });
});

describe("canReadAccount", () => {
  it.each<{ name: string; capabilities: Capability[]; want: boolean }>([
    {
      name: "both reads allowed",
      capabilities: [
        { action: "invoice:read", access: "allow" },
        { action: "payment:read", access: "allow" },
      ],
      want: true,
    },
    {
      name: "both reads conditional",
      capabilities: [
        { action: "invoice:read", access: "conditional" },
        { action: "payment:read", access: "conditional" },
      ],
      want: true,
    },
    {
      name: "payment:read denied",
      capabilities: [
        { action: "invoice:read", access: "allow" },
        { action: "payment:read", access: "deny" },
      ],
      want: false,
    },
    {
      name: "invoice:read not listed",
      capabilities: [{ action: "payment:read", access: "allow" }],
      want: false,
    },
  ])("$name", ({ capabilities, want }) => {
    expect(canReadAccount(createAppAbility(capabilities))).toBe(want);
  });
});

describe("isAccountForbidden", () => {
  it.each([
    {
      name: "a 403",
      error: new ApiError({ status: 403, code: "forbidden", message: "x" }),
      want: true,
    },
    {
      name: "a 404",
      error: new ApiError({ status: 404, code: "not_found", message: "x" }),
      want: false,
    },
    {
      name: "a network error",
      error: new ApiError({ status: 0, code: "network_error", message: "x" }),
      want: false,
    },
    { name: "not an ApiError", error: new Error("x"), want: false },
  ])("$name", ({ error, want }) => {
    expect(isAccountForbidden(error)).toBe(want);
  });
});

describe("correction titles and subtitle", () => {
  it("names the invoice by its period month and the payment by its day", () => {
    expect(invoiceCorrectionTitle({ period: "2026-11-10" })).toBe(
      "Koreksi tagihan November 2026",
    );
    expect(paymentCorrectionTitle({ receivedOn: "2026-11-25" })).toBe(
      "Koreksi pembayaran 25 Nov 2026",
    );
  });

  it("names the tenant, unit and property of an invoice", () => {
    expect(
      invoiceCorrectionSubtitle(
        { unitLabel: "Kamar 3", propertyName: "Kos Melati" },
        "Yulia Kartika",
      ),
    ).toBe("Yulia Kartika · Kamar 3 · Kos Melati");
  });

  it("confirms the save with the dialog's title", () => {
    expect(correctionSavedNotice("Koreksi tagihan November 2026")).toBe(
      "Koreksi tagihan November 2026 disimpan.",
    );
  });
});

describe("correctionDialogView", () => {
  const data = account({
    invoices: [
      invoice({
        id: "inv-nov",
        period: "2026-11-10",
        amount: 1_500_000,
        billed: 1_400_000,
      }),
      invoice({
        id: "inv-dec",
        period: "2026-12-10",
        status: "cancelled",
        billed: 0,
      }),
    ],
    payments: [payment({ id: "pay-1", amount: 1_000_000, net: 900_000 })],
  });

  it("shows what an invoice bills now, not what it first billed", () => {
    expect(
      correctionDialogView(data, { kind: "invoice", id: "inv-nov" }, "Yulia"),
    ).toEqual({
      target: { invoiceId: "inv-nov" },
      title: "Koreksi tagihan November 2026",
      subtitle: "Yulia · Kamar 3 · Kos Melati",
      current: "Rp1.400.000",
    });
  });

  it("shows Rp0 for a cancelled invoice, which a correction can restore", () => {
    expect(
      correctionDialogView(data, { kind: "invoice", id: "inv-dec" }, "Yulia")
        ?.current,
    ).toBe("Rp0");
  });

  it("shows what a payment counts now (net)", () => {
    expect(
      correctionDialogView(data, { kind: "payment", id: "pay-1" }, "Yulia"),
    ).toEqual({
      target: { paymentId: "pay-1" },
      title: "Koreksi pembayaran 25 Okt 2026",
      subtitle: "Yulia",
      current: "Rp900.000",
    });
  });

  it("is null for a record no longer in the account", () => {
    expect(
      correctionDialogView(data, { kind: "invoice", id: "gone" }, "Yulia"),
    ).toBeNull();
    expect(
      correctionDialogView(data, { kind: "payment", id: "gone" }, "Yulia"),
    ).toBeNull();
  });
});

describe("correctionFormSchema", () => {
  const reason = "Potongan kran rusak";

  it.each([
    { name: "digits", newAmount: "1400000", want: 1_400_000 },
    {
      name: "digits grouped by the field",
      newAmount: "1.400.000",
      want: 1_400_000,
    },
    { name: "zero", newAmount: "0", want: 0 },
    {
      name: "the maximum",
      newAmount: "1.000.000.000.000",
      want: 1_000_000_000_000,
    },
  ])("takes $name as integer rupiah", ({ newAmount, want }) => {
    const parsed = correctionFormSchema.safeParse({ newAmount, reason });
    expect(parsed.success && parsed.data.newAmount).toBe(want);
  });

  it.each([
    { name: "empty", newAmount: "", message: "Isi nominal baru." },
    { name: "only spaces", newAmount: "  ", message: "Isi nominal baru." },
    { name: "letters", newAmount: "12a", message: "Hanya angka." },
    { name: "a sign", newAmount: "-5000", message: "Hanya angka." },
    { name: "a fraction", newAmount: "1500,5", message: "Hanya angka." },
    {
      name: "beyond the maximum",
      newAmount: "1000000000001",
      message: "Nominal paling besar Rp1.000.000.000.000.",
    },
  ])("refuses $name", ({ newAmount, message }) => {
    const parsed = correctionFormSchema.safeParse({ newAmount, reason });
    expect(parsed.success).toBe(false);
    expect(
      parsed.error?.issues.map((issue) => [issue.path, issue.message]),
    ).toEqual([[["newAmount"], message]]);
  });

  it("trims the reason", () => {
    const parsed = correctionFormSchema.safeParse({
      newAmount: "5",
      reason: "  Salah ketik  ",
    });
    expect(parsed.success && parsed.data.reason).toBe("Salah ketik");
  });

  it("takes a reason of exactly 500 characters", () => {
    const parsed = correctionFormSchema.safeParse({
      newAmount: "5",
      reason: "a".repeat(500),
    });
    expect(parsed.success).toBe(true);
  });

  it.each([
    { name: "empty", reason: "", message: "Alasan wajib diisi." },
    {
      name: "only whitespace",
      reason: " \n\t ",
      message: "Alasan wajib diisi.",
    },
    {
      name: "longer than 500 characters",
      reason: "a".repeat(501),
      message: "Alasan paling banyak 500 karakter.",
    },
  ])("refuses a reason that is $name", ({ reason: text, message }) => {
    const parsed = correctionFormSchema.safeParse({
      newAmount: "5",
      reason: text,
    });
    expect(
      parsed.error?.issues.map((issue) => [issue.path, issue.message]),
    ).toEqual([[["reason"], message]]);
  });
});

describe("correctionInput", () => {
  it("sends the new amount for an invoice, never amountDelta", () => {
    const body = correctionInput(
      "req-1",
      { invoiceId: "inv-1" },
      { newAmount: 0, reason: "Sewa tidak jadi" },
    );
    expect(body).toEqual({
      requestId: "req-1",
      invoiceId: "inv-1",
      newAmount: 0,
      reason: "Sewa tidak jadi",
    });
    expect(body).not.toHaveProperty("amountDelta");
  });

  it("names only the payment for a payment", () => {
    expect(
      correctionInput(
        "req-2",
        { paymentId: "pay-1" },
        { newAmount: 900_000, reason: "Salah ketik" },
      ),
    ).toEqual({
      requestId: "req-2",
      paymentId: "pay-1",
      newAmount: 900_000,
      reason: "Salah ketik",
    });
  });
});

describe("correctionFormError", () => {
  it.each([
    {
      name: "below zero (amountDelta) goes under Nominal baru",
      error: new ApiError({
        status: 409,
        code: "correction_below_zero",
        message: "Nominal tidak boleh di bawah nol.",
        field: "amountDelta",
      }),
      want: {
        field: "newAmount",
        message: "Nominal tidak boleh di bawah nol.",
      },
    },
    {
      name: "no change stays under Nominal baru",
      error: new ApiError({
        status: 409,
        code: "correction_no_change",
        message: "Nominal baru sama dengan nominal sekarang.",
        field: "newAmount",
      }),
      want: {
        field: "newAmount",
        message: "Nominal baru sama dengan nominal sekarang.",
      },
    },
    {
      name: "a reason error goes under Alasan",
      error: new ApiError({
        status: 400,
        code: "invalid",
        message: "Alasan wajib diisi.",
        field: "reason",
      }),
      want: { field: "reason", message: "Alasan wajib diisi." },
    },
    {
      name: "a reused requestId goes to the alert",
      error: new ApiError({
        status: 409,
        code: "request_id_reused",
        message: "Permintaan ini sudah dipakai.",
        field: "requestId",
      }),
      want: { field: null, message: "Permintaan ini sudah dipakai." },
    },
    {
      name: "a 403 goes to the alert",
      error: new ApiError({
        status: 403,
        code: "forbidden",
        message: "Anda tidak punya akses.",
      }),
      want: { field: null, message: "Anda tidak punya akses." },
    },
    {
      name: "a missing invoice goes to the alert",
      error: new ApiError({
        status: 404,
        code: "not_found",
        message: "Tagihan tidak ditemukan.",
        field: "invoiceId",
      }),
      want: { field: null, message: "Tagihan tidak ditemukan." },
    },
    {
      name: "anything else gets the generic message in the alert",
      error: new Error("boom"),
      want: { field: null, message: "Terjadi kesalahan. Coba lagi." },
    },
  ])("$name", ({ error, want }) => {
    expect(correctionFormError(error)).toEqual(want);
  });
});

describe("isInvoicesQuery", () => {
  const org = "org-1";
  it.each([
    {
      name: "the invoice list",
      key: ["org", org, `/orgs/${org}/invoices`, {}],
      want: true,
    },
    {
      name: "one invoice",
      key: ["org", org, `/orgs/${org}/invoices/inv-1`, {}],
      want: true,
    },
    {
      name: "another organization's invoices",
      key: ["org", "org-2", "/orgs/org-2/invoices", {}],
      want: false,
    },
    {
      name: "a path that only starts like it",
      key: ["org", org, `/orgs/${org}/invoices-export`, {}],
      want: false,
    },
    {
      name: "the dashboard",
      key: ["org", org, `/orgs/${org}/dashboard`, {}],
      want: false,
    },
    { name: "a user query", key: ["user", "/me", {}], want: false },
  ])("$name", ({ key, want }) => {
    expect(isInvoicesQuery(key, org)).toBe(want);
  });
});

describe("canCreateCorrection", () => {
  it.each<{ name: string; capabilities: Capability[]; want: boolean }>([
    {
      name: "allowed",
      capabilities: [{ action: "correction:create", access: "allow" }],
      want: true,
    },
    {
      name: "conditional",
      capabilities: [{ action: "correction:create", access: "conditional" }],
      want: true,
    },
    {
      name: "denied",
      capabilities: [{ action: "correction:create", access: "deny" }],
      want: false,
    },
    { name: "not listed", capabilities: [], want: false },
  ])("$name", ({ capabilities, want }) => {
    expect(canCreateCorrection(createAppAbility(capabilities))).toBe(want);
  });
});
