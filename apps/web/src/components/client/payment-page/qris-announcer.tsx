"use client";

type QrisAnnouncerProps = {
  text: string;
};

// One live region that stays on the page while the QRIS card changes, so a screen reader hears
// every change ("Membuat QR…", "Menunggu pembayaran…", "QR kedaluwarsa", "Pembayaran sedang
// dicek"). The countdown is never put here: it would speak every second.
export function QrisAnnouncer({ text }: QrisAnnouncerProps) {
  return (
    <p role="status" aria-live="polite" className="sr-only">
      {text}
    </p>
  );
}
