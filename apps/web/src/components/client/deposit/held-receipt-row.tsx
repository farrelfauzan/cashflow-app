"use client";

import { RetakePhotoButton } from "@/components/client/receipt-retake/retake-photo-button";
import { TenantChip } from "@/components/client/tenant/tenant-chip";

import type { HeldReceiptView } from "./type";

type HeldReceiptRowProps = {
  orgId: string;
  view: HeldReceiptView;
};

// One receipt: the tenant, their units and when it was recorded, the amount and its photo, with
// "Ambil ulang" under a photo the scan refused.
export function HeldReceiptRow({ orgId, view }: HeldReceiptRowProps) {
  return (
    <li className="flex justify-between gap-2.5 border-t px-4 py-3 first:border-t-0">
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-base font-semibold">{view.tenantName}</span>
        <span className="text-sm text-muted-foreground md:text-base">
          {view.detail}
        </span>
      </div>
      <div className="flex shrink-0 flex-col items-end gap-1">
        <span className="text-base font-semibold tabular-nums">
          {view.amount}
        </span>
        <TenantChip tone={view.photo.tone}>{view.photo.label}</TenantChip>
        <RetakePhotoButton orgId={orgId} payment={view.retake} />
      </div>
    </li>
  );
}
