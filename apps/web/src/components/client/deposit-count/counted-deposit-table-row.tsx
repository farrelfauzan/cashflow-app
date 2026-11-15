"use client";

import { TABLE_COLUMN_GAP } from "@/components/client/list/utils";
import { cn } from "@/lib/utils";

import type { CountedDepositRowView } from "./type";
import { CONFIRMED_COLUMNS } from "./utils";

import { CountedResult } from "./counted-result";

type CountedDepositTableRowProps = {
  row: CountedDepositRowView;
};

// One counted deposit on iPad: the karyawan and when, the receipts' total and count, what the
// admin received, the result and who counted it when. The columns match the header in
// counted-deposit-list.tsx.
export function CountedDepositTableRow({ row }: CountedDepositTableRowProps) {
  return (
    <li
      className={cn(
        "grid items-center border-t px-5 py-3 first:border-t-0",
        CONFIRMED_COLUMNS,
        TABLE_COLUMN_GAP,
      )}
    >
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-[17px] font-semibold">{row.staffName}</span>
        <span className="text-base text-muted-foreground">
          {row.recordedLine}
        </span>
      </div>
      <div className="flex flex-col items-end gap-0.5">
        <span className="text-base tabular-nums">{row.expected}</span>
        <span className="text-base text-muted-foreground">{row.receipts}</span>
      </div>
      <span className="text-right text-base font-semibold tabular-nums">
        {row.received}
      </span>
      <CountedResult row={row} />
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-base">{row.countedBy}</span>
        <span className="text-base text-muted-foreground">{row.countedAt}</span>
      </div>
    </li>
  );
}
