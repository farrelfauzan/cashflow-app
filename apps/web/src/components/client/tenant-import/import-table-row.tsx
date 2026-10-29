"use client";

import { cn } from "@/lib/utils";
import type { ImportRowView } from "./type";

import { ImportRowChip } from "./import-row-chip";
import { ImportRowMessages } from "./import-row-messages";

type ImportTableRowProps = {
  row: ImportRowView;
};

// One checked row in the iPad table (board Impor-Cek.dc.html): its line, who and where, and what
// happens to it, with its problems and notes underneath.
export function ImportTableRow({ row }: ImportTableRowProps) {
  return (
    <li
      className={cn(
        "flex flex-col gap-1.5 border-t px-4 py-3 first:border-t-0",
        row.status.tone === "error" && "bg-destructive/5",
      )}
    >
      <div className="grid grid-cols-[72px_minmax(0,1fr)_220px] items-start gap-3">
        <span className="pt-0.5 text-base font-semibold">{row.lineLabel}</span>
        <div className="flex min-w-0 flex-col gap-0.5">
          <span className="text-base font-semibold">{row.tenantName}</span>
          <span className="text-base text-muted-foreground">
            {row.placeText}
          </span>
          {row.datesText !== null && (
            <span className="text-base text-muted-foreground">
              {row.datesText}
            </span>
          )}
        </div>
        <div className="flex flex-col items-start gap-1.5">
          <ImportRowChip tone={row.status.tone}>
            {row.status.label}
          </ImportRowChip>
          {row.createdChips.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {row.createdChips.map((chip) => (
                <ImportRowChip key={chip} tone="neutral">
                  {chip}
                </ImportRowChip>
              ))}
            </div>
          )}
        </div>
      </div>
      <div className="pl-[84px]">
        <ImportRowMessages row={row} />
      </div>
    </li>
  );
}
