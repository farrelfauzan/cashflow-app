import type { QueryKey } from "@tanstack/react-query";
import { z } from "zod";

import {
  checkPromiseDraft,
  defaultPromiseDraft,
  prefilledPromiseDraft,
  PROMISE_REFETCH_CODES,
  promiseAmountText,
  promiseDraftShape,
  PROMISE_ALL_REST_TEXT,
  promiseFormView,
} from "@/components/client/payment-page/utils";
import type { PromiseOfferView } from "@/components/client/payment-page/type";
import type { ToastMessage } from "@/components/client/toast/type";
import { ApiError } from "@/lib/api/errors";
import { toFormError, type FormError } from "@/lib/api/form-error";
import type {
  AccountInvoice,
  AccountPayment,
  ActivePromise,
  Correction,
  CorrectionInput,
  InvoiceKind,
  MakeTenantPromiseRequest,
  PaymentMethod,
  PromiseSource,
  PromiseStatus,
  StaffRef,
  TenantAccount,
  TenantPromise,
  TenantPromiseList,
} from "@/lib/api/generated/model";
import { PromiseKind } from "@/lib/api/generated/model";
import {
  formatDate,
  formatDayMonth,
  formatMonth,
  formatMonthYear,
  formatTimestampDate,
  formatTimestampDayMonth,
  formatWeekdayDayMonth,
} from "@/lib/format/date";
import {
  formatRupiah,
  formatSignedRupiah,
  parseRupiahInput,
} from "@/lib/format/rupiah";
import type { AppAbility } from "@/lib/capabilities";

import type {
  AccountTileView,
  AllocationView,
  CorrectionDialogView,
  CorrectionRowView,
  CorrectionSelection,
  CorrectionTarget,
  InvoiceRowView,
  InvoiceStatusView,
  PaymentRowView,
  PromiseDialogView,
  PromiseHistoryRowView,
  PromiseStatusView,
} from "./type";

// The tenant's money on Detail penyewa. Every amount shown is a field the API sent, only
// formatted: nothing here adds, subtracts or derives rupiah. Counting, ordering and calendar
// arithmetic on dates are the only computations. A correction sends the new amount the member
// typed; the API takes the difference.

export const ACCOUNT_REGION_LABEL = "Uang penyewa";
export const ACCOUNT_LOADING_TEXT = "Memuat tagihan…";
export const ACCOUNT_FORBIDDEN_TEXT =
  "Anda tidak bisa melihat tagihan penyewa ini.";

export const INVOICES_TITLE = "Tagihan";
export const INVOICES_LIST_LABEL = "Daftar tagihan";
export const NO_INVOICES_TEXT = "Belum ada tagihan.";

export const PAYMENTS_TITLE = "Pembayaran dan alokasi";
export const PAYMENTS_LIST_LABEL = "Daftar pembayaran";
export const NO_PAYMENTS_TEXT = "Belum ada pembayaran.";
export const ALL_CREDIT_TEXT = "Semua masuk ke saldo lebih bayar";

// The final invoice (made at move-out for metered usage no regular invoice billed) is named
// "Tagihan akhir" wherever an invoice is named by its month. Its period is the lease's last day,
// so its month is the month the tenant left.
export const FINAL_INVOICE_LABEL = "Tagihan akhir";

type NamedInvoice = { kind?: InvoiceKind; period: string };

// "Tagihan November 2026", or "Tagihan akhir Oktober 2026" for the final invoice.
export function invoiceName(invoice: NamedInvoice): string {
  const month = formatMonthYear(invoice.period);
  return invoice.kind === "final"
    ? `${FINAL_INVOICE_LABEL} ${month}`
    : `Tagihan ${month}`;
}

// The short name inside a sentence ("sisa November", "→ November Rp700.000"): the month alone, or
// "tagihan akhir" for the final invoice, which no month would tell apart from that month's regular
// invoice.
function shortName(invoice: NamedInvoice): string {
  return invoice.kind === "final"
    ? "tagihan akhir"
    : formatMonth(invoice.period);
}

// The title of a row in "Tagihan": the month alone ("November 2026") for a regular invoice,
// "Tagihan akhir Oktober 2026" for the final one.
export function invoiceTitle(invoice: NamedInvoice): string {
  return invoice.kind === "final"
    ? invoiceName(invoice)
    : formatMonthYear(invoice.period);
}

export const CORRECTIONS_TITLE = "Koreksi nominal";
export const CORRECTIONS_LIST_LABEL = "Daftar koreksi";
export const NO_CORRECTIONS_TEXT = "Koreksi nominal: belum ada.";

export const NO_DEBT_TEXT = "Tidak ada tunggakan";
export const CREDIT_NOTE = "Dipakai otomatis untuk tagihan berikutnya";

export const PAYMENT_METHOD_LABELS: Record<PaymentMethod, string> = {
  cash: "Tunai",
  transfer: "Transfer",
  qris: "QRIS",
  va: "Virtual account",
};

const DAY_MS = 86_400_000;

// A calendar date (YYYY-MM-DD) as midnight UTC, so whole days between two dates are exact.
function dayNumber(date: string): number {
  const [year = 0, month = 1, day = 1] = date.split("-").map(Number);
  return Date.UTC(year, month - 1, day) / DAY_MS;
}

// Whole calendar days from `from` to `to` (both YYYY-MM-DD, Jakarta days); negative when `to`
// is earlier.
export function daysBetween(from: string, to: string): number {
  return dayNumber(to) - dayNumber(from);
}

// Whole calendar months from `from` to `to`: 10 Okt to 9 Nov is 0, to 10 Nov is 1.
export function monthsBetween(from: string, to: string): number {
  const [fy = 0, fm = 1, fd = 1] = from.split("-").map(Number);
  const [ty = 0, tm = 1, td = 1] = to.split("-").map(Number);
  return (ty - fy) * 12 + (tm - fm) - (td < fd ? 1 : 0);
}

// An open or partial invoice is debt from the day after its due date (Asia/Jakarta). `today` is
// the Jakarta day; calendar dates compare as strings.
export function isOverdue(invoice: AccountInvoice, today: string): boolean {
  return (
    (invoice.status === "open" || invoice.status === "partial") &&
    invoice.dueDate < today
  );
}

// The overdue invoices in the API's order, oldest period first.
export function overdueInvoices(
  invoices: readonly AccountInvoice[],
  today: string,
): AccountInvoice[] {
  return invoices.filter((invoice) => isOverdue(invoice, today));
}

// "sisa November", "sisa Oktober dan November", "sisa 3 tagihan"; null when none is overdue.
export function overdueNote(
  invoices: readonly AccountInvoice[],
  today: string,
): string | null {
  const overdue = overdueInvoices(invoices, today);
  const [first, second] = overdue;
  if (first === undefined) return null;
  if (overdue.length >= 3) return `sisa ${overdue.length} tagihan`;
  if (second === undefined) return `sisa ${shortName(first)}`;
  return `sisa ${shortName(first)} dan ${shortName(second)}`;
}

// How long the oldest unpaid invoice has been due: "N hari" under 31 days, then whole months
// ("1 bulan"). Debt starts the day after the due date, so it is at least 1 day.
export function debtAgeText(dueDate: string, today: string): string {
  const days = Math.max(1, daysBetween(dueDate, today));
  if (days < 31) return `${days} hari`;
  return `${Math.max(1, monthsBetween(dueDate, today))} bulan`;
}

// The four tiles, from the account's totals and its active promise. The overdue months and the
// oldest overdue period name invoices, but every amount is one the API sent.
export function accountTiles(
  account: TenantAccount,
  today: string,
): AccountTileView[] {
  const inDebt = account.overdue > 0;
  const debtNotes: string[] = [];
  if (inDebt) {
    const note = overdueNote(account.invoices, today);
    if (note !== null) debtNotes.push(note);
  } else {
    debtNotes.push(NO_DEBT_TEXT);
  }
  if (account.outstanding > account.overdue) {
    debtNotes.push(`Total belum dibayar ${formatRupiah(account.outstanding)}`);
  }

  const oldest = overdueInvoices(account.invoices, today)[0];
  const age =
    inDebt && account.oldestUnpaidDueDate !== undefined
      ? debtAgeText(account.oldestUnpaidDueDate, today)
      : "—";
  const ageNotes = !inDebt
    ? [NO_DEBT_TEXT]
    : oldest === undefined
      ? []
      : [invoiceTitle(oldest)];

  return [
    {
      label: "Tunggakan",
      value: formatRupiah(account.overdue),
      notes: debtNotes,
      tone: inDebt ? "warning" : "default",
    },
    { label: "Tunggakan tertua", value: age, notes: ageNotes, tone: "default" },
    activePromiseTile(account),
    {
      label: "Saldo lebih bayar",
      value: formatRupiah(account.credit),
      notes: [CREDIT_NOTE],
      tone: "default",
    },
  ];
}

// The chip of an invoice. The due date itself is not late: debt starts the day after.
export function invoiceStatusView(
  invoice: AccountInvoice,
  today: string,
): InvoiceStatusView {
  switch (invoice.status) {
    case "paid":
      return { label: "Lunas", tone: "active" };
    case "cancelled":
      return { label: "Dibatalkan", tone: "ended" };
    case "open":
    case "partial":
      if (isOverdue(invoice, today)) {
        return {
          label: `Terlambat ${daysBetween(invoice.dueDate, today)} hari`,
          tone: "warning",
        };
      }
      return invoice.status === "partial"
        ? { label: "Sebagian", tone: "neutral" }
        : { label: "Belum jatuh tempo", tone: "upcoming" };
  }
}

// Newest period first: the API sends oldest first.
export function orderedInvoices(
  invoices: readonly AccountInvoice[],
): AccountInvoice[] {
  return [...invoices].reverse();
}

// -1 when `a` came after `b`: received later, or on the same day and recorded later.
function newerFirst(a: AccountPayment, b: AccountPayment): number {
  if (a.receivedOn !== b.receivedOn)
    return a.receivedOn > b.receivedOn ? -1 : 1;
  return Date.parse(b.createdAt) - Date.parse(a.createdAt);
}

// Newest received first; payments received the same day, the one recorded last first.
export function orderedPayments(
  payments: readonly AccountPayment[],
): AccountPayment[] {
  return [...payments].sort(newerFirst);
}

function allocatesTo(payment: AccountPayment, invoiceId: string): boolean {
  return payment.allocations.some(
    (allocation) => allocation.invoiceId === invoiceId,
  );
}

// How many payments pay part of an invoice ("2 pembayaran").
export function paymentCount(
  invoiceId: string,
  payments: readonly AccountPayment[],
): number {
  return payments.filter((payment) => allocatesTo(payment, invoiceId)).length;
}

// The payment that settled a paid invoice: the newest one allocating to it. Null for an invoice
// that is not paid, or that no payment allocates to.
export function settlingPaymentId(
  invoice: AccountInvoice,
  payments: readonly AccountPayment[],
): string | null {
  if (invoice.status !== "paid") return null;
  const newest = orderedPayments(payments).find((payment) =>
    allocatesTo(payment, invoice.id),
  );
  return newest?.id ?? null;
}

function correctedFrom(current: number, original: number): string | null {
  return current === original
    ? null
    : `dikoreksi dari ${formatRupiah(original)}`;
}

function invoiceDetails(
  invoice: AccountInvoice,
  payments: readonly AccountPayment[],
): string[] {
  switch (invoice.status) {
    case "paid":
      return [`${paymentCount(invoice.id, payments)} pembayaran`];
    case "partial":
      return [
        `dibayar ${formatRupiah(invoice.paid)}`,
        `Sisa ${formatRupiah(invoice.outstanding)}`,
      ];
    case "open":
      return [`Sisa ${formatRupiah(invoice.outstanding)}`];
    case "cancelled":
      return [];
  }
}

// The rows of "Tagihan", newest period first. A cancelled invoice shows what it first billed,
// struck through. The unit is named only when the tenant's invoices come from several leases.
export function invoiceRows(
  account: TenantAccount,
  today: string,
): InvoiceRowView[] {
  const severalLeases =
    new Set(account.invoices.map((invoice) => invoice.leaseId)).size > 1;
  return orderedInvoices(account.invoices).map((invoice) => {
    const cancelled = invoice.status === "cancelled";
    return {
      id: invoice.id,
      title: invoiceTitle(invoice),
      place: severalLeases
        ? `${invoice.unitLabel} · ${invoice.propertyName}`
        : null,
      due: `jatuh tempo ${formatDayMonth(invoice.dueDate)}`,
      amount: formatRupiah(cancelled ? invoice.amount : invoice.billed),
      struck: cancelled,
      correctedFrom: cancelled
        ? null
        : correctedFrom(invoice.billed, invoice.amount),
      status: invoiceStatusView(invoice, today),
      details: invoiceDetails(invoice, account.payments),
      lines: invoice.lines.map((line) => ({
        id: line.id,
        label: line.label,
        amount: formatRupiah(line.amount),
      })),
      correctionTitle: invoiceCorrectionTitle(invoice),
    };
  });
}

function allocationViews(
  payment: AccountPayment,
  invoices: readonly AccountInvoice[],
  settledBy: ReadonlyMap<string, string | null>,
): AllocationView[] {
  return payment.allocations.map((allocation) => {
    const invoice = invoices.find((each) => each.id === allocation.invoiceId);
    const month = invoice === undefined ? "tagihan" : shortName(invoice);
    const settled = settledBy.get(allocation.invoiceId) === payment.id;
    return {
      invoiceId: allocation.invoiceId,
      text: `→ ${month} ${formatRupiah(allocation.amount)}${settled ? " (lunas)" : ""}`,
    };
  });
}

// The rows of "Pembayaran dan alokasi", newest first. "(lunas)" marks the allocation of the
// payment that settled a paid invoice.
export function paymentRows(account: TenantAccount): PaymentRowView[] {
  const settledBy = new Map(
    account.invoices.map((invoice) => [
      invoice.id,
      settlingPaymentId(invoice, account.payments),
    ]),
  );
  return orderedPayments(account.payments).map((payment) => ({
    id: payment.id,
    title: `${formatDate(payment.receivedOn)} · ${PAYMENT_METHOD_LABELS[payment.method]}`,
    amount: formatRupiah(payment.net),
    correctedFrom: correctedFrom(payment.net, payment.amount),
    note: payment.note === "" ? null : payment.note,
    recordedBy:
      payment.recordedBy === undefined
        ? null
        : `dicatat ${payment.recordedBy.name}`,
    allocations: allocationViews(payment, account.invoices, settledBy),
    correctionTitle: paymentCorrectionTitle(payment),
  }));
}

// Every correction of the tenant's invoices and payments, newest first.
export function correctionRows(account: TenantAccount): CorrectionRowView[] {
  const gathered: { correction: Correction; subject: string }[] = [
    ...account.invoices.flatMap((invoice) =>
      invoice.corrections.map((correction) => ({
        correction,
        subject: invoiceName(invoice),
      })),
    ),
    ...account.payments.flatMap((payment) =>
      payment.corrections.map((correction) => ({
        correction,
        subject: `Pembayaran ${formatDate(payment.receivedOn)}`,
      })),
    ),
  ];
  return gathered
    .sort(
      (a, b) =>
        Date.parse(b.correction.createdAt) - Date.parse(a.correction.createdAt),
    )
    .map(({ correction, subject }) => ({
      id: correction.id,
      title: `${formatTimestampDate(correction.createdAt)} · ${subject}`,
      amount: formatSignedRupiah(correction.amountDelta),
      reason: correction.reason,
      by: `oleh ${correction.createdBy.name}`,
    }));
}

// A member limited to some properties, where the tenant has leases in others, is refused
// (code forbidden): the totals span every lease. Clients branch on the code, not the status.
export function isAccountForbidden(error: unknown): boolean {
  return error instanceof ApiError && error.code === "forbidden";
}

// The money section shows only to a member who may read invoices and payments, on some records
// or all ("conditional" shows it and the API answers 403 per tenant). Never by role name.
export function canReadAccount(ability: AppAbility): boolean {
  return ability.can("read", "invoice") && ability.can("read", "payment");
}

// The card look of the tenant screens (lease-card.tsx).
export const ACCOUNT_CARD_CLASS =
  "flex min-w-0 flex-col gap-3 rounded-[14px] border bg-card p-4 md:gap-4 md:rounded-2xl md:px-6 md:py-[22px]";

// "Koreksi nominal" (board "iPad · Koreksi nominal"): a "Koreksi" action on each invoice and
// payment opens a dialog that sends the new amount and a reason.

export const CORRECT_BUTTON_TEXT = "Koreksi";
export const CORRECTION_NOTE =
  "Data lama tidak dihapus. Koreksi ini tercatat atas nama Anda dan muncul di laporan Supervisor.";

// "Koreksi tagihan November 2026": the invoice's period month; "Koreksi tagihan akhir Oktober
// 2026" for the final invoice. Without a kind (a billed meter amount names only the period) the
// invoice reads as a regular one.
export function invoiceCorrectionTitle(invoice: NamedInvoice): string {
  const name = invoiceName(invoice);
  return `Koreksi ${name.charAt(0).toLowerCase()}${name.slice(1)}`;
}

// "Koreksi pembayaran 25 Nov 2026": the day the payment was received.
export function paymentCorrectionTitle(
  payment: Pick<AccountPayment, "receivedOn">,
): string {
  return `Koreksi pembayaran ${formatDate(payment.receivedOn)}`;
}

// The muted line under the title: the tenant, and for an invoice its unit and property.
export function invoiceCorrectionSubtitle(
  invoice: Pick<AccountInvoice, "unitLabel" | "propertyName">,
  tenantName: string,
): string {
  return `${tenantName} · ${invoice.unitLabel} · ${invoice.propertyName}`;
}

// The dialog for what "Koreksi" was pressed on, from the latest account: the current amount is
// what the invoice bills now (billed) or what the payment counts now (net), as the API sent it.
// Null when the record is no longer in the account.
export function correctionDialogView(
  account: TenantAccount,
  selection: CorrectionSelection,
  tenantName: string,
): CorrectionDialogView | null {
  if (selection.kind === "invoice") {
    const invoice = account.invoices.find((each) => each.id === selection.id);
    if (invoice === undefined) return null;
    return {
      target: { invoiceId: invoice.id },
      title: invoiceCorrectionTitle(invoice),
      subtitle: invoiceCorrectionSubtitle(invoice, tenantName),
      current: formatRupiah(invoice.billed),
    };
  }
  const payment = account.payments.find((each) => each.id === selection.id);
  if (payment === undefined) return null;
  return {
    target: { paymentId: payment.id },
    title: paymentCorrectionTitle(payment),
    subtitle: tenantName,
    current: formatRupiah(payment.net),
  };
}

// The short confirmation after a correction is saved.
export function correctionSavedNotice(title: string): string {
  return `${title} disimpan.`;
}

// The "Koreksi" action shows for a member who may create corrections on some records or all;
// "conditional" shows it and the API answers 403 per record. Never by role name.
export function canCreateCorrection(ability: AppAbility): boolean {
  return ability.can("create", "correction");
}

export const EMPTY_NEW_AMOUNT_MESSAGE = "Isi nominal baru.";
export const DIGITS_ONLY_MESSAGE = "Hanya angka.";
// CorrectionInput.newAmount's maximum.
export const MAX_NEW_AMOUNT = 1_000_000_000_000;
export const NEW_AMOUNT_TOO_LARGE_MESSAGE = `Nominal paling besar ${formatRupiah(MAX_NEW_AMOUNT)}.`;
export const EMPTY_REASON_MESSAGE = "Alasan wajib diisi.";
export const REASON_MAX_LENGTH = 500;
export const REASON_TOO_LONG_MESSAGE = "Alasan paling banyak 500 karakter.";

// "Nominal baru" as typed (digits, grouped with dots by the field) becomes integer rupiah from 0
// (cancels an invoice, zeroes a payment) up to MAX_NEW_AMOUNT; the reason is trimmed and
// required. Whether the amount equals the current one is the API's to say
// (correction_no_change).
export const correctionFormSchema = z.object({
  newAmount: z.string().transform((text, ctx) => {
    if (text.trim() === "") {
      ctx.addIssue({ code: "custom", message: EMPTY_NEW_AMOUNT_MESSAGE });
      return z.NEVER;
    }
    const amount = parseRupiahInput(text);
    if (amount === null) {
      ctx.addIssue({ code: "custom", message: DIGITS_ONLY_MESSAGE });
      return z.NEVER;
    }
    if (amount > MAX_NEW_AMOUNT) {
      ctx.addIssue({ code: "custom", message: NEW_AMOUNT_TOO_LARGE_MESSAGE });
      return z.NEVER;
    }
    return amount;
  }),
  reason: z
    .string()
    .trim()
    .min(1, { error: EMPTY_REASON_MESSAGE, abort: true })
    .max(REASON_MAX_LENGTH, { error: REASON_TOO_LONG_MESSAGE }),
});

// The API fields the dialog shows under its own fields.
export const CORRECTION_FORM_API_FIELDS = ["newAmount", "reason"] as const;

// Where the dialog shows a refused correction. correction_below_zero names amountDelta, which the
// dialog never sends: it is about the new amount, so it goes under "Nominal baru". Any other
// field (requestId, invoiceId, paymentId) or none goes to the dialog's alert.
export function correctionFormError(error: unknown): FormError {
  const failed = toFormError(error, [
    ...CORRECTION_FORM_API_FIELDS,
    "amountDelta",
  ]);
  return failed.field === "amountDelta"
    ? { field: "newAmount", message: failed.message }
    : failed;
}

// The request body: the record, the new amount and the reason. Never amountDelta.
export function correctionInput(
  requestId: string,
  target: CorrectionTarget,
  values: { newAmount: number; reason: string },
): CorrectionInput {
  return {
    requestId,
    ...target,
    newAmount: values.newAmount,
    reason: values.reason,
  };
}

// Any query of Penagihan (the organization's invoices, a list or one invoice): a correction
// changes what they show.
export function isInvoicesQuery(queryKey: QueryKey, orgId: string): boolean {
  const [scope, keyOrgId, url] = queryKey;
  return (
    scope === "org" &&
    keyOrgId === orgId &&
    typeof url === "string" &&
    (url === `/orgs/${orgId}/invoices` ||
      url.startsWith(`/orgs/${orgId}/invoices/`))
  );
}

// Janji pelunasan on Detail penyewa: the "Janji aktif" tile, "Riwayat janji" and "Catat janji"
// (no board for its form: it follows the tenant's Janji screen, built from the same helpers).
// Dates compare as YYYY-MM-DD strings and amounts are only compared; the API checks everything.

export const ACTIVE_PROMISE_LABEL = "Janji aktif";
export const NO_ACTIVE_PROMISE_TEXT = "Tidak ada janji aktif";

// "dibuat penyewa" for a promise the tenant made on the payment page, "dicatat Pak Hendra Gunawan"
// for one a member recorded; null when the API names nobody.
export function promiseByText(
  source: PromiseSource,
  createdBy: StaffRef | null,
): string | null {
  if (source === "tenant_page") return "dibuat penyewa";
  return createdBy === null ? null : `dicatat ${createdBy.name}`;
}

// "Janji lewat 2× (sejak awal)": every promise the tenant ever broke; null for none.
export function brokenPromisesNote(count: number): string | null {
  return count > 0 ? `Janji lewat ${count}× (sejak awal)` : null;
}

// "Janji aktif": the promised day, then what and who ("Rp1.250.000 · dibuat penyewa"), the
// member's note, and the broken count as history.
export function activePromiseTile(account: TenantAccount): AccountTileView {
  const broken = brokenPromisesNote(account.brokenPromises);
  const promise = account.activePromise;
  const notes: string[] = [];
  if (promise === null) {
    notes.push(NO_ACTIVE_PROMISE_TEXT);
  } else {
    const by = promiseByText(promise.source, promise.createdBy);
    const amount = promiseAmountText(promise.promisedAmount);
    notes.push(by === null ? amount : `${amount} · ${by}`);
    if (promise.note !== null && promise.note !== "") notes.push(promise.note);
  }
  if (broken !== null) notes.push(broken);
  return {
    label: ACTIVE_PROMISE_LABEL,
    value: promise === null ? "—" : formatWeekdayDayMonth(promise.promisedDate),
    notes,
    tone: "default",
  };
}

export const PROMISE_HISTORY_TITLE = "Riwayat janji";
export const PROMISE_HISTORY_LIST_LABEL = "Daftar janji";
export const NO_PROMISES_TEXT = "Belum ada janji.";
export const PROMISES_LOADING_TEXT = "Memuat janji…";
export const PROMISES_FORBIDDEN_TEXT =
  "Anda tidak bisa melihat janji penyewa ini.";
// "Riwayat janji" shows the first page only.
export const PROMISE_HISTORY_PAGE_SIZE = 20;

export const PROMISE_STATUS_VIEWS: Record<PromiseStatus, PromiseStatusView> = {
  active: { label: "Aktif", tone: "upcoming" },
  kept: { label: "Ditepati", tone: "active" },
  partially_kept: { label: "Ditepati sebagian", tone: "neutral" },
  broken: { label: "Tidak ditepati", tone: "warning" },
};

// The rows of "Riwayat janji" in the API's order (newest first), each from its newest version:
// when and how much, who made it and when, and how often it was changed.
export function promiseHistoryRows(
  promises: readonly TenantPromise[],
): PromiseHistoryRowView[] {
  return promises.flatMap((promise) => {
    const newest = promise.versions.at(-1);
    if (newest === undefined) return [];
    const by = promiseByText(newest.source, newest.createdBy);
    const day = formatTimestampDayMonth(newest.createdAt);
    const edits = promise.versions.length - 1;
    const made = by === null ? `dicatat ${day}` : `${by} ${day}`;
    return [
      {
        id: promise.id,
        title: `${formatWeekdayDayMonth(newest.promisedDate)} · ${promiseAmountText(newest.promisedAmount)}`,
        byline: edits > 0 ? `${made} · diubah ${edits}×` : made,
        status: PROMISE_STATUS_VIEWS[promise.status],
      },
    ];
  });
}

// "Menampilkan 20 dari 23" when the first page does not hold every promise; null when it does.
export function promiseHistoryCountNote(
  list: TenantPromiseList,
): string | null {
  return list.total > list.items.length
    ? `Menampilkan ${list.items.length} dari ${list.total}`
    : null;
}

// "Riwayat janji" shows for a member who may read promises, on some tenants or all. Never by role
// name.
export function canReadPromises(ability: AppAbility): boolean {
  return ability.can("read", "promise");
}

// "Catat janji" shows for a member who may create promises, on some tenants or all
// ("conditional" shows it and the API answers 403 per tenant). Never by role name.
export function canCreatePromise(ability: AppAbility): boolean {
  return ability.can("create", "promise");
}

export const RECORD_PROMISE_TEXT = "Catat janji";
export const PROMISE_SAVED_NOTICE = "Janji tercatat.";
export const PROMISE_NOTE_LABEL = "Catatan (tidak wajib)";
export const PROMISE_NOTE_MAX_LENGTH = 500;
export const PROMISE_NOTE_TOO_LONG_MESSAGE =
  "Catatan paling banyak 500 karakter.";

// What a promise recorded now may name, from the account as the API sent it; null when none can
// be made (nothing is owed, or the active promise reached its last day).
export function staffPromiseOffer(
  account: TenantAccount,
): PromiseOfferView | null {
  if (account.promiseDates === null) return null;
  return {
    outstanding: account.outstanding,
    earliest: account.promiseDates.earliest,
    latest: account.promiseDates.latest,
    nextInvoiceDueDate: account.nextInvoiceDueDate,
  };
}

// "Janji aktif Sab, 12 Des, Rp1.250.000 akan diganti.": saving adds a version to the active
// promise.
export function activePromiseReplacesLine(promise: ActivePromise): string {
  const amount =
    promise.promisedAmount === null
      ? PROMISE_ALL_REST_TEXT.toLowerCase()
      : formatRupiah(promise.promisedAmount);
  return `${ACTIVE_PROMISE_LABEL} ${formatWeekdayDayMonth(promise.promisedDate)}, ${amount} akan diganti.`;
}

// "Catat janji" for the account as it is when the dialog opens: empty, or filled from the active
// promise (its note is not carried over: each version has its own). Null when no promise can be
// made now.
export function promiseDialogView(
  account: TenantAccount,
  tenantName: string,
): PromiseDialogView | null {
  const offer = staffPromiseOffer(account);
  if (offer === null) return null;
  const active = account.activePromise;
  const form = promiseFormView(offer, null, active !== null, null);
  const draft =
    active === null
      ? defaultPromiseDraft(offer)
      : prefilledPromiseDraft(active, offer);
  return {
    subtitle: tenantName,
    replacesLine: active === null ? null : activePromiseReplacesLine(active),
    offer,
    kindOptions: form.kindOptions,
    dateMin: form.dateMin,
    dateMax: form.dateMax,
    amountHint: form.amountHint,
    primaryLabel: form.primaryLabel,
    defaults: { ...draft, note: "" },
  };
}

// "Simpan janji": the tenant's Janji checks (date in range, amount from Rp1 to what is owed) plus
// the note, trimmed, at most 500 letters, sent only when not blank. Never payingNow.
export function staffPromiseSchema(offer: PromiseOfferView) {
  return z
    .object({ ...promiseDraftShape, note: z.string() })
    .transform((values, ctx): MakeTenantPromiseRequest => {
      const note = values.note.trim();
      if (note.length > PROMISE_NOTE_MAX_LENGTH) {
        ctx.addIssue({
          code: "custom",
          path: ["note"],
          message: PROMISE_NOTE_TOO_LONG_MESSAGE,
        });
      }
      const checked = checkPromiseDraft(values, offer, null);
      if (!checked.ok) {
        ctx.addIssue({
          code: "custom",
          path: [checked.field],
          message: checked.message,
        });
        return z.NEVER;
      }
      const { kind, promisedDate, promisedAmount } = checked.body;
      const body: MakeTenantPromiseRequest = { kind };
      if (promisedDate !== undefined) body.promisedDate = promisedDate;
      if (promisedAmount !== undefined) body.promisedAmount = promisedAmount;
      if (note !== "") body.note = note;
      return body;
    });
}

// The dialog's fields an API error can go under.
export const STAFF_PROMISE_FORM_FIELDS = [
  "kind",
  "date",
  "amountText",
  "note",
] as const;

// Each API field the dialog shows, by the form field it goes under.
const STAFF_PROMISE_API_FIELDS: Record<
  string,
  (typeof STAFF_PROMISE_FORM_FIELDS)[number]
> = {
  kind: "kind",
  promisedDate: "date",
  promisedAmount: "amountText",
  note: "note",
};

// Where "Catat janji" shows a refused makeTenantPromise sent with this kind: under the field the
// API names (the date only while its input shows), else in the alert (promise_nothing_owed, a
// refusal, anything unexpected).
export function staffPromiseFormError(
  error: unknown,
  kind: PromiseKind,
): FormError {
  const failed = toFormError(error, Object.keys(STAFF_PROMISE_API_FIELDS));
  const field =
    failed.field === null ? undefined : STAFF_PROMISE_API_FIELDS[failed.field];
  if (
    field === undefined ||
    (field === "date" && kind !== PromiseKind.specific_date)
  ) {
    return { field: null, message: failed.message };
  }
  return { field, message: failed.message };
}

// A refusal after which the account is out of date (nothing owed now, the amount owed, the next
// invoice or the dates moved): the page reads it again, the dialog keeps its message.
export function isPromiseStaleError(error: unknown): boolean {
  return error instanceof ApiError && PROMISE_REFETCH_CODES.has(error.code);
}

// The toasts once the API saved (board "Notifikasi singkat"). A correction changes an amount
// already billed or received, and a changed promise a date or amount already agreed: orange.
export function correctionSavedToast(title: string): ToastMessage {
  return { tone: "caution", title: "Koreksi disimpan", description: title };
}

// `replaces`: the tenant had an active promise, which the saved one replaces.
export function promiseSavedToast(
  replaces: boolean,
  tenantName: string,
): ToastMessage {
  return replaces
    ? { tone: "caution", title: "Janji bayar diubah", description: tenantName }
    : { tone: "create", title: "Janji bayar dicatat", description: tenantName };
}
