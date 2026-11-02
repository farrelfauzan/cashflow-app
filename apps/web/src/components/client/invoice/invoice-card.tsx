"use client";

import { unitText } from "./utils";
import type { InvoiceRowView } from "./type";

import { InvoiceChips } from "./invoice-chips";
import { InvoiceTenantName } from "./invoice-tenant-name";

type InvoiceCardProps = {
  // Detail penyewa, or null when the member may not read tenants.
  tenantHref: string | null;
  row: InvoiceRowView;
};

// One invoice on the phone: the tenant, where the unit is, the chips, the amounts and the due
// date.
export function InvoiceCard({ tenantHref, row }: InvoiceCardProps) {
  return (
    <li className="flex flex-col gap-1.5 rounded-[14px] border bg-card px-4 py-3.5">
      <h3 className="min-w-0 text-[17px] font-semibold">
        <InvoiceTenantName name={row.tenantName} href={tenantHref} />
      </h3>
      <span className="text-[15px]">{unitText(row)}</span>
      <InvoiceChips chips={row.chips} />
      <span className="text-[15px]">
        Tagihan {row.billed} · Sisa{" "}
        <span className="font-semibold">{row.outstanding}</span>
      </span>
      <span className="text-sm text-muted-foreground">
        Dibayar {row.paid} · Jatuh tempo {row.dueDate}
      </span>
    </li>
  );
}
