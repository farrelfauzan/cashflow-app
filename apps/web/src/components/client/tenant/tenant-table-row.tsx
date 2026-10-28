"use client";

import { ChevronRight } from "lucide-react";
import Link from "next/link";

import { propertyText, NO_COLLECTOR, FLAGGED_CHIP } from "./utils";
import type { TenantRowView } from "./type";

import { TenantChip } from "./tenant-chip";

type TenantTableRowProps = {
  href: string;
  row: TenantRowView;
};

// One tenant on iPad: name and phone, unit, collector, status. The whole row opens the tenant.
export function TenantTableRow({ href, row }: TenantTableRowProps) {
  return (
    <li className="border-t first:border-t-0">
      <Link
        href={href}
        className="grid min-h-[68px] grid-cols-[minmax(0,1.25fr)_minmax(0,1.1fr)_minmax(0,1.2fr)_130px_20px] items-center gap-4 px-5 py-2 hover:bg-accent/50 focus-visible:bg-accent/50 focus-visible:outline-none"
      >
        <div className="flex min-w-0 flex-col gap-0.5">
          <span className="truncate text-[17px] font-semibold">{row.name}</span>
          <span className="text-base text-muted-foreground">{row.phone}</span>
        </div>
        <div className="flex min-w-0 flex-col gap-0.5">
          <span className="text-base font-semibold">{row.unit}</span>
          <span className="text-base text-muted-foreground">
            {propertyText(row)}
          </span>
        </div>
        <div className="flex min-w-0 flex-col items-start gap-1">
          {row.collector === null ? (
            <span className="text-base text-muted-foreground">
              {NO_COLLECTOR}
            </span>
          ) : (
            <span className="text-base">{row.collector}</span>
          )}
          {row.flagged && (
            <TenantChip tone="warning">{FLAGGED_CHIP}</TenantChip>
          )}
        </div>
        <div className="flex flex-col items-start gap-1">
          <TenantChip tone={row.status.tone}>{row.status.label}</TenantChip>
          {row.status.detail !== null && (
            <span className="text-base text-muted-foreground">
              {row.status.detail}
            </span>
          )}
        </div>
        <ChevronRight
          aria-hidden="true"
          className="size-5 text-muted-foreground"
        />
      </Link>
    </li>
  );
}
