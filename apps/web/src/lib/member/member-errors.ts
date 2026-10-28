import { ApiError } from "@/lib/api/errors";
import { toFormError, type FormError } from "@/lib/api/form-error";

// Where each members panel shows a failed request. The message is always the API's own (already
// Bahasa Indonesia); only its place differs, decided by the error's code and field.

// "Tambah anggota": invalid_phone and invalid_input on name, phone or roles go under that
// control. already_member names the phone field too, but it is not a typing mistake, so it goes
// in the alert at the top of the panel.
export function inviteFormError(error: unknown): FormError {
  if (error instanceof ApiError && error.code === "already_member") {
    return { field: null, message: error.message };
  }
  return toFormError(error, ["name", "phone", "roles"]);
}

// "Atur akses": invalid_input on roles goes under the roles; everything else (last_admin, 403,
// a member removed meanwhile) in the alert.
export function accessFormError(error: unknown): FormError {
  return toFormError(error, ["roles"]);
}

export const UNDELIVERABLE_HINT = "Periksa nomornya bersama anggota ini.";

// "Kirim ulang undangan". A number that cannot receive WhatsApp (422 whatsapp_undeliverable)
// will fail again whatever happens, and every try spends one of the number's few sends, so the
// row stops offering the resend and asks the admin to check the number instead. Rate limits and
// a WhatsApp outage can be tried again later.
export type ResendError = {
  message: string;
  canRetry: boolean;
};

export function resendError(error: unknown): ResendError {
  const { message } = toFormError(error, []);
  if (error instanceof ApiError && error.code === "whatsapp_undeliverable") {
    return { message: `${message} ${UNDELIVERABLE_HINT}`, canRetry: false };
  }
  return { message, canRetry: true };
}
