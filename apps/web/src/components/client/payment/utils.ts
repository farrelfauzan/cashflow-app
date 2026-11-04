import type { QueryKey } from "@tanstack/react-query";
import { z } from "zod";

import { invoiceName, invoiceTitle } from "@/components/client/billing/utils";
import { isCalendarDate } from "@/components/client/lease/utils";
import type {
  AccountPayment,
  PaymentInput,
  PaymentInputMethod,
  PaymentResult,
  TenantAccount,
} from "@/lib/api/generated/model";
import { formatDate } from "@/lib/format/date";
import { formatRupiah, parseRupiahInput } from "@/lib/format/rupiah";
import type { AppAbility } from "@/lib/capabilities";

import type {
  PaidRowView,
  PaymentFormOutput,
  PaymentResultView,
  ResultBoxRow,
  UnpaidSummaryView,
} from "./type";

// "Catat pembayaran" and "Pembayaran tercatat". Every amount shown is a field the API sent, only
// formatted: nothing here adds, subtracts or derives rupiah. FIFO, credit and what is left are
// the API's.

// The API's limits (PaymentInput).
export const PAYMENT_AMOUNT_MAX = 1_000_000_000_000;
export const PAYMENT_NOTE_MAX_LENGTH = 500;

export const EMPTY_AMOUNT_MESSAGE = "Isi jumlah yang diterima.";
export const AMOUNT_DIGITS_MESSAGE = "Hanya angka.";
export const AMOUNT_ZERO_MESSAGE = "Jumlah harus lebih dari Rp0.";
export const AMOUNT_TOO_LARGE_MESSAGE = `Jumlah paling banyak ${formatRupiah(PAYMENT_AMOUNT_MAX)}.`;
export const EMPTY_RECEIVED_ON_MESSAGE = "Isi tanggal diterima.";
export const RECEIVED_ON_FUTURE_MESSAGE =
  "Tanggal tidak boleh setelah hari ini.";
export const NOTE_TOO_LONG_MESSAGE = `Catatan paling banyak ${PAYMENT_NOTE_MAX_LENGTH} karakter.`;

export const AMOUNT_HINT =
  "Hanya angka. Boleh kurang atau lebih dari tunggakan.";
export const RECEIVED_ON_HINT =
  "Hari ini. Boleh tanggal yang sudah lewat, tidak boleh tanggal yang akan datang.";
export const NOTE_PLACEHOLDER =
  "Contoh: dititip ke penjaga kos, atau transfer dari rekening BCA suami";
export const FIFO_NOTE =
  "Pembayaran dipakai untuk tagihan paling lama dulu. Kelebihannya jadi saldo dan otomatis dipakai untuk tagihan berikutnya.";

export const SUMMARY_TITLE = "Total belum dibayar";
export const NOTHING_OWED_TEXT = "Tidak ada tagihan yang belum lunas.";
export const SUMMARY_LOADING_TEXT = "Memuat tagihan…";

export const RESULT_TITLE = "Pembayaran tercatat";
export const PAID_TITLE = "Dipakai untuk (tagihan paling lama dulu)";
export const PAID_LIST_LABEL = "Tagihan yang dibayar";
export const ALL_CREDIT_TEXT = "Semua masuk ke saldo lebih bayar.";
export const CREDIT_NOTE = "Otomatis dipakai untuk tagihan berikutnya.";
export const CORRECTION_HINT =
  "Salah catat? Betulkan dengan Koreksi nominal pada pembayaran ini. Catatan lama tidak dihapus.";

// The choices of "Cara bayar", cash first (the default).
export const PAYMENT_METHODS: readonly PaymentInputMethod[] = [
  "cash",
  "transfer",
];

export const PAYMENT_METHOD_TEXT: Record<PaymentInputMethod, string> = {
  cash: "Tunai",
  transfer: "Transfer bank",
};

// The labels of AccountPayment's methods in the result; a recorded payment is cash or transfer,
// the others are named for completeness.
const RESULT_METHOD_TEXT: Record<AccountPayment["method"], string> = {
  ...PAYMENT_METHOD_TEXT,
  qris: "QRIS",
  va: "Virtual account",
};

// The fields an API error can name; any other (requestId, tenantId) goes to the alert.
export const PAYMENT_API_FIELDS = [
  "amount",
  "method",
  "receivedOn",
  "note",
] as const;

export function isPaymentMethod(value: string): value is PaymentInputMethod {
  return PAYMENT_METHODS.some((method) => method === value);
}

// The form's starting values: no amount, cash, received today (Jakarta), no note.
export function paymentFormDefaults(today: string) {
  return { amount: "", method: "cash" as const, receivedOn: today, note: "" };
}

// The friendly early checks of "Catat pembayaran". The Go API checks everything again and its
// message wins. `today` is the Jakarta day; calendar dates compare as strings.
export function paymentFormSchema(today: string) {
  return z.object({
    amount: z.string().transform((text, ctx) => {
      if (text.trim() === "") {
        ctx.addIssue({ code: "custom", message: EMPTY_AMOUNT_MESSAGE });
        return z.NEVER;
      }
      const amount = parseRupiahInput(text);
      if (amount === null) {
        ctx.addIssue({ code: "custom", message: AMOUNT_DIGITS_MESSAGE });
        return z.NEVER;
      }
      if (amount <= 0) {
        ctx.addIssue({ code: "custom", message: AMOUNT_ZERO_MESSAGE });
        return z.NEVER;
      }
      if (amount > PAYMENT_AMOUNT_MAX) {
        ctx.addIssue({ code: "custom", message: AMOUNT_TOO_LARGE_MESSAGE });
        return z.NEVER;
      }
      return amount;
    }),
    method: z.enum(["cash", "transfer"]),
    receivedOn: z.string().transform((date, ctx) => {
      if (!isCalendarDate(date)) {
        ctx.addIssue({ code: "custom", message: EMPTY_RECEIVED_ON_MESSAGE });
        return z.NEVER;
      }
      if (date > today) {
        ctx.addIssue({ code: "custom", message: RECEIVED_ON_FUTURE_MESSAGE });
        return z.NEVER;
      }
      return date;
    }),
    note: z.string().trim().max(PAYMENT_NOTE_MAX_LENGTH, NOTE_TOO_LONG_MESSAGE),
  });
}

// The request body: the integer amount, the method, the day, and the trimmed note only when
// there is one. One requestId per opening of the form, sent again unchanged on a retry.
export function paymentRequestBody(
  fields: PaymentFormOutput,
  requestId: string,
  tenantId: string,
): PaymentInput {
  const body: PaymentInput = {
    requestId,
    tenantId,
    amount: fields.amount,
    method: fields.method,
    receivedOn: fields.receivedOn,
  };
  if (fields.note !== "") body.note = fields.note;
  return body;
}

// The summary box: what the tenant owes in total, and the unpaid invoices oldest first (the
// API's order) with what each still owes.
export function unpaidSummary(
  account: Pick<TenantAccount, "outstanding" | "invoices">,
): UnpaidSummaryView {
  const unpaid = account.invoices.filter(
    (invoice) =>
      (invoice.status === "open" || invoice.status === "partial") &&
      invoice.outstanding > 0,
  );
  const total = formatRupiah(account.outstanding);
  if (unpaid.length === 0) return { total, detail: NOTHING_OWED_TEXT };
  const list = unpaid
    .map(
      (invoice) =>
        `${invoiceTitle(invoice)} sisa ${formatRupiah(invoice.outstanding)}`,
    )
    .join(", ");
  return { total, detail: `${unpaid.length} tagihan belum lunas: ${list}` };
}

// "Rp700.000 · Tunai · diterima 3 Nov 2026 · dicatat Pak Hendra Gunawan"; without the last part
// when the API names nobody.
export function paymentHeadline(payment: AccountPayment): string {
  const parts = [
    formatRupiah(payment.amount),
    RESULT_METHOD_TEXT[payment.method],
    `diterima ${formatDate(payment.receivedOn)}`,
  ];
  if (payment.recordedBy !== undefined) {
    parts.push(`dicatat ${payment.recordedBy.name}`);
  }
  return parts.join(" · ");
}

export function paidRows(result: Pick<PaymentResult, "paid">): PaidRowView[] {
  return result.paid.map((invoice) => ({
    invoiceId: invoice.invoiceId,
    text: `${invoiceName(invoice)} · ${formatRupiah(invoice.amount)}`,
    chip:
      invoice.outstanding === 0
        ? { label: "Lunas", settled: true }
        : {
            label: `Sisa ${formatRupiah(invoice.outstanding)}`,
            settled: false,
          },
  }));
}

// The totals after the payment: the credit (only when there is some), the debt, and the whole
// unpaid total only when part of it is not due yet.
export function resultBoxRows(
  totals: Pick<PaymentResult, "credit" | "overdue" | "outstanding">,
): ResultBoxRow[] {
  const rows: ResultBoxRow[] = [];
  if (totals.credit > 0) {
    rows.push({
      label: "Saldo lebih bayar",
      value: formatRupiah(totals.credit),
      note: CREDIT_NOTE,
    });
  }
  rows.push({
    label: "Sisa tunggakan",
    value: formatRupiah(totals.overdue),
    note: null,
  });
  if (totals.outstanding !== totals.overdue) {
    rows.push({
      label: "Total belum dibayar",
      value: formatRupiah(totals.outstanding),
      note: null,
    });
  }
  return rows;
}

export function paymentResultView(result: PaymentResult): PaymentResultView {
  return {
    headline: paymentHeadline(result.payment),
    paid: paidRows(result),
    box: resultBoxRows(result),
  };
}

// Every query of the organization's invoices: the list (any page or filter) and anything under
// it. Keys are ["org", orgId, url, params] (src/lib/api/query-key.ts).
export function isInvoicesQueryKey(queryKey: QueryKey, orgId: string): boolean {
  const [scope, keyOrg, url] = queryKey;
  if (scope !== "org" || keyOrg !== orgId || typeof url !== "string") {
    return false;
  }
  const base = `/orgs/${orgId}/invoices`;
  return (
    url === base || url.startsWith(`${base}/`) || url.startsWith(`${base}?`)
  );
}

// "Catat pembayaran" shows to a member who may record payments on some tenants or all
// ("conditional" shows it and the API decides per tenant). Never by role name.
export function canRecordPayment(ability: AppAbility): boolean {
  return ability.can("record", "payment");
}
