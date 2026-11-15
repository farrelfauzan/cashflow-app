"use client";

import { TenantChip } from "@/components/client/tenant/tenant-chip";

import type { PendingDepositView } from "./type";
import { PENDING_BADGE, PENDING_NOTE } from "./utils";

type PendingDepositCardProps = {
  view: PendingDepositView;
};

// A deposit waiting for an admin to count it (board phone 2): what was handed over, when, and how
// many receipts; receipts taken after it go into the next one.
export function PendingDepositCard({ view }: PendingDepositCardProps) {
  return (
    <section
      aria-label={PENDING_BADGE}
      className="flex flex-col items-start gap-2 rounded-[14px] border-[1.5px] border-primary bg-card px-4 py-3.5"
    >
      <TenantChip tone="active">{PENDING_BADGE}</TenantChip>
      <span className="text-[26px] font-bold tabular-nums">{view.amount}</span>
      <span className="text-[15px] text-muted-foreground md:text-base">
        {view.detail}
      </span>
      <p className="text-[15px] leading-normal md:text-base">{PENDING_NOTE}</p>
    </section>
  );
}
