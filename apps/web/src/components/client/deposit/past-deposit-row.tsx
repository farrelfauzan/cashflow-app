"use client";

import { TenantChip } from "@/components/client/tenant/tenant-chip";

import type { PastDepositView } from "./type";

type PastDepositRowProps = {
  view: PastDepositView;
};

// One counted deposit: its day, who counted it or how it differed, what was counted, and
// "Cocok" or the gap.
export function PastDepositRow({ view }: PastDepositRowProps) {
  return (
    <li className="flex justify-between gap-2.5 border-t px-4 py-3 first:border-t-0">
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-base font-semibold">{view.date}</span>
        <span className="text-sm text-muted-foreground md:text-base">
          {view.detail}
        </span>
      </div>
      <div className="flex shrink-0 flex-col items-end gap-1">
        <span className="text-base font-semibold tabular-nums">
          {view.amount}
        </span>
        <TenantChip tone={view.chip.tone}>{view.chip.label}</TenantChip>
      </div>
    </li>
  );
}
