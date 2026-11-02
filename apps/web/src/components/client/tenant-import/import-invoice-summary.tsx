"use client";

import { TriangleAlert } from "lucide-react";

import { cn } from "@/lib/utils";
import type { InvoiceSummaryView } from "./type";

type ImportInvoiceSummaryProps = {
  invoices: InvoiceSummaryView;
};

// "Tagihan yang langsung dibuat": what saving the file bills at once, from the API's dry run. In
// the warning style when some rows bill many periods.
export function ImportInvoiceSummary({ invoices }: ImportInvoiceSummaryProps) {
  const warning = invoices.warning !== null;
  return (
    <div
      className={cn(
        "flex flex-col gap-1 rounded-xl border px-3.5 py-3",
        warning ? "border-warning/40 bg-warning/10" : "bg-card",
      )}
    >
      <span
        className={cn(
          "text-[15px] md:text-base",
          warning ? "text-warning" : "text-muted-foreground",
        )}
      >
        Tagihan yang langsung dibuat
      </span>
      <span
        className={cn(
          "text-2xl leading-tight font-bold md:text-[28px]",
          warning && "text-warning",
        )}
      >
        {invoices.total}
      </span>
      <span className="text-sm leading-normal text-muted-foreground md:text-base">
        {invoices.countText}
      </span>
      {invoices.warning !== null && (
        <p className="flex items-start gap-1.5 text-sm leading-normal font-semibold text-warning md:text-base">
          <TriangleAlert
            aria-hidden="true"
            className="mt-0.5 size-4 shrink-0"
            strokeWidth={2.2}
          />
          {invoices.warning}
        </p>
      )}
    </div>
  );
}
