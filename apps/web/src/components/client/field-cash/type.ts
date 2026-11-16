import type { FormEvent } from "react";
import type { UseFormReturn } from "react-hook-form";
import type { z } from "zod";

import type {
  CashPaymentResult,
  FieldTodayItem,
  UploadInput,
  UploadTicket,
} from "@/lib/api/generated/model";
import type { PutToStorageOptions } from "@/lib/api/storage-upload";

import type { cashFormSchema } from "./utils";

// The tenant a "Terima tunai" sheet is for: the fields of their Tagihan hari ini row it shows.
// A whole FieldTodayItem fits.
export type ReceiveCashTenant = Pick<
  FieldTodayItem,
  | "tenantId"
  | "tenantName"
  | "reason"
  | "daysOverdue"
  | "daysUntilDue"
  | "oldestUnpaidDueDate"
  | "outstanding"
>;

// "Uang yang diterima" as react-hook-form holds it: digits grouped with dots.
export type CashFormValues = z.input<typeof cashFormSchema>;

// What the schema hands on: the amount in integer rupiah.
export type CashFormOutput = z.output<typeof cashFormSchema>;

// The photo chosen on this phone, made ready to send (shrunk, re-encoded to JPEG), and the object
// URL that shows it.
export type ChosenPhoto = {
  file: File;
  previewUrl: string;
};

// Where "Coba lagi" starts again: a new upload (createUpload, PUT, completeUpload), or only
// completeUpload when the file is already in storage.
export type PhotoResume =
  { step: "create" } | { step: "complete"; uploadId: string };

// Why a photo did not reach the API: no signal ("Sinyal hilang"), or another failure with the
// API's or the page's own message. Either way "Coba lagi" sends the same photo.
export type PhotoFailureReason = "network" | "other";

// "Foto kwitansi (wajib)" (board HP-Foto, draft 5 revised). Simpan works only in "sent".
export type ReceiptPhotoState =
  // 1, "Belum ada foto": "Ambil foto kwitansi".
  | { kind: "empty" }
  // 2, "Mengirim foto… 64%". `photo` is null while the chosen photo is made ready; `percent` null
  // until the browser knows the size.
  | { kind: "uploading"; photo: ChosenPhoto | null; percent: number | null }
  // 3, "Foto belum terkirim": "Coba lagi" (same photo) or "Pilih foto lain".
  | {
      kind: "failed";
      photo: ChosenPhoto;
      reason: PhotoFailureReason;
      message: string;
      resume: PhotoResume;
    }
  // The photo itself was refused (not a JPEG, PNG or WebP, too large, or refused by the scan):
  // the message under the photo, and only "Pilih foto lain".
  | { kind: "refused"; photo: ChosenPhoto; message: string }
  // 4, "Foto terkirim": completeUpload answered; its id goes with the payment.
  | { kind: "sent"; photo: ChosenPhoto; uploadId: string };

// What a failed step of the upload shows.
export type UploadFailureView =
  // The caller cancelled it (Batal, another photo): nothing to show.
  | { kind: "cancelled" }
  | { kind: "refused"; message: string }
  | { kind: "failed"; reason: PhotoFailureReason; message: string };

// What a failed recordCashPayment shows.
export type RecordErrorView =
  // No answer, or a server failure: the payment may be recorded. The amount and photo stay as
  // sent and Simpan sends the same request again, which answers 200 if it was.
  | { kind: "unknown"; message: string }
  // The photo cannot be used (invalid, refused by the scan): under the photo, pick another.
  | { kind: "photo"; message: string }
  // Under "Uang yang diterima".
  | { kind: "amount"; message: string }
  // At the top of the sheet (403, a tenant gone, anything else).
  | { kind: "alert"; message: string };

// One invoice the payment paid, oldest period first: "Tagihan November 2026  Rp1.000.000".
export type CashPaidLineView = {
  invoiceId: string;
  name: string;
  amount: string;
};

// One total under the paid invoices: "Sisa yang belum dibayar  Rp50.000".
export type CashTotalLineView = {
  label: string;
  value: string;
};

// The photo line of "Tersimpan", from the answer's photo status.
export type CashPhotoLineView = {
  title: string;
  detail: string | null;
  // The scan refused it: shown as a warning.
  refused: boolean;
};

// "Tersimpan" (board HP-Tagihan, 3): every figure a field of the API's CashPaymentResult.
export type CashRecordedView = {
  // "Rp1.000.000 tercatat".
  headline: string;
  // "Kwitansi sedang dikirim ke WA Bu Wati Handayani."
  receiptLine: string;
  paid: readonly CashPaidLineView[];
  // "Semua masuk ke saldo lebih bayar." when it paid no invoice, else null.
  paidEmptyText: string | null;
  totals: readonly CashTotalLineView[];
  photo: CashPhotoLineView;
};

// useReceiptPhoto: the photo's state and what the sheet can do with it.
export type ReceiptPhotoUpload = {
  state: ReceiptPhotoState;
  // A photo was chosen (camera or gallery): made ready, then sent at once.
  choose: (file: File) => void;
  // "Coba lagi": the same photo again, from the step that failed.
  retry: () => void;
  // The API refused the sent photo when the cash was saved: pick another.
  refuse: (message: string) => void;
  // Batal or a close: stops the PUT and ignores whatever the API answers after.
  cancel: () => void;
};

// useReceiveCash: one opening of "Terima tunai", from the empty form to "Tersimpan".
export type ReceiveCash = {
  form: UseFormReturn<CashFormValues, unknown, CashFormOutput>;
  // The message at the top of the form, if any.
  alert: string | undefined;
  // "Uang yang diterima" as typed, for the primary's label.
  amountText: string;
  photo: ReceiptPhotoUpload;
  // The API's answer once the cash is recorded; null before.
  result: CashPaymentResult | null;
  // recordCashPayment is running: nothing can be changed and the sheet cannot close.
  saving: boolean;
  // A save got no answer, so the cash may be recorded: the amount and photo stay as sent until
  // the API answers, and Simpan sends the same request again.
  locked: boolean;
  // Something was typed or a photo chosen and nothing is recorded: closing on a phone asks first.
  dirty: boolean;
  submit: (event: FormEvent<HTMLFormElement>) => void;
  // The sheet closed: stop the upload, and reload the lists if cash was (or may have been)
  // recorded.
  finish: () => void;
};

// The three calls of a receipt photo's upload, injected so the order and the retry rules are
// tested without a network: createUpload, the PUT to storage, completeUpload.
export type ReceiptUploadSteps = {
  create: (input: UploadInput) => Promise<UploadTicket>;
  put: (
    ticket: UploadTicket,
    file: Blob,
    options: PutToStorageOptions,
  ) => Promise<void>;
  complete: (uploadId: string) => Promise<unknown>;
};

// How one attempt ended: sent with its upload id, failed (with where "Coba lagi" starts), or
// cancelled by its signal, when nothing it answered may be shown.
export type ReceiptUploadOutcome =
  | { kind: "sent"; uploadId: string }
  | { kind: "failed"; error: unknown; resume: PhotoResume }
  | { kind: "cancelled" };
