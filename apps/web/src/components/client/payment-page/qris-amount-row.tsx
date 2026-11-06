"use client";

import { QRIS_AMOUNT_LABEL } from "./utils";

type QrisAmountRowProps = {
  // The QR's own amount, formatted (QrisStatus.amount).
  amount: string;
};

// "Jumlah" and the amount of the QR on screen, above the image.
export function QrisAmountRow({ amount }: QrisAmountRowProps) {
  return (
    <div className="flex w-full items-center justify-between gap-3">
      <span className="text-sm text-muted-foreground md:text-base">
        {QRIS_AMOUNT_LABEL}
      </span>
      <span className="text-xl font-bold whitespace-nowrap">{amount}</span>
    </div>
  );
}
