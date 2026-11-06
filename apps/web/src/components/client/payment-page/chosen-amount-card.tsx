"use client";

import { Button } from "@/components/ui/button";

import { PARTIAL_CHANGE_TEXT, QRIS_AMOUNT_LABEL } from "./utils";

type ChosenAmountCardProps = {
  // The chosen amount, formatted.
  amount: string;
  // "Sisa Rp1.250.000 tetap jadi tagihan."; null when the amount pays everything.
  remainderLine: string | null;
  // "Ubah nominal"; null while a QR is being made, live or checked.
  onChange: (() => void) | null;
};

// "Jumlah Rp1.000.000" of "Bayar sebagian" above the tabs (board "QRIS bayar sebagian"), what
// stays owed, and "Ubah nominal".
export function ChosenAmountCard({
  amount,
  remainderLine,
  onChange,
}: ChosenAmountCardProps) {
  return (
    <section className="flex flex-col gap-1.5 rounded-xl border bg-card px-4 py-3.5 shadow-xs md:px-5 md:py-4">
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm text-muted-foreground md:text-base">
          {QRIS_AMOUNT_LABEL}
        </span>
        <span className="text-2xl font-bold whitespace-nowrap">{amount}</span>
      </div>
      {remainderLine !== null && (
        <p className="text-sm md:text-base">{remainderLine}</p>
      )}
      {onChange !== null && (
        <Button
          type="button"
          variant="link"
          className="self-start px-0 font-semibold md:text-[17px]"
          onClick={onChange}
        >
          {PARTIAL_CHANGE_TEXT}
        </Button>
      )}
    </section>
  );
}
