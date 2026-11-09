import { describe, expect, it } from "vitest";

import type {
  AccountInvoice,
  AccountPayment,
  Capability,
  PaymentResult,
} from "@/lib/api/generated/model";
import { getListInvoicesQueryKey } from "@/lib/api/generated/money/money";
import { getListTenantsQueryKey } from "@/lib/api/generated/tenants/tenants";
import { createAppAbility } from "@/lib/capabilities";

import type { PaymentFormValues } from "./type";
import {
  ALL_CREDIT_TEXT,
  AMOUNT_DIGITS_MESSAGE,
  AMOUNT_TOO_LARGE_MESSAGE,
  AMOUNT_ZERO_MESSAGE,
  canRecordPayment,
  CREDIT_NOTE,
  EMPTY_AMOUNT_MESSAGE,
  EMPTY_RECEIVED_ON_MESSAGE,
  isInvoicesQueryKey,
  NOTE_TOO_LONG_MESSAGE,
  NOTHING_OWED_TEXT,
  paidRows,
  paymentFormDefaults,
  paymentFormSchema,
  paymentHeadline,
  paymentRequestBody,
  paymentResultView,
  RECEIVED_ON_FUTURE_MESSAGE,
  resultBoxRows,
  unpaidSummary,
  paymentRecordedToast,
} from "./utils";

const TODAY = "2026-11-03";
const ORG = "0b9f7c1e-2a34-4c56-8d78-9e0f1a2b3c4d";

function values(overrides: Partial<PaymentFormValues>): PaymentFormValues {
  return { ...paymentFormDefaults(TODAY), amount: "700.000", ...overrides };
}

function check(overrides: Partial<PaymentFormValues>) {
  return paymentFormSchema(TODAY).safeParse(values(overrides));
}

function messages(overrides: Partial<PaymentFormValues>): string[] {
  const result = check(overrides);
  return result.success
    ? []
    : result.error.issues.map((issue) => issue.message);
}

function invoice(overrides: Partial<AccountInvoice>): AccountInvoice {
  return {
    id: "inv-1",
    leaseId: "lease-1",
    unitLabel: "Kamar 07",
    propertyName: "Kos Melati",
    period: "2026-09-01",
    periodEnd: "2026-09-30",
    dueDate: "2026-09-01",
    amount: 1_200_000,
    billed: 1_200_000,
    paid: 0,
    outstanding: 1_200_000,
    status: "open",
    lines: [],
    corrections: [],
    ...overrides,
  } as AccountInvoice;
}

function payment(overrides: Partial<AccountPayment>): AccountPayment {
  return {
    id: "pay-1",
    amount: 700_000,
    net: 700_000,
    method: "cash",
    receivedOn: "2026-11-03",
    note: "",
    recordedBy: { memberId: "m-1", name: "Pak Hendra Gunawan" },
    allocations: [],
    corrections: [],
    createdAt: "2026-11-03T03:00:00Z",
    ...overrides,
  } as AccountPayment;
}

describe("paymentFormSchema", () => {
  it.each<{ name: string; input: Partial<PaymentFormValues>; want: string }>([
    {
      name: "an empty amount",
      input: { amount: "" },
      want: EMPTY_AMOUNT_MESSAGE,
    },
    {
      name: "only spaces",
      input: { amount: "  " },
      want: EMPTY_AMOUNT_MESSAGE,
    },
    {
      name: "letters",
      input: { amount: "1.5jt" },
      want: AMOUNT_DIGITS_MESSAGE,
    },
    { name: "zero", input: { amount: "0" }, want: AMOUNT_ZERO_MESSAGE },
    {
      name: "more than the API's maximum",
      input: { amount: "1.000.000.000.001" },
      want: AMOUNT_TOO_LARGE_MESSAGE,
    },
    {
      name: "a future date",
      input: { receivedOn: "2026-11-04" },
      want: RECEIVED_ON_FUTURE_MESSAGE,
    },
    {
      name: "an empty date",
      input: { receivedOn: "" },
      want: EMPTY_RECEIVED_ON_MESSAGE,
    },
    {
      name: "a note too long",
      input: { note: "a".repeat(501) },
      want: NOTE_TOO_LONG_MESSAGE,
    },
  ])("refuses $name", ({ input, want }) => {
    expect(messages(input)).toEqual([want]);
  });

  it("accepts today, as an integer amount", () => {
    expect(check({}).data).toEqual({
      amount: 700_000,
      method: "cash",
      receivedOn: TODAY,
      note: "",
    });
  });

  it("accepts a past date, transfer, the maximum and a trimmed note", () => {
    expect(
      check({
        amount: "1.000.000.000.000",
        method: "transfer",
        receivedOn: "2026-10-28",
        note: "  BCA atas nama suami  ",
      }).data,
    ).toEqual({
      amount: 1_000_000_000_000,
      method: "transfer",
      receivedOn: "2026-10-28",
      note: "BCA atas nama suami",
    });
  });

  it("accepts a note of exactly 500 characters", () => {
    expect(check({ note: "a".repeat(500) }).success).toBe(true);
  });

  it("starts with cash, today and nothing typed", () => {
    expect(paymentFormDefaults(TODAY)).toEqual({
      amount: "",
      method: "cash",
      receivedOn: TODAY,
      note: "",
    });
  });
});

describe("paymentRequestBody", () => {
  it("sends the integer amount, method, day, requestId and tenant", () => {
    expect(
      paymentRequestBody(
        {
          amount: 1_250_000,
          method: "transfer",
          receivedOn: "2026-10-28",
          note: "BCA",
        },
        "req-1",
        "tenant-1",
      ),
    ).toEqual({
      requestId: "req-1",
      tenantId: "tenant-1",
      amount: 1_250_000,
      method: "transfer",
      receivedOn: "2026-10-28",
      note: "BCA",
    });
  });

  it("leaves out an empty note", () => {
    const body = paymentRequestBody(
      { amount: 500_000, method: "cash", receivedOn: TODAY, note: "" },
      "req-1",
      "tenant-1",
    );
    expect(body).not.toHaveProperty("note");
    expect(Number.isInteger(body.amount)).toBe(true);
  });
});

describe("unpaidSummary", () => {
  it("lists the unpaid invoices oldest first with what each still owes", () => {
    expect(
      unpaidSummary({
        outstanding: 1_709_000,
        invoices: [
          invoice({
            id: "a",
            period: "2026-08-01",
            status: "paid",
            outstanding: 0,
          }),
          invoice({
            id: "b",
            period: "2026-09-01",
            status: "partial",
            outstanding: 500_000,
          }),
          invoice({ id: "c", period: "2026-10-01", outstanding: 1_209_000 }),
          invoice({
            id: "d",
            period: "2026-07-01",
            status: "cancelled",
            outstanding: 0,
          }),
        ],
      }),
    ).toEqual({
      total: "Rp1.709.000",
      detail:
        "2 tagihan belum lunas: September 2026 sisa Rp500.000, Oktober 2026 sisa Rp1.209.000",
    });
  });

  it("says nothing is owed", () => {
    expect(unpaidSummary({ outstanding: 0, invoices: [] })).toEqual({
      total: "Rp0",
      detail: NOTHING_OWED_TEXT,
    });
  });
});

describe("paymentHeadline", () => {
  it("names the amount, the method, the day and who recorded it", () => {
    expect(paymentHeadline(payment({}))).toBe(
      "Rp700.000 · Tunai · diterima 3 Nov 2026 · dicatat Pak Hendra Gunawan",
    );
  });

  it("says Transfer bank, and leaves out who when the API names nobody", () => {
    expect(
      paymentHeadline(
        payment({
          method: "transfer",
          receivedOn: "2026-10-28",
          recordedBy: undefined,
        }),
      ),
    ).toBe("Rp700.000 · Transfer bank · diterima 28 Okt 2026");
  });
});

const RESULT: PaymentResult = {
  outstanding: 1_209_000,
  overdue: 509_000,
  credit: 0,
  payment: payment({}),
  paid: [
    {
      invoiceId: "a",
      kind: "regular",
      period: "2026-09-01",
      periodEnd: "2026-09-30",
      dueDate: "2026-09-01",
      amount: 500_000,
      outstanding: 0,
    },
    {
      invoiceId: "b",
      kind: "regular",
      period: "2026-10-01",
      periodEnd: "2026-10-31",
      dueDate: "2026-10-01",
      amount: 200_000,
      outstanding: 509_000,
    },
  ],
};

describe("paidRows", () => {
  it("keeps the API's order, with Lunas or what is left", () => {
    expect(paidRows(RESULT)).toEqual([
      {
        invoiceId: "a",
        text: "Tagihan September 2026 · Rp500.000",
        chip: { label: "Lunas", settled: true },
      },
      {
        invoiceId: "b",
        text: "Tagihan Oktober 2026 · Rp200.000",
        chip: { label: "Sisa Rp509.000", settled: false },
      },
    ]);
  });

  it("names the final invoice Tagihan akhir", () => {
    expect(
      paidRows({
        paid: [
          {
            invoiceId: "f",
            kind: "final",
            period: "2026-10-31",
            periodEnd: "2026-10-31",
            dueDate: "2026-10-31",
            amount: 180_000,
            outstanding: 0,
          },
        ],
      })[0]?.text,
    ).toBe("Tagihan akhir Oktober 2026 · Rp180.000");
  });

  it("is empty when it all went to credit", () => {
    expect(paidRows({ paid: [] })).toEqual([]);
    expect(ALL_CREDIT_TEXT).toBe("Semua masuk ke saldo lebih bayar.");
  });
});

describe("resultBoxRows", () => {
  it("shows the debt and the whole total when part is not due yet", () => {
    expect(resultBoxRows(RESULT)).toEqual([
      { label: "Sisa tunggakan", value: "Rp509.000", note: null },
      { label: "Total belum dibayar", value: "Rp1.209.000", note: null },
    ]);
  });

  it("shows the credit first and no second total when nothing else is owed", () => {
    expect(
      resultBoxRows({ credit: 300_000, overdue: 0, outstanding: 0 }),
    ).toEqual([
      { label: "Saldo lebih bayar", value: "Rp300.000", note: CREDIT_NOTE },
      { label: "Sisa tunggakan", value: "Rp0", note: null },
    ]);
  });

  it("is gathered into one view", () => {
    expect(paymentResultView(RESULT)).toEqual({
      headline: paymentHeadline(RESULT.payment),
      paid: paidRows(RESULT),
      box: resultBoxRows(RESULT),
    });
  });
});

describe("isInvoicesQueryKey", () => {
  it("matches the organization's invoice list, any page or filter", () => {
    expect(isInvoicesQueryKey(getListInvoicesQueryKey(ORG), ORG)).toBe(true);
    expect(
      isInvoicesQueryKey(getListInvoicesQueryKey(ORG, { page: 2 }), ORG),
    ).toBe(true);
    expect(
      isInvoicesQueryKey(["org", ORG, `/orgs/${ORG}/invoices/x`, {}], ORG),
    ).toBe(true);
  });

  it("leaves other queries and organizations alone", () => {
    expect(isInvoicesQueryKey(getListTenantsQueryKey(ORG), ORG)).toBe(false);
    expect(isInvoicesQueryKey(getListInvoicesQueryKey("other"), ORG)).toBe(
      false,
    );
    expect(
      isInvoicesQueryKey(["org", ORG, `/orgs/${ORG}/invoicesx`, {}], ORG),
    ).toBe(false);
    expect(isInvoicesQueryKey(["user", "/me", {}], ORG)).toBe(false);
  });
});

describe("canRecordPayment", () => {
  it.each<{ name: string; access: Capability["access"] | null; want: boolean }>(
    [
      { name: "allowed", access: "allow", want: true },
      { name: "conditional", access: "conditional", want: true },
      { name: "denied", access: "deny", want: false },
      { name: "not listed", access: null, want: false },
    ],
  )("$name", ({ access, want }) => {
    const capabilities: Capability[] =
      access === null ? [] : [{ action: "payment:record", access }];
    expect(canRecordPayment(createAppAbility(capabilities))).toBe(want);
  });
});

describe("paymentRecordedToast", () => {
  it("names the amount recorded and the tenant", () => {
    expect(paymentRecordedToast(1250000, "Yulia Kartika")).toEqual({
      tone: "create",
      title: "Pembayaran dicatat",
      description: "Rp1.250.000 dari Yulia Kartika",
    });
  });
});
