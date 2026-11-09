import { ApiError } from "@/lib/api/errors";
import { toFormError } from "@/lib/api/form-error";
import { TELEGRAM_UNAVAILABLE_CODE } from "@/components/client/payment-page/utils";

import type { StaffTelegramErrorView, TelegramMenuItemVariant } from "./type";

// A staff member's Telegram (decision 42, demo only): offered in the side menu only where the
// environment has a bot (getMyTelegram's available), which production never has. A linked member
// gets their login codes and notifications there instead of WhatsApp, and leaves with
// "Putuskan Telegram" here or /stop in the bot. The link steps (deep link check, its 10-minute
// expiry, the button texts) are the tenant payment page's.

export const STAFF_TELEGRAM_MENU_TEXT = "Telegram";
export const STAFF_TELEGRAM_TITLE = "Telegram";
export const STAFF_TELEGRAM_TEXT =
  "Terima kode masuk dan pemberitahuan di Telegram, bukan di WhatsApp.";
export const STAFF_TELEGRAM_LINKED_TEXT =
  "Telegram terhubung. Kode masuk berikutnya dikirim di sini.";
export const STAFF_TELEGRAM_UNLINK_TEXT = "Putuskan Telegram";
export const STAFF_TELEGRAM_UNLINKING_TEXT = "Memutuskan…";

// Staff screens show the API's own message (already Bahasa Indonesia): too many requests, a
// request from another site, or the generic one when no answer came.
export function staffTelegramErrorView(error: unknown): StaffTelegramErrorView {
  if (error instanceof ApiError && error.code === TELEGRAM_UNAVAILABLE_CODE) {
    return { kind: "hidden" };
  }
  return { kind: "alert", message: toFormError(error, []).message };
}

// "Telegram" per place it sits in.
export const TELEGRAM_MENU_ITEM_CLASS: Record<TelegramMenuItemVariant, string> =
  {
    menu: "justify-start px-2 font-semibold text-muted-foreground rail:justify-center rail:px-0",
    sheet:
      "min-h-14 w-full justify-start gap-3.5 rounded-none border-b px-1 text-[17px] font-semibold text-foreground has-[>svg]:px-1 [&>svg:first-child]:size-[22px]! [&_svg]:text-muted-foreground",
  };
