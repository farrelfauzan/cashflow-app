"use client";

import { TABLE_COLUMN_GAP } from "@/components/client/list/utils";
import { orgPath } from "@/lib/org-path";
import { cn } from "@/lib/utils";
import { LIST_LABEL, TABLE_COLUMNS } from "./utils";
import type { InvoiceRowView } from "./type";

import { InvoiceCard } from "./invoice-card";
import { InvoiceTableRow } from "./invoice-table-row";

type InvoiceListProps = {
  orgId: string;
  rows: readonly InvoiceRowView[];
  // ability.can("read", "tenant"): the name links to Detail penyewa only then.
  canReadTenants: boolean;
};

// The invoices in the API's order: a table on iPad, cards on the phone. Only one of the two is
// displayed.
export function InvoiceList({ orgId, rows, canReadTenants }: InvoiceListProps) {
  const hrefOf = (row: InvoiceRowView) =>
    canReadTenants ? orgPath(orgId, `tenants/${row.tenantId}`) : null;
  return (
    <>
      <section className="hidden overflow-hidden rounded-2xl border bg-card lg:block">
        <div
          aria-hidden="true"
          className={cn(
            "grid min-h-11 items-center border-b bg-muted/40 px-5 text-base font-semibold text-muted-foreground",
            TABLE_COLUMNS,
            TABLE_COLUMN_GAP,
          )}
        >
          <span>Penyewa</span>
          <span>Unit</span>
          <span className="text-right">Tagihan</span>
          <span className="text-right">Dibayar</span>
          <span className="text-right">Sisa</span>
          <span>Status</span>
        </div>
        <ul aria-label={LIST_LABEL}>
          {rows.map((row) => (
            <InvoiceTableRow key={row.id} tenantHref={hrefOf(row)} row={row} />
          ))}
        </ul>
      </section>

      <ul aria-label={LIST_LABEL} className="flex flex-col gap-2.5 lg:hidden">
        {rows.map((row) => (
          <InvoiceCard key={row.id} tenantHref={hrefOf(row)} row={row} />
        ))}
      </ul>
    </>
  );
}
