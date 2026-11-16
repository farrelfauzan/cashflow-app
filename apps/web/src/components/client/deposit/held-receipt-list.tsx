"use client";

import type { HeldCashPayment } from "@/lib/api/generated/model";

import { heldReceiptView, moreReceiptsText, RECEIPTS_LABEL } from "./utils";

import { HeldReceiptRow } from "./held-receipt-row";

type HeldReceiptListProps = {
  orgId: string;
  // One page of listMyUndepositedCash, oldest first (the API's order).
  items: readonly HeldCashPayment[];
  // Every receipt held, of which `items` is the first page.
  total: number;
};

// The receipts the deposit covers, each with its photo; one the scan refused offers "Ambil
// ulang" (decision 46.10).
export function HeldReceiptList({ orgId, items, total }: HeldReceiptListProps) {
  const more = moreReceiptsText(total, items.length);
  return (
    <div className="flex flex-col gap-2">
      <ul
        aria-label={RECEIPTS_LABEL}
        className="flex flex-col rounded-[14px] border bg-card"
      >
        {items.map((payment) => (
          <HeldReceiptRow
            key={payment.paymentId}
            orgId={orgId}
            view={heldReceiptView(payment)}
          />
        ))}
      </ul>
      {more !== null && (
        <p className="px-1 text-sm text-muted-foreground md:text-base">
          {more}
        </p>
      )}
    </div>
  );
}
