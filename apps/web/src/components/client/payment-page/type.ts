// What the tenant payment page (/t/[token]) renders, built from getPaymentPage by utils.ts.
// Every amount is a field the API sent, formatted; nothing here is computed rupiah.

import type {
  MakePaymentPromiseRequest,
  PaymentPromise,
  PromiseKind,
  QrisStatus,
} from "@/lib/api/generated/model";

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
  // Null when the organization has not set up QRIS: no tabs, bank transfer only.
  qris: QrisOfferView | null;
  // "Terima kasih, Bu Rina." on Janji tercatat.
  tenantName: string;
  // The API's outstanding, compared only ("Sisa tagihan" on Janji tercatat, when above 0).
  outstanding: number;
  // Null when no promise can be made (the API sent no promiseDates).
  promise: PromiseOfferView | null;
  // The tenant's active promise as the compact card above the choice; null when there is none.
  activePromise: ActivePromiseView | null;
};

// What a promise made now may name, from getPaymentPage as is.
export type PromiseOfferView = {
  // What the tenant owes now: the promise's rest when nothing is paid now.
  outstanding: number;
  // The days "Pilih tanggal sendiri" may take, both included (YYYY-MM-DD).
  earliest: string;
  latest: string;
  // The due date "Bareng tagihan …" takes; null when that choice is not offered.
  nextInvoiceDueDate: string | null;
};

// "Janji pelunasan 1 Des 2026 · Semua sisa tagihan" above the choice.
export type ActivePromiseView = {
  text: string;
  // The promise as the API sent it: "Ubah janji" opens the Janji screen filled from it.
  promise: PaymentPromise;
};

// What the QRIS tab starts from when the organization has set up QRIS.
export type QrisOfferView = {
  // The API's outstanding, sent as is as the QR's amount (never computed here).
  outstanding: number;
  // "Bayar QRIS Rp2.250.000"; "Bayar QRIS Rp10.000.000" when more is owed (one QR's most).
  startLabel: string;
  // Shown when more than Rp10.000.000 is owed: one QR pays at most that, the rest later.
  limitNote: string | null;
  // "qris-kamar-07.png": the name "Unduh QR" saves under; never holds the token.
  downloadName: string;
  // "Total tagihan Rp2.250.000", under the "Bayar sebagian" title.
  totalLine: string;
  // The amounts "Bayar sebagian" offers to fill in with one tap.
  quickAmounts: QuickAmountView[];
};

// One quick-amount button of "Bayar sebagian".
export type QuickAmountView = {
  // Integer rupiah the button fills in: an API balance, or a fixed round amount.
  amount: number;
  // "Sisa Oktober 2026 · Rp750.000", "Rp500.000".
  label: string;
};

// The three answers of "Mau bayar berapa?" (board "Bayar-Pilih").
export type PayChoice = "all" | "partial" | "later";

// One row of a radio group: its value, its bold label and the line under it.
export type ChoiceOptionView = {
  value: string;
  label: string;
  sub: string;
};

// What the Janji screen's fields hold. The amount stays as the field's text (digits grouped with
// dots, empty for "all the rest").
export type PromiseDraft = {
  kind: PromiseKind;
  // YYYY-MM-DD from the date input, empty for none; sent only for specific_date.
  date: string;
  amountText: string;
};

// What the tenant pays right after saving the promise ("Simpan janji, lalu bayar Rp…"): the
// amount of "Bayar sebagian", its field text, and what its preview said would be left.
export type PromisePaying = {
  amount: number;
  text: string;
  outstandingAfter: number;
};

// Which view the open page shows. Every view lives in one flow component, so the QRIS state
// survives moving between them.
export type PayFlowMode =
  // "Mau bayar berapa?": the summary and the choices, with the one the tenant last picked.
  | { kind: "choice"; selected: PayChoice }
  // Everything owed: the QRIS | Transfer bank tabs, or bank transfer alone.
  | { kind: "pay" }
  // "Bayar sebagian": the amount field, quick amounts and the preview.
  | { kind: "entry"; text: string }
  // "Kapan sisanya dilunasi?". `paying` is null when nothing is paid now; `editing` is "Ubah
  // janji"; `back` is the view "Kembali" returns to.
  | {
      kind: "promise";
      draft: PromiseDraft;
      paying: PromisePaying | null;
      editing: boolean;
      back: PayFlowMode;
    }
  // A chosen amount, paid by QRIS. `outstandingAfter` is the preview's, kept when it was chosen;
  // `promise` is the one saved just before, when the rest was promised.
  | {
      kind: "chosen";
      text: string;
      amount: number;
      outstandingAfter: number;
      promise: PaymentPromise | null;
    }
  // "Janji tercatat": the promise just saved, and what this visit's QR paid (null for nothing).
  | { kind: "recorded"; paidAmount: number | null; promise: PaymentPromise };

export type ChosenFlowMode = Extract<PayFlowMode, { kind: "chosen" }>;
export type PromiseFlowMode = Extract<PayFlowMode, { kind: "promise" }>;
export type RecordedFlowMode = Extract<PayFlowMode, { kind: "recorded" }>;

// The Janji screen, from the page and what is paid now.
export type PromiseFormView = {
  // "Janji sebelumnya 1 Des 2026, semua sisa tagihan akan diganti." when saving replaces an active
  // promise from a new form; null otherwise (no active promise, or "Ubah janji").
  replacesLine: string | null;
  // "Sisa setelah bayar hari ini: Rp1.250.000", or "Total tagihan Rp2.250.000".
  subline: string;
  // "Bareng tagihan Desember" (when offered) and "Pilih tanggal sendiri".
  kindOptions: ChoiceOptionView[];
  // The date input's bounds (YYYY-MM-DD).
  dateMin: string;
  dateMax: string;
  // "Kosongkan untuk semua sisa, Rp1.250.000.".
  amountHint: string;
  // "Simpan janji, lalu bayar Rp1.000.000", "Simpan perubahan" or "Simpan janji".
  primaryLabel: string;
};

// Where a message about the promise goes: under one of the fields, or in the alert.
export type PromiseErrorTarget =
  "kind" | "promisedDate" | "promisedAmount" | "alert";

export type PromiseFormError = {
  target: PromiseErrorTarget;
  message: string;
};

// What a refused makePaymentPromise means on screen.
export type PromiseErrorView =
  // The link stopped working: the page shows "Link tidak berlaku".
  | { kind: "link-gone" }
  // `refetchPage`: what the page shows is out of date (nothing owed now, dates moved).
  | { kind: "shown"; error: PromiseFormError; refetchPage: boolean };

// The end of one "Simpan janji".
export type PromiseSaveResult =
  | { kind: "saved"; promise: PaymentPromise }
  // `cause` is the request's own error, for the form to place.
  | { kind: "failed"; error: PromiseErrorView; cause: unknown }
  // Another save of this form is still running; nothing was sent.
  | { kind: "busy" };

// "Janji tercatat" (board "Janji tercatat").
export type RecordedView = {
  // "Pembayaran Rp1.000.000 sudah diterima. Terima kasih, Bu Rina."; null when nothing was paid.
  paidLine: string | null;
  // "Sisa tagihan Rp1.250.000"; null when nothing is owed.
  outstandingLine: string | null;
  // "Selasa, 1 Desember 2026".
  date: string;
  // "Rp500.000", or "Semua sisa tagihan".
  amount: string;
};

// The chosen-amount card above the tabs: the amount formatted, and what stays owed (null when
// it pays everything or when the QR's amount is not the chosen one).
export type ChosenAmountView = {
  amount: string;
  remainderLine: string | null;
};

// What the amount field of "Bayar sebagian" holds, checked by comparison only.
export type PartialAmountCheck =
  // Nothing (or 0) typed: no preview, no message, the primary waits.
  | { kind: "empty" }
  // Typed but not payable by QRIS: the message under the field.
  | { kind: "invalid"; message: string }
  | { kind: "valid"; amount: number };

// What a failed preview read means on screen.
export type PreviewErrorView =
  // The link stopped working: the page shows "Link tidak berlaku".
  | { kind: "link-gone" }
  // The API refused the amount (above what is owed): its message under the field.
  | { kind: "field"; message: string }
  // Rate limit, network, anything else: an alert.
  | { kind: "alert"; message: string };

// One invoice the previewed amount reaches, as the API answered it.
export type PreviewRowView = {
  id: string;
  // "Oktober 2026", or "Tagihan akhir Oktober 2026".
  name: string;
  // "Lunas", or "sisa Rp1.250.000".
  chip: string;
  paidInFull: boolean;
  // What the amount pays on it: "Rp750.000".
  applied: string;
};

// The "Uang ini dipakai untuk" card.
export type PreviewView = {
  // The amount the API previewed; the primary waits until it is the field's amount.
  amount: number;
  rows: PreviewRowView[];
  // "Sisa tagihan setelah ini Rp1.250.000"; null when the amount pays everything.
  outstandingAfterLine: string | null;
  outstandingAfter: number;
};

// The link's invoice under "Rincian" on Lunas.
export type PaidInvoiceView = {
  // "Tagihan November 2026"
  name: string;
  amount: string;
};

// The tenant owes nothing.
export type PaidPaymentPageView = {
  status: "paid";
  header: PaymentPageHeaderView;
  // Null when the link's invoice bills nothing now (cancelled by a correction), so Lunas never
  // lists it as paid Rp0.
  invoice: PaidInvoiceView | null;
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

// The two ways to pay when QRIS is set up.
export type PaymentTab = "qris" | "transfer";

// The QRIS card. Amounts are the API's QrisStatus.amount (null when the API left it out);
// times are its RFC 3339 strings.
export type QrisCard =
  // Reading getPaymentStatus once on load, to resume a pending or checking QR.
  | { kind: "resuming" }
  // No QR in this visit: "Bayar QRIS". `paid`: this visit's QR was just paid (the page is read
  // again), as opposed to a QR that is gone or was never made.
  | { kind: "idle"; paid?: true }
  // createPaymentQris is running (up to about 20 seconds).
  // retried: the API was overloaded once and this is the one automatic retry.
  | { kind: "creating"; retried?: true }
  // 409 qris_busy: another request is making the tenant's QR. Polls as if pending until that QR
  // shows, and gives up at `until` (epoch ms).
  | { kind: "waiting"; until: number }
  // A live QR (board "Bayar QRIS").
  | { kind: "pending"; amount: number | null; expiresAt: string }
  // The tenant said they paid; no answer yet (board "Sedang dicek").
  | { kind: "checking"; amount: number | null; checkedAt: string | null }
  // The QR of this visit expired or was cancelled (board "QR kedaluwarsa").
  | { kind: "expired"; amount: number | null }
  // 503 qris_unavailable (board "QRIS gangguan"): the Transfer bank tab with a banner.
  | { kind: "unavailable" };

export type QrisModel = {
  card: QrisCard;
  tab: PaymentTab;
  // The API's message (or the page's own) under the QRIS buttons; null when there is none.
  alert: string | null;
  // "Saya sudah bayar" is running: status reads that land meanwhile are ignored, so an older
  // "pending" never overwrites the check's answer.
  checkRunning: boolean;
  // Counts each start and end of "Saya sudah bayar". A status read that began under another
  // count is stale (it may say "pending" after the check said "checking") and is dropped.
  checks: number;
};

// What happens on the QRIS card, with the time of the event (epoch ms) where it matters.
export type QrisEvent =
  // `checks`: the model's count when the read began.
  | { type: "status-read"; status: QrisStatus; now: number; checks: number }
  | { type: "status-failed"; error: unknown }
  | { type: "create-started" }
  // Nothing is owed after all, so no QR is asked for.
  | { type: "create-skipped" }
  | { type: "created"; status: QrisStatus; now: number }
  | { type: "create-failed"; error: unknown; now: number }
  | { type: "check-started" }
  | { type: "checked"; status: QrisStatus; now: number }
  | { type: "check-failed"; error: unknown }
  | { type: "tab-selected"; tab: PaymentTab };

// The next model, and what the hook must do besides showing it.
export type QrisStep = {
  model: QrisModel;
  // Read getPaymentPage again: it shows Lunas, or what is left.
  refetchPage: boolean;
  // The link stopped working: the page shows "Link tidak berlaku".
  linkGone: boolean;
  // Read getPaymentStatus once now, outside the polling interval.
  pollNow: boolean;
  // Send the same createPaymentQris again after QRIS_OVERLOADED_RETRY_MS: the API was too busy
  // to reach AutoGoPay (qris_overloaded), so nothing was made and one retry is safe.
  retryCreate: boolean;
};

// One row of the "Sedang dicek" details card.
export type QrisDetailRow = {
  label: string;
  value: string;
};

// What useQrisPayment gives the page.
export type QrisPayment = {
  model: QrisModel;
  // "Bayar QRIS", or "Buat QR baru" (`afterExpiry` reads the page again first, for the current
  // total). `chosen` is the amount of "Bayar sebagian", or null for the total.
  start: (afterExpiry: boolean, chosen: number | null) => void;
  // "Saya sudah bayar".
  check: () => void;
  // Reads the status once now (the countdown reached zero, the image did not load).
  pollNow: () => void;
  selectTab: (tab: PaymentTab) => void;
};

// What useCountdown gives a live QR.
export type Countdown = {
  // The device's clock at the last tick (epoch ms).
  now: number;
  // Whole seconds left, never below 0.
  seconds: number;
};

// What useMakePromise gives the Janji screen.
export type MakePromise = {
  // Sends one makePaymentPromise; a second call while one runs sends nothing ("busy").
  save: (body: MakePaymentPromiseRequest) => Promise<PromiseSaveResult>;
  // A save is running: the primary is disabled.
  pending: boolean;
};

// One app's note in the "Cara bayar" guide: where that app keeps paying from a saved QR image.
export type QrisGuideApp = {
  name: string;
  steps: string;
};
