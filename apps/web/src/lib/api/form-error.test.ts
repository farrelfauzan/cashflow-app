import { describe, expect, it } from "vitest";

import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  NETWORK_ERROR_MESSAGE,
  networkError,
} from "./errors";
import { toFormError } from "./form-error";

const FIELDS = ["code"] as const;

describe("toFormError", () => {
  it.each([
    {
      name: "an error on a field of the form",
      error: new ApiError({
        status: 400,
        code: "invalid_code",
        message: "Kode salah atau sudah kedaluwarsa.",
        field: "code",
      }),
      want: { field: "code", message: "Kode salah atau sudah kedaluwarsa." },
    },
    {
      name: "an error on a field the form does not show",
      error: new ApiError({
        status: 400,
        code: "invalid_phone",
        message: "Nomor WhatsApp tidak valid.",
        field: "phone",
      }),
      want: { field: null, message: "Nomor WhatsApp tidak valid." },
    },
    {
      name: "an error without a field",
      error: new ApiError({
        status: 429,
        code: "rate_limited",
        message: "Terlalu banyak percobaan. Coba lagi dalam 15 menit.",
      }),
      want: {
        field: null,
        message: "Terlalu banyak percobaan. Coba lagi dalam 15 menit.",
      },
    },
    {
      name: "a network error",
      error: networkError(),
      want: { field: null, message: NETWORK_ERROR_MESSAGE },
    },
    {
      name: "something that is not an ApiError",
      error: new Error("boom: internal detail"),
      want: { field: null, message: GENERIC_ERROR_MESSAGE },
    },
  ])("puts $name in the right place", ({ error, want }) => {
    expect(toFormError(error, FIELDS)).toEqual(want);
  });
});
