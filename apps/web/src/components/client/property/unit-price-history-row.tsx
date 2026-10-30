"use client";

import { cn } from "@/lib/utils";
import type { UnitPriceHistoryRowView } from "./type";
import { FUTURE_PRICE_CHIP } from "./utils";

type UnitPriceHistoryRowProps = {
  row: UnitPriceHistoryRowView;
};

// One recorded price: the term and amount, when it starts and who recorded it, and whether it is
// in force or still to come.
export function UnitPriceHistoryRow({ row }: UnitPriceHistoryRowProps) {
  return (
    <li className="flex flex-col gap-1 border-t py-3 first:border-t-0">
      <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
        <span className="text-base font-semibold md:text-[17px]">
          {row.title}
        </span>
        {row.chip !== null && (
          <span
            className={cn(
              "inline-flex min-h-6 items-center rounded-lg px-2 text-sm font-semibold whitespace-nowrap",
              row.chip === FUTURE_PRICE_CHIP
                ? "bg-secondary text-foreground"
                : "bg-accent text-primary",
            )}
          >
            {row.chip}
          </span>
        )}
      </div>
      <span className="text-sm leading-[1.45] text-muted-foreground md:text-base">
        {row.detail}
      </span>
    </li>
  );
}
