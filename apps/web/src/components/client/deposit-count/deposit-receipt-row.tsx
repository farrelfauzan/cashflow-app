"use client";

import type { DepositReceiptView } from "./type";

type DepositReceiptRowProps = {
  view: DepositReceiptView;
};

// One payment in the deposit: the tenant, their units and when, the amount and its photo as text.
export function DepositReceiptRow({ view }: DepositReceiptRowProps) {
  return (
    <li className="flex justify-between gap-2.5 border-t px-4 py-3 first:border-t-0">
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-base font-semibold">{view.tenantName}</span>
        <span className="text-sm text-muted-foreground md:text-base">
          {view.detail}
        </span>
      </div>
      <div className="flex shrink-0 flex-col items-end gap-0.5">
        <span className="text-base font-semibold tabular-nums">
          {view.amount}
        </span>
        <span className="text-sm text-muted-foreground md:text-base">
          {view.photo}
        </span>
      </div>
    </li>
  );
}
