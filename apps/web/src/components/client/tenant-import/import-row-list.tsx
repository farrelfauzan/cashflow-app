"use client";

import { TABLE_COLUMN_GAP } from "@/components/client/list/utils";
import { cn } from "@/lib/utils";
import type { ImportRowView } from "./type";
import { ROW_LIST_LABEL } from "./utils";

import { ImportRowCard } from "./import-row-card";
import { ImportTableRow } from "./import-table-row";

type ImportRowListProps = {
  rows: readonly ImportRowView[];
};

// The checked rows, in the file's order: a table on iPad, cards on the phone (boards
// Impor-Cek.dc.html and Impor-Cek-Phone.dc.html). Only one of the two is displayed.
export function ImportRowList({ rows }: ImportRowListProps) {
  return (
    <>
      <section className="hidden overflow-hidden rounded-2xl border bg-card lg:block">
        <div
          aria-hidden="true"
          className={cn(
            "grid min-h-12 grid-cols-[72px_minmax(0,1fr)_220px] items-center border-b bg-muted/40 px-4 text-base font-semibold text-muted-foreground",
            TABLE_COLUMN_GAP,
          )}
        >
          <span>Baris</span>
          <span>Penyewa dan unit</span>
          <span>Hasil</span>
        </div>
        <ul aria-label={ROW_LIST_LABEL}>
          {rows.map((row) => (
            <ImportTableRow key={row.line} row={row} />
          ))}
        </ul>
      </section>
      <ul
        aria-label={ROW_LIST_LABEL}
        className="flex flex-col gap-2.5 lg:hidden"
      >
        {rows.map((row) => (
          <ImportRowCard key={row.line} row={row} />
        ))}
      </ul>
    </>
  );
}
