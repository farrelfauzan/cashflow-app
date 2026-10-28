import { ApiError, GENERIC_ERROR_MESSAGE } from "./errors";

// Where a form shows a failed request: under one of its own fields, or (field null) in the
// alert at the top of the form. The message is the API's own text, already in Bahasa Indonesia.
export type FormError = {
  field: string | null;
  message: string;
};

// `fields` are the API field names this form shows. An error naming another field (or none)
// goes to the alert, so it is never lost.
export function toFormError(
  error: unknown,
  fields: readonly string[],
): FormError {
  if (!(error instanceof ApiError)) {
    return { field: null, message: GENERIC_ERROR_MESSAGE };
  }
  const field =
    error.field !== undefined && fields.includes(error.field)
      ? error.field
      : null;
  return { field, message: error.message };
}
