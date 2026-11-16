import type { RefObject } from "react";

import type {
  ChosenPhoto,
  PhotoResume,
} from "@/components/client/field-cash/type";
import type { FieldTodayItem } from "@/lib/api/generated/model";

// The tenant a "Bukti transfer dari penyewa" sheet is for: the fields of their Tagihan hari ini
// row it uses. A whole FieldTodayItem fits.
export type FieldProofTenant = Pick<FieldTodayItem, "tenantId" | "tenantName">;

// Why the chosen photo cannot be sent as it is: the API refused the file itself (its type or
// size; under the photo, pick another), or refused the member (403, the tenant is no longer
// theirs) or found no such tenant (404), in the sheet's alert.
export type FieldProofError = {
  kind: "file" | "denied";
  message: string;
};

// The sheet (board HP-Bukti-Karyawan, columns 2 to 4). Nothing is sent until "Kirim bukti".
export type FieldProofState =
  // 2, no photo yet: the dashed "Ambil foto bukti transfer".
  | { kind: "empty" }
  // The chosen photo is made ready (shrunk, re-encoded to JPEG).
  | { kind: "preparing" }
  // 2, the photo shown whole; "Kirim bukti" works unless it carries an error.
  | { kind: "ready"; photo: ChosenPhoto; error: FieldProofError | null }
  // 3, "Mengirim… 62%": the sheet cannot close. `percent` is null until the size is known.
  | { kind: "sending"; photo: ChosenPhoto; percent: number | null }
  // 4, "Bukti belum terkirim": "Coba lagi" sends the same photo, from the step that failed.
  | {
      kind: "failed";
      photo: ChosenPhoto;
      message: string;
      resume: PhotoResume;
    };

// What a failed send shows.
export type FieldProofFailureView =
  // The sheet went away mid-send: nothing to show.
  | { kind: "cancelled" }
  | { kind: "error"; error: FieldProofError }
  // "Bukti belum terkirim" with this line, and "Coba lagi".
  | { kind: "failed"; message: string };

// useFieldProof: one opening of the sheet.
export type FieldProofSend = {
  state: FieldProofState;
  // completeUpload's answer was lost: no other photo, and the sheet is kept for the next opening.
  unsure: boolean;
  // The hidden file input (camera or gallery).
  inputRef: RefObject<HTMLInputElement | null>;
  // Opens the camera or gallery; nothing while a photo is sent.
  pickPhoto: () => void;
  // A photo was chosen: made ready and shown, not sent.
  choose: (file: File) => void;
  // "Kirim bukti": createUpload (transfer_proof), the PUT, completeUpload.
  send: () => void;
  // "Coba lagi": the same photo, from the step that failed.
  retry: () => void;
};
