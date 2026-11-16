"use client";

import type { ReactNode } from "react";

import { formatRupiah } from "@/lib/format/rupiah";
import { cn } from "@/lib/utils";

import { receiptCountText } from "./utils";

type CashHeldCardProps = {
  // "Tunai di tangan sejak setoran Kamis, 12 Nov", or on Tagihan hari ini "Tunai di tangan,
  // belum disetor".
  sinceLine: string;
  amount: number;
  count: number;
  // "strong": the accent card at the top of Tagihan hari ini (board HP-Tagihan), with the count
  // beside the total and `action` under it. "plain" (the default): Setor tunai's card.
  tone?: "plain" | "strong";
  // A link under the total ("Setor ke admin"); only on the strong card.
  action?: ReactNode;
};

// The cash the karyawan holds now: since when, the total and how many receipts (getMyFieldCash).
export function CashHeldCard({
  sinceLine,
  amount,
  count,
  tone = "plain",
  action,
}: CashHeldCardProps) {
  const strong = tone === "strong";
  const secondaryText = strong
    ? "text-sm text-primary-foreground/85 md:text-base"
    : "text-sm text-muted-foreground md:text-base";
  return (
    <section
      aria-label="Tunai di tangan"
      className={cn(
        "flex flex-col rounded-[14px] px-4 py-3.5",
        strong
          ? "gap-2.5 bg-primary-strong text-primary-foreground"
          : "gap-0.5 border bg-card",
      )}
    >
      <span className={secondaryText}>{sinceLine}</span>
      {strong ? (
        <div className="flex items-end justify-between gap-2.5">
          <span className="text-[26px] leading-tight font-bold tabular-nums">
            {formatRupiah(amount)}
          </span>
          <span className={cn(secondaryText, "whitespace-nowrap")}>
            {receiptCountText(count)}
          </span>
        </div>
      ) : (
        <>
          <span className="text-[26px] font-bold tabular-nums">
            {formatRupiah(amount)}
          </span>
          <span className={secondaryText}>{receiptCountText(count)}</span>
        </>
      )}
      {strong && action}
    </section>
  );
}
