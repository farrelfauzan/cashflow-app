"use client";

import type { PendingDepositRowView } from "./type";

import { CountActionCell } from "./count-action-cell";

type PendingDepositCardProps = {
  row: PendingDepositRowView;
  onCount: () => void;
};

// One deposit waiting on the phone: the same facts as the iPad row, "Hitung" full width below.
export function PendingDepositCard({ row, onCount }: PendingDepositCardProps) {
  return (
    <li className="flex flex-col gap-1.5 rounded-[14px] border bg-card px-4 py-3.5">
      <h3 className="text-[17px] font-semibold">{row.staffName}</h3>
      <span className="text-sm text-muted-foreground">{row.recordedLine}</span>
      <span className="text-[15px]">
        {row.paymentCount} kwitansi · Total {row.expected}
      </span>
      <span className="text-[15px]">
        Diserahkan <span className="font-semibold">{row.handed}</span>
        {row.handedNote !== null && (
          <span className="font-semibold text-warning">
            {" "}
            · {row.handedNote}
          </span>
        )}
      </span>
      <CountActionCell
        action={row.action}
        staffName={row.staffName}
        onCount={onCount}
        className="mt-1 w-full"
      />
    </li>
  );
}
