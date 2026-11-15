"use client";

import { TABLE_COLUMN_GAP } from "@/components/client/list/utils";
import { cn } from "@/lib/utils";

import type { PendingDepositRowView } from "./type";
import {
  PENDING_COLUMNS,
  PENDING_COLUMNS_WITH_ACTION,
  PENDING_LIST_LABEL,
} from "./utils";

import { PendingDepositCard } from "./pending-deposit-card";
import { PendingDepositTableRow } from "./pending-deposit-table-row";

type PendingDepositListProps = {
  rows: readonly PendingDepositRowView[];
  // Whether the member may count: the table then has the "Hitung" column.
  canCount: boolean;
  onCount: (depositId: string) => void;
};

// The deposits waiting to be counted, newest first (the API's order): a table on iPad, cards on
// the phone. Only one of the two is displayed.
export function PendingDepositList({
  rows,
  canCount,
  onCount,
}: PendingDepositListProps) {
  const columns = canCount ? PENDING_COLUMNS_WITH_ACTION : PENDING_COLUMNS;
  return (
    <>
      <section className="hidden overflow-hidden rounded-2xl border bg-card lg:block">
        <div
          aria-hidden="true"
          className={cn(
            "grid min-h-11 items-center border-b bg-muted/40 px-5 text-base font-semibold text-muted-foreground",
            columns,
            TABLE_COLUMN_GAP,
          )}
        >
          <span>Karyawan</span>
          <span className="text-right">Kwitansi</span>
          <span className="text-right">Total kwitansi</span>
          <span className="text-right">Diserahkan</span>
          {canCount && <span />}
        </div>
        <ul aria-label={PENDING_LIST_LABEL}>
          {rows.map((row) => (
            <PendingDepositTableRow
              key={row.id}
              row={row}
              columns={columns}
              onCount={() => onCount(row.id)}
            />
          ))}
        </ul>
      </section>

      <ul
        aria-label={PENDING_LIST_LABEL}
        className="flex flex-col gap-2.5 lg:hidden"
      >
        {rows.map((row) => (
          <PendingDepositCard
            key={row.id}
            row={row}
            onCount={() => onCount(row.id)}
          />
        ))}
      </ul>
    </>
  );
}
