"use client";

import { orgPath } from "@/lib/organization/org-menu";
import type { TenantRowView } from "./type";

import { TenantCard } from "./tenant-card";
import { TenantTableRow } from "./tenant-table-row";

type TenantListProps = {
  orgId: string;
  rows: readonly TenantRowView[];
};

// The tenants, in the API's order: a table on iPad, cards on the phone (boards Penyewa.dc.html
// and Penyewa-Phone.dc.html). Only one of the two is displayed.
export function TenantList({ orgId, rows }: TenantListProps) {
  return (
    <>
      <section className="hidden overflow-hidden rounded-2xl border bg-card lg:block">
        <div
          aria-hidden="true"
          className="grid min-h-11 grid-cols-[minmax(0,1.25fr)_minmax(0,1.1fr)_minmax(0,1.2fr)_130px_20px] items-center gap-4 border-b bg-muted/40 px-5 text-base font-semibold text-muted-foreground"
        >
          <span>Penyewa</span>
          <span>Unit</span>
          <span>Penagih</span>
          <span>Status</span>
          <span />
        </div>
        <ul aria-label="Daftar penyewa">
          {rows.map((row) => (
            <TenantTableRow
              key={row.id}
              href={orgPath(orgId, `tenants/${row.id}`)}
              row={row}
            />
          ))}
        </ul>
      </section>

      <ul
        aria-label="Daftar penyewa"
        className="flex flex-col gap-2.5 lg:hidden"
      >
        {rows.map((row) => (
          <TenantCard
            key={row.id}
            href={orgPath(orgId, `tenants/${row.id}`)}
            row={row}
          />
        ))}
      </ul>
    </>
  );
}
