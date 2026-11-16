import { z } from "zod";

import { invoiceName } from "@/components/client/billing/utils";
import {
  AMOUNT_DIGITS_MESSAGE,
  AMOUNT_TOO_LARGE_MESSAGE,
  AMOUNT_ZERO_MESSAGE,
  EMPTY_AMOUNT_MESSAGE,
  PAYMENT_AMOUNT_MAX,
} from "@/components/client/payment/utils";
import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  NETWORK_ERROR_CODE,
} from "@/lib/api/errors";
import type {
  CashPaymentInput,
  CashPaymentResult,
  PaymentReceiptStatus,
  ReceiptPhotoStatus,
  UploadInput,
} from "@/lib/api/generated/model";
import {
  isStorageUploadError,
  type StorageUploadProgress,
} from "@/lib/api/storage-upload";
import type { AppAbility } from "@/lib/capabilities";
import { formatDate } from "@/lib/format/date";
import { formatRupiah, parseRupiahInput } from "@/lib/format/rupiah";

import type {
  CashPhotoLineView,
  CashRecordedView,
  CashTotalLineView,
  PhotoResume,
  PhotoUploadOutcome,
  PhotoUploadSteps,
  ReceiptPhotoState,
  ReceiveCashTenant,
  RecordErrorView,
  TenantUploadPurpose,
  UploadFailureClass,
  UploadFailureView,
} from "./type";

// "Terima tunai" (board HP-Tagihan, draft 9, columns 2 and 3; photo states on board HP-Foto,
// draft 5 revised). Cash is never recorded without its receipt photo and the photo never comes
// later (decision 50): the photo is uploaded first, and Simpan sends its upload id. Every amount
// shown is a field the API sent, only formatted; FIFO, credit and what is left are the API's.

export const RECEIVE_CASH_TITLE = "Terima tunai";
export const RECEIVE_CASH_BUTTON_TEXT = "Terima tunai";
export const CLOSE_LABEL = "Tutup";
export const CANCEL_TEXT = "Batal";
export const DONE_TEXT = "Selesai";

export const OWED_LABEL = "Total belum dibayar";

export const AMOUNT_LABEL = "Uang yang diterima";
export const AMOUNT_HINT =
  "Boleh sebagian, tanpa minimum. Hitung uangnya di depan penyewa.";

export const PHOTO_LABEL = "Foto kwitansi";
export const PHOTO_REQUIRED_TEXT = "(wajib)";
// The file input's accessible name, and the empty state's button.
export const PHOTO_PICK_TEXT = "Ambil foto kwitansi";
export const PHOTO_HINT =
  "Kamera atau galeri. Nama penyewa, jumlah dan tanda tangan harus terbaca.";
// No `capture`: with it the phone opens only the camera; without it iOS offers "Ambil Foto" and
// the photo library, and Android its camera and gallery.
export const PHOTO_ACCEPT = "image/*";
export const PHOTO_SENDING_TEXT = "Mengirim foto…";
export const PHOTO_SENDING_NOTE = "Jangan tutup aplikasi.";
export const PHOTO_PROGRESS_LABEL = "Mengirim foto kwitansi";
export const PHOTO_FAILED_TITLE = "Foto belum terkirim";
export const PHOTO_NETWORK_TEXT = "Sinyal hilang. Uang belum tercatat.";
export const PHOTO_NOT_RECORDED_TEXT = "Uang belum tercatat.";
export const PHOTO_STORAGE_REFUSED_TEXT = "Penyimpanan foto menolak kiriman.";
export const PHOTO_RETRY_TEXT = "Coba lagi";
export const PHOTO_OTHER_TEXT = "Pilih foto lain";
export const PHOTO_REFUSED_TITLE = "Foto tidak bisa dipakai";
export const PHOTO_SENT_TITLE = "Foto terkirim";
export const PHOTO_VIEW_TEXT = "Ketuk untuk melihat";
export const PHOTO_REPLACE_TEXT = "Ganti";
export const PHOTO_REPLACE_LABEL = "Ganti foto kwitansi";
export const PHOTO_VIEWER_TITLE = "Foto kwitansi";
export const PHOTO_PREVIEW_ALT = "Foto kwitansi yang dipilih";
// The API's own words for upload_too_large, for the same refusal made before sending.
export const PHOTO_TOO_LARGE_TEXT = "Ukuran foto paling besar 10 MB.";

export const FIFO_NOTE =
  "Dipakai untuk tagihan paling lama dulu. Penyewa langsung dapat kwitansi lewat WA.";

export const SAVE_TEXT = "Simpan";
export const SAVING_TEXT = "Menyimpan…";
export const WAITING_PHOTO_TEXT = "Menunggu foto…";
export const UNKNOWN_SAVE_TEXT =
  "Sinyal hilang, belum pasti tersimpan. Tekan Simpan lagi; uang tidak akan tercatat dua kali.";

export const PAID_TITLE = "Dipakai untuk";
export const PAID_LIST_LABEL = "Tagihan yang dibayar";
export const ALL_CREDIT_TEXT = "Semua masuk ke saldo lebih bayar.";
export const CREDIT_LABEL = "Saldo lebih bayar";
export const REMAINING_LABEL = "Sisa yang belum dibayar";

// The fields of the form an API error can name; photoUploadId goes under the photo, any other
// field (requestId, note) to the alert.
export const CASH_API_FIELDS = ["amount"] as const;
const PHOTO_UPLOAD_FIELD = "photoUploadId";
const UPLOAD_FILE_FIELDS: readonly string[] = ["contentType", "size"];
const UPLOAD_FILE_CODES: readonly string[] = [
  "upload_type_not_allowed",
  "upload_too_large",
];
const REQUEST_ID_REUSED_CODE = "request_id_reused";

export const CASH_FORM_DEFAULTS = { amount: "" };

// The friendly early check of "Uang yang diterima"; the Go API checks it again and its message
// wins. The amount opens empty, and only digits can be typed.
export const cashFormSchema = z.object({
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
});

// "Terima tunai" shows to a member who may record cash on some tenants or all ("conditional"
// shows it and the API decides per tenant). Never by role name.
export function canRecordCash(ability: AppAbility): boolean {
  return ability.can("record", "cash");
}

// "Bu Wati Handayani · Kos Melati 11"; the name alone without a place.
export function sheetSubtitle(tenantName: string, placeLabel: string): string {
  return placeLabel.trim() === ""
    ? tenantName
    : `${tenantName} · ${placeLabel}`;
}

function dateText(date: string | undefined): string | null {
  if (date === undefined) return null;
  try {
    return formatDate(date);
  } catch {
    return null;
  }
}

// The line under "Total belum dibayar", from the row's oldest unpaid invoice: "Jatuh tempo 9 Nov
// 2026, telat 7 hari", "Jatuh tempo hari ini", "Jatuh tempo besok". The list item has the due
// date, not the invoice's period, so the line names the date. Null when there is nothing to say.
export function owedLine(tenant: ReceiveCashTenant): string | null {
  const due = dateText(tenant.oldestUnpaidDueDate);
  switch (tenant.reason) {
    case "overdue": {
      const late =
        tenant.daysOverdue === undefined
          ? null
          : `telat ${tenant.daysOverdue} hari`;
      if (due !== null && late !== null) return `Jatuh tempo ${due}, ${late}`;
      if (late !== null) return `Telat ${tenant.daysOverdue} hari`;
      return due === null ? null : `Jatuh tempo ${due}`;
    }
    case "due_today":
      return "Jatuh tempo hari ini";
    case "due_in_days":
      if (tenant.daysUntilDue === 1) return "Jatuh tempo besok";
      if (tenant.daysUntilDue === 2) return "Jatuh tempo lusa";
      return due === null ? null : `Jatuh tempo ${due}`;
    case "paid_today":
      return due === null ? null : `Jatuh tempo ${due}`;
  }
}

// createUpload's body for a photo of this tenant (a cash receipt, or a transfer proof the tenant
// showed): the file's type as the browser reports it and its exact size (signed into the PUT
// URL). The API judges the type.
export function tenantUploadInput(
  purpose: TenantUploadPurpose,
  tenantId: string,
  file: Blob,
): UploadInput {
  return {
    purpose,
    tenantId,
    contentType: file.type,
    size: file.size,
  };
}

// One attempt to send a photo of a tenant: createUpload (for its purpose), the PUT to storage
// with its progress, then completeUpload; or, resuming, only completeUpload for a file already in
// storage. The signal cancels the attempt: the PUT is aborted, and an answer that arrives after
// it is dropped (an upload left pending is rejected by the API later). A failure says where "Coba
// lagi" starts: completeUpload again once the file is stored (harmless to repeat), otherwise a
// new upload, since the PUT URL may have expired. Used by "Terima tunai" (cash_receipt) and
// "Bukti transfer dari penyewa" (transfer_proof, which completeUpload records for the admin).
export async function uploadTenantPhoto(
  steps: PhotoUploadSteps,
  {
    purpose,
    tenantId,
    file,
    resume,
    signal,
    onProgress,
  }: {
    purpose: TenantUploadPurpose;
    tenantId: string;
    file: Blob;
    resume: PhotoResume;
    signal: AbortSignal;
    onProgress: (progress: StorageUploadProgress) => void;
  },
): Promise<PhotoUploadOutcome> {
  let uploadId: string;
  if (resume.step === "complete") {
    uploadId = resume.uploadId;
  } else {
    try {
      const ticket = await steps.create(
        tenantUploadInput(purpose, tenantId, file),
      );
      if (signal.aborted) return { kind: "cancelled" };
      await steps.put(ticket, file, { signal, onProgress });
      uploadId = ticket.upload.id;
    } catch (error) {
      if (signal.aborted) return { kind: "cancelled" };
      return { kind: "failed", error, resume: { step: "create" } };
    }
  }
  if (signal.aborted) return { kind: "cancelled" };
  try {
    await steps.complete(uploadId);
  } catch (error) {
    if (signal.aborted) return { kind: "cancelled" };
    return { kind: "failed", error, resume: { step: "complete", uploadId } };
  }
  return signal.aborted ? { kind: "cancelled" } : { kind: "sent", uploadId };
}

// recordCashPayment's body. One requestId per opening of the sheet, sent again unchanged on a
// retry, with the same amount and photo, so a retry after a lost answer records the cash once.
export function cashPaymentBody(
  amount: number,
  requestId: string,
  photoUploadId: string,
): CashPaymentInput {
  return { requestId, amount, photoUploadId };
}

// The primary's label: "Menyimpan…" while recording, "Menunggu foto…" while the photo is sent,
// otherwise "Simpan Rp1.000.000" naming the typed amount, or "Simpan" before one is typed.
export function saveLabel(
  photo: ReceiptPhotoState,
  amountText: string,
  saving: boolean,
): string {
  if (saving) return SAVING_TEXT;
  if (photo.kind === "uploading") return WAITING_PHOTO_TEXT;
  const amount = parseRupiahInput(amountText);
  return amount === null || amount <= 0
    ? SAVE_TEXT
    : `${SAVE_TEXT} ${formatRupiah(amount)}`;
}

// Simpan works only once the photo reached the API (decision 50), and not while it records. The
// amount is checked when it is pressed, so a missing amount says why.
export function canSave(photo: ReceiptPhotoState, saving: boolean): boolean {
  return photo.kind === "sent" && !saving;
}

// "Mengirim foto… 64%", or "Mengirim foto…" before the size is known.
export function uploadingLine(percent: number | null): string {
  return percent === null
    ? PHOTO_SENDING_TEXT
    : `${PHOTO_SENDING_TEXT} ${percent}%`;
}

// What the polite live region says when the photo's state changes; the percent is not read out.
// A refused photo is announced by its error (role="alert").
export function photoAnnouncement(state: ReceiptPhotoState): string {
  switch (state.kind) {
    case "uploading":
      return PHOTO_SENDING_TEXT;
    case "failed":
      return `${PHOTO_FAILED_TITLE}. ${state.message}`;
    case "sent":
      return `${PHOTO_SENT_TITLE}.`;
    case "empty":
    case "refused":
      return "";
  }
}

function sentence(text: string): string {
  const trimmed = text.trim();
  return /[.!?]$/.test(trimmed) ? trimmed : `${trimmed}.`;
}

// The second line of "Foto belum terkirim" for a failure that is not the signal: its message,
// then "Uang belum tercatat."
export function notRecordedLine(message: string): string {
  return `${sentence(message)} ${PHOTO_NOT_RECORDED_TEXT}`;
}

// What went wrong in a failed step of a photo's upload (createUpload, the PUT, completeUpload),
// before any screen words it: the caller's own abort; no answer ("Sinyal hilang"); the API
// refusing the file itself (its type or size), with its message; the API refusing the member
// (403) or not finding the tenant (404), with its message; anything else, with the API's message
// or the page's own.
export function classifyUploadFailure(error: unknown): UploadFailureClass {
  if (isStorageUploadError(error)) {
    if (error.kind === "aborted") return { kind: "cancelled" };
    if (error.kind === "network") return { kind: "network" };
    return { kind: "other", message: PHOTO_STORAGE_REFUSED_TEXT };
  }
  if (!(error instanceof ApiError)) {
    return { kind: "other", message: GENERIC_ERROR_MESSAGE };
  }
  if (error.code === NETWORK_ERROR_CODE || error.status === 0) {
    return { kind: "network" };
  }
  if (
    error.status === 400 &&
    (UPLOAD_FILE_CODES.includes(error.code) ||
      (error.field !== undefined && UPLOAD_FILE_FIELDS.includes(error.field)))
  ) {
    return { kind: "refused", message: error.message };
  }
  if (error.status === 403 || error.status === 404) {
    return { kind: "denied", message: error.message };
  }
  return { kind: "other", message: error.message };
}

// What a failed step of the receipt photo's upload shows. A dropped connection is "Sinyal
// hilang"; the API refusing the file itself (its type or size) puts its message under the photo
// with only "Pilih foto lain"; anything else keeps "Coba lagi".
export function uploadFailureView(error: unknown): UploadFailureView {
  const failure = classifyUploadFailure(error);
  switch (failure.kind) {
    case "cancelled":
      return failure;
    case "network":
      return { kind: "failed", reason: "network", message: PHOTO_NETWORK_TEXT };
    case "refused":
      return { kind: "refused", message: failure.message };
    case "denied":
    case "other":
      return {
        kind: "failed",
        reason: "other",
        message: notRecordedLine(failure.message),
      };
  }
}

// What a failed recordCashPayment shows. No answer or a server failure may have recorded the
// cash, so the same request is sent again (the API answers 200 if it did). The photo's refusal
// goes under the photo; request_id_reused cannot happen while the request stays the same, and is
// a generic error if it does.
export function recordErrorView(error: unknown): RecordErrorView {
  if (!(error instanceof ApiError)) {
    return { kind: "alert", message: GENERIC_ERROR_MESSAGE };
  }
  if (
    error.code === NETWORK_ERROR_CODE ||
    error.status === 0 ||
    error.status >= 500
  ) {
    return { kind: "unknown", message: UNKNOWN_SAVE_TEXT };
  }
  if (error.field === PHOTO_UPLOAD_FIELD) {
    return { kind: "photo", message: error.message };
  }
  if (error.code === REQUEST_ID_REUSED_CODE) {
    return { kind: "alert", message: GENERIC_ERROR_MESSAGE };
  }
  if (error.field === "amount") {
    return { kind: "amount", message: error.message };
  }
  return { kind: "alert", message: error.message };
}

// The receipt line of "Tersimpan", from the API's receipt status: queued ("sedang dikirim"),
// handed to WhatsApp, or not sent.
export function receiptLine(
  status: PaymentReceiptStatus,
  tenantName: string,
): string {
  switch (status) {
    case "pending":
      return `Kwitansi sedang dikirim ke WA ${tenantName}.`;
    case "sent":
      return `Kwitansi dikirim ke WA ${tenantName}.`;
    case "skipped":
      return "Kwitansi tidak dikirim.";
  }
}

// The photo line of "Tersimpan", from the answer's photo status. The scan is not waited for, so
// it is usually still pending.
export function photoLine(status: ReceiptPhotoStatus): CashPhotoLineView {
  switch (status) {
    case "pending":
      return {
        title: "Foto kwitansi tersimpan",
        detail: "Sedang diperiksa sistem",
        refused: false,
      };
    case "clean":
      return { title: "Foto kwitansi tersimpan", detail: null, refused: false };
    case "rejected":
    case "infected":
      return {
        title: "Foto kwitansi ditolak sistem",
        detail: "Ambil ulang dari daftar tagihan.",
        refused: true,
      };
    case "missing":
      return { title: "Foto kwitansi belum ada", detail: null, refused: true };
  }
}

// The totals under the paid invoices: the credit (only when there is some) and what the tenant
// still owes.
export function cashTotalLines(
  result: Pick<CashPaymentResult, "credit" | "outstanding">,
): CashTotalLineView[] {
  const lines: CashTotalLineView[] = [];
  if (result.credit > 0) {
    lines.push({ label: CREDIT_LABEL, value: formatRupiah(result.credit) });
  }
  lines.push({
    label: REMAINING_LABEL,
    value: formatRupiah(result.outstanding),
  });
  return lines;
}

export function cashRecordedView(
  result: CashPaymentResult,
  tenantName: string,
): CashRecordedView {
  return {
    headline: `${formatRupiah(result.payment.amount)} tercatat`,
    receiptLine: receiptLine(result.receipt.status, tenantName),
    paid: result.paid.map((invoice) => ({
      invoiceId: invoice.invoiceId,
      name: invoiceName(invoice),
      amount: formatRupiah(invoice.amount),
    })),
    paidEmptyText: result.paid.length === 0 ? ALL_CREDIT_TEXT : null,
    totals: cashTotalLines(result),
    photo: photoLine(result.photo.status),
  };
}
