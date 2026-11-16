"use client";

import { TenantChip } from "@/components/client/tenant/tenant-chip";

import type { FieldTodayRowView } from "./type";

type FieldTodayRowProps = {
  view: FieldTodayRowView;
};

// One tenant to collect from (board HP-Tagihan, column 1): name and units, why they are on the
// list, what they still owe, and the cash taken from them today with its photo. "Terima tunai"
// and "Catat janji bayar" come with their sheets.
export function FieldTodayRow({ view }: FieldTodayRowProps) {
  return (
    <li className="flex flex-col gap-2 rounded-[14px] border bg-card px-3.5 py-3 md:px-4 md:py-3.5">
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 flex-col gap-0.5">
          <span className="text-base font-semibold md:text-[17px]">
            {view.tenantName}
          </span>
          {view.place !== "" && (
            <span className="text-sm text-muted-foreground md:text-base">
              {view.place}
            </span>
          )}
        </div>
        <TenantChip tone={view.reason.tone}>{view.reason.label}</TenantChip>
      </div>
      <span className="text-[15px] font-semibold tabular-nums md:text-base">
        {view.owed}
      </span>
      {view.cash !== null && (
        <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-2">
          <span className="text-sm text-muted-foreground tabular-nums md:text-base">
            {view.cash.text}
          </span>
          <TenantChip tone={view.cash.photo.tone}>
            {view.cash.photo.label}
          </TenantChip>
        </div>
      )}
    </li>
  );
}
