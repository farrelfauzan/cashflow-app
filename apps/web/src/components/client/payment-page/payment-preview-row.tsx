"use client";

import { cn } from "@/lib/utils";

import type { PreviewRowView } from "./type";

type PaymentPreviewRowProps = {
  row: PreviewRowView;
};

// One invoice the amount reaches: its month, "Lunas" or what it would still owe, and what the
// amount pays on it.
export function PaymentPreviewRow({ row }: PaymentPreviewRowProps) {
  return (
    <li className="flex items-center justify-between gap-3">
      <span className="flex flex-wrap items-center gap-2">
        <span className="text-sm md:text-base">{row.name}</span>
        <span
          className={cn(
            "inline-flex min-h-7 items-center rounded-md px-2.5 text-sm font-semibold whitespace-nowrap",
            row.paidInFull
              ? "bg-accent text-accent-foreground"
              : "bg-warning/15 text-foreground",
          )}
        >
          {row.chip}
        </span>
      </span>
      <span className="text-sm font-semibold whitespace-nowrap md:text-base">
        {row.applied}
      </span>
    </li>
  );
}
