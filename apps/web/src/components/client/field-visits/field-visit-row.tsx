"use client";

import { TenantChip } from "@/components/client/tenant/tenant-chip";
import { Button } from "@/components/ui/button";

import type { FieldVisitRowView } from "./type";
import { RECORD_TEXT } from "./utils";

import { DirectionsLink } from "./directions-link";

type FieldVisitRowProps = {
  view: FieldVisitRowView;
  // The member may record outcomes (visit:record): the row offers "Catat hasil".
  canRecord: boolean;
  onRecord: (view: FieldVisitRowView) => void;
};

// One tenant to visit (board HP-Kunjungan, column 1): name and units, why the visit is open,
// what they owe, "Tadi tidak bertemu" after a visit that found nobody, then "Arahkan" and the
// row's primary, "Catat hasil".
export function FieldVisitRow({
  view,
  canRecord,
  onRecord,
}: FieldVisitRowProps) {
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
        {view.reasons.length > 0 && (
          <div className="flex shrink-0 flex-col items-end gap-1">
            {view.reasons.map((reason) => (
              <TenantChip key={reason.label} tone={reason.tone}>
                {reason.label}
              </TenantChip>
            ))}
          </div>
        )}
      </div>
      <span className="text-[15px] font-semibold tabular-nums md:text-base">
        {view.owed}
      </span>
      {view.notHomeLine !== null && (
        <span className="text-sm text-muted-foreground tabular-nums md:text-base">
          {view.notHomeLine}
        </span>
      )}
      {(view.directionsUrl !== null || canRecord) && (
        <div className="flex gap-2">
          {view.directionsUrl !== null && (
            <DirectionsLink href={view.directionsUrl} />
          )}
          {canRecord && (
            <Button
              type="button"
              onClick={() => onRecord(view)}
              className="h-12 flex-1 rounded-xl px-4 text-base font-semibold"
            >
              {RECORD_TEXT}
            </Button>
          )}
        </div>
      )}
    </li>
  );
}
