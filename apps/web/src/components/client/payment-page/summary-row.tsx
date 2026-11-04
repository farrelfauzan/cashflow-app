"use client";

import type { SummaryRowView } from "./type";
import { EARLIER_MONTH_CHIP } from "./utils";

type SummaryRowProps = {
  row: SummaryRowView;
};

// One open invoice: its name (or what is left of it) and what it still owes.
export function SummaryRow({ row }: SummaryRowProps) {
  return (
    <li className="flex items-center justify-between gap-3">
      <span className="flex flex-wrap items-center gap-2">
        <span className="text-sm text-muted-foreground md:text-base">
          {row.label}
        </span>
        {row.fromEarlierMonth && (
          <span className="inline-flex min-h-7 items-center rounded-md bg-warning/15 px-2.5 text-sm font-semibold whitespace-nowrap text-foreground">
            {EARLIER_MONTH_CHIP}
          </span>
        )}
      </span>
      <span className="text-sm font-semibold whitespace-nowrap md:text-base">
        {row.amount}
      </span>
    </li>
  );
}
