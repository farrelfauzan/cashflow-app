"use client";

import { cn } from "@/lib/utils";
import type { RowInvoicesView } from "./type";
import { MANY_INVOICES_HINT } from "./utils";

type ImportRowInvoicesProps = {
  invoices: RowInvoicesView;
};

// "Langsung ditagih: 2 tagihan, Rp2.600.000" under a row. A row that bills many periods at once
// shows it as a warning, with the hint to fill mulai_tagih.
export function ImportRowInvoices({ invoices }: ImportRowInvoicesProps) {
  return (
    <div className="flex flex-col gap-0.5">
      <span
        className={cn(
          "text-sm md:text-base",
          invoices.many
            ? "font-semibold text-warning"
            : "text-muted-foreground",
        )}
      >
        {invoices.text}
      </span>
      {invoices.many && (
        <span className="text-sm leading-[1.45] text-warning md:text-base">
          {MANY_INVOICES_HINT}
        </span>
      )}
    </div>
  );
}
