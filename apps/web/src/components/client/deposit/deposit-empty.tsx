"use client";

import { HandCoins } from "lucide-react";

import { EMPTY_TEXT } from "./utils";

// Nothing held and nothing waiting for an admin: a calm line, no button.
export function DepositEmpty() {
  return (
    <section
      aria-label="Tunai di tangan"
      className="flex items-center gap-3 rounded-[14px] border bg-card px-4 py-4"
    >
      <HandCoins aria-hidden="true" className="size-6 shrink-0 text-primary" />
      <p className="text-base">{EMPTY_TEXT}</p>
    </section>
  );
}
