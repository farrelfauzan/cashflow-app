"use client";

import type { CashPaidLineView } from "./type";

type CashPaidLineProps = {
  line: CashPaidLineView;
};

// One invoice the cash paid: "Tagihan November 2026" and what it paid on it.
export function CashPaidLine({ line }: CashPaidLineProps) {
  return (
    <li className="flex flex-wrap items-baseline justify-between gap-x-3">
      <span className="text-[15px] md:text-base">{line.name}</span>
      <span className="text-[15px] font-semibold tabular-nums md:text-base">
        {line.amount}
      </span>
    </li>
  );
}
