import type { z } from "zod";

import type { paymentFormSchema } from "./utils";

// "Catat pembayaran" (board "Catat pembayaran") as react-hook-form holds it, under the API's
// field names.
export type PaymentFormValues = z.input<ReturnType<typeof paymentFormSchema>>;

// What the schema hands on: the checked fields, ready for the request body.
export type PaymentFormOutput = z.output<ReturnType<typeof paymentFormSchema>>;

// The summary at the top of the form, from the tenant's account: "Total belum dibayar" and the
// unpaid invoices, oldest first ("2 tagihan belum lunas: September 2026 sisa Rp500.000, ...").
export type UnpaidSummaryView = {
  total: string;
  // The invoices line, or "Tidak ada tagihan yang belum lunas." when nothing is owed.
  detail: string;
};

// One invoice the payment paid, in the API's order (oldest period first).
export type PaidRowView = {
  invoiceId: string;
  // "Tagihan Oktober 2026 · Rp700.000".
  text: string;
  // "Lunas" when the invoice owes nothing now, else "Sisa Rp509.000".
  chip: { label: string; settled: boolean };
};

// One line of the result's totals box.
export type ResultBoxRow = {
  label: string;
  value: string;
  // A note under the value ("Otomatis dipakai untuk tagihan berikutnya."), null for none.
  note: string | null;
};

// "Pembayaran tercatat" (board "Pembayaran tercatat"): every value ready to show, every figure
// a field of the API's PaymentResult.
export type PaymentResultView = {
  // "Rp700.000 · Tunai · diterima 3 Nov 2026 · dicatat Pak Hendra Gunawan".
  headline: string;
  paid: readonly PaidRowView[];
  box: readonly ResultBoxRow[];
};
