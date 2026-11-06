"use client";

import { FOOTER_TEXT } from "./utils";

// "Halaman ini menunjukkan Lunas setelah pembayaran dicatat." at the bottom of the page.
export function PaymentPageFooter() {
  return (
    <p className="mt-auto rounded-xl bg-accent px-4 py-3 text-sm text-foreground md:text-base">
      {FOOTER_TEXT}
    </p>
  );
}
