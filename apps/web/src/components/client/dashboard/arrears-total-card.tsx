"use client";

import { cn } from "@/lib/utils";
import type { ArrearsTotalView } from "./type";
import { ARREARS_TOTAL_TITLE } from "./utils";

type ArrearsTotalCardProps = {
  total: ArrearsTotalView;
};

// "Tunggakan": everything still owed after its due date, whatever the month.
export function ArrearsTotalCard({ total }: ArrearsTotalCardProps) {
  return (
    <div
      role="group"
      aria-label={ARREARS_TOTAL_TITLE}
      className="flex min-w-0 flex-col gap-2.5 rounded-[14px] border bg-card p-4 md:rounded-2xl md:px-6 md:py-[22px]"
    >
      <p className="text-base font-semibold text-muted-foreground md:text-[19px]">
        {ARREARS_TOTAL_TITLE}
      </p>
      <p
        className={cn(
          "text-[28px] leading-tight font-bold tracking-tight md:text-[40px]",
          total.tone === "warning" && "text-warning",
        )}
      >
        {total.value}
      </p>
      <p className="text-[15px] text-muted-foreground md:text-lg">
        {total.note}
      </p>
    </div>
  );
}
