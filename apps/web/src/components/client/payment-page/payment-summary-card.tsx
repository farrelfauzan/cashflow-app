"use client";

import { SummaryRow } from "./summary-row";
import type { SummaryRowView } from "./type";
import { CREDIT_LABEL, SUMMARY_LABEL, TOTAL_LABEL } from "./utils";

type PaymentSummaryCardProps = {
  rows: readonly SummaryRowView[];
  total: string;
  credit: string | null;
};

// Every open invoice, oldest first, then the one total the tenant owes, from the API.
export function PaymentSummaryCard({
  rows,
  total,
  credit,
}: PaymentSummaryCardProps) {
  return (
    <section
      aria-label={SUMMARY_LABEL}
      className="flex flex-col gap-3 rounded-xl border bg-card px-4 py-3.5 shadow-xs md:px-5 md:py-4"
    >
      <ul className="flex flex-col gap-2.5">
        {rows.map((row) => (
          <SummaryRow key={row.id} row={row} />
        ))}
      </ul>
      <div aria-hidden="true" className="h-px bg-border" />
      <div className="flex items-center justify-between gap-3">
        <span className="text-[15px] font-semibold md:text-base">
          {TOTAL_LABEL}
        </span>
        <span className="text-2xl font-bold whitespace-nowrap">{total}</span>
      </div>
      {credit !== null && (
        <div className="flex items-center justify-between gap-3">
          <span className="text-sm text-muted-foreground md:text-base">
            {CREDIT_LABEL}
          </span>
          <span className="text-sm font-semibold whitespace-nowrap md:text-base">
            {credit}
          </span>
        </div>
      )}
    </section>
  );
}
