"use client";

import { TenantChip } from "./tenant-chip";

type LeaseChargesProps = {
  charges: readonly { id: string; label: string }[];
};

// "Biaya tambahan tetap": the lease's fixed extras, "Wi-Fi Rp50.000". Nothing when it has none.
export function LeaseCharges({ charges }: LeaseChargesProps) {
  if (charges.length === 0) return null;
  return (
    <div className="flex flex-col gap-1.5 border-t py-2.5 md:border-t-0 md:py-0">
      <span className="text-sm text-muted-foreground md:text-base">
        Biaya tambahan tetap
      </span>
      <ul className="flex flex-wrap gap-2">
        {charges.map((charge) => (
          <li key={charge.id}>
            <TenantChip tone="neutral">{charge.label}</TenantChip>
          </li>
        ))}
      </ul>
    </div>
  );
}
