"use client";

import { TenantChip } from "@/components/client/tenant/tenant-chip";
import { METER_CHIP_LABEL, unitText } from "./utils";
import type { MissingMeterRowView } from "./type";

import { MeterTenantLink } from "./meter-tenant-link";

type MeterMissingCardProps = {
  // Detail penyewa, or null when the member may not read tenants.
  tenantHref: string | null;
  row: MissingMeterRowView;
};

// One lease with a meter amount missing, on the phone: the tenant, where the unit is, the chip,
// what is missing, the next invoice and the link to Detail penyewa.
export function MeterMissingCard({ tenantHref, row }: MeterMissingCardProps) {
  return (
    <li className="flex flex-col gap-1.5 rounded-[14px] border bg-card px-4 py-3.5">
      <h3 className="min-w-0 text-[17px] font-semibold">{row.tenantName}</h3>
      <span className="text-[15px]">{unitText(row)}</span>
      <div className="flex flex-wrap items-center gap-1.5">
        <TenantChip tone="warning">{METER_CHIP_LABEL}</TenantChip>
      </div>
      <span className="text-[15px]">Belum dicatat: {row.missing}</span>
      <span className="text-sm text-muted-foreground">
        Tagihan berikutnya {row.nextInvoice}
      </span>
      {tenantHref !== null && <MeterTenantLink href={tenantHref} />}
    </li>
  );
}
