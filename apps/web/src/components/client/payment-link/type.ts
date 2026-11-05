// "Salin link bayar" on Detail penyewa: a new payment link for the tenant's newest open invoice,
// copied for the member to paste in WhatsApp.

// The clipboard calls the copy needs; `navigator.clipboard` in the browser. Missing in an
// insecure context or an old browser; `write` is missing where ClipboardItem is.
export type ClipboardWriter = Pick<Clipboard, "writeText"> &
  Partial<Pick<Clipboard, "write">>;

// Builds the clipboard item for text that is still on its way; `ClipboardItem` in the browser.
export type ClipboardItemFactory = (text: Promise<string>) => ClipboardItem;

// What the page needs from a press of "Salin link bayar".
export type CopyPayLinkState = {
  // A link is being made or copied: the button is disabled, so one press makes one link.
  busy: boolean;
  // The API's message when the link could not be made; null otherwise.
  error: string | null;
  // The link, only when the browser could not copy it: shown once until "Tutup". Never stored
  // anywhere else (decision 39: only its hash is kept, so it cannot be shown again).
  fallbackUrl: string | null;
  // Makes a new link for the invoice and copies it.
  copy: (invoiceId: string) => void;
  // "Salin" in the fallback: tries the clipboard again with the link already made.
  copyAgain: () => void;
  // "Tutup": drops the link.
  close: () => void;
};
