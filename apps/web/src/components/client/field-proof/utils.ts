import { classifyUploadFailure } from "@/components/client/field-cash/utils";
import type { ChosenPhoto } from "@/components/client/field-cash/type";
import { PROOF_SENDING_TEXT } from "@/components/client/payment-page/utils";
import type { ToastMessage } from "@/components/client/toast/type";
import type { AppAbility } from "@/lib/capabilities";

import type { FieldProofFailureView, FieldProofState } from "./type";

// "Bukti transfer dari penyewa" (board HP-Bukti-Karyawan, decision 49): the karyawan photographs
// the bank app's screen or ATM slip a tenant shows them and sends it for the admin to check. It
// never records money and never changes the row: completeUpload puts the proof in the admin's
// Antrean cek and on the tenant's payment page. The karyawan cannot see it again afterwards.

export const FIELD_PROOF_BUTTON_TEXT = "Penyewa sudah transfer? Kirim bukti";
export const FIELD_PROOF_TITLE = "Bukti transfer dari penyewa";
export const FIELD_PROOF_EXPLAIN =
  "Foto layar m-banking atau struk ATM yang ditunjukkan penyewa. Admin mencocokkan dengan rekening usaha lalu mencatatnya; tagihan belum berubah.";
// The file input's accessible name, and the empty state's button.
export const FIELD_PROOF_PICK_TEXT = "Ambil foto bukti transfer";
export const FIELD_PROOF_PICK_NOTE = "Kamera atau galeri";
export const FIELD_PROOF_HINT =
  "Jumlah, tanggal dan rekening tujuan harus terbaca.";
export const FIELD_PROOF_SEND_TEXT = "Kirim bukti";
export const FIELD_PROOF_FAILED_TITLE = "Bukti belum terkirim";
export const FIELD_PROOF_NETWORK_TEXT = "Sinyal hilang. Foto masih di sini.";

// "Penyewa sudah transfer? Kirim bukti" shows to a member who may send a proof for some tenants
// or all (payment:proof; "conditional" shows it and the API decides per tenant). Never by role
// name.
export function canSendFieldProofs(ability: AppAbility): boolean {
  return ability.can("proof", "payment");
}

// What a failed send shows (createUpload, the PUT, completeUpload). A dropped connection is
// "Sinyal hilang. Foto masih di sini." with "Coba lagi"; the API refusing the file puts its
// message under the photo; the API refusing the member or not finding the tenant shows its
// message, with no retry; anything else shows its message with "Coba lagi".
export function fieldProofFailureView(error: unknown): FieldProofFailureView {
  const failure = classifyUploadFailure(error);
  switch (failure.kind) {
    case "cancelled":
      return failure;
    case "network":
      return { kind: "failed", message: FIELD_PROOF_NETWORK_TEXT };
    case "refused":
      return {
        kind: "error",
        error: { kind: "file", message: failure.message },
      };
    case "denied":
      return {
        kind: "error",
        error: { kind: "denied", message: failure.message },
      };
    case "other":
      return { kind: "failed", message: failure.message };
  }
}

// The photo on screen, if any.
export function fieldProofPhoto(state: FieldProofState): ChosenPhoto | null {
  switch (state.kind) {
    case "ready":
    case "sending":
    case "failed":
      return state.photo;
    case "empty":
    case "preparing":
      return null;
  }
}

// "Kirim bukti" works once a photo is ready and nothing refused it. After a failure it waits for
// "Coba lagi" (board, column 4).
export function canSendFieldProof(state: FieldProofState): boolean {
  return state.kind === "ready" && state.error === null;
}

// The primary's label: "Mengirim…" while the photo is sent, otherwise "Kirim bukti".
export function fieldProofPrimaryLabel(state: FieldProofState): string {
  return state.kind === "sending" ? PROOF_SENDING_TEXT : FIELD_PROOF_SEND_TEXT;
}

// The toast once completeUpload answered: the board's "Bukti terkirim. Admin akan mengeceknya.",
// as a short title and one line under it like the app's other toasts.
export const FIELD_PROOF_SENT_TITLE = "Bukti terkirim";
export const FIELD_PROOF_SENT_DESCRIPTION = "Admin akan mengeceknya.";

export function fieldProofSentToast(): ToastMessage {
  return {
    tone: "create",
    title: FIELD_PROOF_SENT_TITLE,
    description: FIELD_PROOF_SENT_DESCRIPTION,
  };
}
