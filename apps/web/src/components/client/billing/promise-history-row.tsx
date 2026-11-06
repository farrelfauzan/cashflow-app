"use client";

import { TenantChip } from "@/components/client/tenant/tenant-chip";
import type { PromiseHistoryRowView } from "./type";

type PromiseHistoryRowProps = {
  promise: PromiseHistoryRowView;
};

// One promise in "Riwayat janji": the promised day and amount, who made it and when, and its
// status chip on the right.
export function PromiseHistoryRow({ promise }: PromiseHistoryRowProps) {
  return (
    <li className="flex items-start justify-between gap-3 border-t pt-3 first:border-t-0 first:pt-0">
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <h3 className="text-base font-bold md:text-[17px]">{promise.title}</h3>
        <span className="text-sm text-muted-foreground md:text-base">
          {promise.byline}
        </span>
      </div>
      <TenantChip tone={promise.status.tone}>{promise.status.label}</TenantChip>
    </li>
  );
}
