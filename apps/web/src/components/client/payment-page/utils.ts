import { invoiceName } from "@/components/client/billing/utils";
import { ApiError, INVALID_PATH_CODE } from "@/lib/api/errors";
import type {
  PaymentPage,
  PaymentPageInvoice,
} from "@/lib/api/generated/model";
import { formatMonthYear } from "@/lib/format/date";
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
export const QRIS_SOON_TEXT = "Pembayaran QRIS segera tersedia.";
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
// working (revoked, expired) replaces it.
export function paymentPageState(input: {
  wellFormed: boolean;
  data: PaymentPage | undefined;
  error: unknown;
}): PaymentPageState {
  if (!input.wellFormed) return { kind: "error", error: "not-found" };
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
