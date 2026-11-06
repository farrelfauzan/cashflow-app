import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/errors";
import type {
  AccountInvoice,
  AccountPayment,
  ActivePromise,
  Capability,
  Correction,
  TenantAccount,
  TenantPromise,
  TenantPromiseVersion,
} from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import {
  accountTiles,
  activePromiseReplacesLine,
  activePromiseTile,
  brokenPromisesNote,
  canCreatePromise,
  canReadPromises,
  isPromiseStaleError,
  NO_ACTIVE_PROMISE_TEXT,
  promiseByText,
  promiseDialogView,
  promiseHistoryCountNote,
  promiseHistoryRows,
  staffPromiseFormError,
  staffPromiseSchema,
  canCreateCorrection,
  canReadAccount,
  correctionDialogView,
  correctionFormError,
  correctionFormSchema,
  correctionInput,
  correctionRows,
  correctionSavedNotice,
  CREDIT_NOTE,
  FINAL_INVOICE_LABEL,
  invoiceCorrectionSubtitle,
  invoiceCorrectionTitle,
  invoiceName,
  invoiceTitle,
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
  activePromise: null,
  brokenPromises: 0,
  promiseDates: null,
  nextInvoiceDueDate: null,
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
        label: "Janji aktif",
        value: "—",
        notes: [NO_ACTIVE_PROMISE_TEXT],
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
    expect(tiles[3]?.value).toBe("Rp50.000");
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

describe("the final invoice's names", () => {
  // The final invoice's period is the lease's last day (31 Oct for a tenant who left then).
  const final = invoice({
    id: "fin",
    kind: "final",
    period: "2026-10-31",
    periodEnd: "2026-10-31",
    dueDate: "2026-10-31",
    issuedOn: "2026-10-31",
    amount: 180_000,
    billed: 180_000,
    outstanding: 180_000,
  });
  const oct = invoice({ id: "o", period: "2026-10-01", dueDate: "2026-10-01" });

  it.each([
    {
      name: "a regular invoice",
      of: oct,
      title: "Oktober 2026",
      name_: "Tagihan Oktober 2026",
      correction: "Koreksi tagihan Oktober 2026",
    },
    {
      name: "the final invoice",
      of: final,
      title: "Tagihan akhir Oktober 2026",
      name_: "Tagihan akhir Oktober 2026",
      correction: "Koreksi tagihan akhir Oktober 2026",
    },
  ])("names $name", ({ of, title, name_, correction }) => {
    expect(invoiceTitle(of)).toBe(title);
    expect(invoiceName(of)).toBe(name_);
    expect(invoiceCorrectionTitle(of)).toBe(correction);
  });

  it("reads an invoice without a kind (a billed meter amount) as a regular one", () => {
    expect(invoiceCorrectionTitle({ period: "2026-11-10" })).toBe(
      "Koreksi tagihan November 2026",
    );
    expect(FINAL_INVOICE_LABEL).toBe("Tagihan akhir");
  });

  it("titles its row in Tagihan and the correction subject", () => {
    const data = account({
      invoices: [
        oct,
        {
          ...final,
          corrections: [correction({ id: "c-fin", amountDelta: -20_000 })],
        },
      ],
    });
    expect(invoiceRows(data, TODAY).map((row) => row.title)).toEqual([
      "Tagihan akhir Oktober 2026",
      "Oktober 2026",
    ]);
    expect(
      invoiceRows(data, TODAY).find((row) => row.id === "fin")?.correctionTitle,
    ).toBe("Koreksi tagihan akhir Oktober 2026");
    expect(correctionRows(data)[0]?.title).toContain(
      "Tagihan akhir Oktober 2026",
    );
  });

  it("says tagihan akhir where a sentence names invoices by month", () => {
    expect(overdueNote([oct, final], TODAY)).toBe(
      "sisa Oktober dan tagihan akhir",
    );
    const rows = paymentRows(
      account({
        invoices: [oct, final],
        payments: [
          payment({
            id: "p1",
            allocations: [
              { invoiceId: "o", amount: 1_500_000 },
              { invoiceId: "fin", amount: 180_000 },
            ],
          }),
        ],
      }),
    );
    expect(rows[0]?.allocations.map((each) => each.text)).toEqual([
      "→ Oktober Rp1.500.000",
      "→ tagihan akhir Rp180.000",
    ]);
  });
});

const activePromise = (over: Partial<ActivePromise> = {}): ActivePromise => ({
  id: "promise-1",
  version: 1,
  kind: "specific_date",
  promisedDate: "2026-12-12",
  promisedAmount: 1_250_000,
  payingNow: 0,
  source: "tenant_page",
  note: null,
  createdBy: null,
  createdAt: "2026-11-25T03:00:00Z",
  coveredInvoiceIds: ["inv-oct"],
  ...over,
});

const promiseVersion = (
  over: Partial<TenantPromiseVersion> = {},
): TenantPromiseVersion => ({
  version: 1,
  kind: "specific_date",
  promisedDate: "2026-12-12",
  promisedAmount: 1_250_000,
  payingNow: 0,
  outstandingAt: 1_250_000,
  source: "tenant_page",
  note: null,
  createdBy: null,
  createdAt: "2026-11-25T03:00:00Z",
  coveredInvoiceIds: ["inv-oct"],
  ...over,
});

const tenantPromise = (over: Partial<TenantPromise> = {}): TenantPromise => ({
  id: "promise-1",
  tenantId: "tenant-1",
  status: "active",
  createdAt: "2026-11-25T03:00:00Z",
  closedAt: null,
  versions: [promiseVersion()],
  ...over,
});

const OFFER_DATES = { earliest: "2026-11-03", latest: "2027-01-01" };

describe("promiseByText and brokenPromisesNote", () => {
  it("names who made a promise", () => {
    expect(promiseByText("tenant_page", null)).toBe("dibuat penyewa");
    expect(promiseByText("admin", HENDRA)).toBe("dicatat Pak Hendra Gunawan");
    expect(promiseByText("karyawan", null)).toBeNull();
  });

  it("counts broken promises as history, none for 0", () => {
    expect(brokenPromisesNote(0)).toBeNull();
    expect(brokenPromisesNote(2)).toBe("Janji lewat 2× (sejak awal)");
  });
});

describe("activePromiseTile", () => {
  it("shows the promised day, the amount and that the tenant made it", () => {
    expect(
      activePromiseTile(account({ activePromise: activePromise() })),
    ).toEqual({
      label: "Janji aktif",
      value: "Sab, 12 Des",
      notes: ["Rp1.250.000 · dibuat penyewa"],
      tone: "default",
    });
  });

  it("names the member, the note and the broken count", () => {
    const tile = activePromiseTile(
      account({
        brokenPromises: 1,
        activePromise: activePromise({
          promisedAmount: null,
          source: "admin",
          createdBy: SARI,
          note: "Gajian tanggal 10",
        }),
      }),
    );
    expect(tile.notes).toEqual([
      "Semua sisa tagihan · dicatat Mbak Sari Wulandari",
      "Gajian tanggal 10",
      "Janji lewat 1× (sejak awal)",
    ]);
  });

  it("shows a dash without an active promise, with the broken count", () => {
    const tile = activePromiseTile(account({ brokenPromises: 3 }));
    expect(tile.value).toBe("—");
    expect(tile.notes).toEqual([
      NO_ACTIVE_PROMISE_TEXT,
      "Janji lewat 3× (sejak awal)",
    ]);
  });

  it("sits between Tunggakan tertua and Saldo lebih bayar", () => {
    expect(accountTiles(account(), TODAY).map((tile) => tile.label)).toEqual([
      "Tunggakan",
      "Tunggakan tertua",
      "Janji aktif",
      "Saldo lebih bayar",
    ]);
  });
});

describe("promiseHistoryRows", () => {
  it("shows each promise from its newest version, with its chip", () => {
    const rows = promiseHistoryRows([
      tenantPromise(),
      tenantPromise({
        id: "promise-0",
        status: "partially_kept",
        versions: [
          promiseVersion({
            kind: "with_next_invoice",
            promisedDate: "2026-11-01",
            promisedAmount: null,
            createdAt: "2026-10-05T03:00:00Z",
          }),
        ],
      }),
    ]);
    expect(rows).toEqual([
      {
        id: "promise-1",
        title: "Sab, 12 Des · Rp1.250.000",
        byline: "dibuat penyewa 25 Nov",
        status: { label: "Aktif", tone: "upcoming" },
      },
      {
        id: "promise-0",
        title: "Min, 1 Nov · Semua sisa tagihan",
        byline: "dibuat penyewa 5 Okt",
        status: { label: "Ditepati sebagian", tone: "neutral" },
      },
    ]);
  });

  it("names the member of the newest version and counts the edits", () => {
    const [row] = promiseHistoryRows([
      tenantPromise({
        status: "broken",
        versions: [
          promiseVersion(),
          promiseVersion({
            version: 2,
            promisedDate: "2026-12-20",
            source: "admin",
            createdBy: HENDRA,
            // 23:30 in Jakarta is still 26 Nov there.
            createdAt: "2026-11-26T16:30:00Z",
          }),
        ],
      }),
    ]);
    expect(row?.title).toBe("Min, 20 Des · Rp1.250.000");
    expect(row?.byline).toBe("dicatat Pak Hendra Gunawan 26 Nov · diubah 1×");
    expect(row?.status).toEqual({ label: "Tidak ditepati", tone: "warning" });
  });

  it("labels a kept promise and skips one without versions", () => {
    const rows = promiseHistoryRows([
      tenantPromise({ status: "kept" }),
      tenantPromise({ id: "empty", versions: [] }),
    ]);
    expect(rows.map((row) => row.status.label)).toEqual(["Ditepati"]);
  });
});

describe("promiseHistoryCountNote", () => {
  it("says how many show only when the page is not all of them", () => {
    const items = Array.from({ length: 20 }, (_, index) =>
      tenantPromise({ id: `p-${index}` }),
    );
    expect(
      promiseHistoryCountNote({ items, page: 1, pageSize: 20, total: 23 }),
    ).toBe("Menampilkan 20 dari 23");
    expect(
      promiseHistoryCountNote({ items, page: 1, pageSize: 20, total: 20 }),
    ).toBeNull();
  });
});

describe("promise abilities", () => {
  const ability = (capabilities: Capability[]) =>
    createAppAbility(capabilities);

  it("reads and creates promises by action, conditional included", () => {
    const admin = ability([
      { action: "promise:read", access: "allow" },
      { action: "promise:create", access: "conditional" },
    ]);
    expect(canReadPromises(admin)).toBe(true);
    expect(canCreatePromise(admin)).toBe(true);
    const supervisor = ability([
      { action: "promise:read", access: "allow" },
      { action: "promise:create", access: "deny" },
    ]);
    expect(canReadPromises(supervisor)).toBe(true);
    expect(canCreatePromise(supervisor)).toBe(false);
    expect(canReadPromises(ability([]))).toBe(false);
  });
});

describe("promiseDialogView", () => {
  it("is null when no promise can be made", () => {
    expect(promiseDialogView(account(), "Yulia Kartika")).toBeNull();
  });

  it("opens empty on Bareng tagihan berikutnya when it is offered", () => {
    const view = promiseDialogView(
      account({
        outstanding: 1_250_000,
        promiseDates: OFFER_DATES,
        nextInvoiceDueDate: "2026-12-01",
      }),
      "Yulia Kartika",
    );
    expect(view).toMatchObject({
      subtitle: "Yulia Kartika",
      replacesLine: null,
      dateMin: "2026-11-03",
      dateMax: "2027-01-01",
      amountHint: "Kosongkan untuk semua sisa, Rp1.250.000.",
      primaryLabel: "Simpan janji",
      defaults: {
        kind: "with_next_invoice",
        date: "",
        amountText: "",
        note: "",
      },
    });
    expect(view?.kindOptions).toEqual([
      {
        value: "with_next_invoice",
        label: "Bareng tagihan berikutnya",
        sub: "Jatuh tempo 1 Des 2026",
      },
      {
        value: "specific_date",
        label: "Pilih tanggal sendiri",
        sub: "Paling lambat 1 Jan 2027, tanpa denda",
      },
    ]);
  });

  it("opens on Pilih tanggal sendiri when there is no next invoice", () => {
    const view = promiseDialogView(
      account({ outstanding: 500_000, promiseDates: OFFER_DATES }),
      "Yulia Kartika",
    );
    expect(view?.defaults.kind).toBe("specific_date");
    expect(view?.kindOptions.map((option) => option.value)).toEqual([
      "specific_date",
    ]);
  });

  it("opens filled from the active promise, which it replaces", () => {
    const view = promiseDialogView(
      account({
        outstanding: 1_250_000,
        promiseDates: OFFER_DATES,
        nextInvoiceDueDate: "2026-12-01",
        activePromise: activePromise({
          promisedAmount: 750_000,
          note: "Gajian",
        }),
      }),
      "Yulia Kartika",
    );
    expect(view?.replacesLine).toBe(
      "Janji aktif Sab, 12 Des, Rp750.000 akan diganti.",
    );
    expect(view?.primaryLabel).toBe("Simpan perubahan");
    expect(view?.defaults).toEqual({
      kind: "specific_date",
      date: "2026-12-12",
      amountText: "750.000",
      note: "",
    });
  });

  it("names a promise of everything left in lower case", () => {
    expect(
      activePromiseReplacesLine(activePromise({ promisedAmount: null })),
    ).toBe("Janji aktif Sab, 12 Des, semua sisa tagihan akan diganti.");
  });
});

describe("staffPromiseSchema", () => {
  const offer = {
    outstanding: 1_250_000,
    earliest: "2026-11-03",
    latest: "2027-01-01",
    nextInvoiceDueDate: "2026-12-01",
  };
  const values = {
    kind: "specific_date" as const,
    date: "2026-12-12",
    amountText: "",
    note: "",
  };
  const issues = (input: typeof values | Record<string, string>) => {
    const result = staffPromiseSchema(offer).safeParse(input);
    return result.success
      ? []
      : result.error.issues.map((issue) => ({
          path: issue.path.join("."),
          message: issue.message,
        }));
  };

  it("sends the date, the typed amount and the trimmed note, never payingNow", () => {
    expect(
      staffPromiseSchema(offer).parse({
        ...values,
        amountText: "750.000",
        note: "  Gajian tanggal 10 ",
      }),
    ).toEqual({
      kind: "specific_date",
      promisedDate: "2026-12-12",
      promisedAmount: 750_000,
      note: "Gajian tanggal 10",
    });
  });

  it("sends only the kind for Bareng tagihan with nothing typed", () => {
    expect(
      staffPromiseSchema(offer).parse({
        ...values,
        kind: "with_next_invoice",
        note: "   ",
      }),
    ).toEqual({ kind: "with_next_invoice" });
  });

  it.each([
    {
      name: "a missing date",
      input: { ...values, date: "" },
      want: [{ path: "date", message: "Pilih tanggal pelunasan." }],
    },
    {
      name: "a date after the last one",
      input: { ...values, date: "2027-01-02" },
      want: [
        {
          path: "date",
          message: "Pilih tanggal dari 3 Nov 2026 sampai 1 Jan 2027.",
        },
      ],
    },
    {
      name: "an amount above what is owed",
      input: { ...values, amountText: "1.250.001" },
      want: [
        {
          path: "amountText",
          message: "Nominal janji paling banyak Rp1.250.000.",
        },
      ],
    },
    {
      name: "an amount of 0",
      input: { ...values, amountText: "0" },
      want: [
        {
          path: "amountText",
          message: "Isi nominal minimal Rp1, atau kosongkan untuk semua sisa.",
        },
      ],
    },
    {
      name: "a note too long",
      input: { ...values, note: "a".repeat(501) },
      want: [{ path: "note", message: "Catatan paling banyak 500 karakter." }],
    },
  ])("refuses $name", ({ input, want }) => {
    expect(issues(input)).toEqual(want);
  });
});

describe("staffPromiseFormError", () => {
  const apiError = (code: string, status: number, field?: string) =>
    new ApiError({ status, code, message: `pesan ${code}`, field });

  it.each([
    {
      name: "a date out of range under the date",
      error: apiError("promise_date_out_of_range", 400, "promisedDate"),
      kind: "specific_date" as const,
      want: { field: "date", message: "pesan promise_date_out_of_range" },
    },
    {
      name: "a date error while the date input is hidden in the alert",
      error: apiError("promise_date_out_of_range", 400, "promisedDate"),
      kind: "with_next_invoice" as const,
      want: { field: null, message: "pesan promise_date_out_of_range" },
    },
    {
      name: "no next invoice under the choice",
      error: apiError("promise_no_next_invoice", 400, "kind"),
      kind: "with_next_invoice" as const,
      want: { field: "kind", message: "pesan promise_no_next_invoice" },
    },
    {
      name: "an amount above what is owed under the amount",
      error: apiError(
        "promise_amount_above_outstanding",
        400,
        "promisedAmount",
      ),
      kind: "specific_date" as const,
      want: {
        field: "amountText",
        message: "pesan promise_amount_above_outstanding",
      },
    },
    {
      name: "a note under the note",
      error: apiError("invalid_input", 400, "note"),
      kind: "specific_date" as const,
      want: { field: "note", message: "pesan invalid_input" },
    },
    {
      name: "nothing owed in the alert",
      error: apiError("promise_nothing_owed", 409),
      kind: "specific_date" as const,
      want: { field: null, message: "pesan promise_nothing_owed" },
    },
    {
      name: "a refusal in the alert",
      error: apiError("forbidden", 403),
      kind: "specific_date" as const,
      want: { field: null, message: "pesan forbidden" },
    },
  ])("puts $name", ({ error, kind, want }) => {
    expect(staffPromiseFormError(error, kind)).toEqual(want);
  });

  it("shows the generic message for anything that is not the API's answer", () => {
    expect(
      staffPromiseFormError(new Error("network"), "specific_date").field,
    ).toBeNull();
  });

  it("reloads the account after the refusals that mean it moved", () => {
    expect(isPromiseStaleError(apiError("promise_nothing_owed", 409))).toBe(
      true,
    );
    expect(
      isPromiseStaleError(apiError("promise_date_out_of_range", 400)),
    ).toBe(true);
    expect(isPromiseStaleError(apiError("forbidden", 403))).toBe(false);
    expect(isPromiseStaleError(new Error("network"))).toBe(false);
  });
});
