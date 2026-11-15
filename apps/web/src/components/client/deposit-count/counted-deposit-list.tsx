"use client";

import { TABLE_COLUMN_GAP } from "@/components/client/list/utils";
import { cn } from "@/lib/utils";

import type { CountedDepositRowView } from "./type";
import { CONFIRMED_COLUMNS, CONFIRMED_LIST_LABEL } from "./utils";

import { CountedDepositCard } from "./counted-deposit-card";
import { CountedDepositTableRow } from "./counted-deposit-table-row";

type CountedDepositListProps = {
  rows: readonly CountedDepositRowView[];
};

// The counted deposits, newest first (the API's order): a table on iPad, cards on the phone. Only
// one of the two is displayed.
export function CountedDepositList({ rows }: CountedDepositListProps) {
  return (
    <>
      <section className="hidden overflow-hidden rounded-2xl border bg-card lg:block">
        <div
          aria-hidden="true"
          className={cn(
            "grid min-h-11 items-center border-b bg-muted/40 px-5 text-base font-semibold text-muted-foreground",
            CONFIRMED_COLUMNS,
            TABLE_COLUMN_GAP,
          )}
        >
          <span>Karyawan</span>
          <span className="text-right">Total kwitansi</span>
          <span className="text-right">Diterima admin</span>
          <span>Hasil</span>
          <span>Dihitung oleh</span>
        </div>
        <ul aria-label={CONFIRMED_LIST_LABEL}>
          {rows.map((row) => (
            <CountedDepositTableRow key={row.id} row={row} />
          ))}
        </ul>
      </section>

      <ul
        aria-label={CONFIRMED_LIST_LABEL}
        className="flex flex-col gap-2.5 lg:hidden"
      >
        {rows.map((row) => (
          <CountedDepositCard key={row.id} row={row} />
        ))}
      </ul>
    </>
  );
}
