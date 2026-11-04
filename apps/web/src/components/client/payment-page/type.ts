// What the tenant payment page (/t/[token]) renders, built from getPaymentPage by utils.ts.
// Every amount is a field the API sent, formatted; nothing here is computed rupiah.

// The top of every loaded state: who bills the tenant, the link's invoice and whose it is.
export type PaymentPageHeaderView = {
  organizationName: string;
  // "Bayar tagihan November 2026"
  title: string;
  // "Bu Rina · Kamar 07 Kos Melati"
  subtitle: string;
};

// One open invoice in the summary card, oldest first.
export type SummaryRowView = {
  id: string;
  // "Tagihan November 2026", or "Sisa Oktober 2026" when part of it is paid.
  label: string;
  amount: string;
  // Its period is before the link invoice's: the "dari bulan lalu" chip.
  fromEarlierMonth: boolean;
};

// The organization's bank account for a manual transfer.
export type TransferView = {
  bankName: string;
  accountNumber: string;
  // "a.n. Ani Rahmawati"
  holderLine: string;
  // "Tulis nama Anda di berita transfer: Bu Rina"
  noteLine: string;
};

// The tenant still owes something.
export type OpenPaymentPageView = {
  status: "open";
  header: PaymentPageHeaderView;
  // "Tagihan November 2026 sudah lunas." when the link's invoice is paid but older ones are not.
  linkInvoicePaidNote: string | null;
  rows: SummaryRowView[];
  total: string;
  // Shown only when the tenant has credit.
  credit: string | null;
  // Null when the organization has set no bank account.
  transfer: TransferView | null;
  // "Cara bayar belum diatur. Hubungi … untuk membayar."
  noTransferText: string;
};

// The tenant owes nothing.
export type PaidPaymentPageView = {
  status: "paid";
  header: PaymentPageHeaderView;
  invoiceName: string;
  amount: string;
  credit: string | null;
};

export type PaymentPageView = OpenPaymentPageView | PaidPaymentPageView;

// A request that did not give the page: the link does not work (unknown, revoked, expired or
// malformed, all shown the same), too many requests, or anything else (network, 5xx).
export type PaymentPageErrorKind = "not-found" | "rate-limited" | "failed";

export type PaymentPageState =
  | { kind: "loading" }
  | { kind: "error"; error: PaymentPageErrorKind }
  | { kind: "ready"; view: PaymentPageView };

// The copy of one error state.
export type PaymentPageMessageView = {
  title: string;
  text: string;
  // Shows "Coba lagi" (a refetch) when true.
  retry: boolean;
};

export type CopyStatus = "idle" | "copied" | "failed";
