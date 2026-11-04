"use client";

import { orgPath } from "@/lib/org-path";
import { cn } from "@/lib/utils";
import { METER_LIST_LABEL, METER_TABLE_COLUMNS } from "./utils";
import type { MissingMeterRowView } from "./type";

import { MeterMissingCard } from "./meter-missing-card";
import { MeterMissingTableRow } from "./meter-missing-table-row";

type MeterMissingListProps = {
  orgId: string;
  rows: readonly MissingMeterRowView[];
  // ability.can("read", "tenant"): the link to Detail penyewa shows only then.
  canReadTenants: boolean;
};

// The leases with a meter amount missing, in the API's order: a table on iPad, cards on the
// phone. Only one of the two is displayed.
export function MeterMissingList({
  orgId,
  rows,
  canReadTenants,
}: MeterMissingListProps) {
  const hrefOf = (row: MissingMeterRowView) =>
    canReadTenants ? orgPath(orgId, `tenants/${row.tenantId}`) : null;
  return (
    <>
      <section className="hidden overflow-hidden rounded-2xl border bg-card lg:block">
        <div
          aria-hidden="true"
          className={cn(
            "grid min-h-11 items-center gap-3 border-b bg-muted/40 px-5 text-base font-semibold text-muted-foreground",
            METER_TABLE_COLUMNS,
          )}
        >
          <span>Penyewa</span>
          <span>Unit</span>
          <span>Belum dicatat</span>
          <span>Tagihan berikutnya</span>
          <span>Aksi</span>
        </div>
        <ul aria-label={METER_LIST_LABEL}>
          {rows.map((row) => (
            <MeterMissingTableRow
              key={row.leaseId}
              tenantHref={hrefOf(row)}
              row={row}
            />
          ))}
        </ul>
      </section>

      <ul
        aria-label={METER_LIST_LABEL}
        className="flex flex-col gap-2.5 lg:hidden"
      >
        {rows.map((row) => (
          <MeterMissingCard
            key={row.leaseId}
            tenantHref={hrefOf(row)}
            row={row}
          />
        ))}
      </ul>
    </>
  );
}
