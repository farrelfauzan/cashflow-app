import { invoiceName } from "@/components/client/billing/utils";
import { API_BASE_PATH } from "@/lib/api/client";
import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  INVALID_PATH_CODE,
} from "@/lib/api/errors";
import type {
  PaymentPage,
  PaymentPageInvoice,
  QrisStatus,
} from "@/lib/api/generated/model";
import { formatDateTime, formatMonthYear } from "@/lib/format/date";
import { formatRupiah } from "@/lib/format/rupiah";

import type {
  CopyStatus,
  OpenPaymentPageView,
  PaidPaymentPageView,
  PaymentPageErrorKind,
  PaymentPageHeaderView,
  PaymentPageMessageView,
  PaymentPageState,
  PaymentPageView,
  PaymentTab,
  QrisCard,
  QrisDetailRow,
  QrisEvent,
  QrisModel,
  QrisOfferView,
  QrisStep,
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
export const QRIS_ABOVE_LIMIT_TEXT =
  "QRIS hanya sampai Rp10.000.000 sekali bayar. Pakai transfer bank untuk tagihan ini.";
export const QRIS_CREATING_TEXT = "Membuat QR…";
export const QRIS_HELP_TEXT =
  "Bayar dari HP ini? Unduh QR, lalu pilih dari galeri di aplikasi bank atau e-wallet.";
export const QRIS_DOWNLOAD_TEXT = "Unduh QR";
export const QRIS_PENDING_TEXT =
  "Menunggu pembayaran. Halaman ini berubah sendiri jadi Lunas.";
export const QRIS_NEW_TEXT = "Buat QR baru";
export const QRIS_PAID_CLAIM_TEXT = "Saya sudah bayar";
export const QRIS_CREATE_FAILED_TEXT = "QR belum bisa dibuat. Coba lagi.";

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

// "Bayar QRIS Rp2.250.000", or null when the total is above what one QR can take.
export function qrisStartLabel(outstanding: number): string | null {
  if (outstanding > QRIS_LIMIT) return null;
  return `Bayar QRIS ${formatRupiah(outstanding)}`;
}

// The amount to ask a QR for: the API's outstanding as is, or null when QRIS cannot take it
// (nothing owed, or above Rp10.000.000).
export function qrisCreateAmount(outstanding: number): number | null {
  if (outstanding < 1 || outstanding > QRIS_LIMIT) return null;
  return outstanding;
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
    downloadName: qrisDownloadName(page.invoice.unitLabel),
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
      return step(withCard(model, { kind: "idle" }), { refetchPage: true });
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
): QrisStep {
  if (model.checkRunning) return step(model);
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
        return step(withCard(model, { kind: "idle" }), { refetchPage: true });
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
    return step(withCard(model, { kind: "idle" }), { refetchPage: true });
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
  const done = { ...model, checkRunning: false };
  const fallback = cardAmount(model.card);
  switch (status.status) {
    case "checking":
      return step(withCard(done, checkingCard(status, fallback)));
    case "paid":
      return step(withCard(done, { kind: "idle" }), { refetchPage: true });
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
  const done = { ...model, checkRunning: false };
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
      return statusRead(model, event.status, event.now);
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
      return step({ ...model, checkRunning: true, alert: null });
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
