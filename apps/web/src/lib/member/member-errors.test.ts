import { describe, expect, it } from "vitest";

import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  networkError,
} from "@/lib/api/errors";

import {
  accessFormError,
  inviteFormError,
  resendError,
  UNDELIVERABLE_HINT,
} from "./member-errors";

function apiError(
  status: number,
  code: string,
  message: string,
  field?: string,
): ApiError {
  return new ApiError({ status, code, message, field });
}

const FORBIDDEN = apiError(
  403,
  "forbidden",
  "Anda tidak punya akses untuk tindakan ini.",
);

describe("inviteFormError", () => {
  it.each([
    {
      name: "an invalid phone, under the phone",
      error: apiError(
        400,
        "invalid_phone",
        "Nomor WhatsApp tidak valid.",
        "phone",
      ),
      want: { field: "phone", message: "Nomor WhatsApp tidak valid." },
    },
    {
      name: "a missing name, under the name",
      error: apiError(400, "invalid_input", "Nama wajib diisi.", "name"),
      want: { field: "name", message: "Nama wajib diisi." },
    },
    {
      name: "an unknown property, under the roles",
      error: apiError(
        400,
        "invalid_input",
        "Properti tidak ditemukan.",
        "roles",
      ),
      want: { field: "roles", message: "Properti tidak ditemukan." },
    },
    {
      name: "a number that is already a member, in the alert",
      error: apiError(
        409,
        "already_member",
        "Nomor ini sudah terdaftar di usaha ini.",
        "phone",
      ),
      want: { field: null, message: "Nomor ini sudah terdaftar di usaha ini." },
    },
    {
      name: "a 403, in the alert",
      error: FORBIDDEN,
      want: { field: null, message: FORBIDDEN.message },
    },
    {
      name: "no connection, in the alert",
      error: networkError(),
      want: { field: null, message: networkError().message },
    },
  ])("puts $name", ({ error, want }) => {
    expect(inviteFormError(error)).toEqual(want);
  });
});

describe("accessFormError", () => {
  it.each([
    {
      name: "the last Admin, in the alert",
      error: apiError(
        409,
        "last_admin",
        "Anggota ini admin terakhir yang aktif. Tambahkan admin lain dulu.",
      ),
      want: {
        field: null,
        message:
          "Anggota ini admin terakhir yang aktif. Tambahkan admin lain dulu.",
      },
    },
    {
      name: "invalid roles, under the roles",
      error: apiError(
        400,
        "invalid_input",
        "Pilih minimal satu peran.",
        "roles",
      ),
      want: { field: "roles", message: "Pilih minimal satu peran." },
    },
    {
      name: "invalid input on another field, in the alert",
      error: apiError(
        400,
        "invalid_input",
        "Status harus active atau disabled.",
        "status",
      ),
      want: { field: null, message: "Status harus active atau disabled." },
    },
    {
      name: "a 403, in the alert",
      error: FORBIDDEN,
      want: { field: null, message: FORBIDDEN.message },
    },
    {
      name: "a bug, with the generic message",
      error: new Error("boom"),
      want: { field: null, message: GENERIC_ERROR_MESSAGE },
    },
  ])("puts $name", ({ error, want }) => {
    expect(accessFormError(error)).toEqual(want);
  });
});

describe("resendError", () => {
  it.each([
    {
      name: "an undeliverable number, without a retry",
      error: apiError(
        422,
        "whatsapp_undeliverable",
        "Nomor ini tidak bisa menerima WhatsApp.",
        "phone",
      ),
      want: {
        message: `Nomor ini tidak bisa menerima WhatsApp. ${UNDELIVERABLE_HINT}`,
        canRetry: false,
      },
    },
    {
      name: "a rate limit",
      error: apiError(429, "rate_limited", "Terlalu sering. Coba lagi nanti."),
      want: { message: "Terlalu sering. Coba lagi nanti.", canRetry: true },
    },
    {
      name: "WhatsApp down",
      error: apiError(
        503,
        "whatsapp_unavailable",
        "WhatsApp gagal mengirim. Coba lagi.",
      ),
      want: { message: "WhatsApp gagal mengirim. Coba lagi.", canRetry: true },
    },
    {
      name: "a disabled member",
      error: apiError(409, "member_disabled", "Anggota ini sedang nonaktif."),
      want: { message: "Anggota ini sedang nonaktif.", canRetry: true },
    },
  ])("shows $name", ({ error, want }) => {
    expect(resendError(error)).toEqual(want);
  });
});
