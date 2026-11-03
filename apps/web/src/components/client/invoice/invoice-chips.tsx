"use client";

import { TenantChip } from "@/components/client/tenant/tenant-chip";
import type { InvoiceChip } from "./type";

type InvoiceChipsProps = {
  chips: readonly InvoiceChip[];
};

// The invoice's status chip, and "Lewat rencana keluar" after it when the lease is past its
// planned end.
export function InvoiceChips({ chips }: InvoiceChipsProps) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {chips.map((chip) => (
        <TenantChip key={chip.label} tone={chip.tone}>
          {chip.label}
        </TenantChip>
      ))}
    </div>
  );
}
