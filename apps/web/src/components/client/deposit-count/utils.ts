import { z } from "zod";

import {
  isForbidden,
  moreReceiptsText,
  photoChip,
  receiptCountText,
} from "@/components/client/deposit/utils";
import type { DepositChip } from "@/components/client/deposit/type";
import type { ToastMessage } from "@/components/client/toast/type";
import { ApiError } from "@/lib/api/errors";
import type {
  CashDeposit,
  CashDepositConfirmationInput,
  HeldCashPayment,
  ReceiptPhotoStatus,
} from "@/lib/api/generated/model";
import { everywhere, type AppAbility } from "@/lib/capabilities";
import {
  formatTimestampShortWeekdayDayMonthTime,
  formatTimestampWeekdayDayMonthTime,
} from "@/lib/format/date";
import { formatRupiah, parseRupiahInput } from "@/lib/format/rupiah";

import type {
  ConfirmErrorKind,
  CountAction,
  CountedDepositRowView,
  CountFormOutput,
  CountFormValues,
  CountHint,
  CountSheetView,
  DepositReceiptView,
  DepositTab,
  PendingDepositRowView,
} from "./type";

// "Setoran tunai" (board W20b): the admin counts the cash a karyawan handed over and records what
// they received. Every amount shown is a field the API sent, only formatted; the one number typed
// here is what the admin counted. The differences shown ("kurang Rp50.000") are display only:
// the gap that counts is the one the API saves.

export { isForbidden, moreReceiptsText };

export const TITLE = "Setoran tunai";
export const SUBTITLE =
  "Uang tunai yang diserahkan karyawan. Hitung uangnya, lalu catat jumlah yang Anda terima.";
export const BILLING_LABEL = "Penagihan";
export const BREADCRUMB_LABEL = "Lokasi halaman";

export const LOADING_TEXT = "Memuat…";
export const FORBIDDEN_TEXT = "Anda tidak punya akses ke setoran tunai.";

export const TAB_PARAM = "tab";
export const TABS_LABEL = "Setoran tunai";
export const PENDING_TAB_TEXT = "Menunggu dihitung";
export const CONFIRMED_TAB_TEXT = "Sudah dihitung";
export const PENDING_LIST_LABEL = "Setoran yang menunggu dihitung";
export const CONFIRMED_LIST_LABEL = "Setoran yang sudah dihitung";
export const PENDING_EMPTY_TEXT = "Belum ada setoran yang menunggu dihitung.";
export const CONFIRMED_EMPTY_TEXT = "Belum ada setoran yang sudah dihitung.";
export const CONFIRMED_CAPTION =
  "Terbaru di atas. Selisih ditutup dengan catatan di Antrean cek.";

export const COUNT_BUTTON = "Hitung";
export const OWN_DEPOSIT_TEXT = "Dihitung admin lain";

export const SHEET_TITLE = "Hitung setoran";
export const EXPECTED_LABEL = "Total kwitansi";
export const HANDED_LABEL = "Kata karyawan diserahkan";
export const RECEIPTS_TITLE = "Kwitansi dalam setoran ini";
export const RECEIVED_LABEL = "Uang yang Anda terima";
export const RECEIVED_HINT =
  "Ketik hasil hitungan Anda sendiri. Kolom ini sengaja kosong saat dibuka.";
export const NOTE_LABEL = "Catatan (opsional)";
export const CANCEL_LABEL = "Batal";
export const SAVE_LABEL = "Simpan hitungan";
export const SAVING_LABEL = "Menyimpan…";

// The entry on Penagihan's header.
export const DEPOSIT_LINK_TEXT = "Setoran tunai";

// The API's limits (CashDepositConfirmationInput).
export const RECEIVED_AMOUNT_MAX = 1_000_000_000_000;
export const NOTE_MAX_LENGTH = 500;

// One page of the payments in the drawer: the API's maximum, with "dan N kwitansi lainnya" under
// it when there are more.
export const RECEIPTS_PAGE_SIZE = 100;

export const EMPTY_RECEIVED_MESSAGE = "Isi jumlah uang yang Anda terima.";
export const RECEIVED_DIGITS_MESSAGE = "Hanya angka.";
export const RECEIVED_TOO_LARGE_MESSAGE = `Jumlah paling banyak ${formatRupiah(RECEIVED_AMOUNT_MAX)}.`;
export const NOTE_TOO_LONG_MESSAGE = `Catatan paling banyak ${NOTE_MAX_LENGTH} huruf.`;

// The fields an API error can name on this form; any other goes to the alert.
export const COUNT_API_FIELDS = ["receivedAmount", "note"] as const;

// The tables' columns on iPad and desktop, written out in full so Tailwind sees them. The pending
// table has the "Hitung" column only for a member who may count.
export const PENDING_COLUMNS_WITH_ACTION =
  "grid-cols-[minmax(0,1fr)_minmax(80px,0.4fr)_minmax(130px,0.7fr)_minmax(150px,0.8fr)_128px]";
export const PENDING_COLUMNS =
  "grid-cols-[minmax(0,1fr)_minmax(80px,0.4fr)_minmax(130px,0.7fr)_minmax(150px,0.8fr)]";
export const CONFIRMED_COLUMNS =
  "grid-cols-[minmax(190px,1.2fr)_minmax(120px,0.7fr)_minmax(120px,0.7fr)_minmax(150px,0.8fr)_minmax(0,0.9fr)]";

// The deposits are counted only by a member who may check payments for the whole organization
// (a deposit spans properties). Never by role name.
export function canCountDeposits(ability: AppAbility): boolean {
  return ability.can("check", everywhere("payment"));
}

// The screen (and Penagihan's link to it) is for a member who may count deposits or read cash for
// the whole organization (the Supervisor); the API answers 403 to anyone else.
export function canReadDeposits(ability: AppAbility): boolean {
  return canCountDeposits(ability) || ability.can("read", everywhere("cash"));
}

// "Hitung" on someone else's deposit; nobody counts their own (the API refuses it too).
export function countAction(
  deposit: Pick<CashDeposit, "staffUserId">,
  userId: string,
  canCount: boolean,
): CountAction {
  if (!canCount) return "none";
  return deposit.staffUserId === userId ? "own" : "count";
}

// The tab in the URL: "confirmed" for ?tab=confirmed, the pending tab otherwise.
export function readDepositTab(params: URLSearchParams): DepositTab {
  return params.get(TAB_PARAM) === "confirmed" ? "confirmed" : "pending";
}

export function toDepositTab(value: string): DepositTab {
  return value === "confirmed" ? "confirmed" : "pending";
}

// The tab as a URL param: the default (pending) stays out of the URL.
export function tabParam(tab: DepositTab): string | null {
  return tab === "confirmed" ? "confirmed" : null;
}

// How one amount differs from another, for display: "kurang Rp50.000", "lebih Rp50.000", or null
// when they are the same.
export function differenceText(
  amount: number,
  expected: number,
): string | null {
  if (amount === expected) return null;
  const size = formatRupiah(Math.abs(amount - expected));
  return amount < expected ? `kurang ${size}` : `lebih ${size}`;
}

// The live line under the field, against the payments' total (never against what the karyawan
// said); null while the field holds no amount.
export function countHint(text: string, expected: number): CountHint | null {
  if (text.trim() === "") return null;
  const amount = parseRupiahInput(text);
  if (amount === null) return null;
  if (amount === expected) {
    return { tone: "match", text: "Cocok dengan total kwitansi." };
  }
  const size = formatRupiah(Math.abs(amount - expected));
  const side = amount < expected ? "Kurang" : "Lebih";
  return {
    tone: "gap",
    text: `${side} ${size} dari total kwitansi. Selisihnya masuk Antrean cek. Tagihan penyewa tidak berubah.`,
  };
}

// A photo's status as text for the admin. Opening a photo comes with its own board, so there is
// no link; a refused one reads "Foto ditolak" (taking another is the karyawan's step).
export function photoText(status: ReceiptPhotoStatus): string {
  if (status === "rejected" || status === "infected") return "Foto ditolak";
  return photoChip(status).label;
}

export function pendingRowView(
  deposit: CashDeposit,
  action: CountAction,
): PendingDepositRowView {
  return {
    id: deposit.id,
    staffName: deposit.staffName,
    recordedLine: `Disetor ${formatTimestampWeekdayDayMonthTime(deposit.recordedAt)}`,
    paymentCount: deposit.paymentCount,
    expected: formatRupiah(deposit.expectedAmount),
    handed: formatRupiah(deposit.amount),
    handedNote: differenceText(deposit.amount, deposit.expectedAmount),
    action,
  };
}

// "Cocok" when the count matched the payments' total, else the size of the gap and its side.
export function countedChip(gap: number): DepositChip {
  if (gap === 0) return { label: "Cocok", tone: "active" };
  const size = formatRupiah(Math.abs(gap));
  return {
    label: gap < 0 ? `Kurang ${size}` : `Lebih ${size}`,
    tone: "warning",
  };
}

// Where a gap is: still in Antrean cek, or closed there with a note.
export function gapNote(
  gap: number,
  closure: CashDeposit["gapClosure"],
): string | null {
  if (gap === 0) return null;
  return closure === undefined ? "Di Antrean cek" : "Selisih ditutup";
}

// The counted deposits, in the API's order; one the API sent without its count is left out.
export function countedRowViews(
  deposits: readonly CashDeposit[],
): CountedDepositRowView[] {
  return deposits.flatMap((deposit) => {
    const counted = deposit.confirmation;
    if (counted === undefined) return [];
    return [
      {
        id: deposit.id,
        staffName: deposit.staffName,
        recordedLine: `Disetor ${formatTimestampShortWeekdayDayMonthTime(deposit.recordedAt)}`,
        expected: formatRupiah(deposit.expectedAmount),
        receipts: receiptCountText(deposit.paymentCount),
        received: formatRupiah(counted.receivedAmount),
        result: countedChip(counted.gap),
        resultNote: gapNote(counted.gap, deposit.gapClosure),
        countedBy: counted.confirmedByName,
        countedAt: formatTimestampShortWeekdayDayMonthTime(counted.confirmedAt),
      },
    ];
  });
}

export function countSheetView(deposit: CashDeposit): CountSheetView {
  return {
    subtitle: `${deposit.staffName} · disetor ${formatTimestampWeekdayDayMonthTime(deposit.recordedAt)}`,
    expected: formatRupiah(deposit.expectedAmount),
    receipts: receiptCountText(deposit.paymentCount),
    handed: formatRupiah(deposit.amount),
    photo: photoText(deposit.photo.status),
  };
}

export function depositReceiptView(
  payment: HeldCashPayment,
): DepositReceiptView {
  const units = payment.units.map((unit) => unit.unitLabel).join(", ");
  const recorded = formatTimestampShortWeekdayDayMonthTime(payment.recordedAt);
  return {
    paymentId: payment.paymentId,
    tenantName: payment.tenantName,
    detail: units === "" ? recorded : `${units} · ${recorded}`,
    amount: formatRupiah(payment.amount),
    photo: photoText(payment.photo.status),
  };
}

// The friendly early checks. Rp0 is a real count (the API's minimum is 0). The Go API checks
// everything again and its message wins.
export const countFormSchema = z.object({
  receivedAmount: z.string().transform((text, ctx) => {
    if (text.trim() === "") {
      ctx.addIssue({ code: "custom", message: EMPTY_RECEIVED_MESSAGE });
      return z.NEVER;
    }
    const amount = parseRupiahInput(text);
    if (amount === null) {
      ctx.addIssue({ code: "custom", message: RECEIVED_DIGITS_MESSAGE });
      return z.NEVER;
    }
    if (amount > RECEIVED_AMOUNT_MAX) {
      ctx.addIssue({ code: "custom", message: RECEIVED_TOO_LARGE_MESSAGE });
      return z.NEVER;
    }
    return amount;
  }),
  note: z
    .string()
    .transform((text) => text.trim())
    .refine((text) => Array.from(text).length <= NOTE_MAX_LENGTH, {
      message: NOTE_TOO_LONG_MESSAGE,
    }),
});

// The field opens empty: the admin types their own count.
export const COUNT_FORM_DEFAULTS: CountFormValues = {
  receivedAmount: "",
  note: "",
};

// The request body: the integer counted, and the note only when there is one.
export function confirmRequestBody(
  fields: CountFormOutput,
): CashDepositConfirmationInput {
  return fields.note === ""
    ? { receivedAmount: fields.receivedAmount }
    : { receivedAmount: fields.receivedAmount, note: fields.note };
}

// The toast after a save, from the API's answer (its status and gap), never from the typed hint.
export function countSavedToast(deposit: CashDeposit): ToastMessage {
  const gap = deposit.confirmation?.gap ?? 0;
  if (deposit.status === "matched" || gap === 0) {
    return {
      tone: "create",
      title: "Hitungan disimpan",
      description: "Cocok dengan total kwitansi.",
    };
  }
  return {
    tone: "caution",
    title: "Hitungan disimpan",
    description: `Selisih ${formatRupiah(Math.abs(gap))} masuk Antrean cek.`,
  };
}

export const ALREADY_COUNTED_TOAST: ToastMessage = {
  tone: "neutral",
  title: "Setoran ini sudah dihitung",
  description: null,
};

export function confirmErrorKind(error: unknown): ConfirmErrorKind {
  if (
    error instanceof ApiError &&
    error.status === 409 &&
    error.code === "deposit_already_confirmed"
  ) {
    return "already_confirmed";
  }
  return "other";
}

// The accessible name of Penagihan's link: the count of deposits waiting, when there are any.
export function depositLinkLabel(pending: number): string {
  return pending > 0
    ? `${DEPOSIT_LINK_TEXT}, ${pending} menunggu dihitung`
    : DEPOSIT_LINK_TEXT;
}
