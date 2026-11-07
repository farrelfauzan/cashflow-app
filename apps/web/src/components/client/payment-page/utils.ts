import { invoiceName, invoiceTitle } from "@/components/client/billing/utils";
import { API_BASE_PATH } from "@/lib/api/client";
import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  INVALID_PATH_CODE,
} from "@/lib/api/errors";
import type {
  MakePaymentPromiseRequest,
  PaymentPage,
  PaymentPageInvoice,
  PaymentPreview,
  PaymentPromise,
  QrisStatus,
} from "@/lib/api/generated/model";
import { PromiseKind } from "@/lib/api/generated/model";
import type { FormError } from "@/lib/api/form-error";
import {
  formatDate,
  formatDateTime,
  formatLongDate,
  formatMonthYear,
} from "@/lib/format/date";
import { formatRupiah, parseRupiahInput } from "@/lib/format/rupiah";
import { groupRupiahDigits } from "@/lib/format/rupiah-input";
import { z } from "zod";

import type {
  ActivePromiseView,
  ChoiceOptionView,
  TelegramErrorView,
  TelegramOfferView,
  ChosenAmountView,
  ChosenFlowMode,
  CopyStatus,
  OpenPaymentPageView,
  PayChoice,
  PayFlowMode,
  PromiseDraft,
  PromiseDraftCheck,
  PromiseDraftField,
  PromiseErrorTarget,
  PromiseErrorView,
  PromiseFormView,
  PromiseOfferView,
  PromisePaying,
  RecordedView,
  RecordedFlowMode,
  PaidPaymentPageView,
  PaymentPageErrorKind,
  PaymentPageHeaderView,
  PaymentPageMessageView,
  PaymentPageState,
  PaymentPageView,
  PartialAmountCheck,
  PaymentTab,
  PreviewErrorView,
  PreviewView,
  QrisCard,
  QrisDetailRow,
  QrisEvent,
  QrisGuideApp,
  QrisModel,
  QrisOfferView,
  QrisStep,
  QuickAmountView,
  SummaryRowView,
  TransferView,
} from "./type";

// The tenant payment page, /t/[token] (boards "Bayar · Pilih", "Bayar · Transfer", "Bayar ·
// Lunas"). The token in the path is the tenant's only access and a secret: it goes to one
// request, getPaymentPage, and nowhere else (no log, no other URL, no title). Every amount shown
// is a field the API sent, only formatted: nothing here adds or subtracts rupiah.

// PaymentPage's token parameter in api/openapi.yaml: 32 random bytes, base64url without padding.
const TOKEN_PATTERN = /^[A-Za-z0-9_-]{43}$/;

// Whether the token can be a real one. A malformed token is never sent: the page shows the
// not-found state at once, as the API would answer.
export function isWellFormedToken(token: string): boolean {
  return TOKEN_PATTERN.test(token);
}

export const LOADING_TEXT = "Memuat…";

export const SUMMARY_LABEL = "Ringkasan tagihan";
export const TOTAL_LABEL = "Total tagihan";
export const CREDIT_LABEL = "Saldo lebih";
export const CREDIT_NOTE = "Dipakai otomatis untuk tagihan berikutnya.";
export const EARLIER_MONTH_CHIP = "dari bulan lalu";
export const ALLOCATION_NOTE =
  "Tanpa denda dan bunga. Uang yang masuk melunasi tagihan tertua dulu.";

export const TRANSFER_TITLE = "Transfer bank";
export const TRANSFER_TO_LABEL = "Transfer ke rekening";
export const COPY_BUTTON_TEXT = "Salin nomor rekening";
export const PARTIAL_ALLOWED_TEXT = "Boleh sebagian, tanpa minimum.";
export const RECORDED_BY_ADMIN_TEXT =
  "Admin mencatat pembayaran setelah dana masuk.";
export const FOOTER_TEXT =
  "Halaman ini menunjukkan Lunas setelah pembayaran dicatat.";

export const PAID_TITLE = "Lunas";
export const PAID_TEXT = "Terima kasih, semua tagihan sudah lunas.";
export const PAID_DETAILS_LABEL = "Rincian";

export const RETRY_TEXT = "Coba lagi";

// What the screen reader hears, and the line under the account number, after "Salin".
export const COPY_STATUS_TEXT: Record<CopyStatus, string> = {
  idle: "",
  copied: "Nomor rekening disalin",
  failed: "Tidak bisa menyalin. Salin nomor di atas secara manual.",
};

export const MESSAGES: Record<PaymentPageErrorKind, PaymentPageMessageView> = {
  // Says nothing about whether the token ever existed.
  "not-found": {
    title: "Link tidak berlaku",
    text: "Link pembayaran ini tidak ditemukan atau sudah tidak berlaku. Minta link baru ke pemilik.",
    retry: false,
  },
  "rate-limited": {
    title: "Terlalu sering dibuka",
    text: "Coba lagi beberapa menit lagi.",
    retry: true,
  },
  failed: {
    title: "Halaman tidak bisa dimuat",
    text: "Periksa koneksi internet Anda, lalu coba lagi.",
    retry: true,
  },
};

// The codes of a link that does not work: unknown, malformed or revoked (404), its invoice
// paid or cancelled 30 days ago or longer (410), or a token the API client refused to put in a
// path. All look the same to the tenant.
const NOT_FOUND_CODES: ReadonlySet<string> = new Set([
  "payment_link_not_found",
  "payment_link_expired",
  INVALID_PATH_CODE,
]);

// Which error state a failed getPaymentPage shows. Branches on the API's code; a 404 or 410
// without the API's body (a proxy's page) is still a link that does not work.
export function errorKind(error: unknown): PaymentPageErrorKind {
  if (!(error instanceof ApiError)) return "failed";
  if (NOT_FOUND_CODES.has(error.code)) return "not-found";
  if (error.code === "rate_limited") return "rate-limited";
  if (error.status === 404 || error.status === 410) return "not-found";
  if (error.status === 429) return "rate-limited";
  return "failed";
}

// What the page shows, from the token check and the query. A page already on screen stays when
// a later re-read fails for a passing reason (rate limit, network), but a link that has stopped
// working (revoked, expired) replaces it, also when a QRIS request was the one that found out
// (`linkGone`).
export function paymentPageState(input: {
  wellFormed: boolean;
  data: PaymentPage | undefined;
  error: unknown;
  linkGone?: boolean;
}): PaymentPageState {
  if (!input.wellFormed || input.linkGone === true) {
    return { kind: "error", error: "not-found" };
  }
  const failed =
    input.error === null || input.error === undefined
      ? null
      : errorKind(input.error);
  if (failed === "not-found") return { kind: "error", error: failed };
  if (input.data !== undefined) {
    return { kind: "ready", view: paymentPageView(input.data) };
  }
  if (failed !== null) return { kind: "error", error: failed };
  return { kind: "loading" };
}

function lowerFirst(text: string): string {
  return `${text.charAt(0).toLowerCase()}${text.slice(1)}`;
}

// "Bayar tagihan November 2026", "Bayar tagihan akhir November 2026".
export function pageTitle(invoice: PaymentPageInvoice): string {
  return `Bayar ${lowerFirst(invoiceName(invoice))}`;
}

function headerView(page: PaymentPage): PaymentPageHeaderView {
  return {
    organizationName: page.organizationName,
    title: pageTitle(page.invoice),
    subtitle: `${page.tenantName} · ${page.invoice.unitLabel} ${page.invoice.propertyName}`,
  };
}

// What is left of a partly paid invoice: "Sisa Oktober 2026", "Sisa tagihan akhir Oktober 2026".
export function remainderLabel(invoice: PaymentPageInvoice): string {
  const month = formatMonthYear(invoice.period);
  return invoice.kind === "final"
    ? `Sisa tagihan akhir ${month}`
    : `Sisa ${month}`;
}

// The invoice's name while nothing of it is paid, otherwise what is left of it.
export function summaryRowLabel(invoice: PaymentPageInvoice): string {
  return invoice.balance < invoice.amount
    ? remainderLabel(invoice)
    : invoiceName(invoice);
}

// Calendar dates (YYYY-MM-DD) compare as strings.
function isEarlier(invoice: PaymentPageInvoice, than: PaymentPageInvoice) {
  return invoice.period < than.period;
}

export function summaryRows(page: PaymentPage): SummaryRowView[] {
  return page.openInvoices.map((invoice) => ({
    id: invoice.id,
    label: summaryRowLabel(invoice),
    amount: formatRupiah(invoice.balance),
    fromEarlierMonth: isEarlier(invoice, page.invoice),
  }));
}

// "Tagihan November 2026 sudah lunas." while older invoices are still open. A link invoice that
// bills nothing now (cancelled by a correction) was not paid, so it gets no note.
export function linkInvoicePaidNote(page: PaymentPage): string | null {
  const { invoice } = page;
  if (invoice.balance !== 0 || invoice.amount <= 0) return null;
  return `${invoiceName(invoice)} sudah lunas.`;
}

export function transferView(page: PaymentPage): TransferView | null {
  if (page.bankAccount === null) return null;
  return {
    bankName: page.bankAccount.bankName,
    accountNumber: page.bankAccount.accountNumber,
    holderLine: `a.n. ${page.bankAccount.accountHolder}`,
    noteLine: `Tulis nama Anda di berita transfer: ${page.tenantName}`,
  };
}

export function noTransferText(organizationName: string): string {
  return `Cara bayar belum diatur. Hubungi ${organizationName} untuk membayar.`;
}

function creditText(page: PaymentPage): string | null {
  return page.credit > 0 ? formatRupiah(page.credit) : null;
}

function openView(page: PaymentPage): OpenPaymentPageView {
  return {
    status: "open",
    header: headerView(page),
    linkInvoicePaidNote: linkInvoicePaidNote(page),
    rows: summaryRows(page),
    total: formatRupiah(page.outstanding),
    credit: creditText(page),
    transfer: transferView(page),
    noTransferText: noTransferText(page.organizationName),
    qris: qrisOfferView(page),
    tenantName: page.tenantName,
    outstanding: page.outstanding,
    promise: promiseOfferView(page),
    activePromise: activePromiseView(page.promise),
    telegram: telegramOfferView(page),
  };
}

// A link invoice cancelled by a correction bills nothing, so it is not listed (as in
// linkInvoicePaidNote).
function paidView(page: PaymentPage): PaidPaymentPageView {
  return {
    status: "paid",
    header: headerView(page),
    invoice:
      page.invoice.amount <= 0
        ? null
        : {
            name: invoiceName(page.invoice),
            amount: formatRupiah(page.invoice.amount),
          },
    credit: creditText(page),
  };
}

// The API's `paid` decides between the two states; the page never works it out itself.
export function paymentPageView(page: PaymentPage): PaymentPageView {
  return page.paid ? paidView(page) : openView(page);
}

// Copies the account number. Resolves to the status to show; never throws (no clipboard in an
// insecure context, permission refused).
export async function copyText(
  text: string,
  clipboard: Pick<Clipboard, "writeText"> | undefined,
): Promise<CopyStatus> {
  if (clipboard === undefined) return "failed";
  try {
    await clipboard.writeText(text);
    return "copied";
  } catch {
    return "failed";
  }
}

// QRIS (boards "Bayar QRIS", "QR kedaluwarsa", "Sedang dicek", "QRIS gangguan"). The amount of a
// QR is the API's outstanding sent back unchanged, and every amount shown is an API field
// formatted: nothing here adds or subtracts rupiah.

// AutoGoPay's limit for one QR (CreateQrisRequest.amount maximum).
export const QRIS_LIMIT = 10_000_000;

// The API reuses a live QR for the same amount while it has more than this left, so "Buat QR
// baru" only shows from here on.
export const NEW_QR_THRESHOLD_MS = 2 * 60 * 1000;

// Polling getPaymentStatus. The status read shares 30 requests a minute per token with the page
// and the image, so never faster than this.
export const PENDING_POLL_MS = 5_000;
export const CHECKING_POLL_MS = 15_000;

// After 409 qris_busy, how long the page waits for the other request's QR before it gives up
// (a create takes up to about 20 seconds).
export const BUSY_WAIT_MS = 30_000;

// qris_overloaded comes with Retry-After: 5. The page retries once after it, then offers bank
// transfer as for an outage.
export const QRIS_OVERLOADED_RETRY_MS = 5_000;

export const PAYMENT_TABS_LABEL = "Cara bayar";
export const QRIS_TAB_TEXT = "QRIS";
export const TRANSFER_TAB_TEXT = "Transfer bank";

export const QRIS_AMOUNT_LABEL = "Jumlah";
export const QRIS_LIMIT_NOTE =
  "QRIS paling banyak Rp10.000.000 sekali bayar. Sisanya bayar dengan QR berikutnya atau transfer bank.";
export const QRIS_CREATING_TEXT = "Membuat QR…";
export const QRIS_HELP_TEXT =
  "Bayar dari HP ini? Unduh QR, lalu pilih dari galeri di aplikasi bank atau e-wallet.";
export const QRIS_DOWNLOAD_TEXT = "Unduh QR";
export const QRIS_PENDING_TEXT =
  "Menunggu pembayaran. Halaman ini berubah sendiri jadi Lunas.";
export const QRIS_NEW_TEXT = "Buat QR baru";
export const QRIS_PAID_CLAIM_TEXT = "Saya sudah bayar";
export const QRIS_CREATE_FAILED_TEXT = "QR belum bisa dibuat. Coba lagi.";

// "Cara bayar pakai QR yang diunduh", the guide under a live QR (T-085, no board yet). App
// menus change, so a per-app note names only what the app's own help page said (checked
// 2026-11-06: bca.co.id, bankmandiri.co.id, gopay.co.id); every other app gets the generic hint.
export const QRIS_GUIDE_TITLE = "Cara bayar pakai QR yang diunduh";
export const QRIS_GUIDE_STEPS: readonly string[] = [
  `Ketuk "${QRIS_DOWNLOAD_TEXT}". Bisa juga ambil tangkapan layar, asal seluruh kotak QR ikut.`,
  "Buka aplikasi bank atau e-wallet, lalu pilih menu QRIS atau Scan.",
  'Ketuk ikon galeri atau "Upload QR", lalu pilih gambar QR tadi.',
  "Pastikan jumlahnya sama dengan di halaman ini, lalu konfirmasi pembayarannya di aplikasi.",
  `Kembali ke halaman ini. Kalau belum Lunas, ketuk "${QRIS_PAID_CLAIM_TEXT}".`,
];
export const QRIS_GUIDE_IPHONE_NOTE =
  "Di iPhone, QR yang diunduh biasanya masuk ke app File, bukan Foto. Kalau aplikasi tidak menemukannya, pakai tangkapan layar.";
export const QRIS_GUIDE_APPS_TITLE = "Di aplikasi Anda";
export const QRIS_GUIDE_APPS: readonly QrisGuideApp[] = [
  {
    name: "myBCA",
    steps: 'Buka gambar QR di galeri, ketuk "Bagikan", lalu pilih myBCA.',
  },
  {
    name: "Livin' by Mandiri",
    steps: "Di beranda pilih QRIS, lalu pilih gambar QR dari galeri.",
  },
  {
    name: "GoPay",
    steps: 'Di beranda pilih QRIS, lalu "Upload QR" (ikon galeri).',
  },
  {
    name: "Aplikasi lain",
    steps:
      "Seperti BRImo, wondr by BNI, OVO, DANA atau ShopeePay: di layar scan QRIS, cari ikon galeri atau unggah gambar.",
  },
];
export const QRIS_GUIDE_FALLBACK =
  "Tidak ada pilihan galeri? Scan QR ini dengan kamera dari HP lain, atau bayar lewat Transfer bank.";

export const QRIS_EXPIRED_TITLE = "QR kedaluwarsa";
export const QRIS_EXPIRED_TEXT =
  "QR hanya berlaku 15 menit dan sudah habis. Tagihan tidak berubah, dan QR lama sudah dibatalkan.";
export const QRIS_EXPIRED_NOTE = "Belum ada pembayaran yang masuk dari QR ini.";

export const QRIS_CHECKING_TITLE = "Pembayaran sedang dicek";
export const QRIS_CHECKING_TEXT =
  "Konfirmasi dari bank belum masuk. Biasanya selesai kurang dari 30 menit, dan halaman ini berubah sendiri jadi Lunas.";
export const QRIS_CHECKING_CHIP = "Jangan bayar ulang.";
export const QRIS_CHECKED_SINCE_LABEL = "Dicek sejak";
export const QRIS_METHOD_LABEL = "Metode";
export const QRIS_METHOD_VALUE = "QRIS";

export const QRIS_UNAVAILABLE_TEXT =
  "QRIS sedang gangguan. Pakai transfer bank dulu; tagihan tetap berjalan seperti biasa.";

// The amount one QR asks for: the API's outstanding as is, or Rp10.000.000 when more is owed
// (the most one QR can take; the rest is paid by a next QR or a transfer). Null when nothing is
// owed. Choosing the smaller of two API-given limits, not computing rupiah.
export function qrisCreateAmount(outstanding: number): number | null {
  if (outstanding < 1) return null;
  return Math.min(outstanding, QRIS_LIMIT);
}

// "Bayar QRIS Rp2.250.000", or "Bayar QRIS Rp10.000.000" when more than that is owed.
export function qrisStartLabel(outstanding: number): string {
  return `Bayar QRIS ${formatRupiah(Math.min(outstanding, QRIS_LIMIT))}`;
}

// Says one QR pays at most Rp10.000.000, when more than that is owed; null otherwise.
export function qrisLimitNote(outstanding: number): string | null {
  return outstanding > QRIS_LIMIT ? QRIS_LIMIT_NOTE : null;
}

// The file name "Unduh QR" saves under: "qris-kamar-07.png", "qris-tagihan.png". Built from the
// unit's label only, so it never holds the token.
export function qrisDownloadName(unitLabel: string): string {
  const slug = unitLabel
    .normalize("NFKD")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return `qris-${slug === "" ? "tagihan" : slug}.png`;
}

// Null when the organization has not set up QRIS: the page shows bank transfer only, with no
// tabs and no outage warning.
export function qrisOfferView(page: PaymentPage): QrisOfferView | null {
  if (!page.qrisAvailable) return null;
  return {
    outstanding: page.outstanding,
    startLabel: qrisStartLabel(page.outstanding),
    limitNote: qrisLimitNote(page.outstanding),
    downloadName: qrisDownloadName(page.invoice.unitLabel),
    totalLine: `${TOTAL_LABEL} ${formatRupiah(page.outstanding)}`,
    quickAmounts: quickAmounts(page),
  };
}

// The QR image, same-origin from getPaymentQRImage. The token stays in the path, as the
// contract defines; the query only makes a new QR load instead of the browser's last image, so
// it is built from the attempt's expiry, never from the token.
export function qrisImageUrl(token: string, expiresAt: string): string {
  const path = `${API_BASE_PATH}/t/${token}/qris.png`;
  const version = Date.parse(expiresAt);
  return Number.isNaN(version) ? path : `${path}?v=${version}`;
}

// Whole seconds until the QR expires by the device's clock, never below 0.
export function remainingSeconds(expiresAt: string, now: number): number {
  const left = Date.parse(expiresAt) - now;
  if (Number.isNaN(left) || left <= 0) return 0;
  return Math.ceil(left / 1000);
}

// 872 seconds give "14:32", 0 gives "0:00".
export function formatCountdown(seconds: number): string {
  const whole = Math.max(0, Math.floor(seconds));
  const minutes = Math.floor(whole / 60);
  const rest = String(whole % 60).padStart(2, "0");
  return `${minutes}:${rest}`;
}

// "Berlaku 14:32 lagi".
export function countdownText(seconds: number): string {
  return `Berlaku ${formatCountdown(seconds)} lagi`;
}

// "Buat QR baru" under a live QR: only when it has two minutes or less left (or has run out),
// since before that the API answers the same QR again.
export function showsNewQrUnderLive(expiresAt: string, now: number): boolean {
  const left = Date.parse(expiresAt) - now;
  return Number.isNaN(left) || left <= NEW_QR_THRESHOLD_MS;
}

// Whether getPaymentStatus is read in this state: once on load, and while a QR of this visit can
// still change.
export function readsStatus(card: QrisCard): boolean {
  return (
    card.kind === "resuming" ||
    card.kind === "waiting" ||
    card.kind === "pending" ||
    card.kind === "checking"
  );
}

// How often getPaymentStatus is polled; false stops polling (the one read on load included).
export function statusPollInterval(card: QrisCard): number | false {
  if (card.kind === "pending" || card.kind === "waiting") {
    return PENDING_POLL_MS;
  }
  if (card.kind === "checking") return CHECKING_POLL_MS;
  return false;
}

// What the live region says when the card changes; empty where nothing needs saying (the outage
// banner announces itself as an alert).
export function qrisAnnouncement(card: QrisCard): string {
  switch (card.kind) {
    case "creating":
    case "waiting":
      return QRIS_CREATING_TEXT;
    case "pending":
      return QRIS_PENDING_TEXT;
    case "checking":
      return QRIS_CHECKING_TITLE;
    case "expired":
      return QRIS_EXPIRED_TITLE;
    default:
      return "";
  }
}

// "Jumlah Rp2.250.000" under a QR; null when the API sent no amount.
export function qrisAmountText(amount: number | null): string | null {
  return amount === null ? null : formatRupiah(amount);
}

// The rows of the "Sedang dicek" details card. A missing amount or time leaves its row out.
export function qrisCheckingRows(card: {
  amount: number | null;
  checkedAt: string | null;
}): QrisDetailRow[] {
  const rows: QrisDetailRow[] = [];
  if (card.amount !== null) {
    rows.push({ label: QRIS_AMOUNT_LABEL, value: formatRupiah(card.amount) });
  }
  if (card.checkedAt !== null) {
    rows.push({
      label: QRIS_CHECKED_SINCE_LABEL,
      value: formatDateTime(card.checkedAt),
    });
  }
  rows.push({ label: QRIS_METHOD_LABEL, value: QRIS_METHOD_VALUE });
  return rows;
}

export const INITIAL_QRIS_MODEL: QrisModel = {
  card: { kind: "resuming" },
  tab: "qris",
  alert: null,
  checkRunning: false,
  checks: 0,
};

// Whether "Bayar QRIS" or "Buat QR baru" may start a request now.
export function canStartQris(model: QrisModel): boolean {
  if (model.checkRunning) return false;
  const { kind } = model.card;
  return (
    kind === "idle" ||
    kind === "expired" ||
    kind === "pending" ||
    kind === "unavailable"
  );
}

// Whether "Saya sudah bayar" may run now.
export function canCheckQris(model: QrisModel): boolean {
  return model.card.kind === "pending" && !model.checkRunning;
}

function step(
  model: QrisModel,
  effects: Partial<Omit<QrisStep, "model">> = {},
): QrisStep {
  return {
    model,
    refetchPage: false,
    linkGone: false,
    pollNow: false,
    retryCreate: false,
    ...effects,
  };
}

// A new card; the alert goes with the old one unless the card stays the same kind.
function withCard(
  model: QrisModel,
  card: QrisCard,
  alert: string | null = null,
): QrisModel {
  const keepAlert = alert === null && card.kind === model.card.kind;
  return { ...model, card, alert: keepAlert ? model.alert : alert };
}

function amountOf(status: QrisStatus, fallback: number | null): number | null {
  return status.amount ?? fallback;
}

function isLive(
  status: QrisStatus,
  now: number,
): status is QrisStatus & {
  expiresAt: string;
} {
  return (
    status.status === "pending" &&
    status.expiresAt !== undefined &&
    Date.parse(status.expiresAt) > now
  );
}

function pendingCard(
  status: QrisStatus & { expiresAt: string },
  fallback: number | null,
): QrisCard {
  return {
    kind: "pending",
    amount: amountOf(status, fallback),
    expiresAt: status.expiresAt,
  };
}

function checkingCard(status: QrisStatus, fallback: number | null): QrisCard {
  return {
    kind: "checking",
    amount: amountOf(status, fallback),
    checkedAt: status.checkedAt ?? null,
  };
}

function cardAmount(card: QrisCard): number | null {
  return card.kind === "pending" ||
    card.kind === "checking" ||
    card.kind === "expired"
    ? card.amount
    : null;
}

// What a status read means for a QR this visit is showing (pending or checking). Only here may
// expired and paid be believed: the QR they speak of is the one on screen.
function followQr(model: QrisModel, status: QrisStatus): QrisStep {
  const fallback = cardAmount(model.card);
  switch (status.status) {
    case "pending":
      if (status.expiresAt === undefined) return step(model);
      return step(
        withCard(
          model,
          pendingCard({ ...status, expiresAt: status.expiresAt }, fallback),
        ),
      );
    case "checking":
      return step(withCard(model, checkingCard(status, fallback)));
    case "paid":
      return step(withCard(model, { kind: "idle", paid: true }), {
        refetchPage: true,
      });
    case "expired":
      return step(
        withCard(model, {
          kind: "expired",
          amount: amountOf(status, fallback),
        }),
      );
    default:
      return step(withCard(model, { kind: "idle" }));
  }
}

function statusRead(
  model: QrisModel,
  status: QrisStatus,
  now: number,
  checks: number,
): QrisStep {
  // A read that began before the latest check started or ended can be older than its answer.
  if (model.checkRunning || checks !== model.checks) return step(model);
  const { card } = model;
  switch (card.kind) {
    // On load only a QR that can still be paid, or one being checked, comes back. A paid or
    // expired answer can be weeks old, so it never shows.
    case "resuming":
      if (isLive(status, now)) {
        return step(withCard(model, pendingCard(status, null)));
      }
      if (status.status === "checking") {
        return step(withCard(model, checkingCard(status, null)));
      }
      return step(withCard(model, { kind: "idle" }));
    // Another request is making the QR: wait for it to show. Until then an expired or empty
    // answer is about an older QR.
    case "waiting":
      if (isLive(status, now)) {
        return step(withCard(model, pendingCard(status, null)));
      }
      if (status.status === "checking") {
        return step(withCard(model, checkingCard(status, null)));
      }
      if (status.status === "paid") {
        return step(withCard(model, { kind: "idle", paid: true }), {
          refetchPage: true,
        });
      }
      if (now >= card.until) {
        return step(withCard(model, { kind: "idle" }, QRIS_CREATE_FAILED_TEXT));
      }
      return step(model);
    case "pending":
    case "checking":
      return followQr(model, status);
    default:
      return step(model);
  }
}

// A link that stopped working, by its code or by a bare 404 or 410 (errorKind). QRIS's own
// 404 qris_not_found is not one, so callers that can get it check it first.
function isLinkError(error: unknown): boolean {
  return errorKind(error) === "not-found";
}

function statusFailed(model: QrisModel, error: unknown): QrisStep {
  if (isLinkError(error)) return step(model, { linkGone: true });
  // A failed read on load starts from "Bayar QRIS"; during polling the next tick tries again.
  if (model.card.kind === "resuming") {
    return step(withCard(model, { kind: "idle" }));
  }
  return step(model);
}

function created(model: QrisModel, status: QrisStatus): QrisStep {
  if (model.card.kind !== "creating") return step(model);
  if (status.status === "pending" && status.expiresAt !== undefined) {
    return step(
      withCard(
        model,
        pendingCard({ ...status, expiresAt: status.expiresAt }, null),
      ),
    );
  }
  if (status.status === "checking") {
    return step(withCard(model, checkingCard(status, null)));
  }
  if (status.status === "paid") {
    return step(withCard(model, { kind: "idle", paid: true }), {
      refetchPage: true,
    });
  }
  return step(withCard(model, { kind: "idle" }, QRIS_CREATE_FAILED_TEXT));
}

const AMOUNT_FIELD = "amount";

function createFailed(model: QrisModel, error: unknown, now: number): QrisStep {
  if (model.card.kind !== "creating") return step(model);
  const idle: QrisCard = { kind: "idle" };
  if (!(error instanceof ApiError)) {
    return step(withCard(model, idle, QRIS_CREATE_FAILED_TEXT));
  }
  switch (error.code) {
    case "qris_busy":
      return step(
        withCard(model, { kind: "waiting", until: now + BUSY_WAIT_MS }),
      );
    case "nothing_to_pay":
      return step(withCard(model, idle), { refetchPage: true });
    case "qris_overloaded":
      // Nothing reached AutoGoPay: retry once, then fall back as for an outage.
      if (model.card.kind === "creating" && model.card.retried !== true) {
        return step(withCard(model, { kind: "creating", retried: true }), {
          retryCreate: true,
        });
      }
      return step({
        ...withCard(model, { kind: "unavailable" }),
        tab: "transfer",
      });
    case "qris_unavailable":
      return step({
        ...withCard(model, { kind: "unavailable" }),
        tab: "transfer",
      });
  }
  if (error.status === 429) {
    return step(withCard(model, idle, error.message));
  }
  if (error.status === 400 && error.field === AMOUNT_FIELD) {
    return step(withCard(model, idle, error.message), {
      refetchPage: error.code === "qris_amount_above_outstanding",
    });
  }
  if (isLinkError(error)) {
    return step(withCard(model, idle), { linkGone: true });
  }
  return step(withCard(model, idle, QRIS_CREATE_FAILED_TEXT));
}

function checked(model: QrisModel, status: QrisStatus): QrisStep {
  const done = {
    ...model,
    checkRunning: false,
    checks: model.checks + 1,
  };
  const fallback = cardAmount(model.card);
  switch (status.status) {
    case "checking":
      return step(withCard(done, checkingCard(status, fallback)));
    case "paid":
      return step(withCard(done, { kind: "idle", paid: true }), {
        refetchPage: true,
      });
    case "expired":
      return step(
        withCard(done, { kind: "expired", amount: amountOf(status, fallback) }),
      );
    case "pending":
      if (status.expiresAt === undefined) return step(done);
      return step(
        withCard(
          done,
          pendingCard({ ...status, expiresAt: status.expiresAt }, fallback),
        ),
      );
    default:
      return step(withCard(done, { kind: "idle" }));
  }
}

function checkFailed(model: QrisModel, error: unknown): QrisStep {
  const done = {
    ...model,
    checkRunning: false,
    checks: model.checks + 1,
  };
  if (error instanceof ApiError) {
    if (error.code === "qris_busy") return step(done, { pollNow: true });
    if (error.code === "qris_not_found") {
      return step(withCard(done, { kind: "idle" }));
    }
    if (error.status === 429) return step({ ...done, alert: error.message });
  }
  if (isLinkError(error)) return step(done, { linkGone: true });
  return step({
    ...done,
    alert: error instanceof ApiError ? error.message : GENERIC_ERROR_MESSAGE,
  });
}

// The QRIS card's state machine: the next model for an event, and what the hook does besides.
// Pure, so every API answer is a table test.
export function qrisStep(model: QrisModel, event: QrisEvent): QrisStep {
  switch (event.type) {
    case "status-read":
      return statusRead(model, event.status, event.now, event.checks);
    case "status-failed":
      return statusFailed(model, event.error);
    case "create-started":
      if (!canStartQris(model)) return step(model);
      return step({
        ...model,
        card: { kind: "creating" },
        tab: "qris",
        alert: null,
      });
    case "create-skipped":
      if (model.card.kind !== "creating") return step(model);
      return step(withCard(model, { kind: "idle" }));
    case "created":
      return created(model, event.status);
    case "create-failed":
      return createFailed(model, event.error, event.now);
    case "check-started":
      if (!canCheckQris(model)) return step(model);
      return step({
        ...model,
        checkRunning: true,
        checks: model.checks + 1,
        alert: null,
      });
    case "checked":
      if (!model.checkRunning) return step(model);
      return checked(model, event.status);
    case "check-failed":
      if (!model.checkRunning) return step(model);
      return checkFailed(model, event.error);
    case "tab-selected":
      return step({ ...model, tab: event.tab });
  }
}

// The alt text of the QR image: "Kode QRIS Rp2.250.000".
export function qrisImageAlt(amount: number | null): string {
  return amount === null ? "Kode QRIS" : `Kode QRIS ${formatRupiah(amount)}`;
}

// Radix Tabs hands back the value as a string.
export function toPaymentTab(value: string): PaymentTab {
  return value === "transfer" ? "transfer" : "qris";
}

// "Bayar sebagian" (boards "Bayar sebagian" and "QRIS bayar sebagian"): the tenant picks an
// amount, the API previews what it pays (getPaymentPreview), and one QR is made for it. Amounts
// are compared, never added or subtracted: what the amount pays and what is left are the API's.

export const PARTIAL_TITLE = "Bayar sebagian";
export const PARTIAL_AMOUNT_LABEL = "Nominal yang dibayar sekarang";
export const PARTIAL_AMOUNT_HINT = "Bebas berapa pun, tanpa minimum.";
export const PARTIAL_QUICK_LABEL = "Pilih nominal";
export const PARTIAL_ABOVE_LIMIT_TEXT =
  "QRIS paling banyak Rp10.000.000 sekali bayar.";
export const PREVIEW_TITLE = "Uang ini dipakai untuk";
export const PREVIEW_NOTE = "Tagihan tertua selalu dilunasi lebih dulu.";
export const PREVIEW_PAID_CHIP = "Lunas";
export const PARTIAL_CHANGE_TEXT = "Ubah nominal";

// How long typing pauses before the preview is read. The preview shares 30 requests a minute per
// token with the page and the QR status, so every keystroke must not be a request.
export const PREVIEW_DEBOUNCE_MS = 400;

// The round amounts offered under the field, each only while it is below what is owed.
export const QUICK_ROUND_AMOUNTS: readonly number[] = [
  250_000, 500_000, 1_000_000,
];

// The quick amounts of "Bayar sebagian": what is left of the oldest open invoice (only when more
// than one is open, since otherwise it is the total), then Rp250.000, Rp500.000 and Rp1.000.000,
// each only when strictly below the total. An amount one QR cannot take, or one already offered,
// is left out. Comparisons only.
export function quickAmounts(page: PaymentPage): QuickAmountView[] {
  const chips: QuickAmountView[] = [];
  const oldest = page.openInvoices[0];
  if (
    page.openInvoices.length > 1 &&
    oldest !== undefined &&
    oldest.balance > 0 &&
    oldest.balance < page.outstanding &&
    oldest.balance <= QRIS_LIMIT
  ) {
    chips.push({
      amount: oldest.balance,
      label: `${summaryRowLabel(oldest)} · ${formatRupiah(oldest.balance)}`,
    });
  }
  for (const amount of QUICK_ROUND_AMOUNTS) {
    if (amount >= page.outstanding) continue;
    if (chips.some((chip) => chip.amount === amount)) continue;
    chips.push({ amount, label: formatRupiah(amount) });
  }
  return chips;
}

// What the amount field holds. Empty and 0 are "nothing yet" (no message); above what one QR
// takes is refused here. Above what is owed is the API's to say (the preview's 400).
export function partialAmountCheck(text: string): PartialAmountCheck {
  const amount = parseRupiahInput(text);
  if (amount === null || amount < 1) return { kind: "empty" };
  if (amount > QRIS_LIMIT) {
    return { kind: "invalid", message: PARTIAL_ABOVE_LIMIT_TEXT };
  }
  return { kind: "valid", amount };
}

// The field text for an amount: 750000 gives "750.000".
export function partialFieldText(amount: number): string {
  return groupRupiahDigits(String(amount));
}

// The amount one QR of "Bayar sebagian" asks for: the chosen amount, but never above what is
// owed now nor above one QR's most. Null when nothing is owed. The smallest of three, no math.
export function qrisPartialAmount(
  chosen: number,
  outstanding: number,
): number | null {
  if (outstanding < 1 || chosen < 1) return null;
  return Math.min(chosen, outstanding, QRIS_LIMIT);
}

// "Bayar QRIS Rp1.000.000"; just "Bayar QRIS" while no amount is valid.
export function partialPayLabel(amount: number | null): string {
  return amount === null ? "Bayar QRIS" : `Bayar QRIS ${formatRupiah(amount)}`;
}

// "Sisa Rp1.250.000 tetap jadi tagihan." under the chosen amount; null when it pays everything.
export function chosenRemainderLine(outstandingAfter: number): string | null {
  return outstandingAfter > 0
    ? `Sisa ${formatRupiah(outstandingAfter)} tetap jadi tagihan.`
    : null;
}

// The chosen-amount card: the amount of the QR once there is one, since a QR is capped at what is
// owed when it is made (a payment recorded meanwhile makes the next QR smaller). The line under it
// (what stays owed, or the promised rest) came from the preview of the chosen amount, so it shows
// only while the QR's amount is that amount. Comparisons only.
export function chosenAmountView(
  mode: ChosenFlowMode,
  card: QrisCard,
): ChosenAmountView {
  const chosen = mode.amount;
  const qrAmount =
    (card.kind === "pending" ||
      card.kind === "checking" ||
      card.kind === "expired") &&
    card.amount !== null
      ? card.amount
      : chosen;
  return {
    amount: formatRupiah(qrAmount),
    remainderLine: qrAmount === chosen ? chosenRemainder(mode) : null,
  };
}

// The "Uang ini dipakai untuk" card, from the API's answer as is.
export function previewView(preview: PaymentPreview): PreviewView {
  return {
    amount: preview.amount,
    rows: preview.items.map((item) => ({
      id: item.invoiceId,
      name: invoiceTitle(item),
      chip: item.paidInFull
        ? PREVIEW_PAID_CHIP
        : `sisa ${formatRupiah(item.balanceAfter)}`,
      paidInFull: item.paidInFull,
      applied: formatRupiah(item.applied),
    })),
    outstandingAfterLine:
      preview.outstandingAfter > 0
        ? `Sisa tagihan setelah ini ${formatRupiah(preview.outstandingAfter)}`
        : null,
    outstandingAfter: preview.outstandingAfter,
  };
}

// What a failed preview shows: a link that stopped working replaces the page, the API's refusal
// of the amount goes under the field, a rate limit shows the API's message, anything else the
// generic one.
export function previewErrorView(error: unknown): PreviewErrorView {
  if (isLinkError(error)) return { kind: "link-gone" };
  if (!(error instanceof ApiError)) {
    return { kind: "alert", message: GENERIC_ERROR_MESSAGE };
  }
  if (error.status === 400 && error.field === AMOUNT_FIELD) {
    return { kind: "field", message: error.message };
  }
  if (error.status === 429) return { kind: "alert", message: error.message };
  return { kind: "alert", message: GENERIC_ERROR_MESSAGE };
}

// "Lanjut pilih tanggal pelunasan": the entry's primary when the amount leaves something owed and
// a promise can be made, so the tenant says when the rest comes before paying.
export const PROMISE_NEXT_TEXT = "Lanjut pilih tanggal pelunasan";

// Whether the entry's primary goes on to the Janji screen: a promise can be made and the amount
// leaves something owed. The preview's outstandingAfter decides once it is for exactly this
// amount; until then the amount is compared with what is owed (a comparison, no math).
export function entryLeadsToPromise(input: {
  amount: number | null;
  preview: PreviewView | null;
  outstanding: number;
  promiseOffered: boolean;
}): boolean {
  const { amount, preview, outstanding, promiseOffered } = input;
  if (!promiseOffered || amount === null) return false;
  if (preview !== null && preview.amount === amount) {
    return preview.outstandingAfter > 0;
  }
  return amount < outstanding;
}

// The entry's primary: "Lanjut pilih tanggal pelunasan", or "Bayar QRIS Rp…" when the amount
// pays everything (or no promise can be made).
export function entryPrimaryLabel(
  amount: number | null,
  leadsToPromise: boolean,
): string {
  return leadsToPromise ? PROMISE_NEXT_TEXT : partialPayLabel(amount);
}

// "Berikutnya: pilih kapan sisa Rp1.250.000 dilunasi." under the preview, when the next step is
// the Janji screen.
export function entryNextLine(
  preview: PreviewView | null,
  leadsToPromise: boolean,
): string | null {
  if (!leadsToPromise || preview === null || preview.outstandingAfter <= 0) {
    return null;
  }
  return `Berikutnya: pilih kapan sisa ${formatRupiah(preview.outstandingAfter)} dilunasi.`;
}

// "Sisa Rp1.250.000 dijanjikan Selasa, 1 Desember 2026." under a chosen amount whose rest was
// promised just before. A promise of part of the rest names both amounts.
export function chosenPromiseLine(
  outstandingAfter: number,
  promise: PaymentPromise,
): string {
  const rest = formatRupiah(outstandingAfter);
  const date = formatLongDate(promise.promisedDate);
  if (
    promise.promisedAmount === null ||
    promise.promisedAmount >= outstandingAfter
  ) {
    return `Sisa ${rest} dijanjikan ${date}.`;
  }
  return `Sisa ${rest}, ${formatRupiah(promise.promisedAmount)} dijanjikan ${date}.`;
}

// The line under the chosen amount: the promised rest, or "Sisa … tetap jadi tagihan.", or
// nothing when the amount pays everything.
export function chosenRemainder(mode: ChosenFlowMode): string | null {
  return mode.promise === null
    ? chosenRemainderLine(mode.outstandingAfter)
    : chosenPromiseLine(mode.outstandingAfter, mode.promise);
}

// Bayar-Pilih (board "Bayar-Pilih"): what the tenant does with what they owe. Pay it all, pay a
// part of it by QRIS, or promise when they will.

export const BACK_TEXT = "Kembali";
export const CONTINUE_TEXT = "Lanjut";
export const CHOICE_LEGEND = "Mau bayar berapa?";
export const CHOICE_ALL_LABEL = "Bayar semua";
export const CHOICE_PARTIAL_LABEL = "Bayar sebagian";
export const CHOICE_PARTIAL_SUB = "Tentukan sendiri nominalnya, berapa pun";
export const CHOICE_LATER_LABEL = "Belum bisa bayar sekarang";
export const CHOICE_LATER_SUB = "Catat kapan Anda akan melunasi";

export const PAY_MODE: PayFlowMode = { kind: "pay" };

export function choiceMode(selected: PayChoice): PayFlowMode {
  return { kind: "choice", selected };
}

// "Bayar semua" always; "Bayar sebagian" only with QRIS (a part is paid by QRIS for now);
// "Belum bisa bayar sekarang" only when the API gives the dates a promise may name.
export function payChoices(view: OpenPaymentPageView): ChoiceOptionView[] {
  const choices: ChoiceOptionView[] = [
    {
      value: "all",
      label: CHOICE_ALL_LABEL,
      sub: `${view.total}, langsung lunas`,
    },
  ];
  if (view.qris !== null) {
    choices.push({
      value: "partial",
      label: CHOICE_PARTIAL_LABEL,
      sub: CHOICE_PARTIAL_SUB,
    });
  }
  if (view.promise !== null) {
    choices.push({
      value: "later",
      label: CHOICE_LATER_LABEL,
      sub: CHOICE_LATER_SUB,
    });
  }
  return choices;
}

// Radix RadioGroup hands back the value as a string.
export function toPayChoice(value: string): PayChoice | null {
  return value === "all" || value === "partial" || value === "later"
    ? value
    : null;
}

// The picked choice while it is still offered (a re-read page can drop one), else "Bayar semua".
export function offeredChoice(
  selected: PayChoice,
  choices: readonly ChoiceOptionView[],
): PayChoice {
  return choices.some((choice) => choice.value === selected) ? selected : "all";
}

// The one primary under the choices: "Lanjut bayar Rp2.250.000", or "Lanjut".
export function choicePrimaryLabel(choice: PayChoice, total: string): string {
  return choice === "all" ? `Lanjut bayar ${total}` : CONTINUE_TEXT;
}

// The first view: the choice, or straight to paying when "Bayar semua" is the only one.
export function initialFlowMode(view: OpenPaymentPageView): PayFlowMode {
  return payChoices(view).length > 1 ? choiceMode("all") : PAY_MODE;
}

// "Kembali" under the pay view: there is a choice to go back to, and no QR is being made, live
// or checked (`card` is null without QRIS).
export function showsPayBack(
  view: OpenPaymentPageView,
  card: QrisCard | null,
): boolean {
  if (payChoices(view).length < 2) return false;
  return card === null || canChangePartialAmount(card);
}

// The primary of the choice.
export function nextFromChoice(
  choice: PayChoice,
  view: OpenPaymentPageView,
): PayFlowMode {
  if (choice === "partial" && view.qris !== null) {
    return { kind: "entry", text: "" };
  }
  if (choice === "later" && view.promise !== null) {
    return {
      kind: "promise",
      draft: defaultPromiseDraft(view.promise),
      paying: null,
      editing: false,
      back: choiceMode("later"),
    };
  }
  return PAY_MODE;
}

// The entry's primary, once the API has previewed exactly its amount: on to the Janji screen when
// something stays owed and a promise can be made, otherwise straight to the QR.
export function chooseFromEntry(
  mode: PayFlowMode,
  preview: PreviewView,
  promise: PromiseOfferView | null,
): PayFlowMode {
  if (mode.kind !== "entry") return mode;
  if (preview.outstandingAfter > 0 && promise !== null) {
    return {
      kind: "promise",
      draft: defaultPromiseDraft(promise),
      paying: {
        amount: preview.amount,
        text: mode.text,
        outstandingAfter: preview.outstandingAfter,
      },
      editing: false,
      back: mode,
    };
  }
  return {
    kind: "chosen",
    text: mode.text,
    amount: preview.amount,
    outstandingAfter: preview.outstandingAfter,
    promise: null,
  };
}

// "Ubah nominal": back to the entry with the amount still in the field.
export function changePartialAmount(mode: PayFlowMode): PayFlowMode {
  if (mode.kind !== "chosen") return mode;
  return { kind: "entry", text: mode.text };
}

// "Ubah nominal" shows only while no QR is being made, live or being checked.
export function canChangePartialAmount(card: QrisCard): boolean {
  return (
    card.kind === "idle" ||
    card.kind === "expired" ||
    card.kind === "unavailable"
  );
}

const QR_IN_FLIGHT: ReadonlySet<QrisCard["kind"]> = new Set([
  "creating",
  "waiting",
  "pending",
  "checking",
]);

const QR_RESUMED: ReadonlySet<QrisCard["kind"]> = new Set([
  "waiting",
  "pending",
  "checking",
]);

// After the card changes:
// - a QR found on load (pending or being checked) skips the choice and shows the pay view;
// - a chosen amount whose QR was paid (the card went back to idle marked paid, and the page is
//   read again) shows Janji tercatat when its rest was promised, else the choice, so the tenant
//   sees what is left;
// - a chosen amount whose QR is gone unpaid (status "none", a check's qris_not_found) or whose
//   create failed (idle with an alert) stays, so "Bayar QRIS" makes a new QR for it.
export function flowModeAfterCard(
  mode: PayFlowMode,
  previous: QrisCard,
  next: QrisModel,
): PayFlowMode {
  if (mode.kind === "choice") {
    return previous.kind === "resuming" && QR_RESUMED.has(next.card.kind)
      ? PAY_MODE
      : mode;
  }
  if (mode.kind !== "chosen") return mode;
  if (!QR_IN_FLIGHT.has(previous.kind)) return mode;
  if (next.card.kind !== "idle" || next.card.paid !== true) return mode;
  if (mode.promise === null) return choiceMode("all");
  return {
    kind: "recorded",
    paidAmount: cardAmount(previous) ?? mode.amount,
    promise: mode.promise,
  };
}

// Janji (boards "Janji", "Janji tercatat"): the tenant says when they pay what is left, with no
// approval. A promise covers everything owed now; saving while one is active adds a version
// ("Ubah janji"). Dates are compared as YYYY-MM-DD strings, amounts only compared.

export const PROMISE_TITLE = "Kapan sisanya dilunasi?";
export const PROMISE_KIND_LEGEND = "Pilihan tanggal";
// Labelled by its due date only: the next bill to fall due may be a late-entered period of an
// earlier month (period October, due 4 November), so a month name could be wrong.
export const PROMISE_NEXT_DUE_LABEL = "Bareng tagihan berikutnya";
export const PROMISE_OWN_DATE_LABEL = "Pilih tanggal sendiri";
export const PROMISE_DATE_LABEL = "Tanggal pelunasan";
export const PROMISE_AMOUNT_LABEL = "Nominal janji (boleh dikosongkan)";
export const PROMISE_SAVE_TEXT = "Simpan janji";
export const PROMISE_EDIT_SAVE_TEXT = "Simpan perubahan";
export const PROMISE_SAVING_TEXT = "Menyimpan…";
export const PROMISE_DATE_REQUIRED_TEXT = "Pilih tanggal pelunasan.";
export const PROMISE_AMOUNT_MIN_TEXT =
  "Isi nominal minimal Rp1, atau kosongkan untuk semua sisa.";
export const PROMISE_CARD_LABEL = "Janji pelunasan";
export const PROMISE_ALL_REST_TEXT = "Semua sisa tagihan";
export const EDIT_PROMISE_TEXT = "Ubah janji";
export const RECORDED_TITLE = "Janji tercatat";
export const RECORDED_REMINDER_TEXT =
  "Kami kirim pengingat sehari sebelum tanggal ini. Pengingat tunggakan berhenti sampai tanggal ini; tagihan baru tetap dikirim.";

// Null when no promise can be made: the API sends no dates when nothing is owed.
export function promiseOfferView(page: PaymentPage): PromiseOfferView | null {
  if (page.promiseDates === null) return null;
  return {
    outstanding: page.outstanding,
    earliest: page.promiseDates.earliest,
    latest: page.promiseDates.latest,
    nextInvoiceDueDate: page.nextInvoiceDueDate,
  };
}

// "Rp500.000", or "Semua sisa tagihan" for a promise of everything left.
export function promiseAmountText(promisedAmount: number | null): string {
  return promisedAmount === null
    ? PROMISE_ALL_REST_TEXT
    : formatRupiah(promisedAmount);
}

// "Janji pelunasan 1 Des 2026 · Semua sisa tagihan"; null without an active promise.
export function activePromiseView(
  promise: PaymentPromise | null,
): ActivePromiseView | null {
  if (promise === null || promise.status !== "active") return null;
  return {
    text: `${PROMISE_CARD_LABEL} ${formatDate(promise.promisedDate)} · ${promiseAmountText(promise.promisedAmount)}`,
    promise,
  };
}

// What the promise is of when it is left empty: what stays after paying now, or everything owed.
// Choosing one of two API amounts, no math.
export function promiseRest(
  offer: PromiseOfferView,
  paying: PromisePaying | null,
): number {
  return paying === null ? offer.outstanding : paying.outstandingAfter;
}

// "Bareng tagihan …" is offered only while the API gives the next due date; a kind it no longer
// offers falls back to "Pilih tanggal sendiri".
export function offeredPromiseKind(
  kind: PromiseKind,
  offer: PromiseOfferView,
): PromiseKind {
  return kind === PromiseKind.with_next_invoice &&
    offer.nextInvoiceDueDate === null
    ? PromiseKind.specific_date
    : kind;
}

// A new promise: "Bareng tagihan …" when offered, else "Pilih tanggal sendiri"; no date, no amount.
export function defaultPromiseDraft(offer: PromiseOfferView): PromiseDraft {
  return {
    kind:
      offer.nextInvoiceDueDate === null
        ? PromiseKind.specific_date
        : PromiseKind.with_next_invoice,
    date: "",
    amountText: "",
  };
}

// "Ubah janji": the fields as the active promise has them. A "Bareng tagihan" promise that is no
// longer offered becomes its date, chosen by hand.
export function prefilledPromiseDraft(
  promise: Pick<PaymentPromise, "kind" | "promisedDate" | "promisedAmount">,
  offer: PromiseOfferView,
): PromiseDraft {
  const kind = offeredPromiseKind(promise.kind, offer);
  return {
    kind,
    date: kind === PromiseKind.specific_date ? promise.promisedDate : "",
    amountText:
      promise.promisedAmount === null
        ? ""
        : partialFieldText(promise.promisedAmount),
  };
}

// "Ubah janji" from the active promise card or from Janji tercatat: nothing is paid now, and
// "Kembali" returns to `back`.
export function editPromise(
  promise: PaymentPromise,
  offer: PromiseOfferView,
  back: PayFlowMode,
): PayFlowMode {
  return {
    kind: "promise",
    draft: prefilledPromiseDraft(promise, offer),
    paying: null,
    editing: true,
    back,
  };
}

// "Kembali" on the Janji screen.
export function backFromPromise(mode: PayFlowMode): PayFlowMode {
  return mode.kind === "promise" ? mode.back : mode;
}

// "Janji sebelumnya 1 Des 2026, semua sisa tagihan akan diganti.": a new promise adds a version to
// the active one, so a form that does not start from it says so.
export function promiseReplacesLine(promise: PaymentPromise): string {
  const amount =
    promise.promisedAmount === null
      ? PROMISE_ALL_REST_TEXT.toLowerCase()
      : formatRupiah(promise.promisedAmount);
  return `Janji sebelumnya ${formatDate(promise.promisedDate)}, ${amount} akan diganti.`;
}

// The Janji screen. `active`: the tenant's active promise, which saving replaces.
export function promiseFormView(
  offer: PromiseOfferView,
  paying: PromisePaying | null,
  editing: boolean,
  active: PaymentPromise | null,
): PromiseFormView {
  const kindOptions: ChoiceOptionView[] = [];
  if (offer.nextInvoiceDueDate !== null) {
    kindOptions.push({
      value: PromiseKind.with_next_invoice,
      label: PROMISE_NEXT_DUE_LABEL,
      sub: `Jatuh tempo ${formatDate(offer.nextInvoiceDueDate)}`,
    });
  }
  kindOptions.push({
    value: PromiseKind.specific_date,
    label: PROMISE_OWN_DATE_LABEL,
    sub: `Paling lambat ${formatDate(offer.latest)}, tanpa denda`,
  });
  return {
    replacesLine:
      active !== null && !editing ? promiseReplacesLine(active) : null,
    subline:
      paying === null
        ? `${TOTAL_LABEL} ${formatRupiah(offer.outstanding)}`
        : `Sisa setelah bayar hari ini: ${formatRupiah(paying.outstandingAfter)}`,
    kindOptions,
    dateMin: offer.earliest,
    dateMax: offer.latest,
    amountHint: `Kosongkan untuk semua sisa, ${formatRupiah(promiseRest(offer, paying))}.`,
    primaryLabel:
      paying !== null
        ? `${PROMISE_SAVE_TEXT}, lalu bayar ${formatRupiah(paying.amount)}`
        : editing
          ? PROMISE_EDIT_SAVE_TEXT
          : PROMISE_SAVE_TEXT,
  };
}

// Radix RadioGroup hands back the value as a string.
export function toPromiseKind(value: string): PromiseKind | null {
  return value === PromiseKind.specific_date ||
    value === PromiseKind.with_next_invoice
    ? value
    : null;
}

// "Pilih tanggal dari 3 Nov 2026 sampai 1 Jan 2027."
export function promiseDateRangeText(earliest: string, latest: string): string {
  return `Pilih tanggal dari ${formatDate(earliest)} sampai ${formatDate(latest)}.`;
}

// "Nominal janji paling banyak Rp1.250.000."
export function promiseAmountMaxText(rest: number): string {
  return `Nominal janji paling banyak ${formatRupiah(rest)}.`;
}

// The form fields of "Janji pelunasan" that a message can go under, the API's own field names
// mapped to them by promiseFormError.
export const PROMISE_FORM_API_FIELDS = ["kind", "date", "amountText"] as const;

// A promise's fields checked by comparison, for the tenant's Janji screen and staff's "Catat
// janji": the request they make, or the first field that is wrong (a missing or out-of-range date,
// an amount of 0 or above the rest) with its message. The API checks everything again and its
// message wins. The date is sent only for "Pilih tanggal sendiri", the amount only when typed
// (empty is everything left), payingNow only when something is paid right after. A kind no longer
// offered is sent as "Pilih tanggal sendiri".
export function checkPromiseDraft(
  draft: PromiseDraft,
  offer: PromiseOfferView,
  paying: PromisePaying | null,
): PromiseDraftCheck {
  const invalid = (
    field: PromiseDraftField,
    message: string,
  ): PromiseDraftCheck => ({ ok: false, field, message });
  const kind = offeredPromiseKind(draft.kind, offer);
  const body: MakePaymentPromiseRequest = { kind };
  if (kind === PromiseKind.specific_date) {
    if (draft.date === "") return invalid("date", PROMISE_DATE_REQUIRED_TEXT);
    if (draft.date < offer.earliest || draft.date > offer.latest) {
      return invalid(
        "date",
        promiseDateRangeText(offer.earliest, offer.latest),
      );
    }
    body.promisedDate = draft.date;
  }
  if (draft.amountText.trim() !== "") {
    const amount = parseRupiahInput(draft.amountText);
    if (amount === null || amount < 1) {
      return invalid("amountText", PROMISE_AMOUNT_MIN_TEXT);
    }
    const rest = promiseRest(offer, paying);
    if (amount > rest) {
      return invalid("amountText", promiseAmountMaxText(rest));
    }
    body.promisedAmount = amount;
  }
  if (paying !== null) body.payingNow = paying.amount;
  return { ok: true, body };
}

// The draft's fields, as the Janji form holds them.
export const promiseDraftShape = {
  kind: z.enum(PromiseKind),
  date: z.string(),
  amountText: z.string(),
};

// "Simpan janji": the request checkPromiseDraft makes, or its message under the field.
export function promiseSchema(
  offer: PromiseOfferView,
  paying: PromisePaying | null,
) {
  return z
    .object(promiseDraftShape)
    .transform((draft, ctx): MakePaymentPromiseRequest => {
      const checked = checkPromiseDraft(draft, offer, paying);
      if (checked.ok) return checked.body;
      ctx.addIssue({
        code: "custom",
        path: [checked.field],
        message: checked.message,
      });
      return z.NEVER;
    });
}

// Answers after which what the page shows is out of date: nothing is owed any more, the amount
// owed changed, the next invoice or the date range moved.
export const PROMISE_REFETCH_CODES: ReadonlySet<string> = new Set([
  "promise_nothing_owed",
  "promise_amount_above_outstanding",
  "promise_no_next_invoice",
  "promise_date_out_of_range",
]);

// The field an API error names, on this screen: the date only while its input shows; payingNow
// (the amount of "Bayar sebagian", not on this screen) and anything else in the alert.
function promiseErrorTarget(
  field: string | undefined,
  kind: PromiseKind,
): PromiseErrorTarget {
  switch (field) {
    case "kind":
      return "kind";
    case "promisedDate":
      return kind === PromiseKind.specific_date ? "promisedDate" : "alert";
    case "promisedAmount":
      return "promisedAmount";
    default:
      return "alert";
  }
}

// What a refused makePaymentPromise shows: a link that stopped working replaces the page; the
// API's message goes under the field it names or in the alert (a rate limit too, never retried);
// anything that is not the API's answer shows the generic message.
export function promiseErrorView(
  error: unknown,
  kind: PromiseKind,
): PromiseErrorView {
  if (isLinkError(error)) return { kind: "link-gone" };
  if (!(error instanceof ApiError)) {
    return {
      kind: "shown",
      error: { target: "alert", message: GENERIC_ERROR_MESSAGE },
      refetchPage: false,
    };
  }
  const target =
    error.status === 400 ? promiseErrorTarget(error.field, kind) : "alert";
  return {
    kind: "shown",
    error: { target, message: error.message },
    refetchPage: PROMISE_REFETCH_CODES.has(error.code),
  };
}

// The form field each place of promiseErrorView is shown in; null for the alert.
const PROMISE_ERROR_FIELDS: Record<
  PromiseErrorTarget,
  (typeof PROMISE_FORM_API_FIELDS)[number] | null
> = {
  kind: "kind",
  promisedDate: "date",
  promisedAmount: "amountText",
  alert: null,
};

// Where the Janji form shows a refused makePaymentPromise of this kind, as promiseErrorView
// places it. A link that stopped working replaces the page, so it never reaches the form.
export function promiseFormError(error: unknown, kind: PromiseKind): FormError {
  const view = promiseErrorView(error, kind);
  if (view.kind !== "shown") {
    return { field: null, message: GENERIC_ERROR_MESSAGE };
  }
  return {
    field: PROMISE_ERROR_FIELDS[view.error.target],
    message: view.error.message,
  };
}

// After "Simpan janji": paying now goes on to the chosen amount's QR (the caller starts it), with
// the promise on its card; otherwise Janji tercatat. An edit from Janji tercatat keeps what this
// visit paid.
export function promiseSaved(
  mode: PayFlowMode,
  saved: PaymentPromise,
): PayFlowMode {
  if (mode.kind !== "promise") return mode;
  if (mode.paying !== null) {
    return {
      kind: "chosen",
      text: mode.paying.text,
      amount: mode.paying.amount,
      outstandingAfter: mode.paying.outstandingAfter,
      promise: saved,
    };
  }
  return {
    kind: "recorded",
    paidAmount: mode.back.kind === "recorded" ? mode.back.paidAmount : null,
    promise: saved,
  };
}

// Janji tercatat, from the page as last read and the promise just saved. No breakdown of what the
// payment paid: no API gives the real allocation.
export function promiseRecordedView(
  view: OpenPaymentPageView,
  mode: RecordedFlowMode,
): RecordedView {
  return {
    paidLine:
      mode.paidAmount === null
        ? null
        : `Pembayaran ${formatRupiah(mode.paidAmount)} sudah diterima. Terima kasih, ${view.tenantName}.`,
    outstandingLine:
      view.outstanding > 0
        ? `Sisa tagihan ${formatRupiah(view.outstanding)}`
        : null,
    date: formatLongDate(mode.promise.promisedDate),
    amount: promiseAmountText(mode.promise.promisedAmount),
  };
}

// Telegram (decision 42): a demo channel, offered only where the environment has a bot. A linked
// tenant gets reminders there instead of WhatsApp, and leaves with /stop in the bot.

export const TELEGRAM_TITLE = "Pengingat lewat Telegram";
export const TELEGRAM_TEXT =
  "Terima pengingat tagihan di Telegram, bukan di WhatsApp.";
export const TELEGRAM_LINK_TEXT = "Hubungkan Telegram";
export const TELEGRAM_OPEN_TEXT = "Buka Telegram";
export const TELEGRAM_OPEN_HINT =
  "Tekan Start di Telegram. Link ini berlaku 10 menit.";
export const TELEGRAM_LINKED_TEXT =
  "Telegram terhubung. Pengingat berikutnya dikirim di sini.";

export const TELEGRAM_UNAVAILABLE_CODE = "telegram_unavailable";
// The deep link's one-time code is valid 10 minutes (TelegramDeepLink in the contract).
export const TELEGRAM_CODE_LIFETIME_MS = 10 * 60_000;
// The bot's deep link, as the API answers it. Anything else is not opened.
const TELEGRAM_DEEP_LINK =
  /^https:\/\/t\.me\/[A-Za-z0-9_]+\?start=[A-Za-z0-9_-]+$/;

export function telegramOfferView(page: PaymentPage): TelegramOfferView | null {
  return page.telegram.available ? { linked: page.telegram.linked } : null;
}

// Only the bot's own https://t.me link is shown to open; the code in it is never logged.
export function isTelegramDeepLink(link: string): boolean {
  return TELEGRAM_DEEP_LINK.test(link);
}

// When a deep link asked for at askedAt stops being shown. Counted from the moment the request was
// sent, which is never after the API made the code, so the link never outlives it.
export function telegramLinkExpiresAt(askedAt: number): number {
  return askedAt + TELEGRAM_CODE_LIFETIME_MS;
}

// How long until the shown link goes back to "Hubungkan Telegram"; 0 once it has expired.
export function telegramLinkDelay(expiresAt: number, now: number): number {
  return Math.max(0, expiresAt - now);
}

export function telegramErrorView(error: unknown): TelegramErrorView {
  if (error instanceof ApiError && error.code === TELEGRAM_UNAVAILABLE_CODE) {
    return { kind: "hidden" };
  }
  if (isLinkError(error)) return { kind: "link-gone" };
  if (error instanceof ApiError && error.status === 429) {
    return { kind: "alert", message: error.message };
  }
  return { kind: "alert", message: GENERIC_ERROR_MESSAGE };
}
