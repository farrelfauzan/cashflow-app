import { z } from "zod";

import { isCompleteLoginCode } from "@/lib/auth/login-code";

export const EMPTY_PHONE_MESSAGE = "Isi nomor WhatsApp Anda.";
export const CODE_FORMAT_MESSAGE = "Kode harus 6 angka.";
export const CODE_RESENT_MESSAGE = "Kode baru sudah dikirim.";

// The fields of each step an API error can name.
export const LOGIN_PHONE_API_FIELDS = ["phone"] as const;
export const LOGIN_CODE_API_FIELDS = ["code"] as const;

// Step 1: a number is needed; the API decides whether it is one, and answers the same whether
// or not it is registered.
export const loginPhoneSchema = z.object({
  phone: z.string().trim().min(1, EMPTY_PHONE_MESSAGE),
});

// Step 2: six digits (the field keeps digits only); the API decides whether the code is right.
export const loginCodeSchema = z.object({
  code: z.string().refine(isCompleteLoginCode, CODE_FORMAT_MESSAGE),
});
