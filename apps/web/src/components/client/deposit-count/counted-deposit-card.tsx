"use client";

import type { CountedDepositRowView } from "./type";

import { CountedResult } from "./counted-result";

type CountedDepositCardProps = {
  row: CountedDepositRowView;
};

// One counted deposit on the phone: the same facts as the iPad row.
export function CountedDepositCard({ row }: CountedDepositCardProps) {
  return (
    <li className="flex flex-col gap-1.5 rounded-[14px] border bg-card px-4 py-3.5">
      <h3 className="text-[17px] font-semibold">{row.staffName}</h3>
      <span className="text-sm text-muted-foreground">{row.recordedLine}</span>
      <span className="text-[15px]">
        Total kwitansi {row.expected} · {row.receipts}
      </span>
      <span className="text-[15px]">
        Diterima admin <span className="font-semibold">{row.received}</span>
      </span>
      <CountedResult row={row} />
      <span className="text-sm text-muted-foreground">
        Dihitung {row.countedBy} · {row.countedAt}
      </span>
    </li>
  );
}
