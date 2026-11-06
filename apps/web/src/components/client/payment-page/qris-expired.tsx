"use client";

import { Clock, RefreshCw, TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";

import { QrisAmountRow } from "./qris-amount-row";
import {
  QRIS_EXPIRED_NOTE,
  QRIS_EXPIRED_TEXT,
  QRIS_EXPIRED_TITLE,
  QRIS_NEW_TEXT,
} from "./utils";

type QrisExpiredProps = {
  // The expired QR's amount, formatted; null when the API left it out.
  amount: string | null;
  onNewQr: () => void;
};

// The QR of this visit ran out (board "QR kedaluwarsa"): "Buat QR baru" is the primary action.
export function QrisExpired({ amount, onNewQr }: QrisExpiredProps) {
  return (
    <>
      <div className="flex flex-col items-center gap-3 rounded-xl border bg-card px-4 py-4 shadow-xs md:px-5">
        {amount !== null && <QrisAmountRow amount={amount} />}
        <div className="flex size-[190px] flex-col items-center justify-center gap-2 rounded-md bg-muted text-muted-foreground">
          <Clock aria-hidden="true" className="size-10" />
          <span className="text-sm font-semibold md:text-base">
            {QRIS_EXPIRED_TITLE}
          </span>
        </div>
        <p className="text-center text-sm leading-normal md:text-base">
          {QRIS_EXPIRED_TEXT}
        </p>
        <Button
          type="button"
          size="lg"
          className="w-full font-semibold"
          onClick={onNewQr}
        >
          <RefreshCw aria-hidden="true" />
          {QRIS_NEW_TEXT}
        </Button>
      </div>
      <p className="flex items-start gap-2 text-sm leading-normal text-muted-foreground md:text-base">
        <TriangleAlert
          aria-hidden="true"
          className="mt-0.5 size-4 shrink-0 text-warning"
        />
        {QRIS_EXPIRED_NOTE}
      </p>
    </>
  );
}
