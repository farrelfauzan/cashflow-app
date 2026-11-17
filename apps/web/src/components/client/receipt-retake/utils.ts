import { classifyUploadFailure } from "@/components/client/field-cash/utils";
import type {
  ReceiptPhotoState,
  UploadFailureView,
} from "@/components/client/field-cash/type";
import type { ToastMessage } from "@/components/client/toast/type";
import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  NETWORK_ERROR_CODE,
} from "@/lib/api/errors";
import type {
  AttachCashPaymentPhotoInput,
  ReceiptPhoto,
  ReceiptPhotoStatus,
} from "@/lib/api/generated/model";
import {
  formatTimestampTime,
  formatTimestampWeekdayDayMonthTime,
  jakartaToday,
} from "@/lib/format/date";
import { formatRupiah } from "@/lib/format/rupiah";

import type { AttachErrorView, RetakePayment } from "./type";

// "Ambil ulang" (decision 46.10, board HP-Foto, "Setelah disimpan"): the karyawan retakes the
// receipt photo of a cash payment whose photo the scan refused after it was saved. The cash stays
// recorded; only the photo changes, through attachCashPaymentPhoto. The photo field and its
// states are Terima tunai's.

export const RETAKE_BUTTON_TEXT = "Ambil ulang";
export const RETAKE_TITLE = "Ambil ulang foto kwitansi";
export const RETAKE_EXPLAIN =
  "Foto sebelumnya ditolak pemeriksaan. Uangnya tetap tercatat; hanya fotonya yang diganti.";
export const RETAKE_PAYMENT_LABEL = "Pembayaran tunai";
export const RETAKE_SAVE_TEXT = "Simpan foto";
export const RETAKE_SAVING_TEXT = "Menyimpan…";
export const RETAKE_WAITING_TEXT = "Menunggu foto…";
export const RETAKE_NETWORK_TEXT = "Sinyal hilang. Foto belum diganti.";
export const RETAKE_NOT_REPLACED_TEXT = "Foto belum diganti.";
export const RETAKE_UNKNOWN_TEXT =
  "Sinyal hilang, belum pasti tersimpan. Tekan Simpan foto lagi; foto tidak akan tercatat dua kali.";
export const RETAKE_NOT_NEEDED_TITLE = "Foto tidak perlu diganti";

// The field attachCashPaymentPhoto's errors name for the photo.
const UPLOAD_FIELD = "uploadId";
const PHOTO_ATTACHED_CODE = "receipt_photo_attached";

// Only a photo the scan refused can be replaced (the API takes no other: a clean or pending one
// is never swapped, and "foto belum ada" is not offered here, as on the board).
const RETAKE_STATUSES: readonly ReceiptPhotoStatus[] = ["rejected", "infected"];

export function canRetakePhoto(status: ReceiptPhotoStatus): boolean {
  return RETAKE_STATUSES.includes(status);
}

// A payment as the retake sheet needs it, from a Tagihan hari ini row's payment or a receipt held
// for Setor tunai.
export function retakePayment(
  payment: {
    paymentId: string;
    amount: number;
    recordedAt: string;
    photo: ReceiptPhoto;
  },
  tenant: { tenantId: string; tenantName: string },
  place: string,
): RetakePayment {
  return {
    paymentId: payment.paymentId,
    tenantId: tenant.tenantId,
    tenantName: tenant.tenantName,
    place,
    amount: payment.amount,
    recordedAt: payment.recordedAt,
    photoStatus: payment.photo.status,
  };
}

// "Tunai Rp1.000.000 · 10.42" for cash recorded today (Asia/Jakarta), else with its day: "Tunai
// Rp1.000.000 · Jumat 13 Nov, 10.15".
export function retakeSummary(
  amount: number,
  recordedAt: string,
  now: Date,
): string {
  const recorded = new Date(recordedAt);
  const when =
    !Number.isNaN(recorded.getTime()) &&
    jakartaToday(recorded) === jakartaToday(now)
      ? formatTimestampTime(recordedAt)
      : formatTimestampWeekdayDayMonthTime(recordedAt);
  return `Tunai ${formatRupiah(amount)} · ${when}`;
}

// The button's accessible name: its visible "Ambil ulang" first, then which payment, since a row
// may hold several.
export function retakeButtonLabel(summary: string): string {
  return `${RETAKE_BUTTON_TEXT} foto kwitansi ${summary}`;
}

function sentence(text: string): string {
  const trimmed = text.trim();
  return /[.!?]$/.test(trimmed) ? trimmed : `${trimmed}.`;
}

// A failed step of the new photo's upload. The states are Terima tunai's, but the cash is already
// recorded, so a failure says the photo is not replaced yet rather than "Uang belum tercatat".
export function retakeUploadFailureView(error: unknown): UploadFailureView {
  const failure = classifyUploadFailure(error);
  switch (failure.kind) {
    case "cancelled":
      return failure;
    case "network":
      return {
        kind: "failed",
        reason: "network",
        message: RETAKE_NETWORK_TEXT,
      };
    case "refused":
      return { kind: "refused", message: failure.message };
    case "denied":
    case "other":
      return {
        kind: "failed",
        reason: "other",
        message: `${sentence(failure.message)} ${RETAKE_NOT_REPLACED_TEXT}`,
      };
  }
}

// attachCashPaymentPhoto's body: the completed upload, sent again unchanged on a retry.
export function attachPhotoBody(uploadId: string): AttachCashPaymentPhotoInput {
  return { uploadId };
}

// What a failed attachCashPaymentPhoto shows. No answer or a server failure may have attached the
// photo, so the same uploadId is sent again (the API answers 200 if it did). The new photo's own
// refusal (field uploadId) goes under the photo.
export function attachErrorView(error: unknown): AttachErrorView {
  if (!(error instanceof ApiError)) {
    return { kind: "alert", message: GENERIC_ERROR_MESSAGE };
  }
  if (
    error.code === NETWORK_ERROR_CODE ||
    error.status === 0 ||
    error.status >= 500
  ) {
    return { kind: "unknown", message: RETAKE_UNKNOWN_TEXT };
  }
  if (error.code === PHOTO_ATTACHED_CODE) {
    return { kind: "attached", message: error.message };
  }
  if (error.field === UPLOAD_FIELD) {
    return { kind: "photo", message: error.message };
  }
  return { kind: "alert", message: error.message };
}

// The primary: "Menyimpan…" while attaching, "Menunggu foto…" while the photo is sent, otherwise
// "Simpan foto".
export function retakeSaveLabel(
  photoKind: ReceiptPhotoState["kind"],
  saving: boolean,
): string {
  if (saving) return RETAKE_SAVING_TEXT;
  if (photoKind === "uploading") return RETAKE_WAITING_TEXT;
  return RETAKE_SAVE_TEXT;
}

// "Simpan foto" works once the new photo reached the API (completeUpload answered).
export function canSaveRetake(
  photoKind: ReceiptPhotoState["kind"],
  saving: boolean,
): boolean {
  return photoKind === "sent" && !saving;
}

export function retakenToast(tenantName: string): ToastMessage {
  return {
    tone: "create",
    title: "Foto kwitansi diganti",
    description: tenantName,
  };
}

// receipt_photo_attached: the photo is no longer refused, with the API's own words.
export function notNeededToast(message: string): ToastMessage {
  return {
    tone: "neutral",
    title: RETAKE_NOT_NEEDED_TITLE,
    description: message,
  };
}
