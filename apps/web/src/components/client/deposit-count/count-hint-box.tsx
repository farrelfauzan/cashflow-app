"use client";

import { Check, TriangleAlert } from "lucide-react";

import type { CountHint } from "./type";

type CountHintBoxProps = {
  // null while the field holds no amount: the live region stays, empty.
  hint: CountHint | null;
};

// The typed count against the receipts' total, as the admin types: "Cocok dengan total
// kwitansi." in the accent, or the gap in a warning box. A hint only; the saved gap is the API's.
export function CountHintBox({ hint }: CountHintBoxProps) {
  return (
    <div aria-live="polite">
      {hint !== null &&
        (hint.tone === "match" ? (
          <p className="flex items-center gap-2 text-[15px] font-semibold text-primary md:text-base">
            <Check aria-hidden="true" className="size-5 shrink-0" />
            {hint.text}
          </p>
        ) : (
          <div className="flex items-start gap-2.5 rounded-xl border border-warning bg-warning/15 px-3.5 py-3 text-foreground">
            <TriangleAlert
              aria-hidden="true"
              className="mt-0.5 size-5 shrink-0 text-warning"
            />
            <p className="text-[15px] leading-normal font-semibold md:text-base">
              {hint.text}
            </p>
          </div>
        ))}
    </div>
  );
}
