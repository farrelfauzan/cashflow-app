"use client";

import { TenantChip } from "./tenant-chip";

type LeaseChargesProps = {
  charges: readonly { id: string; label: string }[];
  // "Listrik, Air dari meteran"; null when the lease expects no metered amount.
  metered: string | null;
};

// The lease's extras: "Biaya tambahan tetap" ("Wi-Fi Rp50.000"), and the metered types it expects
// every month ("Listrik, Air dari meteran"). Nothing when it has neither.
export function LeaseCharges({ charges, metered }: LeaseChargesProps) {
  if (charges.length === 0 && metered === null) return null;
  return (
    <div className="flex flex-col gap-2.5 border-t py-2.5 md:flex-row md:gap-10 md:border-t-0 md:py-0">
      {charges.length > 0 && (
        <div className="flex flex-col gap-1.5">
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
      )}
      {metered !== null && (
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-muted-foreground md:text-base">
            Biaya tambahan dari meteran
          </span>
          <div className="flex">
            <TenantChip tone="neutral">{metered}</TenantChip>
          </div>
        </div>
      )}
    </div>
  );
}
