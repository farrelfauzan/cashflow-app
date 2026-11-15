"use client";

import { TenantChip } from "@/components/client/tenant/tenant-chip";
import { cn } from "@/lib/utils";

import type { CountedDepositRowView } from "./type";

type CountedResultProps = {
  row: CountedDepositRowView;
  className?: string;
};

// "Cocok", or the gap with where it is now ("Di Antrean cek", "Selisih ditutup").
export function CountedResult({ row, className }: CountedResultProps) {
  return (
    <div className={cn("flex flex-col items-start gap-1", className)}>
      <TenantChip tone={row.result.tone}>{row.result.label}</TenantChip>
      {row.resultNote !== null && (
        <span className="text-sm text-muted-foreground md:text-base">
          {row.resultNote}
        </span>
      )}
    </div>
  );
}
