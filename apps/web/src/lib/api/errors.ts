// The one error type of the API client. The Go API answers every error as
// {"error":{"code","message","field"}} with a message already in Bahasa Indonesia; screens branch
// on `code`, show `message` and put it under the form field named by `field`.

export const GENERIC_ERROR_MESSAGE = "Terjadi kesalahan. Coba lagi.";
export const NETWORK_ERROR_MESSAGE =
  "Tidak dapat terhubung ke server. Periksa koneksi lalu coba lagi.";

// Codes the client produces itself, next to the API's own codes.
export const UNKNOWN_ERROR_CODE = "unknown";
export const NETWORK_ERROR_CODE = "network_error";

type ApiErrorInit = {
  status: number;
  code: string;
  message: string;
  field?: string;
};

export class ApiError extends Error {
  // HTTP status, or 0 when no response arrived (network_error).
  readonly status: number;
  readonly code: string;
  readonly field?: string;

  constructor({ status, code, message, field }: ApiErrorInit) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    if (field !== undefined) {
      this.field = field;
    }
  }
}

function nonEmptyString(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() !== "" ? value : undefined;
}

// Builds the ApiError for a non-2xx response from its raw body. Anything that is not the API's
// error shape (an HTML page from a proxy, plain text, broken JSON) becomes code "unknown" with
// a generic message, so raw text never reaches the screen.
export function apiErrorFromResponse(status: number, body: string): ApiError {
  let parsed: unknown;
  try {
    parsed = JSON.parse(body);
  } catch {
    parsed = undefined;
  }

  const error =
    typeof parsed === "object" && parsed !== null && "error" in parsed
      ? (parsed as { error: unknown }).error
      : undefined;
  if (typeof error !== "object" || error === null) {
    return new ApiError({
      status,
      code: UNKNOWN_ERROR_CODE,
      message: GENERIC_ERROR_MESSAGE,
    });
  }

  const { code, message, field } = error as Record<string, unknown>;
  const parsedCode = nonEmptyString(code);
  if (parsedCode === undefined) {
    return new ApiError({
      status,
      code: UNKNOWN_ERROR_CODE,
      message: GENERIC_ERROR_MESSAGE,
    });
  }
  return new ApiError({
    status,
    code: parsedCode,
    message: nonEmptyString(message) ?? GENERIC_ERROR_MESSAGE,
    field: nonEmptyString(field),
  });
}

// A request that got no response at all (offline, DNS, connection refused).
export function networkError(): ApiError {
  return new ApiError({
    status: 0,
    code: NETWORK_ERROR_CODE,
    message: NETWORK_ERROR_MESSAGE,
  });
}

// A 2xx response whose body is not the JSON the contract promises.
export function unexpectedResponseError(status: number): ApiError {
  return new ApiError({
    status,
    code: UNKNOWN_ERROR_CODE,
    message: GENERIC_ERROR_MESSAGE,
  });
}
