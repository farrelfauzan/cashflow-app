"use client";

import { TenantChip } from "@/components/client/tenant/tenant-chip";
import { cn } from "@/lib/utils";
import { METER_CHIP_LABEL, METER_TABLE_COLUMNS } from "./utils";
import type { MissingMeterRowView } from "./type";

import { MeterTenantLink } from "./meter-tenant-link";

type MeterMissingTableRowProps = {
  // Detail penyewa, or null when the member may not read tenants.
  tenantHref: string | null;
  row: MissingMeterRowView;
};

// One lease with a meter amount missing, on iPad: tenant, unit and property, what is missing with
// the warning chip under it, the next invoice, and the link to Detail penyewa. The columns match
// the header in meter-missing-list.tsx.
export function MeterMissingTableRow({
  tenantHref,
  row,
}: MeterMissingTableRowProps) {
  return (
    <li
      className={cn(
        "grid items-center gap-3 border-t px-5 py-3 first:border-t-0",
        METER_TABLE_COLUMNS,
      )}
    >
      <span className="min-w-0 text-[17px] font-semibold">
        {row.tenantName}
      </span>
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-base font-semibold">{row.unit}</span>
        <span className="text-base text-muted-foreground">{row.property}</span>
      </div>
      <div className="flex flex-col items-start gap-1">
        <span className="text-base">{row.missing}</span>
        <TenantChip tone="warning">{METER_CHIP_LABEL}</TenantChip>
      </div>
      <span className="text-base">{row.nextInvoice}</span>
      <div>{tenantHref !== null && <MeterTenantLink href={tenantHref} />}</div>
    </li>
  );
}
