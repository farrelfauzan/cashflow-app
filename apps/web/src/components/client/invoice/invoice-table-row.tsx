"use client";

import { TABLE_COLUMN_GAP } from "@/components/client/list/utils";
import { cn } from "@/lib/utils";
import type { InvoiceRowView } from "./type";
import { TABLE_COLUMNS } from "./utils";

import { InvoiceChips } from "./invoice-chips";
import { InvoiceTenantName } from "./invoice-tenant-name";

type InvoiceTableRowProps = {
  // Detail penyewa, or null when the member may not read tenants.
  tenantHref: string | null;
  row: InvoiceRowView;
};

// One invoice on iPad: tenant, unit and property, billed, paid, outstanding, then the status with
// the due date under it (a column of its own does not fit next to the amounts).
// The columns match the header in invoice-list.tsx.
export function InvoiceTableRow({ tenantHref, row }: InvoiceTableRowProps) {
  return (
    <li
      className={cn(
        "grid items-center border-t px-5 py-3 first:border-t-0",
        TABLE_COLUMNS,
        TABLE_COLUMN_GAP,
      )}
    >
      <InvoiceTenantName
        name={row.tenantName}
        href={tenantHref}
        className="min-w-0 text-[17px] font-semibold"
      />
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-base font-semibold">{row.unit}</span>
        <span className="text-base text-muted-foreground">{row.property}</span>
      </div>
      <span className="text-right text-base">{row.billed}</span>
      <span className="text-right text-base">{row.paid}</span>
      <span className="text-right text-base font-semibold">
        {row.outstanding}
      </span>
      <div className="flex flex-col items-start gap-1">
        <InvoiceChips chips={row.chips} />
        <span className="text-base text-muted-foreground">
          Jatuh tempo {row.dueDate}
        </span>
      </div>
    </li>
  );
}
