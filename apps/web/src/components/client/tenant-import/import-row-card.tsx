"use client";

import { cn } from "@/lib/utils";
import type { ImportRowView } from "./type";

import { ImportRowChip } from "./import-row-chip";
import { ImportRowMessages } from "./import-row-messages";

type ImportRowCardProps = {
  row: ImportRowView;
};

// One checked row as a card on the phone (board Impor-Cek-Phone.dc.html).
export function ImportRowCard({ row }: ImportRowCardProps) {
  return (
    <li
      className={cn(
        "flex flex-col gap-1.5 rounded-2xl border bg-card px-3.5 py-3",
        row.status.tone === "error" && "border-destructive/40",
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-bold text-muted-foreground">
          {row.lineLabel}
        </span>
        <ImportRowChip tone={row.status.tone}>{row.status.label}</ImportRowChip>
      </div>
      <span className="text-base font-semibold">{row.tenantName}</span>
      <span className="text-sm text-muted-foreground">{row.placeText}</span>
      {row.datesText !== null && (
        <span className="text-sm text-muted-foreground">{row.datesText}</span>
      )}
      {row.createdChips.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          {row.createdChips.map((chip) => (
            <ImportRowChip key={chip} tone="neutral">
              {chip}
            </ImportRowChip>
          ))}
        </div>
      )}
      <ImportRowMessages row={row} />
    </li>
  );
}
