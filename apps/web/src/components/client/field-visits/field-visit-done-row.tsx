"use client";

import { TenantChip } from "@/components/client/tenant/tenant-chip";

import type { FieldVisitRowView } from "./type";

type FieldVisitDoneRowProps = {
  view: FieldVisitRowView;
};

// A tenant whose visit was recorded today (board HP-Kunjungan, "Sudah dicatat hari ini"): compact,
// with what was recorded and when. Nothing more to do on it today.
export function FieldVisitDoneRow({ view }: FieldVisitDoneRowProps) {
  return (
    <li className="flex items-start justify-between gap-2 rounded-[14px] border bg-card px-3.5 py-3 md:px-4">
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-base font-semibold md:text-[17px]">
          {view.tenantName}
        </span>
        {view.place !== "" && (
          <span className="text-sm text-muted-foreground md:text-base">
            {view.place}
          </span>
        )}
        {view.doneLine !== null && (
          <span className="text-sm text-muted-foreground tabular-nums md:text-base">
            {view.doneLine}
          </span>
        )}
      </div>
      {view.doneChip !== null && (
        <TenantChip tone={view.doneChip.tone}>{view.doneChip.label}</TenantChip>
      )}
    </li>
  );
}
