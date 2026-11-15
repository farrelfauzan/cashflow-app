"use client";

import { TABLE_COLUMN_GAP } from "@/components/client/list/utils";
import { cn } from "@/lib/utils";

import type { PendingDepositRowView } from "./type";

import { CountActionCell } from "./count-action-cell";

type PendingDepositTableRowProps = {
  row: PendingDepositRowView;
  // The grid of the header in pending-deposit-list.tsx.
  columns: string;
  onCount: () => void;
};

// One deposit waiting on iPad: the karyawan and when, the receipts, their total, what was handed
// over (with how it differs, when it does), then "Hitung".
export function PendingDepositTableRow({
  row,
  columns,
  onCount,
}: PendingDepositTableRowProps) {
  return (
    <li
      className={cn(
        "grid items-center border-t px-5 py-3 first:border-t-0",
        columns,
        TABLE_COLUMN_GAP,
      )}
    >
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-[17px] font-semibold">{row.staffName}</span>
        <span className="text-base text-muted-foreground">
          {row.recordedLine}
        </span>
      </div>
      <span className="text-right text-base tabular-nums">
        {row.paymentCount}
      </span>
      <span className="text-right text-base tabular-nums">{row.expected}</span>
      <div className="flex flex-col items-end gap-0.5">
        <span className="text-base font-semibold tabular-nums">
          {row.handed}
        </span>
        {row.handedNote !== null && (
          <span className="text-base font-semibold text-warning">
            {row.handedNote}
          </span>
        )}
      </div>
      {row.action !== "none" && (
        <div className="flex justify-end">
          <CountActionCell
            action={row.action}
            staffName={row.staffName}
            onCount={onCount}
            className="text-right"
          />
        </div>
      )}
    </li>
  );
}
