import type { FormEvent } from "react";

import type { ReceiptPhotoUpload } from "@/components/client/field-cash/type";
import type { ReceiptPhotoStatus } from "@/lib/api/generated/model";

// One field cash payment whose receipt photo may be retaken: a payment of a Tagihan hari ini row
// (FieldCashPayment with its row's tenant) or a receipt held for Setor tunai (HeldCashPayment).
export type RetakePayment = {
  paymentId: string;
  tenantId: string;
  tenantName: string;
  // "Kamar 11 · Kos Melati"; empty without a unit.
  place: string;
  amount: number;
  recordedAt: string;
  photoStatus: ReceiptPhotoStatus;
};

// What a failed attachCashPaymentPhoto shows.
export type AttachErrorView =
  // No answer, or a server failure: the photo may be attached. The photo stays as sent and
  // "Simpan foto" sends the same uploadId again, which answers 200 if it was.
  | { kind: "unknown"; message: string }
  // receipt_photo_attached: the payment's photo is no longer refused (attached from elsewhere,
  // or by a request whose answer was lost). Nothing to retake: close and reload.
  | { kind: "attached"; message: string }
  // The new photo itself cannot be used (field uploadId): under the photo, pick another.
  | { kind: "photo"; message: string }
  // At the top of the sheet (403, the payment gone, anything else).
  | { kind: "alert"; message: string };

// useRetakePhoto: one opening of "Ambil ulang foto kwitansi".
export type RetakePhoto = {
  photo: ReceiptPhotoUpload;
  // The message at the top of the sheet, if any.
  alert: string | null;
  // attachCashPaymentPhoto is running: nothing can change and the sheet cannot close.
  saving: boolean;
  // An attach got no answer, so the photo may be attached: it stays as sent and "Simpan foto"
  // sends the same uploadId again.
  locked: boolean;
  // A photo was chosen and nothing is attached yet: closing on a phone asks first.
  dirty: boolean;
  submit: (event: FormEvent<HTMLFormElement>) => void;
  // The sheet closed: stop the upload, and reload the lists if a photo was (or may have been)
  // attached.
  finish: () => void;
};
