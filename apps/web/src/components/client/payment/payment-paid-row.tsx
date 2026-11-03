"use client";

import { TenantChip } from "@/components/client/tenant/tenant-chip";
import type { PaidRowView } from "./type";

type PaymentPaidRowProps = {
  row: PaidRowView;
};

// One invoice the payment paid: "Tagihan Oktober 2026 · Rp700.000" and "Lunas" or what is left.
export function PaymentPaidRow({ row }: PaymentPaidRowProps) {
  return (
    <li className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1.5 border-t pt-3 first:border-t-0 first:pt-0">
      <span className="text-base font-semibold md:text-[17px]">{row.text}</span>
      <TenantChip tone={row.chip.settled ? "active" : "warning"}>
        {row.chip.label}
      </TenantChip>
    </li>
  );
}
