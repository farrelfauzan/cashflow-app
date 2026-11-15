"use client";

import { formatRupiah } from "@/lib/format/rupiah";

import { receiptCountText } from "./utils";

type CashHeldCardProps = {
  // "Tunai di tangan sejak setoran Kamis, 12 Nov".
  sinceLine: string;
  amount: number;
  count: number;
};

// The cash the karyawan holds now: since when, the total and how many receipts (getMyFieldCash).
export function CashHeldCard({ sinceLine, amount, count }: CashHeldCardProps) {
  return (
    <section
      aria-label="Tunai di tangan"
      className="flex flex-col gap-0.5 rounded-[14px] border bg-card px-4 py-3.5"
    >
      <span className="text-sm text-muted-foreground md:text-base">
        {sinceLine}
      </span>
      <span className="text-[26px] font-bold tabular-nums">
        {formatRupiah(amount)}
      </span>
      <span className="text-sm text-muted-foreground md:text-base">
        {receiptCountText(count)}
      </span>
    </section>
  );
}
