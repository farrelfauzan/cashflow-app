import { z } from "zod";

import type { ToastMessage } from "@/components/client/toast/type";
import { ApiError } from "@/lib/api/errors";
import type {
  CashDeposit,
  CashDepositInput,
  CashTotal,
  HeldCashPayment,
  ReceiptPhotoStatus,
} from "@/lib/api/generated/model";
import type { AppAbility } from "@/lib/capabilities";
import {
  formatTimestampWeekdayDayMonth,
  formatTimestampWeekdayDayMonthTime,
} from "@/lib/format/date";
import { formatRupiah, parseRupiahInput } from "@/lib/format/rupiah";
import { groupRupiahDigits } from "@/lib/format/rupiah-input";

import type {
  DepositChip,
  DepositErrorKind,
  DepositFormOutput,
  DepositFormValues,
  HeldReceiptView,
  PastDepositView,
  PendingDepositView,
} from "./type";

// "Setor tunai" (board HP-Setor): the karyawan hands the cash they hold to the office. Every
// amount shown is a field the API sent, only formatted; the one number typed here is what the
// karyawan says they hand over.

// The API's limit (CashDepositInput.amount).
export const DEPOSIT_AMOUNT_MAX = 1_000_000_000_000;

// One page of the receipts held (the API's maximum) and of the deposits for the history.
export const HELD_PAGE_SIZE = 100;
export const DEPOSITS_PAGE_SIZE = 10;

export const LOADING_TEXT = "Memuat…";
export const FORBIDDEN_TEXT = "Anda tidak punya akses ke setoran tunai.";
export const EMPTY_TEXT = "Belum ada tunai yang perlu disetor.";

export const AMOUNT_LABEL = "Jumlah yang Anda serahkan";
export const AMOUNT_HINT =
  "Terisi dari total kwitansi. Ubah kalau uang yang diserahkan berbeda.";
export const FOOTER_CAPTION = "Admin menghitung uangnya, lalu mengonfirmasi.";
export const RECEIPTS_LABEL = "Kwitansi yang belum disetor";

export const PENDING_BADGE = "Menunggu admin menghitung";
export const PENDING_NOTE =
  "Kwitansi yang Anda terima setelah ini masuk ke setoran berikutnya.";

export const HISTORY_TITLE = "Setoran sebelumnya";
export const GAP_NOTE =
  "Selisih dicatat admin di antrean cek. Kwitansi penyewa tidak berubah: penyewa tetap tercatat sudah bayar.";

export const EMPTY_AMOUNT_MESSAGE = "Isi jumlah yang Anda serahkan.";
export const AMOUNT_DIGITS_MESSAGE = "Hanya angka.";
export const AMOUNT_ZERO_MESSAGE = "Jumlah harus lebih dari Rp0.";
export const AMOUNT_TOO_LARGE_MESSAGE = `Jumlah paling banyak ${formatRupiah(DEPOSIT_AMOUNT_MAX)}.`;
export const CASH_CHANGED_MESSAGE = "Jumlah tunai berubah, periksa lagi.";

// The one field an API error can name on this form; any other (requestId, expectedAmount,
// expectedCount) goes to the alert.
export const DEPOSIT_API_FIELDS = ["amount"] as const;

const PHOTO_CHIPS: Record<ReceiptPhotoStatus, DepositChip> = {
  clean: { label: "Foto tersimpan", tone: "active" },
  pending: { label: "Foto sedang diperiksa", tone: "none" },
  missing: { label: "Foto belum ada", tone: "warning" },
  rejected: { label: "Foto ditolak, ambil ulang", tone: "warning" },
  infected: { label: "Foto ditolak, ambil ulang", tone: "warning" },
};

// The friendly early checks of the amount. The Go API checks it again and its message wins.
export const depositFormSchema = z.object({
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
    if (amount > DEPOSIT_AMOUNT_MAX) {
      ctx.addIssue({ code: "custom", message: AMOUNT_TOO_LARGE_MESSAGE });
      return z.NEVER;
    }
    return amount;
  }),
});

// The amount field's text for an amount from the API: "2.500.000".
export function amountFieldText(amount: number): string {
  return groupRupiahDigits(String(amount));
}

// The form starts at the total of the receipts held.
export function depositFormDefaults(cash: CashTotal): DepositFormValues {
  return { amount: amountFieldText(cash.amount) };
}

// The request body: the total and count the karyawan was shown, and what they hand over. One
// requestId per form, sent again unchanged on a retry.
export function depositRequestBody(
  fields: DepositFormOutput,
  cash: CashTotal,
  requestId: string,
): CashDepositInput {
  return {
    requestId,
    expectedAmount: cash.amount,
    expectedCount: cash.count,
    amount: fields.amount,
  };
}

// The primary button names the amount as typed: "Setor Rp2.500.000", or "Setor" while the field
// holds no amount.
export function submitLabel(text: string): string {
  const amount = parseRupiahInput(text);
  return amount === null ? "Setor" : `Setor ${formatRupiah(amount)}`;
}

export function receiptCountText(count: number): string {
  return `${count} kwitansi`;
}

// Above the total: since the newest deposit when there is one ("Tunai di tangan sejak setoran
// Kamis, 12 Nov"). `newest` is the first deposit of listMyCashDeposits (newest first).
export function cashSinceLine(
  newest: Pick<CashDeposit, "recordedAt"> | undefined,
) {
  if (newest === undefined) return "Tunai di tangan";
  return `Tunai di tangan sejak setoran ${formatTimestampWeekdayDayMonth(newest.recordedAt)}`;
}

// Under the list when the API holds more receipts than one page shows: "dan 3 kwitansi lainnya".
export function moreReceiptsText(total: number, shown: number): string | null {
  const more = total - shown;
  return more > 0 ? `dan ${more} kwitansi lainnya` : null;
}

export function photoChip(status: ReceiptPhotoStatus): DepositChip {
  return PHOTO_CHIPS[status];
}

export function heldReceiptView(payment: HeldCashPayment): HeldReceiptView {
  const units = payment.units.map((unit) => unit.unitLabel).join(", ");
  const recorded = formatTimestampWeekdayDayMonthTime(payment.recordedAt);
  return {
    paymentId: payment.paymentId,
    tenantName: payment.tenantName,
    detail: units === "" ? recorded : `${units} · ${recorded}`,
    amount: formatRupiah(payment.amount),
    photo: photoChip(payment.photo.status),
  };
}

export function pendingDeposits(
  deposits: readonly CashDeposit[],
): CashDeposit[] {
  return deposits.filter((deposit) => deposit.status === "pending");
}

export function pendingDepositView(deposit: CashDeposit): PendingDepositView {
  return {
    id: deposit.id,
    amount: formatRupiah(deposit.amount),
    detail: `Disetor ${formatTimestampWeekdayDayMonthTime(deposit.recordedAt)} · ${receiptCountText(deposit.paymentCount)}`,
  };
}

// "Cocok" when the counted cash matches, else "Selisih Rp50.000" (the size of the gap, either
// way).
export function resultChip(
  status: CashDeposit["status"],
  gap: number,
): DepositChip {
  if (status === "matched") return { label: "Cocok", tone: "active" };
  return { label: `Selisih ${formatRupiah(Math.abs(gap))}`, tone: "warning" };
}

// The counted deposits, newest first; a pending one (or one the API sent without its count) is
// left out.
export function pastDepositViews(
  deposits: readonly CashDeposit[],
): PastDepositView[] {
  return deposits.flatMap((deposit) => {
    const counted = deposit.confirmation;
    if (deposit.status === "pending" || counted === undefined) return [];
    const detail =
      deposit.status === "matched"
        ? `Dihitung ${counted.confirmedByName} · ${receiptCountText(deposit.paymentCount)}`
        : `Disetor ${formatRupiah(deposit.amount)} · dihitung ${formatRupiah(counted.receivedAmount)}`;
    return [
      {
        id: deposit.id,
        date: formatTimestampWeekdayDayMonth(deposit.recordedAt),
        detail,
        amount: formatRupiah(counted.receivedAmount),
        chip: resultChip(deposit.status, counted.gap),
      },
    ];
  });
}

export function hasGap(views: readonly PastDepositView[]): boolean {
  return views.some((view) => view.chip.tone === "warning");
}

export function depositErrorKind(error: unknown): DepositErrorKind {
  if (!(error instanceof ApiError)) return "other";
  if (error.code === "deposit_cash_changed") return "cash_changed";
  if (error.code === "nothing_to_deposit") return "nothing_to_deposit";
  // A deposit under this request id is already recorded (an answer lost, then an edited
  // amount): the screen reloads and shows it waiting.
  if (error.code === "request_id_reused") return "request_reused";
  return "other";
}

export function isForbidden(error: unknown): boolean {
  return error instanceof ApiError && error.status === 403;
}

// "Setor tunai" is for a member who may deposit their own cash ("conditional": the API checks
// the cash is theirs). Never by role name.
export function canDepositCash(ability: AppAbility): boolean {
  return ability.can("deposit", "cash");
}

export const DEPOSIT_RECORDED_TOAST: ToastMessage = {
  tone: "create",
  title: "Setoran dicatat",
  description: "Admin akan menghitungnya.",
};
