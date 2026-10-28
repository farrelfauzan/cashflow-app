"use client";

import Link from "next/link";

import { unitText, FLAGGED_CHIP } from "./utils";
import type { TenantRowView } from "./type";

import { TenantChip } from "./tenant-chip";

type TenantCardProps = {
  href: string;
  row: TenantRowView;
};

// One tenant on the phone (board Penyewa-Phone.dc.html). The whole card opens the tenant.
export function TenantCard({ href, row }: TenantCardProps) {
  return (
    <li>
      <Link
        href={href}
        className="flex flex-col gap-1.5 rounded-[14px] border bg-card px-4 py-3.5 focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
      >
        <div className="flex items-start justify-between gap-2.5">
          <div className="flex min-w-0 flex-col gap-px">
            <span className="text-[17px] font-semibold">{row.name}</span>
            <span className="text-sm text-muted-foreground">{row.phone}</span>
          </div>
          <div className="flex shrink-0 flex-col items-end gap-0.5">
            <TenantChip tone={row.status.tone}>{row.status.label}</TenantChip>
            {row.status.detail !== null && (
              <span className="text-sm text-muted-foreground">
                {row.status.detail}
              </span>
            )}
          </div>
        </div>
        <span className="text-[15px]">{unitText(row)}</span>
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm text-muted-foreground">
            Penagih: {row.collector ?? "tidak ada"}
          </span>
          {row.flagged && (
            <TenantChip tone="warning">{FLAGGED_CHIP}</TenantChip>
          )}
        </div>
      </Link>
    </li>
  );
}
