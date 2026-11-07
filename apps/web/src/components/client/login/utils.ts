import { z } from "zod";

import { isCompleteLoginCode } from "@/lib/auth/login-code";

import type { CodeSentEnding, LoginCodeChannel } from "./type";

export const EMPTY_PHONE_MESSAGE = "Isi nomor WhatsApp Anda.";
export const CODE_FORMAT_MESSAGE = "Kode harus 6 angka.";
export const CODE_RESENT_MESSAGE = "Kode baru sudah dikirim.";
export const CODE_RESENT_WHATSAPP_MESSAGE = "Kode baru dikirim lewat WhatsApp.";

// The quiet "send it over WhatsApp instead" button, shown only when Telegram is available.
export const SEND_WHATSAPP_LABEL = "Kirim lewat WhatsApp";
export const SENDING_WHATSAPP_LABEL = "Mengirim lewat WhatsApp…";

// telegramAvailable is the API's environment setting (identical for every number), and
// whatsappOnly says the latest code was asked for with "Kirim lewat WhatsApp".
export function loginCodeChannel(
  telegramAvailable: boolean,
  whatsappOnly: boolean,
): LoginCodeChannel {
  return telegramAvailable && !whatsappOnly
    ? "telegram-or-whatsapp"
    : "whatsapp";
}

// "…terdaftar, kode 6 angka sudah dikirim …". The WhatsApp wording is the screen's original.
export function codeSentEnding(channel: LoginCodeChannel): CodeSentEnding {
  return channel === "telegram-or-whatsapp"
    ? {
        lead: "terdaftar, kode 6 angka sudah dikirim lewat Telegram atau",
        tail: "WhatsApp.",
      }
    : {
        lead: "terdaftar, kode 6 angka sudah dikirim ke",
        tail: "WhatsApp-nya.",
      };
}

export function codeFieldLabel(channel: LoginCodeChannel): string {
  return channel === "telegram-or-whatsapp"
    ? "Kode masuk"
    : "Kode dari WhatsApp";
}

// The status line after a successful resend.
export function codeResentMessage(whatsappOnly: boolean): string {
  return whatsappOnly ? CODE_RESENT_WHATSAPP_MESSAGE : CODE_RESENT_MESSAGE;
}

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
