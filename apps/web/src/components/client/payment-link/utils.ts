import type { ToastMessage } from "@/components/client/toast/type";
import type { TenantOpenInvoice } from "@/lib/api/generated/model";
import type { AppAbility } from "@/lib/capabilities";

import type { ClipboardItemFactory, ClipboardWriter } from "./type";

// "Salin link bayar" (decision 39): each press makes a new link for the tenant's newest open
// invoice and copies it. The link is never kept: only its hash is stored by the API.

export const COPY_PAY_LINK_TEXT = "Salin link bayar";
// The toast once the link is on the clipboard; it never shows the link.
export const PAY_LINK_COPIED_TOAST: ToastMessage = {
  tone: "neutral",
  title: "Link bayar disalin",
  description: "Tempel di WhatsApp penyewa.",
  icon: "copy",
};

// The fallback when the browser cannot copy.
export const PAY_LINK_FALLBACK_LABEL = "Link bayar";
export const PAY_LINK_FALLBACK_HINT =
  "Link tidak bisa disalin otomatis. Salin link ini lalu tempel di WhatsApp penyewa. Link hanya tampil sekali.";
export const COPY_AGAIN_TEXT = "Salin";
export const CLOSE_TEXT = "Tutup";

// The member may share invoices and find the tenant's open ones (listTenantOpenInvoices needs
// invoice:read), on some records or all: "conditional" shows the button and the API decides per
// tenant and invoice (a karyawan sees only the tenants they collect). Never by role name.
export function canSharePayLink(ability: AppAbility): boolean {
  return ability.can("read", "invoice") && ability.can("share", "invoice");
}

// The invoice a new link is made for: the one with the latest period that still owes something.
// A final invoice's period is the lease's last day, so it is newer than that month's regular
// invoice. Equal periods (two leases) keep the later one in the API's order. Null when nothing
// is owed.
export function newestOpenInvoice(
  invoices: readonly TenantOpenInvoice[],
): TenantOpenInvoice | null {
  let newest: TenantOpenInvoice | null = null;
  for (const invoice of invoices) {
    if (invoice.balance <= 0) continue;
    if (newest === null || invoice.period >= newest.period) newest = invoice;
  }
  return newest;
}

// The invoice "Salin link bayar" shares, or null to hide the button: the member may share
// invoices, the tenant's open invoices have loaded (a refused or failed read never does), and
// something is owed. The API answers [] for a tenant the member may see none of.
export function payLinkInvoice(
  canShare: boolean,
  openInvoices: readonly TenantOpenInvoice[] | undefined,
): TenantOpenInvoice | null {
  if (!canShare || openInvoices === undefined) return null;
  return newestOpenInvoice(openInvoices);
}

// Copies the text; false when the browser has no clipboard or refuses (not a secure context, no
// permission, the press is too long ago), so the caller shows the text to copy by hand.
export async function copyToClipboard(
  clipboard: ClipboardWriter | undefined,
  text: string,
): Promise<boolean> {
  if (clipboard === undefined || typeof clipboard.writeText !== "function") {
    return false;
  }
  try {
    await clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

// Copies text the API has not sent yet. Safari (the iPad and iPhone) refuses a clipboard write
// that starts after an await, because the press no longer counts, so the write starts at once,
// inside the press, with a ClipboardItem whose text is the pending link. Where that is not
// available (or refused) it falls back to writeText once the text arrives. Never rejects: false
// means "show the text to copy by hand", including when the text itself fails (the caller
// handles that error).
export function copyPendingText(
  clipboard: ClipboardWriter | undefined,
  makeItem: ClipboardItemFactory | undefined,
  text: Promise<string>,
): Promise<boolean> {
  if (
    clipboard !== undefined &&
    makeItem !== undefined &&
    typeof clipboard.write === "function"
  ) {
    // Called before any await, so it runs within the press.
    let written: Promise<void>;
    try {
      written = clipboard.write([makeItem(text)]);
    } catch {
      written = Promise.reject(new Error("clipboard write refused"));
    }
    return written.then(
      () => true,
      () =>
        text.then(
          (value) => copyToClipboard(clipboard, value),
          () => false,
        ),
    );
  }
  return text.then(
    (value) => copyToClipboard(clipboard, value),
    () => false,
  );
}

// The browser's ClipboardItem for pending text, or undefined where there is none.
export function browserClipboardItem(): ClipboardItemFactory | undefined {
  if (typeof ClipboardItem === "undefined") return undefined;
  // A Blob, not a string: older Safari (the iPad) accepts only Blob values in a ClipboardItem.
  return (text) =>
    new ClipboardItem({
      "text/plain": text.then(
        (value) => new Blob([value], { type: "text/plain" }),
      ),
    });
}

// The browser's clipboard, or undefined where there is none.
export function browserClipboard(): ClipboardWriter | undefined {
  if (typeof navigator === "undefined") return undefined;
  // Typed as always present, but missing outside a secure context.
  return navigator.clipboard as ClipboardWriter | undefined;
}
